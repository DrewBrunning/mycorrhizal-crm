package compatci

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// TestWebErrorReasonPassthroughAllowlist is the issue #2920 item 2 ratchet for
// errors.reason.* templates that only interpolate {{reason}}. Such a template
// cannot translate the server's English sentence, so every new one has to be
// a reviewed allowlist entry with a reason. The list only shrinks: a fixed
// template (or a removed key) fails until its entry is deleted.
func TestWebErrorReasonPassthroughAllowlist(t *testing.T) {
	allow := parsePassthroughAllowlist(t, "reason_passthrough_allowlist.txt")

	reasonsByLocale := map[string]map[string]string{}
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
		reasonsByLocale[lang] = doc.Errors.Reason
	}

	en := reasonsByLocale["en"]
	for _, problem := range passthroughProblems(en, allow) {
		t.Error(problem)
	}
	for key, text := range en {
		if strings.TrimSpace(text) != "{{reason}}" {
			continue
		}
		for _, lang := range webLocaleFiles {
			if strings.TrimSpace(reasonsByLocale[lang][key]) != "{{reason}}" {
				t.Errorf("%s.json: errors.reason.%s is a passthrough in en but not in this locale; keep the template a pure passthrough in all five locales", lang, key)
			}
		}
	}
}

// passthroughProblems is the ratchet itself, split out so its add/remove/stale
// branches have a table test over synthetic locale maps.
func passthroughProblems(en, allow map[string]string) []string {
	var problems []string
	seen := map[string]bool{}
	for key, text := range en {
		if strings.TrimSpace(text) != "{{reason}}" {
			continue
		}
		seen[key] = true
		if _, ok := allow[key]; !ok {
			problems = append(problems, fmt.Sprintf("en.json: errors.reason.%s is a new passthrough ({{reason}}) with no reasoned allowlist entry; give it a translated template or add a reason to reason_passthrough_allowlist.txt", key))
		}
	}
	for key := range allow {
		if !seen[key] {
			problems = append(problems, fmt.Sprintf("reason_passthrough_allowlist.txt: errors.reason.%s is no longer a passthrough in en.json; remove the stale allowlist entry", key))
		}
	}
	sort.Strings(problems)
	return problems
}

func TestPassthroughProblems(t *testing.T) {
	for _, tc := range []struct {
		name  string
		en    map[string]string
		allow map[string]string
		want  string
	}{
		{
			name: "new passthrough fails",
			en:   map[string]string{"new_key": "{{reason}}"},
			want: "new_key is a new passthrough",
		},
		{
			name:  "stale entry fails",
			en:    map[string]string{"fixed_key": "Translated: {{reason}}"},
			allow: map[string]string{"fixed_key": "was a passthrough"},
			want:  "fixed_key is no longer a passthrough",
		},
		{
			name:  "allowlisted passthrough passes",
			en:    map[string]string{"known_key": " {{reason}} "},
			allow: map[string]string{"known_key": "parser detail"},
		},
		{
			name: "a translated template is not a passthrough",
			en:   map[string]string{"fixed_key": "Translated: {{reason}}"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			problems := passthroughProblems(tc.en, tc.allow)
			if tc.want == "" {
				if len(problems) != 0 {
					t.Fatalf("passthroughProblems() = %v, want none", problems)
				}
				return
			}
			if len(problems) != 1 || !strings.Contains(problems[0], tc.want) {
				t.Fatalf("passthroughProblems() = %v, want one problem containing %q", problems, tc.want)
			}
		})
	}
}

func parsePassthroughAllowlist(t *testing.T, path string) map[string]string {
	t.Helper()
	allow := map[string]string{}
	count := -1
	for i, raw := range strings.Split(readRepoFile(t, path), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			rest, ok := strings.CutPrefix(line, "# count:")
			if !ok {
				continue
			}
			if _, err := fmt.Sscanf(strings.TrimSpace(rest), "%d", &count); err != nil {
				t.Fatalf("%s:%d: bad count header %q", path, i+1, line)
			}
			continue
		}
		key, reason, ok := strings.Cut(raw, "\t")
		if !ok {
			t.Fatalf("%s:%d: want key<TAB>reason, got %q", path, i+1, line)
		}
		key = strings.TrimSpace(key)
		reason = strings.TrimSpace(reason)
		if key == "" || reason == "" {
			t.Fatalf("%s:%d: key and reason are both required, got %q", path, i+1, line)
		}
		if _, duplicate := allow[key]; duplicate {
			t.Fatalf("%s:%d: duplicate entry for %s", path, i+1, key)
		}
		allow[key] = reason
	}
	if count < 0 {
		t.Fatalf("%s: missing # count: header", path)
	}
	if count != len(allow) {
		t.Fatalf("%s: count header says %d but the allowlist has %d entries", path, count, len(allow))
	}
	return allow
}
