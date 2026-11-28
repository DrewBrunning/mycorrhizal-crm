package releaseworkflow

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func loadDocsPublish(t *testing.T) DocsPublishFiles {
	t.Helper()
	return DocsPublishFiles{
		Docs:    committedFile(t, ".github/workflows/docs.yml"),
		Release: committedFile(t, ".github/workflows/release.yml"),
		Promote: committedFile(t, ".github/workflows/promote-rc.yml"),
	}
}

func TestCommittedDocsPublishWiring(t *testing.T) {
	assert.Empty(t, CheckDocsPublish(loadDocsPublish(t)))
}

const docsOK = `
on:
  push:
    branches: ["main"]
    paths: ["docs/**", "CHANGELOG.md", "changelog.d/**", "scripts/build-docs-versions.sh"]
  workflow_dispatch:
jobs:
  build:
    steps:
      - run: scripts/build-docs-versions.sh docs-site
`

const releaseOK = `
on:
  workflow_dispatch:
jobs:
  release:
    steps:
      - run: git push origin "refs/tags/${VERSION}"
  publish-docs:
    needs: [preflight, release]
    if: ${{ !inputs.dry_run && needs.preflight.outputs.is_rc != 'true' }}
    permissions:
      actions: write
    steps:
      - run: gh workflow run docs.yml --ref main --repo "$GITHUB_REPOSITORY"
`

const promoteOK = `
on:
  workflow_dispatch:
jobs:
  promote:
    steps:
      - run: git push origin "refs/tags/${FINAL_TAG}"
  publish-docs:
    needs: promote
    permissions:
      actions: write
    steps:
      - run: gh workflow run docs.yml --ref main --repo "$GITHUB_REPOSITORY"
`

func TestCheckDocsPublishAcceptsTheIntendedWiring(t *testing.T) {
	assert.Empty(t, CheckDocsPublish(DocsPublishFiles{Docs: docsOK, Release: releaseOK, Promote: promoteOK}))
}

func TestCheckDocsPublishCatchesEachRegression(t *testing.T) {
	cases := []struct {
		name  string
		files func() DocsPublishFiles
		want  string
	}{
		{"docs.yml triggers on a tag push", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: replaceOnce(t, docsOK, `branches: ["main"]`, `branches: ["main"]
    tags: ["v*"]`), Release: releaseOK, Promote: promoteOK}
		}, "tag"},
		{"docs.yml cannot be dispatched", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: replaceOnce(t, docsOK, "  workflow_dispatch:\n", ""), Release: releaseOK, Promote: promoteOK}
		}, "workflow_dispatch"},
		{"a fragment-only merge never redeploys /next/", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: replaceOnce(t, docsOK, `, "changelog.d/**"`, ""), Release: releaseOK, Promote: promoteOK}
		}, "changelog.d/**"},
		{"a docs-only merge never redeploys", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: replaceOnce(t, docsOK, `"docs/**", `, ""), Release: releaseOK, Promote: promoteOK}
		}, "docs/**"},
		{"release.yml never dispatches the docs", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: replaceOnce(t, releaseOK, "gh workflow run docs.yml", "echo"), Promote: promoteOK}
		}, "release.yml"},
		{"promote-rc.yml never dispatches the docs", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: releaseOK, Promote: replaceOnce(t, promoteOK, "gh workflow run docs.yml", "echo")}
		}, "promote-rc.yml"},
		{"the dispatch targets the tag, not main", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: replaceOnce(t, releaseOK, "--ref main", `--ref "$VERSION"`), Promote: promoteOK}
		}, "--ref main"},
		{"the dispatching job lacks actions: write", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: releaseOK, Promote: replaceOnce(t, promoteOK, "actions: write", "actions: read")}
		}, "actions: write"},
		{"the dispatch runs before the tag exists", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: replaceOnce(t, releaseOK, "needs: [preflight, release]", "needs: [preflight]"), Promote: promoteOK}
		}, "needs"},
		{"release.yml also publishes on a dry run", func() DocsPublishFiles {
			return DocsPublishFiles{Docs: docsOK, Release: replaceOnce(t, releaseOK, "!inputs.dry_run && ", ""), Promote: promoteOK}
		}, "dry_run"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckDocsPublish(tc.files())
			if assert.NotEmpty(t, got) {
				assert.Contains(t, joined(got), tc.want)
			}
		})
	}
}

// No paths filter means every push to main redeploys, which is never stale.
func TestCheckDocsPublishAcceptsNoPathsFilter(t *testing.T) {
	docs := replaceOnce(t, docsOK, `    paths: ["docs/**", "CHANGELOG.md", "changelog.d/**", "scripts/build-docs-versions.sh"]
`, "")
	assert.Empty(t, CheckDocsPublish(DocsPublishFiles{Docs: docs, Release: releaseOK, Promote: promoteOK}))
}

func TestCheckDocsPublishParseErrors(t *testing.T) {
	got := CheckDocsPublish(DocsPublishFiles{Docs: "a: [\n", Release: "b: [\n", Promote: "c: [\n"})
	assert.Len(t, got, 3)
}
