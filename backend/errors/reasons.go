package errors

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"mycorrhizal/logger"
)

// ReasonCode is a stable, machine-readable error reason (ADR 0038, issue
// #2095). Clients localize from it; `message` stays English.
type ReasonCode string

type reasonSpec struct {
	// Template is the English rendering; placeholders are {name}.
	Template string
	// Params is the declared param order. Android positional args (%N$s) are
	// numbered in this order.
	Params []string
}

// reasonAreas is the closed set of first segments a reason code may use. A
// new area is added here in the same PR as its first code.
var reasonAreas = []string{
	"common", "auth", "account", "admin", "import", "meerkat", "monica",
	"bundle", "csv", "vcard", "integration", "immich", "seafile", "paperless",
	"geopulse", "webdav", "calendar", "feed", "notification", "contact",
	"household", "relationship", "occasion", "photo", "attachment", "validation",
	"circle", "activity", "nextcloud", "trash",
}

// Reason constants. Each must be a key of reasonRegistry (and vice versa);
// TestEveryReasonConstantRegistered enforces both directions.
const (
	// ReasonCommonTestOnly exists for tests only and is removed in #2100.
	ReasonCommonTestOnly ReasonCode = "common_test_only"

	ReasonCircleColorInvalid             ReasonCode = "circle_color_invalid"
	ReasonImportCirclesNotFound          ReasonCode = "import_circles_not_found"
	ReasonImportTagsNotFound             ReasonCode = "import_tags_not_found"
	ReasonValidationCursorMalformed      ReasonCode = "validation_cursor_malformed"
	ReasonValidationActivityIDsRange     ReasonCode = "validation_activity_ids_range"
	ReasonRelationshipContactAmbiguous   ReasonCode = "relationship_contact_ambiguous"
	ReasonRelationshipContactMissing     ReasonCode = "relationship_contact_missing"
	ReasonRelationshipSelfEdge           ReasonCode = "relationship_self_edge"
	ReasonRelationshipPartialDateMissing ReasonCode = "relationship_partial_date_missing"
	ReasonRelationshipMonthOutOfRange    ReasonCode = "relationship_month_out_of_range"
	ReasonRelationshipDayOutOfRange      ReasonCode = "relationship_day_out_of_range"
	ReasonRelationshipEndBeforeStart     ReasonCode = "relationship_end_before_start"
	ReasonRelationshipNotSuggestion      ReasonCode = "relationship_not_suggestion"
	ReasonTrashUIDInUse                  ReasonCode = "trash_uid_in_use"
	ReasonTrashSnapshotUnsupported       ReasonCode = "trash_snapshot_unsupported"
	ReasonContactBulkNoOperations        ReasonCode = "contact_bulk_no_operations"
	ReasonContactBulkFieldNotEditable    ReasonCode = "contact_bulk_field_not_editable"
	ReasonContactBulkValueEmpty          ReasonCode = "contact_bulk_value_empty"
	ReasonContactBulkValueTooLong        ReasonCode = "contact_bulk_value_too_long"
	ReasonContactBulkFieldInSetAndClear  ReasonCode = "contact_bulk_field_in_set_and_clear"
	ReasonContactBulkCustomFieldNotFound ReasonCode = "contact_bulk_custom_field_not_found"
	ReasonContactBulkCustomFieldDupe     ReasonCode = "contact_bulk_custom_field_duplicate"
	ReasonContactBulkCustomFieldInBoth   ReasonCode = "contact_bulk_custom_field_in_both"
	ReasonContactBulkCustomFieldInvalid  ReasonCode = "contact_bulk_custom_field_invalid"

	// Meerkat live import (issue #2100, services/meerkat_import_*.go).
	ReasonMeerkatUploadMissing         ReasonCode = "meerkat_upload_missing"
	ReasonMeerkatSessionNotFound       ReasonCode = "meerkat_session_not_found"
	ReasonMeerkatSessionNotOwned       ReasonCode = "meerkat_session_not_owned"
	ReasonMeerkatSessionExpired        ReasonCode = "meerkat_session_expired"
	ReasonMeerkatDBSize                ReasonCode = "meerkat_db_size"
	ReasonMeerkatUploadNotSQLite       ReasonCode = "meerkat_upload_not_sqlite"
	ReasonMeerkatUploadUnreadable      ReasonCode = "meerkat_upload_unreadable"
	ReasonMeerkatFileNotMeerkat        ReasonCode = "meerkat_file_not_meerkat"
	ReasonMeerkatDBUnreadable          ReasonCode = "meerkat_db_unreadable"
	ReasonMeerkatDBTooManyContacts     ReasonCode = "meerkat_db_too_many_contacts"
	ReasonMeerkatDBTooManyRecords      ReasonCode = "meerkat_db_too_many_records"
	ReasonMeerkatImportInProgress      ReasonCode = "meerkat_import_in_progress"
	ReasonMeerkatNotPrepared           ReasonCode = "meerkat_not_prepared"
	ReasonMeerkatAPIError              ReasonCode = "meerkat_api_error"
	ReasonMeerkatUpstreamError         ReasonCode = "meerkat_upstream_error"
	ReasonMeerkatSignInInputMissing    ReasonCode = "meerkat_sign_in_input_missing"
	ReasonMeerkatRemoteTooManyContacts ReasonCode = "meerkat_remote_too_many_contacts"
	ReasonMeerkatVersionUntested       ReasonCode = "meerkat_version_untested"
	// Constructor defaults (issue #2099).
	ReasonCommonDatabaseError                       ReasonCode = "common_database_error"
	ReasonCommonDatabaseBusy                        ReasonCode = "common_database_busy"
	ReasonAuthInvalidCredentials                    ReasonCode = "auth_invalid_credentials" // #nosec G101 -- reason code, not a credential
	ReasonAuthTokenExpired                          ReasonCode = "auth_token_expired"       // #nosec G101 -- reason code, not a credential
	ReasonAuthTokenInvalid                          ReasonCode = "auth_token_invalid"       // #nosec G101 -- reason code, not a credential
	ReasonCommonClientNotSupported                  ReasonCode = "common_client_not_supported"
	ReasonCommonRateLimited                         ReasonCode = "common_rate_limited"
	ReasonCommonPayloadTooLarge                     ReasonCode = "common_payload_too_large"
	ReasonCommonIdempotencyKeyReused                ReasonCode = "common_idempotency_key_reused"
	ReasonCommonRequestInProgress                   ReasonCode = "common_request_in_progress"
	ReasonCommonRequestResultUnavailable            ReasonCode = "common_request_result_unavailable"
	ReasonValidationMissingField                    ReasonCode = "validation_missing_field"
	ReasonCommonUnexpectedError                     ReasonCode = "common_unexpected_error"
	ReasonActivityNotFound                          ReasonCode = "activity_not_found"
	ReasonContactAddressNotFound                    ReasonCode = "contact_address_not_found"
	ReasonAccountApiTokenNotFound                   ReasonCode = "account_api_token_not_found"
	ReasonAttachmentNotFound                        ReasonCode = "attachment_not_found"
	ReasonAttachmentFileNotFound                    ReasonCode = "attachment_file_not_found"
	ReasonAdminAuditEventNotFound                   ReasonCode = "admin_audit_event_not_found"
	ReasonContactCadencePolicyNotFound              ReasonCode = "contact_cadence_policy_not_found"
	ReasonContactCadencePolicyAlreadyExists         ReasonCode = "contact_cadence_policy_already_exists"
	ReasonCalendarSubscriptionNotFound              ReasonCode = "calendar_subscription_not_found"
	ReasonCircleNotFound                            ReasonCode = "circle_not_found"
	ReasonCircleMembershipNotFound                  ReasonCode = "circle_membership_not_found"
	ReasonCircleMembershipAlreadyExists             ReasonCode = "circle_membership_already_exists"
	ReasonContactNotFound                           ReasonCode = "contact_not_found"
	ReasonContactShareNotFound                      ReasonCode = "contact_share_not_found"
	ReasonContactSubscriptionNotFound               ReasonCode = "contact_subscription_not_found"
	ReasonContactConversationAgendaItemNotFound     ReasonCode = "contact_conversation_agenda_item_not_found"
	ReasonContactDataDecayPolicyNotFound            ReasonCode = "contact_data_decay_policy_not_found"
	ReasonContactDataDecayPolicyAlreadyExists       ReasonCode = "contact_data_decay_policy_already_exists"
	ReasonAuthDeviceGrantNotFound                   ReasonCode = "auth_device_grant_not_found" // #nosec G101 -- reason code, not a credential
	ReasonNotificationDeviceRegistrationNotFound    ReasonCode = "notification_device_registration_not_found"
	ReasonOccasionEventAttendeeNotFound             ReasonCode = "occasion_event_attendee_not_found"
	ReasonOccasionEventAttendeeAlreadyExists        ReasonCode = "occasion_event_attendee_already_exists"
	ReasonContactExternalActivityNotFound           ReasonCode = "contact_external_activity_not_found"
	ReasonContactExternalActivityAlreadyExists      ReasonCode = "contact_external_activity_already_exists"
	ReasonIntegrationExternalIdentityNotFound       ReasonCode = "integration_external_identity_not_found"
	ReasonIntegrationExternalIdentityAlreadyExists  ReasonCode = "integration_external_identity_already_exists"
	ReasonFeedNotFound                              ReasonCode = "feed_not_found"
	ReasonContactFieldDefinitionNotFound            ReasonCode = "contact_field_definition_not_found"
	ReasonContactFieldDefinitionAlreadyExists       ReasonCode = "contact_field_definition_already_exists"
	ReasonGeopulseResourceNotFound                  ReasonCode = "geopulse_resource_not_found"
	ReasonContactGiftNotFound                       ReasonCode = "contact_gift_not_found"
	ReasonHouseholdNotFound                         ReasonCode = "household_not_found"
	ReasonHouseholdMembershipNotFound               ReasonCode = "household_membership_not_found"
	ReasonHouseholdMembershipAlreadyExists          ReasonCode = "household_membership_already_exists"
	ReasonHouseholdSuggestionDismissalAlreadyExists ReasonCode = "household_suggestion_dismissal_already_exists"
	ReasonPhotoImageNotFound                        ReasonCode = "photo_image_not_found"
	ReasonImmichLinkNotFound                        ReasonCode = "immich_link_not_found"
	ReasonImmichLinkAlreadyExists                   ReasonCode = "immich_link_already_exists"
	ReasonImmichPersonNotFound                      ReasonCode = "immich_person_not_found"
	ReasonContactLifeEventNotFound                  ReasonCode = "contact_life_event_not_found"
	ReasonContactLinkFieldTypeNotFound              ReasonCode = "contact_link_field_type_not_found"
	ReasonContactLinkFieldTypeAlreadyExists         ReasonCode = "contact_link_field_type_already_exists"
	ReasonCommonMemberNotFound                      ReasonCode = "common_member_not_found"
	ReasonNextcloudFileOrFolderNotFound             ReasonCode = "nextcloud_file_or_folder_not_found"
	ReasonNextcloudLinkNotFound                     ReasonCode = "nextcloud_link_not_found"
	ReasonNextcloudLinkAlreadyExists                ReasonCode = "nextcloud_link_already_exists"
	ReasonContactNoteNotFound                       ReasonCode = "contact_note_not_found"
	ReasonOccasionEventNotFound                     ReasonCode = "occasion_event_not_found"
	ReasonOccasionObligationNotFound                ReasonCode = "occasion_obligation_not_found"
	ReasonPaperlessDocumentNotFound                 ReasonCode = "paperless_document_not_found"
	ReasonPaperlessLinkNotFound                     ReasonCode = "paperless_link_not_found"
	ReasonPaperlessLinkAlreadyExists                ReasonCode = "paperless_link_already_exists"
	ReasonAuthPasskeyNotFound                       ReasonCode = "auth_passkey_not_found"      // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyAlreadyExists                  ReasonCode = "auth_passkey_already_exists" // #nosec G101 -- reason code, not a credential
	ReasonPhotoNotFound                             ReasonCode = "photo_not_found"
	ReasonAccountPreferenceNotFound                 ReasonCode = "account_preference_not_found"
	ReasonNotificationPushSubscriptionNotFound      ReasonCode = "notification_push_subscription_not_found"
	ReasonContactReachOutSuggestionNotFound         ReasonCode = "contact_reach_out_suggestion_not_found"
	ReasonAccountRecipientUserNotFound              ReasonCode = "account_recipient_user_not_found"
	ReasonRelationshipEdgeNotFound                  ReasonCode = "relationship_edge_not_found"
	ReasonRelationshipEdgeAlreadyExists             ReasonCode = "relationship_edge_already_exists"
	ReasonContactReminderNotFound                   ReasonCode = "contact_reminder_not_found"
	ReasonContactReminderCompletionNotFound         ReasonCode = "contact_reminder_completion_not_found"
	ReasonSeafileLibraryOrFileNotFound              ReasonCode = "seafile_library_or_file_not_found"
	ReasonSeafileLinkNotFound                       ReasonCode = "seafile_link_not_found"
	ReasonSeafileLinkAlreadyExists                  ReasonCode = "seafile_link_already_exists"
	ReasonAuthSessionNotFound                       ReasonCode = "auth_session_not_found" // #nosec G101 -- reason code, not a credential
	ReasonCommonSyncConflictNotFound                ReasonCode = "common_sync_conflict_not_found"
	ReasonContactTagNotFound                        ReasonCode = "contact_tag_not_found"
	ReasonContactTaggingNotFound                    ReasonCode = "contact_tagging_not_found"
	ReasonContactTaggingAlreadyExists               ReasonCode = "contact_tagging_already_exists"
	ReasonPhotoThumbnailNotFound                    ReasonCode = "photo_thumbnail_not_found"
	ReasonAccountUserNotFound                       ReasonCode = "account_user_not_found"
	ReasonAccountUserAlreadyExists                  ReasonCode = "account_user_already_exists"
	ReasonIntegrationWebhookNotFound                ReasonCode = "integration_webhook_not_found"
	ReasonContactSomeNotFound                       ReasonCode = "contact_some_not_found"

	// Sign-in errors (issue #2101: login, 2FA, passkeys, password reset, lockout).
	ReasonAuthAccountLocked                    ReasonCode = "auth_account_locked"                        // #nosec G101 -- reason code, not a credential
	ReasonAuthPasswordBreached                 ReasonCode = "auth_password_breached"                     // #nosec G101 -- reason code, not a credential
	ReasonAuthPasswordTooLong                  ReasonCode = "auth_password_too_long"                     // #nosec G101 -- reason code, not a credential
	ReasonAuthPasswordChangeDisabledDemo       ReasonCode = "auth_password_change_disabled_demo"         // #nosec G101 -- reason code, not a credential
	ReasonAuthResetTokenInvalid                ReasonCode = "auth_reset_token_invalid"                   // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorAlreadyEnabled          ReasonCode = "auth_two_factor_already_enabled"            // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorUnavailableOIDC         ReasonCode = "auth_two_factor_unavailable_oidc"           // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorSetupRequired           ReasonCode = "auth_two_factor_setup_required"             // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorCodeInvalid             ReasonCode = "auth_two_factor_code_invalid"               // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorNotEnabled              ReasonCode = "auth_two_factor_not_enabled"                // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorLoginNotPending         ReasonCode = "auth_two_factor_login_not_pending"          // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorSessionInvalid          ReasonCode = "auth_two_factor_session_invalid"            // #nosec G101 -- reason code, not a credential
	ReasonAuthTwoFactorNoLongerEnabled         ReasonCode = "auth_two_factor_no_longer_enabled"          // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyNameTooLong               ReasonCode = "auth_passkey_name_too_long"                 // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyNameLengthInvalid         ReasonCode = "auth_passkey_name_length_invalid"           // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyRegistrationNotStarted    ReasonCode = "auth_passkey_registration_not_started"      // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyRegistrationFailed        ReasonCode = "auth_passkey_registration_failed"           // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyNoneRegistered            ReasonCode = "auth_passkey_none_registered"               // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyNoneRegisteredSignInAgain ReasonCode = "auth_passkey_none_registered_sign_in_again" // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyLoginNotStarted           ReasonCode = "auth_passkey_login_not_started"             // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyVerificationFailed        ReasonCode = "auth_passkey_verification_failed"           // #nosec G101 -- reason code, not a credential
	ReasonAuthPasskeyNoOtherRegistered         ReasonCode = "auth_passkey_no_other_registered"           // #nosec G101 -- reason code, not a credential
	ReasonValidationInvalidJSON                ReasonCode = "validation_invalid_json"                    // #nosec G101 -- reason code, not a credential
	ReasonValidationInvalidRequestBody         ReasonCode = "validation_invalid_request_body"            // #nosec G101 -- reason code, not a credential
)

