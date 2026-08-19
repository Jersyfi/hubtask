// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var completionProfile = work.CapabilityProfile{
	Type:         work.ItemTask,
	Capabilities: []work.Capability{work.CapabilityCompletion, work.CapabilityNotes},
	MaxDepth:     3,
}

func openTask() work.WorkItem {
	return work.WorkItem{
		ID: newID, TenantID: tenant, CollectionID: hubID, Type: work.ItemTask,
		Path: work.RootPath(newID), Depth: 1, Title: "Buy milk", OrderKey: "a0",
		CreatedBy: actorID, CreatedAt: created, UpdatedAt: created, Version: 1,
	}
}

func TestCompletingAnItemRecordsWhoAndWhen(t *testing.T) {
	at := created.Add(time.Hour)

	done := openTask().Completed(actorID, at)
	if !done.Completion.IsCompleted {
		t.Fatal("the item is not completed")
	}
	if done.Completion.CompletedAt == nil || !done.Completion.CompletedAt.Equal(at) {
		t.Errorf("completed_at is %v, want %v", done.Completion.CompletedAt, at)
	}
	if done.Completion.CompletedBy != actorID {
		t.Errorf("completed_by is %s", done.Completion.CompletedBy)
	}
	if !done.UpdatedAt.Equal(at) {
		t.Errorf("updated_at is %v, want %v", done.UpdatedAt, at)
	}
	// Stored in UTC, so that two items completed in two time zones compare as the instants they are.
	if done.Completion.CompletedAt.Location() != time.UTC {
		t.Errorf("completed_at is in %v", done.Completion.CompletedAt.Location())
	}
}

// Idempotence is not politeness towards a client retrying: the roll-up may reach the same parent twice
// from two children, and `completed_by` has to stay the truth about who finished the work.
func TestCompletingATwiceKeepsTheFirstAnswer(t *testing.T) {
	first := created.Add(time.Hour)
	later := created.Add(2 * time.Hour)
	somebodyElse := shared.MustParseID("0192f000-0000-7000-8000-0000000000ee")

	done := openTask().Completed(actorID, first)
	again := done.Completed(somebodyElse, later)

	if again.Completion.CompletedBy != actorID {
		t.Errorf("completed_by moved to %s", again.Completion.CompletedBy)
	}
	if !again.Completion.CompletedAt.Equal(first) {
		t.Errorf("completed_at moved to %v", again.Completion.CompletedAt)
	}
	if !again.UpdatedAt.Equal(done.UpdatedAt) {
		t.Errorf("updated_at moved to %v, so a repeat would spend a version", again.UpdatedAt)
	}
}

// Cleared rather than kept: the two fields answer "when was this finished, and by whom", and an open item
// has no answer. Keeping them would make completed_at a record of the last time it happened to be
// closed, which is what the activity history is for (B-11).
func TestReopeningClearsWhoAndWhen(t *testing.T) {
	at := created.Add(2 * time.Hour)

	open := openTask().Completed(actorID, created.Add(time.Hour)).Reopened(at)
	if open.Completion.IsCompleted {
		t.Fatal("the item is still completed")
	}
	if open.Completion.CompletedAt != nil {
		t.Errorf("completed_at survived as %v", open.Completion.CompletedAt)
	}
	if !open.Completion.CompletedBy.IsZero() {
		t.Errorf("completed_by survived as %s", open.Completion.CompletedBy)
	}
	if !open.UpdatedAt.Equal(at) {
		t.Errorf("updated_at is %v, want %v", open.UpdatedAt, at)
	}
}

func TestReopeningAnOpenItemChangesNothing(t *testing.T) {
	item := openTask()

	again := item.Reopened(created.Add(time.Hour))
	if !again.UpdatedAt.Equal(item.UpdatedAt) {
		t.Errorf("updated_at moved to %v, so a repeat would spend a version", again.UpdatedAt)
	}
}

// The capability is the type's, the lifecycle is this item's, and the two are different kinds of answer:
// one says nobody can ever complete this kind of thing, the other says not while it is in the trash.
func TestWhatCannotHaveItsCompletionChanged(t *testing.T) {
	trashedAt := created.Add(time.Hour)

	noCompletion := work.CapabilityProfile{
		Type: work.ItemActivity, Capabilities: []work.Capability{work.CapabilityNotes}, MaxDepth: 1,
	}

	trashed := openTask()
	trashed.DeletedAt = &trashedAt
	archived := openTask()
	archived.ArchivedAt = &trashedAt

	cases := map[string]struct {
		item       work.WorkItem
		profile    work.CapabilityProfile
		sentinel   error
		detailCode string
	}{
		"a type whose profile has no completion": {
			openTask(), noCompletion,
			shared.ErrCapabilityNotSupported, "items.capability_not_supported",
		},
		"an item in the trash": {trashed, completionProfile, shared.ErrConflict, "items.trashed"},
		"an archived item":     {archived, completionProfile, shared.ErrConflict, "items.archived"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.item.EnsureCompletable(c.profile)
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("answered %v, want %v", err, c.sentinel)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("the detail code is %q, want %q", got, c.detailCode)
			}
		})
	}
}

// The capability is checked before the lifecycle: "an activity has no completion" is true of the type
// whatever state one activity is in, and reporting the state first would send a client off to restore an
// item that still could not be completed afterwards.
func TestTheCapabilityIsReportedBeforeTheLifecycle(t *testing.T) {
	trashedAt := created.Add(time.Hour)

	item := openTask()
	item.DeletedAt = &trashedAt
	noCompletion := work.CapabilityProfile{Type: work.ItemActivity, MaxDepth: 1}

	err := item.EnsureCompletable(noCompletion)
	if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
		t.Errorf("a trashed item of a type without completion answered %q", got)
	}
}

func TestAnOrdinaryItemIsCompletable(t *testing.T) {
	if err := openTask().EnsureCompletable(completionProfile); err != nil {
		t.Errorf("an open task is not completable: %v", err)
	}
}
