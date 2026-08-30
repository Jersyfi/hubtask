// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"

	"github.com/Jersyfi/hubtask/core/application/condition"
	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/eventbus"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ConsumerName identifies this subscriber in the deduplication and in the logs. Stable across
// versions: renaming it makes every event it has already seen look new.
const ConsumerName = "automation"

// Queue is the slice of the queue this package writes to.
type Queue interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// MatchRules is the outbox consumer that turns one event into one job per rule that wants it.
//
// It runs inside the dispatcher's transaction, which is the reliability argument (ADR-0007): the
// jobs it queues and the record that says this event was handled commit together, so a process that
// dies halfway leaves neither and the event is handed over again.
//
// It decides only *which* rules are interested. Whether a rule's conditions hold, and what its
// actions do, is the engine's - and it is the engine's because that work reaches the use case
// registry, which is not something a subscriber may do from inside the dispatcher's transaction.
//
// **One job per rule, not one per event.** Failure isolation per rule, the queue's backoff per rule,
// and a dead letter naming which rule rather than which batch. An event matching six rules that
// costs one job would make one rule's misconfiguration everybody else's outage.
//
// **It never takes a replay.** eventbus.TakesReplays is opt-in and this type deliberately does not
// implement it: a restore's events are real changes and already-old states, and backup-restore.md
// §8.4 is unambiguous that no rule fires for one.
type MatchRules struct {
	Rules      repository.Matching
	Containers Containers
	Jobs       Queue
	// Conditions renders a rule's dedupe key, which is a template rather than a condition. Optional:
	// a rule with no dedupe expression needs none, and a build without an engine still dispatches
	// every other rule rather than none.
	Conditions expression.Compiler
	Clock      clock.Clock
}

var _ eventbus.Subscriber = MatchRules{}

// Name identifies the subscriber.
func (m MatchRules) Name() string { return ConsumerName }

// Wants reports whether any rule could be triggered by an event of this type.
//
// Every type, because the answer depends on the rules a tenant has written rather than on the type:
// a workspace with a rule on `item.moved` and one without it are the same build. The narrowing is
// the query's, and it is indexed (migration 0053) - asking the database is cheaper than keeping a
// set of subscribed types in step with a table anybody can write.
//
// The three the engine itself publishes are excluded, and that is the loop protection's first line.
// A rule that reacted to a run would be a rule reacting to itself, and the depth limit would stop it
// five hops later rather than never letting it start.
func (m MatchRules) Wants(eventType event.Type) bool {
	switch eventType {
	case event.RuleRunStarted, event.RuleRunFinished, event.RuleRunFailed:
		return false
	default:
		return true
	}
}

// Deliver queues one job per rule that wants this event.
func (m MatchRules) Deliver(ctx context.Context, envelope event.Envelope) error {
	rules, err := m.Rules.ForEventType(ctx, envelope.Type)
	if err != nil {
		return err
	}
	if len(rules) == 0 {
		return nil
	}

	// Resolved once for the whole event rather than once per rule: a workspace with twenty rules on
	// `item.updated` would otherwise make twenty identical reads of one collection.
	location, err := m.locate(ctx, envelope)
	if err != nil {
		return err
	}

	for _, rule := range rules {
		if !covers(rule.Scope, location) {
			continue
		}
		if err := m.enqueue(ctx, rule, envelope); err != nil {
			return err
		}
	}
	return nil
}

// enqueue writes one job for one rule.
func (m MatchRules) enqueue(
	ctx context.Context, rule domain.Rule, envelope event.Envelope,
) error {
	key, err := m.dedupeKey(ctx, rule, envelope)
	if err != nil {
		return err
	}

	_, err = m.Jobs.Enqueue(ctx, queue.Request{
		Kind:     queue.KindAutomationRun,
		TenantID: envelope.TenantID,
		Payload: map[string]any{
			"rule_id": rule.ID.String(),
			// The kind is written even though this producer only ever queues one, because the
			// engine checks it against the rule: a job for a rule that has since been edited into
			// another kind must not run, and a job that named nothing could not be checked.
			"trigger":         string(domain.TriggerEvent),
			"event_id":        envelope.ID.String(),
			"causation_depth": envelope.CausationDepth,
		},
		DedupeKey: key,
	})
	return err
}

