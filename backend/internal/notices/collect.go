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
	out, parseErr := parseGoList(stdout)
	waitErr := cmd.Wait()
	if parseErr != nil {
		return nil, parseErr
	}
	if waitErr != nil {
		return nil, fmt.Errorf("go list: %w: %s", waitErr, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

type goPackage struct {
	Module *goModule
}

type goModule struct {
	Path    string
	Version string
	Dir     string
	Main    bool
	Replace *goModule
}

func parseGoList(r io.Reader) ([]Dependency, error) {
	dec := json.NewDecoder(r)
	seen := map[string]bool{}
	var deps []Dependency
	for {
		var pkg goPackage
		if err := dec.Decode(&pkg); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("decode go list output: %w", err)
		}
		if pkg.Module == nil {
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
			return nil, fmt.Errorf("module %s: %w", key, err)
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
	return deps, nil
}

var licenseFilePrefixes = []string{
	"license", "licence", "copying", "notice", "unlicense", "copyright",
}

func isLicenseFile(name string) bool {
	lower := strings.ToLower(name)
	for _, prefix := range licenseFilePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func moduleLicenseText(dir string) (string, string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", "", err
	}
	var names []string
	for _, entry := range entries {
		if entry.IsDir() || !isLicenseFile(entry.Name()) {
			continue
		}
		names = append(names, entry.Name())
	}
	if len(names) == 0 {
		return "", "", fmt.Errorf("no license file in %s", dir)
	}
	sort.Strings(names)
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
