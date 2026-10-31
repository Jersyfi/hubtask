// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

const (
	ReadEntrySLAName = "ReadEntrySla"
	ReportSLAName    = "ReportSla"

	entrySLATarget = "entry_sla"
	// MaxReportSpan bounds a report's period, so that it stays one indexed scan.
	MaxReportSpan = 366 * 24 * time.Hour
)

// RecordOutput is one goal's standing on an entry (api/openapi.yaml, schema EntrySlaGoal).
func RecordOutput(r domain.EntrySLA) usecase.Output {
	out := usecase.Output{
		"goal": string(r.Goal), "policy_key": r.PolicyKey, "standing": string(r.Standing),
		"target_seconds": int64(r.Target / time.Second), "armed_at": r.ArmedAt.UTC(),
		"due_at": nil, "at_risk_at": nil, "met_at": nil, "breached_at": nil,
	}
	for field, value := range map[string]*time.Time{
		"due_at": r.DueAt, "at_risk_at": r.AtRiskAt, "met_at": r.MetAt, "breached_at": r.BreachedAt,
	} {
		if value != nil {
			out[field] = value.UTC()
		}
	}
	return out
}

// ReadEntrySla answers how an entry's goals stand (UC-SVC-02 check 6): for whoever may read the
// entry, so the handler and the requester see the same clock (P-11).
//
//nolint:revive // var-naming: a use case type is named as the catalogue names it
type ReadEntrySla struct {
	Desk       Desk
	Items      workrepo.Items
	Containers workrepo.Containers
}

// Execute answers the entry's goals, by kind; none when no policy measures it.
func (h ReadEntrySla) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) ([]domain.EntrySLA, error) {
	if err := actor.RequireScope(deskRead); err != nil {
		return nil, err
	}
	var (
		item       = struct{ assignee, collection shared.ID }{}
		collection []identity.Scope
		records    []domain.EntrySLA
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		item.assignee, item.collection = found.AssigneeID, found.CollectionID
		container, err := h.Containers.Find(ctx, found.CollectionID)
		if err != nil {
			return err
		}
		collection = service.ContainerScopes(container)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := h.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: collection, Action: EntrySLAReadAction,
		TokenScope: deskRead, TargetType: entrySLATarget, TargetID: itemID,
		On: access.ItemSubject{Does: service.ItemRead, ID: itemID, Assignee: item.assignee},
	}); err != nil {
		return nil, err
	}
	err = h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		records, err = h.Desk.Records.OfItem(ctx, itemID)
		return err
	})
	return records, err
}

// Descriptor is the catalogue entry.
func (h ReadEntrySla) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadEntrySLAName, Permission: service.PermissionRead,
		Summary: "How an entry's SLA goals stand: per goal its policy, target in working seconds, whether it " +
			"runs, stands still, was met or breached, when it falls due and when it was at risk.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."},
		},
		Audit: usecase.AuditDeclaration{Action: EntrySLAReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			records, err := h.Execute(ctx, actor, itemID)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(records))
			for _, r := range records {
				data = append(data, RecordOutput(r))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, entrySLATarget)
}

// ReportSla counts how the goals armed in a period stand, per policy and goal (UC-SVC-02 check 7).
// The workspace's configurers read it: it counts entries across every hub, private ones included,
// and names none of them.
//
//nolint:revive // var-naming: a use case type is named as the catalogue names it
type ReportSla struct{ Desk Desk }

// Execute counts the goals armed in [from, to).
func (h ReportSla) Execute(ctx context.Context, actor appshared.ActorContext, from, to time.Time) ([]domain.ReportRow, error) {
	if !to.After(from) || to.Sub(from) > MaxReportSpan {
		return nil, malformed("/to", "sla_reports.period_invalid")
	}
	if err := h.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure, Path: []identity.Scope{identity.TenantScope()},
		Action: SLAReportReadAction, TokenScope: deskRead, TargetType: entrySLATarget, TargetID: actor.TenantID,
	}); err != nil {
		return nil, err
	}
	var rows []domain.ReportRow
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		rows, err = h.Desk.Records.Report(ctx, from, to)
		return err
	})
	return rows, err
}

// Descriptor is the catalogue entry.
func (h ReportSla) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReportSLAName, Permission: service.PermissionStructure,
		Summary: "Counts the SLA goals armed in a period - at most a year - per policy, goal and standing: met, " +
			"breached, running, standing still. Names no entry.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "from", Kind: usecase.KindString, Required: true, Format: usecase.FormatDateTime, Description: "The period's start, inclusive."},
			{Name: "to", Kind: usecase.KindString, Required: true, Format: usecase.FormatDateTime, Description: "The period's end, exclusive."},
		},
		Audit: usecase.AuditDeclaration{Action: SLAReportReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			from, errFrom := time.Parse(time.RFC3339, in.String("from"))
			to, errTo := time.Parse(time.RFC3339, in.String("to"))
			if errFrom != nil || errTo != nil {
				return nil, malformed("/from", "sla_reports.period_invalid")
			}
			rows, err := h.Execute(ctx, actor, from, to)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(rows))
			for _, r := range rows {
				data = append(data, usecase.Output{
					"policy_key": r.PolicyKey, "goal": string(r.Goal), "standing": string(r.Standing), "entries": r.Entries,
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	}, entrySLATarget)
}
