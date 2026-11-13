// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	ReadMyDeadlinesKeptName = "ReadMyDeadlinesKept"
	// KeptReadAction is a person reading the deadlines they kept.
	KeptReadAction audit.Action = "entry_sla.kept_read"
	// DefaultKeptDays is the window a reading without one reaches back.
	DefaultKeptDays = 30
	// MaxKeptDays bounds the window to the report's year.
	MaxKeptDays = 365
)

// ReadMyDeadlinesKept answers the signed-in person how many of the deadlines on their own entries
// they kept (UC-SVC-02 check 15): any member reads their own, nobody another person's - the account
// asked about is the actor's, never an input - so it needs no report permission.
type ReadMyDeadlinesKept struct {
	Desk Desk
	Kept repository.KeptDeadlines
}

// DeadlinesKept is the answer: the window and a row per kind of deadline that ended in it.
type DeadlinesKept struct {
	From, To time.Time
	Rows     []domain.KeptRow
}

// Execute reads the window that ends now and reaches back days.
func (h ReadMyDeadlinesKept) Execute(ctx context.Context, actor appshared.ActorContext, days int) (DeadlinesKept, error) {
	if days < 1 || days > MaxKeptDays {
		return DeadlinesKept{}, malformed("/days", "sla_reports.period_invalid")
	}
	if err := h.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		AnyMember: true, Action: KeptReadAction, TokenScope: deskRead, TargetType: entrySLATarget, TargetID: actor.AccountID,
	}); err != nil {
		return DeadlinesKept{}, err
	}
	to := h.Desk.Clock.Now()
	answer := DeadlinesKept{From: to.AddDate(0, 0, -days), To: to}
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		answer.Rows, err = h.Kept.Kept(ctx, actor.AccountID, answer.From, answer.To)
		return err
	})
	return answer, err
}

// Descriptor is the catalogue entry.
func (h ReadMyDeadlinesKept) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadMyDeadlinesKeptName,
		Summary: "How many of the deadlines on the caller's own entries they kept: per kind - the requester's " +
			"goals and each team's OLA by its group - the deadlines met or breached in the window and how " +
			"many were met. Always the caller's own; no report permission needed.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "days", Kind: usecase.KindInt, Description: "How many days the window reaches back from now, 1 to 365; 30 when left out."},
		},
		Audit: usecase.AuditDeclaration{Action: KeptReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			days := DefaultKeptDays
			if _, given := in["days"]; given {
				days = in.Int("days")
			}
			answer, err := h.Execute(ctx, actor, days)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(answer.Rows))
			for _, row := range answer.Rows {
				out := usecase.Output{"kind": "SLA", "goal": string(row.Goal), "group_id": nil, "ended": row.Ended, "kept": row.Kept}
				if row.OLA {
					out["kind"], out["group_id"] = "OLA", row.Group.String()
				}
				data = append(data, out)
			}
			return usecase.Output{"from": answer.From.UTC(), "to": answer.To.UTC(), "data": data}, nil
		}),
	}, entrySLATarget)
}
