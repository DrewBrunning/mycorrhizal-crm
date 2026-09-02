package integrations

import "time"

// Criticality answers "does the app work without this integration?".
type Criticality string

const (
	// CriticalityRequired: while the integration is configured and down, a
	// whole workflow is unavailable and there is no local fallback. Today only
	// OIDC — an unreachable provider means no OIDC account can authenticate.
	CriticalityRequired Criticality = "required-when-enabled"
	// CriticalityOptional: the app runs without it; losing it degrades or
	// blocks one feature, never the whole product.
	CriticalityOptional Criticality = "optional"
)

// Direction records who initiates the network calls.
type Direction string

const (
	DirectionOutbound      Direction = "outbound"      // we call them
	DirectionInbound       Direction = "inbound"       // they call us
	DirectionBidirectional Direction = "bidirectional" // both
)

// Cadence records when the calls happen — this determines whether a failure is
// seen by a user immediately or only by a background job.
type Cadence string

const (
	CadenceInteractive Cadence = "interactive"  // inline in a user request
	CadenceScheduled   Cadence = "scheduled"    // a recurring background job
	CadenceEventDriven Cadence = "event-driven" // fired by an app event
)

// DataAuthority records whether the remote holds data we cannot reconstruct.
type DataAuthority string

const (
	// AuthorityRemote: the remote is the system of record for data we only
	// hold a reference to. Losing the remote loses the data.
	AuthorityRemote DataAuthority = "remote-authoritative"
	// AuthorityShared: the remote and this app are co-authorities over the
	// same records (two-way or full-overwrite sync).
	AuthorityShared DataAuthority = "shared"
	// AuthorityEnrichment: the remote adds data on top of records we own. If
	// it goes away we keep everything we authored, minus the enrichment.
	AuthorityEnrichment DataAuthority = "enrichment"
	// AuthorityNone: no data authority — the integration is a transport for
	// events or messages we generate.
	AuthorityNone DataAuthority = "none"
)

// FailureImpact is what actually goes wrong when the integration is down.
type FailureImpact string

const (
	// ImpactDegradedFeature: one feature is worse or absent; the user can tell
	// something is missing.
	ImpactDegradedFeature FailureImpact = "degraded-feature"
	// ImpactBlockedWorkflow: the user tries to do a specific thing and cannot.
	ImpactBlockedWorkflow FailureImpact = "blocked-workflow"
	// ImpactSilentStaleness: nothing looks wrong. A background sync stopped and
	// the data just ages. The most dangerous case (issues #465–#467, #390).
	ImpactSilentStaleness FailureImpact = "silent-staleness"
)

// SSRFPosture records how the integration's transport is constrained, since
// every one of these takes a user- or operator-supplied URL (issue #464 point
// 5). The guard lives in the dialer (httputil.SafeDialContext): it re-resolves
// the host and pins a public address at dial time, so redirect-to-internal and
// DNS-rebinding cannot reach loopback/private ranges.
type SSRFPosture string

const (
	// SSRFGuardedAlways: every connection goes through httputil.SafeDialContext,
	// unconditionally.
	SSRFGuardedAlways SSRFPosture = "guarded-always"
	// SSRFGuardedWhenEnabled: guarded only when the operator opts in
	// (WEBHOOK_BLOCK_PRIVATE_URLS / CALDAV_BLOCK_PRIVATE_URLS). Off by default
	// because self-hosted installs legitimately point at LAN services.
	SSRFGuardedWhenEnabled SSRFPosture = "guarded-when-enabled"
	// SSRFFixedEndpoint: no user-supplied URL — the destination is a compiled-in
	// vendor host, so there is no SSRF surface to guard.
	SSRFFixedEndpoint SSRFPosture = "fixed-endpoint"
	// SSRFUnguarded: a user/operator URL reaches a transport that is NOT
	// SafeDialContext-guarded. A known gap, recorded here rather than hidden.
	SSRFUnguarded SSRFPosture = "unguarded"
)

// Persistence is the transient-vs-permanent axis. This is the distinction
// #465–#467 depend on: a transient failure is retried quietly, a permanent one
// is surfaced to a human and not retried.
type Persistence string

const (
	// Transient: expected to clear on its own. Retry with backoff.
	Transient Persistence = "transient"
	// PermanentUntilHuman: will not clear until a person re-authenticates,
	// fixes configuration, or restores a resource. Stop retrying; surface it.
	PermanentUntilHuman Persistence = "permanent-until-human"
)

// FailureMode is one of the seven failure conditions every integration must
// have a stated behavior for (issue #464 point 2).
type FailureMode string

const (
	FailureUnreachableHost       FailureMode = "unreachable-host"        // DNS failure, connection refused, no route
	FailureTimeout               FailureMode = "timeout"                 // connect or read timeout, hung remote
	FailureAuthExpiry            FailureMode = "auth-expiry"             // 401 — credentials that worked yesterday do not today
	FailureAuthzRevoked          FailureMode = "authz-revoked"           // 403 — authenticated but permission withdrawn
	FailureMalformedResponse     FailureMode = "malformed-response"      // unparseable, truncated, or larger than expected
	FailureRateLimited           FailureMode = "rate-limited"            // 429 or 503, with or without Retry-After
	FailureRemoteResourceDeleted FailureMode = "remote-resource-deleted" // 404/410 on a resource that previously resolved
)

