package releasegates

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

const dispatchReg = `{"gates": [
  {"name": "scan", "workflow": "docker-publish.yml", "check_context": null, "check_kind": "internal", "tier": "release-internal", "mandatory": true, "release_gate": false, "criterion": "c"},
  {"name": "apk-provenance", "workflow": "docker-publish.yml", "check_context": null, "check_kind": "internal", "tier": "release-internal", "mandatory": true, "release_gate": false, "criterion": "c"},
  {"name": "verify-release-assets", "workflow": "docker-publish.yml", "check_context": null, "check_kind": "internal", "tier": "release-internal", "mandatory": true, "release_gate": false, "criterion": "c"},
  {"name": "optional-thing", "workflow": "docker-publish.yml", "check_context": "Optional thing", "check_kind": "check_run", "tier": "advisory", "mandatory": false, "release_gate": false, "criterion": "c"}
]}`

const goodWorkflow = `
jobs:
  scan:
    if: ${{ always() && needs.build.result == 'success' }}
  apk-provenance:
    if: ${{ always() && github.event_name == 'push' }}
  verify-release-assets:
    if: ${{ always() }}
    steps:
      - run: |
          echo "::error::missing mycorrhizal-apk.intoto.jsonl"
  optional-thing:
    if: ${{ github.event_name == 'push' }}
`

func TestCheckDispatchPath(t *testing.T) {
	reg, f := Parse([]byte(dispatchReg))
	assert.Empty(t, f)

	cases := []struct {
		name     string
		workflow string
		want     string // substring one of the findings must contain; "" = clean
		count    int    // exact number of findings expected (ignored when want == "")
	}{
		{"good", goodWorkflow, "", 0},
		{"new push-only guard on a mandatory gate fails",
			`
jobs:
  scan:
    if: ${{ always() && github.event_name == 'push' }}
  apk-provenance:
    if: ${{ github.event_name == 'push' }}
  verify-release-assets:
    steps:
      - run: echo "::error::x mycorrhizal-apk.intoto.jsonl"
`, `mandatory gate "scan"`, 1},
		{"missing job", `
jobs:
  apk-provenance:
    if: ${{ github.event_name == 'push' }}
  verify-release-assets:
    steps:
      - run: echo "::error::x mycorrhizal-apk.intoto.jsonl"
`, `names no job`, 1},
		{"compensator missing", `
jobs:
  scan: {}
  apk-provenance:
    if: ${{ github.event_name == 'push' }}
`, `has no such job`, 2},
		{"compensator push-only", `
jobs:
  scan: {}
  apk-provenance:
    if: ${{ github.event_name == 'push' }}
  verify-release-assets:
    if: ${{ github.event_name == 'push' }}
    steps:
      - run: echo "::error::x mycorrhizal-apk.intoto.jsonl"
`, `itself push-only`, 2},
		{"compensator no longer asserts", `
jobs:
  scan: {}
  apk-provenance:
    if: ${{ github.event_name == 'push' }}
  verify-release-assets:
    steps:
      - run: echo hello
`, `asserts "mycorrhizal-apk.intoto.jsonl"`, 1},
		{"invalid yaml", "jobs: [", `does not parse`, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := CheckDispatchPath(reg, tc.workflow)
			if tc.want == "" {
				assert.Empty(t, got)
				return
			}
			assert.Len(t, got, tc.count, "%v", got)
			found := false
			for _, g := range got {
				if strings.Contains(g, tc.want) {
					found = true
				}
			}
			assert.True(t, found, "no finding contains %q: %v", tc.want, got)
		})
	}
}
