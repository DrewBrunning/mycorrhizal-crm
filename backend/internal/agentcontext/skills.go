package agentcontext

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"mycorrhizal/internal/mdref"

	yaml "go.yaml.in/yaml/v3"
)

var skillNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// frontmatter is the subset of SKILL.md YAML the validator understands.
type frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	License     string `yaml:"license"`
}

// checkSkills validates the skill tree: the .claude/skills alias, the skill
// contract, and reference integrity (issues #1956/#1957). The alias and the
// skill tree are both required now that #1957 has created them.
func checkSkills(root string) ([]Skill, []mdref.Finding) {
	var findings []mdref.Finding
	findings = append(findings, checkClaudeSkillsAlias(root)...)

	skills, skillFindings := discoverSkills(root, SkillsRoot)
	findings = append(findings, skillFindings...)

	// A materialized .claude/skills (not a symlink) is a copy, not an alias.
	// Scan it only to report duplicate names; the alias check above already
	// fails it.
	if claudeSkillsIsRealDir(root) {
		cloned, cloneFindings := discoverSkills(root, ClaudeSkillsRoot)
		findings = append(findings, cloneFindings...)
		seen := map[string]string{}
		for _, s := range skills {
			seen[s.Name] = s.File
		}
		for _, s := range cloned {
			if first, ok := seen[s.Name]; ok {
				findings = append(findings, mdref.Finding{
					File:    s.File,
					Line:    1,
					Message: fmt.Sprintf("duplicate skill name %q (also %s); .claude/skills must be an alias, not a copy", s.Name, first),
				})
				continue
			}
			seen[s.Name] = s.File
		}
	}
	return skills, findings
}

func claudeSkillsIsRealDir(root string) bool {
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(ClaudeSkillsRoot)))
	return err == nil && info.IsDir()
}

func checkClaudeSkillsAlias(root string) []mdref.Finding {
	rel := ClaudeSkillsRoot
	abs := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(abs)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []mdref.Finding{{File: rel, Line: 1,
				Message: fmt.Sprintf("required alias is missing; create a symlink to %s", ClaudeSkillsLink)}}
		}
		return []mdref.Finding{{File: rel, Line: 1, Message: fmt.Sprintf("unreadable: %v", err)}}
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return []mdref.Finding{{
			File:    rel,
			Line:    1,
			Message: fmt.Sprintf("must be a symlink to %s (a file or real directory is not an alias)", ClaudeSkillsLink),
		}}
	}
	link, err := os.Readlink(abs)
	if err != nil {
		return []mdref.Finding{{File: rel, Line: 1, Message: fmt.Sprintf("unreadable symlink: %v", err)}}
	}
	target := filepath.Clean(filepath.Join(filepath.Dir(abs), filepath.FromSlash(link)))
	want := filepath.Clean(filepath.Join(root, filepath.FromSlash(SkillsRoot)))
	if target != want {
		return []mdref.Finding{{
			File:    rel,
			Line:    1,
			Message: fmt.Sprintf("alias resolves to %s, want %s", filepath.ToSlash(link), ClaudeSkillsLink),
		}}
	}
	if _, err := os.Stat(target); err != nil {
		return []mdref.Finding{{
			File:    rel,
			Line:    1,
			Message: fmt.Sprintf("alias target missing: %s", filepath.ToSlash(link)),
		}}
	}
	return nil
}

// discoverSkills parses every directory under skillsRoot that contains a
// SKILL.md. The returned skills carry repo-relative file paths.
func discoverSkills(root, skillsRoot string) ([]Skill, []mdref.Finding) {
	absRoot := filepath.Join(root, filepath.FromSlash(skillsRoot))
	entries, err := os.ReadDir(absRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, []mdref.Finding{{File: skillsRoot, Line: 1, Message: fmt.Sprintf("unreadable: %v", err)}}
	}
	var skills []Skill
	var findings []mdref.Finding
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dirRel := skillsRoot + "/" + e.Name()
		fileRel := dirRel + "/SKILL.md"
		abs := filepath.Join(root, filepath.FromSlash(fileRel))
		body, err := os.ReadFile(abs) // #nosec G304 -- discovered under the fixed skills root, never request input
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				findings = append(findings, mdref.Finding{File: dirRel, Line: 1, Message: "skill directory has no SKILL.md"})
				continue
			}
			findings = append(findings, mdref.Finding{File: fileRel, Line: 1, Message: fmt.Sprintf("unreadable: %v", err)})
			continue
		}
		skill, skillFindings := validateSkill(e.Name(), fileRel, string(body), root)
		findings = append(findings, skillFindings...)
		if skill != nil {
			skills = append(skills, *skill)
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, findings
}

