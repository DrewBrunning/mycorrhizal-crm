package integrations

import "time"

// This file is the classification itself — one constructor per external system.
// Keep each row's prose specific: "handle the failure" is not a behavior, "the
// job records failure, releases its lock, leaves local contacts untouched, and
// advances sync-health to failing" is. The transient/permanent judgment is not
// repeated here; it lives once in Dispositions().

func carddavIntegration() Integration {
	return Integration{
		ID:   "carddav",
		Name: "CardDAV contact sync",
		What: "Two-way sync of a subscribed remote address book into contacts (full-overwrite reconcile, RFC 6352).",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Contacts exist and are fully editable without any subscription.",

		Direction:     DirectionBidirectional,
		DirectionNote: "Pull via REPORT sync-collection; push local changes back with PUT + If-Match.",

		Cadence:     CadenceInteractive,
		CadenceNote: "Runs when the user triggers a subscription sync from the UI — there is no scheduler entry for CardDAV today (calendar sync has one; contacts do not).",

		DataAuthority:     AuthorityShared,
		DataAuthorityNote: "reconcileContactSync intentionally lets the remote overwrite local edits on change (documented, test-pinned). The remote is a co-authority, not a backup.",

		FailureImpact:     ImpactSilentStaleness,
		FailureImpactNote: "A subscription that stops syncing looks identical to one where nothing changed; contacts silently age. sync_health (#390) is the surface that makes it visible.",

		Timeout:     60 * time.Second,
		TimeoutNote: "services.contactSyncRequestTimeout on the http.Client; a per-request context deadline bounds each REPORT/PUT.",

		RetryBudget: "No in-call retry. The user re-triggers; the next run re-fetches from the stored sync-token (or does a full refetch if the token was rejected). No partial state is committed on failure.",

		SSRF:     SSRFGuardedWhenEnabled,
		SSRFNote: "A custom RoundTripper wraps httputil.SafeDialContext; address filtering is applied only when CALDAV_BLOCK_PRIVATE_URLS is set (shared with CalDAV). Default off so LAN DAV servers work.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Sync run fails cleanly; sync_health goes to failing with the error; local contacts and the stored sync-token are untouched; the user sees the failure on the subscription.",
			FailureTimeout:               "The 60s deadline fires; the run ends as a failure (not a success); no half-applied batch — reconcile is all-or-nothing per run.",
			FailureAuthExpiry:            "401 → terminal auth-failure state on the subscription with a re-enter-credentials prompt; stop auto-attempts until the user acts (#467).",
			FailureAuthzRevoked:          "403 → same terminal state as auth expiry, worded as 'access to this address book was revoked'.",
			FailureMalformedResponse:     "Unparseable/truncated multistatus → run fails; nothing imported; a truncated body must never be read as 'these contacts were deleted remotely'.",
			FailureRateLimited:           "429/503 → back off honoring Retry-After; the run fails and is retried on the next user action / future scheduled run, not hammered.",
			FailureRemoteResourceDeleted: "A contact 404/410 on MultiGet is treated as 'removed on the server' only when the sync-collection delta says so; a bare fetch failure never deletes the local contact — it is left and reported.",
		},

		SourceFiles: []string{"services/contact_sync_service.go"},
		Verify:      "#465 (INT-02) failure matrix; contact_sync_hostile_input_test.go; sync_health advance test.",
	}
}

func caldavIntegration() Integration {
	return Integration{
		ID:   "caldav",
		Name: "CalDAV calendar sync",
		What: "Imports a subscribed remote calendar's events as activities/life events; optionally pushes local edits back (RFC 4791).",

		Criticality:     CriticalityOptional,
		CriticalityNote: "The calendar/timeline works without any subscription.",

		Direction:     DirectionBidirectional,
		DirectionNote: "Read-primary (calendar-query REPORT). Write-back is gated on CALDAV_TWO_WAY_ENABLED (default off); when on, local edits go out as PUT.",

		Cadence:     CadenceScheduled,
		CadenceNote: "Every CALDAV_SYNC_INTERVAL_HOURS (default 6h) via the calendar_sync job, plus on user trigger. Rate-limited by a job lock so a slow remote cannot overlap runs.",

		DataAuthority:     AuthorityShared,
		DataAuthorityNote: "The remote calendar is the source for imported events; two-way mode makes them co-authoritative for the fields it pushes (attendees excluded — they cannot converge).",

		FailureImpact:     ImpactSilentStaleness,
		FailureImpactNote: "A failing scheduled sync ages the imported timeline with nothing visibly wrong; sync_health (#390) surfaces it.",

		Timeout:     60 * time.Second,
		TimeoutNote: "services.calendarRequestTimeout on the http.Client; the push phase also wraps each PUT in a context.WithTimeout of the same value. The timeout is far below the 6h cadence so runs cannot pile up.",

		RetryBudget: "No in-call retry. The job releases its lock on failure; the next scheduled run (≤ interval) retries. Two-way push overwrites the remote unconditionally on the next run rather than tracking a retry queue.",

		SSRF:     SSRFGuardedWhenEnabled,
		SSRFNote: "Custom RoundTripper over httputil.SafeDialContext; filtering applied only when CALDAV_BLOCK_PRIVATE_URLS is set.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Job run records failure, releases the job lock, leaves imported activities and CalendarEventLink rows intact; sync_health → failing.",
			FailureTimeout:               "60s deadline fires; run ends as failure with the lock released; no partial import — event application is transactional per event and a mid-stream cut stops cleanly.",
			FailureAuthExpiry:            "401 → terminal auth-failure on the subscription; scheduled attempts stop bothering the user beyond the first clear signal (#467).",
			FailureAuthzRevoked:          "403 → terminal 'calendar access revoked' state, same handling as auth expiry.",
			FailureMalformedResponse:     "Unparseable iCalendar/multistatus → run fails; no events imported; a truncated feed must not be read as 'the remaining events were deleted'.",
			FailureRateLimited:           "429/503 → honor Retry-After, fail the run, retry next interval; never tighten the schedule in response.",
			FailureRemoteResourceDeleted: "An event gone from the remote is applied as a local deletion only when the calendar-query result set says it is absent; a fetch 404 for one object fails that object and is reported, it does not delete the linked activity.",
		},

		SourceFiles: []string{"services/calendar_sync_service.go"},
		Verify:      "#465 (INT-02); calendar_sync_hostile_input_test.go; calendar_two_way_test.go; cadence_job_lock_test.go.",
	}
}

