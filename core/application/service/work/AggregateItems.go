// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// AggregateItemsName is the catalogue name (core/application/catalogue).
const AggregateItemsName = "AggregateItems"

// AggregateItems summarises what a query keeps: how many entries, and the sum, average, least or
// greatest of a custom number, money or duration or of a built-in quantity - per value of a field
// and per day, week or month (S-11). The scope, the filter, the permission and the shaping are the
// query's own (QueryItems), so a summary never counts an entry the same query would not list.
//
// The whole workspace is the one scope a query does not take: the collections the actor reads are
// decided here, each by its own path, and handed to the statement as one bound list - the
// narrowing inside the statement that a page of rows cannot have. An entry shared with the actor
// alone, in a collection they hold no role on, is counted as a level list shows it: each such
// entry judged at its own path, and bound as a second list.
type AggregateItems struct {
	Query      QueryItems
	Aggregates repository.ItemAggregates
	// Collections and Reader decide a workspace summary's collections; unwired, it is refused.
	Collections repository.Collections
	Reader      Reader
	// Shared answers the entries shared with the actor alone; unwired, none is counted.
	Shared identityrepo.SharedEntries
	// Workflows answers the statuses a flow quantity counts as started (cycle time, WIP age);
	// unwired, none is, and those measures hold nothing.
	Workflows repository.Workflows
	// Time decides whose time a time summary counts; unwired, the reader's own alone.
	Time TimeReach
	// Pages decides the spaces a pages summary counts; unwired, it is refused.
	Pages PageReach
}

// PageReach decides the spaces a pages summary counts (UC-DOC-01): among the hubs, those whose
// pages the reader may read. A hub the request named alone is asked with its refusal recorded.
type PageReach interface {
	Readable(ctx context.Context, actor appshared.ActorContext, hubs []shared.ID, named bool) ([]shared.ID, error)
}

// TimeReach decides whose time a time summary counts (UC-TIM-04): per collection, whether the
// reader sees other people's time on its entries - the time module's question, not the query's -,
// and whether the workspace shows other people's time only as sums.
type TimeReach interface {
	Reach(ctx context.Context, actor appshared.ActorContext, collections []domain.Container) (shown []bool, sumsOnly bool, err error)
}

// MaxSummaryCollections bounds the collections a workspace summary reads: the bound list is one
// parameter, and past this size a summary belongs to a hub. MaxSummaryShared bounds the entries
// shared with the actor alone the same way.
const (
	MaxSummaryCollections = 2000
	MaxSummaryShared      = 5000
)

// Execute answers the summary.
func (h AggregateItems) Execute(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec, aggregation view.Aggregation,
) (repository.ItemAggregateResult, error) {
	q := h.Query
	if aggregation.CountsPages() {
		return h.pages(ctx, actor, spec, aggregation)
	}
	resolved, err := q.resolvePlaceholders(actor, spec)
	if err != nil {
		return repository.ItemAggregateResult{}, err
	}

	var (
		anchor      repository.Anchor
		path        = []identity.Scope{identity.TenantScope()}
		scope       shared.ID
		collections []domain.Container
	)
	if resolved.Scope.Workspace {
		if anchor, collections, err = h.workspace(ctx, actor, resolved.IncludeArchived); err != nil {
			return repository.ItemAggregateResult{}, err
		}
	} else {
		var container domain.Container
		anchor, container, err = q.anchor(ctx, actor, resolved.Scope)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		path = containerPath(container)
		if err := q.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionRead,
			Path:       path,
			Action:     ItemReadAction,
			TokenScope: itemsRead,
			TargetType: containerTarget,
			TargetID:   container.ID,
		}); err != nil {
			return repository.ItemAggregateResult{}, err
		}
		scope = anchor.CollectionID
		if anchor.Kind == repository.AnchorHub {
			scope = anchor.HubID
		} else {
			collections = []domain.Container{container}
		}
	}
	if aggregation.CountsTime() {
		if aggregation.Sight, err = h.timeSight(ctx, actor, anchor, collections, aggregation); err != nil {
			return repository.ItemAggregateResult{}, err
		}
	}
	// A field hidden from the reader is no more summed than it is filtered by (UC-ID-27 check 3).
	if q.Gate.Sight.Wired() {
		lens, err := q.Gate.Sight.Lens(ctx, q.UnitOfWork, actor)
		if err != nil {
			return repository.ItemAggregateResult{}, err
		}
		keys := append(resolved.CustomKeys(), aggregation.CustomKeys()...)
		hidden, err := lens.HiddenFrom(ctx, path, keys)
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
		shaped, err := shapeCustomFields(ctx, q.Fields, resolved, scope)
		if err != nil {
			return err
		}
		summary, err := shapeAggregation(ctx, q.Fields, aggregation, scope)
		if err != nil {
			return err
		}
		if summary.Quantity.ReadsFlow() {
			if summary, err = h.flow(ctx, summary); err != nil {
				return err
			}
		}
		result, err = h.Aggregates.Aggregate(ctx, repository.ItemSearch{
			Anchor: anchor, Spec: shaped, Language: actor.Locale,
		}, summary, location)
		return err
	})
	return result, err
}

