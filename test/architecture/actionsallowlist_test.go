// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"
)

// The repository setting "Allow select actions" names the third-party repositories a workflow may
// run (ci-cd.md §4). The setting cannot be read without the administration right, so its content
// is kept in .github/actions-allowlist.txt, and this test holds that file to the workflows: a
// repository used and not listed would be refused by GitHub at job set-up - after the merge, for a
// workflow that runs only on a tag - and an entry nobody uses widens the setting for nothing.
//
// Whether the setting itself matches the file is the `actions-allowed` job's question, asked of
// GitHub on the pull request.

const allowlistPath = "../../.github/actions-allowlist.txt"

// An entry is `owner/repo`, optionally `# nested in owner/repo`: an action that a listed composite
// action runs inside itself. Job set-up resolves those too, but no workflow names them, so they are
// declared rather than discovered - the offline gates do not read another repository's action.yml.
var allowlistEntry = regexp.MustCompile(`^([A-Za-z0-9._-]+/[A-Za-z0-9._-]+)(?:\s+#\s*nested in\s+([A-Za-z0-9._-]+/[A-Za-z0-9._-]+))?$`)

type allowEntry struct {
	repository, nestedIn string
}

// githubOwned are the owners the setting allows as "actions created by GitHub"; they are not listed.
var githubOwned = []string{"actions", "github"}

func parseAllowlist(content string) ([]allowEntry, []string) {
	var entries []allowEntry
	var problems []string
	for i, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := allowlistEntry.FindStringSubmatch(line)
		if m == nil {
			problems = append(problems, fmt.Sprintf("line %d %q is neither `owner/repo` nor `owner/repo  # nested in owner/repo`", i+1, line))
			continue
		}
		entries = append(entries, allowEntry{repository: m[1], nestedIn: m[2]})
	}
	return entries, problems
}

// thirdPartyRepositories are the repositories of every pinned `uses:` whose owner GitHub does not own.
func thirdPartyRepositories(workflows map[string]string) map[string][]string {
	used := map[string][]string{}
	for file, content := range workflows {
		for _, match := range workflowUses.FindAllStringSubmatch(content, -1) {
			parts := strings.Split(match[1], "/")
			if len(parts) < 2 || slices.Contains(githubOwned, parts[0]) {
				continue
			}
			repository := parts[0] + "/" + parts[1]
			if !slices.Contains(used[repository], file) {
				used[repository] = append(used[repository], file)
			}
		}
	}
	return used
}

func allowlistProblems(entries []allowEntry, used map[string][]string) []string {
	var problems []string
	listed := map[string]allowEntry{}
	var order []string
	for _, e := range entries {
		if _, twice := listed[e.repository]; twice {
			problems = append(problems, e.repository+" is listed twice")
		}
		listed[e.repository] = e
		order = append(order, e.repository)
		if owner := strings.Split(e.repository, "/")[0]; slices.Contains(githubOwned, owner) {
			problems = append(problems, e.repository+" is owned by GitHub, which the setting allows without a list - drop the entry")
		}
	}
	if !sort.StringsAreSorted(order) {
		problems = append(problems, "the entries are not sorted")
	}

	repositories := make([]string, 0, len(used))
	for repository := range used {
		repositories = append(repositories, repository)
	}
	sort.Strings(repositories)
	for _, repository := range repositories {
		e, ok := listed[repository]
		switch {
		case !ok:
			problems = append(problems, fmt.Sprintf("%s is used by %s and not in .github/actions-allowlist.txt - a new third-party action is the owner's word, then an entry here and in the repository setting (ci-cd.md §4)",
				repository, strings.Join(sorted(used[repository]), ", ")))
		case e.nestedIn != "":
			problems = append(problems, fmt.Sprintf("%s is marked nested in %s, but %s uses it directly - drop the comment",
				repository, e.nestedIn, strings.Join(sorted(used[repository]), ", ")))
		}
	}

	for _, e := range entries {
		if _, direct := used[e.repository]; direct {
			continue
		}
		if e.nestedIn == "" {
			problems = append(problems, e.repository+" is listed and no workflow uses it - remove it here and from the repository setting, or mark what nests it (`# nested in owner/repo`)")
			continue
		}
		if _, parentUsed := used[e.nestedIn]; !parentUsed || listed[e.nestedIn].repository == "" {
			problems = append(problems, fmt.Sprintf("%s is marked nested in %s, which is not a listed action a workflow uses", e.repository, e.nestedIn))
		}
	}
	return problems
}