func immichIntegration() Integration {
	return Integration{
		ID:   "immich",
		Name: "Immich photo enrichment",
		What: "Matches contacts to Immich people and pulls a face thumbnail as a profile photo.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Pure enrichment; every contact is complete without it.",

		Direction: DirectionOutbound,

		Cadence:     CadenceScheduled,
		CadenceNote: "Every IMMICH_SYNC_INTERVAL_HOURS (default 6h) via the immich_sync job, plus on user trigger from the Immich settings page.",

		DataAuthority:     AuthorityEnrichment,
		DataAuthorityNote: "Immich owns the person catalog and the thumbnails; we keep only a derived photo. Removing the integration leaves contacts intact minus Immich-sourced photos.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "New matches stop appearing and thumbnails stop refreshing; existing data is unaffected.",

		Timeout:     30 * time.Second,
		TimeoutNote: "services.immichRequestTimeout on the shared http.Client (also IdleConnTimeout 30s / TLSHandshakeTimeout 10s / ResponseHeaderTimeout 15s on the transport).",

		RetryBudget: "No in-call retry. A person that fails to sync is skipped and retried on the next scheduled run; nothing is deleted.",

		SSRF:     SSRFGuardedAlways,
		SSRFNote: "immichPrivateBlockingDialContext → httputil.SafeDialContext on the shared transport; every connection is re-resolved and pinned to a public address.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Sync job records failure and releases its lock; existing profile photos and match links are kept; the failure shows on the Immich settings page.",
			FailureTimeout:               "30s deadline fires; the run ends as a failure; a person mid-fetch is skipped, not half-written.",
			FailureAuthExpiry:            "401 (API key rotated) → terminal 'reconnect Immich' state with the key re-entry prompt; stop retrying until fixed (#467).",
			FailureAuthzRevoked:          "403 → same terminal state; message names a permissions problem on the Immich side.",
			FailureMalformedResponse:     "Unparseable/oversized body → that person is skipped with a logged warning; enrichment already applied is not rolled back and nothing new is written from a bad body.",
			FailureRateLimited:           "429/503 → back off honoring Retry-After; the run ends and resumes next interval.",
			FailureRemoteResourceDeleted: "A person/asset now 404 → drop the stale match link and, if the photo came from it, mark the photo for refresh; never delete the contact.",
		},

		SourceFiles: []string{"services/immich_client.go", "services/immich_service.go"},
		Verify:      "#465 (INT-02); immich_fault_injection_test.go; immich_fake_test.go.",
	}
}