// workspace decides the collections a workspace summary reads: every live collection - archived
// ones too when the query keeps archived entries -, private hubs the actor is in included, kept where reading its entries is permitted at its own path - the
// question QueryItems asks of one collection, asked of each. A collection left out is nobody's
// refusal, so nothing is recorded per row (audit.md §4).
func (h AggregateItems) workspace(
	ctx context.Context, actor appshared.ActorContext, includeArchived bool,
) (repository.Anchor, []domain.Container, error) {
	if err := actor.RequireScope(itemsRead); err != nil {
		return repository.Anchor{}, nil, err
	}
	if h.Collections == nil || h.Reader == nil {
		return repository.Anchor{}, nil, shared.ErrInternal.WithDetail("query.workspace_unwired")
	}

	var (
		rows   []repository.CollectionInHub
		shares []identityrepo.SharedEntry
	)
	err := h.Query.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if h.Shared != nil {
			read, err := h.Shared.SharedAcross(ctx, actor.AccountID, MaxSummaryShared+1)
			if err != nil {
				return err
			}
			if len(read) > MaxSummaryShared {
				return shared.ErrValidation.WithDetail("query.workspace_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxSummaryShared)}).
					WithFields(shared.FieldError{Path: "/scope/workspace", Code: "query.workspace_too_large"})
			}
			shares = read
		}
		page := repository.Page{Size: 200}
		for {
			read, err := h.Collections.InOrder(ctx, repository.CollectionQuery{
				IncludePrivate: true, IncludeArchived: includeArchived, Page: page,
			})
			if err != nil {
				return err
			}
			rows = append(rows, read.Collections...)
			if len(rows) > MaxSummaryCollections {
				return shared.ErrValidation.WithDetail("query.workspace_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxSummaryCollections)}).
					WithFields(shared.FieldError{Path: "/scope/workspace", Code: "query.workspace_too_large"})
			}
			if !read.Info.HasMore {
				return nil
			}
			page.Cursor = read.Info.NextCursor
		}
	})
	if err != nil {
		return repository.Anchor{}, nil, err
	}

	anchor := repository.Anchor{Kind: repository.AnchorCollections, IncludeDescendants: true}
	if len(rows) == 0 {
		return anchor, nil, nil
	}
	paths := make([][]identity.Scope, 0, len(rows))
	collections := make([]domain.Container, 0, len(rows))
	for _, row := range rows {
		paths = append(paths, containerPath(row.Collection))
		collections = append(collections, row.Collection)
	}
	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: containerTarget,
	}, paths)
	if err != nil {
		return repository.Anchor{}, nil, err
	}
	unread := make(map[shared.ID]domain.Container, len(rows))
	for i, row := range rows {
		if i < len(allowed) && allowed[i] {
			anchor.CollectionIDs = append(anchor.CollectionIDs, row.Collection.ID)
			continue
		}
		unread[row.Collection.ID] = row.Collection
	}
	anchor.ItemIDs, err = h.sharedReached(ctx, actor, shares, unread)
	return anchor, collections, err
}

// timeSight decides whose records a time summary counts: the reader's own, and other people's on
// the entries of each collection where the time module says the reader sees them - a hub's
// collections read one by one. Where the workspace shows other people's time only as sums, a
// grouping by person is refused and a small group withheld (UC-TIM-04 check 4).
func (h AggregateItems) timeSight(
	ctx context.Context, actor appshared.ActorContext, anchor repository.Anchor,
	collections []domain.Container, aggregation view.Aggregation,
) (view.TimeSight, error) {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return view.TimeSight{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	sight := view.TimeSight{Reader: actor.AccountID, Shown: []shared.ID{}}
	if h.Time == nil {
		return sight, nil
	}
	if anchor.Kind == repository.AnchorHub {
		err := h.Query.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			page := repository.Page{Size: 200}
			for {
				read, err := h.Query.Containers.List(ctx, repository.ContainerQuery{
					ParentID: anchor.HubID, Type: domain.ContainerCollection, IncludeArchived: true, Page: page,
				})
				if err != nil {
					return err
				}
				collections = append(collections, read.Containers...)
				if len(collections) > MaxSummaryCollections || !read.Info.HasMore {
					return nil
				}
				page.Cursor = read.Info.NextCursor
			}
		})
		if err != nil {
			return view.TimeSight{}, err
		}
	}
	shown, sumsOnly, err := h.Time.Reach(ctx, actor, collections)
	if err != nil {
		return view.TimeSight{}, err
	}
	for i, collection := range collections {
		if i < len(shown) && shown[i] {
			sight.Shown = append(sight.Shown, collection.ID)
		}
	}
	if sumsOnly && len(sight.Shown) > 0 {
		if aggregation.TimeGroup == view.TimeByPerson {
			return view.TimeSight{}, shared.ErrForbidden.WithDetail("time.aggregated_only")
		}
		sight.Withhold = true
	}
	return sight, nil
}

