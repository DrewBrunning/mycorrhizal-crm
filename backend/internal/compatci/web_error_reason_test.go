package compatci

import (
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"testing"

	apperrors "mycorrhizal/errors"
)

// webLocaleFiles are the five locale files every web key must exist in
// (frontend/src/i18n/locales, CLAUDE.md "all five locale files").
var webLocaleFiles = []string{"en", "de", "es", "fr", "it"}

var webPlaceholder = regexp.MustCompile(`\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}`)

// TestWebErrorReasonsMatchBackendRegistry pins the web client's error reason
// strings (ADR 0038, issue #2096) to apperrors.ReasonCodes(): every registered
// code has an errors.reason.<code> string in every locale, its {{name}}
// placeholders are exactly the registry's params, and no errors.reason.* key
// exists that the registry does not declare.
func TestWebErrorReasonsMatchBackendRegistry(t *testing.T) {
	for _, lang := range webLocaleFiles {
		raw := readRepoFile(t, "../../../frontend/src/i18n/locales/"+lang+".json")
		var doc struct {
			Errors struct {
				Reason map[string]string `json:"reason"`
			} `json:"errors"`
		}
		if err := json.Unmarshal([]byte(raw), &doc); err != nil {
			t.Fatalf("%s.json: %v", lang, err)
		}
		reasons := doc.Errors.Reason

		declared := map[string]bool{}
		for _, code := range apperrors.ReasonCodes() {
			name := string(code)
			declared[name] = true
			tpl, params, ok := apperrors.ReasonSpecFor(code)
			if !ok {
				t.Fatalf("ReasonSpecFor(%s): not registered", name)
			}
			text, present := reasons[name]
			if !present {
				t.Errorf("%s.json: missing errors.reason.%s", lang, name)
				continue
			}
			want := append([]string(nil), params...)
			got := placeholderNames(text)
			sort.Strings(want)
			sort.Strings(got)
			if strings.Join(want, ",") != strings.Join(got, ",") {
				t.Errorf("%s.json: errors.reason.%s placeholders %v, registry params %v", lang, name, got, want)
			}
			if lang == "en" {
				// The English string is the template the server renders: with
				// {{name}} written back as {name}, it must equal the registry text.
				if normalized := webPlaceholder.ReplaceAllString(text, "{${1}}"); normalized != tpl {
					t.Errorf("en.json: errors.reason.%s is %q, registry template is %q", name, normalized, tpl)
				}
			}
		}
		for name := range reasons {
			if !declared[name] {
				t.Errorf("%s.json: errors.reason.%s is not in the backend registry", lang, name)
			}
		}
	}
}

func placeholderNames(s string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range webPlaceholder.FindAllStringSubmatch(s, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	sort.Strings(out)
	return out
}
