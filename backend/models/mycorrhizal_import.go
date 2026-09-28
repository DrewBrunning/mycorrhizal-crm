package models

// DTOs for the `mycorrhizal` import source (issue #1260, ADR 0028 Decision 3)
// — an uploaded account bundle produced by GET /export/account. The acquisition
// step is an upload + parse; from the review step on it uses the shared
// source-import DTOs in source_import.go.

// MycorrhizalBundleCounts is the per-section tally of an uploaded bundle,
// shown after upload so the user sees what it carries before importing.
type MycorrhizalBundleCounts struct {
	Contacts               int `json:"contacts"`
	Relationships          int `json:"relationships"`
	Notes                  int `json:"notes"`
	Reminders              int `json:"reminders"`
	ReminderCompletions    int `json:"reminder_completions"`
	Activities             int `json:"activities"`
	LifeEvents             int `json:"life_events"`
	Gifts                  int `json:"gifts"`
	Preferences            int `json:"preferences"`
	ConversationAgenda     int `json:"conversation_agenda"`
	CadencePolicies        int `json:"cadence_policies"`
	DataDecayPolicies      int `json:"data_decay_policies"`
	Households             int `json:"households"`
	Circles                int `json:"circles"`
	Tags                   int `json:"tags"`
	CustomFieldDefinitions int `json:"custom_field_definitions"`
	CustomFieldValues      int `json:"custom_field_values"`
	Occasions              int `json:"occasions"`
	OccasionEvents         int `json:"occasion_events"`
}

// MycorrhizalUploadResponse is returned once an uploaded file validates as a
// v1 account bundle.
type MycorrhizalUploadResponse struct {
	SessionID string                  `json:"session_id"`
	Version   int                     `json:"version"`
	Totals    MycorrhizalBundleCounts `json:"totals"`
}

// MycorrhizalFetchRequest starts the background map + preview build for an
// uploaded bundle session.
type MycorrhizalFetchRequest struct {
	SessionID string `json:"session_id" validate:"required"`
}
