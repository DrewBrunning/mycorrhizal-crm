// Android build-toolchain migration guards (issue #1858).
//
// The Android build has three coupled invariants that the rest of the repo
// only states in comments: it compiles Kotlin through AGP's built-in Kotlin
// (never the standalone kotlin-android plugin), it analyses with detekt 2.x
// (the only line that accepts a JDK 25 build JVM), and the Android CI jobs
// actually run that JDK. Each is easy to undo accidentally in an unrelated
// build edit, and none fails until CI (or a contributor's machine) hits it,
// so these tests parse the real files as ground truth -- the same shape as
// ci_claims_test.go's workflow-vs-comment checks.
package compatci

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

const (
	androidGradlePropertiesRel = androidTreeRel + "/gradle.properties"
	androidCatalogRel          = androidTreeRel + "/gradle/libs.versions.toml"
	androidBuildLogicRel       = androidTreeRel + "/build-logic"
	androidDetektConfigRel     = androidTreeRel + "/config/detekt/detekt.yml"
	androidConfigKtRel         = androidBuildLogicRel +
		"/src/main/kotlin/com/mycorrhizal/crm/buildlogic/AndroidConfig.kt"
)

// deprecatedAgpOptOuts are gradle.properties keys AGP 9.3 warns are deprecated
// (and that AGP 10 removes). Keeping any of them -- even at their current
// default -- reintroduces the warning wall issue #1858 clears, and the
// builtInKotlin/newDsl pair additionally disables the migration itself.
var deprecatedAgpOptOuts = map[string]string{
	"android.usesSdkInManifest.disallowed":                       "default is now true",
	"android.sdk.defaultTargetSdkToCompileSdkIfUnset":            "default is now true",
	"android.enableAppCompileTimeRClass":                         "default is now true",
	"android.defaults.buildfeatures.resvalues":                   "default is now false",
	"android.r8.optimizedResourceShrinking":                      "default is now true",
	"android.builtInKotlin":                                      "built-in Kotlin is the only mode in AGP 10",
	"android.newDsl":                                             "the legacy variant API is gone in AGP 10",
	"android.dependency.excludeLibraryComponentsFromConstraints": "deprecated; use android.dependency.useConstraints=false",
}

func readRepoFile(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Clean(rel))
	if err != nil {
		t.Fatalf("reading %s: %v", rel, err)
	}
	return string(data)
}

// TestAndroidBuildHasNoDeprecatedAgpOptOuts fails if gradle.properties sets any
// property AGP 9.3 marked deprecated. It also pins the one deliberate
// suppression: the build keeps project dependency constraints (disabling them
// silently downgrades inter-module versions, see TestAndroidDependencyConstraintsStayOn),
// so the advisory import-performance warning must stay suppressed in its place.
func TestAndroidBuildHasNoDeprecatedAgpOptOuts(t *testing.T) {
	props := readRepoFile(t, androidGradlePropertiesRel)
	seen := make(map[string]string)
	for _, line := range strings.Split(props, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		key = strings.TrimSpace(key)
		seen[key] = strings.TrimSpace(value)
		if reason, deprecated := deprecatedAgpOptOuts[key]; deprecated {
			t.Errorf("%s sets deprecated AGP property %q (%s); remove it (issue #1858)", androidGradlePropertiesRel, key, reason)
		}
	}

	if seen["android.dependency.useConstraints"] != "true" {
		t.Fatalf("%s must keep android.dependency.useConstraints=true -- disabling project "+
			"constraints drops inter-module version alignment (issue #1858)",
			androidGradlePropertiesRel)
	}
	if seen["android.generateSyncIssueWhenLibraryConstraintsAreEnabled"] != "false" {
		t.Errorf("%s keeps library constraints but does not set "+
			"android.generateSyncIssueWhenLibraryConstraintsAreEnabled=false; AGP will print the "+
			"library-constraints warning on every configuration (issue #1858)",
			androidGradlePropertiesRel)
	}
}

