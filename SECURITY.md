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

## Supported Versions

This project is pre-1.0 (currently in beta) and does not yet maintain
long-term-support branches. Only the **latest tagged release** receives
security fixes; previous tags are not backported to. Users are expected to
upgrade to the latest release to receive security fixes.

Once the project reaches a 1.0/stable milestone, this policy will be
revisited to define a real support window.

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
