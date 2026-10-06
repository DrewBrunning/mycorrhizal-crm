package carddav

// Issue #1434 — geo: coordinates over CardDAV.
//
// The vcard4 adapter emits a coordinate as the RFC 9554 ADR GEO parameter and
// makes its bytes correct with a post-encode splice (vcard4/adapter.go's
// geoSplice), because the pinned go-vcard cannot encode a parameter value
// containing ':' or ',' — a geo: URI always has both. That splice works for the
// file-export endpoints (which write the adapter's bytes verbatim), but NOT for
// CardDAV: go-webdav re-encodes the vcard.Card we hand it, so our bytes never
// reach the wire. The decoded card's GEO parameter is already split into
// ["geo:1", "1"], and go-webdav writes that as `GEO=geo:1;GEO=1`, where the
// colon inside the first value terminates the parameter list early — the rest
// of the line is parsed as the ADR value. A client that then PUTs the card back
// stores the mangled address.
//
// go-webdav's AddressObject carries a vcard.Card and nothing else (no raw-bytes
// escape hatch), and forking go-vcard is prohibited by
// docs/dependency-upgrade-policy.md ("never vendored by copying source in"), so
// the mismatch has to be worked around at this boundary.
//
// The workaround: carry the coordinate as a standalone GEO property (RFC 6350
// defines GEO as a property of value type uri, cardinality 0..*), tagged with
// the ADR's PROP-ID so the per-address association survives. go-vcard round-
// trips a GEO *property* value correctly because property values — unlike
// parameter values — may contain the ':' and (go-vcard-escaped) ',' of a geo:
// URI. On the way back in, the coordinate is read off the property and applied
// to the neutral address; on the way out it replaces the unrepresentable ADR
// parameter.

import (
	"strings"

	"github.com/emersion/go-vcard"

	"mycorrhizal/contactmodel"
	"mycorrhizal/vcard4"
)

// isVCard4 reports whether a decoded card declares vCard 4.0. Only the 4.0
// adapter uses the ADR GEO parameter; the 3.0 adapter emits and imports a
// standalone GEO property in the `lat;lon` form, which go-vcard already round-
// trips, so the workaround must not touch a 3.0 card.
func isVCard4(card vcard.Card) bool {
	return strings.HasPrefix(strings.TrimSpace(card.Value(vcard.FieldVersion)), "4")
}

// adrGeoParamsToProperties is the read-side (GET) half: it moves every ADR GEO
// parameter onto a standalone GEO property carrying the ADR's PROP-ID, so
// go-webdav can encode a coordinate it otherwise cannot. A no-op for a card
// with no ADR GEO parameters (e.g. vCard 3.0).
func adrGeoParamsToProperties(card vcard.Card) {
	for _, adr := range card[vcard4.PropAdr] {
		raw, ok := adr.Params[vcard4.ParamGeo]
		if !ok || len(raw) == 0 {
			continue
		}
		delete(adr.Params, vcard4.ParamGeo)

		coord := normalizeGeoCoordinate(strings.Join(raw, ","))
		if coord == "" {
			continue // unparseable: drop rather than re-emit a possibly-broken value
		}
		geo := &vcard.Field{Value: coord}
		if id := adr.Params.Get(vcard4.ParamPropID); id != "" {
			geo.Params = vcard.Params{vcard4.ParamPropID: []string{id}}
		}
		card[vcard4.PropGeo] = append(card[vcard4.PropGeo], geo)
	}
}

