package pragmacheck

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// CheckTree walks backendDir for non-test Go files and frontendSrcDir (if
// non-empty) for frontend source files, returning every marker with no
// discoverable reason, sorted by path then line.
func CheckTree(backendDir, frontendSrcDir string) ([]Finding, error) {
	var findings []Finding

	if backendDir != "" {
		f, err := checkGoTree(backendDir)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}
	if frontendSrcDir != "" {
		f, err := checkTSTree(frontendSrcDir)
		if err != nil {
			return nil, err
		}
		findings = append(findings, f...)
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].Path != findings[j].Path {
			return findings[i].Path < findings[j].Path
		}
		return findings[i].Line < findings[j].Line
	})
	return findings, nil
}

func checkGoTree(dir string) ([]Finding, error) {
	var findings []Finding
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// #nosec G304 G122 -- path comes from filepath.Walk over a caller-supplied root, not request input; this is a read-only lint check, not a security boundary a symlink swap could defeat meaningfully
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, CheckGoSource(path, string(b))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}

var frontendExts = map[string]bool{
	".ts": true, ".tsx": true, ".js": true, ".jsx": true,
}

func checkTSTree(dir string) ([]Finding, error) {
	var findings []Finding
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		if !frontendExts[filepath.Ext(path)] {
			return nil
		}
		// #nosec G304 G122 -- path comes from filepath.Walk over a caller-supplied root, not request input; this is a read-only lint check, not a security boundary a symlink swap could defeat meaningfully
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		findings = append(findings, CheckTSSource(path, string(b))...)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return findings, nil
}
