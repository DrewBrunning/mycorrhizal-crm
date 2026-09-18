// Client/server compatibility floor coupling (issue #944).
//
// docs/client-compatibility-policy.md used to claim, in its supported
// matrix, that "every released Android build and every web client remain
// compatible with every released server version" — and
// docs/versioning-policy.md repeated it. That is false below v0.6.0:
// released servers exist back to v0.1.0-alpha-candidate, but the Android app
// refuses any server under its SERVER_BASELINE and the backend refuses to
// migrate a pre-v0.6.0 database. The policy now qualifies the promise to
// servers >= v0.6.0; this test is the drift guard that keeps the matrix's
// lower bound, the app's baseline, and the backend's migration floor from
// silently diverging again.
//
// It parses the doc's matrix cell and the two Kotlin constants as text (the
// same "parse the real source of truth, don't trust prose" shape as
// ci_claims_test.go and version_test.go) and compares them to
// database.SupportedUpgradeFloorTag, the code's own migration floor.
package compatci

import (
	"fmt"
	"os"
	"regexp"
	"testing"

	"mycorrhizal/database"
)

const clientCompatDocRel = "../../../docs/client-compatibility-policy.md"

// The two Android constants that state the app's server baseline. Both are
// 0.6.0 today; the test pins them equal to each other and to the doc/backend
// floor so neither can quietly become a second source of truth.
const (
	serverFeatureKtRel      = "../../../android/core/domain/src/main/kotlin/com/mycorrhizal/crm/domain/compat/ServerFeature.kt"
	serverCapabilitiesKtRel = "../../../android/core/domain/src/main/kotlin/com/mycorrhizal/crm/domain/compat/ServerCapabilities.kt"
)

// matrixServerLowerBoundPattern captures the lower bound of the supported
// client/server matrix's "server version range" column, e.g.
//
//	| **`v0.6.0` and later** | *(none declared)* | ... |
//
// -> "v0.6.0". A revert to an unqualified "all releases" row fails to match,
// which fails the test loudly rather than letting the false claim return.
var matrixServerLowerBoundPattern = regexp.MustCompile("\\|\\s*\\*\\*`(v[0-9]+\\.[0-9]+\\.[0-9]+)` and later\\*\\*")

// androidAppVersionConstant reads a Kotlin file and returns the AppVersion
// assigned to the named constant, formatted as a "vX.Y.Z" tag. It fails the
// test if the constant or its format changes out from under the regex.
func androidAppVersionConstant(t *testing.T, path, name string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	pattern := regexp.MustCompile(
		regexp.QuoteMeta(name) + `[^=]*=\s*AppVersion\(\s*(\d+)\s*,\s*(\d+)\s*,\s*(\d+)\s*\)`)
	m := pattern.FindStringSubmatch(string(data))
	if m == nil {
		t.Fatalf("%s: found no `%s = AppVersion(...)` assignment -- did the constant get renamed or "+
			"reformatted? update the pattern in client_compat_floor_test.go", path, name)
	}
	return fmt.Sprintf("v%s.%s.%s", m[1], m[2], m[3])
}

// TestClientCompatibilityMatrixMatchesServerFloors is the issue #944
// regression guard: the matrix's lower bound must equal both the Android
// app's server baseline and the backend's supported-upgrade floor, so the
// three cannot drift apart.
func TestClientCompatibilityMatrixMatchesServerFloors(t *testing.T) {
	doc, err := os.ReadFile(clientCompatDocRel)
	if err != nil {
		t.Fatalf("reading %s: %v", clientCompatDocRel, err)
	}
	m := matrixServerLowerBoundPattern.FindSubmatch(doc)
	if m == nil {
		t.Fatalf("%s: found no `| **`vX.Y.Z` and later** |` row -- the supported matrix must state "+
			"its server lower bound, not an unqualified \"all releases\" claim (issue #944); update "+
			"the pattern in client_compat_floor_test.go if the row was reformatted", clientCompatDocRel)
	}
	matrixLowerBound := string(m[1])

	if matrixLowerBound != database.SupportedUpgradeFloorTag {
		t.Errorf("client/server matrix floor drift: %s states the server lower bound as %q, but the "+
			"backend migration floor is %q (database.SupportedUpgradeFloorTag, issue #529) -- raising "+
			"or lowering either means updating BOTH",
			clientCompatDocRel, matrixLowerBound, database.SupportedUpgradeFloorTag)
	}

	featureBaseline := androidAppVersionConstant(t, serverFeatureKtRel, "SERVER_BASELINE")
	if featureBaseline != matrixLowerBound {
		t.Errorf("Android server baseline drift: ServerFeature.SERVER_BASELINE is %q, but %s states "+
			"the compatible server lower bound as %q -- they must be the same version (issue #944)",
			featureBaseline, clientCompatDocRel, matrixLowerBound)
	}

	capabilityBaseline := androidAppVersionConstant(t, serverCapabilitiesKtRel, "MIN_SUPPORTED_SERVER_VERSION")
	if capabilityBaseline != matrixLowerBound {
		t.Errorf("Android server baseline drift: ServerCapabilities.MIN_SUPPORTED_SERVER_VERSION is %q, "+
			"but %s states the compatible server lower bound as %q -- they must be the same version "+
			"(issue #944)", capabilityBaseline, clientCompatDocRel, matrixLowerBound)
	}
}
