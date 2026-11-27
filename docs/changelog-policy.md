---
title: Changelog policy
parent: Development
nav_order: 14
---

# Changelog policy

**This is the canonical changelog statement (REL-05, issue #449).** It defines
what the changelog *is* for this project, how the Release notes are produced,
where operator actions go, when a change must carry an entry, and where the
record begins. `cmd/changelogsection`, `cmd/changelogcheck`,
`cmd/changelogfragmentcheck`, `cmd/changelogassemble` and
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
   bullet per user-visible change, grouped by release. A pull request adds a
   one-file **fragment** under [`changelog.d/`](../changelog.d/README.md)
   instead of editing `[Unreleased]` directly, so parallel PRs do not conflict
   on the shared section; the release assembly writes the section before tag
   time.
2. **The GitHub Release notes** — a copy. `docker-publish.yml` writes the
   Release body from the `CHANGELOG.md` section for the tag (via
   `cmd/changelogsection`), plus the Docker image block and a compare link to the
   previous tag, which keeps every pull request reachable. Relative links become
   absolute links to the tagged tree. A final tag needs its `## [X.Y.Z]`
   section; `release.yml` refuses to tag without one, and the Release job fails
   on a missing or empty section. A release candidate publishes `[X.Y.Z]` if it
   exists, else `[Unreleased]`. `promote-rc.yml` publishes the RC commit's
   `[X.Y.Z]` section and refuses to promote without one, so assemble the
   fragments and rename `[Unreleased]` before the last RC.

The same `CHANGELOG.md` is also published as the **Changelog** page of the
documentation site. `scripts/build-docs-versions.sh` runs
`cd backend && go run ./cmd/gensitechangelog` for every published docs version,
which writes the gitignored `changelog.md` page into the docs folder (relative
links are rewritten for the site). A merge to `main` that changes `CHANGELOG.md`
redeploys the site, so the `[Unreleased]` section is public under `/next/` as
soon as it is written. Never edit the generated page.
See [the documentation site](development/documentation-site.md).

GitHub's generated "What's Changed" list and the harvest of `## Upgrade notes`
blocks from PR bodies ([#2242](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2242))
no longer feed the Release. Operator actions go in the `> **Upgrade**` blockquote
of the changelog section.

## Adding an entry: `changelog.d/` fragments

One file per change, never a direct edit to `[Unreleased]`:

- **Name:** `<section>-<issue>[-<n>].md`, where `<section>` is one of
  `added`, `changed`, `deprecated`, `removed`, `fixed`, `security`, `internal`
  (the lowercase form of the `CHANGELOG.md` heading) and `<issue>` is the GitHub
  issue number. Add `-<n>` for a second fragment for the same issue and section.
  Examples: `fixed-2917.md`, `added-2513-2.md`. [`changelog.d/README.md`](../changelog.d/README.md)
  is ignored by the loader.
- **Content:** exactly one Keep-a-Changelog bullet line, with no section heading
  and no sub-bullets:

  ```markdown
  - **Data settings says when the server cannot be reached** ([#2917](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2917)): the page now explains the connection failure instead of showing an empty form.
  ```

- **Rules:** the bullet rules below apply unchanged, and the issue number in the
  filename must appear in an `issues/<n>` URL in the bullet.
- A follow-up PR that finishes the same feature edits that feature's fragment
  (or adds a `-<n>` fragment when the first already merged) rather than writing a
  second bullet.

Fragments exist so a batch of pull requests can merge in any order without
editing the same lines of `[Unreleased]`: each PR touches its own file, and the
release notes are assembled once, at release time. `cmd/changelogfragmentcheck`
validates every fragment in CI and the pre-commit hook, so a malformed filename,
section, bullet or issue link fails the PR rather than the release.

## Writing entries

Sections, in this order, using only those that apply: `Added`, `Changed`,
`Deprecated`, `Removed`, `Fixed`, `Security`, `Internal`.

Format: `- **Bold lead phrase** ([#123](https://github.com/DrewBrunning/mycorrhizal-crm/issues/123)): what changed.`

- One bullet per change, **two lines of prose and about 40 words at most**. No
  sub-bullets. Several PRs that deliver one feature get one bullet.
- **One bullet per feature across platforms.** A feature or fix that lands on the web and
  Android (or the backend) in the same release is one bullet with every issue linked, and the
  bullet does not name a platform. Name a platform only for something that exists on that
  platform alone, or that catches one platform up to a feature the other shipped in an earlier
  release. Follow-up PRs that finish the same feature extend its existing bullet.
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
- The maintainer assembles the fragments, commits the result, then renames
  `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD` as part of the release.
- `cmd/changelogcheck` lints `[Unreleased]` for section names and order, the
  bold lead phrase, sub-bullets, the word limit, full-URL issue links and the
  `Internal` cap, in CI and the pre-commit hook. Released sections are not
  checked.

## Assembling the release

`cmd/changelogassemble` is the only writer of fragments into `CHANGELOG.md`:

- `cd backend && go run ./cmd/changelogassemble -check` fails unless every
  fragment's exact bullet is already present in `[Unreleased]`. `release.yml`
  and `promote-rc.yml` run this gate immediately before the
  `changelogsection` gate, so a release cannot tag unassembled notes.
- `cd backend && go run ./cmd/changelogassemble -write` inserts the missing
  bullets under their canonical `### Section`, creating a section in policy
  order when absent. It preserves existing content (including the
  `> **Upgrade**` blockquote) and the existing bullets' order, dedupes exact
  bullet text, then deletes the folded fragment files. Commit `CHANGELOG.md`
  together with those deletions. `-keep` retains the fragment files.
- A second `-write` changes nothing; the command does not touch released
  sections.

## Related policies

`Deprecated` entries are also recorded in
[`deprecations.md`](deprecations.md) (MAINT-01, issue #490); `Breaking Changes`
are classified by [`breaking-change-policy.md`](breaking-change-policy.md)
(MAINT-02, issue #491). This page does not restate those.

## When a pull request must carry a changelog entry

A user-visible change adds a fragment under `changelog.d/` in the same pull
request. Operator-visible changes need one without exception:

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
fails a PR that touches those paths, adds no `changelog.d/*.md` fragment (and
does not change `CHANGELOG.md`), and has no `no-changelog:` line.

Internal refactors, test-only changes and other changes with no user-visible
effect need no entry.

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

`CHANGELOG.md` begins at `v0.6.0`, the first release with a supported-upgrade
floor ([`upgrade-compatibility.md`](upgrade-compatibility.md)). The `v0.6.x` to
`v0.9.0` sections were backfilled from the GitHub Release notes and are
shorter and more technical than later ones; the word limits apply to
`[Unreleased]` only. Earlier releases are not backfilled, and nothing supported
upgrades from below the floor in one hop anyway.

## Document consistency

- `cmd/changelogcheck`, `cmd/changelogfragmentcheck` and
  `cmd/changelogassemble` share section order and bullet validation in
  `internal/changelog`, so the per-PR fragment check and the `[Unreleased]`
  lint cannot drift apart.
- `cmd/changelogsection` is covered by its unit tests; `release.yml` and
  `docker-publish.yml` both call it.
- `changelogfragmentcheck` runs in CI and the pre-commit hook;
  `changelogassemble -check` gates `release.yml` and `promote-rc.yml`.

These run in the normal `go test ./...` and CI, so the policy cannot drift from
the tooling without a red build.
