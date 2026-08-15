package models

import (
	"reflect"

	"mycorrhizal/contactmodel"
)

// mergeRecordFromFlat reconciles the neutral Record derived fresh from a
// Contact's current flat legacy fields (fresh) with the authoritative
// full-fidelity Record already persisted on that Contact (loaded), and
// returns the Record BeforeSave should write back.
//
// T75:
// BeforeSave used to overwrite the loaded Card/CRM/Passthrough wholesale with
// the fresh flat-field derivation, which silently destroyed every member that
// has no flat-field home — pronouns (SpeakToAs), hobbies/expertise
// (PersonalInfo), address components outside the flat projection (the nine
// kinds with no editor demand: room, building, block, district,
// subdistrict, direction, landmark, number, separator — the PO box /
// apartment / floor kinds gained a flat home in T79), rich per-entry metadata
// (pref, features, extra contexts, entry ids), CRMEnvelope.Kind (T27's
// pet/animal marker), and the whole Passthrough column. The merge rule,
// chosen and documented 2026-08-12 (T75 item 2):
//
//   - The flat fields are the source of truth for everything they can
//     express; the loaded Card is the source of truth for everything they
//     cannot.
//
//   - Array-backed flat collections (Emails, Phones, IMPPs, URLs, Addresses)
//     are merged per-entry (see mergeProjectedArray): an entry whose flat
//     projection still matches the loaded card's entry was never touched by
//     the caller, so the loaded entry survives whole — unprojected data
//     included. An entry whose projection differs (or that sits beyond the
//     loaded array's length) was deliberately expressed through the lossy
//     flat shape, so the fresh derivation wins; losing that entry's
//     unprojected data there is correct, because the caller said so.
//
//     This is a strict superset of the ticket's original whole-array
//     dirty-comparison rule (compare the whole flat array; rebuild
//     everything from flat when any entry differs). The per-entry rule
//     preserves at least as much in every case and additionally preserves
//     untouched entries' unprojected components when the caller appends a new
//     one — which matters because MergeImportedContact (T49) appends to the
//     flat arrays, so the whole-array rule would have destroyed the existing
//     entries' apartment/PO-box data the first time an import added a
//     different address. The ticket rejected "positional pairing" on the
//     (incorrect) grounds that no plain-save path writes the flat arrays;
//     the actual writers do, so per-index dirty comparison is required to
//     keep their non-flat data intact.
//
//   - Scalar-backed collections (Name, Nicknames, Organizations, Titles,
//     Anniversaries) use the same idea against their flat scalar(s): keep
//     the loaded collection whole when the flat scalars still project from
//     it, take the fresh derivation otherwise.
//
//   - Card members with no flat representation at all (SpeakToAs,
//     PersonalInfo, Notes, Keywords, SocialProfiles, OtherOnlineServices,
//     Members, RelatedTo, Localizations, ContactURIs, Calendars,
//     FreeBusyURLs, SchedulingAddresses, CryptoKeys, Directories,
//     PreferredLanguages, Card.Kind, ...) are preserved unconditionally:
//     nothing can edit them via the flat shape, so there is nothing to
//     compare and no ambiguity.
//
//   - Media is split by kind: photo is flat-owned (Contact.Photo /
//     PhotoThumbnail) and always comes from fresh; logo/sound have no flat
//     home and are preserved from loaded.
//
//   - CRMEnvelope.Kind is preserved unconditionally (no flat field, T27);
//     the flat-owned envelope fields (Circles, HowWeMet, WorkInformation,
//     ContactInformation) always come from fresh.
//
//   - Passthrough is preserved when fresh has none (VCardExtra empty — the
//     common ApplyRecordToContact-created case, whose imported unknown
//     vCard/JSContact properties live only on the loaded column), and taken
//     from fresh otherwise (a caller that set VCardExtra deliberately).
func mergeRecordFromFlat(loaded, fresh contactmodel.Record) contactmodel.Record {
	merged := contactmodel.Record{
		UID:  fresh.UID,
		ETag: fresh.ETag,
		Card: mergeCardWithFlat(loaded.Card, fresh.Card),
		Envelope: contactmodel.CRMEnvelope{
			// Kind has no flat-field home — it lives only in the crm JSON
			// column, set via ApplyRecordToContact (T27). Preserve it.
			Kind:               loaded.Envelope.Kind,
			Circles:            fresh.Envelope.Circles,
			HowWeMet:           fresh.Envelope.HowWeMet,
			WorkInformation:    fresh.Envelope.WorkInformation,
			ContactInformation: fresh.Envelope.ContactInformation,
		},
	}
	if reflect.DeepEqual(fresh.Passthrough, contactmodel.Passthrough{}) {
		// No flat source (VCardExtra empty) — the loaded Passthrough holds
		// imported unknown-property data with no flat home; preserve it.
		merged.Passthrough = loaded.Passthrough
	} else {
		merged.Passthrough = fresh.Passthrough
	}
	return merged
}

