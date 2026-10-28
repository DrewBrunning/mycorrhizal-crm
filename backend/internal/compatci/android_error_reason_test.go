package compatci

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"

	apperrors "mycorrhizal/errors"
)

// androidStringsFiles are the five strings.xml files every error reason
// string must exist in (core/ui/src/main/res, one per locale).
var androidStringsFiles = []string{
	"values/strings.xml",
	"values-de/strings.xml",
	"values-es/strings.xml",
	"values-fr/strings.xml",
	"values-it/strings.xml",
}

var androidPositional = regexp.MustCompile(`%(\d+)\$[sd]`)

// TestAndroidErrorReasonsMatchBackendRegistry pins the Android client's error
// reason mirror (ADR 0038, issue #2097) to apperrors.ReasonCodes(): the
// ErrorReasonCode constants equal the registry, every code has an
// error_reason_<code> string in every locale whose positional arguments are
// exactly 1..len(params), and error_request_ref exists in every locale.
func TestAndroidErrorReasonsMatchBackendRegistry(t *testing.T) {
	src := readRepoFile(t, androidTreeRel+"/core/model/src/main/kotlin/com/mycorrhizal/crm/model/network/ErrorReason.kt")
	body := regexp.MustCompile(`(?s)object ErrorReasonCode \{(.*?)\n\}`).FindStringSubmatch(src)
	if body == nil {
		t.Fatal("object ErrorReasonCode not found in ErrorReason.kt")
	}
	var android []string
	for _, m := range regexp.MustCompile(`const val [A-Z_0-9]+ = "([a-z_0-9]+)"`).FindAllStringSubmatch(body[1], -1) {
		android = append(android, m[1])
	}
	var backend []string
	for _, c := range apperrors.ReasonCodes() {
		backend = append(backend, string(c))
	}
	sort.Strings(android)
	sort.Strings(backend)
	if strings.Join(android, ",") != strings.Join(backend, ",") {
		t.Fatalf("ErrorReasonCode differs from apperrors.ReasonCodes():\nandroid=%v\nbackend=%v", android, backend)
	}

	for _, file := range androidStringsFiles {
		xml := readRepoFile(t, androidTreeRel+"/core/ui/src/main/res/"+file)
		if !strings.Contains(xml, `<string name="error_request_ref">`) {
			t.Errorf("%s: missing error_request_ref", file)
		}
		for _, c := range apperrors.ReasonCodes() {
			name := "error_reason_" + string(c)
			text, ok := androidStringValue(xml, name)
			if !ok {
				t.Errorf("%s: missing <string name=%q>", file, name)
				continue
			}
			_, params, _ := apperrors.ReasonSpecFor(c)
			if got, want := positionalArgs(text), len(params); !sameArgs(got, want) {
				t.Errorf("%s: %s uses positional args %v, registry declares %d param(s)", file, name, got, want)
			}
		}
	}
}

func androidStringValue(xml, name string) (string, bool) {
	m := regexp.MustCompile(fmt.Sprintf(`<string name="%s">(.*?)</string>`, regexp.QuoteMeta(name))).FindStringSubmatch(xml)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// positionalArgs returns the distinct %N$ indices used in text, sorted.
func positionalArgs(text string) []int {
	seen := map[int]bool{}
	var out []int
	for _, m := range androidPositional.FindAllStringSubmatch(text, -1) {
		var n int
		fmt.Sscanf(m[1], "%d", &n)
		if !seen[n] {
			seen[n] = true
			out = append(out, n)
		}
	}
	sort.Ints(out)
	return out
}

// sameArgs reports whether got is exactly 1..want.
func sameArgs(got []int, want int) bool {
	if len(got) != want {
		return false
	}
	for i, n := range got {
		if n != i+1 {
			return false
		}
	}
	return true
}
