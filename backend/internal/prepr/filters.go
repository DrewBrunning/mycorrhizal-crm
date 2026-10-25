package prepr

import (
	"fmt"
	"io"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Filters is the parsed .github/filters.yaml: one named filter per area, each
// holding the repo-relative globs dorny/paths-filter matches against the
// changed-path list. Order is preserved for stable output.
type Filters struct {
	Names []string
	Globs map[string][]string
}

// LoadFilters parses the path-filter map CI consumes. A non-list value or a
// malformed document is an error rather than a silent empty area: an unreadable
// filters.yaml must stop the gate, not make it report "nothing changed".
func LoadFilters(r io.Reader) (*Filters, error) {
	var doc yaml.Node
	if err := yaml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("parse .github/filters.yaml: %w", err)
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf(".github/filters.yaml: expected a top-level mapping of area -> glob list")
	}

	mapping := doc.Content[0]
	f := &Filters{Globs: map[string][]string{}}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		key, value := mapping.Content[i], mapping.Content[i+1]
		if key.Kind != yaml.ScalarNode {
			return nil, fmt.Errorf(".github/filters.yaml: filter name at line %d is not a scalar", key.Line)
		}
		if value.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf(".github/filters.yaml: filter %q at line %d is not a list", key.Value, key.Line)
		}
		globs := make([]string, 0, len(value.Content))
		for _, item := range value.Content {
			if item.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf(".github/filters.yaml: filter %q has a non-string glob at line %d", key.Value, item.Line)
			}
			globs = append(globs, item.Value)
		}
		f.Names = append(f.Names, key.Value)
		f.Globs[key.Value] = globs
	}
	if len(f.Names) == 0 {
		return nil, fmt.Errorf(".github/filters.yaml: no filters declared")
	}
	return f, nil
}

// AreaNames returns the filter names in file order.
func (f *Filters) AreaNames() []string {
	out := make([]string, len(f.Names))
	copy(out, f.Names)
	return out
}

// Select returns the sorted names of every filter matching at least one
// changed path.
func (f *Filters) Select(changed []string) []string {
	matched := map[string]bool{}
	for name, globs := range f.Globs {
		for _, path := range changed {
			if matchesAny(globs, path) {
				matched[name] = true
				break
			}
		}
	}
	out := make([]string, 0, len(matched))
	for name := range matched {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// MatchingPaths returns the changed paths that match one filter, sorted.
func (f *Filters) MatchingPaths(name string, changed []string) []string {
	var out []string
	for _, path := range changed {
		if matchesAny(f.Globs[name], path) {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

func matchesAny(globs []string, path string) bool {
	for _, pattern := range globs {
		if Match(pattern, path) {
			return true
		}
	}
	return false
}
