// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	// ruleID here is the fixture's, distinct from the one the writer tests mint.
	ruleID       = shared.ID("01936f2a-7c1e-7000-8000-0000000000c0")
	collectionID = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	otherHubID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000c3")
	itemID       = shared.ID("01936f2a-7c1e-7000-8000-0000000000c4")
)

// matching answers which rules want an event type.
type matching struct{ rules []domain.Rule }

func (m matching) ForEventType(context.Context, event.Type) ([]domain.Rule, error) {
	return m.rules, nil
}

func (m matching) ByTriggerKind(
	_ context.Context, kind domain.TriggerKind,
) ([]domain.Rule, error) {
	var of []domain.Rule
	for _, rule := range m.rules {
		if rule.Trigger.Kind == kind {
			of = append(of, rule)
		}
	}
	return of, nil
}

// jobs records what was queued, which is the whole of what this subscriber does.
type jobs struct{ queued []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.queued = append(j.queued, request)
	return shared.ID("01936f2a-7c1e-7000-8000-0000000000f1"), nil
}

// containers answers where a collection sits, and counts the asking - which is how "resolved once
// per event rather than once per rule" is proved rather than asserted.
type containers struct {
	rows  map[shared.ID]work.Container
	asked int
}

func (c *containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	c.asked++
	container, found := c.rows[id]
	if !found {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return container, nil
}

func ruleAt(scope domain.Scope, id shared.ID) domain.Rule {
	return domain.Rule{
		ID: id, TenantID: tenant, Scope: scope, RunAs: serviceID, Enabled: true,
		Trigger: domain.Trigger{Kind: domain.TriggerEvent, EventType: event.ItemUpdated},
		Actions: []domain.Action{{Kind: "ADD_LABEL"}},
	}
}

func itemEvent() event.Envelope {
	return event.Envelope{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), Type: event.ItemUpdated,
		TenantID: tenant, Subject: "item/" + itemID.String(), OccurredAt: now,
		Payload: map[string]any{"collection_id": collectionID.String()},
	}
}

func newMatcher(rules []domain.Rule) (MatchRules, *jobs, *containers) {
	queued := &jobs{}
	places := &containers{rows: map[shared.ID]work.Container{
		collectionID: {ID: collectionID, Type: work.ContainerCollection, ParentID: hubID},
	}}
	return MatchRules{
		Rules: matching{rules: rules}, Containers: places, Jobs: queued,
		Clock: clock.Fixed(now),
	}, queued, places
}

// One job per matching rule, not one per event: failure isolation per rule, the queue's backoff per
// rule, and a dead letter naming which rule rather than which batch.
func TestOneEventBecomesOneJobPerMatchingRule(t *testing.T) {
	first := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-000000000101"))
	second := ruleAt(domain.Scope{Type: domain.ScopeTenant}, shared.ID("01936f2a-7c1e-7000-8000-000000000102"))
	matcher, queued, _ := newMatcher([]domain.Rule{first, second})

	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 2 {
		t.Fatalf("%d jobs, want one per rule", len(queued.queued))
	}
	for i, request := range queued.queued {
		if request.Kind != queue.KindAutomationRun {
			t.Errorf("job %d is of kind %q", i, request.Kind)
		}
		if request.TenantID != tenant {
			t.Errorf("job %d names tenant %s", i, request.TenantID)
		}
		if request.Payload["event_id"] != itemEvent().ID.String() {
			t.Errorf("job %d names event %v", i, request.Payload["event_id"])
		}
	}
	if queued.queued[0].Payload["rule_id"] == queued.queued[1].Payload["rule_id"] {
		t.Error("both jobs name the same rule")
	}
	// Different keys, so nothing collapses: two rules reacting to one event are two runs.
	if queued.queued[0].DedupeKey == queued.queued[1].DedupeKey {
		t.Error("two rules share a dedupe key")
	}
}

// A rule scoped below the tenant fires only where it is scoped.
func TestAScopedRuleOnlyMatchesItsOwnPlace(t *testing.T) {
	cases := map[string]struct {
		scope domain.Scope
		want  bool
	}{
		"the tenant":         {domain.Scope{Type: domain.ScopeTenant}, true},
		"its own collection": {domain.Scope{Type: domain.ScopeCollection, ID: collectionID}, true},
		"another collection": {domain.Scope{Type: domain.ScopeCollection, ID: otherHubID}, false},
		"the hub above it":   {domain.Scope{Type: domain.ScopeHub, ID: hubID}, true},
		"another hub":        {domain.Scope{Type: domain.ScopeHub, ID: otherHubID}, false},
		"a level nobody has": {domain.Scope{Type: domain.ScopeType("GALAXY")}, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			matcher, queued, _ := newMatcher([]domain.Rule{ruleAt(tc.scope, ruleID)})

			if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if fired := len(queued.queued) == 1; fired != tc.want {
				t.Errorf("queued %d jobs, want fired=%v", len(queued.queued), tc.want)
			}
		})
	}
}

