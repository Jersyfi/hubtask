// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// AggregateItemsName is the catalogue name (core/application/catalogue).
const AggregateItemsName = "AggregateItems"

// AggregateItems summarises what a query keeps: how many entries, and the sum, average, least or
// greatest of a custom number, money or duration - per value of a field and per day, week or month
// (S-11). The scope, the filter, the permission and the shaping are the query's own (QueryItems),
// so a summary never counts an entry the same query would not list.
type AggregateItems struct {
	Query      QueryItems
	Aggregates repository.ItemAggregates
}

// Execute answers the summary.
func (h AggregateItems) Execute(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec, aggregation view.Aggregation,
) (repository.ItemAggregateResult, error) {
	q := h.Query
	resolved, err := q.resolvePlaceholders(actor, spec)
	if err != nil {
		return repository.ItemAggregateResult{}, err
	}
	anchor, container, err := q.anchor(ctx, actor, resolved.Scope)
	if err != nil {
		return repository.ItemAggregateResult{}, err
	}
	if err := q.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       containerPath(container),
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
		TargetID:   container.ID,
	}); err != nil {
		return repository.ItemAggregateResult{}, err
	}
	// A field hidden from the reader is no more summed than it is filtered by (UC-ID-27 check 3).
	if q.Gate.Sight.Wired() {
		lens, err := q.Gate.Sight.Lens(ctx, q.UnitOfWork, actor)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		keys := append(resolved.CustomKeys(), aggregation.CustomKeys()...)
		hidden, err := lens.HiddenFrom(ctx, containerPath(container), keys)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		if err := refuseHidden(hidden); err != nil {
			return repository.ItemAggregateResult{}, err
		}
	}

	// The zone resolves as the placeholders' does: one the zone database no longer knows is UTC.
	location := "UTC"
	if _, err := time.LoadLocation(actor.TimeZone); err == nil && actor.TimeZone != "" {
		location = actor.TimeZone
	}

	var result repository.ItemAggregateResult
	err = q.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		scope := anchor.CollectionID
		if anchor.Kind == repository.AnchorHub {
			scope = anchor.HubID
		}
		shaped, err := shapeCustomFields(ctx, q.Fields, resolved, scope)
		if err != nil {
			return err
		}
		summary, err := shapeAggregation(ctx, q.Fields, aggregation, scope)
		if err != nil {
			return err
		}
		result, err = h.Aggregates.Aggregate(ctx, repository.ItemSearch{
			Anchor: anchor, Spec: shaped, Language: actor.Locale,
		}, summary, location)
		return err
	})
	return result, err
}

// shapeAggregation puts the definitions' shapes on the custom fields the aggregation names, as
// shapeCustomFields does for the query.
func shapeAggregation(
	ctx context.Context, fields repository.CustomFields, aggregation view.Aggregation, scope shared.ID,
) (view.Aggregation, error) {
	keys := aggregation.CustomKeys()
	shapes := make(map[string]view.CustomShape, len(keys))
	if len(keys) > 0 {
		if fields == nil {
			return view.Aggregation{}, shared.ErrInternal.WithDetail("query.custom_fields_unwired")
		}
		kinds, err := fields.KindsByKey(ctx, keys, scope)
		if err != nil {
			return view.Aggregation{}, err
		}
		for _, key := range keys {
			shapes[key] = view.ShapeOf(kinds[key])
		}
	}
	return aggregation.Shaped(shapes)
}

// Descriptor is the catalogue entry: the query's scope and filter, and the aggregation.
func (h AggregateItems) Descriptor() usecase.Descriptor {
	query := QueryItems{}.Descriptor()
	input := []usecase.Field{}
	for _, field := range query.Input {
		switch field.Name {
		case "scope_container_id", "scope_item_id", "include_descendants", "filter",
			"include_archived", "include_trashed":
			input = append(input, field)
		}
	}
	input = append(input, usecase.Field{
		Name: "aggregate", Kind: usecase.KindObject, Required: true,
		Description: "The summary: function (COUNT, SUM, AVG, MIN or MAX), field (a custom field " +
			"holding a number, money or a duration; not for COUNT), group_by (a field a query may " +
			"group by), and bucket (field, a timestamp, and period: day, week or month, cut in the " +
			"caller's time zone, a week starting on Monday).",
	})
	return usecase.Descriptor{
		Name:       AggregateItemsName,
		Permission: service.PermissionRead,
		Summary: "Summarises the entries a query keeps inside one hub, collection or entry: how " +
			"many, and the sum, average, least or greatest of a custom number, money or duration, " +
			"optionally per value of a field and per day, week or month. At most 5000 rows; a " +
			"longer answer says it was cut.",
		SideEffects: "None. Reads only.",
		TokenScope:  itemsRead,
		ReadOnly:    true,
		Input:       input,
		Audit: usecase.AuditDeclaration{
			Action: ItemReadAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AggregateItems) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("scope_container_id")
	if err != nil {
		return nil, err
	}
	itemID, err := in.ID("scope_item_id")
	if err != nil {
		return nil, err
	}
	includeDescendants := true
	if in.Present("include_descendants") {
		includeDescendants = in.Bool("include_descendants")
	}
	scope, err := view.ParseScope(containerID, itemID, includeDescendants, "/scope")
	if err != nil {
		return nil, err
	}
	filter, err := view.ParseFilter(in["filter"], "/filter")
	if err != nil {
		return nil, err
	}
	aggregation, err := view.ParseAggregation(in["aggregate"], "/aggregate")
	if err != nil {
		return nil, err
	}
	spec := view.Spec{
		Scope: scope, Filter: filter,
		IncludeArchived: in.Bool("include_archived"), IncludeTrashed: in.Bool("include_trashed"),
	}

	result, err := h.Execute(ctx, actor, spec, aggregation)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(result.Rows))
	for _, row := range result.Rows {
		out := usecase.Output{"count": row.Count, "group": nil, "period": nil, "value": nil}
		if aggregation.IsGrouped() && row.Group != "" {
			out["group"] = row.Group
		}
		if aggregation.IsBucketed() && row.Period != "" {
			out["period"] = row.Period
		}
		if row.Value != nil {
			out["value"] = *row.Value
		}
		rows = append(rows, out)
	}
	return usecase.Output{"data": rows, "truncated": result.Truncated}, nil
}
