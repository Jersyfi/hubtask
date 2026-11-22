// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
	"time"
)

// A procedure is a skill: one folder under .agents/skills with a SKILL.md, where most coding tools
// look. One tool reads .claude/skills only, so every skill has a link of its name there
// (ADR-0106). A skill without its link is one that tool never offers, and nothing else would say.
func TestEverySkillHasItsFileAndItsLink(t *testing.T) {
	problems, skills := skillProblems("../..", gitLinkModes(t, "../.."))
	for _, problem := range problems {
		t.Error(problem)
	}
	if len(skills) == 0 {
		// A test that read nothing passes for the wrong reason.
		t.Fatal("no skill found under .agents/skills - the path is wrong")
	}
}

// The reviewer's wrapper names only skills that exist. That AGENTS.md lists every skill by its
// path is held by tools/checkdocs, which runs on a change to AGENTS.md alone.
func TestTheReviewersWrapperNamesSkillsThatExist(t *testing.T) {
	_, skills := skillProblems("../..", gitLinkModes(t, "../.."))

	wrappers, err := filepath.Glob("../../.claude/agents/*.md")
	if err != nil || len(wrappers) == 0 {
		t.Fatalf("no agent definition found under .claude/agents (%v) - the path is wrong", err)
	}
	for _, wrapper := range wrappers {
		text, err := os.ReadFile(wrapper)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range namedSkillProblems(".claude/agents/"+filepath.Base(wrapper), string(text), skills) {
			t.Error(problem)
		}
	}
}

// gitLinkModes reads the mode git holds for each entry under .claude/skills. Git's mode is the
// truth about a link: a checkout without symbolic links writes one as a file, and a file committed
// in a link's place looks the same there.
func gitLinkModes(t *testing.T, root string) map[string]string {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	out, err := exec.CommandContext(ctx, "git", "-C", root, "ls-files", "-s", "--", ".claude/skills").Output()
	if err != nil {
		t.Fatalf("git ls-files -s: %v", err)
	}
	return parseLinkModes(string(out))
}

// parseLinkModes reads the lines of `git ls-files -s` - "<mode> <object> <stage>\t<path>" - into
// the mode of each entry directly under .claude/skills. A path below an entry means git holds a
// folder there, which is no link either.
func parseLinkModes(listing string) map[string]string {
	modes := map[string]string{}
	for _, line := range strings.Split(listing, "\n") {
		meta, file, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		name, below, nested := strings.Cut(strings.TrimPrefix(file, ".claude/skills/"), "/")
		mode, _, _ := strings.Cut(meta, " ")
		if nested {
			mode = "a folder holding " + below
		}
		modes[name] = mode
	}
	return modes
}

// gitSymlink is the mode git gives a symbolic link.
const gitSymlink = "120000"

var namedSkill = regexp.MustCompile(`\.agents/skills/([a-z0-9][a-z0-9-]*)/SKILL\.md`)

// namedSkillProblems holds a document to the skills it names by path: each exists.
func namedSkillProblems(where, doc string, skills map[string]bool) []string {
	var problems []string
	named := map[string]bool{}
	for _, m := range namedSkill.FindAllStringSubmatch(doc, -1) {
		if !named[m[1]] && !skills[m[1]] {
			problems = append(problems, fmt.Sprintf("%s names %s, and .agents/skills has no skill of that name", where, m[0]))
		}
		named[m[1]] = true
	}
	sort.Strings(problems)
	return problems
}

var (
	skillName        = regexp.MustCompile(`(?m)^name:[ \t]*(\S+)[ \t]*$`)
	skillDescription = regexp.MustCompile(`(?m)^description:[ \t]*\S`)
)

