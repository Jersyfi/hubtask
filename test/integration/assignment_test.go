// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Who an entry is on, against a real database (C-01): the column an assignment writes under its
// optimistic lock, the OR-set tags a member merge reads, and a cross-tenant negative for every
// method (gate SG-3).

func itemMemberRepo() postgres.ItemMemberRepository { return postgres.NewItemMemberRepository() }

// seedAccount writes one account into a tenant and returns it. Directly, because inviting somebody
// is its own use case and this file is about what an assignment stores.
func seedAccount(ctx context.Context, t *testing.T, tenant shared.ID) shared.ID {
	t.Helper()

	id := freshID(t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		"INSERT INTO account (id, tenant_id, display_name) VALUES ($1, $2, $3)",
		id.String(), tenant.String(), freshName(t)); err != nil {
		t.Fatalf("seeding the account: %v", err)
	}
	return id
}

func TestTheAssigneeIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	item := findItem(ctx, t, tenantA, task)
	assigned := item.Assigned(account, changedAt)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAssignee(ctx, assigned, item.Version)
	}); err != nil {
		t.Fatalf("assigning: %v", err)
	}

	stored := findItem(ctx, t, tenantA, task)
	if stored.AssigneeID != account {
		t.Errorf("the entry is on %q, want %q", stored.AssigneeID, account)
	}
	if stored.Version != item.Version+1 {
		t.Errorf("version %d, want the write to have spent one", stored.Version)
	}

	// The zero identifier is nobody, and reaches the column as NULL rather than as a zero UUID: a
	// row of zeroes would be a reference to an account that cannot exist.
	cleared := stored.Unassigned(changedAt)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAssignee(ctx, cleared, stored.Version)
	}); err != nil {
		t.Fatalf("unassigning: %v", err)
	}
	if again := findItem(ctx, t, tenantA, task); !again.AssigneeID.IsZero() {
		t.Errorf("the entry is still on %q", again.AssigneeID)
	}
}

// The optimistic lock is in the WHERE clause: the update matches nothing when somebody else has
// moved the row on, and the caller learns that rather than overwriting them.
func TestAssigningIsVersionLocked(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	item := findItem(ctx, t, tenantA, task)
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAssignee(ctx, item.Assigned(account, changedAt), item.Version+1)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error = %v, want a version conflict", err)
	}
	if stored := findItem(ctx, t, tenantA, task); !stored.AssigneeID.IsZero() {
		t.Errorf("the entry was assigned anyway, to %q", stored.AssigneeID)
	}
}

// The membership and the tag are written together, and the tag is what survives an offline merge
// (offline-sync.md §4.2). A membership without one would merge as last writer wins over the whole
// set, which is the loss the OR-set exists to prevent.
func TestAddingAMemberWritesTheMembershipAndTheTag(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemMemberRepo().Add(ctx, task, account, added)
	}); err != nil {
		t.Fatalf("adding the member: %v", err)
	}

	// Twice, because a client that repeats a request after a lost response must not be refused.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemMemberRepo().Add(ctx, task, account, added)
	}); err != nil {
		t.Fatalf("adding the member again: %v", err)
	}

	carried := itemMembers(ctx, t, tenantA, task)
	if len(carried) != 1 || carried[0] != account {
		t.Fatalf("the entry carries %v, want the account", carried)
	}

	elements := memberElements(ctx, t, tenantA, task)
	if len(elements) != 1 || !elements[0].IsPresent() {
		t.Fatalf("the tags say the account is absent: %+v", elements)
	}
	if elements[0].AddedAt.Compare(added) != 0 {
		t.Errorf("the addition tag is %v, want %v", elements[0].AddedAt, added)
	}
}

// A removal keeps the addition's tag. Erasing it would make a concurrent re-add on another device
// indistinguishable from an element that had never been added at all.
func TestRemovingAMemberKeepsTheAdditionTag(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	removed, err := shared.NewHLC(changedAt, 0, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemMemberRepo().Add(ctx, task, account, added)
	}); err != nil {
		t.Fatalf("adding the member: %v", err)
	}

	var carriedIt bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		carriedIt, err = itemMemberRepo().Remove(ctx, task, account, removed)
		return err
	}); err != nil {
		t.Fatalf("removing the member: %v", err)
	}
	if !carriedIt {
		t.Error("the removal reported that the entry did not carry the account")
	}

	if carried := itemMembers(ctx, t, tenantA, task); len(carried) != 0 {
		t.Errorf("the entry still carries %v", carried)
	}

	elements := memberElements(ctx, t, tenantA, task)
	if len(elements) != 1 || elements[0].IsPresent() {
		t.Fatalf("the tags say the account is present: %+v", elements)
	}
	if elements[0].AddedAt.IsZero() {
		t.Error("the addition tag was erased - a concurrent re-add could no longer be merged")
	}

	// Removing what the entry does not carry is not a failure: the tag is still recorded, because a
	// device that removes something this replica never saw has made a decision to merge against.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		carried, err := itemMemberRepo().Remove(ctx, task, account, removed)
		if carried {
			t.Error("the second removal reported that the entry carried the account")
		}
		return err
	}); err != nil {
		t.Fatalf("removing the member again: %v", err)
	}
}

