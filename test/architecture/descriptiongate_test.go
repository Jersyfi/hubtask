// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"fmt"
	"strings"
	"testing"
)

// The description gate holds the readiness record, the merged migrations and the use cases that
// may not be deleted, besides the description itself. Dependabot is the one author it skips: it
// writes its own description and settles nothing. Any other bot - a tool that steers the work
// under its own GitHub App identity among them - is held like a person, so the condition names
// Dependabot by login rather than every account of type Bot.

const dependabotOnly = "github.event.pull_request.user.login != 'dependabot[bot]'"

func exemptsOnlyDependabot(condition string) error {
	if strings.Contains(condition, "user.type") {
		return fmt.Errorf("it exempts an account by its type, which lets every bot past: %q", condition)
	}
	if !strings.Contains(condition, dependabotOnly) {
		return fmt.Errorf("it does not exempt Dependabot by its login (%s): %q", dependabotOnly, condition)
	}
	return nil
}

func TestOnlyDependabotSkipsTheDescriptionGate(t *testing.T) {
	for file, job := range map[string]string{"ci.yml": "pr-description", "pr-description-rerun.yml": "rerun"} {
		j, ok := loadWorkflow(t, file).Jobs[job]
		if !ok {
			t.Fatalf("%s has no job %s", file, job)
		}
		if err := exemptsOnlyDependabot(j.If); err != nil {
			t.Errorf("%s, job %s: %v", file, job, err)
		}
	}
}

func TestExemptsOnlyDependabotRefusesEveryBot(t *testing.T) {
	if exemptsOnlyDependabot("github.event_name == 'pull_request' && github.event.pull_request.user.type != 'Bot'") == nil {
		t.Error("a condition that skips every bot passed")
	}
	if exemptsOnlyDependabot("github.event_name == 'pull_request'") == nil {
		t.Error("a condition that skips nobody passed - Dependabot's own description would be judged")
	}
}
