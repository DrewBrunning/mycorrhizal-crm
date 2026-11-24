# Changelog fragments

`CHANGELOG.md` is assembled at release time. A pull request does not edit
`[Unreleased]` directly: it adds **one fragment per change** in this directory,
so concurrent PRs never conflict on the shared section. The policy is
[`docs/changelog-policy.md`](../docs/changelog-policy.md).

## Filename

```
<section>-<issue>[-<n>].md
```

- `<section>` is one of `added`, `changed`, `deprecated`, `removed`, `fixed`,
  `security`, `internal` — the lowercase form of the `CHANGELOG.md` heading.
- `<issue>` is the GitHub issue number this change links.
- `-<n>` is optional, for a second fragment for the same issue and section.

Examples: `fixed-2917.md`, `added-2513-2.md`.

## Content

Exactly one Keep-a-Changelog bullet line, with no section heading and no
sub-bullets:

```markdown
- **Data settings says when the server cannot be reached** ([#2917](https://github.com/DrewBrunning/mycorrhizal-crm/issues/2917)): the page now explains the connection failure instead of showing an empty form.
```

The bullet follows the same rules as `CHANGELOG.md`
([`docs/changelog-policy.md`](../docs/changelog-policy.md)):

- **Bold lead phrase**, then the issue link with a full URL (no bare `#2917`).
- The issue number in the filename must appear in an `issues/<n>` URL in the
  bullet.
- Keep it to about 40 words (hard limit 60), one bullet per feature, and name a
  platform only when the change is platform-only.
- `Internal` is for the few internal changes worth recording, not commit
  history.
- An operator action goes in the release's `> **Upgrade**` blockquote; say so in
  the PR description so the maintainer adds it when assembling.

`no-changelog: <reason>` in the PR description still applies to a change with no
user-visible or operator-visible effect.

## Checks

```bash
cd backend && go run ./cmd/changelogfragmentcheck
```

validates every fragment here (CI and the pre-commit hook run it). The release
workflow refuses to tag until the fragments are assembled:

```bash
cd backend && go run ./cmd/changelogassemble -check
```

## Release assembly

The maintainer runs this before renaming `[Unreleased]` and tagging:

```bash
cd backend && go run ./cmd/changelogassemble -write
```

It inserts each missing bullet under its canonical section in policy order,
preserves existing content (including the `> **Upgrade**` blockquote) and
bullet order, dedupes exact bullets, then deletes the folded fragment files.
Commit both `CHANGELOG.md` and the deletions. A second run changes nothing;
pass `-keep` to retain the fragment files.

`README.md` and non-Markdown files are ignored by the loader.