func paperlessIntegration() Integration {
	return Integration{
		ID:   "paperless",
		Name: "Paperless-ngx document links",
		What: "Links contacts to documents in a Paperless-ngx instance and fetches titles/previews on demand.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Only the document-linking feature depends on it.",

		Direction: DirectionOutbound,

		Cadence:     CadenceInteractive,
		CadenceNote: "Called inline when the user browses, links, or opens a document from a contact.",

		DataAuthority:     AuthorityRemote,
		DataAuthorityNote: "Paperless is the system of record for the document; we store only a document ID. If the instance is gone the reference cannot be reconstructed.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "The user tries to open or attach a document and cannot; already-stored links remain but do not resolve.",

		Timeout:     30 * time.Second,
		TimeoutNote: "services.paperlessRequestTimeout on the http.Client; transport IdleConnTimeout 30s / TLSHandshakeTimeout 10s / ResponseHeaderTimeout 15s.",

		RetryBudget: "No retry — the call is inline in a user request. The request fails with a mapped error and the user retries.",

		SSRF:     SSRFGuardedAlways,
		SSRFNote: "paperlessPrivateBlockingDialContext → httputil.SafeDialContext, unconditionally.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "The request returns a mapped 'Paperless unreachable' error; no stored links are touched.",
			FailureTimeout:               "30s deadline → mapped timeout error to the caller; no partial link written.",
			FailureAuthExpiry:            "401 → mapped 'Paperless credentials invalid' error pointing at the settings page; stored links are kept for when it is fixed (#467).",
			FailureAuthzRevoked:          "403 → mapped 'Paperless access denied' error; same as auth expiry for stored links.",
			FailureMalformedResponse:     "Unparseable body → mapped 'unexpected Paperless response' error; nothing is inferred from a partial parse.",
			FailureRateLimited:           "429/503 → mapped 'Paperless busy, try again' error surfacing Retry-After to the user where present.",
			FailureRemoteResourceDeleted: "404 on a previously-valid document ID → surface 'this document no longer exists in Paperless' and offer to remove the dangling link; do not silently drop it.",
		},

		SourceFiles: []string{"services/paperless_client.go", "services/paperless_service.go"},
		Verify:      "#465 (INT-02); paperless_fake_test.go; controllers/paperless_real_db_test.go.",
	}
}

func seafileIntegration() Integration {
	return Integration{
		ID:   "seafile",
		Name: "Seafile file storage",
		What: "Stores and retrieves contact attachments in a Seafile library via its Web API.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Only attachment storage routed to Seafile depends on it.",

		Direction: DirectionOutbound,

		Cadence:     CadenceInteractive,
		CadenceNote: "Called inline on attachment upload, download, and listing.",

		DataAuthority:     AuthorityRemote,
		DataAuthorityNote: "Seafile holds the file bytes; we keep a repo/path reference. Losing the library loses the attachment content.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "Upload or download of a Seafile-backed attachment fails; references remain but do not resolve.",

		Timeout:     30 * time.Second,
		TimeoutNote: "services.seafileRequestTimeout on the http.Client; transport IdleConnTimeout 30s / TLSHandshakeTimeout 10s / ResponseHeaderTimeout 15s.",

		RetryBudget: "No retry — inline in a user request; the request fails with a mapped error and the user retries.",

		SSRF:     SSRFGuardedAlways,
		SSRFNote: "seafilePrivateBlockingDialContext → httputil.SafeDialContext, unconditionally.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Mapped 'Seafile unreachable' error to the caller; no reference rows changed.",
			FailureTimeout:               "30s deadline → mapped timeout error; a partial upload is not recorded as a stored attachment.",
			FailureAuthExpiry:            "401 (token expired) → mapped 'reconnect Seafile' error; existing references kept (#467).",
			FailureAuthzRevoked:          "403 → mapped 'Seafile access denied'; same as auth expiry for references.",
			FailureMalformedResponse:     "Unparseable body → mapped 'unexpected Seafile response'; no reference inferred from a partial parse.",
			FailureRateLimited:           "429/503 → mapped 'Seafile busy' error, Retry-After surfaced where present.",
			FailureRemoteResourceDeleted: "404 on a stored path → 'this file is no longer in Seafile', offer to remove the reference; never drop it silently.",
		},

		SourceFiles: []string{"services/seafile_client.go", "services/seafile_service.go"},
		Verify:      "#465 (INT-02); seafile_fake_test.go; controllers/seafile_real_db_test.go.",
	}
}

func webdavIntegration() Integration {
	return Integration{
		ID:   "webdav",
		Name: "Generic WebDAV storage",
		What: "Stores and retrieves contact attachments on any RFC 4918 WebDAV server.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Only attachment storage routed to WebDAV depends on it.",

		Direction: DirectionOutbound,

		Cadence:     CadenceInteractive,
		CadenceNote: "Called inline on attachment upload, download, and listing (PROPFIND/GET/PUT).",

		DataAuthority:     AuthorityRemote,
		DataAuthorityNote: "The WebDAV server holds the file bytes; we keep a URL/path reference.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "Upload or download of a WebDAV-backed attachment fails; references remain but do not resolve.",

		Timeout:     30 * time.Second,
		TimeoutNote: "services.webdavRequestTimeout on the http.Client; transport IdleConnTimeout 30s / TLSHandshakeTimeout 10s / ResponseHeaderTimeout 15s. XXE is blocked in the PROPFIND parser (webdav_client_xxe_test.go).",

		RetryBudget: "No retry — inline in a user request; fails with a mapped error and the user retries.",

		SSRF:     SSRFGuardedAlways,
		SSRFNote: "webdavPrivateBlockingDialContext → httputil.SafeDialContext, unconditionally.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Mapped 'WebDAV unreachable' error; no reference rows changed.",
			FailureTimeout:               "30s deadline → mapped timeout error; a partial PUT is not recorded as a stored attachment.",
			FailureAuthExpiry:            "401 → mapped 'WebDAV credentials invalid' error to the settings page; references kept (#467).",
			FailureAuthzRevoked:          "403 → mapped 'WebDAV access denied'; references kept.",
			FailureMalformedResponse:     "Unparseable/entity-expanded PROPFIND body → rejected by the hardened parser and surfaced as 'unexpected WebDAV response'; nothing inferred.",
			FailureRateLimited:           "429/503 → mapped 'WebDAV busy' error, Retry-After surfaced where present.",
			FailureRemoteResourceDeleted: "404 on a stored path → 'this file is no longer on the WebDAV server', offer to remove the reference.",
		},

		SourceFiles: []string{"services/webdav_client.go", "services/webdav_service.go"},
		Verify:      "#465 (INT-02); webdav_fake_test.go; webdav_client_xxe_test.go.",
	}
}