func validateSkill(dirName, fileRel, text, root string) (*Skill, []mdref.Finding) {
	var findings []mdref.Finding
	fm, body, err := splitFrontmatter(text)
	if err != nil {
		return nil, []mdref.Finding{{File: fileRel, Line: 1, Message: err.Error()}}
	}
	if fm.Name != dirName {
		findings = append(findings, mdref.Finding{
			File: fileRel, Line: 1,
			Message: fmt.Sprintf("frontmatter name %q must equal the directory name %q", fm.Name, dirName),
		})
	}
	if !skillNameRe.MatchString(fm.Name) {
		findings = append(findings, mdref.Finding{
			File: fileRel, Line: 1,
			Message: fmt.Sprintf("name %q must match %s", fm.Name, skillNameRe.String()),
		})
	}
	desc := strings.TrimSpace(fm.Description)
	if desc == "" {
		findings = append(findings, mdref.Finding{File: fileRel, Line: 1, Message: "frontmatter description is required (a trigger, 1-1024 characters)"})
	} else if utf8.RuneCountInString(desc) > 1024 {
		findings = append(findings, mdref.Finding{
			File: fileRel, Line: 1,
			Message: fmt.Sprintf("frontmatter description is %d characters; the maximum is 1024", utf8.RuneCountInString(desc)),
		})
	}
	if n := lineCount([]byte(body)); n > maxLines {
		findings = append(findings, mdref.Finding{
			File: fileRel, Line: maxLines + 1,
			Message: fmt.Sprintf("SKILL.md body exceeds hard maximum of %d lines (%d)", maxLines, n),
		})
	}

	skillDir := filepath.Dir(filepath.FromSlash(fileRel))
	references := discoverReferences(filepath.Join(root, filepath.FromSlash(skillDir)), filepath.ToSlash(skillDir))
	for _, ref := range references {
		base := filepath.Base(filepath.FromSlash(ref))
		if !strings.Contains(text, "references/"+base) {
			findings = append(findings, mdref.Finding{
				File: fileRel, Line: 1,
				Message: fmt.Sprintf("reference %s is not linked from SKILL.md", ref),
			})
			continue
		}
		refBody, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(ref))) // #nosec G304 -- path comes from the skill's own references directory
		if err != nil {
			findings = append(findings, mdref.Finding{File: ref, Line: 1, Message: fmt.Sprintf("unreadable: %v", err)})
			continue
		}
		if n := lineCount(refBody); n > maxLines {
			findings = append(findings, mdref.Finding{
				File: ref, Line: maxLines + 1,
				Message: fmt.Sprintf("reference exceeds hard maximum of %d lines (%d)", maxLines, n),
			})
		}
	}
	return &Skill{Name: dirName, File: fileRel, References: references}, findings
}

// discoverReferences returns the repo-relative .md files under a skill's
// references/ directory. A missing directory is simply no references.
func discoverReferences(skillAbs, skillRelDir string) []string {
	dir := filepath.Join(skillAbs, "references")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		out = append(out, filepath.ToSlash(filepath.Join(skillRelDir, "references", e.Name())))
	}
	sort.Strings(out)
	return out
}

// splitFrontmatter returns the parsed frontmatter and the markdown body after
// the closing --- delimiter.
func splitFrontmatter(text string) (frontmatter, string, error) {
	var fm frontmatter
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return fm, "", errors.New("missing YAML frontmatter delimited by --- lines")
	}
	rest := normalized[len("---\n"):]
	idx := strings.Index(rest, "\n---")
	if idx < 0 {
		return fm, "", errors.New("missing closing --- frontmatter delimiter")
	}
	header := rest[:idx]
	body := rest[idx+len("\n---"):]
	body = strings.TrimPrefix(body, "\n")
	if err := yaml.Unmarshal([]byte(header), &fm); err != nil {
		return fm, "", fmt.Errorf("invalid frontmatter: %v", err)
	}
	return fm, body, nil
}