// mergeCardWithFlat applies the T75 rules to the Card half of the record:
// flat-owned sub-structures come from fresh (subject to the per-entry /
// scalar dirty comparison below), everything else survives from loaded.
func mergeCardWithFlat(loaded, fresh contactmodel.Card) contactmodel.Card {
	merged := loaded
	// Card.UID mirrors the flat VCardUID column (docs/adrs/0002-correspondence-table-locked-oracle.md "uid"
	// row, populated by RecordFromContact), so it is flat-owned — always from
	// fresh, never carried over from a stale loaded value.
	merged.UID = fresh.UID
	merged.Name = mergeName(loaded.Name, fresh.Name)
	merged.Nicknames = mergeNicknames(loaded.Nicknames, fresh.Nicknames)
	merged.Organizations = mergeOrganizations(loaded.Organizations, fresh.Organizations)
	merged.Titles = mergeTitles(loaded.Titles, fresh.Titles)
	merged.Emails = mergeProjectedArray(loaded.Emails, fresh.Emails, projectEmail)
	merged.Phones = mergeProjectedArray(loaded.Phones, fresh.Phones, projectPhone)
	merged.ImppAddresses = mergeProjectedArray(loaded.ImppAddresses, fresh.ImppAddresses, projectImpp)
	merged.Addresses = mergeProjectedArray(loaded.Addresses, fresh.Addresses, projectAddress)
	merged.Anniversaries = mergeAnniversaries(loaded.Anniversaries, fresh.Anniversaries)
	merged.Links = mergeProjectedArray(loaded.Links, fresh.Links, projectLink)
	merged.Media = mergeMedia(loaded.Media, fresh.Media)
	// Every other Card member (SpeakToAs, PersonalInfo, Notes, Keywords,
	// SocialProfiles, OtherOnlineServices, Members, RelatedTo,
	// Localizations, ContactURIs, Calendars, FreeBusyURLs,
	// SchedulingAddresses, CryptoKeys, Directories, PreferredLanguages,
	// Kind/Language/ProdID/Created/Updated) has no flat home and is
	// carried over from `loaded` untouched by virtue of the copy above.
	return merged
}

// mergeProjectedArray merges a freshly-derived card collection onto the
// loaded one. proj maps a card entry to its flat projection (the flat struct
// the same data round-trips through). For each position, if the loaded
// entry's projection still matches the fresh entry's, the caller never
// touched that entry's flat form and the loaded entry survives whole; else
// (or past the loaded array's length) the fresh entry wins. Trailing loaded
// entries past the fresh array's length are dropped — the caller deleted
// them.
func mergeProjectedArray[T, F any](loaded, fresh []T, proj func(T) F) []T {
	result := make([]T, len(fresh))
	for i := range fresh {
		if i < len(loaded) && reflect.DeepEqual(proj(loaded[i]), proj(fresh[i])) {
			result[i] = loaded[i]
		} else {
			result[i] = fresh[i]
		}
	}
	return result
}

// mergeName keeps the loaded Name whole when the flat name scalars still
// project from it (its components produce the same five scalar fields), and
// takes the fresh derivation otherwise — so sortAs/isOrdered/full/phonetic
// survive a plain save but a real name edit replaces the whole Name.
func mergeName(loaded, fresh *contactmodel.Name) *contactmodel.Name {
	if namesProjectionEqual(loaded, fresh) {
		return loaded
	}
	return fresh
}