// Two devices adding two different people converge to both, and a concurrent add and remove of the
// same person resolves by tag rather than by row order. The merge itself is decided in the domain;
// what this proves is that the tags a real database stores are the ones it decides from.
func TestTwoMemberSetsMergeToTheUnion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	mine := seedAccount(ctx, t, tenantA)
	theirs := seedAccount(ctx, t, tenantA)

	early, err := shared.NewHLC(created, 0, "device-a")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	late, err := shared.NewHLC(changedAt, 0, "device-b")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := itemMemberRepo().Add(ctx, task, mine, early); err != nil {
			return err
		}
		return itemMemberRepo().Add(ctx, task, theirs, early)
	}); err != nil {
		t.Fatalf("adding both members: %v", err)
	}

	// One device removes one of the two, later. The other's addition must survive it - which is
	// exactly what last writer wins over the whole array would destroy.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := itemMemberRepo().Remove(ctx, task, mine, late)
		return err
	}); err != nil {
		t.Fatalf("removing one member: %v", err)
	}

	elements := memberElements(ctx, t, tenantA, task)
	present := work.PresentElements(elements)
	if len(present) != 1 || present[0] != theirs {
		t.Errorf("the tags say %v is present, want only the member nobody removed", present)
	}

	// A re-add later than the removal brings them back, and by the tag rather than by the order the
	// rows happen to be scanned in.
	later, err := shared.NewHLC(changedAt.Add(1), 0, "device-a")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemMemberRepo().Add(ctx, task, mine, later)
	}); err != nil {
		t.Fatalf("re-adding the member: %v", err)
	}
	if present := work.PresentElements(memberElements(ctx, t, tenantA, task)); len(present) != 2 {
		t.Errorf("the tags say %v is present, want both", present)
	}
}

// The two sets live in one table and must not read each other's tags: `set_name` is the whole of
// what keeps a label out of a member list, and it is a parameter rather than a constant of either
// repository.
func TestTheLabelAndMemberTagsDoNotReadEachOther(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	label := seedLabel(ctx, t, tenantA, collection)
	account := seedAccount(ctx, t, tenantA)

	tag, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := itemLabelRepo().Add(ctx, task, label.ID, tag); err != nil {
			return err
		}
		return itemMemberRepo().Add(ctx, task, account, tag)
	}); err != nil {
		t.Fatalf("adding a label and a member: %v", err)
	}

	labels := labelElements(ctx, t, tenantA, task)
	if len(labels) != 1 || labels[0].ElementID != label.ID {
		t.Errorf("the label tags are %+v", labels)
	}
	members := memberElements(ctx, t, tenantA, task)
	if len(members) != 1 || members[0].ElementID != account {
		t.Errorf("the member tags are %+v", members)
	}
}

func TestTheAssigneeIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	item := findItem(ctx, t, tenantA, task)
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return itemRepo().SetAssignee(ctx, item.Assigned(account, changedAt), item.Version)
	})
	// Row level security removed the row from the update's reach, so nothing matched - which comes
	// back as a version conflict, the same answer as a row somebody else moved on. A caller must
	// not be able to tell the two apart (multi-tenancy.md §2).
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error = %v, want a version conflict", err)
	}
	if stored := findItem(ctx, t, tenantA, task); !stored.AssigneeID.IsZero() {
		t.Errorf("tenant B assigned tenant A's entry to %q", stored.AssigneeID)
	}
}

func TestItemMembersAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	account := seedAccount(ctx, t, tenantA)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemMemberRepo().Add(ctx, task, account, added)
	}); err != nil {
		t.Fatalf("adding the member: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		if carried := itemMembers(ctx, t, tenantB, task); len(carried) != 0 {
			t.Errorf("tenant B read tenant A's members: %v", carried)
		}
	})

	t.Run("elements", func(t *testing.T) {
		if elements := memberElements(ctx, t, tenantB, task); len(elements) != 0 {
			t.Errorf("tenant B read tenant A's tags: %+v", elements)
		}
	})

	t.Run("remove", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			carried, err := itemMemberRepo().Remove(ctx, task, account, added)
			if carried {
				t.Error("tenant B removed a member of tenant A's")
			}
			return err
		}); err != nil {
			t.Fatalf("removing from tenant B: %v", err)
		}
		if carried := itemMembers(ctx, t, tenantA, task); !slices.Contains(carried, account) {
			t.Error("tenant A's entry lost its member to tenant B")
		}
	})

	// An account the entry does not already carry, so that the write is a genuine insert rather
	// than one the primary key swallows: the foreign key is (tenant_id, item_id), and tenant B has
	// no such entry to hang it on.
	t.Run("add", func(t *testing.T) {
		other := seedAccount(ctx, t, tenantA)

		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemMemberRepo().Add(ctx, task, other, added)
		})
		if err == nil {
			t.Fatal("tenant B put a member on tenant A's entry")
		}
		if carried := itemMembers(ctx, t, tenantA, task); slices.Contains(carried, other) {
			t.Error("tenant B's member reached tenant A's entry")
		}
	})
}

func itemMembers(ctx context.Context, t *testing.T, tenant, item shared.ID) []shared.ID {
	t.Helper()

	var carried []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		carried, err = itemMemberRepo().List(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("listing the members of the entry: %v", err)
	}
	return carried
}

func memberElements(ctx context.Context, t *testing.T, tenant, item shared.ID) []work.SetElement {
	t.Helper()

	var elements []work.SetElement
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		elements, err = itemMemberRepo().Elements(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("listing the member tags of the entry: %v", err)
	}
	return elements
}
