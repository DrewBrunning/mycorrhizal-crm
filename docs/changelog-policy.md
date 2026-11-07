---
title: Changelog policy
nav_order: 13
---

# Changelog policy

**This is the canonical changelog statement (REL-05, issue #449).** It defines
what the changelog *is* for this project, how the "what changed" list is
produced, where the operator-facing part comes from, when a change must carry
one, and where the record begins. `.github/release.yml`, `cmd/releasenotes`, and
the `changelog-note` job in `.github/workflows/unit-tests.yml` are defined
against this document; a change here is a change to those, and vice versa (see
[Document consistency](#document-consistency)).

This page is contributor- and maintainer-facing. Users and operators read the
curated [`CHANGELOG.md`](../CHANGELOG.md) at the repository root; the
machine-assembled record is on each
[GitHub Release](https://github.com/DrewBrunning/mycorrhizal-crm/releases).

## What the changelog is

There are two records, with different audiences:

1. **`CHANGELOG.md`** — the human-readable changelog, in
   [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) format. It is
   written for people who use or self-host the app: plain language, one short
   bullet per user-visible change, grouped by release. Every user-visible change
   adds its entry to `[Unreleased]` in the same pull request, so contributors see
   what is coming and the release section is already written at tag time.
2. **The GitHub Release notes** — the complete, traceable record, assembled from
   two sources:
   - **What changed** — GitHub's own release-notes generator
     (`generate_release_notes`), turned into labelled sections by
     `.github/release.yml`. One section per category, populated from the labels
     on the pull requests merged since the previous release.
   - **Operator notes** — an **Upgrade notes** section, a **Security-relevant
     changes** section when a change affects the security posture, and a
     **Breaking changes** section when applicable, **hand-written in each pull
     request's description** and harvested at release time by `cmd/releasenotes`
     into the top of the release body. Security-relevant changes are their own
     section (not folded into the upgrade notes) so a new default-off guard, a
     changed floor, or a new required config cannot be missed among feature
     notes (issue #953).

The generated list answers *what changed*; only a human can answer *what an
operator must do about it*, which is why the operator notes are never generated
from a diff. `CHANGELOG.md` is the curated summary of both. Until the release
workflow copies the changelog section into the release body (tracked in
[#2242](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2242)), the two
are maintained side by side: the PR-body operator notes still feed the release,
and the `CHANGELOG.md` entry feeds the file.

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

## Categories

`.github/release.yml` defines these sections, each filled by a PR label:

| Section | Label |
|---|---|
| Security | `security` |
| Breaking Changes | `breaking-change` |
| Deprecated | `deprecation` |
| Added | `enhancement` |
| Fixed | `bug` |
| Dependencies | `dependencies` |
| Documentation | `documentation` |
| Other Changes | `*` (catch-all — every merged PR appears somewhere) |

The `breaking-change` and `deprecation` labels are created when the first such
change ships; until then those sections simply stay empty. The **Breaking
changes** operator section (part 2) is harvested from PR *bodies* and does not
depend on the label.

`Deprecated` entries are also recorded in
[`deprecations.md`](deprecations.md) (MAINT-01, issue #490); `Breaking Changes`
are classified by [`breaking-change-policy.md`](breaking-change-policy.md)
(MAINT-02, issue #491). This page does not restate those.

## When a pull request must carry a changelog entry

A user-visible change adds its entry to `[Unreleased]` in the same pull request.
Operator-visible changes need one without exception:

- a **database migration** (add, rename, retype, drop);
- a **configuration variable** (add, rename, or change of default or meaning);
- **observable behavior** an operator or client could rely on;
- a **deprecation** on a covered surface
  ([`deprecations.md`](deprecations.md); breaking changes follow
  [`breaking-change-policy.md`](breaking-change-policy.md));
- anything that requires **action before or after upgrading**. Put it in the
  release's `> **Upgrade**` blockquote.

A change to the security posture an operator should notice (a new guard or
changed default, a raised version floor, a new required configuration value, a
new authentication path or trust boundary) gets a `Security` bullet. A new
security-relevant surface also needs a per-release delta row
([`adversarial-deltas.md`](security/adversarial-deltas.md), issue #953).

If a PR touches one of the tracked paths (`backend/database/migrations/**`,
`backend/config/config.go`, `.env.example`) but genuinely needs no entry, put
`no-changelog: <reason>` in the PR description. The `changelog-note` CI job
fails a PR that touches those paths, does not change `CHANGELOG.md`, and has no
`no-changelog:` line.

Internal refactors, test-only changes and other changes with no user-visible
effect need no entry.

## Upgrade notes are always answered

`cmd/releasenotes` always emits an **Upgrade notes** section. When no PR in the
range contributed one, it emits:

> None — no action required for an instance already on ≥ v1.0.0.

So every release states the operator position explicitly rather than by
omission.

## Skip-version upgrades

Self-hosted operators skip versions — `v1.0.0 → current` directly is supported
([`upgrade-compatibility.md`](upgrade-compatibility.md), issue #529). Because
every release body carries an **Upgrade notes** section, the accumulated
operator actions across a range are read release-by-release down the
[Releases page](https://github.com/DrewBrunning/mycorrhizal-crm/releases). The
one known cross-version upgrade defect (a crash upgrading into `v0.6.1`–`v0.6.8`
from `≤ v0.6.0` with audit history, fixed in `v0.6.9`) is documented in
[`upgrade-compatibility.md`](upgrade-compatibility.md) and repeated in the
affected releases' notes.

## Versions and issue links

The version a set of notes belongs to is the **git tag**
([`versioning-policy.md`](versioning-policy.md), REL-01). Publication and note
assembly happen together: the harvest runs inside the release build
(`docker-publish.yml`, `create-release`), so a release cannot be published
without its notes. Each generated line links its PR, and each harvested block is
prefixed with its `#NNN` link — which is how the `0.9.x` release-candidate
traceability requirement (issues #503/#504), *every change traceable to a
finding*, becomes checkable rather than aspirational.

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

- `.github/release.yml`'s category set is pinned by
  `backend/internal/releasenotes` (`RequiredCategories`);
  `TestReleaseConfigHasRequiredCategories` fails the build if a section is
  dropped or the `*` catch-all loses its label.
- `cmd/releasenotes` (harvest + assembly, including the **Security-relevant
  changes** section) is covered by `backend/internal/releasenotes` unit tests.
- `CHANGELOG.md` entries are not machine-checked yet; a lint check for
  `[Unreleased]` is part of the follow-up in
  [#2242](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2242).
- `changelog-note` in `.github/workflows/unit-tests.yml` enforces the
  operator-note requirement on every pull request; its accepted heading set is
  the three operator sections above.

These run in the normal `go test ./...` and CI, so the policy cannot drift from
the tooling without a red build.