func namesProjectionEqual(loaded, fresh *contactmodel.Name) bool {
	proj := func(n *contactmodel.Name) map[string]string {
		out := map[string]string{"title": "", "given": "", "given2": "", "surname": "", "credential": ""}
		if n == nil {
			return out
		}
		for _, comp := range n.Components {
			switch comp.Kind {
			case "title":
				out["title"] = comp.Value
			case "given":
				out["given"] = comp.Value
			case "given2":
				out["given2"] = comp.Value
			case "surname":
				out["surname"] = comp.Value
			case "credential":
				out["credential"] = comp.Value
			}
		}
		return out
	}
	return reflect.DeepEqual(proj(loaded), proj(fresh))
}

// mergeNicknames keeps the loaded set whole when the flat Nickname scalar
// still matches its first entry's name (extra nicknames have no flat home),
// and takes the fresh derivation otherwise (a changed scalar, or an empty
// scalar meaning "clear them").
func mergeNicknames(loaded, fresh []contactmodel.Nickname) []contactmodel.Nickname {
	if len(loaded) > 0 && len(fresh) > 0 && loaded[0].Name == fresh[0].Name {
		return loaded
	}
	if len(loaded) == 0 && len(fresh) == 0 {
		return loaded
	}
	return fresh
}

// mergeOrganizations keeps the loaded set whole when the flat
// Organization/Department scalars still project from it (Organization[0] and
// its first unit), and takes the fresh derivation otherwise — so
// sortAs/extra-units survive a plain save but a real org edit replaces it.
func mergeOrganizations(loaded, fresh []contactmodel.Organization) []contactmodel.Organization {
	if orgProjectionEqual(loaded, fresh) {
		return loaded
	}
	return fresh
}

func orgProjectionEqual(loaded, fresh []contactmodel.Organization) bool {
	proj := func(orgs []contactmodel.Organization) [2]string {
		if len(orgs) == 0 {
			return [2]string{"", ""}
		}
		unit := ""
		if len(orgs[0].Units) > 0 {
			unit = orgs[0].Units[0].Name
		}
		return [2]string{orgs[0].Name, unit}
	}
	return proj(loaded) == proj(fresh)
}

// mergeTitles keeps the loaded set whole when the flat JobTitle/Role scalars
// still project from it (last title/role of each kind, matching applyTitles),
// and takes the fresh derivation otherwise.
func mergeTitles(loaded, fresh []contactmodel.Title) []contactmodel.Title {
	if titlesProjectionEqual(loaded, fresh) {
		return loaded
	}
	return fresh
}

func titlesProjectionEqual(loaded, fresh []contactmodel.Title) bool {
	proj := func(titles []contactmodel.Title) [2]string {
		var title, role string
		for _, t := range titles {
			switch t.Kind {
			case "title":
				title = t.Name
			case "role":
				role = t.Name
			}
		}
		return [2]string{title, role}
	}
	return proj(loaded) == proj(fresh)
}

// mergeAnniversaries keeps the loaded set whole when the flat Birthday/
// Anniversary scalars still project from it (the birth/wedding dates
// formatted exactly as DeriveProjection formats them), and takes the fresh
// derivation otherwise — so anniversary ids and places survive a plain save.
func mergeAnniversaries(loaded, fresh []contactmodel.Anniversary) []contactmodel.Anniversary {
	if anniversariesProjectionEqual(loaded, fresh) {
		return loaded
	}
	return fresh
}

func anniversariesProjectionEqual(loaded, fresh []contactmodel.Anniversary) bool {
	proj := func(anns []contactmodel.Anniversary) [2]string {
		var birth, wedding string
		for _, a := range anns {
			formatted := formatAnniversaryForMerge(a.Date)
			switch a.Kind {
			case "birth":
				birth = formatted
			case "wedding":
				wedding = formatted
			}
		}
		return [2]string{birth, wedding}
	}
	return proj(loaded) == proj(fresh)
}