var reasonRegistry = map[ReasonCode]reasonSpec{
	ReasonCommonTestOnly: {Template: "Test {n}", Params: []string{"n"}},

	ReasonCircleColorInvalid:             {Template: "The colour must be one of the palette colours"},
	ReasonImportCirclesNotFound:          {Template: "One or more circles were not found"},
	ReasonImportTagsNotFound:             {Template: "One or more tags were not found"},
	ReasonValidationCursorMalformed:      {Template: "The cursor id is malformed"},
	ReasonValidationActivityIDsRange:     {Template: "Must contain between 1 and 100 ids"},
	ReasonRelationshipContactAmbiguous:   {Template: "Specify either an existing contact or a new contact, not both"},
	ReasonRelationshipContactMissing:     {Template: "Specify an existing contact or a new contact"},
	ReasonRelationshipSelfEdge:           {Template: "The source and target must be different contacts"},
	ReasonRelationshipPartialDateMissing: {Template: "A date must include at least a year, month or day"},
	ReasonRelationshipMonthOutOfRange:    {Template: "The month must be between 1 and 12"},
	ReasonRelationshipDayOutOfRange:      {Template: "The day must be between 1 and 31"},
	ReasonRelationshipEndBeforeStart:     {Template: "The end date cannot be before the start date"},
	ReasonRelationshipNotSuggestion:      {Template: "This relationship is not a pending suggestion"},
	ReasonTrashUIDInUse:                  {Template: "A live contact already uses this contact's UID; merge or delete it first"},
	ReasonTrashSnapshotUnsupported:       {Template: "This contact's deletion snapshot is not supported by this version; it can still be deleted permanently"},
	ReasonContactBulkNoOperations:        {Template: "Provide at least one of set, clear, custom_set or custom_clear"},
	ReasonContactBulkFieldNotEditable:    {Template: "{field} is not bulk-editable", Params: []string{"field"}},
	ReasonContactBulkValueEmpty:          {Template: "{field} must not be empty; list it in clear to empty it", Params: []string{"field"}},
	ReasonContactBulkValueTooLong:        {Template: "{field} must be at most {limit} characters", Params: []string{"field", "limit"}},
	ReasonContactBulkFieldInSetAndClear:  {Template: "{field} cannot be in both set and clear", Params: []string{"field"}},
	ReasonContactBulkCustomFieldNotFound: {Template: "A custom field definition was not found or is not yours"},
	ReasonContactBulkCustomFieldDupe:     {Template: "A custom field definition appears more than once"},
	ReasonContactBulkCustomFieldInBoth:   {Template: "A custom field cannot be in both custom_set and custom_clear"},
	ReasonContactBulkCustomFieldInvalid:  {Template: "{field} has an invalid value", Params: []string{"field"}},

	ReasonMeerkatUploadMissing:         {Template: "No file uploaded"},
	ReasonMeerkatSessionNotFound:       {Template: "Import session expired or not found"},
	ReasonMeerkatSessionNotOwned:       {Template: "Session does not belong to current user"},
	ReasonMeerkatSessionExpired:        {Template: "Import session expired"},
	ReasonMeerkatDBSize:                {Template: "The database file is empty or larger than the 100 MB limit"},
	ReasonMeerkatUploadNotSQLite:       {Template: "Upload a Meerkat SQLite database (.db, .sqlite or .sqlite3)"},
	ReasonMeerkatUploadUnreadable:      {Template: "The upload could not be read"},
	ReasonMeerkatFileNotMeerkat:        {Template: "That file is not a Meerkat database"},
	ReasonMeerkatDBUnreadable:          {Template: "The database could not be read as a Meerkat database"},
	ReasonMeerkatDBTooManyContacts:     {Template: "This database has more contacts than the import supports"},
	ReasonMeerkatDBTooManyRecords:      {Template: "This database has more records than the import supports"},
	ReasonMeerkatImportInProgress:      {Template: "This import is already being prepared"},
	ReasonMeerkatNotPrepared:           {Template: "The Meerkat database is not prepared yet"},
	ReasonMeerkatAPIError:              {Template: "{reason}", Params: []string{"reason"}},
	ReasonMeerkatUpstreamError:         {Template: "{reason}", Params: []string{"reason"}},
	ReasonMeerkatSignInInputMissing:    {Template: "Enter a Meerkat API token, or a username and password"},
	ReasonMeerkatRemoteTooManyContacts: {Template: "Contacts on this Meerkat account: {total}. The import supports at most {max}", Params: []string{"total", "max"}},
	ReasonMeerkatVersionUntested:       {Template: "This Meerkat version is not tested with the importer. Confirm to try anyway; nothing is written until you review the preview."},
	// Constructor defaults (issue #2099).
	ReasonCommonDatabaseError:                       {Template: "Something went wrong while saving or loading your data"},
	ReasonCommonDatabaseBusy:                        {Template: "The server is busy. Try again in a moment"},
	ReasonAuthInvalidCredentials:                    {Template: "Invalid email or password"},
	ReasonAuthTokenExpired:                          {Template: "Your session has expired. Sign in again"},
	ReasonAuthTokenInvalid:                          {Template: "Your session is no longer valid. Sign in again"},
	ReasonCommonClientNotSupported:                  {Template: "This app version is no longer supported. Update to {required_version} or later", Params: []string{"required_version"}},
	ReasonCommonRateLimited:                         {Template: "Too many requests. Try again later"},
	ReasonCommonPayloadTooLarge:                     {Template: "The upload is too large"},
	ReasonCommonIdempotencyKeyReused:                {Template: "Idempotency-Key was already used for a different request; use a fresh key"},
	ReasonCommonRequestInProgress:                   {Template: "a request with this Idempotency-Key is still being processed; retry shortly"},
	ReasonCommonRequestResultUnavailable:            {Template: "a previous request with this Idempotency-Key completed but its response is not available for replay; do not resubmit it with this key"},
	ReasonValidationMissingField:                    {Template: "Required field '{field}' is missing", Params: []string{"field"}},
	ReasonCommonUnexpectedError:                     {Template: "An unexpected error occurred"},
	ReasonActivityNotFound:                          {Template: "Activity not found"},
	ReasonContactAddressNotFound:                    {Template: "Address not found"},
	ReasonAccountApiTokenNotFound:                   {Template: "API token not found"},
	ReasonAttachmentNotFound:                        {Template: "Attachment not found"},
	ReasonAttachmentFileNotFound:                    {Template: "Attachment file not found"},
	ReasonAdminAuditEventNotFound:                   {Template: "Audit event not found"},
	ReasonContactCadencePolicyNotFound:              {Template: "Cadence policy not found"},
	ReasonContactCadencePolicyAlreadyExists:         {Template: "Cadence policy already exists"},
	ReasonCalendarSubscriptionNotFound:              {Template: "Calendar subscription not found"},
	ReasonCircleNotFound:                            {Template: "Circle not found"},
	ReasonCircleMembershipNotFound:                  {Template: "Circle membership not found"},
	ReasonCircleMembershipAlreadyExists:             {Template: "Circle membership already exists"},
	ReasonContactNotFound:                           {Template: "Contact not found"},
	ReasonContactShareNotFound:                      {Template: "Contact share not found"},
	ReasonContactSubscriptionNotFound:               {Template: "Contact subscription not found"},
	ReasonContactConversationAgendaItemNotFound:     {Template: "Conversation agenda item not found"},
	ReasonContactDataDecayPolicyNotFound:            {Template: "Data decay policy not found"},
	ReasonContactDataDecayPolicyAlreadyExists:       {Template: "Data decay policy already exists"},
	ReasonAuthDeviceGrantNotFound:                   {Template: "Device grant not found"},
	ReasonNotificationDeviceRegistrationNotFound:    {Template: "Device registration not found"},
	ReasonOccasionEventAttendeeNotFound:             {Template: "Event attendee not found"},
	ReasonOccasionEventAttendeeAlreadyExists:        {Template: "Event attendee already exists"},
	ReasonContactExternalActivityNotFound:           {Template: "External activity not found"},
	ReasonContactExternalActivityAlreadyExists:      {Template: "External activity already exists"},
	ReasonIntegrationExternalIdentityNotFound:       {Template: "External identity not found"},
	ReasonIntegrationExternalIdentityAlreadyExists:  {Template: "External identity already exists"},
	ReasonFeedNotFound:                              {Template: "Feed not found"},
	ReasonContactFieldDefinitionNotFound:            {Template: "Field definition not found"},
	ReasonContactFieldDefinitionAlreadyExists:       {Template: "Field definition already exists"},
	ReasonGeopulseResourceNotFound:                  {Template: "GeoPulse resource not found"},
	ReasonContactGiftNotFound:                       {Template: "Gift not found"},
	ReasonHouseholdNotFound:                         {Template: "Household not found"},
	ReasonHouseholdMembershipNotFound:               {Template: "Household membership not found"},
	ReasonHouseholdMembershipAlreadyExists:          {Template: "Household membership already exists"},
	ReasonHouseholdSuggestionDismissalAlreadyExists: {Template: "Household suggestion dismissal already exists"},
	ReasonPhotoImageNotFound:                        {Template: "Image not found"},
	ReasonImmichLinkNotFound:                        {Template: "Immich link not found"},
	ReasonImmichLinkAlreadyExists:                   {Template: "Immich link already exists"},
	ReasonImmichPersonNotFound:                      {Template: "Immich person not found"},
	ReasonContactLifeEventNotFound:                  {Template: "Life event not found"},
	ReasonContactLinkFieldTypeNotFound:              {Template: "Link field type not found"},
	ReasonContactLinkFieldTypeAlreadyExists:         {Template: "Link field type already exists"},
	ReasonCommonMemberNotFound:                      {Template: "Member not found"},
	ReasonNextcloudFileOrFolderNotFound:             {Template: "Nextcloud file or folder not found"},
	ReasonNextcloudLinkNotFound:                     {Template: "Nextcloud link not found"},
	ReasonNextcloudLinkAlreadyExists:                {Template: "Nextcloud link already exists"},
	ReasonContactNoteNotFound:                       {Template: "Note not found"},
	ReasonOccasionEventNotFound:                     {Template: "Occasion event not found"},
	ReasonOccasionObligationNotFound:                {Template: "Occasion obligation not found"},
	ReasonPaperlessDocumentNotFound:                 {Template: "Paperless document not found"},
	ReasonPaperlessLinkNotFound:                     {Template: "Paperless link not found"},
	ReasonPaperlessLinkAlreadyExists:                {Template: "Paperless link already exists"},
	ReasonAuthPasskeyNotFound:                       {Template: "Passkey not found"},
	ReasonAuthPasskeyAlreadyExists:                  {Template: "Passkey already exists"},
	ReasonPhotoNotFound:                             {Template: "Photo not found"},
	ReasonAccountPreferenceNotFound:                 {Template: "Preference not found"},
	ReasonNotificationPushSubscriptionNotFound:      {Template: "Push subscription not found"},
	ReasonContactReachOutSuggestionNotFound:         {Template: "Reach-out suggestion not found"},
	ReasonAccountRecipientUserNotFound:              {Template: "Recipient user not found"},
	ReasonRelationshipEdgeNotFound:                  {Template: "Relationship edge not found"},
	ReasonRelationshipEdgeAlreadyExists:             {Template: "Relationship edge already exists"},
	ReasonContactReminderNotFound:                   {Template: "Reminder not found"},
	ReasonContactReminderCompletionNotFound:         {Template: "Reminder completion not found"},
	ReasonSeafileLibraryOrFileNotFound:              {Template: "Seafile library or file not found"},
	ReasonSeafileLinkNotFound:                       {Template: "Seafile link not found"},
	ReasonSeafileLinkAlreadyExists:                  {Template: "Seafile link already exists"},
	ReasonAuthSessionNotFound:                       {Template: "session not found"},
	ReasonCommonSyncConflictNotFound:                {Template: "Sync conflict not found"},
	ReasonContactTagNotFound:                        {Template: "Tag not found"},
	ReasonContactTaggingNotFound:                    {Template: "Tagging not found"},
	ReasonContactTaggingAlreadyExists:               {Template: "Tagging already exists"},
	ReasonPhotoThumbnailNotFound:                    {Template: "Thumbnail not found"},
	ReasonAccountUserNotFound:                       {Template: "User not found"},
	ReasonAccountUserAlreadyExists:                  {Template: "User already exists"},
	ReasonIntegrationWebhookNotFound:                {Template: "Webhook not found"},
	ReasonContactSomeNotFound:                       {Template: "One or more contacts not found"},

	// Sign-in errors (issue #2101).
	ReasonAuthAccountLocked:                    {Template: "Too many failed attempts. Try again in {retry_after} s", Params: []string{"retry_after"}},
	ReasonAuthPasswordBreached:                 {Template: "This password has appeared in a known data breach; please choose a different one."},
	ReasonAuthPasswordTooLong:                  {Template: "password must not exceed 72 characters"},
	ReasonAuthPasswordChangeDisabledDemo:       {Template: "Password changes are disabled in demo mode"},
	ReasonAuthResetTokenInvalid:                {Template: "Password reset token is invalid or expired"},
	ReasonAuthTwoFactorAlreadyEnabled:          {Template: "Two-factor authentication is already enabled"},
	ReasonAuthTwoFactorUnavailableOIDC:         {Template: "Two-factor authentication is not available for accounts that sign in through an identity provider"},
	ReasonAuthTwoFactorSetupRequired:           {Template: "Run 2FA setup first to generate a secret"},
	ReasonAuthTwoFactorCodeInvalid:             {Template: "Invalid code. Please try again."},
	ReasonAuthTwoFactorNotEnabled:              {Template: "Two-factor authentication is not enabled"},
	ReasonAuthTwoFactorLoginNotPending:         {Template: "No pending two-factor login found. Please sign in again."},
	ReasonAuthTwoFactorSessionInvalid:          {Template: "Invalid or expired two-factor session. Please sign in again."},
	ReasonAuthTwoFactorNoLongerEnabled:         {Template: "Two-factor authentication is no longer enabled. Please sign in again."},
	ReasonAuthPasskeyNameTooLong:               {Template: "Name must be 100 characters or fewer"},
	ReasonAuthPasskeyNameLengthInvalid:         {Template: "Name must be between 1 and 100 characters"},
	ReasonAuthPasskeyRegistrationNotStarted:    {Template: "No passkey registration in progress. Start again."},
	ReasonAuthPasskeyRegistrationFailed:        {Template: "Passkey registration could not be verified"},
	ReasonAuthPasskeyNoneRegistered:            {Template: "No passkey is registered for this account"},
	ReasonAuthPasskeyNoneRegisteredSignInAgain: {Template: "No passkey is registered for this account. Please sign in again."},
	ReasonAuthPasskeyLoginNotStarted:           {Template: "No passkey login in progress. Start again."},
	ReasonAuthPasskeyVerificationFailed:        {Template: "Passkey could not be verified"},
	ReasonAuthPasskeyNoOtherRegistered:         {Template: "No other passkey is registered to verify with"},
	ReasonValidationInvalidJSON:                {Template: "Invalid JSON"},
	ReasonValidationInvalidRequestBody:         {Template: "Invalid request body"},
}

