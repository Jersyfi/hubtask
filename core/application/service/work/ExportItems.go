// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ExportItemsName = "ExportItems"

	// ItemsExportedAction is the audit code of an export without a saved view. A bulk read of
	// somebody's work, exactly as a view's export is, and recorded the same way (audit.md §2).
	ItemsExportedAction audit.Action = "items.exported"
)

// ExportItems renders a query's result whole, without a saved view: a collection as it is
// filtered on screen, handed over as a file.
//
// The permission is QueryItems' own: the query is checked against its scope once, under the
// caller's authorisation, so a refusal is a refusal rather than an empty file. The walk and the
// cap are the view export's (walkToCap), so the same question exports the same rows whether or
// not somebody saved it first.
type ExportItems struct {
	Query      QueryItems
	ItemLabels repository.ItemLabels
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// ExportedItems is what an export without a view answers.
type ExportedItems struct {
	Items       []domain.WorkItem
	Labels      map[shared.ID][]shared.ID
	Truncated   bool
	GeneratedAt time.Time
}

// Execute selects the rows and records the read.
func (h ExportItems) Execute(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec,
) (ExportedItems, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return ExportedItems{}, err
	}
	items, truncated, err := walkToCap(ctx, h.Query, actor, spec)
	if err != nil {
		return ExportedItems{}, err
	}
	carried, err := labelsOf(ctx, h.UnitOfWork, h.ItemLabels, actor, true, items)
	if err != nil {
		return ExportedItems{}, err
	}
	exported := ExportedItems{
		Items: items, Labels: carried, Truncated: truncated, GeneratedAt: h.Clock.Now(),
	}
	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.record(ctx, actor, spec.Scope, exported)
	}); err != nil {
		return ExportedItems{}, err
	}
	return exported, nil
}

// record writes the audit entry against the scope that was exported. What it records is how much
// was read rather than what was in it: the rows are content and the count is not (rule 10).
func (h ExportItems) record(
	ctx context.Context, actor appshared.ActorContext, scope view.Scope, exported ExportedItems,
) error {
	targetType, targetID := containerTarget, scope.ContainerID
	if !scope.ItemID.IsZero() {
		targetType, targetID = itemTarget, scope.ItemID
	}
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: exported.GeneratedAt,
		Action:     ItemsExportedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: targetType,
		TargetID:   targetID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "rows", Classification: audit.Open, To: strconv.Itoa(len(exported.Items))},
			audit.Change{Field: "truncated", Classification: audit.Open, To: boolText(exported.Truncated)},
		),
	})
}

// Descriptor is the catalogue entry.
func (h ExportItems) Descriptor() usecase.Descriptor {
	query := QueryItems{}.Descriptor()
	input := make([]usecase.Field, 0, len(query.Input))
	for _, field := range query.Input {
		// The export walks every page itself and renders rows, never groups.
		switch field.Name {
		case "cursor", "size", "count", "group_by", "expand_labels":
			continue
		}
		input = append(input, field)
	}
	return usecase.Descriptor{
		Name: ExportItemsName,
		Summary: "The result of a query inside one hub, collection or entry, whole and without " +
			"saving a view first - the same filter, order and scope the query takes. Under the " +
			"caller's own authorisation; synchronous and bounded at max_export_rows entries, and " +
			"a result that reached the cap says so. The rows come back as entries; which file " +
			"they become is the channel's business, and REST offers CSV, JSON and ICS.",
		SideEffects: "Writes an audit entry, because an export is a bulk read of somebody's " +
			"work. Changes nothing.",
		TokenScope: itemsRead,
		ReadOnly:   true,
		Input:      input,
		Audit: usecase.AuditDeclaration{
			Action: ItemsExportedAction, TargetType: containerTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Reading is not a change to an entry, and the item history is what changed.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ExportItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	spec, err := specOf(in)
	if err != nil {
		return nil, err
	}
	exported, err := h.Execute(ctx, actor, spec)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"generated_at": exported.GeneratedAt.UTC(),
		"count":        len(exported.Items),
		"truncated":    exported.Truncated,
		"rows":         rowsOf(exported.Items, exported.Labels, nil),
	}, nil
}