func webhooksIntegration() Integration {
	return Integration{
		ID:   "webhooks",
		Name: "Outbound webhook delivery",
		What: "POSTs a JSON envelope to operator-configured receiver URLs when app events fire.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Nothing in the app depends on a receiver being up.",

		Direction: DirectionOutbound,

		Cadence:     CadenceEventDriven,
		CadenceNote: "Fired from a tracked goroutine on the event; failed deliveries are retried by the webhook_retries job every 5 minutes.",

		DataAuthority:     AuthorityNone,
		DataAuthorityNote: "A transport for events we already hold; the receiver is not a data source.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "The receiver's downstream automation misses events; delivery rows record every attempt and outcome.",

		Timeout:     15 * time.Second,
		TimeoutNote: "http.Client.Timeout on both deliveryClient and guardedDeliveryClient; guarded client also caps redirects at 3.",

		RetryBudget: "maxDeliveryAttempts = 3. Delays: +5m then +15m (retryDelays). Retry state is a webhook_deliveries row (NextRetryAt), so it survives a restart. ProcessWebhookRetries runs under a job lock; a permanently-failing receiver stops after attempt 3 with the reason recorded — it does not retry forever.",

		SSRF:     SSRFGuardedWhenEnabled,
		SSRFNote: "clientFor(cfg) returns the SafeDialContext-guarded client only when WEBHOOK_BLOCK_PRIVATE_URLS is set (default off — self-hosted installs legitimately target LAN receivers). Guarding is in the dialer so redirect targets are checked too.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Dialer sentinel (ErrWebhookUnreachable / ErrWebhookPrivateAddress) stored in the delivery row's Error; attempt counted; NextRetryAt set per the schedule until attempt 3.",
			FailureTimeout:               "15s deadline → delivery row records the timeout, attempt counted, retry scheduled; the goroutine never blocks the event that triggered it.",
			FailureAuthExpiry:            "A receiver 401 is recorded and retried within the 3-attempt budget, then goes terminal (#467) — webhooks carry a signing secret, not a refreshable credential, so this is 'the receiver rejected us', surfaced on the webhook's status.",
			FailureAuthzRevoked:          "403 handled exactly like 401: recorded, bounded retries, then terminal with the last status shown.",
			FailureMalformedResponse:     "The response body is not used; any 2xx is success. A non-2xx status is recorded with the code and retried within budget.",
			FailureRateLimited:           "429/503 → recorded and retried on the fixed +5m/+15m schedule; Retry-After larger than the schedule is honored (INT-03, #466).",
			FailureRemoteResourceDeleted: "404 on the receiver URL is a non-2xx like any other: recorded, bounded retries, then terminal. A deleted receiver endpoint is an operator configuration problem surfaced on the webhook.",
		},

		SourceFiles: []string{"services/webhook_service.go"},
		Verify:      "#465 (INT-02), #466 (INT-03 retry safety); webhook_delivery_test.go; webhook_ssrf_test.go; webhook_service_job_lock_test.go.",
	}
}