// takeGeoCoordinates is the write-side (PUT) half: it removes every ADR GEO
// parameter, and every standalone GEO property that applyGeoCoordinates will
// actually attach to an address, from card, returning the coordinates keyed by
// PROP-ID and, for a client that stripped PROP-ID, in the order seen. Callers
// apply them to the imported neutral addresses with applyGeoCoordinates.
//
// GEO is a contact-level property (RFC 6350, cardinality *), so a standalone
// GEO that no address will claim — no ADR carries its PROP-ID, or there are more
// positional GEOs than addresses, or its value is not a geo: URI — is left on
// the card: the vcard4 adapter does not map a standalone GEO, so it keeps it in
// Record.Passthrough.VCard and the GET path re-emits it, exactly as before
// issue #1434. Consuming it unconditionally silently deleted it.
func takeGeoCoordinates(card vcard.Card) (byID map[string]string, ordered []string) {
	byID = map[string]string{}
	ordered = []string{}

	adrIDs := map[string]bool{}
	for _, adr := range card[vcard4.PropAdr] {
		if id := adr.Params.Get(vcard4.ParamPropID); id != "" {
			adrIDs[id] = true
		}
	}
	addrCount := len(card[vcard4.PropAdr])

	type entry struct {
		id    string
		field *vcard.Field // the standalone GEO property; nil for an ADR parameter
	}
	var entries []entry

	collect := func(raw []string, id string, field *vcard.Field) bool {
		coord := normalizeGeoCoordinate(strings.Join(raw, ","))
		if coord == "" {
			return false
		}
		ordered = append(ordered, coord)
		if id != "" {
			byID[id] = coord
		}
		entries = append(entries, entry{id: id, field: field})
		return true
	}

	var kept []*vcard.Field
	for _, geo := range card[vcard4.PropGeo] {
		if !collect([]string{geo.Value}, geo.Params.Get(vcard4.ParamPropID), geo) {
			kept = append(kept, geo) // not a coordinate: keep verbatim
		}
	}

	for _, adr := range card[vcard4.PropAdr] {
		raw, ok := adr.Params[vcard4.ParamGeo]
		if !ok {
			continue
		}
		collect(raw, adr.Params.Get(vcard4.ParamPropID), nil)
		delete(adr.Params, vcard4.ParamGeo)
	}

	// Mirror applyGeoCoordinates: by PROP-ID when any coordinate carried one,
	// otherwise positionally over the addresses.
	positional := len(byID) == 0
	for i, e := range entries {
		if e.field == nil {
			continue
		}
		if (positional && i < addrCount) || (!positional && e.id != "" && adrIDs[e.id]) {
			continue // applied to an address: consumed
		}
		kept = append(kept, e.field)
	}

	if len(kept) == 0 {
		delete(card, vcard4.PropGeo)
	} else {
		card[vcard4.PropGeo] = kept
	}
	return byID, ordered
}

// applyGeoCoordinates sets the coordinate on each imported address, matching by
// the same ID the vcard4 adapter derived from the ADR's PROP-ID. If the client
// preserved no PROP-ID at all, the coordinates are applied positionally.
func applyGeoCoordinates(record *contactmodel.Record, byID map[string]string, ordered []string) {
	if record == nil {
		return
	}
	for i := range record.Card.Addresses {
		if coord, ok := byID[record.Card.Addresses[i].ID]; ok {
			record.Card.Addresses[i].Coordinates = coord
			continue
		}
		if len(byID) == 0 && i < len(ordered) {
			record.Card.Addresses[i].Coordinates = ordered[i]
		}
	}
}

// normalizeGeoCoordinate canonicalizes a coordinate read off the wire into a
// `geo:LAT,LON` URI, undoing go-vcard's TEXT-style comma escape (`\,`) that it
// applies even to a uri-valued property. Returns "" for anything that is not a
// valid geo: URI, so a hostile or non-coordinate GEO property is ignored rather
// than stored.
func normalizeGeoCoordinate(v string) string {
	v = strings.TrimSpace(v)
	v = strings.ReplaceAll(v, `\,`, ",")
	if v == "" || len(v) < len("geo:") || !strings.EqualFold(v[:len("geo:")], "geo:") {
		return ""
	}
	if _, _, ok := contactmodel.ParseGeoURI(v); !ok {
		return ""
	}
	return v
}
