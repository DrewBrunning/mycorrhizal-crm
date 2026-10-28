package compatci

import (
	"encoding/json"
	"regexp"
	"sort"
	"testing"

	apperrors "mycorrhizal/errors"
)

const webEnLocaleRel = "../../../frontend/src/i18n/locales/en.json"

// TestWebErrorReasonsMatchBackend pins the web client's errors.reason.* strings
// (frontend/src/i18n/locales/en.json) to apperrors.ReasonCodes() (issue #2096,
// ADR 0038): every registered code needs a translation key whose {{x}}
// placeholder set equals the registry's params, and a key the registry lacks is
// dead UI. The four other locales are held to en's shape by locales.test.ts.
func TestWebErrorReasonsMatchBackend(t *testing.T) {
	var en struct {
		Errors struct {
			Reason map[string]string `json:"reason"`
		} `json:"errors"`
	}
	if err := json.Unmarshal([]byte(readRepoFile(t, webEnLocaleRel)), &en); err != nil {
		t.Fatalf("parsing en.json: %v", err)
	}

	placeholder := regexp.MustCompile(`\{\{\s*(\w+)\s*\}\}`)
	registered := map[string]bool{}
	for _, code := range apperrors.ReasonCodes() {
		registered[string(code)] = true
		_, params, _ := apperrors.ReasonSpecFor(code)
		tpl, ok := en.Errors.Reason[string(code)]
		if !ok {
			t.Errorf("en.json has no errors.reason.%s", code)
			continue
		}
		var got []string
		seen := map[string]bool{}
		for _, m := range placeholder.FindAllStringSubmatch(tpl, -1) {
			if !seen[m[1]] {
				seen[m[1]] = true
				got = append(got, m[1])
			}
		}
		want := append([]string(nil), params...)
		sort.Strings(got)
		sort.Strings(want)
		if len(got) != len(want) {
			t.Errorf("errors.reason.%s placeholders %v != registry params %v", code, got, want)
			continue
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("errors.reason.%s placeholders %v != registry params %v", code, got, want)
				break
			}
		}
	}
	for key := range en.Errors.Reason {
		if !registered[key] {
			t.Errorf("en.json errors.reason.%s is not in apperrors.ReasonCodes()", key)
		}
	}
}
