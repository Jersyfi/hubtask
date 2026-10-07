// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// `make verify` is what a session runs while it works. The contract gate needs no container, so it
// belongs there: a drifted contract found at the pull request's end costs a round trip.

var verifyTarget = regexp.MustCompile(`(?m)^verify:([^\n]*)$`)

func verifyRuns(makefile string, gates ...string) error {
	m := verifyTarget.FindStringSubmatch(makefile)
	if m == nil {
		return fmt.Errorf("the Makefile has no verify target")
	}
	runs := strings.Fields(m[1])
	for _, gate := range gates {
		if !slices.Contains(runs, gate) {
			return fmt.Errorf("make verify does not run %s (it runs %v)", gate, runs)
		}
	}
	return nil
}

func TestVerifyRunsTheContractGate(t *testing.T) {
	if err := verifyRuns(string(readFile(t, "../../Makefile")), "gate-unit", "gate-docs", "gate-contract"); err != nil {
		t.Error(err)
	}
	if verifyRuns("verify: gate-quick gate-unit gate-docs\n", "gate-contract") == nil {
		t.Error("a verify target without the contract gate passed")
	}
}
