package compatci

import (
	"regexp"
	"sort"
	"testing"

	apperrors "mycorrhizal/errors"
)

// TestAndroidErrorReasonsMatchBackend pins the Android client's hand-mirrored
// ErrorReasonCode (frontend trap #4) and its error_reason_<code> strings to
// apperrors.ReasonCodes() (issue #2097, ADR 0038): a code the app does not know
// shows the server's English, and a code it knows that the server never sends
// is dead UI. The web copy is pinned by TestWebErrorReasonsMatchBackend.
func TestAndroidErrorReasonsMatchBackend(t *testing.T) {
	src := readRepoFile(t, androidTreeRel+"/core/model/src/main/kotlin/com/mycorrhizal/crm/model/network/ErrorReason.kt")
	body := regexp.MustCompile(`(?s)object ErrorReasonCode \{(.*?)\n\}`).FindStringSubmatch(src)
	if body == nil {
		t.Fatal("object ErrorReasonCode not found in ErrorReason.kt")
	}
	var android []string
	for _, m := range regexp.MustCompile(`const val [A-Z_]+ = "([a-z_0-9]+)"`).FindAllStringSubmatch(body[1], -1) {
		android = append(android, m[1])
	}
	var backend []string
	for _, c := range apperrors.ReasonCodes() {
		backend = append(backend, string(c))
	}
	sort.Strings(android)
	sort.Strings(backend)
	if len(android) != len(backend) {
		t.Fatalf("Android ErrorReasonCode has %d codes, backend ReasonCodes() has %d:\nandroid=%v\nbackend=%v", len(android), len(backend), android, backend)
	}
	for i := range backend {
		if android[i] != backend[i] {
			t.Fatalf("Android ErrorReasonCode differs from ReasonCodes():\nandroid=%v\nbackend=%v", android, backend)
		}
	}

	strings := readRepoFile(t, androidTreeRel+"/core/ui/src/main/res/values/strings.xml")
	positional := regexp.MustCompile(`%(\d+)\$`)
	for _, code := range apperrors.ReasonCodes() {
		_, params, _ := apperrors.ReasonSpecFor(code)
		m := regexp.MustCompile(`<string name="error_reason_` + string(code) + `"[^>]*>(.*?)</string>`).FindStringSubmatch(strings)
		if m == nil {
			t.Errorf("values/strings.xml has no error_reason_%s", code)
			continue
		}
		seen := map[string]bool{}
		for _, p := range positional.FindAllStringSubmatch(m[1], -1) {
			seen[p[1]] = true
		}
		if len(seen) != len(params) {
			t.Errorf("error_reason_%s uses %d positional args, registry declares %d (%v)", code, len(seen), len(params), params)
		}
		for i := range params {
			if !seen[string(rune('1'+i))] {
				t.Errorf("error_reason_%s is missing positional arg %%%d$ (registry order %v)", code, i+1, params)
			}
		}
	}
}