// A workspace with twenty rules on one event type would otherwise make twenty identical reads of
// one collection.
func TestTheEventsPlaceIsResolvedOncePerEvent(t *testing.T) {
	var rules []domain.Rule
	for i := range 5 {
		rules = append(rules, ruleAt(domain.Scope{Type: domain.ScopeHub, ID: hubID},
			shared.ID("01936f2a-7c1e-7000-8000-00000000020"+string(rune('0'+i)))))
	}
	matcher, queued, places := newMatcher(rules)

	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 5 {
		t.Fatalf("%d jobs, want five", len(queued.queued))
	}
	if places.asked != 1 {
		t.Errorf("the collection was read %d times for one event", places.asked)
	}
}

// An event about something that is not in a collection carries no place, and a rule scoped below
// the tenant does not match it - the honest answer rather than a guess.
func TestAnEventWithNoPlaceMatchesOnlyTenantRules(t *testing.T) {
	placeless := itemEvent()
	placeless.Payload = map[string]any{}

	for name, scope := range map[string]domain.Scope{
		"a hub rule":        {Type: domain.ScopeHub, ID: hubID},
		"a collection rule": {Type: domain.ScopeCollection, ID: collectionID},
	} {
		t.Run(name, func(t *testing.T) {
			matcher, queued, _ := newMatcher([]domain.Rule{ruleAt(scope, ruleID)})

			if err := matcher.Deliver(context.Background(), placeless); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(queued.queued) != 0 {
				t.Errorf("a scoped rule fired on an event with no place")
			}
		})
	}

	t.Run("a tenant rule", func(t *testing.T) {
		matcher, queued, _ := newMatcher(
			[]domain.Rule{ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)})

		if err := matcher.Deliver(context.Background(), placeless); err != nil {
			t.Fatalf("delivering: %v", err)
		}
		if len(queued.queued) != 1 {
			t.Error("a tenant rule did not fire")
		}
	})
}

// A hub sits under nothing, so a hub's own event names the hub directly rather than through a
// collection lookup that would answer the tenant.
func TestAHubsOwnEventMatchesAHubScopedRule(t *testing.T) {
	hubEvent := itemEvent()
	hubEvent.Type = event.ContainerRenamed
	hubEvent.Subject = "container/" + hubID.String()
	hubEvent.Payload = map[string]any{"id": hubID.String(), "type": string(work.ContainerHub)}

	matcher, queued, places := newMatcher(
		[]domain.Rule{ruleAt(domain.Scope{Type: domain.ScopeHub, ID: hubID}, ruleID)})

	if err := matcher.Deliver(context.Background(), hubEvent); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 1 {
		t.Error("a hub-scoped rule did not fire on its own hub's event")
	}
	if places.asked != 0 {
		t.Errorf("the hub's own event cost %d container reads", places.asked)
	}
}

// A collection names itself, which no `collection_id` key would carry.
func TestACollectionsOwnEventMatchesACollectionScopedRule(t *testing.T) {
	collectionEvent := itemEvent()
	collectionEvent.Type = event.ContainerRenamed
	collectionEvent.Payload = map[string]any{
		"id": collectionID.String(), "type": string(work.ContainerCollection),
	}

	matcher, queued, _ := newMatcher([]domain.Rule{
		ruleAt(domain.Scope{Type: domain.ScopeCollection, ID: collectionID}, ruleID),
	})

	if err := matcher.Deliver(context.Background(), collectionEvent); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(queued.queued) != 1 {
		t.Error("a collection-scoped rule did not fire on its own collection's event")
	}
}

// Without a dedupe expression nothing collapses: every event gets its own run, which is what a rule
// means by default.
func TestWithoutADedupeExpressionEveryEventGetsItsOwnRun(t *testing.T) {
	matcher, queued, _ := newMatcher(
		[]domain.Rule{ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)})

	first := itemEvent()
	second := itemEvent()
	second.ID = shared.ID("01936f2a-7c1e-7000-8000-0000000000e2")

	for _, envelope := range []event.Envelope{first, second} {
		if err := matcher.Deliver(context.Background(), envelope); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}
	if len(queued.queued) != 2 {
		t.Fatalf("%d jobs, want two", len(queued.queued))
	}
	if queued.queued[0].DedupeKey == queued.queued[1].DedupeKey {
		t.Error("two events collapsed into one job without a dedupe expression")
	}
	for _, request := range queued.queued {
		if !strings.Contains(request.DedupeKey, ruleID.String()) {
			t.Errorf("the key %q does not name the rule", request.DedupeKey)
		}
	}
}

