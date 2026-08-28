// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package condition_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/condition"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The list automation.md §1.2 writes, checked against the list the code declares. Two places state
// it and this is what stops them drifting.
func TestTheRuleEnvironmentIsTheDocumentedList(t *testing.T) {
	want := []string{
		"event", "item", "parent", "collection", "hub", "actor", "now", "payload", "tenant",
	}

	var got []string
	for _, variable := range condition.RuleEnvironment().Variables {
		got = append(got, variable.Name)
		if variable.Description == "" {
			t.Errorf("%s has no description - a rule editor shows one beside the name", variable.Name)
		}
	}
	slices.Sort(got)
	slices.Sort(want)
	if len(got) != len(want) {
		t.Fatalf("the environment declares %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("declared %q, want %q", got[i], want[i])
		}
	}
}

// Smaller on purpose rather than by omission: a retention pass is deciding about one entry against
// the clock, and nobody caused it.
func TestTheRetentionEnvironmentHasNoEventNoActorAndNoPayload(t *testing.T) {
	for _, variable := range condition.RetentionEnvironment().Variables {
		switch variable.Name {
		case "event", "actor", "payload", "parent", "collection", "hub":
			t.Errorf("a retention condition may name %q, which nothing can ever fill", variable.Name)
		}
	}
	if len(condition.RetentionEnvironment().Variables) == 0 {
		t.Fatal("a retention condition may name nothing at all")
	}
}

func fixture() work.WorkItem {
	completed := time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	return work.WorkItem{
		ID:           shared.ID("01936f2a-7c1e-7000-8000-0000000000c1"),
		CollectionID: shared.ID("01936f2a-7c1e-7000-8000-0000000000c2"),
		Type:         work.ItemTask,
		Title:        "Pay the invoice",
		Path:         "/01936f2a-7c1e-7000-8000-0000000000c1/",
		Depth:        1,
		Completion:   work.Completion{IsCompleted: true, CompletedAt: &completed},
	}
}

// Snake case, because that is what somebody writing a condition has in front of them.
func TestTheItemDocumentUsesTheContractsNames(t *testing.T) {
	document := condition.ItemDocument(fixture())

	for _, name := range []string{"id", "type", "title", "collection_id", "completed", "completed_at"} {
		if _, present := document[name]; !present {
			t.Errorf("the document has no %q", name)
		}
	}
	for _, camel := range []string{"collectionId", "completedAt", "orderKey"} {
		if _, present := document[camel]; present {
			t.Errorf("the document answers %q, which is the Go field rather than the contract", camel)
		}
	}
	if document["completed"] != true {
		t.Errorf("completed is %v", document["completed"])
	}
}

// A null would make `item.due_at == null` and `!has(item.due_at)` two different questions with one
// answer.
func TestAnAbsentValueIsAnAbsentKeyRatherThanANull(t *testing.T) {
	document := condition.ItemDocument(work.WorkItem{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000c1"), Type: work.ItemTask,
	})

	for _, optional := range []string{
		"due_at", "start_at", "completed_at", "archived_at", "trashed_at",
		"parent_id", "bucket_id", "assignee_id", "custom_fields", "content_language",
	} {
		if value, present := document[optional]; present {
			t.Errorf("%q is present as %v on an entry that has none", optional, value)
		}
	}

	// The flags are always there, because "not archived" is a fact rather than an absence.
	for _, always := range []string{"archived", "trashed", "completed"} {
		if _, present := document[always]; !present {
			t.Errorf("%q is absent, and it is a fact rather than an absence", always)
		}
	}
}

// An empty list and "not loaded" are the same value and different facts, which is why the sets are
// a separate projection rather than a default on the first one.
func TestTheSetsTravelOnlyWhenTheyWereRead(t *testing.T) {
	plain := condition.ItemDocument(fixture())
	if _, present := plain["labels"]; present {
		t.Error("the plain projection answers labels it never read")
	}

	withSets := condition.LabelledItemDocument(fixture(), []string{"approval"}, nil)
	labels, ok := withSets["labels"].([]any)
	if !ok || len(labels) != 1 || labels[0] != "approval" {
		t.Errorf("labels came back %v", withSets["labels"])
	}
	// Read and empty, which is a fact the expression may act on.
	if members, ok := withSets["members"].([]any); !ok || len(members) != 0 {
		t.Errorf("members came back %v, want an empty list", withSets["members"])
	}
}
