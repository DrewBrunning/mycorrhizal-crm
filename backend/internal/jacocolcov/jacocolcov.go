// Package jacocolcov converts the Android app's aggregated JaCoCo XML report
// into lcov, the format Codecov's `android` flag is uploaded in.
//
// Why lcov and not the JaCoCo XML directly: since 2026-10-16 Codecov's
// processing ends every JaCoCo upload for this repository in state `error`
// (no detail, no incident on its status page), so codecov/patch/android
// never reports and every Android PR needs a bypass. The probe in PR #1748
// isolated it to the format: a 13 KB single-package JaCoCo report errors
// under any flag, network filter, file-fix or endpoint setting, while the
// same coverage converted to lcov merges. lcov is also the format this
// repo's frontend already uploads, and the conversion resolves every file to
// a real repository path instead of leaving Codecov to guess one from
// `<package>/<sourcefile>`.
//
// The JaCoCo XML stays the source of truth: the per-file ratchet
// (internal/androidcoverage) reads it unchanged, and CI still uploads it to
// Codecov under the non-gating `android-jacoco` flag so the two can be
// compared once Codecov processes JaCoCo again (docs/development/coverage.md).
//
// Conversion rules:
//   - A JaCoCo `<line>` becomes `DA:<nr>,<1 if ci > 0 else 0>`. JaCoCo has
//     no execution counts, only covered/missed instructions, so 1/0 loses
//     nothing a line-based patch gate reads.
//   - A line's branch counters become BRDA records: one per covered branch
//     (taken 1) and one per missed branch (taken 0, or "-" when the line
//     itself never ran), so Codecov still marks partially-covered branch
//     lines as partial.
//   - Instruction-level partials (some instructions of a branchless line
//     missed) have no lcov form; such a line is covered. This is the one
//     fidelity loss, and Reconcile reports how many lines it affects.
//   - A `<package>/<sourcefile>` key is resolved to the single repository file
//     whose path ends with it. Keys with no match (generated code such as
//     Hilt factories, Kotlin stdlib inlined into our packages) or with no
//     `<line>` elements are dropped; keys matching more than one file are
//     dropped and reported as ambiguous rather than guessed.
package jacocolcov

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
)

// Line is one JaCoCo `<line>` element: instruction (MI/CI) and branch (MB/CB)
// counters for source line Nr.
type Line struct {
	Nr int `xml:"nr,attr"`
	MI int `xml:"mi,attr"`
	CI int `xml:"ci,attr"`
	MB int `xml:"mb,attr"`
	CB int `xml:"cb,attr"`
}

// SourceFile is one JaCoCo `<sourcefile>`, keyed "<package>/<name>".
type SourceFile struct {
	Key   string
	Lines []Line
}

type report struct {
	XMLName  xml.Name `xml:"report"`
	Packages []struct {
		Name        string `xml:"name,attr"`
		SourceFiles []struct {
			Name  string `xml:"name,attr"`
			Lines []Line `xml:"line"`
		} `xml:"sourcefile"`
	} `xml:"package"`
}

// ParseReport reads a JaCoCo XML report into its source files, in document
// order.
func ParseReport(r io.Reader) ([]SourceFile, error) {
	var rep report
	if err := xml.NewDecoder(r).Decode(&rep); err != nil {
		return nil, fmt.Errorf("parsing JaCoCo XML: %w", err)
	}
	var files []SourceFile
	for _, pkg := range rep.Packages {
		for _, sf := range pkg.SourceFiles {
			files = append(files, SourceFile{Key: pkg.Name + "/" + sf.Name, Lines: sf.Lines})
		}
	}
	return files, nil
}

// skippedDirs are never source roots: build output and tool caches can hold
// generated copies of a source path, which would make real files ambiguous.
var skippedDirs = map[string]bool{"build": true, ".gradle": true, ".idea": true, "node_modules": true}

