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

This project is pre-1.0 (currently in beta) and does not yet maintain
long-term-support branches. Only the **latest tagged release** receives
security fixes; previous tags are not backported to. Users are expected to
upgrade to the latest release to receive security fixes.

**The support window at 1.0** (the policy is defined now and enforced from the
`1.0.0` tag, issue #956):

- Every `1.x` release is supported for **12 months from the next minor
  release**; the supported set is therefore the newest minor plus the prior
  minor while it is inside that window.
- A release that falls outside the window is **end-of-life** — no security
  fixes, no migration support. The supported set is stated on the release page
  and in [`docs/supported-versions.md`](docs/supported-versions.md).
- Security fixes are authored on `main` and **cherry-picked** onto the
  still-supported `release/vX.Y.0` branch(es) before tagging; a fix that cannot
  be cherry-picked cleanly is re-implemented on the older branch and called out
  in the advisory.
- Which tags cross EOL is reviewed annually as part of the
  [security stewardship cadence](docs/security/security-cadence.md); the
  pre-1.0 statement above is reviewed on the same cadence so it cannot quietly
  become stale.

## Scope

In scope: the backend (Go), frontend (React/TypeScript), Android app
(Kotlin), and the CardDAV/CalDAV sync implementation in this repository.

Out of scope: vulnerabilities in third-party dependencies should be
reported upstream (though we're happy to hear about them too, so we can
track and update); social engineering; and denial-of-service reports against
a self-hosted instance you don't control.

## What We Already Do

For context, this repo already runs: CodeQL static analysis, Trivy
container scanning, GitHub secret scanning, and dependency review on every
pull request, and pins its Go toolchain and container base images. See
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
