// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// AggregateViewName is the catalogue name.
const AggregateViewName = "AggregateView"

// AggregateView runs the summary a saved view stores (S-11): the document's scope, filter and
// aggregation, through AggregateItems, under the caller's own authorisation - two readers of one
// shared summary view see two different numbers, exactly as two exports differ (ExportView).
type AggregateView struct {
	Views      viewrepo.SavedViews
	Containers repository.Containers
	Permits    Permitting
	Aggregate  AggregateItems
	UnitOfWork persistence.UnitOfWork
}

// Execute answers the view's summary.
func (h AggregateView) Execute(
	ctx context.Context, actor appshared.ActorContext, viewID shared.ID,
) (view.Aggregation, repository.ItemAggregateResult, error) {
	if err := actor.RequireScope(viewsRead); err != nil {
		return view.Aggregation{}, repository.ItemAggregateResult{}, err
	}
	if viewID.IsZero() {
		return view.Aggregation{}, repository.ItemAggregateResult{}, viewNotFound(viewID)
	}
	var saved view.SavedView
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Views.Find(ctx, viewID)
		saved = found
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return view.Aggregation{}, repository.ItemAggregateResult{}, viewNotFound(viewID)
		}
		return view.Aggregation{}, repository.ItemAggregateResult{}, err
	}
	visible, err := viewVisibleTo(ctx, h.UnitOfWork, h.Containers, h.Permits, actor, saved)
	if err != nil {
		return view.Aggregation{}, repository.ItemAggregateResult{}, err
	}
	if !visible {
		return view.Aggregation{}, repository.ItemAggregateResult{}, viewNotFound(saved.ID)
	}
	if !saved.IsSummary() {
		return view.Aggregation{}, repository.ItemAggregateResult{}, shared.ErrValidation.
			WithDetail("views.aggregate_missing").
			WithFields(shared.FieldError{Path: "/query/aggregate", Code: "views.aggregate_missing"})
	}

	// A stored document that no longer parses answers the grammar's own code, as an export does.
	spec, aggregation, err := summaryOf(usecase.Input(saved.Query))
	if err != nil {
		return view.Aggregation{}, repository.ItemAggregateResult{}, err
	}
	result, err := h.Aggregate.Execute(ctx, actor, spec, aggregation)
	return aggregation, result, err
}

// Descriptor is the catalogue entry.
func (h AggregateView) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AggregateViewName,
		Permission: service.PermissionRead,
		Summary: "The summary a saved view stores - its scope, filter and aggregate - under the " +
			"caller's own authorisation, as POST /items:aggregate answers it. A view that stores " +
			"no aggregate is refused.",
		SideEffects: "None. Reads only.",
		TokenScope:  viewsRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "view_id", Kind: usecase.KindID, Required: true,
				Description: "The saved view. One the caller may read - a view they cannot see " +
					"answers not found, exactly as reading it would.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AggregateView) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	viewID, err := in.ID("view_id")
	if err != nil {
		return nil, err
	}
	aggregation, result, err := h.Execute(ctx, actor, viewID)
	if err != nil {
		return nil, err
	}
	return aggregateOutput(aggregation, result), nil
}