// IndexSources walks dir (a subtree of root) for .kt and .java files and maps
// every path suffix of each one ("File.kt", "pkg/File.kt", ...,
// "android/.../pkg/File.kt") to the root-relative, slash-separated paths that
// end with it.
func IndexSources(fsys fs.FS, dir string) (map[string][]string, error) {
	index := map[string][]string{}
	err := fs.WalkDir(fsys, dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skippedDirs[d.Name()] {
				return fs.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext != ".kt" && ext != ".java" {
			return nil
		}
		parts := strings.Split(path, "/")
		for i := range parts {
			suffix := strings.Join(parts[i:], "/")
			index[suffix] = append(index[suffix], path)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("indexing %s: %w", dir, err)
	}
	return index, nil
}

// Totals counts lines (`<line>` elements / DA records), lines with at least
// one covered instruction, branches and covered branches.
type Totals struct {
	Files           int
	Lines           int
	CoveredLines    int
	Branches        int
	CoveredBranches int
}

func (t *Totals) add(f SourceFile) {
	t.Files++
	for _, l := range f.Lines {
		t.Lines++
		if l.CI > 0 {
			t.CoveredLines++
		}
		t.Branches += l.MB + l.CB
		t.CoveredBranches += l.CB
	}
}

// Result reconciles what the JaCoCo report held against what the lcov output
// carries.
type Result struct {
	JaCoCo Totals // every source file in the report
	LCOV   Totals // the source files written as lcov records

	Unresolved []string // keys with no repository file (generated/inlined code)
	NoLines    []string // keys whose sourcefile has no <line> elements
	Ambiguous  []string // keys matching more than one repository file

	// InstructionPartials counts written lines with both missed and covered
	// instructions but no branches: partial in JaCoCo, covered in lcov.
	InstructionPartials int
}

// Convert writes files as lcov to w, resolving each key through index, and
// returns the reconciliation. Records are written sorted by path so the
// output is deterministic.
func Convert(files []SourceFile, index map[string][]string, w io.Writer) (Result, error) {
	var res Result
	type record struct {
		path string
		file SourceFile
	}
	var records []record
	for _, f := range files {
		res.JaCoCo.add(f)
		matches := index[f.Key]
		switch {
		case len(f.Lines) == 0:
			res.NoLines = append(res.NoLines, f.Key)
		case len(matches) == 0:
			res.Unresolved = append(res.Unresolved, f.Key)
		case len(matches) > 1:
			res.Ambiguous = append(res.Ambiguous, f.Key)
		default:
			records = append(records, record{path: matches[0], file: f})
		}
	}
	sort.Slice(records, func(i, j int) bool { return records[i].path < records[j].path })

	var buf bytes.Buffer
	for _, rec := range records {
		res.LCOV.add(rec.file)
		writeRecord(&buf, rec.path, rec.file, &res)
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return res, fmt.Errorf("writing lcov: %w", err)
	}
	return res, nil
}

func writeRecord(w *bytes.Buffer, path string, f SourceFile, res *Result) {
	fmt.Fprintf(w, "SF:%s\n", path)
	var lf, lh, brf, brh int
	for _, l := range f.Lines {
		hit := 0
		if l.CI > 0 {
			hit = 1
			lh++
			if l.MI > 0 && l.MB+l.CB == 0 {
				res.InstructionPartials++
			}
		}
		lf++
		fmt.Fprintf(w, "DA:%d,%d\n", l.Nr, hit)
	}
	for _, l := range f.Lines {
		branch := 0
		for i := 0; i < l.CB; i++ {
			fmt.Fprintf(w, "BRDA:%d,0,%d,1\n", l.Nr, branch)
			branch++
		}
		missed := "0"
		if l.CI == 0 {
			missed = "-" // lcov: the branch's block never executed
		}
		for i := 0; i < l.MB; i++ {
			fmt.Fprintf(w, "BRDA:%d,0,%d,%s\n", l.Nr, branch, missed)
			branch++
		}
		brf += l.MB + l.CB
		brh += l.CB
	}
	fmt.Fprintf(w, "LF:%d\nLH:%d\n", lf, lh)
	if brf > 0 {
		fmt.Fprintf(w, "BRF:%d\nBRH:%d\n", brf, brh)
	}
	fmt.Fprintln(w, "end_of_record")
}

// Summary renders the reconciliation as Markdown (CI step summary / PR body).
func (r Result) Summary() string {
	var b strings.Builder
	pct := func(n, d int) string {
		if d == 0 {
			return "n/a"
		}
		return fmt.Sprintf("%.2f%%", 100*float64(n)/float64(d))
	}
	b.WriteString("| | Files | Lines | Covered lines | Line % | Branches | Covered branches | Branch % |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, row := range []struct {
		name string
		t    Totals
	}{{"JaCoCo report", r.JaCoCo}, {"lcov output", r.LCOV}} {
		fmt.Fprintf(&b, "| %s | %d | %d | %d | %s | %d | %d | %s |\n", row.name, row.t.Files, row.t.Lines,
			row.t.CoveredLines, pct(row.t.CoveredLines, row.t.Lines), row.t.Branches, row.t.CoveredBranches,
			pct(row.t.CoveredBranches, row.t.Branches))
	}
	fmt.Fprintf(&b, "\nDropped: %d unresolved (no repository file: generated/inlined code), %d with no lines, %d ambiguous.\n",
		len(r.Unresolved), len(r.NoLines), len(r.Ambiguous))
	fmt.Fprintf(&b, "Instruction-level partial lines counted as covered in lcov: %d.\n", r.InstructionPartials)
	return b.String()
}
