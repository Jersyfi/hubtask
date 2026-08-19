// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package event holds the domain events: what happened, in a form other systems can subscribe to.
//
// An event is a public contract (domain-model.md §4). Automation rules, webhook subscribers, n8n
// and Zapier read it, which is why the type name carries a version and why fields may only be
// added - removing or reinterpreting one needs a `.v2` alongside continued delivery of `.v1`.
//
// The wire format is CloudEvents 1.0 (ADR-0007), but that mapping lives in the adapter. What is
// here is the event itself: its type, who caused it, and the business payload.
package event

// Type is the event type, `de.hubtask.<context>.<entity>.<action>.v<n>` (domain-model.md §4).
//
// The context segment is `work` for work management, matching the examples in the schema and in
// automation.md §1. The backlog entry for A-07 wrote `workmanagement`; the shorter form was
// confirmed as the contract, and the backlog was corrected rather than the two documents.
type Type string

const (
	// ContainerCreated announces a new hub or collection. Consumers: automation, webhooks, search.
	ContainerCreated Type = "de.hubtask.work.container.created.v1"
	// ItemCreated announces a new task, work package or activity. One type for all three levels,
	// because they are one aggregate (ADR-0006): a subscriber filters on the payload's `type`
	// rather than on three event names, and a fifth level reaches it without a new subscription.
	// Consumers: automation, search, SSE.
	ItemCreated Type = "de.hubtask.work.item.created.v1"
	// ItemCompleted announces that an item is done. Consumers: automation (the commonest trigger there
	// is), the roll-up itself, ON_COMPLETION recurrence, search, SSE.
	//
	// Announced for a roll-up as much as for a person's click, and a consumer cannot tell the two apart
	// from the type - it reads the causation chain for that. The alternative, a separate event for an
	// automatic completion, would make every rule that reacts to "done" subscribe to two names and
	// forget one.
	ItemCompleted Type = "de.hubtask.work.item.completed.v1"
	// ItemReopened announces that a completed item is open again. Its own type rather than a field on
	// the completed event: a rule that reacts to work being finished must not fire when work is
	// unfinished, and a subscriber filtering on a payload field to avoid that is a subscriber that will
	// eventually not.
	ItemReopened Type = "de.hubtask.work.item.reopened.v1"
	// ItemMoved announces that an item sits somewhere else: under a different parent, in a different
	// collection, or at a different rank among the same siblings. Consumers: kanban automation, search, SSE.
	//
	// One type for all three, because domain-model.md §4 gives it one and names `orderKey` among its
	// payload: a drag within a list and a drag between lists are the same gesture to a person and the same
	// event to a rule. A consumer that cares only about reparenting compares `from_parent_id` with
	// `to_parent_id`.
	ItemMoved Type = "de.hubtask.work.item.moved.v1"
)

// types is the closed set. Everything that needs to know which events exist reads it here - the
// event schemas under api/events/ are reconciled against it by the contract test, so an event
// without a schema, or a schema without an event, is a red build rather than a discovery made by
// a subscriber.
var types = [...]Type{ContainerCreated, ItemCreated, ItemCompleted, ItemReopened, ItemMoved}

// Types returns every defined event type.
func Types() []Type { return types[:] }

// Valid reports whether the type is one of the defined ones.
func (t Type) Valid() bool {
	for _, known := range types {
		if known == t {
			return true
		}
	}
	return false
}

func (t Type) String() string { return string(t) }
