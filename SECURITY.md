# Security Policy

Mycorrhizal CRM is self-hosted software that stores real personal data (contacts,
notes, activity history). We take vulnerability reports seriously and ask that
they be reported privately rather than as a public issue.

## Reporting a Vulnerability

Please report suspected vulnerabilities using GitHub's private
[Security Advisory](https://github.com/DrewBrunning/mycorrhizal-crm/security/advisories/new)
flow rather than a public issue, pull request, or discussion. This lets us
assess and fix the problem before it's disclosed publicly.

Include, if known:

- The affected component (backend, frontend, Android app, or a specific
  integration such as CardDAV/CalDAV sync)
- Steps to reproduce, or a proof of concept
- The potential impact (e.g. data exposure, authentication bypass, IDOR)
- The version/tag or commit you tested against

We'll acknowledge new reports within 5 business days and aim to provide a
fix or mitigation timeline once the report is triaged. Coordinated
disclosure is welcome — let us know if you have a timeline in mind.

## If You Operate an Instance and Suspect a Compromise

If you run your own instance and think it (or a credential, key, or backup)
has been exposed, the operator runbook is
[`docs/security/incident-response.md`](docs/security/incident-response.md): a
step-by-step for contain → assess → rotate → recover → notify, a per-secret
rotation reference (`JWT_SECRET_KEY`, the at-rest key, API tokens, account
passwords, 2FA, the OIDC client secret), and scenario playbooks (compromised
account, leaked key, leaked backup, compromised host). Preserve evidence
first — the runbook's capture step comes before any rotation or restore.

## Supported Versions

Mycorrhizal CRM is on the `1.x` line and supports a deliberately small,
predictable set of release lines (in force since `1.0.0`, issue #956). Only
supported tags receive security fixes; users should run a supported release to
receive them.

**The support window:**

- **Within a major version:** the newest released minor (`X.Y`) and, if one
  exists, the immediately preceding minor (`X.Y-1`) are supported. When a new
  minor is released, the older of those two becomes **end-of-life**
  immediately. Patch releases (`X.Y.Z`) do not change the supported set.
- **Across a major version:** when `(X+1).0.0` is released, the previous
  major's final minor line remains supported for security fixes for
  **12 months**, then becomes end-of-life. No other minor line from the
  previous major is supported.
- **Only security fixes are backported** to a previous minor or to a previous
  major's final minor. Feature work and non-security bug fixes land on the
  newest minor only. A critical fix (security or data-loss) may be backported
  outside this window at the maintainer's discretion, and that exception is
  announced.
- **End-of-life** means no security fixes and no migration support. This
  section is the canonical statement of the supported set; the operator-facing
  summary is in [`docs/supported-versions.md`](docs/supported-versions.md).
- Security fixes are authored on `main` and **cherry-picked** onto the
  still-supported `release/vX.Y.0` branch(es) before tagging; a fix that cannot
  be cherry-picked cleanly is re-implemented on the older branch and called out
  in the advisory.
- The supported set and the tags crossing EOL are reviewed annually as part of
  the [security stewardship cadence](docs/security/security-cadence.md); this
  statement is reviewed on the same cadence so it cannot quietly become stale.

## Scope

In scope: the backend (Go), frontend (React/TypeScript), Android app
(Kotlin), and the CardDAV/CalDAV sync implementation in this repository.

Out of scope: vulnerabilities in third-party dependencies should be
reported upstream (though we're happy to hear about them too, so we can
track and update); social engineering; and denial-of-service reports against
a self-hosted instance you don't control.

## What We Already Do

For context, this repo already runs: CodeQL static analysis, Trivy
container scanning, GitHub secret scanning (provider patterns), a
`gitleaks` pre-commit scan that also knows this app's own `mycorrhizal_`
token format (plus a full-history CI `gitleaks` scan of that format in the
required docs-citations job, using the base branch's rule config) and catches
its labelled webhook-signing/device-grant secrets via the default generic rule
(an unlabelled unprefixed value remains indistinguishable — see ASVS V1.1.1),
and dependency review on every pull request, and pins its Go toolchain and
container base images. See
[README-developer.md](README-developer.md) for the full CI posture.

## Security Documentation

- [`docs/security/threat-model.md`](docs/security/threat-model.md) — assets, trust boundaries, threat
  actors, and the written record of every deliberate security trade-off this project has made (e.g.
  why data-at-rest encryption has a plaintext-search exception, why the Android app targets MASVS-L1).
- [`docs/security/asvs-l2.md`](docs/security/asvs-l2.md) — OWASP ASVS 4.0.3 (L2) + API Security Top 10
  control checklist for the backend, frontend, and deployment. The level claim is **self-assessed**
  (see the [verification report](docs/security/asvs-l2-verification-report.md) §6), not a
  commissioned third-party audit.
- [`docs/security/masvs-l1.md`](docs/security/masvs-l1.md) — OWASP MASVS 1.5.0 (L1) control checklist
  for the Android client, likewise self-assessed.
- [`docs/security/incident-response.md`](docs/security/incident-response.md) — operator runbook for
  responding to a suspected compromise: containment, credential/key rotation procedures, and
  scenario playbooks.
- [`docs/security/security-cadence.md`](docs/security/security-cadence.md) — the rotation intervals
  for the root secrets, the register of when each was last performed, the Android signing-keystore
  custody/backup posture, and the annual access-list and support-window reviews (issues #955, #956).
- [`GOVERNANCE.md`](GOVERNANCE.md) — project roles and the list of members with access to
  sensitive resources (repository admin, release-signing secrets, deployment host).
