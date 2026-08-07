package routes

import (
	"mycorrhizal/caldav"
	"mycorrhizal/carddav"
	"mycorrhizal/config"
	"mycorrhizal/controllers"
	"mycorrhizal/middleware"
	"mycorrhizal/models"
	"mycorrhizal/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func RegisterRoutes(router *gin.Engine, cfg *config.Config, db *gorm.DB, oidcProvider *services.OIDCProvider) {

	// Health check endpoint (no versioning, standard practice)
	router.GET("/health", controllers.HealthCheck)

	// API v1 routes
	v1 := router.Group("/api/v1")
	{
		// OIDC routes (config always public; login/callback only when OIDC is enabled)
		v1.GET("/auth/oidc/config", controllers.OIDCConfigHandler(cfg))
		if cfg.OIDC.Enabled && oidcProvider != nil {
			v1.GET("/auth/oidc/login", middleware.AuthRateLimitMiddleware(), controllers.OIDCLoginHandler(oidcProvider, cfg))
			v1.GET("/auth/oidc/callback", middleware.AuthRateLimitMiddleware(), controllers.OIDCCallbackHandler(oidcProvider, cfg))
		}

		// Public routes (no authentication required, strict rate limiting)
		v1.POST("/register", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.UserRegistrationInput{}), controllers.RegisterUser(cfg))
		v1.POST("/login", middleware.AuthRateLimitMiddleware(), func(c *gin.Context) {
			controllers.LoginUser(c, cfg)
		})
		v1.POST("/logout", func(c *gin.Context) {
			controllers.LogoutUser(c, cfg, oidcProvider)
		})
		v1.POST("/check-password-strength", middleware.AuthRateLimitMiddleware(), controllers.CheckPasswordStrength)
		v1.POST("/password-reset/request", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.PasswordResetRequestInput{}), func(c *gin.Context) {
			controllers.RequestPasswordReset(c, cfg)
		})
		v1.POST("/password-reset/confirm", middleware.AuthRateLimitMiddleware(), middleware.ValidateJSONMiddleware(&models.PasswordResetConfirmInput{}), controllers.ConfirmPasswordReset)

		// Protected routes (authentication required, general rate limiting)
		protected := v1.Group("/")
		protected.Use(middleware.APIRateLimitMiddleware())
		protected.Use(middleware.AuthMiddleware(cfg))
		{
			protected.POST("/users/change-password", middleware.ValidateJSONMiddleware(&models.ChangePasswordInput{}), controllers.ChangePassword)
			protected.PATCH("/users/language", controllers.UpdateLanguage)
			protected.PATCH("/users/date-format", controllers.UpdateDateFormat)
			protected.GET("/users/enabled-contact-fields", controllers.GetEnabledContactFields)
			protected.PATCH("/users/enabled-contact-fields", middleware.ValidateJSONMiddleware(&models.EnabledContactFieldsInput{}), controllers.UpdateEnabledContactFields)
			protected.GET("/users/me", controllers.GetCurrentUser)
			// P1 contact sharing recipient picker (docs/fork-plan/tickets/
			// 31-P1-contact-sharing.md) — the only non-admin way to discover
			// other users on the instance; deliberately thinner than
			// admin-only ListUsers (id+username only).
			protected.GET("/users/directory", controllers.ListUserDirectory)

			// Contact routes
			protected.GET("/contacts", controllers.GetContacts)
			protected.GET("/contacts/circles", controllers.GetCircles)
			protected.GET("/contacts/random", controllers.GetContactsRandom)
			protected.GET("/contacts/birthdays", controllers.GetUpcomingBirthdays)
			protected.POST("/contacts/merge/preview", middleware.ValidateJSONMiddleware(&models.ContactMergeRequest{}), controllers.PreviewContactMerge)
			protected.POST("/contacts/merge", middleware.ValidateJSONMiddleware(&models.ContactMergeRequest{}), controllers.CommitContactMerge)
			protected.POST("/contacts", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), controllers.CreateContact)
			// N5 bulk operations — registered before /contacts/:id so the
			// literal path is never captured as a contact ID.
			protected.POST("/contacts/bulk", middleware.ValidateJSONMiddleware(&models.BulkContactOperationInput{}), controllers.BulkContactOperation)
			protected.GET("/contacts/:id", controllers.GetContact)
			// N2 prep view: read-only aggregation of everything the user
			// wants to remember before seeing this contact. Registered after
			// GET /contacts/:id so the literal /briefing path is never
			// captured as a contact ID.
			protected.GET("/contacts/:id/briefing", controllers.GetContactBriefing)
			protected.PUT("/contacts/:id", middleware.ValidateJSONMiddleware(&models.ContactRecordInput{}), controllers.UpdateContact)
			protected.DELETE("/contacts/:id", controllers.DeleteContact)
			protected.POST("/contacts/:id/archive", controllers.ArchiveContact)
			protected.POST("/contacts/:id/unarchive", controllers.UnarchiveContact)

			// Contact import routes (CSV)
			protected.POST("/contacts/import/upload", controllers.UploadCSVForImport)
			protected.POST("/contacts/import/preview", middleware.ValidateJSONMiddleware(&models.ImportPreviewRequest{}), controllers.PreviewImport)
			protected.POST("/contacts/import/confirm", middleware.ValidateJSONMiddleware(&models.ImportConfirmRequest{}), controllers.ConfirmImport)

			// Contact import routes (VCF)
			protected.POST("/contacts/import/vcf/upload", func(c *gin.Context) {
				controllers.UploadVCFForImport(c, cfg)
			})
			protected.POST("/contacts/import/vcf/confirm", middleware.ValidateJSONMiddleware(&models.ImportConfirmRequest{}), func(c *gin.Context) {
				controllers.ConfirmVCFImport(c, cfg)
			})

			// Contact import routes (JSContact JSON) — WP-71 Gap 4 extension.
			// Confirmation deliberately reuses /contacts/import/vcf/confirm
			// (see UploadJSContactForImport's doc comment): the session it
			// creates is format-agnostic once parsed into []VCFContactData.
			protected.POST("/contacts/import/jscontact/upload", controllers.UploadJSContactForImport)

			// P1 contact sharing (docs/fork-plan/tickets/31-P1-contact-sharing.md)
			// — one-time filtered copy between two users on the same
			// instance. Accept is preview-only (parses the stored payload
			// through the same import pipeline above); Confirm delegates to
			// ConfirmVCF (the exact method /contacts/import/vcf/confirm
			// uses) and only then flips the share to accepted.
			protected.POST("/contact-shares", middleware.ValidateJSONMiddleware(&models.ContactShareInput{}), controllers.CreateContactShare)
			protected.GET("/contact-shares/incoming", controllers.ListIncomingContactShares)
			protected.GET("/contact-shares/outgoing", controllers.ListOutgoingContactShares)
			protected.POST("/contact-shares/:id/accept", controllers.AcceptContactShare)
			protected.POST("/contact-shares/:id/confirm", middleware.ValidateJSONMiddleware(&models.ImportConfirmRequest{}), func(c *gin.Context) {
				controllers.ConfirmContactShare(c, cfg)
			})
			protected.POST("/contact-shares/:id/decline", controllers.DeclineContactShare)

			// RelationshipEdge routes (graph-model relationship API; replaces the
			// legacy /contacts/:id/relationships stack, removed in §3d WP5)
			protected.POST("/relationship-edges", middleware.ValidateJSONMiddleware(&models.RelationshipEdgeInput{}), controllers.CreateRelationshipEdge)
			protected.GET("/relationship-edges", controllers.ListRelationshipEdges)
			protected.GET("/relationship-edges/:id", controllers.GetRelationshipEdge)
			protected.PUT("/relationship-edges/:id", middleware.ValidateJSONMiddleware(&models.RelationshipEdgeInput{}), controllers.UpdateRelationshipEdge)
			protected.DELETE("/relationship-edges/:id", controllers.DeleteRelationshipEdge)
			protected.PATCH("/relationship-edges/:id/accept", controllers.AcceptRelationshipEdge)

			// Profile picture routes
			protected.POST("/contacts/:id/profile_picture", func(c *gin.Context) {
				controllers.AddPhotoToContact(c, cfg)
			})
			protected.GET("/contacts/:id/profile_picture", func(c *gin.Context) {
				controllers.GetProfilePicture(c, cfg)
			})

			// Image proxy route (for fetching images from external URLs)
			protected.GET("/proxy/image", controllers.ProxyImage)

			// Note routes
			protected.GET("/contacts/:id/notes", controllers.GetNotesForContact)
			protected.POST("/contacts/:id/notes", middleware.ValidateJSONMiddleware(&models.NoteInput{}), controllers.CreateNote)
			protected.GET("/notes/:id", controllers.GetNote)
			protected.GET("/notes", controllers.GetUnassignedNotes)
			protected.POST("/notes", middleware.ValidateJSONMiddleware(&models.NoteInput{}), controllers.CreateUnassignedNote)
			protected.PUT("/notes/:id", middleware.ValidateJSONMiddleware(&models.NoteInput{}), controllers.UpdateNote)
			protected.DELETE("/notes/:id", controllers.DeleteNote)

			// Activity routes
			protected.GET("/contacts/:id/activities", controllers.GetActivitiesForContact)
			protected.POST("/activities", middleware.ValidateJSONMiddleware(&models.ActivityInput{}), controllers.CreateActivity)
			protected.GET("/activities", controllers.GetActivities)
			protected.GET("/activities/:id", controllers.GetActivity)
			protected.PUT("/activities/:id", middleware.ValidateJSONMiddleware(&models.ActivityInput{}), controllers.UpdateActivity)
			protected.DELETE("/activities/:id", controllers.DeleteActivity)

			// Circle routes (WP-84c)
			protected.POST("/circles", middleware.ValidateJSONMiddleware(&models.CircleInput{}), controllers.CreateCircle)
			protected.GET("/circles", controllers.ListCircles)
			protected.GET("/circles/:id", controllers.GetCircle)
			protected.PUT("/circles/:id", middleware.ValidateJSONMiddleware(&models.CircleInput{}), controllers.UpdateCircle)
			protected.DELETE("/circles/:id", controllers.DeleteCircle)
			protected.POST("/circles/:id/members", middleware.ValidateJSONMiddleware(&models.CircleMemberInput{}), controllers.AddCircleMember)
			protected.DELETE("/circles/:id/members/:vcard_uid", controllers.RemoveCircleMember)

			// LinkFieldType routes (T34 — docs/fork-plan/tickets/43-T34-contact-field-linking.md)
			protected.POST("/link-field-types", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), controllers.CreateLinkFieldType)
			protected.GET("/link-field-types", controllers.ListLinkFieldTypes)
			protected.GET("/link-field-types/:id", controllers.GetLinkFieldType)
			protected.PUT("/link-field-types/reorder", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeReorderInput{}), controllers.ReorderLinkFieldTypes)
			protected.PUT("/link-field-types/:id", middleware.ValidateJSONMiddleware(&models.LinkFieldTypeInput{}), controllers.UpdateLinkFieldType)
			protected.DELETE("/link-field-types/:id", controllers.DeleteLinkFieldType)

			// Household routes (T1 — docs/fork-plan/tickets/09-T1-households.md)
			protected.POST("/households", middleware.ValidateJSONMiddleware(&models.HouseholdInput{}), controllers.CreateHousehold)
			protected.GET("/households", controllers.ListHouseholds)
			protected.GET("/households/:id", controllers.GetHousehold)
			protected.PUT("/households/:id", middleware.ValidateJSONMiddleware(&models.HouseholdInput{}), controllers.UpdateHousehold)
			protected.DELETE("/households/:id", controllers.DeleteHousehold)
			protected.POST("/households/:id/members", middleware.ValidateJSONMiddleware(&models.HouseholdMemberInput{}), controllers.AddHouseholdMember)
			protected.DELETE("/households/:id/members/:vcard_uid", controllers.RemoveHouseholdMember)
			protected.PATCH("/households/:id/members/:vcard_uid", controllers.UpdateHouseholdMember)
			protected.POST("/households/:id/suggest-relationships", controllers.SuggestHouseholdRelationships)

			// T40 address-based household suggestions (docs/fork-plan/
			// tickets/49-T40-household-suggestions-shared-address.md). The
			// literal /suggestions paths are registered before any /:id
			// capture would matter (there is no GET /households/:id/... below
			// that could collide), and accept/dismiss re-validate the group
			// server-side from its member VCardUIDs.
			protected.POST("/households/suggest-addresses", controllers.SuggestAddressHouseholds)
			protected.POST("/households/suggestions/accept", middleware.ValidateJSONMiddleware(&models.AcceptHouseholdSuggestionInput{}), controllers.AcceptAddressHouseholdSuggestion)
			protected.POST("/households/suggestions/dismiss", middleware.ValidateJSONMiddleware(&models.DismissHouseholdSuggestionInput{}), controllers.DismissAddressHouseholdSuggestion)

			// Tag routes (WP-84c)
			protected.POST("/tags", middleware.ValidateJSONMiddleware(&models.TagInput{}), controllers.CreateTag)
			protected.GET("/tags", controllers.ListTags)
			protected.GET("/tags/:id", controllers.GetTag)
			protected.PUT("/tags/:id", middleware.ValidateJSONMiddleware(&models.TagInput{}), controllers.UpdateTag)
			protected.DELETE("/tags/:id", controllers.DeleteTag)
			protected.POST("/tags/:id/contacts", middleware.ValidateJSONMiddleware(&models.ContactTagInput{}), controllers.AddContactTag)
			protected.DELETE("/tags/:id/contacts/:vcard_uid", controllers.RemoveContactTag)

			// Custom field definition routes (T6 — docs/fork-plan/tickets/
			// 11-T6-custom-fields-api.md). FieldValue routes are nested under
			// the contact (GET/PUT /contacts/:id/field-values) per the
			// endpoint-shape decision documented in
			// field_definition_controller.go.
			protected.POST("/field-definitions", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), controllers.CreateFieldDefinition)
			protected.GET("/field-definitions", controllers.ListFieldDefinitions)
			protected.GET("/field-definitions/:id", controllers.GetFieldDefinition)
			protected.PUT("/field-definitions/:id", middleware.ValidateJSONMiddleware(&models.FieldDefinitionInput{}), controllers.UpdateFieldDefinition)
			protected.DELETE("/field-definitions/:id", controllers.DeleteFieldDefinition)
			protected.GET("/contacts/:id/field-values", controllers.ListContactFieldValues)
			protected.PUT("/contacts/:id/field-values", middleware.ValidateJSONMiddleware(&models.ContactFieldValuesInput{}), controllers.ReplaceContactFieldValues)

			// LifeEvent routes (WP-84c)
			protected.POST("/life-events", middleware.ValidateJSONMiddleware(&models.LifeEventInput{}), controllers.CreateLifeEvent)
			protected.GET("/life-events", controllers.ListLifeEvents)
			protected.GET("/life-events/:id", controllers.GetLifeEvent)
			protected.PUT("/life-events/:id", middleware.ValidateJSONMiddleware(&models.LifeEventInput{}), controllers.UpdateLifeEvent)
			protected.DELETE("/life-events/:id", controllers.DeleteLifeEvent)

			// ConversationAgenda routes (T21 — docs/fork-plan/tickets/
			// 21-T21-conversation-agenda.md). /:id/discuss is registered
			// after /:id reads but the PATCH method never collides with
			// them.
			protected.POST("/conversation-agenda", middleware.ValidateJSONMiddleware(&models.ConversationAgendaInput{}), controllers.CreateConversationAgenda)
			protected.GET("/conversation-agenda", controllers.ListConversationAgenda)
			protected.GET("/conversation-agenda/:id", controllers.GetConversationAgenda)
			protected.PUT("/conversation-agenda/:id", middleware.ValidateJSONMiddleware(&models.ConversationAgendaInput{}), controllers.UpdateConversationAgenda)
			protected.PATCH("/conversation-agenda/:id/discuss", middleware.ValidateJSONMiddleware(&models.ConversationAgendaDiscussInput{}), controllers.DiscussConversationAgenda)
			protected.DELETE("/conversation-agenda/:id", controllers.DeleteConversationAgenda)

			// Gift routes (T20b — docs/fork-plan/tickets/28-T20b-gift-tracking.md)
			protected.POST("/gifts", middleware.ValidateJSONMiddleware(&models.GiftInput{}), controllers.CreateGift)
			protected.GET("/gifts", controllers.ListGifts)
			protected.GET("/gifts/:id", controllers.GetGift)
			protected.PUT("/gifts/:id", middleware.ValidateJSONMiddleware(&models.GiftInput{}), controllers.UpdateGift)
			protected.DELETE("/gifts/:id", controllers.DeleteGift)

			// Preference routes (T20a — docs/fork-plan/tickets/10-T20a-preferences.md)
			protected.POST("/preferences", middleware.ValidateJSONMiddleware(&models.PreferenceInput{}), controllers.CreatePreference)
			protected.GET("/preferences", controllers.ListPreferences)
			protected.GET("/preferences/:id", controllers.GetPreference)
			protected.PUT("/preferences/:id", middleware.ValidateJSONMiddleware(&models.PreferenceInput{}), controllers.UpdatePreference)
			protected.DELETE("/preferences/:id", controllers.DeletePreference)

			// CadencePolicy routes (T19 — docs/fork-plan/tickets/20-T19-cadence.md).
			// /overdue is registered before /:id so the literal path is never
			// captured as a policy ID.
			protected.GET("/cadence-policies/overdue", controllers.GetOverdueCadences)
			protected.POST("/cadence-policies", middleware.ValidateJSONMiddleware(&models.CadencePolicyInput{}), controllers.CreateCadencePolicy)
			protected.GET("/cadence-policies", controllers.ListCadencePolicies)
			protected.GET("/cadence-policies/:id", controllers.GetCadencePolicy)
			protected.PUT("/cadence-policies/:id", middleware.ValidateJSONMiddleware(&models.CadencePolicyInput{}), controllers.UpdateCadencePolicy)
			protected.DELETE("/cadence-policies/:id", controllers.DeleteCadencePolicy)

			// Reminder routes
			protected.GET("/reminders", controllers.GetAllReminders)
			protected.GET("/reminders/upcoming", controllers.GetUpcomingReminders)
			protected.GET("/contacts/:id/reminders", controllers.GetRemindersForContact)
			protected.POST("/contacts/:id/reminders", middleware.ValidateJSONMiddleware(&models.Reminder{}), controllers.CreateReminder)
			protected.GET("/reminders/:id", controllers.GetReminder)
			protected.PUT("/reminders/:id", middleware.ValidateJSONMiddleware(&models.Reminder{}), controllers.UpdateReminder)
			protected.POST("/reminders/:id/complete", controllers.CompleteReminder)
			protected.DELETE("/reminders/:id", controllers.DeleteReminder)

			// Reminder completion routes (for timeline)
			protected.GET("/contacts/:id/reminder-completions", controllers.GetCompletionsForContact)
			protected.DELETE("/reminder-completions/:id", controllers.DeleteCompletion)

			// Export routes
			protected.GET("/export", controllers.ExportData)
			protected.GET("/export/vcf", func(c *gin.Context) {
				controllers.ExportContactsAsVCF(c, cfg.ProfilePhotoDir)
			})
			protected.GET("/export/jscontact", controllers.ExportContactsAsJSContact)

			// Graph/Network visualization route
			protected.GET("/graph", controllers.GetGraph)
			// Graph traversal / multi-hop chains (T10) — GET /graph/connections
			// with ?from=<VCardUID>&depth=<n>&relation=<token|synonym>
			protected.GET("/graph/connections", controllers.GetGraphConnections)

			// Full-text search (T11) — GET /search?q=<term>&limit=<n> across
			// contacts, notes, and interactions
			protected.GET("/search", controllers.SearchAll)

			// API token routes
			protected.GET("/api-tokens", controllers.ListApiTokens)
			protected.POST("/api-tokens", middleware.ValidateJSONMiddleware(&models.ApiTokenInput{}), controllers.CreateApiToken)
			protected.DELETE("/api-tokens/:id", controllers.RevokeApiToken)

			// Webhook routes
			protected.GET("/webhooks", controllers.ListWebhooks)
			protected.POST("/webhooks", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), controllers.CreateWebhook)
			protected.GET("/webhooks/:id", controllers.GetWebhook)
			protected.PUT("/webhooks/:id", middleware.ValidateJSONMiddleware(&models.WebhookInput{}), controllers.UpdateWebhook)
			protected.DELETE("/webhooks/:id", controllers.DeleteWebhook)
			protected.POST("/webhooks/:id/test", controllers.TestWebhook)
			protected.GET("/webhooks/:id/deliveries", controllers.GetWebhookDeliveries)

			// Audit trail routes (T18 — docs/fork-plan/tickets/
			// 34-T18-audit-trail.md). Read-only log surface + update-only undo.
			protected.GET("/audit", controllers.ListAuditEvents)
			protected.POST("/audit/:id/undo", controllers.UndoAuditEvent)

			// Notification routes (N9 — docs/fork-plan/tickets/
			// 30-N9-notification-channels.md). Per-user channel config, the
			// per-user channel toggles, per-channel test notification, and Web
			// Push device registrations.
			protected.GET("/notifications/config", controllers.GetNotificationConfig)
			protected.PUT("/notifications/config", middleware.ValidateJSONMiddleware(&models.NotificationConfigInput{}), controllers.SaveNotificationConfig)
			protected.POST("/notifications/config/test", controllers.TestNotificationChannel)
			protected.GET("/notifications/push-subscriptions", controllers.ListPushSubscriptions)
			protected.POST("/notifications/push-subscriptions", middleware.ValidateJSONMiddleware(&models.PushSubscriptionInput{}), controllers.CreatePushSubscription)
			protected.DELETE("/notifications/push-subscriptions/:id", controllers.DeletePushSubscription)

			// Calendar subscription routes (CalDAV/iCS activity import)
			protected.GET("/calendars", controllers.ListCalendarSubscriptions)
			protected.POST("/calendars", middleware.ValidateJSONMiddleware(&models.CalendarSubscriptionInput{}), controllers.CreateCalendarSubscription)
			protected.PUT("/calendars/:id", middleware.ValidateJSONMiddleware(&models.CalendarSubscriptionInput{}), controllers.UpdateCalendarSubscription)
			protected.DELETE("/calendars/:id", controllers.DeleteCalendarSubscription)
			protected.POST("/calendars/:id/sync", controllers.SyncCalendarSubscription)

			// Contact subscription routes (CardDAV client: sync contacts in
			// from an external address book, WP-73b)
			protected.GET("/contact-subscriptions", controllers.ListContactSubscriptions)
			protected.POST("/contact-subscriptions", middleware.ValidateJSONMiddleware(&models.ContactSubscriptionInput{}), controllers.CreateContactSubscription)
			protected.PUT("/contact-subscriptions/:id", middleware.ValidateJSONMiddleware(&models.ContactSubscriptionInput{}), controllers.UpdateContactSubscription)
			protected.DELETE("/contact-subscriptions/:id", controllers.DeleteContactSubscription)
			protected.POST("/contact-subscriptions/:id/sync", controllers.SyncContactSubscription)

			// ExternalIdentity routes (T14 — docs/fork-plan/tickets/
			// 32-T14-external-link-substrate.md): the generic integration
			// substrate's link/enrichment CRUD. System-agnostic — no
			// integration-specific route lives here.
			protected.POST("/external-identities", middleware.ValidateJSONMiddleware(&models.ExternalIdentityInput{}), controllers.CreateExternalIdentity)
			protected.GET("/external-identities", controllers.ListExternalIdentities)
			protected.GET("/external-identities/:id", controllers.GetExternalIdentity)
			protected.PUT("/external-identities/:id", middleware.ValidateJSONMiddleware(&models.ExternalIdentityInput{}), controllers.UpdateExternalIdentity)
			protected.DELETE("/external-identities/:id", controllers.DeleteExternalIdentity)

			// ExternalActivity routes (T14): generic enrichment events,
			// linkable into a contact's timeline via ?contact_id=.
			protected.POST("/external-activities", middleware.ValidateJSONMiddleware(&models.ExternalActivityInput{}), controllers.CreateExternalActivity)
			protected.GET("/external-activities", controllers.ListExternalActivities)
			protected.GET("/external-activities/:id", controllers.GetExternalActivity)
			protected.PUT("/external-activities/:id", middleware.ValidateJSONMiddleware(&models.ExternalActivityInput{}), controllers.UpdateExternalActivity)
			protected.DELETE("/external-activities/:id", controllers.DeleteExternalActivity)

			// Immich routes (T15/T16 — docs/fork-plan/tickets/
			// 33-T15-T16-immich.md): the first concrete integration on the
			// generic substrate. Config is per-user-global; links are written
			// as ExternalIdentity (system: "immich"), enrichment as
			// ExternalActivity. /contacts/:vcard_uid/thumbnail is the hardened
			// person-thumbnail proxy (same SVG rejection + Content-Disposition
			// as /proxy/image).
			protected.GET("/immich/config", controllers.GetImmichConfig)
			protected.PUT("/immich/config", middleware.ValidateJSONMiddleware(&models.ImmichConfigInput{}), controllers.SaveImmichConfig)
			protected.DELETE("/immich/config", controllers.DeleteImmichConfig)
			protected.POST("/immich/test-connection", controllers.TestImmichConnection)
			protected.GET("/immich/people", controllers.ListImmichPeople)
			protected.POST("/immich/sync", controllers.SyncImmichNow)
			protected.POST("/immich/contacts/:vcard_uid/link", controllers.LinkImmichContact)
			protected.DELETE("/immich/contacts/:vcard_uid/link", controllers.UnlinkImmichContact)
			protected.GET("/immich/contacts/:vcard_uid/summary", controllers.GetImmichContactSummary)
			protected.GET("/immich/contacts/:vcard_uid/thumbnail", controllers.GetImmichThumbnail)
			protected.GET("/immich/contacts/:vcard_uid/assets", controllers.ListImmichContactAssets)
			protected.GET("/immich/contacts/:vcard_uid/assets/:asset_id/image", controllers.GetImmichAssetImage)
		}

		// Admin routes (admin authentication required)
		admin := v1.Group("/admin")
		admin.Use(middleware.APIRateLimitMiddleware())
		admin.Use(middleware.AuthMiddleware(cfg))
		admin.Use(middleware.AdminMiddleware())
		{
			admin.GET("/users", controllers.ListUsers)
			admin.POST("/users", middleware.ValidateJSONMiddleware(&models.AdminUserCreateInput{}), controllers.CreateUser)
			admin.GET("/users/:id", controllers.GetUser)
			admin.PATCH("/users/:id", middleware.ValidateJSONMiddleware(&models.AdminUserUpdateInput{}), controllers.UpdateUser)
			admin.DELETE("/users/:id", controllers.DeleteUser)
			admin.POST("/trigger-reminders", func(c *gin.Context) {
				controllers.TriggerReminders(c, *cfg)
			})
			admin.POST("/trigger-purge", func(c *gin.Context) {
				controllers.TriggerPurge(c, *cfg)
			})
			// Rebuild the FTS search index (T11) — needed after bulk data
			// changes that bypassed the FTS triggers
			admin.POST("/search/rebuild", controllers.RebuildSearchIndexHandler)
		}
	}

	// CardDAV routes (optional, enabled via CARDDAV_ENABLED)
	if cfg.CardDAVEnabled {
		registerCardDAVRoutes(router, cfg, db)
	}

	// CalDAV routes (optional, enabled via CALDAV_ENABLED) — serve the CRM's
	// own Activities/LifeEvents out as an iCalendar collection (T12b).
	if cfg.CalDAVEnabled {
		registerCalDAVRoutes(router, db)
	}
}

