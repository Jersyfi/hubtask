// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// ListHubQueuesName is the catalogue name (core/application/catalogue).
const ListHubQueuesName = "ListHubQueues"

// QueuesReadAction is recorded when reading a hub's queues is refused.
const QueuesReadAction audit.Action = "container.queues_read"

const queuesTarget = "container"

// The standard queues a hub with a desk and no queue view answers, by key: the four the handler's
// desk has always shown (user-experience.md §7.4), named by the client (rule 8).
const (
	QueueMine       = "mine"
	QueueUnassigned = "unassigned"
	QueueBreaching  = "breaching"
	QueueOpen       = "open"
)

// ListHubQueues answers a hub's queues with their counts (UC-SVC-03 check 10, navigation concept
// §7.4): the hub's views marked as queues the caller sees, else the four standard queues where
// the hub has a desk. Each is counted by its own query through QueryItems, so a count is asked
// with the reader's permissions exactly as opening the queue asks them (UC-SVC-03 check 3).
type ListHubQueues struct {
	Desk       Desk
	Views      viewrepo.SavedViews
	EntryTypes workrepo.EntryTypes
	// Gate answers what each collection offers, which says whether it acts as a desk.
	Gate work.EntryGate
	// Query counts each queue; its Counter is required.
	Query work.QueryItems
}

// HubQueue is one queue and its counts. ViewID is zero for a standard queue; Counted is false
// where the count outlasted its budget or the view's query could not be asked by the reader.
type HubQueue struct {
	ViewID  shared.ID
	Key     string
	Name    string
	Count   int
	AtRisk  int
	Counted bool
}

// Execute answers the hub's queues in their order.
func (h ListHubQueues) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]HubQueue, error) {
	if err := h.Desk.hub(ctx, actor, hubID, service.PermissionRead, QueuesReadAction, deskRead); err != nil {
		return nil, err
	}

	var (
		saved []view.SavedView
		desks []shared.ID
	)
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if saved, err = h.Views.ListHubQueues(ctx, actor.AccountID, hubID); err != nil || len(saved) > 0 {
			return err
		}
		desks, err = h.desksOf(ctx, actor, hubID)
		return err
	})
	if err != nil {
		return nil, err
	}

	within, err := atRisk()
	if err != nil {
		return nil, err
	}
	queues := make([]HubQueue, 0, max(len(saved), 4))
	if len(saved) > 0 {
		for _, queueView := range saved {
			queue := HubQueue{ViewID: queueView.ID, Key: queueView.Key, Name: queueView.Name}
			spec, err := work.SpecOfView(queueView)
			if err == nil {
				err = h.count(ctx, actor, spec, within, &queue)
			}
			if err != nil && !unanswerable(err) {
				return nil, err
			}
			queues = append(queues, queue)
		}
		return queues, nil
	}
	if len(desks) == 0 {
		return queues, nil
	}
	for _, key := range []string{QueueMine, QueueUnassigned, QueueBreaching, QueueOpen} {
		spec, err := StandardQueue(key, hubID, desks)
		if err != nil {
			return nil, err
		}
		queue := HubQueue{Key: key}
		if err := h.count(ctx, actor, spec, within, &queue); err != nil && !unanswerable(err) {
			return nil, err
		}
		queues = append(queues, queue)
	}
	return queues, nil
}

func (h ListHubQueues) count(
	ctx context.Context, actor appshared.ActorContext, spec view.Spec, within view.Node, queue *HubQueue,
) error {
	counted, err := h.Query.CountWithin(ctx, actor, spec, within)
	if err != nil || counted.Uncounted {
		return err
	}
	queue.Count, queue.AtRisk, queue.Counted = counted.Total, counted.Within, true
	return nil
}

// unanswerable is a queue whose view the reader cannot ask: an anchor out of their reach, a field
// hidden from them, a query a later change made invalid. The list still answers, that one uncounted.
func unanswerable(err error) bool {
	return errors.Is(err, shared.ErrForbidden) || errors.Is(err, shared.ErrNotFound) ||
		errors.Is(err, shared.ErrValidation)
}