// sharedReached keeps the entries shared with the actor alone that lie in a live collection the
// actor does not read, each judged at its own path - an entry in a readable collection is
// counted already, and one in a collection the summary does not read is not counted at all.
func (h AggregateItems) sharedReached(
	ctx context.Context, actor appshared.ActorContext, shares []identityrepo.SharedEntry,
	unread map[shared.ID]domain.Container,
) ([]shared.ID, error) {
	var (
		candidates []shared.ID
		paths      [][]identity.Scope
	)
	for _, entry := range shares {
		collection, found := unread[entry.CollectionID]
		if !found {
			continue
		}
		candidates = append(candidates, entry.ItemID)
		paths = append(paths, append(containerPath(collection), identity.ItemScope(entry.ItemID)))
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     ItemReadAction,
		TokenScope: itemsRead,
		TargetType: itemTarget,
		On:         access.ItemSubject{Does: service.ItemRead},
	}, paths)
	if err != nil {
		return nil, err
	}
	var kept []shared.ID
	for i, id := range candidates {
		if i < len(allowed) && allowed[i] {
			kept = append(kept, id)
		}
	}
	return kept, nil
}

// pages answers a pages summary: the spaces of one hub or of every hub the reader may read pages
// in, counted in one statement. A page is no entry, so the entries' filter is refused rather than
// ignored, and a collection or an entry holds no space.
func (h AggregateItems) pages(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec, aggregation view.Aggregation,
) (repository.ItemAggregateResult, error) {
	if h.Pages == nil {
		return repository.ItemAggregateResult{}, shared.ErrInternal.WithDetail("query.pages_unwired")
	}
	if spec.Filter != nil {
		return repository.ItemAggregateResult{}, shared.ErrValidation.WithDetail("query.aggregate_pages_unfiltered").
			WithFields(shared.FieldError{Path: "/filter", Code: "query.aggregate_pages_unfiltered"})
	}
	hubScope := shared.ErrValidation.WithDetail("query.aggregate_pages_hub_scope").
		WithFields(shared.FieldError{Path: "/scope", Code: "query.aggregate_pages_hub_scope"})
	if !spec.Scope.ItemID.IsZero() {
		return repository.ItemAggregateResult{}, hubScope
	}
	var hubs []shared.ID
	err := h.Query.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !spec.Scope.Workspace {
			found, err := h.Query.Containers.Find(ctx, spec.Scope.ContainerID)
			if errors.Is(err, shared.ErrNotFound) {
				return scopeNotFound("/scope/container_id", spec.Scope.ContainerID)
			}
			if err != nil {
				return err
			}
			if found.Type != domain.ContainerHub {
				return hubScope
			}
			hubs = []shared.ID{found.ID}
			return nil
		}
		page := repository.Page{Size: 200}
		for {
			read, err := h.Query.Containers.List(ctx, repository.ContainerQuery{
				Type: domain.ContainerHub, IncludeArchived: spec.IncludeArchived, Page: page,
			})
			if err != nil {
				return err
			}
			for _, hub := range read.Containers {
				hubs = append(hubs, hub.ID)
			}
			if len(hubs) > MaxSummaryCollections {
				return shared.ErrValidation.WithDetail("query.workspace_too_large").
					WithParams(map[string]string{"maximum": strconv.Itoa(MaxSummaryCollections)}).
					WithFields(shared.FieldError{Path: "/scope/workspace", Code: "query.workspace_too_large"})
			}
			if !read.Info.HasMore {
				return nil
			}
			page.Cursor = read.Info.NextCursor
		}
	})
	if err != nil {
		return repository.ItemAggregateResult{}, err
	}
	if aggregation.Hubs, err = h.Pages.Readable(ctx, actor, hubs, !spec.Scope.Workspace); err != nil {
		return repository.ItemAggregateResult{}, err
	}
	aggregation.Reader = actor.AccountID
	if h.Query.Clock != nil {
		aggregation.Now = h.Query.Clock.Now()
	}
	location := "UTC"
	if _, err := time.LoadLocation(actor.TimeZone); err == nil && actor.TimeZone != "" {
		location = actor.TimeZone
	}
	var result repository.ItemAggregateResult
	err = h.Query.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		result, err = h.Aggregates.Aggregate(ctx, repository.ItemSearch{Spec: spec, Language: actor.Locale}, aggregation, location)
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
		Name: "scope_workspace", Kind: usecase.KindBool,
		Description: "The whole workspace instead of one hub, collection or entry: every " +
			"collection the caller may read, at most 2000, archived ones with include_archived; " +
			"an entry shared with the caller alone in a collection they do not read is counted " +
			"too, at most 5000 of them.",
	})
	input = append(input, usecase.Field{
		Name: "aggregate", Kind: usecase.KindObject, Required: true,
		Description: "The summary: source (entries, the default, or links - the links whose " +
			"two ends the scope and filter both keep, counted only, grouped by link_type or a " +
			"field of the source end -, or time - the time records on the kept entries, the " +
			"caller's own and other people's where the caller approves time, measured by minutes, " +
			"grouped by work_type, billable, person or a field of the entry, bucketed by day or an " +
			"entry's timestamp; where the workspace shows others' time only as sums, person is " +
			"refused and a group of fewer than three people left out -, or pages - the pages of a " +
			"hub's space or of every readable space for the workspace, counted only, without a " +
			"filter, grouped by owner, template or verification, bucketed by created_at or " +
			"updated_at), function (COUNT, SUM, AVG, MIN or MAX), field (a custom field " +
			"holding a number, money or a duration, or lead_time - creation to completion - or " +
			"scheduled_span - start to due -, both in minutes; not for COUNT), group_by (a field a " +
			"query may group by), and bucket (field, a timestamp, and period: day, week or month, " +
			"cut in the caller's time zone; week_start monday, the default, or sunday).",
	})
	return usecase.Descriptor{
		Name:       AggregateItemsName,
		Permission: service.PermissionRead,
		Summary: "Summarises the entries a query keeps inside one hub, collection or entry, or " +
			"the whole workspace: how many, and the sum, average, least or greatest of a custom " +
			"number, money or duration or of an entry's lead time or scheduled span, " +
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
	spec, aggregation, err := summaryOf(in)
	if err != nil {
		return nil, err
	}

	result, err := h.Execute(ctx, actor, spec, aggregation)
	if err != nil {
		return nil, err
	}
	return aggregateOutput(aggregation, result), nil
}

