package agentcontext

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func copySkillFixture(t *testing.T, root, name string) {
	t.Helper()
	src := filepath.Join("testdata", "skills", name)
	err := filepath.WalkDir(src, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		body, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		writeFile(t, root, filepath.ToSlash(filepath.Join(SkillsRoot, name, rel)), string(body))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestValidSkillFixturePasses(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	if got := findings(root); len(got) != 0 {
		t.Fatalf("valid skill fixture should pass:\n%s", strings.Join(got, "\n"))
	}
}

func TestSkillNameMustMatchDirectory(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "bad-name")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "frontmatter name \"not-the-directory\" must equal the directory name \"bad-name\"") {
		t.Fatalf("name mismatch not reported: %s", got)
	}
}

func TestSkillNameMustMatchPattern(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	writeFile(t, root, SkillsRoot+"/Bad_Name/SKILL.md", "---\nname: Bad_Name\ndescription: x\n---\n# Body\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "must match ^[a-z0-9]+(-[a-z0-9]+)*$") {
		t.Fatalf("invalid name not reported: %s", got)
	}
}

func TestSkillDescriptionIsRequired(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "missing-description")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "frontmatter description is required") {
		t.Fatalf("missing description not reported: %s", got)
	}
}

func TestSkillDescriptionMaximum(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	writeFile(t, root, SkillsRoot+"/long-desc/SKILL.md",
		"---\nname: long-desc\ndescription: "+strings.Repeat("x", 1025)+"\n---\n# Body\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "maximum is 1024") {
		t.Fatalf("over-long description not reported: %s", got)
	}
}

func TestSkillBodyLineBudget(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	writeFile(t, root, SkillsRoot+"/valid/SKILL.md",
		"---\nname: valid\ndescription: fixture\n---\nSee [the checklist](references/checklist.md).\n"+strings.Repeat("step\n", 150)+
			"\n## DoD\n\n```bash\ncd backend && go test ./database/... ./internal/dbtest/...\n```\n")
	if got := findings(root); len(got) != 0 {
		t.Fatalf("a 150-line body should pass: %s", strings.Join(got, "\n"))
	}

	root = validTree(t)
	copySkillFixture(t, root, "long-body")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "SKILL.md body exceeds hard maximum of 200 lines") {
		t.Fatalf("a 201-line body should fail: %s", got)
	}
}

func TestSkillFrontmatterErrors(t *testing.T) {
	for _, tc := range []struct{ name, body, want string }{
		{"missing", "# no frontmatter\n", "missing YAML frontmatter"},
		{"unclosed", "---\nname: unclosed\n", "missing closing ---"},
		{"bad-frontmatter", "unused", "invalid frontmatter"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := validTree(t)
			if tc.body == "unused" {
				copySkillFixture(t, root, tc.name)
			} else {
				writeFile(t, root, SkillsRoot+"/"+tc.name+"/SKILL.md", tc.body)
			}
			got := strings.Join(findings(root), "\n")
			if !strings.Contains(got, tc.want) {
				t.Fatalf("want %q in findings: %s", tc.want, got)
			}
		})
	}
}

func TestOrphanReferenceFails(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "orphan-reference")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "reference .agents/skills/orphan-reference/references/orphan.md is not linked from SKILL.md") {
		t.Fatalf("orphan reference not reported: %s", got)
	}
}

func TestMissingReferenceLinkFails(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "missing-reference")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "link target missing: references/missing.md") {
		t.Fatalf("missing reference link not reported: %s", got)
	}
}

func TestReferenceLineBudget(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "long-reference")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "reference exceeds hard maximum of 200 lines") {
		t.Fatalf("over-long reference not reported: %s", got)
	}
}

func TestSkillDirectoryWithoutSkillFileFails(t *testing.T) {
	root := validTree(t)
	if err := os.MkdirAll(filepath.Join(root, SkillsRoot, "empty"), 0o755); err != nil {
		t.Fatal(err)
	}
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, ".agents/skills/empty:1: skill directory has no SKILL.md") {
		t.Fatalf("empty skill directory not reported: %s", got)
	}
}

func TestDuplicateSkillAcrossRootsFails(t *testing.T) {
	root := validTree(t)
	copySkillFixture(t, root, "valid")
	if err := os.Remove(filepath.Join(root, ClaudeSkillsRoot)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ClaudeSkillsRoot), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, root, ClaudeSkillsRoot+"/valid/SKILL.md", "---\nname: valid\ndescription: copy\n---\n# Body\n")
	got := strings.Join(findings(root), "\n")
	if !strings.Contains(got, "duplicate skill name \"valid\"") {
		t.Fatalf("duplicate skill not reported: %s", got)
	}
}

func TestClaudeSkillsAlias(t *testing.T) {
	t.Run("absent alias fails", func(t *testing.T) {
		root := validTree(t)
		if err := os.Remove(filepath.Join(root, ClaudeSkillsRoot)); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(findings(root), "\n")
		if !strings.Contains(got, "required alias is missing") {
			t.Fatalf("absent .claude/skills not reported: %s", got)
		}
	})
	t.Run("correct symlink passes", func(t *testing.T) {
		root := validTree(t)
		copySkillFixture(t, root, "valid")
		if got := findings(root); len(got) != 0 {
			t.Fatalf("correct alias should pass: %s", strings.Join(got, "\n"))
		}
	})
	t.Run("wrong target fails", func(t *testing.T) {
		root := validTree(t)
		if err := os.Remove(filepath.Join(root, ClaudeSkillsRoot)); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink("../.agents/other", filepath.Join(root, ClaudeSkillsRoot)); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(findings(root), "\n")
		if !strings.Contains(got, "alias resolves to ../.agents/other, want ../.agents/skills") {
			t.Fatalf("wrong alias target not reported: %s", got)
		}
	})
	t.Run("regular file fails", func(t *testing.T) {
		root := validTree(t)
		if err := os.Remove(filepath.Join(root, ClaudeSkillsRoot)); err != nil {
			t.Fatal(err)
		}
		writeFile(t, root, ClaudeSkillsRoot, "not a symlink\n")
		got := strings.Join(findings(root), "\n")
		if !strings.Contains(got, "must be a symlink to ../.agents/skills") {
			t.Fatalf("regular-file alias not reported: %s", got)
		}
	})
	t.Run("broken target fails", func(t *testing.T) {
		root := validTree(t)
		if err := os.Remove(filepath.Join(root, ".agents", "skills")); err != nil {
			t.Fatal(err)
		}
		got := strings.Join(findings(root), "\n")
		if !strings.Contains(got, "alias target missing: ../.agents/skills") {
			t.Fatalf("broken alias target not reported: %s", got)
		}
	})
}
