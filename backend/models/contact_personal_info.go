package models

import (
	"strings"

	"github.com/google/uuid"

	"mycorrhizal/contactmodel"
)

// PersonalInfo IDs + CRM-side metadata (issue #1738).
//
// Card.PersonalInfo is the single canonical home for hobbies: it is what
// CardDAV round-trips. Each entry gets a stable ID on write so
// CRMEnvelope.PersonalInfoMeta (sensitivity/favorite/notes/provenance) can be
// keyed to it without a second hobby store. The metadata has no RFC home and
// is never projected into a Card.

// ensurePersonalInfoIDs gives every Card.PersonalInfo entry a stable ID and
// drops metadata keys whose entry no longer exists. It runs at the top of
// deriveDenormalized, so every save path (REST, import, sync, backfill,
// audit undo) converges on "every stored entry has an ID" and no dangling
// metadata.
func (c *Contact) ensurePersonalInfoIDs() {
	if len(c.Card.PersonalInfo) > 0 {
		var out []contactmodel.PersonalInfo
		for i, entry := range c.Card.PersonalInfo {
			if entry.ID != "" {
				continue
			}
			if out == nil {
				out = append([]contactmodel.PersonalInfo(nil), c.Card.PersonalInfo...)
			}
			out[i].ID = uuid.NewString()
		}
		if out != nil {
			c.Card.PersonalInfo = out
		}
	}

	if len(c.CRM.PersonalInfoMeta) == 0 {
		return
	}
	live := make(map[string]bool, len(c.Card.PersonalInfo))
	for _, entry := range c.Card.PersonalInfo {
		if entry.ID != "" {
			live[entry.ID] = true
		}
	}
	orphaned := false
	for id := range c.CRM.PersonalInfoMeta {
		if !live[id] {
			orphaned = true
			break
		}
	}
	if !orphaned {
		return
	}
	var kept map[string]contactmodel.PersonalInfoMeta
	if len(live) > 0 {
		kept = make(map[string]contactmodel.PersonalInfoMeta, len(live))
		for id, meta := range c.CRM.PersonalInfoMeta {
			if live[id] {
				kept[id] = meta
			}
		}
		if len(kept) == 0 {
			kept = nil
		}
	}
	// Assign the whole envelope rather than mutating c.CRM.PersonalInfoMeta in
	// place: the semgrep trap rule (CLAUDE.md trap #2) guards direct nested CRM
	// mutation because BeforeSave can otherwise overwrite it.
	envelope := c.CRM
	envelope.PersonalInfoMeta = kept
	c.CRM = envelope
}

// filterSensitivePersonalInfo drops Card.PersonalInfo entries whose CRM
// metadata marks them above normal sensitivity, and prunes the returned
// metadata to the entries that remain. It is the personalInfo half of the
// outward-copy default-deny (vCard 3/4, JSContact, CardDAV, contact shares);
// the caller decides whether this is an owner read (no filter) or an outward
// projection. Neither input is mutated; the inputs are returned unchanged
// when there is nothing to drop.
func filterSensitivePersonalInfo(entries []contactmodel.PersonalInfo, meta map[string]contactmodel.PersonalInfoMeta) ([]contactmodel.PersonalInfo, map[string]contactmodel.PersonalInfoMeta) {
	if len(entries) == 0 || len(meta) == 0 {
		return entries, meta
	}
	drop := 0
	for _, entry := range entries {
		if isSensitivePersonalInfoMeta(meta[entry.ID]) {
			drop++
		}
	}
	if drop == 0 {
		return entries, meta
	}
	out := make([]contactmodel.PersonalInfo, 0, len(entries)-drop)
	kept := make(map[string]contactmodel.PersonalInfoMeta, len(meta))
	for _, entry := range entries {
		if isSensitivePersonalInfoMeta(meta[entry.ID]) {
			continue
		}
		out = append(out, entry)
		if m, ok := meta[entry.ID]; ok {
			kept[entry.ID] = m
		}
	}
	if len(kept) == 0 {
		kept = nil
	}
	return out, kept
}

// MergePersonalInfoFromRemote combines the server's persisted personalInfo
// entries (and their CRM metadata) with an incoming full-overwrite card from a
// remote CardDAV source.
//
// Remote content is authoritative for entries it can see, but the outward
// projection withholds above-normal entries, so the remote never received
// them: their absence is not a delete and they are re-attached with their
// metadata. Metadata for an entry the remote still has is carried forward
// (matching by stable ID, then case-insensitive kind+value). Metadata for an
// entry that no longer exists on either side is dropped. An incoming entry
// that matches a local one inherits the local ID when it has none, so the
// metadata key survives a server that does not send ID/PROP-ID.
//
// Neither input slice or map is mutated. A nil/empty metadata map means there
// is nothing to preserve, so incoming is returned unchanged.
func MergePersonalInfoFromRemote(
	existing []contactmodel.PersonalInfo,
	existingMeta map[string]contactmodel.PersonalInfoMeta,
	incoming []contactmodel.PersonalInfo,
) ([]contactmodel.PersonalInfo, map[string]contactmodel.PersonalInfoMeta) {
	if len(existingMeta) == 0 {
		return incoming, existingMeta
	}

	out := append([]contactmodel.PersonalInfo(nil), incoming...)
	kept := make(map[string]contactmodel.PersonalInfoMeta, len(existingMeta))
	for _, local := range existing {
		meta, ok := existingMeta[local.ID]
		if !ok || local.ID == "" {
			continue
		}
		idx := matchPersonalInfoIndex(out, local)
		if idx < 0 {
			// The remote never saw this entry if it is above normal; its
			// absence is not a delete. A normal entry's absence is a real
			// delete, so its metadata goes with it.
			if isSensitivePersonalInfoMeta(meta) {
				out = append(out, local)
				kept[local.ID] = meta
			}
			continue
		}
		out[idx].ID = local.ID
		kept[local.ID] = meta
	}
	if len(kept) == 0 {
		kept = nil
	}
	return out, kept
}

// matchPersonalInfoIndex finds the incoming entry corresponding to local: by
// stable ID when both carry one, else by case-insensitive kind+value (vCard 3
// has no entry ID, and plain CardDAV clients do not echo one).
func matchPersonalInfoIndex(entries []contactmodel.PersonalInfo, local contactmodel.PersonalInfo) int {
	if local.ID != "" {
		for i := range entries {
			if entries[i].ID == local.ID {
				return i
			}
		}
	}
	value := strings.ToLower(strings.TrimSpace(local.Value))
	if value == "" {
		return -1
	}
	for i := range entries {
		entry := entries[i]
		if strings.ToLower(strings.TrimSpace(entry.Value)) != value {
			continue
		}
		if local.Kind == "" || entry.Kind == "" || entry.Kind == local.Kind {
			return i
		}
	}
	return -1
}

// isSensitivePersonalInfoMeta reports whether metadata classifies an entry
// above the always-shareable normal tier (an empty sensitivity is normal).
func isSensitivePersonalInfoMeta(meta contactmodel.PersonalInfoMeta) bool {
	switch meta.NormalizedSensitivity() {
	case RelationshipSensitivityPrivate, RelationshipSensitivitySecret:
		return true
	default:
		return false
	}
}