// notFoundReasons maps the string-literal resources passed to ErrNotFound to their
// reason code. A resource not listed (including one passed as a variable) gets
// no code: that is the fallback, not a bug (issue #2099).
var notFoundReasons = map[string]ReasonCode{
	"API token":                ReasonAccountApiTokenNotFound,
	"Activity":                 ReasonActivityNotFound,
	"Address":                  ReasonContactAddressNotFound,
	"Attachment":               ReasonAttachmentNotFound,
	"Attachment file":          ReasonAttachmentFileNotFound,
	"Audit event":              ReasonAdminAuditEventNotFound,
	"Cadence policy":           ReasonContactCadencePolicyNotFound,
	"Calendar subscription":    ReasonCalendarSubscriptionNotFound,
	"Circle":                   ReasonCircleNotFound,
	"Circle membership":        ReasonCircleMembershipNotFound,
	"Contact":                  ReasonContactNotFound,
	"Contact share":            ReasonContactShareNotFound,
	"Contact subscription":     ReasonContactSubscriptionNotFound,
	"Conversation agenda item": ReasonContactConversationAgendaItemNotFound,
	"Data decay policy":        ReasonContactDataDecayPolicyNotFound,
	"Device grant":             ReasonAuthDeviceGrantNotFound,
	"Device registration":      ReasonNotificationDeviceRegistrationNotFound,
	"Event attendee":           ReasonOccasionEventAttendeeNotFound,
	"External activity":        ReasonContactExternalActivityNotFound,
	"External identity":        ReasonIntegrationExternalIdentityNotFound,
	"Feed":                     ReasonFeedNotFound,
	"Field definition":         ReasonContactFieldDefinitionNotFound,
	"GeoPulse resource":        ReasonGeopulseResourceNotFound,
	"Gift":                     ReasonContactGiftNotFound,
	"Household":                ReasonHouseholdNotFound,
	"Household membership":     ReasonHouseholdMembershipNotFound,
	"Image":                    ReasonPhotoImageNotFound,
	"Immich link":              ReasonImmichLinkNotFound,
	"Immich person":            ReasonImmichPersonNotFound,
	"Life event":               ReasonContactLifeEventNotFound,
	"Link field type":          ReasonContactLinkFieldTypeNotFound,
	"Member":                   ReasonCommonMemberNotFound,
	"Nextcloud file or folder": ReasonNextcloudFileOrFolderNotFound,
	"Nextcloud link":           ReasonNextcloudLinkNotFound,
	"Note":                     ReasonContactNoteNotFound,
	"Occasion event":           ReasonOccasionEventNotFound,
	"Occasion obligation":      ReasonOccasionObligationNotFound,
	"One or more contacts":     ReasonContactSomeNotFound,
	"Paperless document":       ReasonPaperlessDocumentNotFound,
	"Paperless link":           ReasonPaperlessLinkNotFound,
	"Passkey":                  ReasonAuthPasskeyNotFound,
	"Photo":                    ReasonPhotoNotFound,
	"Preference":               ReasonAccountPreferenceNotFound,
	"Push subscription":        ReasonNotificationPushSubscriptionNotFound,
	"Reach-out suggestion":     ReasonContactReachOutSuggestionNotFound,
	"Recipient user":           ReasonAccountRecipientUserNotFound,
	"Relationship edge":        ReasonRelationshipEdgeNotFound,
	"Reminder":                 ReasonContactReminderNotFound,
	"Reminder completion":      ReasonContactReminderCompletionNotFound,
	"Seafile library or file":  ReasonSeafileLibraryOrFileNotFound,
	"Seafile link":             ReasonSeafileLinkNotFound,
	"Sync conflict":            ReasonCommonSyncConflictNotFound,
	"Tag":                      ReasonContactTagNotFound,
	"Tagging":                  ReasonContactTaggingNotFound,
	"Thumbnail":                ReasonPhotoThumbnailNotFound,
	"User":                     ReasonAccountUserNotFound,
	"Webhook":                  ReasonIntegrationWebhookNotFound,
	"session":                  ReasonAuthSessionNotFound,
}