// TestAndroidModulesUseBuiltInKotlin fails if any build script or the version
// catalog reintroduces the standalone kotlin-android plugin. AGP 9's
// built-in Kotlin replaces it, and applying both fails configuration outright
// ("an extension already registered with that name").
func TestAndroidModulesUseBuiltInKotlin(t *testing.T) {
	forbidden := []struct {
		pattern *regexp.Regexp
		where   string
	}{
		{regexp.MustCompile(`pluginManager\.apply\("org\.jetbrains\.kotlin\.android"\)`), "build-logic plugin application"},
		{regexp.MustCompile(`id\("org\.jetbrains\.kotlin\.android"\)`), "module plugins {} block"},
		{regexp.MustCompile(`alias\(libs\.plugins\.kotlin\.android\)`), "version-catalog alias use"},
		{regexp.MustCompile(`(?m)^\s*kotlin-android\s*=\s*\{\s*id\s*=\s*"org\.jetbrains\.kotlin\.android"`), "version catalog declaration"},
	}

	err := filepath.WalkDir(androidTreeRel, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if skipAndroidDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".kt") && !strings.HasSuffix(path, ".kts") && !strings.HasSuffix(path, ".toml") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, f := range forbidden {
			if f.pattern.Match(data) {
				t.Errorf("%s reintroduces the standalone kotlin-android plugin (%s); AGP 9 "+
					"built-in Kotlin is the supported path (issue #1858)", path, f.where)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", androidTreeRel, err)
	}

	// Built-in Kotlin derives kotlin.compilerOptions.jvmTarget from
	// android.compileOptions.targetCompatibility; the detekt pin must stay in
	// sync with it or detekt analyses a different bytecode target than the app.
	config := readRepoFile(t, androidConfigKtRel)
	if !regexp.MustCompile(`targetCompatibility\s*=\s*JavaVersion\.VERSION_17`).MatchString(config) {
		t.Errorf("%s no longer sets compileOptions.targetCompatibility = JavaVersion.VERSION_17; "+
			"built-in Kotlin no longer defaults jvmTarget to the documented target (issue #1858)", androidConfigKtRel)
	}
	if !regexp.MustCompile(`jvmTarget\.set\("17"\)`).MatchString(config) {
		t.Errorf("%s no longer pins detekt's jvmTarget to 17, which must match "+
			"compileOptions.targetCompatibility (issue #1858)", androidConfigKtRel)
	}
}

// TestAndroidDetektUpgradeIsConsistent parses the catalog, build-logic and
// convention plugin to prove they all name the same detekt 2.x version/id, and
// checks the YAML for the two rule renames the 2.0 migration required. A
// partial upgrade (plugin id changed but version left on 1.23.8, or a stale
// rule name) otherwise fails only when Gradle runs.
func TestAndroidDetektUpgradeIsConsistent(t *testing.T) {
	catalog := readRepoFile(t, androidCatalogRel)

	versionMatch := regexp.MustCompile(`(?m)^\s*detekt\s*=\s*"([^"]+)"`).FindStringSubmatch(catalog)
	if versionMatch == nil {
		t.Fatalf("%s no longer declares a detekt version", androidCatalogRel)
	}
	version := versionMatch[1]
	major, err := strconv.Atoi(strings.SplitN(version, ".", 2)[0])
	if err != nil || major < 2 {
		t.Fatalf("detekt version %q is not a 2.x release; only detekt 2.x supports the "+
			"JDK 25 build JVM this project runs (issue #1858)", version)
	}
	if !regexp.MustCompile(`(?m)^\s*detekt\s*=\s*\{\s*id\s*=\s*"dev\.detekt"`).MatchString(catalog) {
		t.Errorf("%s must declare the detekt plugin as dev.detekt (the 2.0 plugin id), not the "+
			"1.x io.gitlab.arturbosch.detekt id (issue #1858)", androidCatalogRel)
	}

	buildLogic := readRepoFile(t, androidBuildLogicRel+"/build.gradle.kts")
	logicMatch := regexp.MustCompile(`implementation\("dev\.detekt:detekt-gradle-plugin:([^"]+)"\)`).
		FindStringSubmatch(buildLogic)
	if logicMatch == nil {
		t.Fatalf("%s/build.gradle.kts no longer puts dev.detekt:detekt-gradle-plugin on the "+
			"convention-plugin classpath", androidBuildLogicRel)
	}
	if logicMatch[1] != version {
		t.Errorf("detekt version drift: catalog has %s, build-logic has %s -- keep them in sync "+
			"(issue #1858)", version, logicMatch[1])
	}

	config := readRepoFile(t, androidConfigKtRel)
	for _, want := range []string{
		`pluginManager.apply("dev.detekt")`,
		`dev.detekt.gradle.extensions.DetektExtension`,
		`dev.detekt.gradle.Detekt`,
	} {
		if !strings.Contains(config, want) {
			t.Errorf("%s no longer references %s; the detekt 2.x plugin API/package changed "+
				"(issue #1858)", androidConfigKtRel, want)
		}
	}
	if strings.Contains(config, "io.gitlab.arturbosch.detekt") {
		t.Errorf("%s still references the detekt 1.x package; the plugin id and task/extension "+
			"packages all moved in 2.0 (issue #1858)", androidConfigKtRel)
	}

	// detekt 2.0 renamed style.MayBeConst -> style.MayBeConstant and split the
	// single ExplicitItLambdaParameter into a single- and multiple-parameter
	// rule. The repo deliberately disables both of the latter (no style
	// enforcement), so both keys must be present and false.
	var detektConfig struct {
		Style map[string]struct {
			Active *bool `yaml:"active"`
		} `yaml:"style"`
	}
	if err := yaml.Unmarshal([]byte(readRepoFile(t, androidDetektConfigRel)), &detektConfig); err != nil {
		t.Fatalf("parsing %s: %v", androidDetektConfigRel, err)
	}
	if _, stale := detektConfig.Style["MayBeConst"]; stale {
		t.Errorf("%s still configures style.MayBeConst; detekt 2.0 renamed it to MayBeConstant "+
			"(issue #1858)", androidDetektConfigRel)
	}
	for _, rule := range []string{"ExplicitItLambdaParameter", "ExplicitItLambdaMultipleParameters"} {
		setting, ok := detektConfig.Style[rule]
		if !ok || setting.Active == nil || *setting.Active {
			t.Errorf("%s must set style.%s.active: false -- detekt 2.0 split the old rule in two "+
				"and this repo deliberately keeps both off (issue #1858)", androidDetektConfigRel, rule)
		}
	}
}

// TestAndroidCIRunsOnJDK25 parses android-tests.yml and requires every
// actions/setup-java step to request JDK 25, and the detekt step to live in a
// job that does. Before issue #1858 the workflow pinned JDK 17 solely because
// detekt 1.23.x rejected JDK > 22; reverting either half would bring that
// local/CI split back.
func TestAndroidCIRunsOnJDK25(t *testing.T) {
	text := readWorkflow(t, androidWorkflowRel)

	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Name string         `yaml:"name"`
				Uses string         `yaml:"uses"`
				With map[string]any `yaml:"with"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal([]byte(text), &workflow); err != nil {
		t.Fatalf("parsing %s: %v", androidWorkflowRel, err)
	}

	setupJavaJobs := 0
	detektJob := ""
	for jobID, job := range workflow.Jobs {
		requestsJDK25 := false
		for _, step := range job.Steps {
			if strings.HasPrefix(step.Uses, "actions/setup-java@") {
				setupJavaJobs++
				got := strings.TrimPrefix(strings.TrimSpace(toString(step.With["java-version"])), "v")
				want := "25"
				if !strings.HasPrefix(got, want+".") && got != want {
					t.Errorf("%s job %q requests java-version %v via setup-java; the Android build "+
						"runs on JDK 25 (issue #1858)", androidWorkflowRel, jobID, step.With["java-version"])
					continue
				}
				requestsJDK25 = true
			}
			if step.Name == "detekt" {
				detektJob = jobID
				if !requestsJDK25 {
					t.Errorf("%s job %q runs detekt without an actions/setup-java JDK 25 step "+
						"earlier in the job (issue #1858)", androidWorkflowRel, jobID)
				}
			}
		}
	}

	if setupJavaJobs == 0 {
		t.Fatalf("%s has no actions/setup-java step -- did the workflow shape change? "+
			"update this test along with it", androidWorkflowRel)
	}
	if detektJob == "" {
		t.Fatalf("%s no longer has a step named %q; the detekt gate must stay wired (issue #358)", androidWorkflowRel, "detekt")
	}
	if regexp.MustCompile(`detekt 1\.23|Requires\s*\n?\s*#\s*JDK 17`).MatchString(text) {
		t.Errorf("%s still carries the pre-#1858 detekt/JDK-17 caveat; update the comment with the "+
			"workflow (issue #1858)", androidWorkflowRel)
	}
}

func toString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case int:
		return strconv.Itoa(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}
