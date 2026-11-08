// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"testing"

	viewrepo "github.com/Jersyfi/hubtask/core/application/repository/view"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var (
	queueHub   = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a01")
	queueDesk  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a02")
	queuePlain = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a03")
	elsewhere  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a04")
)

// queueTree is the hub, its two collections and a hub the reader may not read.
type queueTree struct{ workrepo.Containers }

func (queueTree) Find(_ context.Context, id shared.ID) (work.Container, error) {
	switch id {
	case queueHub, elsewhere:
		return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerHub}, nil
	case queueDesk, queuePlain:
		return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerCollection, ParentID: queueHub}, nil
	}
	return work.Container{}, shared.ErrNotFound
}

func (queueTree) List(_ context.Context, query workrepo.ContainerQuery) (workrepo.ContainerPage, error) {
	if query.ParentID != queueHub {
		return workrepo.ContainerPage{}, nil
	}
	return workrepo.ContainerPage{Containers: []work.Container{
		{ID: queueDesk, TenantID: tenantID, Type: work.ContainerCollection, ParentID: queueHub},
		{ID: queuePlain, TenantID: tenantID, Type: work.ContainerCollection, ParentID: queueHub},
	}}, nil
}

// portalTypes offers a type the portal takes, so every collection that offers it is a desk.
type portalTypes struct {
	workrepo.EntryTypes
	none bool
}

func (p portalTypes) List(context.Context) ([]work.EntryType, error) {
	if p.none {
		return []work.EntryType{{Key: "task", Level: work.ItemTask, Name: "Task"}}, nil
	}
	return []work.EntryType{{Key: "incident", Level: work.ItemTask, Name: "Incident",
		Form: work.Form{Channels: []work.FormChannelRule{{Channel: work.ChannelPortal}}}}}, nil
}

type queueViews struct {
	viewrepo.SavedViews
	queues []view.SavedView
}

func (q queueViews) ListHubQueues(_ context.Context, _, hub shared.ID) ([]view.SavedView, error) {
	if hub != queueHub {
		return nil, nil
	}
	return q.queues, nil
}

// counting answers each count and keeps what it was asked.
type counting struct {
	asked  []workrepo.ItemSearch
	within []view.Node
	slow   bool
}

func (c *counting) CountWithin(_ context.Context, search workrepo.ItemSearch, within view.Node) (workrepo.ItemCount, error) {
	c.asked, c.within = append(c.asked, search), append(c.within, within)
	if c.slow {
		return workrepo.ItemCount{Uncounted: true}, nil
	}
	return workrepo.ItemCount{Total: 7, Within: len(c.asked)}, nil
}

// keptOut refuses READ on one container and admits everything else.
type keptOut struct{ hub shared.ID }

func (k keptOut) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	if request.TargetID == k.hub {
		return shared.ErrNotFound
	}
	return nil
}

func queueUseCase(views []view.SavedView, types portalTypes, counter *counting, authorizer workservice.Authorizer) ListHubQueues {
	return ListHubQueues{
		Desk:  Desk{Containers: queueTree{}, Authorizer: authorizer, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday)},
		Views: queueViews{queues: views}, EntryTypes: types,
		Query: workservice.QueryItems{
			Containers: queueTree{}, Authorizer: authorizer, UnitOfWork: inline{}, Clock: at(monday), Counter: counter,
		},
	}
}

func queueView(id, scope shared.ID, key, name string, position int) view.SavedView {
	return view.SavedView{
		ID: id, TenantID: tenantID, ScopeType: view.ViewScopeHub, ScopeID: queueHub, OwnerID: handlerID,
		Key: key, Name: name, Layout: view.LayoutListExpanded, Sharing: view.SharingScope,
		Queue: true, QueuePosition: position,
		Query: map[string]any{
			"scope_container_id": scope.String(),
			"filter":             map[string]any{"field": "is_completed", "op": "EQ", "value": false},
		},
	}
}