// alreadyExistsReasons is notFoundReasons for ErrAlreadyExists.
var alreadyExistsReasons = map[string]ReasonCode{
	"Cadence policy":                 ReasonContactCadencePolicyAlreadyExists,
	"Circle membership":              ReasonCircleMembershipAlreadyExists,
	"Data decay policy":              ReasonContactDataDecayPolicyAlreadyExists,
	"Event attendee":                 ReasonOccasionEventAttendeeAlreadyExists,
	"External activity":              ReasonContactExternalActivityAlreadyExists,
	"External identity":              ReasonIntegrationExternalIdentityAlreadyExists,
	"Field definition":               ReasonContactFieldDefinitionAlreadyExists,
	"Household membership":           ReasonHouseholdMembershipAlreadyExists,
	"Household suggestion dismissal": ReasonHouseholdSuggestionDismissalAlreadyExists,
	"Immich link":                    ReasonImmichLinkAlreadyExists,
	"Link field type":                ReasonContactLinkFieldTypeAlreadyExists,
	"Nextcloud link":                 ReasonNextcloudLinkAlreadyExists,
	"Paperless link":                 ReasonPaperlessLinkAlreadyExists,
	"Passkey":                        ReasonAuthPasskeyAlreadyExists,
	"Relationship edge":              ReasonRelationshipEdgeAlreadyExists,
	"Seafile link":                   ReasonSeafileLinkAlreadyExists,
	"Tagging":                        ReasonContactTaggingAlreadyExists,
	"User":                           ReasonAccountUserAlreadyExists,
}

