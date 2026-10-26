package notices

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// CollectGo returns the modules linked into the backend binary and the license
// text shipped in each module directory. `go list -deps .` is deliberately
// narrower than `./...`: test-only packages and their dependencies do not reach
// the distributed binary and must not be advertised as shipped.
func CollectGo(ctx context.Context, moduleDir string) ([]Dependency, error) {
	cmd := exec.CommandContext(ctx, "go", "list", "-deps", "-json", ".")
	cmd.Dir = moduleDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("go list: %w", err)
	}
	out, hasStdlib, parseErr := parseGoList(stdout)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return nil, parseErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("go list: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	if hasStdlib {
		stdlib, err := collectGoStandardLibrary(ctx, moduleDir)
		if err != nil {
			return nil, err
		}
		out = append(out, stdlib)
	}
	return out, nil
}

type goPackage struct {
	Module   *goModule
	Standard bool
}

type goModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *goModule
}

func parseGoList(r io.Reader) ([]Dependency, bool, error) {
	dec := json.NewDecoder(r)
	seen := map[string]bool{}
	var deps []Dependency
	hasStdlib := false
	for {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, false, fmt.Errorf("decode go list output: %w", err)
		}
		if pkg.Module == nil {
			if pkg.Standard {
				hasStdlib = true
			}
			continue
		}
		module := *pkg.Module
		if module.Replace != nil {
			module = *module.Replace
		}
		if module.Main {
			continue
		}
		key := module.Path + "@" + module.Version
		if seen[key] || module.Dir == "" {
			continue
		}
		seen[key] = true
		text, filename, err := moduleLicenseText(module.Dir)
		if err != nil {
			return nil, false, fmt.Errorf("module %s: %w", key, err)
		}
		license := strings.Join(DetectLicenses(text), ", ")
		if license == "" {
			license = filename
		}
		deps = append(deps, Dependency{
			Ecosystem: "go",
			Name:      module.Path,
			Version:   module.Version,
			License:   license,
			Source:    "https://pkg.go.dev/" + module.Path + "@" + module.Version,
			Text:      text,
		})
	}
	return deps, hasStdlib, nil
}

// collectGoStandardLibrary synthesises the one attribution entry for the Go
// standard library compiled into the binary. The version comes from go.mod's
// toolchain directive (the same pin the Dockerfile is checked against), not
// the host's runtime, so the generated notice does not change with the
// machine running the generator. The license text is the toolchain's own
// $GOROOT/LICENSE.
func collectGoStandardLibrary(ctx context.Context, moduleDir string) (Dependency, error) {
	version, err := pinnedGoVersion(moduleDir)
	if err != nil {
		return Dependency{}, err
	}
	cmd := exec.CommandContext(ctx, "go", "env", "GOROOT")
	cmd.Dir = moduleDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return Dependency{}, fmt.Errorf("go env GOROOT: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	goroot := strings.TrimSpace(string(out))
	if goroot == "" {
		return Dependency{}, errors.New("go env GOROOT returned an empty path")
	}
	text, err := os.ReadFile(filepath.Join(goroot, "LICENSE")) // #nosec G304 -- goroot comes from the Go tool, not request input
	if err != nil {
		return Dependency{}, fmt.Errorf("read Go standard library license: %w", err)
	}
	return Dependency{
		Ecosystem: "go",
		Name:      "Go standard library",
		Version:   version,
		License:   "BSD-3-Clause",
		Source:    "https://go.dev/LICENSE",
		Text:      normalizeText(string(text)),
	}, nil
}

func pinnedGoVersion(moduleDir string) (string, error) {
	path := filepath.Join(moduleDir, "go.mod")
	data, err := os.ReadFile(path) // #nosec G304 -- moduleDir is the backend module directory, a build input
	if err != nil {
		return "", fmt.Errorf("read pinned Go toolchain: %w", err)
	}
	var goDirective string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		switch fields[0] {
		case "toolchain":
			if strings.HasPrefix(fields[1], "go") {
				return fields[1], nil
			}
		case "go":
			if goDirective == "" {
				goDirective = "go" + fields[1]
			}
		}
	}
	if goDirective != "" {
		return goDirective, nil
	}
	return "", fmt.Errorf("%s has no toolchain or go directive", path)
}

