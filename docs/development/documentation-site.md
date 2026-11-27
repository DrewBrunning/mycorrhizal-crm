---
title: Documentation site
parent: Development
nav_order: 20
---

# Documentation site

The user documentation is a Jekyll (just-the-docs) site published to GitHub Pages
(issue #2244). It is versioned: a reader sees the docs for the release they run.

| URL | What it is |
|---|---|
| `/` | The newest release, built from its tag |
| `/X.Y/` | The newest patch of each older minor release, built from its tag |
| `/next/` | The unreleased docs on `main`, including the unreleased changelog |
| `/versions.json` | The list the version switcher reads |

Before the first release at or above the floor, `main` is the whole site at `/`.

## How it is built

- **What triggers a deploy:** a push to `main` that changes `docs/**`, `CHANGELOG.md` or the build script; a
  `v*` tag push (a new release; a release candidate changes nothing because release candidates are not
  published); or a manual `workflow_dispatch`.
- **Which tags:** `cd backend && go run ./cmd/docsversions` reads `git tag -l 'v*'` and picks the newest
  `vX.Y.Z` of each minor at or above its `-floor` (default `v1.7.0`, the first release with the
  restructured site). Raise the floor when a very old release stops being supported.
- **Build:** `scripts/build-docs-versions.sh` archives `docs/` (and `CHANGELOG.md`) at each tag, adds
  the current version switcher (`docs/_includes/header_custom.html`) and the generated Changelog page,
  and runs one Jekyll build per version. Everything is deployed as one Pages artifact.
- **The Changelog page** is generated from the root `CHANGELOG.md` and never committed
  ([Changelog policy](../changelog-policy.md)).

## Writing docs

`docs/` on `main` is the documentation for the **next** release: edit it in the same PR as the code
change. A published release's docs are rebuilt from its tag, so correcting them needs a patch release;
the site does not read maintenance branches.

Link between pages with `page.html` (not `page.md`): a plain Jekyll build does not rewrite `.md` links,
so they would 404 on the site.

## Previewing locally

```sh
cd docs && bundle install && bundle exec jekyll serve
```

shows a single version without the switcher. To build the full versioned site into `docs-site/` (needs
Go, Ruby and `jq`, and your tags fetched):

```sh
PAGES_BASE_PATH= scripts/build-docs-versions.sh
```
