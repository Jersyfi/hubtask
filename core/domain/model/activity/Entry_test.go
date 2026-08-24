// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package activity_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	entryID      = shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	tenantID     = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	itemID       = shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")
	collectionID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	accountID    = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	occurredAt   = time.Date(2026, 8, 20, 9, 12, 0, 0, time.UTC)
)

func entry() activity.Entry {
	return activity.Entry{
		ID: entryID, TenantID: tenantID, ItemID: itemID, CollectionID: collectionID,
		Actor:      activity.Actor{Kind: shared.ActorUser, ID: accountID},
		Verb:       activity.ItemCompleted,
		ChangeSet:  map[string]any{},
		OccurredAt: occurredAt,
	}
}

func TestACompleteEntryIsAccepted(t *testing.T) {
	if err := entry().Validate(); err != nil {
		t.Fatalf("a complete entry was refused: %v", err)
	}
}

// Every one of these aborts the change the entry belongs to. That is the right way round: an item
// whose history silently missed a step is one nobody can trust afterwards.
func TestAnIncompleteEntryIsRefused(t *testing.T) {
	cases := map[string]func(*activity.Entry){
		"no identifier": func(e *activity.Entry) { e.ID = shared.ID("") },
		"no tenant":     func(e *activity.Entry) { e.TenantID = shared.ID("") },
		"no item":       func(e *activity.Entry) { e.ItemID = shared.ID("") },
		"no time":       func(e *activity.Entry) { e.OccurredAt = time.Time{} },
	}

	for name, damage := range cases {
		t.Run(name, func(t *testing.T) {
			e := entry()
			damage(&e)

			err := e.Validate()
			if !errors.Is(err, shared.ErrInternal) {
				t.Fatalf("error %v, want an internal one", err)
			}
			if detail := shared.AsError(err).DetailCode; detail != "activity.entry_incomplete" {
				t.Errorf("detail %q, want activity.entry_incomplete", detail)
			}
		})
	}
}

// A verb the catalogue does not know would reach a client as a key nothing renders, and the
// fallback is the key itself (i18n-l10n.md §3).
func TestAnUnknownVerbIsRefused(t *testing.T) {
	e := entry()
	e.Verb = "item.folded"

	err := e.Validate()
	if detail := shared.AsError(err).DetailCode; detail != "activity.verb_unknown" {
		t.Fatalf("detail %q, want activity.verb_unknown", detail)
	}
}

// The column's CHECK constraint allows the five auditable kinds. An entry naming the anonymous
// actor would be refused at commit time, which is the worst moment to find out.
func TestAnAnonymousActorIsRefused(t *testing.T) {
	e := entry()
	e.Actor.Kind = shared.ActorAnonymous

	err := e.Validate()
	if detail := shared.AsError(err).DetailCode; detail != "activity.actor_kind_invalid" {
		t.Fatalf("detail %q, want activity.actor_kind_invalid", detail)
	}
}

func TestEveryVerbIsValidAndNothingElseIs(t *testing.T) {
	if len(activity.Verbs()) != 22 {
		t.Fatalf("%d verbs, want the twenty-two the history knows", len(activity.Verbs()))
	}
	for _, verb := range activity.Verbs() {
		if !verb.Valid() {
			t.Errorf("%s is in the list and reports itself invalid", verb)
		}
	}
	if activity.Verb("").Valid() {
		t.Error("the empty verb reports itself valid")
	}
}

// The catalogue key is the verb in the `activity` namespace, and the key is what a client renders
// (i18n-l10n.md §1). That every key is in locales/en.json is checked by the architecture gate,
// which is where the catalogue is read.
func TestTheMessageCodeIsTheVerbInTheActivityNamespace(t *testing.T) {
	if code := activity.ItemCompleted.MessageCode(); code != "activity.item_completed" {
		t.Errorf("message code %q, want activity.item_completed", code)
	}
	if code := activity.ItemLabelAdded.MessageCode(); code != "activity.item_label_added" {
		t.Errorf("message code %q, want activity.item_label_added", code)
	}
}

// Names always, values where the product needs them. A note is a page of text and its history is
// that somebody edited it; a rename means nothing without both titles.
func TestTheChangeSetKeepsNamesAlwaysAndValuesWhereItIsTold(t *testing.T) {
	set := activity.ChangeSet(activity.Full,
		activity.Field{Name: "title", Detail: activity.WithValues, From: "Milk", To: "Oat milk"},
		activity.Field{Name: "notes", Detail: activity.NameOnly, From: "a page", To: "another page"},
		activity.Field{Name: "bucket_id", Detail: activity.WithValues, To: "b1"},
		activity.Field{Name: "", Detail: activity.WithValues, To: "dropped"},
	)

	if len(set) != 3 {
		t.Fatalf("the change set holds %d fields, want 3: %v", len(set), set)
	}

	title, _ := set["title"].(map[string]any)
	if title["from"] != "Milk" || title["to"] != "Oat milk" {
		t.Errorf("the rename reads %v, want both titles", title)
	}

	notes, _ := set["notes"].(map[string]any)
	if notes["changed"] != true || len(notes) != 1 {
		t.Errorf("the note reads %v, want that it changed and nothing else", notes)
	}
	if notes["from"] != nil || notes["to"] != nil {
		t.Errorf("the note's text reached the history: %v", notes)
	}

	// A field that had no value on one side is recorded as having none, so that a client can tell
	// "it was empty" from "it was set".
	bucket, _ := set["bucket_id"].(map[string]any)
	if _, present := bucket["from"]; present {
		t.Errorf("the empty previous value was recorded: %v", bucket)
	}
	if bucket["to"] != "b1" {
		t.Errorf("the new value reads %v, want b1", bucket["to"])
	}

	cleared := activity.ChangeSet(activity.Full,
		activity.Field{Name: "bucket_id", Detail: activity.WithValues, From: "b1"})
	if entry, _ := cleared["bucket_id"].(map[string]any); entry["from"] != "b1" || len(entry) != 1 {
		t.Errorf("clearing a field reads %v, want the value it lost and nothing else", entry)
	}
}

// The compact form of the capability matrix: the verb, the actor and the time, and nothing else.
func TestACompactChangeSetIsEmpty(t *testing.T) {
	set := activity.ChangeSet(activity.Compact,
		activity.Field{Name: "title", Detail: activity.WithValues, From: "Milk", To: "Oat milk"})

	if len(set) != 0 {
		t.Errorf("the compact change set holds %v, want nothing", set)
	}
}