var licenseFileBases = map[string]bool{
	"license": true, "licence": true, "copying": true, "notice": true,
	"unlicense": true, "copyright": true,
}

var sourceExtensions = map[string]bool{
	".go": true, ".c": true, ".h": true, ".cc": true, ".cpp": true,
	".hpp": true, ".m": true, ".mm": true, ".rs": true, ".java": true,
	".kt": true, ".kts": true, ".scala": true, ".cs": true, ".rb": true,
	".py": true, ".js": true, ".jsx": true, ".ts": true, ".tsx": true,
	".php": true, ".swift": true, ".sh": true, ".bash": true, ".zsh": true,
	".fish": true, ".ps1": true, ".lua": true, ".pl": true, ".r": true,
	".sql": true, ".html": true, ".css": true, ".json": true, ".yaml": true,
	".yml": true, ".toml": true, ".xml": true, ".proto": true,
}

func isLicenseFile(name string) bool {
	lower := strings.ToLower(name)
	ext := filepath.Ext(lower)
	if sourceExtensions[ext] {
		return false
	}
	stem := strings.TrimSuffix(lower, ext)
	if licenseFileBases[stem] {
		return true
	}
	for base := range licenseFileBases {
		if strings.HasPrefix(stem, base+"-") || strings.HasPrefix(stem, base+"_") {
			return true
		}
	}
	return false
}

func moduleLicenseText(dir string) (string, string, error) {
	names, err := moduleLicenseFiles(dir)
	if err != nil {
		return "", "", err
	}
	if len(names) == 0 {
		return "", "", fmt.Errorf("no license file in %s", dir)
	}
	var b strings.Builder
	for i, name := range names {
		data, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- dir is a Go module cache directory from go list, name is a license file in it
		if err != nil {
			return "", "", err
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "%s\n\n%s\n", name, normalizeText(string(data)))
	}
	return b.String(), strings.Join(names, ", "), nil
}

func moduleLicenseFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() {
			// REUSE keeps the actual license texts in a top-level LICENSES/
			// directory (<SPDX-ID>.txt) instead of naming each file LICENSE*.
			if !strings.EqualFold(entry.Name(), "LICENSES") {
				continue
			}
			nested, err := os.ReadDir(filepath.Join(dir, entry.Name()))
			if err != nil {
				return nil, err
			}
			for _, licenseEntry := range nested {
				if licenseEntry.IsDir() || strings.HasPrefix(licenseEntry.Name(), ".") {
					continue
				}
				if sourceExtensions[strings.ToLower(filepath.Ext(licenseEntry.Name()))] {
					continue
				}
				names = append(names, filepath.Join(entry.Name(), licenseEntry.Name()))
			}
			continue
		}
		if isLicenseFile(entry.Name()) {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

type npmPackage struct {
	Licenses    flexString `json:"licenses"`
	LicenseFile string     `json:"licenseFile"`
	Path        string     `json:"path"`
	Publisher   string     `json:"publisher"`
	Repository  flexString `json:"repository"`
	Private     bool       `json:"private"`
}

// flexString accepts the string-or-array shape license-checker uses for
// licenses, and the string-or-object shape npm uses for repository.
type flexString string

func (f *flexString) UnmarshalJSON(data []byte) error {
	var text string
	if err := json.Unmarshal(data, &text); err == nil {
		*f = flexString(text)
		return nil
	}
	var values []string
	if err := json.Unmarshal(data, &values); err == nil {
		*f = flexString(strings.Join(values, " OR "))
		return nil
	}
	var object struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(data, &object); err == nil && object.URL != "" {
		*f = flexString(object.URL)
		return nil
	}
	return nil
}

// CollectNPM runs the pinned license-checker dev dependency and reads each
// package's license file. Production dependencies only: dev/build tooling does
// not ship in the container.
func CollectNPM(ctx context.Context, root string) ([]Dependency, error) {
	frontend := filepath.Join(root, "frontend")
	filesDir := filepath.Join(root, "thirdparty", "licenses")
	checker := filepath.Join(frontend, "node_modules", ".bin", "license-checker-rseidelsohn")
	if _, err := os.Stat(checker); err != nil {
		return nil, fmt.Errorf("license-checker-rseidelsohn not installed in %s (run yarn install --frozen-lockfile in frontend)", frontend)
	}
	cmd := exec.CommandContext(ctx, checker, "--production", "--json") // #nosec G204 -- checker is the pinned repo-local license-checker binary, not request input
	cmd.Dir = frontend
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("license-checker: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return parseNPM(out, frontend, filesDir)
}

func parseNPM(data []byte, frontend, filesDir string) ([]Dependency, error) {
	var packages map[string]npmPackage
	if err := json.Unmarshal(data, &packages); err != nil {
		return nil, fmt.Errorf("decode license-checker output: %w", err)
	}
	keys := make([]string, 0, len(packages))
	for key := range packages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var deps []Dependency
	for _, key := range keys {
		pkg := packages[key]
		name, version := splitNPMPackageKey(key)
		if pkg.Path != "" && samePath(pkg.Path, frontend) {
			continue
		}
		license := strings.TrimSpace(string(pkg.Licenses))
		if license == "" || strings.EqualFold(license, "UNLICENSED") {
			return nil, fmt.Errorf("npm package %s has no declared license", key)
		}
		text, err := npmLicenseText(pkg, license, filesDir)
		if err != nil {
			return nil, fmt.Errorf("npm package %s: %w", key, err)
		}
		source := strings.TrimSpace(string(pkg.Repository))
		if source == "" {
			source = "https://www.npmjs.com/package/" + name + "/v/" + version
		}
		deps = append(deps, Dependency{
			Ecosystem: "npm",
			Name:      name,
			Version:   version,
			License:   license,
			Source:    source,
			Text:      text,
		})
	}
	return deps, nil
}

func npmLicenseText(pkg npmPackage, license, filesDir string) (string, error) {
	if pkg.LicenseFile != "" {
		data, err := os.ReadFile(pkg.LicenseFile)
		if err != nil {
			return "", err
		}
		return normalizeText(string(data)), nil
	}
	id := firstLicenseID(license)
	data, err := os.ReadFile(filepath.Join(filesDir, id+".txt")) // #nosec G304 -- filesDir is the repo license corpus, id comes from a package's declared license
	if err != nil {
		return "", fmt.Errorf("no license file and no bundled text for %q (add thirdparty/licenses/%s.txt)", license, id)
	}
	return synthesizeCopyright(string(data), pkg.Publisher), nil
}

func firstLicenseID(license string) string {
	license = strings.TrimPrefix(strings.TrimSpace(license), "(")
	if i := strings.IndexAny(license, " )"); i >= 0 {
		license = license[:i]
	}
	return license
}

func synthesizeCopyright(text, holder string) string {
	holder = strings.TrimSpace(holder)
	if holder == "" {
		holder = "the copyright holders"
	}
	text = strings.ReplaceAll(text, "<year> <copyright holders>", holder)
	text = strings.ReplaceAll(text, "<year>", "")
	text = strings.ReplaceAll(text, "<copyright holders>", holder)
	text = strings.ReplaceAll(text, "<owner>", holder)
	text = strings.ReplaceAll(text, "<organization>", holder)
	if strings.Contains(strings.ToLower(text), "copyright") {
		return normalizeText(text)
	}
	return normalizeText("Copyright (c) " + holder + "\n\n" + text)
}

func splitNPMPackageKey(key string) (string, string) {
	if i := strings.LastIndex(key, "@"); i > 0 {
		return key[:i], key[i+1:]
	}
	return key, ""
}

func samePath(left, right string) bool {
	leftAbs, leftErr := filepath.Abs(left)
	rightAbs, rightErr := filepath.Abs(right)
	if leftErr == nil && rightErr == nil {
		return leftAbs == rightAbs
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
