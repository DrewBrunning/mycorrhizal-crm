package notices

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderIsDeterministicAndDeduplicatesLicenseTexts(t *testing.T) {
	deps := []Dependency{
		{Ecosystem: "npm", Name: "zeta", Version: "1.0.0", License: "MIT", Source: "https://example/zeta", Text: "shared text"},
		{Ecosystem: "go", Name: "example.org/alpha", Version: "v1.2.3", License: "Apache-2.0", Source: "https://pkg.go.dev/example.org/alpha@v1.2.3", Text: "shared text"},
	}
	first := Render(deps)
	second := Render([]Dependency{deps[1], deps[0]})
	if first != second {
		t.Fatalf("Render is not deterministic:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	if got := strings.Count(first, "shared text"); got != 1 {
		t.Fatalf("license text emitted %d times, want once", got)
	}
	if !strings.Contains(first, "`example.org/alpha`") || !strings.Contains(first, "`zeta`") {
		t.Fatalf("notice is missing a package:\n%s", first)
	}
	if !strings.HasSuffix(first, "\n") || strings.HasSuffix(first, "\n\n") {
		t.Fatalf("notice should end with exactly one newline, got %q", first[len(first)-4:])
	}
}

func TestDetectLicenses(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []string
	}{
		{"mit", "Permission is hereby granted, free of charge, to any person", []string{"MIT"}},
		{"apache", "Apache License\nVersion 2.0, January 2004", []string{"Apache-2.0"}},
		{"bsd3 not bsd2", "Redistributions of source code must retain ... Neither the name of the copyright holder", []string{"BSD-3-Clause"}},
		{"bsd2", "Redistributions of source code must retain ...", []string{"BSD-2-Clause"}},
		{"mpl", "Mozilla Public License Version 2.0", []string{"MPL-2.0"}},
		{"unknown", "Proprietary", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := DetectLicenses(tc.text)
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("DetectLicenses() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestModuleLicenseTextReadsAllTopLevelLicenseFiles(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"LICENSE":      "MIT text\r\n",
		"NOTICE":       "notice text",
		"README.md":    "readme",
		"sub/LICENSE":  "nested is ignored",
		"COPYING.txt":  "copying text",
		"licence":      "british spelling",
		"UNLICENSE":    "public domain",
		"NOTLICENSE.x": "ignored",
	} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	text, files, err := moduleLicenseText(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"COPYING.txt", "LICENSE", "NOTICE", "UNLICENSE", "licence"} {
		if !strings.Contains(files, want) {
			t.Fatalf("license file list %q is missing %q", files, want)
		}
	}
	for _, unwanted := range []string{"README", "NOTLICENSE", "nested"} {
		if strings.Contains(text, unwanted) {
			t.Fatalf("license text unexpectedly contains %q:\n%s", unwanted, text)
		}
	}
	if strings.Contains(text, "\r") {
		t.Fatal("license text should be normalized to LF")
	}
}

func TestModuleLicenseTextFailsWithoutALicenseFile(t *testing.T) {
	if _, _, err := moduleLicenseText(t.TempDir()); err == nil {
		t.Fatal("expected an error for a module with no license file")
	}
}

func TestParseGoList(t *testing.T) {
	input := `{"Module":{"Path":"mycorrhizal","Main":true}}
{"ImportPath":"a","Module":{"Path":"example.org/direct","Version":"v1.0.0","Dir":"DIRECT"}}
{"ImportPath":"b","Module":{"Path":"example.org/direct","Version":"v1.0.0","Dir":"DIRECT"}}
{"ImportPath":"c","Module":{"Path":"example.org/replaced","Version":"v1.0.0","Dir":"OLD","Replace":{"Path":"example.org/fork","Version":"v1.1.0","Dir":"REPLACED"}}}
{"ImportPath":"d"}
`
	dir := t.TempDir()
	for _, name := range []string{"DIRECT", "REPLACED"} {
		if err := os.MkdirAll(filepath.Join(dir, name), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, name, "LICENSE"), []byte("Permission is hereby granted, free of charge"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	input = strings.ReplaceAll(input, "DIRECT", filepath.Join(dir, "DIRECT"))
	input = strings.ReplaceAll(input, "REPLACED", filepath.Join(dir, "REPLACED"))

	deps, err := parseGoList(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2: %+v", len(deps), deps)
	}
	if deps[0].Name != "example.org/direct" || deps[0].License != "MIT" {
		t.Fatalf("unexpected first dep: %+v", deps[0])
	}
	if deps[1].Name != "example.org/fork" || deps[1].Version != "v1.1.0" {
		t.Fatalf("replacement was not applied: %+v", deps[1])
	}
}

func TestParseNPMUsesLicenseFileAndFallbackText(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join(root, "frontend")
	files := filepath.Join(root, "thirdparty", "licenses")
	if err := os.MkdirAll(frontend, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(files, 0o755); err != nil {
		t.Fatal(err)
	}
	licenseFile := filepath.Join(frontend, "LICENSE")
	if err := os.WriteFile(licenseFile, []byte("package license"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(files, "ISC.txt"), []byte("Permission to use, copy, modify, and/or distribute this software"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := `{
  "frontend@0.1.0": {"licenses":"UNLICENSED","path":"` + frontend + `","private":true},
  "@scope/pkg@1.2.3": {"licenses":["MIT"],"licenseFile":"` + licenseFile + `","repository":"https://example/repo"},
  "fallback@2.0.0": {"licenses":"ISC","publisher":"Somebody","path":"` + frontend + `/node_modules/fallback"}
}`
	deps, err := parseNPM([]byte(input), frontend, files)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 2 {
		t.Fatalf("got %d deps, want 2: %+v", len(deps), deps)
	}
	if deps[0].Name != "@scope/pkg" || deps[0].Version != "1.2.3" || deps[0].Text != "package license" {
		t.Fatalf("unexpected scoped dep: %+v", deps[0])
	}
	if !strings.Contains(deps[1].Text, "Copyright (c) Somebody") {
		t.Fatalf("fallback license text is missing the copyright holder: %q", deps[1].Text)
	}
}

func TestParseNPMRejectsUnknownLicense(t *testing.T) {
	_, err := parseNPM([]byte(`{"x@1.0.0":{"licenses":"","path":"/tmp/x"}}`), "/tmp/frontend", t.TempDir())
	if err == nil {
		t.Fatal("expected an error for an npm package with no declared license")
	}
}

func TestSplitNPMPackageKey(t *testing.T) {
	tests := map[string][2]string{
		"react@19.3.1":           {"react", "19.3.1"},
		"@scope/name@1.2.3":      {"@scope/name", "1.2.3"},
		"@scope/name":            {"@scope/name", ""},
		"name-with@-in-it@1.0.0": {"name-with@-in-it", "1.0.0"},
	}
	for key, want := range tests {
		name, version := splitNPMPackageKey(key)
		if name != want[0] || version != want[1] {
			t.Fatalf("splitNPMPackageKey(%q) = (%q, %q), want (%q, %q)", key, name, version, want[0], want[1])
		}
	}
}

func TestCollectGoRunsTheGoTool(t *testing.T) {
	root := t.TempDir()
	moduleDir := filepath.Join(root, "module")
	if err := os.MkdirAll(moduleDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(moduleDir, "LICENSE"), []byte("Apache License\nVersion 2.0"), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"printf '%s\\n' '{\"Module\":{\"Path\":\"example.org/lib\",\"Version\":\"v1.2.3\",\"Dir\":\"" + moduleDir + "\"}}'\n"
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	deps, err := CollectGo(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Name != "example.org/lib" || deps[0].License != "Apache-2.0" {
		t.Fatalf("unexpected deps: %+v", deps)
	}
}

func TestCollectGoReportsAFailingTool(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\necho 'module exploded' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := CollectGo(t.Context(), t.TempDir()); err == nil {
		t.Fatal("expected a go list failure")
	}
}

func TestCollectNPMRunsThePinnedChecker(t *testing.T) {
	root := t.TempDir()
	frontend := filepath.Join(root, "frontend")
	files := filepath.Join(root, "thirdparty", "licenses")
	binDir := filepath.Join(frontend, "node_modules", ".bin")
	for _, dir := range []string{binDir, files} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	licenseFile := filepath.Join(frontend, "LICENSE")
	if err := os.WriteFile(licenseFile, []byte("Permission is hereby granted, free of charge"), 0o644); err != nil {
		t.Fatal(err)
	}
	output := `{"pkg@1.0.0":{"licenses":"MIT","licenseFile":"` + licenseFile + `","path":"` + frontend + `/node_modules/pkg"}}`
	script := "#!/bin/sh\ncat <<'JSON'\n" + output + "\nJSON\n"
	if err := os.WriteFile(filepath.Join(binDir, "license-checker-rseidelsohn"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	deps, err := CollectNPM(t.Context(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].Name != "pkg" {
		t.Fatalf("unexpected deps: %+v", deps)
	}
}

func TestCollectNPMFailsWhenTheCheckerIsMissing(t *testing.T) {
	if _, err := CollectNPM(t.Context(), t.TempDir()); err == nil {
		t.Fatal("expected a missing-checker error")
	}
}

func TestCollectNPMReportsAFailingChecker(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "frontend", "node_modules", ".bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(binDir, "license-checker-rseidelsohn"), []byte("#!/bin/sh\necho 'checker exploded' >&2\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := CollectNPM(t.Context(), root); err == nil {
		t.Fatal("expected a checker failure")
	}
}

func TestFlexStringAcceptsTheNpmShapes(t *testing.T) {
	tests := map[string]string{
		`"MIT"`:                          "MIT",
		`["MIT","Apache-2.0"]`:           "MIT OR Apache-2.0",
		`{"url":"https://example/repo"}`: "https://example/repo",
		`null`:                           "",
	}
	for input, want := range tests {
		var got flexString
		if err := got.UnmarshalJSON([]byte(input)); err != nil {
			t.Fatalf("UnmarshalJSON(%s): %v", input, err)
		}
		if string(got) != want {
			t.Fatalf("UnmarshalJSON(%s) = %q, want %q", input, got, want)
		}
	}
}

func TestFirstLicenseID(t *testing.T) {
	tests := map[string]string{
		"MIT":                 "MIT",
		"(MIT OR Apache-2.0)": "MIT",
		"Apache-2.0 AND MIT":  "Apache-2.0",
		"  BSD-3-Clause  ":    "BSD-3-Clause",
	}
	for input, want := range tests {
		if got := firstLicenseID(input); got != want {
			t.Fatalf("firstLicenseID(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestSynthesizeCopyrightFillsPlaceholders(t *testing.T) {
	text := "MIT License\n\nCopyright (c) <year> <copyright holders>\n\nPermission is hereby granted, free of charge"
	got := synthesizeCopyright(text, "Ada Lovelace")
	if strings.Contains(got, "<year>") || strings.Contains(got, "<copyright holders>") {
		t.Fatalf("placeholders were not replaced: %q", got)
	}
	if !strings.Contains(got, "Copyright (c) Ada Lovelace") {
		t.Fatalf("copyright holder missing: %q", got)
	}
	prefixed := synthesizeCopyright("Permission is hereby granted, free of charge", "Grace Hopper")
	if !strings.HasPrefix(prefixed, "Copyright (c) Grace Hopper") {
		t.Fatalf("missing synthesized copyright line: %q", prefixed)
	}
}

func TestRenderWithoutDependenciesOmitsTheTables(t *testing.T) {
	out := Render(nil)
	if strings.Contains(out, "## Go modules") || strings.Contains(out, "## npm packages") {
		t.Fatalf("empty notice should not have dependency tables:\n%s", out)
	}
	if !strings.Contains(out, "## License texts") {
		t.Fatalf("notice header is missing:\n%s", out)
	}
}

func TestRenderSectionsAreIndependent(t *testing.T) {
	goOnly := Render([]Dependency{{Ecosystem: "go", Name: "g", Version: "v1", License: "MIT", Source: "s", Text: "text"}})
	if !strings.Contains(goOnly, "## Go modules") || strings.Contains(goOnly, "## npm packages") {
		t.Fatalf("go-only notice has the wrong sections:\n%s", goOnly)
	}
	npmOnly := Render([]Dependency{{Ecosystem: "npm", Name: "n", Version: "1", License: "MIT", Source: "s", Text: "text"}})
	if strings.Contains(npmOnly, "## Go modules") || !strings.Contains(npmOnly, "## npm packages") {
		t.Fatalf("npm-only notice has the wrong sections:\n%s", npmOnly)
	}
}

func TestParseGoListRejectsInvalidJSON(t *testing.T) {
	if _, err := parseGoList(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestParseNPMRejectsInvalidJSON(t *testing.T) {
	if _, err := parseNPM([]byte("{not json"), t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("expected a decode error")
	}
}

func TestModuleLicenseTextReportsReadFailures(t *testing.T) {
	dir := t.TempDir()
	if err := os.Symlink(filepath.Join(dir, "missing"), filepath.Join(dir, "LICENSE")); err != nil {
		t.Fatal(err)
	}
	if _, _, err := moduleLicenseText(dir); err == nil {
		t.Fatal("expected a read error")
	}
}

func TestNpmLicenseTextReportsAMissingLicenseFile(t *testing.T) {
	_, err := npmLicenseText(npmPackage{LicenseFile: filepath.Join(t.TempDir(), "missing")}, "MIT", t.TempDir())
	if err == nil {
		t.Fatal("expected a missing-file error")
	}
}

func TestNpmLicenseTextFailsWhenNoFallbackTextExists(t *testing.T) {
	_, err := npmLicenseText(npmPackage{Licenses: "Proprietary"}, "Proprietary", t.TempDir())
	if err == nil {
		t.Fatal("expected a missing-fallback error")
	}
}

func TestSynthesizeCopyrightWithoutAHolder(t *testing.T) {
	got := synthesizeCopyright("Permission is hereby granted, free of charge", "")
	if !strings.Contains(got, "the copyright holders") {
		t.Fatalf("expected the generic holder: %q", got)
	}
}

func TestFirstLicenseIDEmpty(t *testing.T) {
	if got := firstLicenseID(""); got != "" {
		t.Fatalf("firstLicenseID(\"\") = %q", got)
	}
}

func TestCollectGoReportsAMissingTool(t *testing.T) {
	t.Setenv("PATH", "")
	if _, err := CollectGo(t.Context(), t.TempDir()); err == nil {
		t.Fatal("expected a missing-go error")
	}
}

func TestCollectGoReportsUnparseableOutput(t *testing.T) {
	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\necho '{not json'\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := CollectGo(t.Context(), t.TempDir()); err == nil {
		t.Fatal("expected a parse error")
	}
}

func TestParseGoListReportsAModuleWithoutALicense(t *testing.T) {
	moduleDir := t.TempDir()
	input := `{"Module":{"Path":"example.org/nolicense","Version":"v1.0.0","Dir":"` + moduleDir + `"}}`
	if _, err := parseGoList(strings.NewReader(input)); err == nil {
		t.Fatal("expected a missing-license error")
	}
}

func TestParseGoListFallsBackToTheLicenseFilename(t *testing.T) {
	moduleDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(moduleDir, "LICENSE.custom"), []byte("Proprietary terms"), 0o644); err != nil {
		t.Fatal(err)
	}
	input := `{"Module":{"Path":"example.org/custom","Version":"v1.0.0","Dir":"` + moduleDir + `"}}`
	deps, err := parseGoList(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if len(deps) != 1 || deps[0].License != "LICENSE.custom" {
		t.Fatalf("unexpected dep: %+v", deps)
	}
}

func TestModuleLicenseTextReportsAnUnreadableDirectory(t *testing.T) {
	if _, _, err := moduleLicenseText(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("expected a directory error")
	}
}

func TestParseNPMReportsALicenseFileError(t *testing.T) {
	input := `{"pkg@1.0.0":{"licenses":"MIT","licenseFile":"/definitely/missing","path":"/tmp/pkg"}}`
	if _, err := parseNPM([]byte(input), t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("expected a license-file error")
	}
}

func TestRenderSortsByNameVersionAndLicenseText(t *testing.T) {
	deps := []Dependency{
		{Ecosystem: "go", Name: "b", Version: "v2", License: "MIT", Source: "s", Text: "second text"},
		{Ecosystem: "go", Name: "a", Version: "v2", License: "MIT", Source: "s", Text: "first text"},
		{Ecosystem: "go", Name: "a", Version: "v1", License: "MIT", Source: "s", Text: "third text"},
	}
	out := Render(deps)
	if strings.Index(out, "`a` | `v1`") > strings.Index(out, "`a` | `v2`") {
		t.Fatalf("same-name packages are not version-sorted:\n%s", out)
	}
	if strings.Count(out, "---") < 2 {
		t.Fatalf("multiple license texts were not separated:\n%s", out)
	}
}

func TestFlexStringRejectsAnUnsupportedShape(t *testing.T) {
	var got flexString
	if err := got.UnmarshalJSON([]byte("123")); err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}
