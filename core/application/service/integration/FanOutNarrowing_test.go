// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/expression"
)

var (
	homeHub      = shared.ID("01936f2a-7c1e-7000-8000-000000000f31")
	homeList     = shared.ID("01936f2a-7c1e-7000-8000-000000000f32")
	workHub      = shared.ID("01936f2a-7c1e-7000-8000-000000000f33")
	workList     = shared.ID("01936f2a-7c1e-7000-8000-000000000f34")
	narrowedItem = shared.ID("01936f2a-7c1e-7000-8000-000000000f35")
)

// placed is the workspace's two hubs with a collection each, as the container store answers.
type placed map[shared.ID]work.Container

func (p placed) Find(_ context.Context, id shared.ID) (work.Container, error) {
	container, found := p[id]
	if !found {
		return work.Container{}, shared.ErrNotFound
	}
	return container, nil
}

func twoHubs() placed {
	return placed{
		homeHub:  {ID: homeHub, TenantID: tenant, Type: work.ContainerHub},
		homeList: {ID: homeList, TenantID: tenant, Type: work.ContainerCollection, ParentID: homeHub},
		workHub:  {ID: workHub, TenantID: tenant, Type: work.ContainerHub},
		workList: {ID: workList, TenantID: tenant, Type: work.ContainerCollection, ParentID: workHub},
	}
}

// evaluator answers a condition by a Go function over what the expression would be told, so that
// the test reads `data` through the same activation the engine would.
type evaluator map[string]func(ctx context.Context, in expression.Activation) (expression.Value, error)

func (e evaluator) Compile(text string, _ expression.Environment, _ expression.Result) (expression.Program, error) {
	run, found := e[text]
	if !found {
		return nil, shared.ErrValidation.WithDetail(expression.CodeSyntax)
	}
	return program(run), nil
}

type program func(ctx context.Context, in expression.Activation) (expression.Value, error)

func (p program) Evaluate(ctx context.Context, in expression.Activation) (expression.Value, error) {
	return p(ctx, in)
}

const (
	titledShip  = `data.title == "Ship"`
	stopsItself = `data.depth / 0 > 1`
)

func conditions() evaluator {
	return evaluator{
		titledShip: func(ctx context.Context, in expression.Activation) (expression.Value, error) {
			data, _, err := in.Resolve(ctx, "data")
			if err != nil {
				return expression.Value{}, err
			}
			document, _ := data.(map[string]any)
			return expression.Value{Bool: document["title"] == "Ship"}, nil
		},
		stopsItself: func(context.Context, expression.Activation) (expression.Value, error) {
			return expression.Value{}, shared.ErrUnavailable.WithDetail(expression.CodeEvaluationFailed).
				WithParams(map[string]string{"kind": string(expression.FailureDivisionByZero)})
		},
	}
}

