// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	ada  = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	bert = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
)

func parsed(t *testing.T, raw any) Node {
	t.Helper()
	node, err := ParseFilter(raw, "/filter")
	if err != nil {
		t.Fatal(err)
	}
	return *node
}

func ids(values []Value) []shared.ID {
	out := make([]shared.ID, 0, len(values))
	for _, v := range values {
		out = append(out, v.ID)
	}
	return out
}

// A group placeholder becomes its people, read by the use case: EQ asks IN, CONTAINS asks
// CONTAINS_ANY, each person once, an empty group matches nobody.
func TestAGroupBecomesItsPeople(t *testing.T) {
	groups := map[string][]shared.ID{"@my_groups": {ada, bert, ada}, "@group:Support": {}}
	cases := []struct {
		name   string
		raw    map[string]any
		op     Operator
		people []shared.ID
	}{
		{"my groups", map[string]any{"field": "assignee_id", "op": "EQ", "value": "@my_groups"}, OpIn, []shared.ID{ada, bert}},
		{"not my groups", map[string]any{"field": "assignee_id", "op": "NEQ", "value": "@my_groups"}, OpNotIn, []shared.ID{ada, bert}},
		{"with my groups", map[string]any{"field": "members", "op": "CONTAINS", "value": "@my_groups"}, OpContainsAny, []shared.ID{ada, bert}},
		{"an empty group", map[string]any{"field": "created_by", "op": "EQ", "value": "@group:Support"}, OpIn, []shared.ID{noAccount}},
		{"me and a group", map[string]any{"field": "assignee_id", "op": "IN", "value": []any{"@me", "@group:Support"}}, OpIn, []shared.ID{actor}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			resolved, err := parsed(t, c.raw).Resolve(Resolution{ActorID: actor, Groups: groups}, "/filter")
			if err != nil {
				t.Fatal(err)
			}
			if resolved.Op != c.op || len(resolved.Values) != len(c.people) {
				t.Fatalf("got %s %v", resolved.Op, ids(resolved.Values))
			}
			for i, id := range c.people {
				if resolved.Values[i].ID != id {
					t.Fatalf("got %v, want %v", ids(resolved.Values), c.people)
				}
			}
		})
	}
}

func TestAGroupNeedsAnAskerAndTheUseCasesRead(t *testing.T) {
	node := parsed(t, map[string]any{"field": "assignee_id", "op": "EQ", "value": "@my_groups"})
	for _, c := range []struct {
		at   Resolution
		code string
	}{
		{Resolution{}, "query.placeholder_needs_actor"},
		{Resolution{ActorID: actor}, "query.groups_not_here"},
	} {
		_, err := node.Resolve(c.at, "/filter")
		var fault *shared.Error
		if !errors.As(err, &fault) || fault.DetailCode != c.code {
			t.Fatalf("got %v, want %s", err, c.code)
		}
	}
}

func TestAGroupPlaceholderIsRefusedWhereNoPersonIs(t *testing.T) {
	for _, raw := range []map[string]any{
		{"field": "due_at", "op": "EQ", "value": "@my_groups"},
		{"field": "due_at", "op": "EQ", "value": "@group:Support"},
		{"field": "assignee_id", "op": "EQ", "value": "@group:"},
		{"field": "assignee_id", "op": "EQ", "value": "@my_groups+P1D"},
	} {
		if _, err := ParseFilter(raw, "/filter"); err == nil {
			t.Fatalf("%v is read", raw)
		}
	}
}

// time.logged and plan.critical are the asker's questions: the system asks as nobody, and the use
// case's answer is carried on the leaf.
func TestTheAskersKeysAreAnsweredByTheUseCase(t *testing.T) {
	tree := parsed(t, map[string]any{"op": "AND", "nodes": []any{
		map[string]any{"field": "time_logged", "op": "GT", "value": 0},
		map[string]any{"field": "critical", "op": "EQ", "value": true},
		map[string]any{"field": "assignee_id", "op": "EQ", "value": "@group:Support"},
	}})
	needs := NeedsOf(&tree)
	if !needs.Time || !needs.Critical || len(needs.Groups) != 1 || needs.Groups[0].Group != "Support" {
		t.Fatalf("needs %+v", needs)
	}
	if _, err := tree.Nodes[0].Resolve(Resolution{}, "/filter"); err == nil {
		t.Fatal("time.logged resolved without an asker")
	}
	answered := tree.Answer(Asked{Sight: TimeSight{Reader: actor}, Critical: []shared.ID{ada}})
	if answered.Nodes[0].Asked == nil || answered.Nodes[0].Asked.Sight.Reader != actor ||
		answered.Nodes[1].Asked == nil || answered.Nodes[2].Asked != nil {
		t.Fatalf("answered %+v", answered)
	}
	if !answered.Nodes[1].CriticalWanted() {
		t.Fatal("critical: true keeps the path")
	}
	off := parsed(t, map[string]any{"field": "critical", "op": "NEQ", "value": true})
	if off.CriticalWanted() {
		t.Fatal("critical NEQ true keeps the path")
	}
}