// registerCalDAVRoutes sets up CalDAV endpoints for Interaction/LifeEvent
// sync (T12b, docs/fork-plan/tickets/35-T12b-caldav-serve.md). Authentication
// reuses the CardDAV BasicAuth path (password or a DAV-scoped API token), so
// both DAV surfaces share one credential story. Read-only: clients subscribe,
// they never write through this endpoint.
func registerCalDAVRoutes(router *gin.Engine, db *gorm.DB) {
	router.GET("/.well-known/caldav", caldav.WellKnownRedirect)

	handler := caldav.NewHandler(db)

	calDAVGroup := router.Group("/caldav")
	calDAVGroup.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	calDAVGroup.Use(middleware.CardDAVRateLimitMiddleware())
	calDAVGroup.Use(carddav.BasicAuthMiddleware())
	{
		ginHandler := handler.GinHandler()
		calDAVGroup.Any("/*path", ginHandler)
		calDAVGroup.Handle("PROPFIND", "/*path", ginHandler)
		calDAVGroup.Handle("REPORT", "/*path", ginHandler)
	}
}

// registerCardDAVRoutes sets up CardDAV endpoints for contact synchronization
func registerCardDAVRoutes(router *gin.Engine, cfg *config.Config, db *gorm.DB) {
	// Well-known discovery endpoint (no auth required for discovery)
	router.GET("/.well-known/carddav", carddav.WellKnownRedirect)

	handler := carddav.NewHandler(db, cfg.ProfilePhotoDir)

	cardDAVGroup := router.Group("/carddav")
	cardDAVGroup.Use(func(c *gin.Context) {
		c.Set("db", db)
		c.Next()
	})
	cardDAVGroup.Use(middleware.CardDAVRateLimitMiddleware())
	cardDAVGroup.Use(carddav.BasicAuthMiddleware())
	{
		ginHandler := handler.GinHandler()
		cardDAVGroup.Any("/*path", ginHandler)
		// WebDAV methods required for CardDAV
		cardDAVGroup.Handle("PROPFIND", "/*path", ginHandler)
		cardDAVGroup.Handle("REPORT", "/*path", ginHandler)
		cardDAVGroup.Handle("MKCOL", "/*path", ginHandler)
		cardDAVGroup.Handle("COPY", "/*path", ginHandler)
		cardDAVGroup.Handle("MOVE", "/*path", ginHandler)
	}
}
