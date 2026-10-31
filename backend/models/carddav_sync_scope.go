package models

import "time"

// CardDAV sync-scope modes (ADR 0041 decision 1). A user with no
// CardDAVSyncScope row is CardDAVSyncScopeModeAll, exactly today's behaviour.
const (
	CardDAVSyncScopeModeAll     = "all"
	CardDAVSyncScopeModeInclude = "include"
	CardDAVSyncScopeModeExclude = "exclude"
)

// CardDAV sync-scope warning codes, derived on read (ADR 0041 decision 3).
const (
	CardDAVSyncScopeWarningEmpty          = "scope_empty"
	CardDAVSyncScopeWarningDefaultMissing = "default_circle_missing"
	CardDAVSyncScopeWarningExclusionEmpty = "exclusion_empty"
)

// CardDAVSyncScope is the per-user CardDAV sync scope singleton (ADR 0041).
// It is device configuration, not user-authored contact data: it holds only a
// mode and circle ids and is deliberately outside the account bundle.
type CardDAVSyncScope struct {
	UserID uint   `gorm:"column:user_id;primaryKey;autoIncrement:false" json:"-"`
	Mode   string `gorm:"not null;default:all" json:"mode"`

	// DefaultCircleID is where a phone-created contact is filed in include
	// mode (and optionally in exclude mode). NULL means no default circle.
	// The FK is ON DELETE SET NULL so deleting the circle never deletes data.
	DefaultCircleID *string `gorm:"column:default_circle_id" json:"default_circle_id"`

	CreatedAt time.Time `json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TableName pins the table name; GORM's acronym splitting would otherwise
// derive card_dav_sync_scopes from CardDAVSyncScope (backend trap #1).
func (CardDAVSyncScope) TableName() string {
	return "carddav_sync_scopes"
}

// CardDAVSyncScopeCircle is one selected circle for a user's sync scope. The
// mode on CardDAVSyncScope says whether the rows are included or excluded.
type CardDAVSyncScopeCircle struct {
	UserID   uint   `gorm:"column:user_id;primaryKey;autoIncrement:false" json:"-"`
	CircleID string `gorm:"column:circle_id;primaryKey" json:"circle_id"`
}

// TableName pins the table name (see CardDAVSyncScope.TableName).
func (CardDAVSyncScopeCircle) TableName() string {
	return "carddav_sync_scope_circles"
}

// CardDAVSyncScopeInput is the whole-object body for PUT
// /users/me/carddav-sync-scope and for the preview endpoint.
type CardDAVSyncScopeInput struct {
	Mode            string   `json:"mode" validate:"required,oneof=all include exclude"`
	CircleIDs       []string `json:"circle_ids" validate:"dive,uuid4"`
	DefaultCircleID *string  `json:"default_circle_id" validate:"omitempty,uuid4"`
}

// CardDAVSyncScopeWarning is one derived warning on the current scope.
type CardDAVSyncScopeWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// CardDAVSyncScopeResponse is the GET/PUT response. Counts are computed from
// the live contacts and the stored scope; warnings are derived, never stored.
type CardDAVSyncScopeResponse struct {
	Mode            string                    `json:"mode"`
	CircleIDs       []string                  `json:"circle_ids"`
	DefaultCircleID *string                   `json:"default_circle_id"`
	UpdatedAt       *time.Time                `json:"updated_at"`
	InScopeCount    int                       `json:"in_scope_count"`
	TotalCount      int                       `json:"total_count"`
	Warnings        []CardDAVSyncScopeWarning `json:"warnings"`
}

// CardDAVSyncScopePreviewResponse is the unpersisted preview result for a
// proposed scope, so the UI can warn before saving.
type CardDAVSyncScopePreviewResponse struct {
	Mode         string                    `json:"mode"`
	CircleIDs    []string                  `json:"circle_ids"`
	InScopeCount int                       `json:"in_scope_count"`
	TotalCount   int                       `json:"total_count"`
	Warnings     []CardDAVSyncScopeWarning `json:"warnings"`
}
