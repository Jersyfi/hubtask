// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The table's use case counts equal what the use cases' front matter says. The front matter is
// read here line by line, apart from the gate's own reading, so a fault in that reading shows as a
// difference.
func TestTheProofStateEqualsTheUseCasesStates(t *testing.T) {
	root, err := repositoryRoot()
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]int{}
	err = filepath.WalkDir(filepath.Join(root, "docs", "usecases"), func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasPrefix(entry.Name(), "UC-") {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer func() { _ = file.Close() }()
		lines := bufio.NewScanner(file)
		for fences := 0; lines.Scan() && fences < 2; {
			line := lines.Text()
			if line == "---" {
				fences++
			}
			if state, found := strings.CutPrefix(line, "state:"); found {
				want[strings.TrimSpace(state)]++
			}
		}
		return lines.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(want) == 0 {
		t.Fatal("no use case with a state: found under docs/usecases")
	}

	figures, problems := readProofFigures(root)
	if len(problems) > 0 {
		t.Fatalf("the tree does not render: %v", problems)
	}
	page := renderProofState(figures)
	row := regexp.MustCompile("(?m)^\\| (?:`([a-z]+)`|All) \\| (\\d+) \\| (\\d+) \\| (\\d+) \\|$")
	got := map[string]string{}
	for _, m := range row.FindAllStringSubmatch(page, -1) {
		got[m[1]] = m[2]
	}
	total := 0
	for _, state := range proofStates {
		total += want[state]
		if got[state] != fmt.Sprint(want[state]) {
			t.Errorf("the table shows %s use cases in state %s, the front matter says %d", got[state], state, want[state])
		}
	}
	if got[""] != fmt.Sprint(total) {
		t.Errorf("the table shows %s use cases in all, the front matter says %d", got[""], total)
	}
	for state := range want {
		if !ucStates[state] {
			t.Errorf("the front matter holds the state %q, which the table has no row for", state)
		}
	}
}

// A bounds file that does not parse is said, never counted as no debt.
func TestTheProofStateRefusesBoundsThatDoNotParse(t *testing.T) {
	root := t.TempDir()
	for path, content := range map[string]string{walkBoundsFile: "{", walkBaseline: ""} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, path)), 0o750); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, path), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, problems := readProofFigures(root); len(problems) == 0 {
		t.Fatal("bounds that do not parse rendered a table")
	}
}

// A check counts for its use case's state, and as cited only when a checked_by file names it.
func TestCountStatesCountsChecksAndCitations(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "a_test.go"), []byte("// UC-ID-16 check 1 and UC-ID-16/3.\n// UC-ID-17 check 2.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	uc := func(id, state string, evidence ...string) useCase {
		return useCase{id: id, fields: map[string]string{"state": state}, lists: map[string][]string{"checked_by": evidence}, body: useCaseBody}
	}
	got := countStates(root, []useCase{
		uc("UC-ID-16", "built", "a_test.go"),
		uc("UC-ID-17", "built"),
		uc("UC-ID-18", "specified"),
	})
	if want := (stateFigures{useCases: 2, checks: 6, cited: 2}); got["built"] != want {
		t.Errorf("built: got %+v, want %+v", got["built"], want)
	}
	if want := (stateFigures{useCases: 1, checks: 3}); got["specified"] != want {
		t.Errorf("specified: got %+v, want %+v", got["specified"], want)
	}
}

func TestRenderProofStateShowsEveryFigure(t *testing.T) {
	page := renderProofState(proofFigures{
		states:       map[string]stateFigures{"verified": {useCases: 2, checks: 11, cited: 11}, "built": {useCases: 1, checks: 5, cited: 3}},
		uncitedBuilt: 2, fixedWaits: 7, timerWaits: 4, suitesWithoutUseCase: 9, answersOutsideContract: 13,
	})
	for _, want := range []string{
		"| `verified` | 2 | 11 | 11 |", "| `built` | 1 | 5 | 3 |", "| `retired` | 0 | 0 | 0 |", "| All | 3 | 16 | 14 |",
		"file cites | 2 |", "Fixed waits in the browser walks | 7 |", "Timer waits in the browser walks | 4 |",
		"cite no use case check | 9 |", "outside the contract | 13 |",
	} {
		if !strings.Contains(page, want) {
			t.Errorf("the table lacks %q:\n%s", want, page)
		}
	}
}
