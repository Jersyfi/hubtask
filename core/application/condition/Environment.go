// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package condition is the vocabulary an expression may use, and the shape of what it is told.
//
// Two engines evaluate conditions - the rule engine and the retention sweep - and they read the same
// language (ADR-0009, automation.md §1.2, data-retention.md §2). This package is what makes that one
// statement: the names, and the projection of an entry into the document an expression reads. Two
// copies would be two dialects, and the day they diverged nobody would find out from a test.
//
// The projection is deliberately narrow. Every field named here becomes part of what a rule written
// today may depend on, so adding one is a promise and removing one is a breaking change - which is
// why this is a written list rather than a reflection over the aggregate.
package condition

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	port "github.com/Jersyfi/hubtask/core/port/expression"
)

// The names an expression may use. Written out rather than derived, because they are a contract:
// automation.md §1.2 lists them, and an expression naming anything else fails when it is written.
const (
	VarEvent      = "event"
	VarItem       = "item"
	VarParent     = "parent"
	VarCollection = "collection"
	VarHub        = "hub"
	VarActor      = "actor"
	VarNow        = "now"
	VarPayload    = "payload"
	VarTenant     = "tenant"
)

// RuleEnvironment is what an automation rule's condition may name (automation.md §1.2).
//
// `tenant` is a document rather than a name of its own, because the document writes
// `tenant.settings` - a variable called `tenant.settings` is not a name CEL can declare, and the
// settings are one field of the workspace rather than the whole of what a rule may know about it.
func RuleEnvironment() port.Environment {
	return port.Environment{Variables: []port.Variable{
		{Name: VarEvent, Kind: port.KindMap, Description: "The event that started the run: its type, subject and causal chain."},
		{Name: VarItem, Kind: port.KindMap, Description: "The entry the event is about, when it is about one."},
		{Name: VarParent, Kind: port.KindMap, Description: "The entry above it, when it has one."},
		{Name: VarCollection, Kind: port.KindMap, Description: "The collection the entry sits in."},
		{Name: VarHub, Kind: port.KindMap, Description: "The hub above that collection."},
		{Name: VarActor, Kind: port.KindMap, Description: "Who caused the event: kind, id, and whom they acted on behalf of."},
		{Name: VarNow, Kind: port.KindTimestamp, Description: "The instant the run began. One instant per run, not per expression."},
		{Name: VarPayload, Kind: port.KindMap, Description: "The body an inbound webhook delivered, for a rule triggered by one."},
		{Name: VarTenant, Kind: port.KindMap, Description: "The workspace: `tenant.settings` is its configuration."},
	}}
}

// RetentionEnvironment is what a retention rule's condition may name (data-retention.md §2).
//
// A smaller set of the same names, and smaller on purpose rather than by omission. A retention pass
// is deciding about one entry against the clock; there is no event, no actor and no payload,
// because nobody caused it - the clock did. Declaring them anyway would let somebody write
// `actor.id == …` into a retention rule and get a condition that is never true.
func RetentionEnvironment() port.Environment {
	return port.Environment{Variables: []port.Variable{
		{Name: VarItem, Kind: port.KindMap, Description: "The entry the pass is judging."},
		{Name: VarNow, Kind: port.KindTimestamp, Description: "The instant the pass began."},
		{Name: VarTenant, Kind: port.KindMap, Description: "The workspace: `tenant.settings` is its configuration."},
	}}
}

// ItemDocument is one entry as an expression reads it.
//
// Snake case, because that is what the API answers and what somebody writing a condition has in
// front of them - `item.completed_at`, not `item.completedAt`. The names match the contract rather
// than the Go field, which is the whole reason this is a written projection.
//
// Absent optional values are absent keys rather than nulls, so that `has(item.due_at)` answers what
// it looks like it answers. A null would make `item.due_at == null` and `!has(item.due_at)` two
// different questions with one answer.
func ItemDocument(item work.WorkItem) map[string]any {
	document := map[string]any{
		"id":            item.ID.String(),
		"type":          string(item.Type),
		"title":         item.Title,
		"notes":         item.Notes,
		"collection_id": item.CollectionID.String(),
		"path":          item.Path,
		"depth":         item.Depth,
		"completed":     item.Completion.IsCompleted,
		"order_key":     item.OrderKey,
		"archived":      item.ArchivedAt != nil,
		"trashed":       item.DeletedAt != nil,
	}
	putID(document, "parent_id", item.ParentID)
	putID(document, "bucket_id", item.BucketID)
	putID(document, "assignee_id", item.AssigneeID)
	putTime(document, "completed_at", item.Completion.CompletedAt)
	putTime(document, "start_at", item.StartAt)
	putTime(document, "archived_at", item.ArchivedAt)
	putTime(document, "trashed_at", item.DeletedAt)
	if item.Due != nil {
		putTime(document, "due_at", &item.Due.At)
	}
	if item.CustomFields != nil {
		document["custom_fields"] = item.CustomFields
	}
	if item.ContentLanguage != "" {
		document["content_language"] = item.ContentLanguage
	}
	return document
}

// LabelledItemDocument is ItemDocument with the sets an expression is most likely to ask about.
//
// Separate, because the labels and members of an entry are not on the aggregate - they are read
// beside it - and a projection that pretended otherwise would answer an empty list for an entry
// whose labels nobody had loaded. An empty list and "not loaded" are the same value and different
// facts, and a condition on the wrong one of them is silently false.
func LabelledItemDocument(item work.WorkItem, labels []string, members []string) map[string]any {
	document := ItemDocument(item)
	document["labels"] = texts(labels)
	document["members"] = texts(members)
	return document
}

// putID writes an identifier only when there is one. An absent reference is an absent key rather
// than an empty string, so that `has(item.parent_id)` answers what it looks like it answers.
func putID(document map[string]any, key string, id shared.ID) {
	if !id.IsZero() {
		document[key] = id.String()
	}
}

func putTime(document map[string]any, key string, value *time.Time) {
	if value != nil {
		document[key] = value.UTC()
	}
}

func texts(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
