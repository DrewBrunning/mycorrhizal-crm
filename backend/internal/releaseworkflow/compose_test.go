package releaseworkflow

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func committedFile(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot(t), name))
	require.NoError(t, err)
	return string(b)
}

// TestCommittedComposeImages pins the operator/source-build split: the
// documented compose file pulls a published image (so a two-file download is
// enough), while the build override is the only place that builds the
// checked-out tree.
func TestCommittedComposeImages(t *testing.T) {
	assert.Empty(t, CheckComposeImages(
		committedFile(t, CanonicalComposeFile),
		committedFile(t, BuildComposeFile),
	))
}

func TestCheckComposeImagesCatchesEachRegression(t *testing.T) {
	const goodCanonical = `
services:
  mycorrhizal:
    image: ghcr.io/drewbrunning/mycorrhizal-crm:${IMAGE_TAG:-latest}
`
	const goodBuild = `
services:
  mycorrhizal:
    image: mycorrhizal-crm-local:latest
    build:
      context: .
      dockerfile: Dockerfile
`
	require.Empty(t, CheckComposeImages(goodCanonical, goodBuild))

	cases := []struct {
		name      string
		canonical string
		build     string
		want      string
	}{
		{
			name:      "canonical has no service",
			canonical: "services: {}\n",
			build:     goodBuild,
			want:      "has no `mycorrhizal` service",
		},
		{
			name:      "canonical builds from source",
			canonical: "services:\n  mycorrhizal:\n    build: .\n",
			build:     goodBuild,
			want:      "must not declare `build:`",
		},
		{
			name:      "canonical pulls the wrong image",
			canonical: "services:\n  mycorrhizal:\n    image: ghcr.io/someone-else/crm:${IMAGE_TAG:-latest}\n",
			build:     goodBuild,
			want:      "must pull the published image",
		},
		{
			name:      "canonical requires IMAGE_TAG",
			canonical: "services:\n  mycorrhizal:\n    image: ghcr.io/drewbrunning/mycorrhizal-crm:${IMAGE_TAG:?set IMAGE_TAG}\n",
			build:     goodBuild,
			want:      "must default `IMAGE_TAG`",
		},
		{
			name:      "build override has no service",
			canonical: goodCanonical,
			build:     "services: {}\n",
			want:      "has no `mycorrhizal` service",
		},
		{
			name:      "build override does not build",
			canonical: goodCanonical,
			build:     "services:\n  mycorrhizal:\n    image: mycorrhizal-crm-local:latest\n",
			want:      "must declare `build:`",
		},
		{
			name:      "build override has no stable local tag",
			canonical: goodCanonical,
			build:     "services:\n  mycorrhizal:\n    build: .\n",
			want:      "must pin a local `image:` tag",
		},
		{
			name:      "build override reuses the published tag",
			canonical: goodCanonical,
			build:     "services:\n  mycorrhizal:\n    image: ghcr.io/drewbrunning/mycorrhizal-crm:latest\n    build: .\n",
			want:      "must not reuse the published image tag",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckComposeImages(tc.canonical, tc.build)
			found := false
			for _, f := range got {
				if strings.Contains(f, tc.want) {
					found = true
				}
			}
			assert.True(t, found, "want a finding containing %q, got %q", tc.want, got)
		})
	}
}

func TestCheckComposeImagesOnUnparseableFiles(t *testing.T) {
	got := CheckComposeImages(":\tnot yaml", ":\tnot yaml")
	require.NotEmpty(t, got)
	assert.Contains(t, got[0], CanonicalComposeFile+" does not parse")

	got = CheckComposeImages("services:\n  mycorrhizal:\n    image: ghcr.io/drewbrunning/mycorrhizal-crm:${IMAGE_TAG:-latest}\n", ":\tnot yaml")
	require.NotEmpty(t, got)
	assert.Contains(t, got[0], BuildComposeFile+" does not parse")
}
