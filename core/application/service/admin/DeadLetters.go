// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The installation's dead letters for its operator (UC-INS-12, alert A-07): which jobs ran out of
// attempts, and the one act on them - running one again once its cause is fixed. Codes and moments
// only; what a job was given names people by identifier and stays in the table (rule 10).

const (
	ListDeadLettersName   = "ListDeadLetters"
	RequeueDeadLetterName = "RequeueDeadLetter"

	deadLettersReadAction    audit.Action = "instance.dead_letters_read"
	deadLetterRequeuedAction audit.Action = "instance.dead_letter_requeued"

	deadLetterTarget = "job"
)

var deadLetterActivityExempt = usecase.ActivityDeclaration{
	Exempt: "A job of the queue is not an entry, and the item history is keyed on an entry.",
}

// ListDeadLetters answers the installation's dead letters, newest first, a page at a time.
type ListDeadLetters struct {
	Writer      InstanceWriter
	DeadLetters adminrepo.DeadLetters
	Cursors     DeliveryCursors
}

// DeadLetterPage is one page.
type DeadLetterPage struct {
	Letters []adminrepo.DeadLetter
	Next    string
	More    bool
}

// Execute reads one page, of one kind when it is named.
func (h ListDeadLetters) Execute(
	ctx context.Context, actor appshared.ActorContext, kind, cursor string, size int,
) (DeadLetterPage, error) {
	if err := h.Writer.authorize(ctx, actor); err != nil {
		return DeadLetterPage{}, err
	}
	var page DeadLetterPage
	limit := journalPageSize(size)
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var before shared.ID
		if cursor != "" {
			decoded, err := h.Cursors.DecodeID(cursor)
			if err != nil {
				return err
			}
			before = decoded
		}
		read, err := h.DeadLetters.Page(ctx, kind, before, limit+1)
		if err != nil {
			return err
		}
		if page.More = len(read) > limit; page.More {
			read = read[:limit]
			page.Next = h.Cursors.EncodeID(read[len(read)-1].ID)
		}
		page.Letters = read
		return nil
	})
	return page, err
}

// RequeueDeadLetter runs one dead letter again: due now, its attempts from zero, journalled in the
// same transaction.
type RequeueDeadLetter struct {
	Writer      InstanceWriter
	DeadLetters adminrepo.DeadLetters
}

// Execute requeues it, or answers why not: no such dead letter, or another job of its kind and key
// already open.
func (h RequeueDeadLetter) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (adminrepo.DeadLetter, error) {
	w := h.Writer
	if err := w.authorize(ctx, actor); err != nil {
		return adminrepo.DeadLetter{}, err
	}
	var letter adminrepo.DeadLetter
	err := w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		requeued, err := h.DeadLetters.Requeue(ctx, id, now)
		if err != nil {
			return err
		}
		letter = requeued
		details := map[string]any{"job_id": requeued.ID.String(), "kind": requeued.Kind}
		if requeued.TenantID != "" {
			details["tenant_id"] = requeued.TenantID.String()
		}
		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: now, Action: string(deadLetterRequeuedAction),
			ActorLabel: actor.AccountName, Details: details,
		})
	})
	return letter, err
}

// DeadLetterOutput is the contract's DeadLetter.
func DeadLetterOutput(d adminrepo.DeadLetter) usecase.Output {
	out := usecase.Output{
		"id": d.ID.String(), "kind": d.Kind, "tenant_id": nil, "state": "DEAD_LETTER",
		"attempts": d.Attempts, "max_attempts": d.MaxAttempts, "error_code": nil,
		"created_at": d.CreatedAt, "finished_at": nil,
	}
	if d.Requeued {
		out["state"] = "PENDING"
	}
	if d.TenantID != "" {
		out["tenant_id"] = d.TenantID.String()
	}
	if d.ErrorCode != "" {
		out["error_code"] = d.ErrorCode
	}
	if !d.FinishedAt.IsZero() {
		out["finished_at"] = d.FinishedAt
	}
	return out
}

func (h ListDeadLetters) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListDeadLettersName,
		Summary: "The installation's jobs that ran out of attempts, newest first: kind, workspace, " +
			"attempts, the code of the last failure and when it ended. Never what a job was given.",
		SideEffects: "None. Reads only.",
		TokenScope:  adminTenantsScope,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "kind", Kind: usecase.KindString, Description: "Only the dead letters of this kind of job."},
			{Name: "cursor", Kind: usecase.KindString, Description: "Where the last page ended."},
			{Name: "limit", Kind: usecase.KindInt, Description: "How many at most: fifty by default, two hundred at most."},
		},
		Audit: usecase.AuditDeclaration{
			Action: deadLettersReadAction, TargetType: instanceTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: deadLetterActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			page, err := h.Execute(ctx, actor, in.String("kind"), in.String("cursor"), in.Int("limit"))
			if err != nil {
				return nil, err
			}
			rows := make([]usecase.Output, 0, len(page.Letters))
			for _, d := range page.Letters {
				rows = append(rows, DeadLetterOutput(d))
			}
			var next any
			if page.More {
				next = page.Next
			}
			return usecase.Output{"data": rows, "page": map[string]any{"next_cursor": next, "has_more": page.More}}, nil
		}),
	}
}

func (h RequeueDeadLetter) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RequeueDeadLetterName,
		Summary: "Runs a dead letter of the installation again once its cause is fixed: due now, its " +
			"attempts from zero. Refused while another job of the same kind and key is open.",
		SideEffects: "Returns the job to the queue and writes one journal entry.",
		TokenScope:  adminTenantsScope,
		Input: []usecase.Field{
			{Name: "job_id", Kind: usecase.KindID, Required: true, Description: "The dead letter."},
		},
		Audit: usecase.AuditDeclaration{
			Action: deadLetterRequeuedAction, TargetType: deadLetterTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: deadLetterActivityExempt,
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("job_id")
			if err != nil {
				return nil, err
			}
			letter, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return DeadLetterOutput(letter), nil
		}),
	}
}
