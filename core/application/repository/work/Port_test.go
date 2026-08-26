// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Find(context.Context, shared.ID) (work.Container, error) {
	return work.Container{}, shared.ErrNotFound
}
func (double) List(context.Context, ContainerQuery) (ContainerPage, error) {
	return ContainerPage{}, nil
}
func (double) LastOrderKey(context.Context, shared.ID) (string, error)  { return "", nil }
func (double) Insert(context.Context, work.Container) error             { return nil }
func (double) SetAttributes(context.Context, work.Container, int) error { return nil }
func (double) SetPolicies(context.Context, work.Container, int) error   { return nil }
func (double) SetArchived(context.Context, work.Container, int) error   { return nil }
func (double) SetPlacement(context.Context, work.Container, int) error  { return nil }
func (double) TrashSubtree(context.Context, ContainerTrash) (Cascade, error) {
	return Cascade{}, nil
}
func (double) RestoreBatch(context.Context, ContainerTrash) (Cascade, error) {
	return Cascade{}, nil
}
func (double) Neighbours(context.Context, shared.ID, shared.ID, shared.ID) (string, string, error) {
	return "", "", nil
}

var _ Containers = double{}

// Two answers the use case tells apart, so both have to be expressible: a container that is not
// there, and a level that has no containers yet.
func TestTheTwoEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing container is reported as %v", err)
	}

	key, err := (double{}).LastOrderKey(t.Context(), "")
	if err != nil {
		t.Fatalf("an empty level is reported as an error: %v", err)
	}
	if key != "" {
		t.Errorf("an empty level answered %q rather than nothing", key)
	}
}

// The same for items. A separate double rather than one type implementing both ports: two
// repositories that happened to share a fake would let a use case be wired to the wrong one and
// still compile.
type itemDouble struct{}

func (itemDouble) Find(context.Context, shared.ID) (work.WorkItem, error) {
	return work.WorkItem{}, shared.ErrNotFound
}
func (itemDouble) List(context.Context, ItemQuery) (ItemPage, error) { return ItemPage{}, nil }
func (itemDouble) ChildCompletion(context.Context, shared.ID) (work.ChildCompletion, error) {
	return work.ChildCompletion{}, nil
}
func (itemDouble) SetCompletion(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) SetAttributes(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) Neighbours(context.Context, Level, shared.ID, shared.ID) (string, string, error) {
	return "", "", nil
}
func (itemDouble) SetOrderKey(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) SetAssignee(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) CountOpenByAssignee(context.Context, []shared.ID) (map[shared.ID]int, error) {
	return nil, nil
}
func (itemDouble) SetCustomField(
	context.Context, work.WorkItem, string, shared.ID, int,
) error {
	return nil
}
func (itemDouble) SetCover(context.Context, work.WorkItem, int) error { return nil }

func (itemDouble) SetDueDate(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) MoveSubtree(
	context.Context, Move,
) (int, []work.DroppedReference, error) {
	return 0, nil, nil
}
func (itemDouble) LastOrderKey(context.Context, shared.ID, shared.ID) (string, error) {
	return "", nil
}
func (itemDouble) Insert(context.Context, work.WorkItem) error { return nil }
func (itemDouble) Subtree(context.Context, work.WorkItem, int) ([]work.WorkItem, error) {
	return nil, nil
}
func (itemDouble) InsertCopy(context.Context, Copy) error                { return nil }
func (itemDouble) SetArchived(context.Context, work.WorkItem, int) error { return nil }
func (itemDouble) TrashSubtree(context.Context, ItemTrash) (int, error)  { return 0, nil }
func (itemDouble) RestoreBatch(context.Context, ItemTrash) (int, error)  { return 0, nil }
func (itemDouble) Query(context.Context, ItemSearch) (ItemQueryResult, error) {
	return ItemQueryResult{}, nil
}
func (itemDouble) Search(context.Context, TextSearch) (ItemHitPage, error) {
	return ItemHitPage{}, nil
}

var _ Items = itemDouble{}

// The policy store's double, for the same reason the others exist: the use case tests fake it,
// and this is where the interface is proven fakeable.
type policyDouble struct{}