var reasonPlaceholder = regexp.MustCompile(`\{([^{}]*)\}`)

// WithReason attaches a reason code and params and sets Message and
// Details["reason"] to the rendered English template. If the code is unknown
// or the params do not match the registered names exactly, the error is left
// untouched (clients fall back to English) and an info line is logged; a
// request path never panics over this.
func (e *AppError) WithReason(code ReasonCode, params map[string]any) *AppError {
	return e.applyReason(code, params, true)
}

// withDefaultReason is WithReason for a constructor's built-in default code.
// It leaves Details alone: the sentence is the message itself, so a
// details["reason"] copy would be noise (and a 5xx must carry no details).
func (e *AppError) withDefaultReason(code ReasonCode, params map[string]any) *AppError {
	return e.applyReason(code, params, false)
}

func (e *AppError) applyReason(code ReasonCode, params map[string]any, setDetail bool) *AppError {
	spec, ok := reasonRegistry[code]
	if !ok || !paramsMatch(spec.Params, params) {
		logger.Logger.Info().
			Str(logger.FieldEvent, "reason_params_mismatch").
			Str(logger.FieldReasonCode, string(code)).
			Msg("reason params do not match registry; sending error without reason")
		return e
	}
	rendered := reasonPlaceholder.ReplaceAllStringFunc(spec.Template, func(m string) string {
		return fmt.Sprint(params[m[1:len(m)-1]])
	})
	e.ReasonCode = string(code)
	e.ReasonParams = params
	e.Message = rendered
	if setDetail {
		e.WithDetails("reason", rendered)
	}
	return e
}

func paramsMatch(names []string, params map[string]any) bool {
	if len(names) != len(params) {
		return false
	}
	for _, n := range names {
		if _, ok := params[n]; !ok {
			return false
		}
	}
	return true
}

// ReasonCodes returns every registered code, sorted. Client drift tests
// consume it.
func ReasonCodes() []ReasonCode {
	out := make([]ReasonCode, 0, len(reasonRegistry))
	for c := range reasonRegistry {
		out = append(out, c)
	}
	sort.Slice(out, func(i, j int) bool { return strings.Compare(string(out[i]), string(out[j])) < 0 })
	return out
}

// ReasonSpecFor returns the English template and declared param order.
func ReasonSpecFor(code ReasonCode) (template string, params []string, ok bool) {
	spec, ok := reasonRegistry[code]
	if !ok {
		return "", nil, false
	}
	return spec.Template, append([]string(nil), spec.Params...), true
}