func ntfyIntegration() Integration {
	return Integration{
		ID:   "ntfy",
		Name: "ntfy push notifications",
		What: "Posts reminder notifications to a user-configured ntfy topic.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "One notification channel of several; reminders still exist in-app.",

		Direction: DirectionOutbound,

		Cadence:     CadenceScheduled,
		CadenceNote: "From the daily reminder job (and test-notification button). Event-driven for any immediate notifications.",

		DataAuthority:     AuthorityNone,
		DataAuthorityNote: "Transport for reminders we own.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "A reminder push is missed; the reminder is not marked sent for that channel, so the next run retries it. notification_health (#422) surfaces the failure rate.",

		Timeout:     15 * time.Second,
		TimeoutNote: "clientFor(cfg) — the shared webhook delivery client (15s). No dedicated ntfy timeout.",

		RetryBudget: "No in-call retry. A failed send records a NotificationDelivery row with status 'failed'; the reminder stays due for that channel and the next scheduled run retries. (reminder, channel) keying de-duplicates so a retry cannot double-send a reminder that actually landed.",

		SSRF:     SSRFGuardedWhenEnabled,
		SSRFNote: "postNotificationJSON uses clientFor(cfg); the SafeDialContext guard applies when WEBHOOK_BLOCK_PRIVATE_URLS is set. URLs are validated as http(s) at save time (normalizeNotificationURL); ErrNotificationPrivateAddress is stored in the delivery row on a blocked address.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Send fails; 'failed' NotificationDelivery row with the error; reminder stays due; retried next run.",
			FailureTimeout:               "15s deadline → 'failed' row; reminder stays due; no partial send that could be re-sent as a duplicate.",
			FailureAuthExpiry:            "ntfy topics are typically unauthenticated; a 401 (protected topic, bad token) is recorded as a failure and, on repeated failures, surfaced via notification_health as a channel that needs attention (#467).",
			FailureAuthzRevoked:          "403 handled as 401.",
			FailureMalformedResponse:     "The ntfy response body is not consumed for meaning; a non-2xx status is recorded as a failed delivery with the code.",
			FailureRateLimited:           "429/503 → failed delivery row; the reminder is retried on the next scheduled run rather than immediately, which is itself the backoff. Retry-After is respected where the run would otherwise retry sooner.",
			FailureRemoteResourceDeleted: "Not applicable in the usual sense (a topic is created on first publish); a 404 is recorded as a failed delivery and, if persistent, flagged as a misconfigured channel.",
		},

		SourceFiles: []string{"services/notification_service.go"},
		Verify:      "#465 (INT-02); notification_service_test.go; notification_health_test.go.",
	}
}

func gotifyIntegration() Integration {
	in := ntfyIntegration()
	in.ID = "gotify"
	in.Name = "Gotify push notifications"
	in.What = "Posts reminder notifications to a user-configured Gotify server (X-Gotify-Key app token)."
	in.SSRFNote = "sendGotifyMessage → postNotificationJSON → clientFor(cfg); SafeDialContext guard applies when WEBHOOK_BLOCK_PRIVATE_URLS is set. The app token is decrypted from NotificationConfig at send time."
	in.Behavior = map[FailureMode]string{
		FailureUnreachableHost:       "Send fails; 'failed' NotificationDelivery row; reminder stays due; retried next run.",
		FailureTimeout:               "15s deadline → 'failed' row; reminder stays due; no re-sendable partial.",
		FailureAuthExpiry:            "Gotify uses an app token; a 401 means the token was revoked/rotated — recorded as failed and surfaced through notification_health as a channel needing re-configuration (#467).",
		FailureAuthzRevoked:          "403 handled as 401.",
		FailureMalformedResponse:     "Response body not consumed for meaning; non-2xx recorded with the status code.",
		FailureRateLimited:           "429/503 → failed delivery row; retried on the next scheduled run; Retry-After respected where it would otherwise retry sooner.",
		FailureRemoteResourceDeleted: "404 (application deleted on the Gotify side) → recorded as failed; if persistent, flagged as a misconfigured channel.",
	}
	in.Verify = "#465 (INT-02); notification_service_test.go."
	return in
}

func webPushIntegration() Integration {
	return Integration{
		ID:   "webpush",
		Name: "Web Push (VAPID) browser notifications",
		What: "Delivers reminder notifications to browser push endpoints (RFC 8291/8292) for registered PushSubscriptions.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "One notification channel; reminders still exist in-app.",

		Direction: DirectionOutbound,

		Cadence:     CadenceScheduled,
		CadenceNote: "From the daily reminder job; event-driven for immediate notifications.",

		DataAuthority:     AuthorityNone,
		DataAuthorityNote: "Transport for reminders we own. The push service (FCM/Mozilla/etc.) is chosen by the subscriber's browser, not configured here.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "A push is missed; the reminder stays due for the channel and retries next run. A dead subscription is pruned so it stops failing.",

		Timeout:     15 * time.Second,
		TimeoutNote: "webpush.Options.HTTPClient = clientFor(cfg) (15s). RecordSize is clamped to webpush.MaxRecordSize.",

		RetryBudget: "No in-call retry. Failed send → 'failed' delivery row, reminder stays due, next run retries. A 404/410 from the push service permanently removes the PushSubscription row (it will never work again) — the one place a notification channel self-heals by dropping state.",

		SSRF:     SSRFGuardedWhenEnabled,
		SSRFNote: "clientFor(cfg); SafeDialContext guard applies when WEBHOOK_BLOCK_PRIVATE_URLS is set. The endpoint URL comes from the browser's PushManager, not user text input, but is still an arbitrary URL so the guard matters.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Send fails; 'failed' delivery row; reminder stays due; retried next run; the subscription is kept (could be transient).",
			FailureTimeout:               "15s deadline → 'failed' row; reminder stays due; subscription kept.",
			FailureAuthExpiry:            "A 401/403 from the push service means the VAPID keys are wrong for this endpoint — recorded as failed; if it affects all subscriptions it is a server-config problem surfaced via notification_health (#467), not a per-user re-auth.",
			FailureAuthzRevoked:          "403 handled as 401 (usually a VAPID audience/key mismatch).",
			FailureMalformedResponse:     "The push service response body is not meaningful; the HTTP status drives handling. An unexpected non-2xx that is not 404/410 is a retryable failure.",
			FailureRateLimited:           "429/503 → 'failed' row; retried next run (the schedule is the backoff); Retry-After respected where present.",
			FailureRemoteResourceDeleted: "404/410 → the subscription is permanently dead: delete the PushSubscription row so it stops being tried, and record the delivery as failed with that reason.",
		},

		SourceFiles: []string{"services/notification_service.go"},
		Verify:      "#465 (INT-02); notification_service_test.go; push_subscription_test.go.",
	}
}

