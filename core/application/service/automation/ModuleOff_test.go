// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
)

// An action whose module is switched off - a TAKE_STEP while WORKFLOWS is off - is skipped, not
// failed: switching a module off for a while must not switch the rules that use it off for good
// (UC-INS-21 check 4).
func TestAnActionWhoseModuleIsOffIsSkippedNotFailed(t *testing.T) {
	rule := enabledRule()
	rule.Actions = []domain.Action{{Kind: "TAKE_STEP"}, {Kind: "ADD_LABEL"}}
	h := newEngine(t, rule)
	h.dispatcher.scopes["TAKE_STEP"] = "items:write"
	h.dispatcher.refuse["TAKE_STEP"] = module.Set{}.Require(module.Workflows)

	for range MaxConsecutiveFailures + 1 {
		run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
		if err != nil {
			t.Fatalf("running: %v", err)
		}
		if got := run.ActionResults[0]; got.Status != domain.ActionSkipped || got.ErrorCode != module.OffCode {
			t.Fatalf("the action of the switched-off module is %q / %q", got.Status, got.ErrorCode)
		}
	}
	if h.failures.count != 0 || h.failures.disabled {
		t.Errorf("the streak counted %d, disabled %v", h.failures.count, h.failures.disabled)
	}
}
