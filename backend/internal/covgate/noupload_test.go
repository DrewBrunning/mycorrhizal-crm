package covgate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filtersFixture = `
backend:
  - 'backend/**'
  - 'testdata/**'
frontend:
  - 'frontend/**'
android:
  - 'android/**'
openapi:
  - 'backend/openapi.yaml'
workflows:
  - '.github/**'
  - 'scripts/tests/**'
shell:
  - '**/*.sh'
`

func TestMatchGlob(t *testing.T) {
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"backend/**", "backend/main.go", true},
		{"backend/**", "backend/a/b/c.go", true},
		{"backend/**", "backendish/main.go", false},
		{"backend/**", "docs/backend/main.go", false},
		{".github/**", ".github/workflows/x.yml", true}, // dotfiles match, like paths-filter's dot:true
		{"**/*.sh", "scripts/a.sh", true},
		{"**/*.sh", "a.sh", true}, // ** also matches zero directories
		{"**/*.sh", "scripts/a.txt", false},
		{"android/**/build.gradle.kts", "android/app/build.gradle.kts", true},
		{"android/**/build.gradle.kts", "android/build.gradle.kts", true},
		{"android/**/build.gradle.kts", "android/app/x/settings.gradle.kts", false},
		{"backend/openapi.yaml", "backend/openapi.yaml", true},
		{"backend/openapi.yaml", "backend/openapiXyaml", false}, // '.' is literal
		{"backend/*.go", "backend/main.go", true},
		{"backend/*.go", "backend/a/main.go", false}, // * stops at '/'
		{"a?c", "abc", true},
		{"a?c", "a/c", false},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, matchGlob(c.pattern, c.path), "%q vs %q", c.pattern, c.path)
	}
}

func TestTriggersCoverageUpload(t *testing.T) {
	cases := []struct {
		name  string
		files []string
		want  bool
	}{
		{"docs and changelog only", []string{"CHANGELOG.md", "docs/changelog-policy.md"}, false},
		{"codecov.yml only", []string{"codecov.yml"}, false},
		{"shell filter alone is not an upload suite", []string{"tools/x.sh"}, false},
		{"backend file", []string{"docs/a.md", "backend/main.go"}, true},
		{"frontend file", []string{"frontend/src/a.ts"}, true},
		{"android file", []string{"android/app/A.kt"}, true},
		{"openapi", []string{"backend/openapi.yaml"}, true},
		{"workflow change re-runs everything", []string{".github/workflows/x.yml"}, true},
		{"no files is fail-safe: assume upload", nil, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := TriggersCoverageUpload([]byte(filtersFixture), c.files)
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestTriggersCoverageUploadErrors(t *testing.T) {
	_, err := TriggersCoverageUpload([]byte("backend: [unterminated"), []string{"a"})
	require.Error(t, err)

	// A missing upload-suite filter must fail loudly, not silently read as
	// "never matches" (which would post success for a real code change).
	_, err = TriggersCoverageUpload([]byte("backend:\n  - 'backend/**'\n"), []string{"docs/a.md"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "frontend")
}