func emailResendIntegration() Integration {
	return Integration{
		ID:   "email-resend",
		Name: "Transactional email — Resend API",
		What: "Sends transactional email (password reset, invitations, reminder digests) through the Resend HTTP API.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Optional as a channel, but note: with no email channel configured, password-reset and invitation emails cannot be delivered at all. SendEmail is a no-op when unconfigured.",

		Direction: DirectionOutbound,

		Cadence:     CadenceEventDriven,
		CadenceNote: "On password reset / invite / verification; also from the daily reminder job for email digests.",

		DataAuthority:     AuthorityNone,
		DataAuthorityNote: "Transport only.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "A user cannot complete a password reset or accept an invite until email works or the operator uses an alternate path.",

		Timeout:     0,
		TimeoutNote: "No timeout is wired on our side; the call inherits resend-go's default http.Client, which sets Timeout: 1m (resend.go). So it is bounded, but at 60s and not by a value this project chose. Passing an explicit client via resend.NewCustomClient is INT-02 (#465) work — recorded so the bound is a deliberate number, not a library default.",

		RetryBudget: "No retry. Best-effort: SendEmail tries every configured channel (Resend and/or SMTP) and returns success if at least one succeeds, a combined error only if all fail. The caller decides what a total failure means (password reset surfaces it; a reminder digest logs it).",

		SSRF:     SSRFFixedEndpoint,
		SSRFNote: "Destination is Resend's fixed API host (api.resend.com); no user-supplied URL, so there is no SSRF surface. The API key is operator config.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "SDK returns an error; logged as 'Failed to send email via Resend'; if SMTP is also configured it is tried; if not, SendEmail returns the combined error to the caller.",
			FailureTimeout:               "Bounded by resend-go's default 60s client timeout (see the timeout note). On error, same fallback-to-SMTP-or-combined-error path.",
			FailureAuthExpiry:            "401 (API key revoked) → error surfaced to the caller / logged; a persistent 401 is an operator problem — the fix is a new key in config, there is no runtime re-auth.",
			FailureAuthzRevoked:          "403 (domain not verified, sending disabled) handled as 401: surfaced/logged, operator must fix the Resend account.",
			FailureMalformedResponse:     "Handled by the SDK; a decode error becomes a send error and the fallback/reporting path runs.",
			FailureRateLimited:           "429 → send error this attempt; falls back to SMTP if configured. There is no queue, so a rate-limited reminder digest is simply logged as failed for that run.",
			FailureRemoteResourceDeleted: "Not applicable — there is no addressable remote resource, only the send endpoint.",
		},

		SourceFiles: []string{"services/mailer.go"},
		Verify:      "#465 (INT-02); mailer_test.go.",
	}
}

func emailSMTPIntegration() Integration {
	return Integration{
		ID:   "email-smtp",
		Name: "Transactional email — SMTP",
		What: "Sends the same transactional email through an operator-configured SMTP server (STARTTLS or implicit TLS).",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Same as Resend: optional as a channel, but the only email path if Resend is not configured.",

		Direction: DirectionOutbound,

		Cadence:     CadenceEventDriven,
		CadenceNote: "Same triggers as the Resend path.",

		DataAuthority:     AuthorityNone,
		DataAuthorityNote: "Transport only.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "Same as Resend — password reset / invite delivery is blocked until it works.",

		Timeout:     0,
		TimeoutNote: "GAP: net/smtp and tls.Dial are used with no deadline set, so a hung SMTP server is bounded only by the OS TCP stack. This runs off the request path (reminder job / async), but a bounded dial+send is INT-02 (#465) work. Recorded as a known gap.",

		RetryBudget: "No retry. Part of the same best-effort multi-channel SendEmail: SMTP failure alone is tolerated if Resend succeeded; if SMTP is the only channel, the caller gets the error.",

		SSRF:     SSRFFixedEndpoint,
		SSRFNote: "SMTPHost is operator configuration read from env, not a per-request user value, and net/smtp is not routed through SafeDialContext. Treated as a fixed operator endpoint rather than an SSRF surface; if SMTP host ever becomes user-supplied this row must change.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Dial error → logged 'Failed to send email via SMTP'; combined-error path if SMTP is the only channel.",
			FailureTimeout:               "No explicit deadline (see the gap); an OS-level connection failure surfaces as a dial error and takes the same path.",
			FailureAuthExpiry:            "SMTP auth failure (535) → send error, surfaced/logged; operator must fix credentials in config.",
			FailureAuthzRevoked:          "Relay-denied / not-permitted (550/554) → send error, surfaced/logged; operator/relay configuration problem.",
			FailureMalformedResponse:     "An unexpected SMTP reply code aborts that step with a wrapped error ('smtp mail from', 'smtp data', …); nothing partial is considered sent.",
			FailureRateLimited:           "Greylisting / 421 / 4xx throttle → send error for this attempt; no queue, so a throttled reminder digest is logged as failed for the run.",
			FailureRemoteResourceDeleted: "Not applicable.",
		},

		SourceFiles: []string{"services/mailer.go"},
		Verify:      "#465 (INT-02); mailer_test.go.",
	}
}

