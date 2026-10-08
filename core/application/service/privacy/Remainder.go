// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	lifecycle "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The rest of an erasure a legal hold kept part of (data-protection.md §4.1, UC-PRV-03 check 12).
// Releasing the hold is the write that seeds it, in the release's own transaction, so a committed
// release always has its jobs; the retention pass seeds whatever nothing seeded - a release served
// by an older binary, a restore that dropped or lifted a hold. The job decides nothing (rule 2): the
// erasure was authorised when it was started, and it runs the same erasure again against the holds
// in force now.

// RemainderErasedAction is the entry the rest of an erasure leaves. Its own action rather than a
// second `dsr.erased`: an erasure writes one of those (check 7), and the rest is a later act.
const RemainderErasedAction audit.Action = "dsr.remainder_erased"

// ErasureRemainders seeds the rest of the erasures a lifted hold kept part of.
type ErasureRemainders struct {
	Kept repository.Kept
	Jobs Enqueuer
}

// Seed queues the rest of every case the hold still keeps something of, one job per case. Called
// in the transaction that lifts the hold.
func (r ErasureRemainders) Seed(ctx context.Context, tenantID, holdID shared.ID) error {
	pending, err := r.Kept.PendingKept(ctx, holdID)
	if err != nil {
		return err
	}
	for _, part := range pending {
		if err := r.enqueue(ctx, tenantID, part); err != nil {
			return err
		}
	}
	return nil
}

// Reconcile queues the rest of every case kept under a hold that is no longer in force, for the
// remainders nothing seeded. The dedupe key makes a second seed of a queued one harmless.
func (r ErasureRemainders) Reconcile(ctx context.Context, tenantID shared.ID, active lifecycle.Holds) error {
	pending, err := r.Kept.PendingKept(ctx, "")
	if err != nil {
		return err
	}
	inForce := make(map[shared.ID]bool, len(active))
	for _, hold := range active {
		inForce[hold.ID] = true
	}
	for _, part := range pending {
		if inForce[part.HoldID] {
			continue
		}
		if err := r.enqueue(ctx, tenantID, part); err != nil {
			return err
		}
	}
	return nil
}

func (r ErasureRemainders) enqueue(ctx context.Context, tenantID shared.ID, part repository.PendingKept) error {
	_, err := r.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindPrivacyErasureRemainder,
		TenantID:  tenantID,
		Payload:   map[string]any{"request_id": part.RequestID.String()},
		DedupeKey: "dsr-remainder:" + part.RequestID.String() + ":" + part.HoldID.String(),
	})
	return err
}

// ResumeErasure carries out the rest of one case's erasure.
type ResumeErasure struct {
	Requests   repository.Requests
	Kept       repository.Kept
	Eraser     Eraser
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Resume runs the erasure again against the holds in force. What a hold still keeps stays and is
// recorded under it; what none keeps goes, and its part is marked erased. A full deletion that
// meets a rule acting as the person is a refusal, not a failure to retry: the case says why the rest
// waits, and the retention pass tries again.
func (h ResumeErasure) Resume(ctx context.Context, in PerformInput) error {
	actor := appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: in.TenantID, AccountName: "the installation",
	}

	var request domain.Request
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		request, err = h.Requests.Find(ctx, in.RequestID)
		return err
	}); err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The case is gone, and its record of what was kept with it.
			return nil
		}
		return err
	}
	if request.Kind != domain.KindErasure || request.Status == domain.StatusRejected {
		return nil
	}

	if request.SubjectAccountID.IsZero() {
		// The account is already gone; nothing of the person's can still be kept.
		return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			return h.Kept.RecordKept(ctx, request.ID, nil, h.Clock.Now())
		})
	}

	erased, err := h.Eraser.erase(ctx, actor, request, RemainderErasedAction)
	if err != nil {
		if problem := shared.AsError(err); problem != nil && problem.DetailCode == domain.CodeErasureBlockedByRule {
			return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				return h.Kept.BlockKept(ctx, request.ID, problem.DetailCode, problem.Params)
			})
		}
		return err
	}

	gone := request.ErasureMode == domain.ModeFullDelete && !erased.AccountKept && !erased.AccountAnonymised
	if !gone || request.Status != domain.StatusCompleted {
		// A case still in progress is completed by its own job, whose retry finds the account gone.
		return nil
	}
	cleared := request
	cleared.SubjectAccountID = ""
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Requests.Save(ctx, cleared); err != nil {
			return err
		}
		return nil
	})
}