// formatAnniversaryForMerge renders an AnniversaryDate the way the flat
// Birthday/Anniversary scalars are rendered, without duplicating
// contactmodel's formatting logic: DeriveProjection is the single existing
// implementation (the same trick applyAnniversaries already uses for the
// wedding date), so reuse it via a one-entry temporary Record.
func formatAnniversaryForMerge(d contactmodel.AnniversaryDate) string {
	proj := contactmodel.DeriveProjection(&contactmodel.Record{Card: contactmodel.Card{
		Anniversaries: []contactmodel.Anniversary{{Kind: "birth", Date: d}},
	}})
	return proj.Birthday
}

// mergeMedia splits the media by kind: photo entries are flat-owned (they
// bridge Contact.Photo/PhotoThumbnail) and always come from fresh; logo and
// sound entries have no flat home and are preserved from loaded
// unconditionally.
func mergeMedia(loaded, fresh []contactmodel.Resource) []contactmodel.Resource {
	result := make([]contactmodel.Resource, 0, len(fresh)+len(loaded))
	for _, r := range fresh {
		if r.Kind == "photo" {
			result = append(result, r)
		}
	}
	for _, r := range loaded {
		if r.Kind != "photo" {
			result = append(result, r)
		}
	}
	return result
}

// The projection functions below are the pure flat projections of a neutral
// card entry — the inverse of the corresponding buildX in contact_record.go,
// and the mirror of the applyX functions in contact_record_reverse.go. Each
// returns exactly the flat struct the flat side round-trips through, so two
// card entries "project equal" iff their flat form is indistinguishable.
func projectEmail(e contactmodel.Email) ContactEmail {
	return ContactEmail{Type: e.Label, Value: e.Address}
}

func projectPhone(p contactmodel.Phone) ContactPhone {
	return ContactPhone{Type: p.Label, Value: p.Number}
}

func projectImpp(s contactmodel.OnlineService) ContactIMPP {
	return ContactIMPP{Type: s.Service, Value: s.URI}
}

func projectLink(r contactmodel.Resource) ContactURL {
	return ContactURL{Type: r.Label, Value: r.URI}
}

func projectAddress(a contactmodel.Address) ContactAddress {
	return contactAddressFromNeutral(a)
}

// RestoreFlatStateFrom overwrites dst's flat, snapshot-carried fields with
// src's — the exact set an audit before-snapshot records (T18): everything
// json-tagged on Contact except the association collections, which are
// relationships, not flat state. json:"-" fields (PhotoThumbnail, VCardUID,
// VCardExtra, ETag, FN, Org, AddressesFlat) never appear in a snapshot, so
// they are deliberately left untouched — copying a zero value over them
// would clear real data undo never saw.
//
// This is the T75 stopgap for the audit Undo button (undoContact in
// controllers/audit_controller.go): restoring flat state is all a snapshot
// can express, and it lets BeforeSave's merge carry the Card-only members
// (SpeakToAs, PersonalInfo, unprojected address components,
// CRMEnvelope.Kind) that no snapshot has ever captured through the save.
func (dst *Contact) RestoreFlatStateFrom(src *Contact) {
	if dst == nil || src == nil {
		return
	}
	dst.Firstname = src.Firstname
	dst.Lastname = src.Lastname
	dst.Nickname = src.Nickname
	dst.Gender = src.Gender
	dst.Email = src.Email
	dst.Phone = src.Phone
	dst.Birthday = src.Birthday
	dst.Photo = src.Photo
	dst.Address = src.Address
	dst.HowWeMet = src.HowWeMet
	dst.WorkInformation = src.WorkInformation
	dst.ContactInformation = src.ContactInformation
	dst.Circles = src.Circles
	dst.Emails = src.Emails
	dst.Phones = src.Phones
	dst.Addresses = src.Addresses
	dst.URLs = src.URLs
	dst.IMPPs = src.IMPPs
	dst.Prefix = src.Prefix
	dst.MiddleName = src.MiddleName
	dst.Suffix = src.Suffix
	dst.Organization = src.Organization
	dst.Department = src.Department
	dst.JobTitle = src.JobTitle
	dst.Role = src.Role
	dst.Anniversary = src.Anniversary
	dst.Archived = src.Archived
}
