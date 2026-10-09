// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"errors"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A legal hold stops a reset of the workspace to a backup (data-protection.md §5, backup-restore.md
// §8.2): the real REPLACE_TENANT is refused while any hold is in force - at the request, where the
// caller reads it, and again at the job, before anything is emptied.

// holdsDouble is the holds in force, read.
type holdsDouble struct {
	inForce lifecycle.Holds
	reads   int
}

func (h *holdsDouble) Active(context.Context) (lifecycle.Holds, error) {
	h.reads++
	return h.inForce, nil
}

func (h *holdsDouble) Contributors(context.Context, []shared.ID) (map[shared.ID][]shared.ID, error) {
	return nil, nil
}

// anAccountHold is the hold whose scope the refusal must not tell: a person under a hold.
func anAccountHold() lifecycle.Holds {
	return lifecycle.Holds{{
		ID: "0192f000-0000-7000-8000-0000000000a1", Scope: lifecycle.HoldAccount,
		ScopeID: "0192f000-0000-7000-8000-0000000000a2", Reason: "Pending litigation",
	}}
}

func refusedUnderHold(t *testing.T, err error) {
	t.Helper()
	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != lifecycle.CodeLegalHold {
		t.Fatalf("refused with %v, want %s", err, lifecycle.CodeLegalHold)
	}
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the refusal is not a conflict: %v", err)
	}
	if scope := domainErr.Params["scope"]; scope != lifecycle.RefusalScopeWorkspace {
		t.Errorf("the refusal names the scope %q, want %q and never the hold's own", scope,
			lifecycle.RefusalScopeWorkspace)
	}
}

func TestAReplaceUnderAHoldIsRefusedBeforeTheNameAndTheProof(t *testing.T) {
	h := newStartHarness(t)
	h.stepUp.available, h.stepUp.satisfied = true, true
	holds := &holdsDouble{inForce: anAccountHold()}
	restorer := h.restorer()
	restorer.Holds = holds

	_, err := (StartRestore{Restorer: restorer}).Execute(context.Background(), caller(),
		restoreRequest(func(r *domain.RestoreRequest) {
			r.Mode, r.DryRun, r.Confirmation = domain.RestoreReplaceTenant, false, "Acme GmbH"
			r.StepUpToken = "a-proof"
		}))

	refusedUnderHold(t, err)
	if len(h.stepUp.tokens) != 0 {
		t.Error("the refusal burned the step-up proof")
	}
	if len(h.restores.stored) != 0 || len(h.queued.requests) != 0 {
		t.Error("a refused restore left a run or a job behind")
	}
}

// The rehearsal writes nothing, so it stays possible under a hold (UC-BAK-05 check 1), and so do
// the modes that keep the workspace's holds.
func TestUnderAHoldARehearsalAndAMergeAreStillAccepted(t *testing.T) {
	cases := map[string]func(*domain.RestoreRequest){
		"a replace rehearsed": func(r *domain.RestoreRequest) {
			r.Mode, r.DryRun, r.Confirmation = domain.RestoreReplaceTenant, true, "Acme GmbH"
			r.StepUpToken = "a-proof"
		},
		"a merge": func(r *domain.RestoreRequest) { r.Mode, r.DryRun = domain.RestoreMerge, false },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			h := newStartHarness(t)
			h.stepUp.available, h.stepUp.satisfied = true, true
			restorer := h.restorer()
			restorer.Holds = &holdsDouble{inForce: anAccountHold()}

			if _, err := (StartRestore{Restorer: restorer}).Execute(
				context.Background(), caller(), restoreRequest(change)); err != nil {
				t.Fatalf("refused: %v", err)
			}
		})
	}
}

// A hold placed between the request and the job: the job asks again before it empties anything,
// fails the run with the hold's refusal, and empties nothing. The safety copy it took first stays.
func TestAReplaceMetByAHoldAtTheJobEmptiesNothing(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	h.imports.tables["work_item"] = map[string]map[string]any{"live": {"id": "live"}}
	in := h.accept(t, func(r *domain.Restore) {
		r.Mode, r.CreateSafetyBackup = domain.RestoreReplaceTenant, true
	})
	applier := h.applier()
	holds := &holdsDouble{inForce: anAccountHold()}
	applier.Holds = holds

	_, err := applier.Apply(context.Background(), in)

	refusedUnderHold(t, err)
	if len(h.imports.cleared) != 0 {
		t.Errorf("the replace emptied %v under a hold", h.imports.cleared)
	}
	if _, kept := h.imports.tables["work_item"]["live"]; !kept {
		t.Error("the living entry is gone")
	}
	if len(h.restores.outcomes) != 1 || h.restores.outcomes[0].Status != domain.RestoreFailed ||
		h.restores.outcomes[0].ErrorCode != lifecycle.CodeLegalHold {
		t.Fatalf("the run was closed as %+v", h.restores.outcomes)
	}
	if holds.reads == 0 {
		t.Error("the job never asked for the holds")
	}

	// The runner tries the job again; a closed run is not claimed, so nothing is emptied then either.
	h.restores.refuse = true
	if _, err := applier.Apply(context.Background(), in); err == nil {
		t.Fatal("the retry of a closed run was accepted as a restore")
	}
	if len(h.imports.cleared) != 0 || len(h.restores.outcomes) != 1 {
		t.Errorf("the retry emptied %v and closed the run %d times", h.imports.cleared, len(h.restores.outcomes))
	}
}

// With no hold in force the replace runs as before, and it read the holds to know.
func TestAReplaceWithNoHoldInForceRuns(t *testing.T) {
	h := newApplyHarness(t, containerRows)
	in := h.accept(t, func(r *domain.Restore) { r.Mode = domain.RestoreReplaceTenant })
	applier := h.applier()
	holds := &holdsDouble{}
	applier.Holds = holds

	if _, err := applier.Apply(context.Background(), in); err != nil {
		t.Fatalf("restoring: %v", err)
	}
	if holds.reads == 0 {
		t.Error("the replace emptied the workspace without asking for the holds")
	}
	if len(h.imports.tables["work_item"]) != 2 {
		t.Errorf("%d work items after the replace", len(h.imports.tables["work_item"]))
	}
}
