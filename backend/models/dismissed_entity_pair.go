package models

import "time"

// DismissedEntityPair records that the user hid an entity pair from the merge
// suggestions (ADR 0043 decision 8) -- the DismissedDuplicatePair pattern.
// uint PK, hard delete (trap #7): a join-shaped row whose identity is its
// natural-key composite unique index. IDLow < IDHigh (enforced by a CHECK), so
// a pair has one canonical spelling. Swept by EntityCascadeRegistry.
type DismissedEntityPair struct {
	ID        uint      `gorm:"primarykey" json:"id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	UserID uint   `gorm:"not null;index;uniqueIndex:idx_dismissed_entity_pairs_unique,priority:1" json:"-"`
	IDLow  string `gorm:"column:id_low;not null;uniqueIndex:idx_dismissed_entity_pairs_unique,priority:2" json:"id_low"`
	IDHigh string `gorm:"column:id_high;not null;uniqueIndex:idx_dismissed_entity_pairs_unique,priority:3" json:"id_high"`
}
