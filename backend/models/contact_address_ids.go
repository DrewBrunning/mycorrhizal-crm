package models

import (
	"strconv"

	"github.com/google/uuid"

	"mycorrhizal/contactmodel"
)

// ensureAddressIDs gives every address on the contact a stable identifier
// (ADR 0031, issue #694: the geocode route keys on it). It runs at the top of
// deriveDenormalized, so every save path — REST, import, sync, audit undo —
// converges on "every stored address has an ID" without each writer knowing
// about it.
//
// An address that already carries an ID keeps it. One that does not inherits
// the ID of the Card address at the same position when the two agree on every
// postal field (an editor that predates ID-awareness resubmitting an
// unchanged address must not churn its ID, and the minted ID is written to the
// Card entry too so the T75 merge still sees the two as the same projection
// and keeps the entry's unprojected components). Anything else gets a fresh
// UUID.
//
// directCard is true on the cardSetDirectly path, where the Card is the
// authority and c.Addresses was just derived from it 1:1: IDs are minted on
// the Card and mirrored back onto the flat entries by index.
func (c *Contact) ensureAddressIDs(directCard bool) {
	if directCard {
		if len(c.Card.Addresses) == 0 {
			return
		}
		cardAddrs := append([]contactmodel.Address(nil), c.Card.Addresses...)
		for i := range cardAddrs {
			if cardAddrs[i].ID == "" {
				cardAddrs[i].ID = uuid.NewString()
			}
		}
		c.Card.Addresses = cardAddrs
		for i := range c.Addresses {
			if i < len(cardAddrs) {
				c.Addresses[i].ID = cardAddrs[i].ID
			}
		}
		return
	}

	var cardAddrs []contactmodel.Address // copy-on-write: only allocated if a Card entry needs an ID
	for i := range c.Addresses {
		if c.Addresses[i].ID != "" {
			continue
		}
		if i < len(c.Card.Addresses) && samePostalFields(contactAddressFromNeutral(c.Card.Addresses[i]), c.Addresses[i]) {
			id := c.Card.Addresses[i].ID
			if id == "" {
				id = uuid.NewString()
				if cardAddrs == nil {
					cardAddrs = append([]contactmodel.Address(nil), c.Card.Addresses...)
				}
				cardAddrs[i].ID = id
			}
			c.Addresses[i].ID = id
			continue
		}
		c.Addresses[i].ID = uuid.NewString()
	}
	if cardAddrs != nil {
		c.Card.Addresses = cardAddrs
	}
}

// samePostalFields compares the postal slots of two flat addresses, ignoring
// the map fields (ID, Coordinates, Sensitivity).
func samePostalFields(a, b ContactAddress) bool {
	a.ID, a.Coordinates, a.Sensitivity = "", "", ""
	b.ID, b.Coordinates, b.Sensitivity = "", "", ""
	return a == b
}

// FindAddressByID returns the contact's flat address with the given ID, its
// index, and whether it was found.
func (c *Contact) FindAddressByID(id string) (ContactAddress, int, bool) {
	if id == "" {
		return ContactAddress{}, -1, false
	}
	for i, a := range c.Addresses {
		if a.ID == id {
			return a, i, true
		}
	}
	return ContactAddress{}, -1, false
}

// SetAddressCoordinates stores a geo: URI on the address with the given ID, on
// BOTH the flat copy and the matching Card entry (matched by ID, copy-on-write
// so a shared backing array is never mutated). Writing both is what keeps the
// T75 merge from reading the save as "the caller edited this address" and
// replacing the Card entry with the lossy flat one — which would discard the
// components with no flat slot. Reports whether the flat address was found.
func (c *Contact) SetAddressCoordinates(id, coordinates string) bool {
	_, idx, ok := c.FindAddressByID(id)
	if !ok {
		return false
	}
	c.Addresses = append([]ContactAddress(nil), c.Addresses...)
	c.Addresses[idx].Coordinates = coordinates
	for i := range c.Card.Addresses {
		if c.Card.Addresses[i].ID == id {
			cardAddrs := append([]contactmodel.Address(nil), c.Card.Addresses...)
			cardAddrs[i].Coordinates = coordinates
			c.Card.Addresses = cardAddrs
			break
		}
	}
	return true
}

// ValidateAddressMapFields checks the contact-map fields (ADR 0031) on a
// submitted Card's addresses: Coordinates must be a geo: URI with an in-range
// WGS-84 latitude/longitude, Sensitivity one of normal|private|secret (empty
// means normal), and IDs unique within the card. It returns one human-readable
// reason per offending field, keyed by the request path, for ErrValidation.
func ValidateAddressMapFields(addrs []contactmodel.Address) map[string]string {
	problems := map[string]string{}
	seen := make(map[string]bool, len(addrs))
	for i, a := range addrs {
		prefix := "card.addresses[" + itoa(i) + "]."
		if a.Coordinates != "" {
			if _, _, ok := contactmodel.ParseGeoURI(a.Coordinates); !ok {
				problems[prefix+"coordinates"] = "must be a geo: URI with latitude -90..90 and longitude -180..180"
			}
		}
		switch a.Sensitivity {
		case "", RelationshipSensitivityNormal, RelationshipSensitivityPrivate, RelationshipSensitivitySecret:
		default:
			problems[prefix+"sensitivity"] = "must be one of normal, private, secret"
		}
		if len(a.ID) > 128 {
			problems[prefix+"id"] = "must be at most 128 characters"
		} else if a.ID != "" {
			if seen[a.ID] {
				problems[prefix+"id"] = "duplicates another address id on this card"
			}
			seen[a.ID] = true
		}
	}
	return problems
}

func itoa(i int) string { return strconv.Itoa(i) }
