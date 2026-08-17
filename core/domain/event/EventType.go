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
)

// types is the closed set. Everything that needs to know which events exist reads it here - the
// event schemas under api/events/ are reconciled against it by the contract test, so an event
// without a schema, or a schema without an event, is a red build rather than a discovery made by
// a subscriber.
var types = [...]Type{ContainerCreated}

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