// With one, a storm of events meaning the same thing becomes one job - the queue's own uniqueness
// does the collapsing.
func TestADedupeExpressionCollapsesAStorm(t *testing.T) {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Throttle = domain.Throttle{DedupeKeyExpr: "event.subject"}

	matcher, queued, _ := newMatcher([]domain.Rule{rule})
	matcher.Conditions = renderer{}

	for i := range 3 {
		envelope := itemEvent()
		envelope.ID = shared.ID("01936f2a-7c1e-7000-8000-00000000030" + string(rune('0'+i)))
		if err := matcher.Deliver(context.Background(), envelope); err != nil {
			t.Fatalf("delivering: %v", err)
		}
	}

	if len(queued.queued) != 3 {
		t.Fatalf("%d jobs enqueued, want three requests", len(queued.queued))
	}
	// Three requests, one key: the queue collapses them, which is the mechanism rather than this
	// subscriber counting.
	for _, request := range queued.queued[1:] {
		if request.DedupeKey != queued.queued[0].DedupeKey {
			t.Errorf("three events about one subject produced the keys %q and %q",
				queued.queued[0].DedupeKey, request.DedupeKey)
		}
	}
}

// The whole batch would stop, and one tenant's misconfigured rule would hold up every other
// tenant's events.
func TestADedupeExpressionThatCannotBeRenderedDoesNotStopTheBatch(t *testing.T) {
	rule := ruleAt(domain.Scope{Type: domain.ScopeTenant}, ruleID)
	rule.Throttle = domain.Throttle{DedupeKeyExpr: "item."}

	matcher, queued, _ := newMatcher([]domain.Rule{rule})
	matcher.Conditions = renderer{}

	if err := matcher.Deliver(context.Background(), itemEvent()); err != nil {
		t.Fatalf("a rule with an unrenderable key stopped the delivery: %v", err)
	}
	if len(queued.queued) != 1 {
		t.Fatalf("%d jobs, want the run queued without collapsing", len(queued.queued))
	}
	if !strings.Contains(queued.queued[0].DedupeKey, itemEvent().ID.String()) {
		t.Errorf("the key %q collapsed anyway", queued.queued[0].DedupeKey)
	}
}

// backup-restore.md §8.4: no rule fires for a restore's events. The engine never asks, which is the
// opt-in working rather than a filter somebody has to remember.
func TestTheEngineNeverAsksForReplays(t *testing.T) {
	var subscriber eventbus.Subscriber = MatchRules{}

	if _, asks := subscriber.(eventbus.TakesReplays); asks {
		t.Error("the automation engine opted into replays")
	}
}

// A rule that reacted to a run would be a rule reacting to itself, and the depth limit would stop it
// five hops later rather than never letting it start.
func TestTheEngineDoesNotReactToItsOwnEvents(t *testing.T) {
	matcher, _, _ := newMatcher(nil)

	for _, own := range []event.Type{
		event.RuleRunStarted, event.RuleRunFinished, event.RuleRunFailed,
	} {
		if matcher.Wants(own) {
			t.Errorf("the engine wants %s, which it publishes itself", own)
		}
	}
	if !matcher.Wants(event.ItemUpdated) {
		t.Error("the engine does not want an ordinary event")
	}
}

// renderer is the expression port as this file needs it: a template that reads one field of the
// event, and a refusal for the shape the tests use to mean "broken".
type renderer struct{}

func (renderer) Compile(text string, _ expression.Environment, _ expression.Result) (expression.Program, error) {
	if strings.HasSuffix(strings.TrimSpace(text), ".") {
		return nil, shared.ErrValidation.WithDetail("expression.syntax")
	}
	return rendered{}, nil
}

type rendered struct{}

func (rendered) Evaluate(ctx context.Context, in expression.Activation) (expression.Value, error) {
	value, found, err := in.Resolve(ctx, "event")
	if err != nil || !found {
		return expression.Value{}, err
	}
	document, _ := value.(map[string]any)
	subject, _ := document["subject"].(string)
	return expression.Value{Text: subject}, nil
}
