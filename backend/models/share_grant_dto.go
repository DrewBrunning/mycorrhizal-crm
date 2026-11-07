package models

import (
	"bytes"
	"encoding/json"
	"time"
)

// ShareGrantInput is the POST /share-grants body (ADR 0042 decision 13). It is
// the only thing a client names on create; status, from_user_id, timestamps,
// revision, reconfirm_sections and revoked_* are set by the service. There is
// deliberately no include_sensitive (decision 6).
type ShareGrantInput struct {
	ToUserID   uint       `json:"to_user_id" validate:"required"`
	EntityType string     `json:"entity_type" validate:"required,oneof=contact note activity circle"`
	EntityID   string     `json:"entity_id" validate:"required,max=128"`
	Sections   []string   `json:"sections" validate:"max=64,dive,max=64"`
	ExpiresAt  *time.Time `json:"expires_at"`
}

// NullableTime distinguishes an absent JSON field from an explicit null so a
// PATCH can both set and clear expires_at.
type NullableTime struct {
	Set   bool
	Value *time.Time
}

// UnmarshalJSON records presence; null clears.
func (n *NullableTime) UnmarshalJSON(b []byte) error {
	n.Set = true
	if bytes.Equal(bytes.TrimSpace(b), []byte("null")) {
		n.Value = nil
		return nil
	}
	var t time.Time
	if err := json.Unmarshal(b, &t); err != nil {
		return err
	}
	n.Value = &t
	return nil
}

// MarshalJSON renders the value (null when unset).
func (n NullableTime) MarshalJSON() ([]byte, error) {
	if n.Value == nil {
		return []byte("null"), nil
	}
	return json.Marshal(n.Value)
}

// ShareGrantPatchInput is the PATCH /share-grants/{id} body: only expires_at
// and a narrowing sections list. Sections is a pointer so "absent" and
// "present but empty" differ (empty is rejected).
type ShareGrantPatchInput struct {
	ExpiresAt NullableTime `json:"expires_at"`
	Sections  *[]string    `json:"sections" validate:"omitempty,max=64,dive,max=64"`
}

// ShareGrantDeclineInput is the optional POST /share-grants/{id}/decline body.
type ShareGrantDeclineInput struct {
	Block bool `json:"block"`
}