func (policyDouble) FindForScope(context.Context, work.AutoAssignScope, shared.ID) (work.AutoAssignPolicy, error) {
	return work.AutoAssignPolicy{}, shared.ErrNotFound
}
func (policyDouble) Lock(context.Context, work.AutoAssignScope, shared.ID) (work.AutoAssignPolicy, error) {
	return work.AutoAssignPolicy{}, shared.ErrNotFound
}
func (policyDouble) Upsert(context.Context, work.AutoAssignPolicy) error { return nil }
func (policyDouble) Delete(context.Context, work.AutoAssignScope, shared.ID) error {
	return nil
}
func (policyDouble) SaveState(context.Context, work.AutoAssignPolicy) error { return nil }

var _ AutoAssignPolicies = policyDouble{}

// The comment store's double, for the same reason the others exist.
type commentDouble struct{}

func (commentDouble) Find(context.Context, shared.ID) (work.Comment, error) {
	return work.Comment{}, shared.ErrNotFound
}
func (commentDouble) List(context.Context, shared.ID, Page) (CommentPage, error) {
	return CommentPage{}, nil
}
func (commentDouble) Insert(context.Context, work.Comment) error       { return nil }
func (commentDouble) SetBody(context.Context, work.Comment, int) error { return nil }
func (commentDouble) SetDeleted(context.Context, work.Comment, int) error {
	return nil
}

var _ Comments = commentDouble{}

// The reminder store's double, for the same reason the others exist.
type reminderDouble struct{}

func (reminderDouble) Find(context.Context, shared.ID) (work.Reminder, error) {
	return work.Reminder{}, shared.ErrNotFound
}
func (reminderDouble) ListForItem(context.Context, shared.ID) ([]work.Reminder, error) {
	return nil, nil
}
func (reminderDouble) ListPendingForItem(context.Context, shared.ID) ([]work.Reminder, error) {
	return nil, nil
}
func (reminderDouble) CountForItem(context.Context, shared.ID) (int, error) { return 0, nil }
func (reminderDouble) Insert(context.Context, work.Reminder) error          { return nil }
func (reminderDouble) Update(context.Context, work.Reminder, int) error     { return nil }
func (reminderDouble) Reschedule(context.Context, work.Reminder) error      { return nil }
func (reminderDouble) Delete(context.Context, shared.ID, int) error         { return nil }
func (reminderDouble) ClaimDue(context.Context, time.Time, int) ([]work.Reminder, error) {
	return nil, nil
}
func (reminderDouble) Settle(context.Context, shared.ID, work.ReminderState) (bool, error) {
	return false, nil
}
func (reminderDouble) NextMoment(context.Context) (*time.Time, error) { return nil, nil }

var _ Reminders = reminderDouble{}

// The series store's double, for the same reason the others exist.
type recurrenceDouble struct{}

func (recurrenceDouble) FindForItem(context.Context, shared.ID) (work.RecurrenceRule, error) {
	return work.RecurrenceRule{}, shared.ErrNotFound
}
func (recurrenceDouble) Insert(context.Context, work.RecurrenceRule) error      { return nil }
func (recurrenceDouble) Update(context.Context, work.RecurrenceRule, int) error { return nil }
func (recurrenceDouble) Delete(context.Context, work.RecurrenceRule, int) error { return nil }
func (recurrenceDouble) ClaimToMaterialize(
	context.Context, time.Time, int,
) ([]work.RecurrenceRule, error) {
	return nil, nil
}
func (recurrenceDouble) Advance(context.Context, work.RecurrenceRule, time.Time) (bool, error) {
	return false, nil
}
func (recurrenceDouble) OpenOccurrences(context.Context, shared.ID) (int, error) { return 0, nil }
func (recurrenceDouble) LatestCompletion(context.Context, shared.ID) (*time.Time, error) {
	return nil, nil
}

var _ Recurrences = recurrenceDouble{}

// The sibling level is decided by two identifiers, not one: the same parent in another collection
// is not a sibling, and a port that took only the parent could not express the level directly
// under a collection at all.
func TestTheSiblingLevelOfAnItemNeedsBothIdentifiers(t *testing.T) {
	if _, err := (itemDouble{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing item is reported as %v", err)
	}

	key, err := (itemDouble{}).LastOrderKey(t.Context(), "collection", "")
	if err != nil {
		t.Fatalf("an empty level is reported as an error: %v", err)
	}
	if key != "" {
		t.Errorf("an empty level answered %q rather than nothing", key)
	}
}
