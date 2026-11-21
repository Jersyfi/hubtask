// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// A procedure is a skill: one folder under .agents/skills with a SKILL.md, where most coding tools
// look. One tool reads .claude/skills only, so every skill has a link of its name there
// (ADR-0106). A skill without its link is one that tool never offers, and nothing else would say.
func TestEverySkillHasItsFileAndItsLink(t *testing.T) {
	problems, skills := skillProblems("../..")
	for _, problem := range problems {
		t.Error(problem)
	}
	if skills == 0 {
		// A test that read nothing passes for the wrong reason.
		t.Fatal("no skill found under .agents/skills - the path is wrong")
	}
}

var (
	skillName        = regexp.MustCompile(`(?m)^name:\s*(\S+)\s*$`)
	skillDescription = regexp.MustCompile(`(?m)^description:\s*\S`)
)

// skillProblems holds every folder under .agents/skills to a SKILL.md whose front matter names the
// folder and describes the skill, and to a link of the same name under .claude/skills that
// resolves to the folder; and every entry under .claude/skills to a skill. It also returns how
// many skills it read.
func skillProblems(root string) ([]string, int) {
	skillsDir := filepath.Join(root, ".agents", "skills")
	linksDir := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return []string{fmt.Sprintf(".agents/skills is not readable: %v", err)}, 0
	}

	var problems []string
	skills := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir() {
			problems = append(problems, fmt.Sprintf(".agents/skills/%s is not a folder - a skill is a folder with a SKILL.md", entry.Name()))
			continue
		}
		name := entry.Name()
		skills[name] = true

		text, err := os.ReadFile(filepath.Join(skillsDir, name, "SKILL.md"))
		if err != nil {
			problems = append(problems, fmt.Sprintf(".agents/skills/%s has no SKILL.md", name))
		} else {
			front := frontMatter(string(text))
			if m := skillName.FindStringSubmatch(front); m == nil || m[1] != name {
				problems = append(problems, fmt.Sprintf(".agents/skills/%s/SKILL.md: its front matter does not name the skill %q - a tool finds a skill by the name its folder carries", name, name))
			}
			if !skillDescription.MatchString(front) {
				problems = append(problems, fmt.Sprintf(".agents/skills/%s/SKILL.md: its front matter has no description - it is what tells a worker when to use the skill", name))
			}
		}

		if problem := linkProblem(skillsDir, linksDir, name); problem != "" {
			problems = append(problems, problem)
		}
	}

	links, err := os.ReadDir(linksDir)
	if err != nil {
		problems = append(problems, fmt.Sprintf(".claude/skills is not readable: %v", err))
	}
	for _, link := range links {
		if !skills[link.Name()] {
			problems = append(problems, fmt.Sprintf(".claude/skills/%s links no skill - .agents/skills has no folder of that name", link.Name()))
		}
	}
	sort.Strings(problems)
	return problems, len(skills)
}

// frontMatter is the block between the first two lines of three hyphens.
func frontMatter(text string) string {
	rest, ok := strings.CutPrefix(text, "---\n")
	if !ok {
		return ""
	}
	front, _, ok := strings.Cut(rest, "\n---")
	if !ok {
		return ""
	}
	return front
}

// linkProblem holds .claude/skills/<name> to a relative link to the skill's folder. A checkout
// without symbolic links holds the link as a file with the target as its text; that counts, since
// the link is what the repository carries.
func linkProblem(skillsDir, linksDir, name string) string {
	want := "../../.agents/skills/" + name
	link := filepath.Join(linksDir, name)
	info, err := os.Lstat(link)
	if err != nil {
		return fmt.Sprintf(".claude/skills/%s is missing - link it to %s", name, want)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		text, err := os.ReadFile(link)
		if err != nil || strings.TrimSpace(string(text)) != want {
			return fmt.Sprintf(".claude/skills/%s is not a link to %s", name, want)
		}
		return ""
	}
	target, err := os.Readlink(link)
	if err != nil || filepath.ToSlash(target) != want {
		return fmt.Sprintf(".claude/skills/%s links %q, not %s", name, target, want)
	}
	resolved, err := os.Stat(link)
	folder, ferr := os.Stat(filepath.Join(skillsDir, name))
	if err != nil || ferr != nil || !os.SameFile(resolved, folder) {
		return fmt.Sprintf(".claude/skills/%s does not resolve to .agents/skills/%s", name, name)
	}
	return ""
}

// The check catches each way a skill goes missing: a scratch tree - never this one - holds one
// sound skill beside a folder without a file, a skill without a link, a link to another skill, a
// front matter with another name, and a link that links nothing.
func TestSkillsThatGoMissingAreCaught(t *testing.T) {
	root := t.TempDir()
	skill := func(name, front string) {
		dir := filepath.Join(root, ".agents", "skills", name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if front == "" {
			return
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("---\n"+front+"\n---\n\n# A skill\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := func(name, target string) {
		if err := os.MkdirAll(filepath.Join(root, ".claude", "skills"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(root, ".claude", "skills", name)); err != nil {
			t.Skipf("this file system makes no symbolic link: %v", err)
		}
	}

	skill("sound", "name: sound\ndescription: Does one thing.")
	link("sound", "../../.agents/skills/sound")
	skill("no-file", "")
	link("no-file", "../../.agents/skills/no-file")
	skill("no-link", "name: no-link\ndescription: Does one thing.")
	skill("wrong-link", "name: wrong-link\ndescription: Does one thing.")
	link("wrong-link", "../../.agents/skills/sound")
	skill("wrong-name", "name: another\ndescription: Does one thing.")
	link("wrong-name", "../../.agents/skills/wrong-name")
	link("orphan", "../../.agents/skills/orphan")

	problems, skills := skillProblems(root)
	if skills != 5 {
		t.Fatalf("want 5 skills read, got %d", skills)
	}
	got := strings.Join(problems, "\n")
	for _, want := range []string{
		".agents/skills/no-file has no SKILL.md",
		".claude/skills/no-link is missing",
		`.claude/skills/wrong-link links "../../.agents/skills/sound"`,
		`.agents/skills/wrong-name/SKILL.md: its front matter does not name the skill "wrong-name"`,
		".claude/skills/orphan links no skill",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("want a problem containing %q, got:\n%s", want, got)
		}
	}
	if strings.Contains(got, "sound ") || len(problems) != 5 {
		t.Errorf("want exactly the five planted problems, got %d:\n%s", len(problems), got)
	}
}
