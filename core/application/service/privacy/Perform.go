// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Performer carries out a case that has been started, and closes it.
//
// The application layer's half of the `privacy.request` job: the worker owns the queue, the lease
// and the retries; what a case *is* - an archive or an erasure, and the record that it was
// answered - lives here.
//
// It is the one place the two halves meet, and that is why it exists rather than the worker
// calling both: a case that produced its archive and was never completed would sit in the list of
// what is owed for ever, and a case completed before the work finished would say the person was
// answered when they were not.
type Performer struct {
	Requests repository.Requests
	Eraser   Eraser
	Exporter Exporter
	// System is the actor a job acts as. A case is carried out by the installation rather than by
	// whoever asked for it: the person who started it may have gone home, and the entry the work
	// leaves says which case it belonged to (audit.md §2).
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// PerformInput is one case as the job's payload describes it.
type PerformInput struct {
	RequestID shared.ID
	TenantID  shared.ID
}

// Perform does the work and completes the case.
func (p Performer) Perform(ctx context.Context, in PerformInput) (domain.Request, error) {
	actor := appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: in.TenantID, AccountName: "the installation",
	}

	var request domain.Request
	if err := p.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		request, err = p.Requests.Find(ctx, in.RequestID)
		return err
	}); err != nil {
		return domain.Request{}, err
	}

	if request.Status != domain.StatusInProgress {
		// Somebody refused or completed the case between the job being queued and it being
		// claimed. Not an error to retry into: the case has an answer, and doing the work now
		// would be doing it against a decision that has already been taken.
		return request, nil
	}

	archive := ""
	switch {
	case request.Kind == domain.KindErasure:
		if _, err := p.Eraser.Erase(ctx, actor, request); err != nil {
			return domain.Request{}, err
		}
	case request.Kind.ProducesArchive():
		written, err := p.Exporter.Export(ctx, actor, request)
		if err != nil {
			return domain.Request{}, err
		}
		archive = written.Archive
	default:
		// A kind with no work behind it should never have been queued; carrying on and completing
		// the case is the harmless reading, and the queue's own deduplication makes a second
		// arrival of this the same no-op.
	}

	done, err := request.Complete(p.Clock.Now(), archive)
	if err != nil {
		return domain.Request{}, err
	}
	if request.Kind == domain.KindErasure && request.ErasureMode == domain.ModeFullDelete {
		// The subject is gone, and the case may not keep pointing at a row that no longer exists.
		// The column has been nullable since `0001_init` for exactly this: "may be NULL once
		// fulfilled".
		done.SubjectAccountID = ""
	}

	err = p.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		saved, err := p.Requests.Save(ctx, done)
		if err != nil {
			return err
		}
		if !saved {
			return shared.ErrNotFound.WithDetail(domain.CodeRequestNotFound)
		}
		return nil
	})
	if err != nil {
		return domain.Request{}, err
	}
	return done, nil
}

// RequestOf reads the job's payload back into the input this performer takes.
func RequestOf(payload map[string]any, tenantID shared.ID) (PerformInput, error) {
	raw, _ := payload["request_id"].(string)
	requestID, err := shared.ParseID(raw)
	if err != nil {
		return PerformInput{}, shared.Internalf("privacy: a request job without a readable case")
	}
	return PerformInput{RequestID: requestID, TenantID: tenantID}, nil
}