func oidcIntegration() Integration {
	return Integration{
		ID:   "oidc",
		Name: "OIDC single sign-on provider",
		What: "Authenticates users via an external OpenID Connect provider (discovery, authorization-code + PKCE, ID-token verification, optional UserInfo).",

		Criticality:     CriticalityRequired,
		CriticalityNote: "When OIDC is the login method, an unreachable provider / failed discovery / unreachable JWKS or UserInfo means no OIDC account can authenticate. Local password accounts (if any) are unaffected.",

		Direction:     DirectionBidirectional,
		DirectionNote: "Inbound: the browser redirect back with the code. Outbound: discovery at startup, token exchange, JWKS fetch, UserInfo.",

		Cadence:     CadenceInteractive,
		CadenceNote: "Token exchange and UserInfo happen inline in the login callback. Discovery runs once at startup (InitOIDCProvider) and is cached for the process lifetime.",

		DataAuthority:     AuthorityRemote,
		DataAuthorityNote: "The provider is authoritative for identity (subject, email, name). We store the subject/provider mapping; we cannot mint an account without a successful exchange.",

		FailureImpact:     ImpactBlockedWorkflow,
		FailureImpactNote: "The user reaches the login callback and cannot get a session. Immediate and obvious (unlike the sync cases), so no silent-staleness risk.",

		Timeout:     0,
		TimeoutNote: "No dedicated client timeout: go-oidc / golang.org/x/oauth2 use their default transports. The calls are bounded by the login request's context deadline. A per-call timeout via oidc.ClientContext is INT-02 (#465) work.",

		RetryBudget: "No retry. A failed exchange fails the login; the user retries by starting the flow again. Discovery is not re-fetched after startup, so a provider that changes its endpoints needs a restart.",

		SSRF:     SSRFUnguarded,
		SSRFNote: "GAP: the provider URL is operator configuration, and discovery/JWKS/UserInfo/token calls run on library default transports — NOT httputil.SafeDialContext. Lower risk than the per-user-URL integrations (the value is set once by the operator, not per request), but a redirect from the provider host to an internal address is not blocked. Recorded as a known gap; a guarded oidc.ClientContext is the fix.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Startup: InitOIDCProvider returns an error and OIDC is not enabled (login falls back to whatever else is configured). Runtime: the token/UserInfo call fails, the callback returns a login error, no partial session is created.",
			FailureTimeout:               "Bounded by the request context; on deadline the callback fails with a login error. No session, no partial user row.",
			FailureAuthExpiry:            "Not applicable to a client credential (the client secret is operator config, a 401 there is a setup error surfaced at the callback). For the end user, an expired provider session just means they re-authenticate at the provider.",
			FailureAuthzRevoked:          "Provider returns access_denied / the user's account is disabled provider-side → the callback surfaces 'sign-in was refused by the provider'; no local account is created or unlocked.",
			FailureMalformedResponse:     "ID-token signature/claims verification failure, UserInfo sub mismatch (OIDC Core 5.3.2), or an unparseable discovery doc → the flow aborts with a specific error; a claim is never trusted from a body that failed verification.",
			FailureRateLimited:           "429/503 from the token or UserInfo endpoint → the login fails for that attempt; the user retries. There is no background retry to rate-limit.",
			FailureRemoteResourceDeleted: "The OIDC client/registration deleted provider-side → discovery or exchange fails; surfaced as a provider configuration error for the operator, not a per-user problem.",
		},

		SourceFiles: []string{"services/oidc_service.go"},
		Verify:      "#465 (INT-02); oidc_service_test.go; oidc_attack_matrix_test.go; oidc_userinfo_test.go.",
	}
}

