// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The proof state (ci-cd.md §12): what a person can do with Hubtask and whether something proves
// it, as figures.
//
// The table is rendered from what this gate already reads - the use cases' front matter, their
// numbered checks and the citations in their checked_by files - so a figure in it cannot be raised
// without a file that cites a check. `-proof-state` prints it (`make proof-state`). It is printed,
// never committed: two pull requests that each change a figure merge cleanly and would leave a
// committed page stale (ci-cd.md §3.5).

const (
	walkBoundsFile = "apps/webapp/e2e/bounds.json"
	walkBaseline   = "test/contract/testdata/walks-baseline.txt"
)

// proofStates is the order of the table's rows: the five states of docs/usecases/README.md.
var proofStates = []string{"verified", "built", "partial", "specified", "retired"}

// stateFigures are one state's use cases, their numbered checks, and the checks a file in
// checked_by cites.
type stateFigures struct {
	useCases, checks, cited int
}

// proofFigures is everything the table shows.
type proofFigures struct {
	states map[string]stateFigures
	// The falling bounds: each number may only go down, and a gate holds each.
	uncitedBuilt, fixedWaits, timerWaits, suitesWithoutUseCase, answersOutsideContract int
}

func readProofFigures(root string) (proofFigures, []string) {
	cases, problems := collectUseCases(root, filepath.Join(root, "docs", "usecases"))
	figures := proofFigures{states: countStates(root, cases), uncitedBuilt: uncitedBuiltAtMost}

	var bounds struct {
		FixedWaits  map[string]int `json:"fixed_waits"`
		TimerWaits  map[string]int `json:"timer_waits"`
		NoCheckCite []string       `json:"no_check_cited"`
	}
	if err := json.Unmarshal([]byte(read(root, walkBoundsFile)), &bounds); err != nil {
		problems = append(problems, fmt.Sprintf("%s does not parse: %v", walkBoundsFile, err))
	}
	for _, n := range bounds.FixedWaits {
		figures.fixedWaits += n
	}
	for _, n := range bounds.TimerWaits {
		figures.timerWaits += n
	}
	figures.suitesWithoutUseCase = len(bounds.NoCheckCite)

	for _, row := range strings.Split(read(root, walkBaseline), "\n") {
		if row = strings.TrimSpace(row); row != "" && !strings.HasPrefix(row, "#") {
			figures.answersOutsideContract++
		}
	}
	return figures, problems
}

// countStates counts by state as the gate does: a check is proven when a file in the use case's
// checked_by cites it by number (uncitedChecks).
func countStates(root string, cases []useCase) map[string]stateFigures {
	states := map[string]stateFigures{}
	for _, uc := range cases {
		checks := numberedChecks(sectionsOf(uc.body)["How to check"])
		figures := states[uc.fields["state"]]
		figures.useCases++
		figures.checks += len(checks)
		figures.cited += len(checks) - len(uncitedChecks(root, uc.id, checks, uc.lists["checked_by"]))
		states[uc.fields["state"]] = figures
	}
	return states
}

func renderProofState(f proofFigures) string {
	var b strings.Builder
	b.WriteString(`## The proof state

What a person can do with Hubtask is written as use cases (` + "`docs/usecases/`" + `), each with numbered
checks. This counts them: how many use cases are in which state, and how many of their checks are
cited by number in a file the use case names in ` + "`checked_by`" + ` - a test, or an evidence document, a
script or a workflow where no test can make the run. No figure here can be raised without such a
file.

### Use cases by state

| State | Use cases | Checks | Checks a ` + "`checked_by`" + ` file cites |
|---|---|---|---|
`)
	var total stateFigures
	for _, state := range proofStates {
		s := f.states[state]
		fmt.Fprintf(&b, "| `%s` | %d | %d | %d |\n", state, s.useCases, s.checks, s.cited)
		total.useCases += s.useCases
		total.checks += s.checks
		total.cited += s.cited
	}
	fmt.Fprintf(&b, "| All | %d | %d | %d |\n", total.useCases, total.checks, total.cited)
	b.WriteString(`
` + "`verified`" + ` means every check is cited; ` + "`built`" + ` means the use case works and some checks are not
cited yet; ` + "`partial`" + ` and ` + "`specified`" + ` name what is not met in their *Today* section.

### Bounds that only fall

Each number is a debt a gate holds: a change may lower it and may not raise it.

| Bound | Now | Held by |
|---|---|---|
`)
	fmt.Fprintf(&b, "| Checks of built use cases that no `checked_by` file cites | %d | `make gate-docs` |\n", f.uncitedBuilt)
	fmt.Fprintf(&b, "| Fixed waits in the browser walks | %d | the web app's tests (`%s`) |\n", f.fixedWaits, walkBoundsFile)
	fmt.Fprintf(&b, "| Timer waits in the browser walks | %d | the web app's tests (`%s`) |\n", f.timerWaits, walkBoundsFile)
	fmt.Fprintf(&b, "| Walk suites that cite no use case check | %d | the web app's tests (`%s`) |\n", f.suitesWithoutUseCase, walkBoundsFile)
	fmt.Fprintf(&b, "| Answers of the walks' stand-in server outside the contract | %d | `make gate-walks` (`%s`) |\n", f.answersOutsideContract, walkBaseline)
	return b.String()
}