func TestTheActionsAllowlistNamesExactlyTheActionsTheWorkflowsUse(t *testing.T) {
	files, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(files) == 0 {
		t.Fatalf("no workflows found under .github/workflows: %v", err)
	}
	workflows := map[string]string{}
	for _, path := range files {
		workflows[filepath.Base(path)] = string(readFile(t, path))
	}

	used := thirdPartyRepositories(workflows)
	// A pattern that stopped matching would find every entry unused and say nothing about the rest.
	if len(used) == 0 {
		t.Fatal("no third-party `uses:` found in .github/workflows - the pattern has stopped matching")
	}

	entries, problems := parseAllowlist(string(readFile(t, allowlistPath)))
	problems = append(problems, allowlistProblems(entries, used)...)
	for _, p := range problems {
		t.Error(".github/actions-allowlist.txt: " + p)
	}
}

func TestTheActionsAllowlistCheckRefusesWhatItShould(t *testing.T) {
	workflows := map[string]string{
		"a.yml": "steps:\n" +
			"  - uses: actions/checkout@0000000000000000000000000000000000000000 # v1\n" +
			"  - uses: github/codeql-action/init@0000000000000000000000000000000000000000 # v1\n" +
			"  - uses: acme/scan-action@1111111111111111111111111111111111111111 # v2\n" +
			"  - uses: zeta/tool/sub@2222222222222222222222222222222222222222 # v3\n",
	}
	used := thirdPartyRepositories(workflows)
	if got := len(used); got != 2 {
		t.Fatalf("GitHub-owned actions must not count as third-party, and a sub-path counts by its repository: got %v", used)
	}

	cases := []struct {
		name, file string
		want       string // a fragment of the one expected problem; empty for none
	}{
		{"complete", "acme/scan-action\nacme/setup  # nested in acme/scan-action\nzeta/tool\n", ""},
		{"unlisted", "acme/scan-action\n", "zeta/tool is used by a.yml and not in"},
		{"unused", "acme/scan-action\nomega/old\nzeta/tool\n", "omega/old is listed and no workflow uses it"},
		{"nested in an unused parent", "acme/scan-action\nacme/setup  # nested in omega/old\nzeta/tool\n", "which is not a listed action a workflow uses"},
		{"nested in an unlisted parent", "acme/setup  # nested in acme/scan-action\nzeta/tool\n", "acme/scan-action is used by a.yml and not in"},
		{"nested but used directly", "acme/scan-action\nzeta/tool  # nested in acme/scan-action\n", "uses it directly"},
		{"GitHub-owned listed", "acme/scan-action\nactions/checkout  # nested in acme/scan-action\nzeta/tool\n", "owned by GitHub"},
		{"unsorted", "zeta/tool\nacme/scan-action\n", "not sorted"},
		{"twice", "acme/scan-action\nacme/scan-action\nzeta/tool\n", "listed twice"},
		{"malformed", "acme/scan-action@*\nzeta/tool\n", "neither"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			entries, problems := parseAllowlist(c.file)
			problems = append(problems, allowlistProblems(entries, used)...)
			if c.want == "" {
				if len(problems) != 0 {
					t.Fatalf("want none, got %q", problems)
				}
				return
			}
			found := false
			for _, p := range problems {
				found = found || strings.Contains(p, c.want)
			}
			if !found {
				t.Fatalf("want a problem containing %q, got %q", c.want, problems)
			}
		})
	}
}