// UC-SVC-03 check 10's read: a hub with a desk and no queue view answers the four standard
// queues by key, each counted over the hub's desks by the query the queue opens on, with how many
// of them are at risk or breached beside it.
func TestAHubWithADeskAnswersTheStandardQueues(t *testing.T) {
	counter := &counting{}
	queues, err := queueUseCase(nil, portalTypes{}, counter, &gate{allow: true}).Execute(context.Background(), reader(), queueHub)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{QueueMine, QueueUnassigned, QueueBreaching, QueueOpen}
	if len(queues) != len(want) {
		t.Fatalf("answered %+v", queues)
	}
	for i, queue := range queues {
		if queue.Key != want[i] || !queue.ViewID.IsZero() || !queue.Counted || queue.Count != 7 || queue.AtRisk != i+1 {
			t.Errorf("queue %d answered %+v", i, queue)
		}
		search := counter.asked[i]
		if search.Anchor.Kind != workrepo.AnchorHub || search.Anchor.HubID != queueHub {
			t.Errorf("queue %s was not counted at the hub: %+v", queue.Key, search.Anchor)
		}
		desks := search.Spec.Filter.Nodes[0]
		if desks.Field.Name != "collection_id" || len(desks.Values) != 2 {
			t.Errorf("queue %s was not narrowed to the desks: %+v", queue.Key, desks)
		}
		within := counter.within[i]
		if within.Field.Name != view.FieldSLAStanding || len(within.Values) != 2 {
			t.Errorf("at_risk of %s is not counted by the standing: %+v", queue.Key, within)
		}
	}
	mine := counter.asked[0].Spec.Filter.Nodes[2]
	if mine.Field.Name != "assignee_id" || len(mine.Values) != 1 || mine.Values[0].ID != handlerID {
		t.Errorf("mine is not the reader's: %+v", mine)
	}
	if breaching := counter.asked[2].Spec.Filter.Nodes[2]; breaching.Field.Name != view.FieldSLAStanding {
		t.Errorf("breaching is not narrowed by the standing: %+v", breaching)
	}
}

// The hub's views marked as queues come in their order, counted by their own filter; one whose
// anchor the reader may not read, or whose count outlasts its budget, answers no count.
func TestAHubsQueueViewsAreCountedByTheirOwnFilter(t *testing.T) {
	first := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a11")
	second := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000a12")
	views := []view.SavedView{
		queueView(first, queueHub, "mine", "Mine", 1),
		queueView(second, elsewhere, "theirs", "Theirs", 2),
	}
	counter := &counting{}
	queues, err := queueUseCase(views, portalTypes{none: true}, counter, keptOut{hub: elsewhere}).
		Execute(context.Background(), reader(), queueHub)
	if err != nil {
		t.Fatal(err)
	}
	if len(queues) != 2 || queues[0].ViewID != first || queues[0].Name != "Mine" || !queues[0].Counted ||
		queues[1].ViewID != second || queues[1].Counted {
		t.Fatalf("answered %+v", queues)
	}
	if len(counter.asked) != 1 || counter.asked[0].Spec.Filter.Field.Name != "is_completed" {
		t.Errorf("the view's own filter was not the one counted: %+v", counter.asked)
	}

	slow := &counting{slow: true}
	queues, err = queueUseCase(views[:1], portalTypes{}, slow, &gate{allow: true}).Execute(context.Background(), reader(), queueHub)
	if err != nil || len(queues) != 1 || queues[0].Counted {
		t.Fatalf("a count past its budget answered %+v, %v", queues, err)
	}
	out := HubQueueOutput(queues[0])
	if out["count"] != nil || out["at_risk"] != nil || out["view_id"] != first.String() {
		t.Errorf("an uncounted queue is answered as %v", out)
	}
}

// A hub without a desk and without queue views has no queues; a reader who may not read the hub
// is told it does not exist.
func TestAHubWithoutADeskHasNoQueues(t *testing.T) {
	queues, err := queueUseCase(nil, portalTypes{none: true}, &counting{}, &gate{allow: true}).
		Execute(context.Background(), reader(), queueHub)
	if err != nil || len(queues) != 0 {
		t.Fatalf("answered %+v, %v", queues, err)
	}
	if _, err := queueUseCase(nil, portalTypes{}, &counting{}, &gate{allow: false}).
		Execute(context.Background(), reader(), queueHub); !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a reader without READ at the hub: %v", err)
	}
}

// Through the door: the standard queues as the contract's HubQueue, nulls where a standard
// queue has no view.
func TestTheHubsQueuesThroughTheDoor(t *testing.T) {
	doors, err := usecase.NewRegistry(nil, queueUseCase(nil, portalTypes{}, &counting{}, &gate{allow: true}).Descriptor())
	if err != nil {
		t.Fatal(err)
	}
	out, err := doors.Invoke(context.Background(), ListHubQueuesName, reader(), usecase.Input{"hub_id": queueHub.String()})
	if err != nil {
		t.Fatal(err)
	}
	data := out["data"].([]usecase.Output)
	if len(data) != 4 || data[0]["key"] != QueueMine || data[0]["view_id"] != nil || data[0]["name"] != nil ||
		data[0]["count"] != 7 || data[0]["at_risk"] != 1 {
		t.Fatalf("answered %v", data)
	}
}
