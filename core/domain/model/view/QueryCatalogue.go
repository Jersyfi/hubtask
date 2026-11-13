// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import "strings"

// QueryKey is one key of the Hubtask query as a client offers it (Q-1, Q-3): what is typed, what
// kind of value it takes, which objects it asks about, and examples. Names are the client's to
// localise (rule 8); the key and the examples are the language's and stay English.
type QueryKey struct {
	Key string
	// Field is the filter tree's field the key compares; empty for is, type and sort, which
	// choose several fields or none.
	Field string
	// Value is the kind of value: state, kind, person, place, label, time, number, text, words,
	// choice, flag, relation, sort, custom.
	Value string
	// Product is the prefix's product (desk, cf), empty for the fields of every kind.
	Product string
	// Objects are the kinds the key asks about; a key of entries alone keeps a search to entries.
	Objects  []string
	Examples []string
	// Values are the words a key with a closed list takes.
	Values []string
}

// entryObjects are the kinds a key of entries asks about.
var entryObjects = []string{"task", "workpackage", "activity", "ticket"}

// QueryKeys is the catalogue the doors serve (GET /query/keys): only keys the server answers, so
// a client never offers what the server refuses (api-guidelines.md §3.1). A product prefix with no
// key here - asset., page., post., goal., time. - has none the server answers yet.
func QueryKeys() []QueryKey {
	isValues := []string{"open", "done", "blocked", "blocking", "archived", "trashed"}
	times := []string{"due: <today", "updated: -7d", "due: this-week", "created: 2026-10-01..2026-10-31"}
	keys := []QueryKey{
		{Key: "is", Value: "state", Objects: entryObjects, Values: isValues, Examples: []string{"is: open", "-is: done"}},
		{Key: "type", Value: "kind", Objects: ObjectKinds, Values: ObjectKinds, Examples: []string{"type: task, workpackage", "type: page"}},
		{Key: "status", Field: FieldStatus, Value: "choice", Objects: entryObjects, Examples: []string{"status: in_progress, review"}},
		{Key: "who", Field: FieldAssigneeID, Value: "person", Objects: entryObjects, Values: []string{"me", "none"}, Examples: []string{"who: me, none"}},
		{Key: "with", Field: FieldMembers, Value: "person", Objects: entryObjects, Values: []string{"me"}, Examples: []string{"with: me"}},
		{Key: "by", Field: FieldCreatedBy, Value: "person", Objects: entryObjects, Values: []string{"me"}, Examples: []string{"by: me"}},
		{Key: "in", Field: FieldCollection, Value: "place", Objects: entryObjects, Examples: []string{"in: here"}},
		{Key: "parent", Field: FieldParentID, Value: "relation", Objects: entryObjects, Values: []string{"none"}, Examples: []string{"parent: none"}},
		{Key: "linked", Field: FieldLinkedTo, Value: "relation", Objects: entryObjects},
		{Key: "label", Field: FieldLabels, Value: "label", Objects: entryObjects, Examples: []string{"-label: intern"}},
		{Key: "title", Field: FieldTitle, Value: "text", Objects: entryObjects, Examples: []string{"title: angebot", "title: =Angebot", "title: Ange*"}},
		{Key: "text", Field: FieldText, Value: "words", Objects: entryObjects, Examples: []string{"text: passkey"}},
		{Key: "note", Field: FieldNotes, Value: "text", Objects: entryObjects, Examples: []string{"note: none"}},
		{Key: "created", Field: FieldCreatedAt, Value: "time", Objects: entryObjects, Examples: times[3:]},
		{Key: "updated", Field: FieldUpdatedAt, Value: "time", Objects: entryObjects, Examples: times[1:2]},
		{Key: "due", Field: FieldDueAt, Value: "time", Objects: entryObjects, Values: []string{"overdue", "today", "this-week", "none"}, Examples: []string{times[0], times[2], "due: +2w"}},
		{Key: "start", Field: FieldStartAt, Value: "time", Objects: entryObjects, Examples: []string{"start: >=today"}},
		{Key: "done", Field: FieldCompletedAt, Value: "time", Objects: entryObjects, Examples: []string{"done: last-week"}},
		{Key: "desk.sla", Field: FieldSLADue, Value: "time", Product: "desk", Objects: entryObjects, Examples: []string{"desk.sla: <+1d"}},
		{Key: "desk.standing", Field: FieldSLAStanding, Value: "choice", Product: "desk", Objects: entryObjects, Values: lower(SLAStandings()), Examples: []string{"desk.standing: at_risk, breached"}},
		{Key: "desk.satisfaction", Field: FieldSatisfaction, Value: "number", Product: "desk", Objects: entryObjects, Examples: []string{"desk.satisfaction: <=2"}},
		{Key: "hub", Field: FieldHub, Value: "place", Objects: entryObjects, Examples: []string{"hub: here"}},
		{Key: "under", Field: FieldUnder, Value: "relation", Objects: entryObjects, Examples: []string{"under: TAMA-1"}},
		{Key: "blocks", Field: FieldBlocks, Value: "relation", Objects: entryObjects, Examples: []string{"blocks: TAMA-1"}},
		{Key: "affects", Field: FieldAffects, Value: "relation", Objects: entryObjects},
		{Key: "plan.sprint", Field: FieldSprint, Value: "relation", Product: "plan", Objects: entryObjects, Values: []string{"none"}, Examples: []string{"plan.sprint: none"}},
		{Key: "plan.release", Field: FieldRelease, Value: "relation", Product: "plan", Objects: entryObjects, Values: []string{"none"}, Examples: []string{"plan.release: none"}},
		{Key: "plan.slip", Field: FieldSlip, Value: "number", Product: "plan", Objects: entryObjects, Examples: []string{"plan.slip: >0", "plan.slip: >=5"}},
		{Key: CustomKeyPrefix, Field: CustomFieldPrefix, Value: "custom", Product: "cf", Objects: entryObjects, Examples: []string{"cf.priority: >=3"}},
		{Key: "sort", Value: "sort", Objects: entryObjects, Examples: []string{"sort: -updated", "sort: due"}},
	}
	return keys
}

func lower(values []string) []string {
	out := make([]string, len(values))
	for i, value := range values {
		out[i] = strings.ToLower(value)
	}
	return out
}

// EntryFilter is the query as a filter of entries: the free words become a MATCHES leaf beside
// the tree. A kind other than an entry level refuses, because a query of entries cannot answer it.
func (q Query) EntryFilter(path string) (any, error) {
	for _, kind := range q.Kinds {
		if kind != "ticket" {
			return nil, fieldError(path, "query.kind_not_here", map[string]string{"kind": kind})
		}
	}
	var nodes []any
	if q.Filter != nil {
		nodes = append(nodes, q.Filter)
	}
	if q.Words != "" {
		nodes = append(nodes, matches(q.Words))
	}
	return joinAnd(nodes), nil
}
