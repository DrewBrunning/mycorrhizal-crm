---
title: Changelog policy
nav_order: 13
---

# Changelog policy

**This is the canonical changelog statement (REL-05, issue #449).** It defines
what the changelog *is* for this project, how the Release notes are produced,
where operator actions go, when a change must carry an entry, and where the
record begins. `cmd/changelogsection` and
the `changelog-note` job in `.github/workflows/unit-tests.yml` are defined
against this document; a change here is a change to those, and vice versa (see
[Document consistency](#document-consistency)).

This page is contributor- and maintainer-facing. Users and operators read the
curated [`CHANGELOG.md`](../CHANGELOG.md) at the repository root; each
[GitHub Release](https://github.com/DrewBrunning/mycorrhizal-crm/releases)
carries a copy of its section.

## What the changelog is

There are two records, with different audiences:

1. **`CHANGELOG.md`** — the human-readable changelog, in
   [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. It is
   written for people who use or self-host the app: plain language, one short
   bullet per user-visible change, grouped by release. Every user-visible change
   adds its entry to `[Unreleased]` in the same pull request, so contributors see
   what is coming and the release section is already written at tag time.
2. **The GitHub Release notes** — a copy. `docker-publish.yml` writes the
   Release body from the `CHANGELOG.md` section for the tag (via
   `cmd/changelogsection`), plus the Docker image block and a compare link to the
   previous tag, which keeps every pull request reachable. Relative links become
   absolute links to the tagged tree. A final tag needs its `## [X.Y.Z]`
   section; `release.yml` refuses to tag without one, and the Release job fails
   on a missing or empty section. A release candidate publishes `[X.Y.Z]` if it
   exists, else `[Unreleased]`. `promote-rc.yml` publishes the RC commit's
   `[X.Y.Z]` section and refuses to promote without one, so rename
   `[Unreleased]` before the last RC.

GitHub's generated "What's Changed" list and the harvest of `## Upgrade notes`
blocks from PR bodies ([#2242](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2242))
no longer feed the Release. Operator actions go in the `> **Upgrade**` blockquote
of the changelog section.

## Writing `CHANGELOG.md` entries

Sections, in this order, using only those that apply: `Added`, `Changed`,
`Deprecated`, `Removed`, `Fixed`, `Security`, `Internal`.

Format: `- **Bold lead phrase** ([#123](https://github.com/DrewBrunning/mycorrhizal-crm/issues/123)): what changed.`

- One bullet per change, **two lines of prose and about 40 words at most**. No
  sub-bullets. Several PRs that deliver one feature get one bullet.
- Link the GitHub **issue** (not the PR number GitHub appends to a squashed
  subject) with a full URL, so the file reads correctly outside GitHub. No issue,
  no link.
- Write for users and operators: no file, function, library or framework names
  outside `Internal`; no rationale, implementation story or future plans — those
  belong in the commit message, an ADR or `docs/development/`.
- A breaking change, a migration the operator must know about, or a required
  action goes in a `> **Upgrade**: …` blockquote at the top of the release
  section. A change that needs more than two lines of explanation is explained in
  the user documentation and linked from the bullet.
- `Internal` is not a second commit history: roughly three one-line entries per
  release, only for internal changes that are valuable or disruptive. Tests,
  refactors, dependency bumps and tidy-ups get no entry.
- A PR with no user-visible effect (test-only, CI, refactor, docs-only) adds no
  entry and says `no-changelog: <reason>` in its description.
- The maintainer renames `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD` as part of the
  release.
- `cmd/changelogcheck` lints `[Unreleased]` for section names and order, the
  bold lead phrase, sub-bullets, the word limit, full-URL issue links and the
  `Internal` cap, in CI and the pre-commit hook. Released sections are not
  checked.

## Related policies

`Deprecated` entries are also recorded in
[`deprecations.md`](deprecations.md) (MAINT-01, issue #490); `Breaking Changes`
are classified by [`breaking-change-policy.md`](breaking-change-policy.md)
(MAINT-02, issue #491). This page does not restate those.

## When a pull request must carry operator notes

A PR needs an `## Upgrade notes` block in its description when it does any of:

- adds, renames, retypes, or drops a **database migration**;
- adds, renames, or changes the default or meaning of a **configuration
  variable**;
- changes **observable behavior** an operator or client could rely on;
- **deprecates** anything on a covered surface;
- otherwise requires **action before or after upgrading**.

Add `## Breaking changes` as well when the change is breaking under
[`breaking-change-policy.md`](breaking-change-policy.md).

A PR adds a `## Security-relevant changes` block when it changes the security
posture in a way an operator should notice — a new guard or a changed default,
a raised version floor, a new required configuration value, a new
authentication path or trust boundary. It complements the `security` label
(the generated *what changed* half) with the operator-facing *what this means
for you* half; a change that adds a new security-relevant surface also needs a
per-release delta row ([`adversarial-deltas.md`](security/adversarial-deltas.md),
issue #953).

If a PR touches one of the tracked paths (`backend/database/migrations/**`,
`backend/config/config.go`, `.env.example`) but genuinely needs no operator
note, put `no-changelog: <reason>` in the PR description. The `changelog-note`
CI job fails a PR that touches those paths with none of an `## Upgrade notes` /
`## Security-relevant changes` / `## Breaking changes` heading and no
`no-changelog:` line.

Everything else — internal refactors, test-only changes, most bug fixes — needs
nothing beyond the right label.

## Skip-version upgrades

Self-hosted operators skip versions — `v1.0.0 → current` directly is supported
([`upgrade-compatibility.md`](upgrade-compatibility.md), issue #529). The
accumulated operator actions across a range are read section by section, newest
first, in [`CHANGELOG.md`](../CHANGELOG.md): each release's `> **Upgrade**`
blockquote states what an operator must do. The
one known cross-version upgrade defect (a crash upgrading into `v0.6.1`–`v0.6.8`
from `≤ v0.6.0` with audit history, fixed in `v0.6.9`) is documented in
[`upgrade-compatibility.md`](upgrade-compatibility.md) and repeated in the
affected releases' notes.

## Versions and issue links

The version a set of notes belongs to is the **git tag**
([`versioning-policy.md`](versioning-policy.md), REL-01). A release cannot be
tagged or published without its changelog section, and the Release body links
the compare view, so every change stays traceable to a pull request.

## Where the record begins

`CHANGELOG.md` begins at `v1.0.0` (the current supported-upgrade floor); earlier
releases are not backfilled there. The GitHub Release record begins at `v0.6.0` — the
[supported-upgrade floor](upgrade-compatibility.md) when this
policy landed (issue #529; the floor is now `v1.0.0`, issue #1170).
Releases from `v0.6.0` onward carry an **Upgrade notes** section (the
`v0.6.0`–`v0.6.12` releases were backfilled with one when this policy landed).
Earlier releases have generated notes only; reconstructing twenty pre-floor
releases is not worth it, and nothing supported upgrades from below the floor in
one hop anyway.

## Document consistency

- `cmd/changelogsection` is covered by its unit tests; `release.yml` and
  `docker-publish.yml` both call it.

These run in the normal `go test ./...` and CI, so the policy cannot drift from
the tooling without a red build.