func hibpIntegration() Integration {
	return Integration{
		ID:   "hibp",
		Name: "Have I Been Pwned — breached-password check",
		What: "Checks a candidate password against HIBP's k-anonymity range API during registration / password change / reset (only a 5-char SHA-1 prefix ever leaves the process).",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Advisory only, and off unless HIBP checking is enabled (ASVS 2.1.7, issue #376).",

		Direction: DirectionOutbound,

		Cadence:     CadenceInteractive,
		CadenceNote: "Inline in the registration / change-password / reset-confirm request.",

		DataAuthority:     AuthorityEnrichment,
		DataAuthorityNote: "Adds a 'this password is known-breached' signal; owns nothing of ours.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "Fails open by design: any error means the breach check is skipped for that request and the password is allowed. A third-party outage must never block registering or changing a password on a self-hosted app.",

		Timeout:     5 * time.Second,
		TimeoutNote: "hibpClient = &http.Client{Timeout: 5s} — deliberately shorter than the webhook client because it sits inline in an auth request and must fail fast.",

		RetryBudget: "No retry. On any error the function logs a warning and returns (false, err); the caller treats that as 'not breached, HIBP unavailable' and proceeds. The next password operation re-checks.",

		SSRF:     SSRFFixedEndpoint,
		SSRFNote: "Destination is HIBP's fixed host (hibpAPIBaseURL — a var only so tests can point at httptest; there is no env var or config field for it). No user-supplied URL, so no SSRF surface: hibpClient is a bare http.Client{Timeout: 5s} with no custom dialer. If HIBP checking ever accepts a self-hosted mirror URL this row must move to guarded.",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Warning logged ('allowing password, fail open'); returns (false, err); the auth request proceeds normally.",
			FailureTimeout:               "5s deadline fires → same fail-open path; the auth request is not held up.",
			FailureAuthExpiry:            "Not applicable — the range API is unauthenticated.",
			FailureAuthzRevoked:          "Not applicable — unauthenticated.",
			FailureMalformedResponse:     "A body that does not parse as HIBP range lines → treated as 'no match found' / unavailable; fail open; never blocks on a garbled response.",
			FailureRateLimited:           "429 → fail open for that request (the padding header is already sent per HIBP's guidance); no retry storm because there is no retry.",
			FailureRemoteResourceDeleted: "Not applicable — every prefix is a valid range query.",
		},

		SourceFiles: []string{"services/hibp_service.go"},
		Verify:      "#465 (INT-02); hibp_service_test.go.",
	}
}

func updateCheckIntegration() Integration {
	return Integration{
		ID:   "update-check",
		Name: "GitHub releases — update-availability check",
		What: "Asks the GitHub releases API whether a newer release of this project exists, for the admin system-status page.",

		Criticality:     CriticalityOptional,
		CriticalityNote: "Off by default (UPDATE_CHECK_ENABLED). Strictly informational when on — nothing blocks or errors on it.",

		Direction: DirectionOutbound,

		Cadence:     CadenceInteractive,
		CadenceNote: "Computed when an admin loads system-status; the result is memoized for updateCheckCacheTTL (6h). Errors are never cached.",

		DataAuthority:     AuthorityEnrichment,
		DataAuthorityNote: "Adds a 'newer version available' string; owns nothing.",

		FailureImpact:     ImpactDegradedFeature,
		FailureImpactNote: "The system-status page omits the update line or shows it as unavailable. No other surface is affected.",

		Timeout:     3 * time.Second,
		TimeoutNote: "updateCheckTimeout = 3s, applied as both the client timeout and a context deadline, so an injected test transport is still bounded. Fails fast because it feeds an admin snapshot.",

		RetryBudget: "No retry. A failed lookup is not cached and is retried on the next system-status load.",

		SSRF:     SSRFGuardedAlways,
		SSRFNote: "newUpdateCheckClient wires httputil.SafeDialContext; the host re-resolves and pins a public address at dial time (ASVS 5.2.6). Destination is a fixed GitHub API URL (a var only for tests).",

		Behavior: map[FailureMode]string{
			FailureUnreachableHost:       "Lookup returns an error; not cached; the status page shows the update state as unavailable; retried next load.",
			FailureTimeout:               "3s deadline → same as unreachable; the admin snapshot is never held open on it.",
			FailureAuthExpiry:            "Not applicable — the call is unauthenticated (public releases endpoint).",
			FailureAuthzRevoked:          "Not applicable.",
			FailureMalformedResponse:     "An unparseable release body → treated as 'could not determine'; never surfaced as a spurious 'update available'.",
			FailureRateLimited:           "GitHub 403/429 with rate-limit headers → treated as unavailable for this load; the 6h memoization of the last success already keeps call volume trivially low.",
			FailureRemoteResourceDeleted: "A 404 for the releases endpoint (repo moved/renamed) → treated as 'could not determine'; it is a build-time constant, so this would be a project bug, not a runtime condition.",
		},

		SourceFiles: []string{"services/update_check.go"},
		Verify:      "#465 (INT-02); update_check_test.go.",
	}
}