// dedupeKey decides what collapses into what.
//
// Without a dedupe expression the key names this rule and this event, so nothing collapses: every
// event gets its own run, which is what a rule means by default.
//
// With one, the key names this rule and the expression's value, and a storm of events meaning the
// same thing becomes one job - which is what automation.md §2 asks the dedupe key for. The job
// table's key is unique per kind while a job is pending or running, so the collapsing is the
// queue's existing machinery rather than a second mechanism.
//
// An expression that cannot be rendered does not fail the delivery. The whole batch would stop, and
// one tenant's misconfigured rule would hold up every other tenant's events; the run is queued
// without collapsing instead, and the engine records what went wrong where somebody can read it.
func (m MatchRules) dedupeKey(
	ctx context.Context, rule domain.Rule, envelope event.Envelope,
) (string, error) {
	unique := ConsumerName + ":" + rule.ID.String() + ":" + envelope.ID.String()
	if rule.Throttle.DedupeKeyExpr == "" || m.Conditions == nil {
		return unique, nil
	}

	program, err := m.Conditions.Compile(
		rule.Throttle.DedupeKeyExpr, condition.RuleEnvironment(), expression.Text)
	if err != nil {
		return unique, nil
	}
	value, err := program.Evaluate(ctx, condition.Values{Envelope: envelope, Now: m.Clock.Now()})
	if err != nil {
		return unique, nil
	}
	return ConsumerName + ":" + rule.ID.String() + ":key:" + value.Text, nil
}

// location is where an event happened, as far as anything can tell from the event itself.
//
// Both may be zero. An event about something that is not in a collection - a label, a template -
// carries no location, and a rule scoped below the tenant does not match it. That is the honest
// answer rather than a guess: a rule scoped to one hub must not fire on a workspace-wide act
// because nothing said otherwise.
type location struct {
	CollectionID shared.ID
	HubID        shared.ID
}

// locate reads the event's collection and the hub above it.
//
// The collection comes from the payload, which every item event carries (`collection_id`) and every
// container event carries as its own identity. The hub is one read, and only when the event is in a
// collection at all.
func (m MatchRules) locate(ctx context.Context, envelope event.Envelope) (location, error) {
	// A hub's own event names the hub directly, which no collection lookup could find: a hub sits
	// under nothing, so asking for its parent would answer the tenant.
	if hubID := condition.HubOf(envelope); !hubID.IsZero() {
		return location{HubID: hubID}, nil
	}

	collectionID := condition.CollectionOf(envelope)
	if collectionID.IsZero() {
		return location{}, nil
	}

	if m.Containers == nil {
		// A rule scoped to a collection can still be matched; one scoped to a hub cannot, and says
		// so by not matching rather than by matching everything.
		return location{CollectionID: collectionID}, nil
	}

	container, err := m.Containers.Find(ctx, collectionID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// Gone between the event and the delivery. Not an error, and it does not let a hub-scoped
		// rule match: a rule scoped to one hub must not fire because nothing could say otherwise.
		return location{CollectionID: collectionID}, nil
	case err != nil:
		return location{}, err
	}
	return location{CollectionID: collectionID, HubID: container.ParentID}, nil
}

// covers reports whether a rule's scope reaches this event.
//
// A tenant-scoped rule reaches everything, which is what the level means. A hub-scoped one reaches
// its collections as well as itself, because a permission held at a hub applies downwards
// (domain-model.md §3.2) and a rule scoped to a hub is scoped to what happens under it.
func covers(scope domain.Scope, at location) bool {
	switch scope.Type {
	case domain.ScopeTenant:
		return true
	case domain.ScopeCollection:
		return !at.CollectionID.IsZero() && scope.ID == at.CollectionID
	case domain.ScopeHub:
		return !at.HubID.IsZero() && scope.ID == at.HubID
	default:
		// A scope this build does not know is a rule it cannot place, and a rule it cannot place
		// does not fire. Unreachable while the aggregate validates the three levels.
		return false
	}
}
