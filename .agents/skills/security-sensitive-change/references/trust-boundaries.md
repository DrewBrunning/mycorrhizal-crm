# Trust boundaries by surface

Use this to find the row a change must update and the negative test it needs.
Do not copy the checklist; the ASVS/MASVS files are canonical.

| Surface | Attacker-controlled input | Required check |
|---|---|---|
| Auth/session | credentials, cookies, JWT, reset/verify tokens | login/lockout/expiry/rotation negative tests; `middleware/` clock boundaries |
| Every user-scoped route | path/body IDs, query filters | ownership scoped by `user_id` (or `Contact.VCardUID`); `backend/routes/ownership_matrix_test.go` |
| Uploads | filename, MIME, bytes, remote URL | size/type allowlist, path containment (`backend/internal/fsguard`), no `os/exec` |
| Imports (vCard/JSContact/CSV/iCal) | untrusted structured data | parser bounds, formula neutralization on CSV export, no template execution |
| Exports/shares/sync | the data leaving the instance | sensitivity filtered in the query; only the three documented full-fidelity exceptions |
| Outbound integrations | target URL, redirects, response body | SSRF-guarded dialer, size/time limits, no secret reflection |
| Templates | user text rendered into HTML/email | embedded FS, contextual escaping, no user-controlled template source |
| Secrets/config | env and config file | no secrets in logs/exports; `.env.example` names only |
| Database | SQL/path input | parameterized SQL; no string-built queries |
| Dependencies/build | upstream packages, release artifacts | pinned toolchain, `depexceptions`, `gennotices`, provenance |

## Which ASVS row

- Authentication/session: V2/V3.
- Access control and BOLA/IDOR: V4 and the API Security Top 10.
- Validation, encoding, uploads: V5/V12.
- Cryptography and key handling: V6.
- Logging/errors: V7.
- Data protection, retention, PII: V8 and `docs/security/data-retention-lifecycle.md`.
- Transport, headers, SSRF: V9/V14.
- Business logic and abuse limits: V11.
- Configuration/secrets: V14.
- Malicious code/`os/exec`: V10 and P12.

## Threat-model updates

Update `docs/security/threat-model.md` when the change adds or moves a trust
boundary (new external service, new client-side store, new privileged path).
Record why the boundary exists and what crosses it; link the ADR.

## Tests

Negative tests are the proof, not the checklist edit. They belong beside the
handler/service they exercise; a security test that needs a CI-only tool uses
`internal/citest.SkipOrRequire`, never skip-as-pass.

Canonical: `docs/security/asvs-l2.md`, `docs/security/masvs-l1.md`,
`docs/security/threat-model.md`, `docs/security/asvs-l2-verification-report.md`.