// desksOf answers the hub's collections that act as a desk: those offering a type the portal
// takes (UC-SVC-05), as the client's desksOf reads them. At most view.MaxValues, the most one
// query's IN names; a hub holds far fewer desks.
func (h ListHubQueues) desksOf(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) ([]shared.ID, error) {
	rows, err := h.EntryTypes.List(ctx)
	if err != nil {
		return nil, err
	}
	types := workdomain.NewEntryTypes(rows)
	var desks []shared.ID
	query := workrepo.ContainerQuery{ParentID: hubID, Type: workdomain.ContainerCollection, Page: workrepo.Page{Size: work.MaxPageSize}}
	for {
		page, err := h.Desk.Containers.List(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, collection := range page.Containers {
			offered, err := h.Gate.OfferedTaskTypes(ctx, collection)
			if err != nil {
				return nil, err
			}
			if len(domain.RequestCatalogue(types, offered, nil, actor.Locale)) > 0 && len(desks) < view.MaxValues {
				desks = append(desks, collection.ID)
			}
		}
		if !page.Info.HasMore {
			return desks, nil
		}
		query.Page.Cursor = page.Info.NextCursor
	}
}

// StandardQueue is a standard queue's query at a hub: the open entries of its desks, narrowed by
// the queue - the query the client's queueQuery asks, so the count is the one the queue opens on.
func StandardQueue(key string, hubID shared.ID, desks []shared.ID) (view.Spec, error) {
	ids := make([]any, 0, len(desks))
	for _, desk := range desks {
		ids = append(ids, desk.String())
	}
	nodes := []any{
		map[string]any{"op": "IN", "field": "collection_id", "value": ids},
		map[string]any{"op": "EQ", "field": "is_completed", "value": false},
	}
	switch key {
	case QueueMine:
		nodes = append(nodes, map[string]any{"op": "EQ", "field": "assignee_id", "value": string(view.PlaceholderMe)})
	case QueueUnassigned:
		nodes = append(nodes, map[string]any{"op": "IS_NULL", "field": "assignee_id"})
	case QueueBreaching:
		// A breached promise is still the handler's to keep first, so it stays in the queue.
		nodes = append(nodes, standingsAtRisk())
	case QueueOpen:
	default:
		return view.Spec{}, shared.ErrInternal.WithDetail("queues.key_unknown")
	}
	filter, err := view.ParseFilter(map[string]any{"op": "AND", "nodes": nodes}, "/filter")
	if err != nil {
		return view.Spec{}, err
	}
	scope, err := view.ParseScope(hubID, "", true, "/scope")
	if err != nil {
		return view.Spec{}, err
	}
	return view.Spec{Scope: scope, Filter: filter, Count: view.CountExact}, nil
}

// atRisk is the filter `at_risk` counts by: an entry whose worst goal is at risk or breached.
func atRisk() (view.Node, error) {
	node, err := view.ParseFilter(standingsAtRisk(), "/within")
	if err != nil || node == nil {
		return view.Node{}, shared.ErrInternal.WithDetail("queues.within_invalid").WithCause(err)
	}
	return *node, nil
}

func standingsAtRisk() map[string]any {
	return map[string]any{"op": "IN", "field": view.FieldSLAStanding, "value": []any{"AT_RISK", "BREACHED"}}
}

// HubQueueOutput is a queue as every door answers it (schema HubQueue).
func HubQueueOutput(queue HubQueue) usecase.Output {
	out := usecase.Output{"view_id": nil, "key": queue.Key, "name": nil, "count": nil, "at_risk": nil}
	if !queue.ViewID.IsZero() {
		out["view_id"], out["name"] = queue.ViewID.String(), queue.Name
	}
	if queue.Counted {
		out["count"], out["at_risk"] = queue.Count, queue.AtRisk
	}
	return out
}

// Descriptor is the catalogue entry.
func (h ListHubQueues) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListHubQueuesName, Permission: service.PermissionRead,
		Summary: "Lists a hub's queues with their counts: the hub's saved views marked as queues the caller " +
			"sees, by their place, each with how many entries its query holds and how many of them are " +
			"AT_RISK or BREACHED, counted with the caller's own permissions. A hub with a desk and no queue " +
			"view answers the standard queues mine, unassigned, breaching and open by key; a count that " +
			"cannot be made in two seconds, or whose view the caller cannot ask, is null.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."},
		},
		Audit: usecase.AuditDeclaration{Action: QueuesReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			queues, err := h.Execute(ctx, actor, hub)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(queues))
			for _, queue := range queues {
				data = append(data, HubQueueOutput(queue))
			}
			return usecase.Output{"data": data}, nil
		}),
	}, queuesTarget)
}