// summaryOf reads a summary's input - the scope, the filter, the lifecycle flags and the
// aggregation - from the flat use case input a request or a saved view's document is.
func summaryOf(in usecase.Input) (view.Spec, view.Aggregation, error) {
	containerID, err := in.ID("scope_container_id")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	itemID, err := in.ID("scope_item_id")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	includeDescendants := true
	if in.Present("include_descendants") {
		includeDescendants = in.Bool("include_descendants")
	}
	scope, err := view.ParseSummaryScope(
		containerID, itemID, includeDescendants, in.Bool("scope_workspace"), "/scope")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	filter, err := view.ParseFilter(in["filter"], "/filter")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	aggregation, err := view.ParseAggregation(in["aggregate"], "/aggregate")
	if err != nil {
		return view.Spec{}, view.Aggregation{}, err
	}
	return view.Spec{
		Scope: scope, Filter: filter,
		IncludeArchived: in.Bool("include_archived"), IncludeTrashed: in.Bool("include_trashed"),
	}, aggregation, nil
}

// aggregateOutput is a summary's answer as every door returns it.
func aggregateOutput(aggregation view.Aggregation, result repository.ItemAggregateResult) usecase.Output {
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
	return usecase.Output{"data": rows, "truncated": result.Truncated}
}

// ParseSummary reads a summary document as POST /items:aggregate takes it, so that a module that
// stores one - a dashboard's widget - is held to the same grammar at write.
func ParseSummary(in usecase.Input) (view.Spec, view.Aggregation, error) { return summaryOf(in) }

// flow hands a flow quantity the started statuses of the workspace's workflows and the moment of
// asking (UC-GOL-04 check 11).
func (h AggregateItems) flow(ctx context.Context, aggregation view.Aggregation) (view.Aggregation, error) {
	aggregation.Started = []string{}
	if h.Query.Clock != nil {
		aggregation.Now = h.Query.Clock.Now()
	}
	if h.Workflows == nil {
		return aggregation, nil
	}
	rows, err := h.Workflows.List(ctx)
	if err != nil {
		return view.Aggregation{}, err
	}
	aggregation.Started = domain.NewWorkflows(rows).Started()
	return aggregation, nil
}
