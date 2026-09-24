package contactmodel

// AnniversaryKindDeath is the vCard DEATHDATE / JSContact "death" anniversary
// kind (RFC 6474).

// Issue #1193: Card.Anniversaries[kind=death] IS the deceased state — the
// single source of truth. No parallel boolean, no mirrored LifeEvent (see
// backend/services/wedding_sync.go's doc comment for why that mirroring
// pattern deliberately does not extend to death).
const AnniversaryKindDeath = "death"

// DeathAnniversary returns the Card's death anniversary, or nil if none is
// recorded. When more than one death anniversary entry exists (which import
// from a malformed source could produce), the first is authoritative,
// matching DeriveProjection's "first match wins" convention for Anniversaries.
func (c Card) DeathAnniversary() *Anniversary {
	for i := range c.Anniversaries {
		if c.Anniversaries[i].Kind == AnniversaryKindDeath {
			return &c.Anniversaries[i]
		}
	}
	return nil
}

// IsDeceased reports whether the Card records a death anniversary.
func (c Card) IsDeceased() bool {
	return c.DeathAnniversary() != nil
}