// FailureModes is the canonical ordered list — the order rows appear in the
// generated doc, and the set every integration is checked against.
var FailureModes = []FailureMode{
	FailureUnreachableHost,
	FailureTimeout,
	FailureAuthExpiry,
	FailureAuthzRevoked,
	FailureMalformedResponse,
	FailureRateLimited,
	FailureRemoteResourceDeleted,
}

// Disposition is the per-error-mode classification. It is keyed by FailureMode
// alone — not by integration — because the distinction is a property of the
// error, not of who returned it (issue #464 point 4).
type Disposition struct {
	Persistence     Persistence
	Retryable       bool   // is a retry ever safe / worthwhile for this mode?
	HonorRetryAfter bool   // must a Retry-After / rate-limit hint be respected?
	Rationale       string // one line: why this classification
}

// Dispositions is the single transient/permanent table. #465 (INT-02), #466
// (INT-03) and #467 (INT-04) assert against this map; they must not carry their
// own copy of the judgment.
func Dispositions() map[FailureMode]Disposition {
	return map[FailureMode]Disposition{
		FailureUnreachableHost: {
			Persistence: Transient, Retryable: true,
			Rationale: "DNS/refused/no-route is almost always the remote being down or a transient network fault; retry with backoff, escalate to a permanent state only after a bounded run of consecutive failures (#467).",
		},
		FailureTimeout: {
			Persistence: Transient, Retryable: true,
			Rationale: "A hung or slow remote clears on its own; the request must be bounded (see each integration's timeout) so a retry is possible rather than an indefinite block.",
		},
		FailureAuthExpiry: {
			Persistence: PermanentUntilHuman, Retryable: false,
			Rationale: "A 401 from previously-valid credentials will not become a 200 by repetition — a token expired or a password changed. Stop, and surface an actionable re-auth prompt (#467).",
		},
		FailureAuthzRevoked: {
			Persistence: PermanentUntilHuman, Retryable: false,
			Rationale: "A 403 means the account is known but the grant was withdrawn (scope removed, share revoked). Retrying is waste; a human must restore access.",
		},
		FailureMalformedResponse: {
			Persistence: Transient, Retryable: true,
			Rationale: "A truncated/garbled body is usually a mid-flight cut or a transient proxy error; retry a bounded number of times, but never partially apply what did parse.",
		},
		FailureRateLimited: {
			Persistence: Transient, Retryable: true, HonorRetryAfter: true,
			Rationale: "429/503 is the remote asking for less traffic, not a failure. Back off for at least the Retry-After interval; ignoring it turns a rate limit into a ban.",
		},
		FailureRemoteResourceDeleted: {
			Persistence: PermanentUntilHuman, Retryable: false,
			Rationale: "A 404/410 on something that used to resolve means the remote deleted it. It will not come back by retrying; the local reference must be reconciled (dropped or re-linked), never used to delete local data.",
		},
	}
}

// Integration is one external system, classified.
type Integration struct {
	ID   string // stable kebab-case slug; the doc and tests key on it
	Name string // human name for the doc heading
	What string // one sentence: what it does for the product

	Criticality     Criticality
	CriticalityNote string

	Direction     Direction
	DirectionNote string

	Cadence     Cadence
	CadenceNote string

	DataAuthority     DataAuthority
	DataAuthorityNote string

	FailureImpact     FailureImpact
	FailureImpactNote string

	// Timeout is the per-request deadline. Zero means "no explicit timeout is
	// wired" — a real gap, in which case TimeoutNote must explain what bounds
	// the call instead.
	Timeout     time.Duration
	TimeoutNote string

	// RetryBudget is the outbound retry policy in words: attempts, backoff,
	// where the state lives, and whether it survives a restart (#466).
	RetryBudget string

	SSRF     SSRFPosture
	SSRFNote string

	// Behavior is the concrete required action for each of the seven failure
	// modes — what "handle it correctly" means for this specific integration.
	// The transient/permanent judgment stays in Dispositions(); this is the
	// operational consequence. Every FailureMode must be present.
	Behavior map[FailureMode]string

	// SourceFiles are the backend/services files that implement the outbound
	// client, relative to backend/. The structural test asserts each exists and
	// that every outbound-client file in services/ is claimed by some entry.
	SourceFiles []string

	// Verify points at the ticket(s) that exercise this row's failure behavior.
	Verify string
}

// Registry is the classified list, in doc order: sync first (the silent-
// staleness cases), then enrichment/storage, then messaging, then auth and the
// advisory lookups.
func Registry() []Integration {
	return []Integration{
		carddavIntegration(),
		caldavIntegration(),
		immichIntegration(),
		paperlessIntegration(),
		seafileIntegration(),
		webdavIntegration(),
		webhooksIntegration(),
		ntfyIntegration(),
		gotifyIntegration(),
		webPushIntegration(),
		emailResendIntegration(),
		emailSMTPIntegration(),
		oidcIntegration(),
		hibpIntegration(),
		updateCheckIntegration(),
	}
}

// ByID returns the integration with the given slug, and whether it was found.
func ByID(id string) (Integration, bool) {
	for _, in := range Registry() {
		if in.ID == id {
			return in, true
		}
	}
	return Integration{}, false
}

// nonIntegrationClients lists backend/services files that trip the outbound-
// client signal but are NOT their own integration — they reuse another
// integration's client or probe on its behalf. Each needs a reason. The
// structural test uses this as the allowlist and also fails on a dead entry
// (a file here that no longer trips the signal), so it cannot rot.
var nonIntegrationClients = map[string]string{
	"diagnostics.go": "issue #423 admin health sweep — time-bounded reachability probes that reuse the per-integration clients (integrationProbeTimeout), not a new external system",
}