// itemIn is an entry's creation in a collection, built by the event's own constructor so that the
// payload is the one the dispatcher really sees.
func itemIn(t *testing.T, collection shared.ID, title string) event.Envelope {
	t.Helper()
	envelope, err := event.NewItemCreated(eventID, work.WorkItem{
		ID: narrowedItem, TenantID: tenant, CollectionID: collection, Type: work.ItemTask, Title: title,
	}, event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func narrowed(t *testing.T, scope domain.WebhookScope, filter string) *harness {
	t.Helper()
	h, _ := narrowingHarness()
	h.writer.Compiler = conditions()
	h.writer.Containers = twoHubs()
	create := validCreate()
	create.Scope, create.Filter = &scope, filter
	if _, err := (CreateWebhookSubscription{Writer: h.writer}).Execute(t.Context(), actor(), create); err != nil {
		t.Fatalf("preparing the subscription: %v", err)
	}
	return h
}

func narrowingFanOut(h *harness, queued *jobs) FanOut {
	out := fanOut(h, queued)
	out.Containers, out.Conditions = twoHubs(), conditions()
	return out
}

// UC-INT-01 check 9: a subscription scoped to a hub receives its collections' events and no other
// hub's; one scoped to a collection receives that collection's alone.
func TestAScopedSubscriptionReceivesOnlyWhatHappensThere(t *testing.T) {
	cases := map[string]struct {
		scope domain.WebhookScope
		at    shared.ID
		want  int
	}{
		"the hub's collection":       {domain.WebhookScope{Level: domain.ScopeHub, ID: homeHub}, homeList, 1},
		"another hub's collection":   {domain.WebhookScope{Level: domain.ScopeHub, ID: homeHub}, workList, 0},
		"the collection itself":      {domain.WebhookScope{Level: domain.ScopeCollection, ID: workList}, workList, 1},
		"a sibling's collection":     {domain.WebhookScope{Level: domain.ScopeCollection, ID: workList}, homeList, 0},
		"the workspace reaches both": {domain.WebhookScope{Level: domain.ScopeTenant}, workList, 1},
	}
	for name, c := range cases {
		h := narrowed(t, c.scope, "")
		queued := &jobs{}
		if err := narrowingFanOut(h, queued).Deliver(t.Context(), itemIn(t, c.at, "Plan")); err != nil {
			t.Fatalf("%s: fanning out: %v", name, err)
		}
		if len(queued.requests) != c.want || len(h.delivered.rows) != c.want {
			t.Errorf("%s: %d jobs and %d rows, want %d", name, len(queued.requests), len(h.delivered.rows), c.want)
		}
	}
}

// An event about no container - a rule's run, an arrival in the jumble - reaches no scoped
// subscription, and a moved entry is judged where it landed.
func TestAnEventAboutNoContainerOrAMoveIsJudgedAsRulesJudgeIt(t *testing.T) {
	h := narrowed(t, domain.WebhookScope{Level: domain.ScopeHub, ID: homeHub}, "")
	h.store.rows[hookID] = withTypes(h.store.rows[hookID], event.ItemCreated, event.ItemMoved, event.RuleRunFailed)
	queued := &jobs{}
	out := narrowingFanOut(h, queued)

	nowhere, err := event.NewEnvelope(eventID, event.RuleRunFailed, tenant, "rule/"+narrowedItem.String(),
		event.Actor{Kind: shared.ActorSystem}, now, event.Cause{}, map[string]any{"rule_id": narrowedItem.String()})
	if err != nil {
		t.Fatalf("building the run's event: %v", err)
	}
	if err := out.Deliver(t.Context(), nowhere); err != nil {
		t.Fatalf("fanning out: %v", err)
	}
	if len(queued.requests) != 0 {
		t.Errorf("an event about no container reached a hub's subscription")
	}

	for _, move := range []struct {
		from, to shared.ID
		want     int
	}{{workList, homeList, 1}, {homeList, workList, 0}} {
		queued.requests = nil
		moved, err := event.NewItemMoved(eventID, work.WorkItem{
			ID: narrowedItem, TenantID: tenant, CollectionID: move.to, Type: work.ItemTask, Title: "Plan",
		}, event.Movement{FromCollectionID: move.from}, event.Actor{Kind: shared.ActorUser, ID: author}, now, event.Cause{})
		if err != nil {
			t.Fatalf("building the move: %v", err)
		}
		if err := out.Deliver(t.Context(), moved); err != nil {
			t.Fatalf("fanning out the move: %v", err)
		}
		if len(queued.requests) != move.want {
			t.Errorf("a move into %s produced %d jobs, want %d", move.to, len(queued.requests), move.want)
		}
	}
}

// A condition delivers what it is true for; one that cannot be evaluated sends nothing, says so in
// the delivery log with the expression's code - never the condition's text - and leaves every other
// subscriber's delivery standing.
func TestAConditionDeliversWhatItHoldsForAndHoldsBackWhatItCannotJudge(t *testing.T) {
	h := narrowed(t, domain.WebhookScope{Level: domain.ScopeTenant}, titledShip)
	queued := &jobs{}
	out := narrowingFanOut(h, queued)

	if err := out.Deliver(t.Context(), itemIn(t, homeList, "Plan")); err != nil {
		t.Fatalf("fanning out: %v", err)
	}
	if len(queued.requests) != 0 || len(h.delivered.rows) != 0 {
		t.Fatalf("an event the condition is false for was delivered")
	}
	if err := out.Deliver(t.Context(), itemIn(t, homeList, "Ship")); err != nil {
		t.Fatalf("fanning out: %v", err)
	}
	if len(queued.requests) != 1 {
		t.Fatalf("an event the condition holds for produced %d jobs", len(queued.requests))
	}

	// A second subscription whose condition stops itself, beside one without a condition.
	broken := h.store.rows[hookID]
	broken.Subscription.Filter = stopsItself
	h.store.rows[hookID] = broken
	plain := broken
	plain.Subscription.ID, plain.Subscription.Filter = secondHook, ""
	h.store.rows[secondHook] = plain
	queued.requests, h.delivered.rows = nil, nil

	if err := out.Deliver(t.Context(), itemIn(t, homeList, "Ship")); err != nil {
		t.Fatalf("an evaluation failure failed the batch: %v", err)
	}
	if len(queued.requests) != 1 || queued.requests[0].Payload["subscription_id"] != secondHook.String() {
		t.Errorf("queued %+v, want the plain subscription's job alone", queued.requests)
	}
	var held []domain.WebhookDelivery
	for _, row := range h.delivered.rows {
		if row.Status == domain.DeliveryFilterFailed {
			held = append(held, row)
		}
	}
	if len(held) != 1 || held[0].SubscriptionID != hookID || held[0].Attempt != 0 ||
		held[0].ErrorCode != expression.CodeEvaluationFailed {
		t.Errorf("held back %+v, want one row with the expression's code", held)
	}

	// No evaluator: held back too, rather than sent past the condition.
	out.Conditions = nil
	h.delivered.rows = nil
	if err := out.Deliver(t.Context(), itemIn(t, homeList, "Ship")); err != nil {
		t.Fatalf("fanning out without an evaluator: %v", err)
	}
	for _, row := range h.delivered.rows {
		if row.SubscriptionID == hookID && row.Status != domain.DeliveryFilterFailed {
			t.Errorf("a condition nobody could evaluate was delivered past: %+v", row)
		}
	}
}

func withTypes(stored repository.StoredSubscription, types ...event.Type) repository.StoredSubscription {
	stored.Subscription.EventTypes = types
	return stored
}
