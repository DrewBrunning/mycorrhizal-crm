package reasoncode

import (
	"bufio"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
)

// ParseAllowlist reads `path:count` lines (blank lines and # comments are
// ignored). Counts, not line numbers, so unrelated edits do not churn it.
func ParseAllowlist(r io.Reader) (map[string]int, error) {
	out := map[string]int{}
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		i := strings.LastIndex(line, ":")
		if i < 0 {
			return nil, fmt.Errorf("allowlist line %d: want path:count, got %q", n, line)
		}
		path := line[:i]
		count, err := strconv.Atoi(line[i+1:])
		if err != nil || count < 1 {
			return nil, fmt.Errorf("allowlist line %d: count must be a positive integer, got %q", n, line[i+1:])
		}
		if _, dup := out[path]; dup {
			return nil, fmt.Errorf("allowlist line %d: duplicate entry for %s", n, path)
		}
		out[path] = count
	}
	return out, sc.Err()
}

// WriteAllowlist writes counts in the format ParseAllowlist reads, sorted.
func WriteAllowlist(w io.Writer, counts map[string]int) error {
	var b strings.Builder
	b.WriteString("# Unmigrated user-facing apperrors sites per file (issue #2098). A ratchet:\n")
	b.WriteString("# counts may only fall. Regenerate with: go run ./cmd/reasoncodecheck -write-allowlist ./...\n")
	for _, p := range sortedKeys(counts) {
		fmt.Fprintf(&b, "%s:%d\n", p, counts[p])
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// Compare returns one problem per file whose count differs from the allowlist:
// above the entry (or absent) is a new unmigrated error; below it means the
// entry must be lowered so the gain cannot silently regress.
func Compare(counts, allow map[string]int) []string {
	var problems []string
	for _, p := range sortedKeys(counts) {
		got := counts[p]
		want, ok := allow[p]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s: %d site(s) without a reason code and the file is not in the allowlist; chain .WithReason(...) (ADR 0038)", p, got))
		case got > want:
			problems = append(problems, fmt.Sprintf("%s: %d site(s) without a reason code, above the allowlist's %d; chain .WithReason(...) on the new ones (ADR 0038)", p, got, want))
		case got < want:
			problems = append(problems, fmt.Sprintf("%s: %d site(s) remain but the allowlist says %d; lower it to %d (regenerate with -write-allowlist)", p, got, want, got))
		}
	}
	for _, p := range sortedKeys(allow) {
		if _, ok := counts[p]; !ok {
			problems = append(problems, fmt.Sprintf("%s: fully migrated but still in the allowlist (%d); lower it by removing the entry (regenerate with -write-allowlist)", p, allow[p]))
		}
	}
	return problems
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
