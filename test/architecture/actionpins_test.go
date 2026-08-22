// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// workflowUses matches `uses: owner/repo[/path]@<pin>` with the version comment ci-cd.md §4 asks
// for. `uses: ./…` and `uses: docker://…` name no GitHub repository - the character class matches
// neither, which is the intended exclusion rather than an oversight.
var workflowUses = regexp.MustCompile(`(?m)^\s*(?:-\s+)?uses:\s*([A-Za-z0-9._/-]+)@(\S+)\s*(?:#\s*(.*))?$`)

// TestActionsOfOneRepositoryAgreeOnTheirVersion closes the half of the pin rule that a diff cannot
// show and the network gate cannot see.
//
// `scripts/check-action-pins.sh` asks GitHub whether each pin resolves and whether its comment
// names the right tag. Both questions are about one line at a time, and that is exactly how #16
// got past them: `github/codeql-action/init`, `/autobuild`, `/analyze` and `/upload-sarif` are
// four dependencies to Dependabot and one repository with one commit to CodeQL. Bumping `init`
// alone left two pins of the same repository at the previous major in the same job - every pin
// resolving, every comment correct, and the analysis dead on arrival with "configuration error".
//
// Grouping the updates (.github/dependabot.yml) stops that from being proposed. This stops it from
// being merged, which is the half that also covers a hand-edit.
//
// It runs here rather than in the nightly script because it needs no network: a version two
// `uses:` lines disagree about is visible in the checkout itself, and a rule that only bites at
// 02:00 bites after the merge.
//
// A repository that genuinely needs two versions at once - a matrix testing an upgrade, say - is
// not a case this repository has. If it ever arrives, it belongs here as a named exemption rather
// than as a deleted test.
func TestActionsOfOneRepositoryAgreeOnTheirVersion(t *testing.T) {
	type use struct{ file, pin, comment string }

	uses := map[string][]use{}
	var total int

	entries, err := filepath.Glob("../../.github/workflows/*.yml")
	if err != nil || len(entries) == 0 {
		t.Fatalf("no workflows found under .github/workflows: %v", err)
	}
	sort.Strings(entries)

	for _, path := range entries {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("%s is not readable: %v", filepath.Base(path), err)
		}
		for _, match := range workflowUses.FindAllStringSubmatch(string(content), -1) {
			action, pin, comment := match[1], match[2], strings.TrimSpace(match[3])
			repository := action
			if parts := strings.Split(action, "/"); len(parts) > 2 {
				repository = parts[0] + "/" + parts[1]
			}
			total++
			uses[repository] = append(uses[repository], use{filepath.Base(path), pin, comment})
		}
	}

	// A regular expression that has stopped matching would otherwise report every repository as
	// consistent, which is the one failure this test must not produce silently.
	if total == 0 {
		t.Fatal("no pinned `uses:` found in .github/workflows - the pattern has stopped matching, not the workflows stopped using actions")
	}

	repositories := make([]string, 0, len(uses))
	for repository := range uses {
		repositories = append(repositories, repository)
	}
	sort.Strings(repositories)

	for _, repository := range repositories {
		distinct := map[string][]string{}
		for _, u := range uses[repository] {
			where := u.file
			if u.comment != "" {
				where = u.file + " (" + u.comment + ")"
			}
			distinct[u.pin] = append(distinct[u.pin], where)
		}
		if len(distinct) == 1 {
			continue
		}

		pins := make([]string, 0, len(distinct))
		for pin := range distinct {
			pins = append(pins, pin)
		}
		sort.Strings(pins)

		var detail []string
		for _, pin := range pins {
			detail = append(detail, pin[:min(7, len(pin))]+" in "+strings.Join(distinct[pin], ", "))
		}
		t.Errorf("%s is pinned to %d different commits: %s - actions of one repository share one commit, and mixing them is a configuration error (ci-cd.md §4)",
			repository, len(distinct), strings.Join(detail, "; "))
	}
}
