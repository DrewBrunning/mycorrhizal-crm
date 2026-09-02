// Package integrations is the INT-01 (issue #464) classification matrix for
// every external system this backend talks to but does not control: CardDAV and
// CalDAV servers, Immich, Paperless, Seafile, generic WebDAV, outbound
// webhooks, ntfy/Gotify/Web Push, Resend and SMTP email, the OIDC provider, the
// Have I Been Pwned range API, and the GitHub releases update check.
//
// The point is that these are not one category. A CalDAV server being
// unreachable is a temporary degradation of a background sync; an OIDC provider
// being unreachable means nobody can log in; a Paperless instance being
// permanently gone means a stored reference never resolves again. Same word,
// three different correct behaviors. This package writes the classification
// down once, in typed form, so every client's retry/timeout/surfacing decision
// is made against a shared position rather than re-derived per author.
//
// # What is in here
//
//   - Registry() — one Integration per external system, classified along the
//     axes that determine correct failure handling (criticality, direction,
//     cadence, data authority, failure impact), plus its per-request timeout,
//     retry budget, SSRF posture, and the concrete required behavior for each
//     of the seven failure modes.
//
//   - Dispositions() — the transient-vs-permanent classification, keyed by
//     failure mode, not by integration. Per issue #464 the distinction is
//     per-error: a 503 is transient, a 401 after a revoked token is permanent
//     until a human acts. This is the single table #465 (INT-02), #466
//     (INT-03) and #467 (INT-04) test against — not a second opinion.
//
//   - Render() — projects the registry to docs/int-01-integration-classification-matrix.md.
//     The drift test integrations/matrix_test.go fails until the committed doc
//     matches, so a classification change is always a reviewable diff. DOC-03
//     (issue #488) cites that doc rather than restating it.
//
// # The structural check
//
// TestEveryOutboundClientIsClassified walks backend/services, finds every
// non-test file that opens an outbound network client, and fails if one is not
// claimed by a Registry() entry (or listed, with a reason, in
// nonIntegrationClients). Adding a new client without a matrix row breaks the
// build at the moment of introduction — the mechanism the ASVS verification
// report's §9 "outbound HTTP client" row asks for, scoped to services/.
package integrations