// skillProblems holds every folder under .agents/skills to a SKILL.md whose front matter names the
// folder and describes the skill, and to a link of the same name under .claude/skills that git
// holds as a symbolic link and that resolves to the folder; and every entry under .claude/skills
// to a skill. modes is what git holds for the links (gitLinkModes). It also returns the skills it
// read.
func skillProblems(root string, modes map[string]string) ([]string, map[string]bool) {
	skillsDir := filepath.Join(root, ".agents", "skills")
	linksDir := filepath.Join(root, ".claude", "skills")
	entries, err := os.ReadDir(skillsDir)
	if err != nil {
		return []string{fmt.Sprintf(".agents/skills is not readable: %v", err)}, nil
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
			switch m := skillName.FindStringSubmatch(front); {
			case m == nil:
				problems = append(problems, fmt.Sprintf(".agents/skills/%s/SKILL.md: its front matter has no name - a tool finds a skill by the name its folder carries", name))
			case m[1] != name:
				problems = append(problems, fmt.Sprintf(".agents/skills/%s/SKILL.md: its front matter does not name the skill %q - a tool finds a skill by the name its folder carries", name, name))
			}
			if !skillDescription.MatchString(front) {
				problems = append(problems, fmt.Sprintf(".agents/skills/%s/SKILL.md: its front matter has no description - it is what tells a worker when to use the skill", name))
			}
		}

		if problem := linkProblem(skillsDir, linksDir, name, modes[name]); problem != "" {
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
	return problems, skills
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

// linkProblem holds .claude/skills/<name> to a relative link to the skill's folder, held by git
// as a symbolic link (mode is git's). A checkout without symbolic links holds the link as a file
// with the target as its text; that counts only because git's mode says it is a link.
func linkProblem(skillsDir, linksDir, name, mode string) string {
	want := "../../.agents/skills/" + name
	link := filepath.Join(linksDir, name)
	info, err := os.Lstat(link)
	if err != nil {
		return fmt.Sprintf(".claude/skills/%s is missing - link it to %s", name, want)
	}
	switch mode {
	case gitSymlink:
	case "":
		return fmt.Sprintf(".claude/skills/%s is not tracked by git - a link nobody committed reaches no other checkout", name)
	default:
		return fmt.Sprintf(".claude/skills/%s is no symbolic link in git (it holds %s, a link is %s) - the tool that reads .claude/skills follows a link, not a file", name, mode, gitSymlink)
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
// sound skill, and one more that is sound on a checkout without symbolic links, beside each
// planted case below.
func TestSkillsThatGoMissingAreCaught(t *testing.T) {
	root := t.TempDir()
	write := func(path, text string) {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sound := func(name string) string { return "name: " + name + "\ndescription: Does one thing." }
	target := func(name string) string { return "../../.agents/skills/" + name }

	cases := []struct {
		name string
		// front is the SKILL.md's front matter; "-" plants a folder without the file.
		front string
		// link is the target of the symbolic link under .claude/skills; file is the text of a
		// regular file planted there instead. Both empty: no entry.
		link, file string
		// mode is what git holds for the entry; empty means git does not track it.
		mode string
		// want is part of the one problem the case produces; empty means the case is sound.
		want string
	}{
		{name: "sound", front: sound("sound"), link: target("sound"), mode: gitSymlink},
		{name: "link-as-text", front: sound("link-as-text"), file: target("link-as-text") + "\n", mode: gitSymlink},
		{name: "no-file", front: "-", link: target("no-file"), mode: gitSymlink,
			want: ".agents/skills/no-file has no SKILL.md"},
		{name: "no-name", front: "description: Does one thing.", link: target("no-name"), mode: gitSymlink,
			want: ".agents/skills/no-name/SKILL.md: its front matter has no name"},
		{name: "wrong-name", front: sound("another"), link: target("wrong-name"), mode: gitSymlink,
			want: `.agents/skills/wrong-name/SKILL.md: its front matter does not name the skill "wrong-name"`},
		{name: "no-description", front: "name: no-description", link: target("no-description"), mode: gitSymlink,
			want: ".agents/skills/no-description/SKILL.md: its front matter has no description"},
		{name: "empty-description", front: "name: empty-description\ndescription:", link: target("empty-description"), mode: gitSymlink,
			want: ".agents/skills/empty-description/SKILL.md: its front matter has no description"},
		{name: "no-link", front: sound("no-link"),
			want: ".claude/skills/no-link is missing"},
		{name: "wrong-link", front: sound("wrong-link"), link: target("sound"), mode: gitSymlink,
			want: `.claude/skills/wrong-link links "../../.agents/skills/sound"`},
		{name: "file-in-git", front: sound("file-in-git"), file: target("file-in-git") + "\n", mode: "100644",
			want: ".claude/skills/file-in-git is no symbolic link in git (it holds 100644"},
		{name: "folder-in-git", front: sound("folder-in-git"), link: target("folder-in-git"), mode: "a folder holding SKILL.md",
			want: ".claude/skills/folder-in-git is no symbolic link in git (it holds a folder holding SKILL.md"},
		{name: "untracked", front: sound("untracked"), link: target("untracked"),
			want: ".claude/skills/untracked is not tracked by git"},
		{name: "other-text", front: sound("other-text"), file: "see the skills folder\n", mode: gitSymlink,
			want: ".claude/skills/other-text is not a link to ../../.agents/skills/other-text"},
		{name: "orphan", link: target("orphan"), mode: gitSymlink,
			want: ".claude/skills/orphan links no skill"},
	}

	modes := map[string]string{}
	for _, c := range cases {
		switch c.front {
		case "":
		case "-":
			if err := os.MkdirAll(filepath.Join(root, ".agents", "skills", c.name), 0o755); err != nil {
				t.Fatal(err)
			}
		default:
			write(filepath.Join(root, ".agents", "skills", c.name, "SKILL.md"), "---\n"+c.front+"\n---\n\n# A skill\n")
		}
		entry := filepath.Join(root, ".claude", "skills", c.name)
		switch {
		case c.link != "":
			if err := os.MkdirAll(filepath.Dir(entry), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(c.link, entry); err != nil {
				t.Skipf("this file system makes no symbolic link: %v", err)
			}
		case c.file != "":
			write(entry, c.file)
		}
		if c.mode != "" {
			modes[c.name] = c.mode
		}
	}
	// A loose file beside the skills' folders is no skill.
	write(filepath.Join(root, ".agents", "skills", "loose.md"), "# Not a skill\n")

	problems, skills := skillProblems(root, modes)
	got := strings.Join(problems, "\n")
	for _, c := range cases {
		var mine []string
		for _, problem := range problems {
			if strings.Contains(problem, "/"+c.name+" ") || strings.Contains(problem, "/"+c.name+"/") {
				mine = append(mine, problem)
			}
		}
		switch {
		case c.want == "" && len(mine) != 0:
			t.Errorf("%s: want no problem, got %q", c.name, mine)
		case c.want != "" && (len(mine) != 1 || !strings.Contains(mine[0], c.want)):
			t.Errorf("%s: want exactly one problem containing %q, got %q", c.name, c.want, mine)
		}
	}
	if !strings.Contains(got, ".agents/skills/loose.md is not a folder") {
		t.Errorf("want the loose file reported, got:\n%s", got)
	}
	if len(skills) != 13 {
		t.Errorf("want 13 skills read, got %d", len(skills))
	}
	// Twelve planted cases and the loose file; the two sound skills produce nothing.
	if len(problems) != 13 {
		t.Errorf("want exactly the thirteen planted problems, got %d:\n%s", len(problems), got)
	}
}

// What git lists is read by entry: a link's mode, and a folder where git holds files below it.
func TestLinkModesAreReadFromGitsListing(t *testing.T) {
	modes := parseLinkModes("120000 ec97ccdd9e5c2d708dd6e611d4b7238d704b48cb 0\t.claude/skills/a-link\n" +
		"100644 ffb1ec61c0c66e9773adadbabd6cf97853ae8dbd 0\t.claude/skills/a-file\n" +
		"100644 ffb1ec61c0c66e9773adadbabd6cf97853ae8dbd 0\t.claude/skills/a-folder/SKILL.md\n")
	for name, want := range map[string]string{"a-link": gitSymlink, "a-file": "100644", "a-folder": "a folder holding SKILL.md"} {
		if modes[name] != want {
			t.Errorf("%s: want %q, got %q", name, want, modes[name])
		}
	}
	if len(modes) != 3 {
		t.Errorf("want three entries, got %v", modes)
	}
}

// A wrapper is held to the skills it names: one that is gone is a problem, once. The placeholder
// path of the placement table names no skill.
func TestNamedSkillsAreHeldToTheFolders(t *testing.T) {
	skills := map[string]bool{"tidy": true, "design-brief": true}
	for _, c := range []struct {
		name, doc string
		want      []string
	}{
		{name: "a wrapper names one skill", doc: "Read `.agents/skills/tidy/SKILL.md`, or `.agents/skills/<name>/SKILL.md`."},
		{name: "a wrapper names a skill that is gone", doc: "Read `.agents/skills/gone/SKILL.md`, twice: `.agents/skills/gone/SKILL.md`.",
			want: []string{"the list names .agents/skills/gone/SKILL.md"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := namedSkillProblems("the list", c.doc, skills)
			if len(got) != len(c.want) {
				t.Fatalf("want %d problems, got %q", len(c.want), got)
			}
			for i, want := range c.want {
				if !strings.Contains(got[i], want) {
					t.Errorf("want a problem containing %q, got %q", want, got[i])
				}
			}
		})
	}
}
