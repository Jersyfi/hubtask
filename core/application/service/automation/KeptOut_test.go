// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// An AI action with nothing it may work with - an empty entry, or one whose content is kept out
// of AI - is skipped, not failed: a rule must not stop at, or switch itself off over, somebody's
// objection (UC-PRV-05 check 7).
func TestAnAiActionWithNothingToWorkWithIsSkippedNotFailed(t *testing.T) {
	for _, code := range []string{"ai.nothing_to_work_with", "ai.kept_out"} {
		t.Run(code, func(t *testing.T) {
			rule := enabledRule()
			rule.OnError = domain.OnErrorStop
			rule.Actions = []domain.Action{{Kind: "CREATE_BUCKET"}, {Kind: "ADD_LABEL"}}
			// The engine's double knows a few kinds; which one answers the code is beside the point.
			h := newEngine(t, rule)
			h.dispatcher.refuse["CREATE_BUCKET"] = shared.ErrValidation.WithDetail(code)

			run, err := h.engine.Execute(context.Background(), engineActor(), command(0))
			if err != nil {
				t.Fatalf("running: %v", err)
			}
			first := run.ActionResults[0]
			if first.Status != domain.ActionSkipped || first.ErrorCode != code {
				t.Errorf("the AI action read %+v, want SKIPPED with its code", first)
			}
			// STOP did not stop: nothing failed.
			if len(run.ActionResults) < 2 || run.ActionResults[1].Status != domain.ActionSucceeded {
				t.Errorf("the next action did not run: %+v", run.ActionResults)
			}
			if run.Status != domain.RunSucceeded || run.Failed() != 0 {
				t.Errorf("status %q with %d failed, want SUCCEEDED and none", run.Status, run.Failed())
			}
		})
	}
}
