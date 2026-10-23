package compatci

import (
	"regexp"
	"sort"
	"testing"

	"mycorrhizal/internal/scoring"
)

// TestAndroidScoreReasonCodesMatchBackend pins the Android client's
// hand-mirrored score reason codes (frontend trap #4) to scoring.ReasonCodes():
// a code the app does not know renders the English fallback, and a code it
// knows that the server never sends is dead UI. The web copy is pinned to the
// spec in frontend/src/api/contractConformance.ts.
func TestAndroidScoreReasonCodesMatchBackend(t *testing.T) {
	src := readRepoFile(t, androidTreeRel+"/core/model/src/main/kotlin/com/mycorrhizal/crm/model/network/ContactScore.kt")
	body := regexp.MustCompile(`(?s)object ScoreReasonCode \{(.*?)\n\}`).FindStringSubmatch(src)
	if body == nil {
		t.Fatal("object ScoreReasonCode not found in ContactScore.kt")
	}
	var android []string
	for _, m := range regexp.MustCompile(`const val [A-Z_]+ = "([a-z_]+)"`).FindAllStringSubmatch(body[1], -1) {
		android = append(android, m[1])
	}
	backend := append([]string(nil), scoring.ReasonCodes()...)
	sort.Strings(android)
	sort.Strings(backend)
	if len(android) != len(backend) {
		t.Fatalf("Android ScoreReasonCode has %d codes, backend scoring.ReasonCodes() has %d:\nandroid=%v\nbackend=%v", len(android), len(backend), android, backend)
	}
	for i := range backend {
		if android[i] != backend[i] {
			t.Fatalf("Android ScoreReasonCode differs from scoring.ReasonCodes():\nandroid=%v\nbackend=%v", android, backend)
		}
	}
}
