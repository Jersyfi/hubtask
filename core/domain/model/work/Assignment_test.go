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

var assignmentProfile = work.CapabilityProfile{
	Type:         work.ItemTask,
	Capabilities: []work.Capability{work.CapabilityAssignment, work.CapabilityMembers},
	MaxDepth:     3,
}

// assignee is somebody other than the actor: an assignment is a thing done to a person, and a
// fixture that used the actor's own identifier would pass a method that ignored its argument.
var assignee = shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")

func TestAssigningPutsTheAccountOnTheEntry(t *testing.T) {
	at := created.Add(time.Hour)

	assigned := openTask().Assigned(assignee, at)
	if assigned.AssigneeID != assignee {
		t.Errorf("the assignee is %q, want %q", assigned.AssigneeID, assignee)
	}
	if !assigned.UpdatedAt.Equal(at) {
		t.Errorf("updated_at is %v, want %v", assigned.UpdatedAt, at)
	}
}

// Idempotence is what makes assigning the same person twice from two devices converge: the second
// call writes nothing, spends no version and announces nothing.
func TestAssigningTheSamePersonAgainChangesNothing(t *testing.T) {
	first := created.Add(time.Hour)
	later := created.Add(2 * time.Hour)

	assigned := openTask().Assigned(assignee, first)
	again := assigned.Assigned(assignee, later)

	if !again.UpdatedAt.Equal(first) {
		t.Errorf("updated_at moved to %v although nothing changed", again.UpdatedAt)
	}
}

// Handing the entry on is one call rather than an unassign followed by an assign: the field is a
// scalar, and the person who had it is in the history rather than in the row.
func TestAssigningSomebodyElseReplacesTheAssignee(t *testing.T) {
	somebodyElse := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	at := created.Add(time.Hour)

	handed := openTask().Assigned(assignee, created).Assigned(somebodyElse, at)
	if handed.AssigneeID != somebodyElse {
		t.Errorf("the assignee is %q, want %q", handed.AssigneeID, somebodyElse)
	}
	if !handed.UpdatedAt.Equal(at) {
		t.Errorf("updated_at is %v, want %v", handed.UpdatedAt, at)
	}
}

func TestUnassigningClearsTheAssignee(t *testing.T) {
	at := created.Add(time.Hour)

	cleared := openTask().Assigned(assignee, created).Unassigned(at)
	if !cleared.AssigneeID.IsZero() {
		t.Errorf("the assignee is %q, want nobody", cleared.AssigneeID)
	}
	if !cleared.UpdatedAt.Equal(at) {
		t.Errorf("updated_at is %v, want %v", cleared.UpdatedAt, at)
	}
}

func TestUnassigningAnEntryNobodyIsOnChangesNothing(t *testing.T) {
	unchanged := openTask().Unassigned(created.Add(time.Hour))
	if !unchanged.UpdatedAt.Equal(created) {
		t.Errorf("updated_at moved to %v although nobody was assigned", unchanged.UpdatedAt)
	}
}

// The capability is the type's and the lifecycle is this entry's, which are two different answers:
// one says this kind of thing has no assignee at all, the other says not while it is in the trash.
func TestWhatCannotHaveItsAssigneeChanged(t *testing.T) {
	stampedAt := created.Add(time.Hour)

	noAssignment := work.CapabilityProfile{
		Type: work.ItemActivity, Capabilities: []work.Capability{work.CapabilityCompletion},
		MaxDepth: 1,
	}

	trashed := openTask()
	trashed.DeletedAt = &stampedAt
	archived := openTask()
	archived.ArchivedAt = &stampedAt

	cases := map[string]struct {
		item       work.WorkItem
		profile    work.CapabilityProfile
		sentinel   error
		detailCode string
	}{
		"a type whose profile has no assignment": {
			openTask(), noAssignment,
			shared.ErrCapabilityNotSupported, "items.capability_not_supported",
		},
		"an entry in the trash": {trashed, assignmentProfile, shared.ErrConflict, "items.trashed"},
		"an archived entry":     {archived, assignmentProfile, shared.ErrConflict, "items.archived"},
	}

	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			err := c.item.EnsureAssignable(c.profile)
			if !errors.Is(err, c.sentinel) {
				t.Fatalf("answered %v, want %v", err, c.sentinel)
			}
			if got := shared.AsError(err).DetailCode; got != c.detailCode {
				t.Errorf("the detail code is %q, want %q", got, c.detailCode)
			}
		})
	}
}

// The capability before the lifecycle, for the reason EnsureCompletable checks it in that order: an
// activity of a type without assignment stays unassignable however many times it is restored.
func TestTheAssignmentCapabilityIsReportedBeforeTheLifecycle(t *testing.T) {
	stampedAt := created.Add(time.Hour)

	item := openTask()
	item.DeletedAt = &stampedAt

	err := item.EnsureAssignable(work.CapabilityProfile{Type: work.ItemActivity, MaxDepth: 1})
	if got := shared.AsError(err).DetailCode; got != "items.capability_not_supported" {
		t.Errorf("a trashed entry of a type without assignment answered %q", got)
	}
}

// The refusal points at the field the caller filled in, so that a form can mark the input rather
// than the request.
func TestTheAssignmentRefusalNamesTheAccountField(t *testing.T) {
	err := openTask().EnsureAssignable(work.CapabilityProfile{Type: work.ItemActivity, MaxDepth: 1})

	fields := shared.AsError(err).Fields
	if len(fields) != 1 || fields[0].Path != "/account_id" {
		t.Errorf("the refusal points at %+v, want /account_id", fields)
	}
}

func TestAnOrdinaryEntryIsAssignable(t *testing.T) {
	if err := openTask().EnsureAssignable(assignmentProfile); err != nil {
		t.Errorf("an open task is not assignable: %v", err)
	}
}
