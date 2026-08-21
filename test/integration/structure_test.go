// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"slices"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The buckets and labels a collection carries, against a real database (B-09): the unique index
// that decides a name, the ranks a board is ordered by, the OR-set tags a merge reads, and a
// cross-tenant negative for every method (gate SG-3).

func bucketRepo() postgres.BucketRepository { return postgres.NewBucketRepository() }
func labelRepo() postgres.LabelRepository   { return postgres.NewLabelRepository() }

func itemLabelRepo() postgres.ItemLabelRepository { return postgres.NewItemLabelRepository() }

func bucketIn(tenant, collection, id shared.ID, name, orderKey string) work.Bucket {
	return work.Bucket{
		ID: id, TenantID: tenant, CollectionID: collection,
		Name: name, OrderKey: orderKey, Version: 1,
	}
}

func labelIn(tenant, collection, id shared.ID, name string) work.Label {
	return work.Label{
		ID: id, TenantID: tenant, CollectionID: collection,
		Name: name, ColorToken: "accent.red", Version: 1,
	}
}

// seedBucket writes one bucket into a collection and returns it as it was written.
func seedBucket(
	ctx context.Context, t *testing.T, tenant, collection shared.ID, orderKey string,
) work.Bucket {
	t.Helper()

	bucket := bucketIn(tenant, collection, freshID(t), freshName(t), orderKey)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return bucketRepo().Insert(ctx, bucket)
	}); err != nil {
		t.Fatalf("seeding the bucket: %v", err)
	}
	return bucket
}

func seedLabel(ctx context.Context, t *testing.T, tenant, collection shared.ID) work.Label {
	t.Helper()

	label := labelIn(tenant, collection, freshID(t), freshName(t))
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return labelRepo().Insert(ctx, label)
	}); err != nil {
		t.Fatalf("seeding the label: %v", err)
	}
	return label
}

func findBucket(ctx context.Context, t *testing.T, tenant, id shared.ID) work.Bucket {
	t.Helper()

	var stored work.Bucket
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = bucketRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the bucket: %v", err)
	}
	return stored
}

func TestABucketIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	limit := 4
	bucket := bucketIn(tenantA, collection, freshID(t), freshName(t), "a0")
	bucket.WipLimit, bucket.IsDoneBucket, bucket.ColorToken = &limit, true, "surface.green"

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().Insert(ctx, bucket)
	}); err != nil {
		t.Fatalf("writing the bucket: %v", err)
	}

	stored := findBucket(ctx, t, tenantA, bucket.ID)

	switch {
	case stored.Name != bucket.Name || stored.OrderKey != "a0" || stored.Version != 1:
		t.Errorf("unexpected bucket: %+v", stored)
	case stored.WipLimit == nil || *stored.WipLimit != 4:
		t.Errorf("the limit did not survive: %v", stored.WipLimit)
	case !stored.IsDoneBucket || stored.ColorToken != "surface.green":
		t.Errorf("the optional fields did not survive: %+v", stored)
	case stored.TenantID != tenantA || stored.CollectionID != collection:
		t.Errorf("the row was written into the wrong place: %+v", stored)
	case stored.IsDeleted():
		t.Error("a new bucket is deleted")
	}
}

// The unique index decides a free name, and it does so case- and accent-insensitively: "Doing" and
// "dóing" are one name to a person, and a check that disagreed with a person would be a bug report
// waiting (B-09 acceptance).
func TestABucketNameIsUniquePerCollectionIgnoringCaseAndAccents(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	_, other := hubWithCollection(ctx, t, tenantA, authorA)

	first := bucketIn(tenantA, collection, freshID(t), "Doing", "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().Insert(ctx, first)
	}); err != nil {
		t.Fatalf("writing the first bucket: %v", err)
	}

	for _, name := range []string{"Doing", "doing", "DÓING"} {
		t.Run(name, func(t *testing.T) {
			err := write(ctx, t, tenantA, func(ctx context.Context) error {
				return bucketRepo().Insert(ctx, bucketIn(tenantA, collection, freshID(t), name, "a1"))
			})
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("%q was accepted beside %q: %v", name, first.Name, err)
			}
		})
	}

	// Another collection is another board, and the same name is free there.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().Insert(ctx, bucketIn(tenantA, other, freshID(t), "Doing", "a0"))
	}); err != nil {
		t.Fatalf("the name was refused in another collection: %v", err)
	}

	// And it is free again once the first one is deleted: the index is partial on deleted_at.
	deleted, _, err := first.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting the first bucket: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().SetDeleted(ctx, deleted, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().Insert(ctx, bucketIn(tenantA, collection, freshID(t), "Doing", "a2"))
	}); err != nil {
		t.Fatalf("the name was still taken after the deletion: %v", err)
	}
}

// The board comes back in its manual order and without the deleted columns, and Find still returns
// a deleted one: the two answer different questions, and the difference is deliberate.
func TestListBucketsIsTheBoardLeftToRight(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	third := seedBucket(ctx, t, tenantA, collection, "a2")
	first := seedBucket(ctx, t, tenantA, collection, "a0")
	second := seedBucket(ctx, t, tenantA, collection, "a1")

	gone, _, err := second.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting a bucket: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().SetDeleted(ctx, gone, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}

	var board []work.Bucket
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		board, err = bucketRepo().List(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("listing the board: %v", err)
	}

	if len(board) != 2 || board[0].ID != first.ID || board[1].ID != third.ID {
		t.Fatalf("the board is %+v, want the first and the third", board)
	}
	if stored := findBucket(ctx, t, tenantA, second.ID); !stored.IsDeleted() {
		t.Error("Find hid the deleted bucket, which turns a deletion into an absence")
	}
}

// The rank a new column lands after counts deleted ones: a restore has to land where it was, and
// reusing the key would put two columns in the same place.
func TestLastBucketOrderKeyCountsDeletedColumns(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	var empty string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		empty, err = bucketRepo().LastOrderKey(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the last rank of an empty board: %v", err)
	}
	if empty != "" {
		t.Errorf("an empty board reports the rank %q", empty)
	}

	seedBucket(ctx, t, tenantA, collection, "a0")
	last := seedBucket(ctx, t, tenantA, collection, "a5")
	deleted, _, err := last.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting the last bucket: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().SetDeleted(ctx, deleted, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}

	var key string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		key, err = bucketRepo().LastOrderKey(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the last rank: %v", err)
	}
	if key != "a5" {
		t.Errorf("last rank %q, want the deleted column's a5", key)
	}
}

func TestBucketNeighboursBoundAPosition(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	first := seedBucket(ctx, t, tenantA, collection, "a0")
	second := seedBucket(ctx, t, tenantA, collection, "a1")
	third := seedBucket(ctx, t, tenantA, collection, "a2")

	for _, c := range []struct {
		name             string
		before, moving   shared.ID
		previous, follow string
	}{
		{name: "before the second", before: second.ID, moving: third.ID, previous: "a0", follow: "a1"},
		{name: "before the first", before: first.ID, moving: third.ID, previous: "", follow: "a0"},
		{name: "at the end", before: "", moving: third.ID, previous: "a1", follow: ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var previous, follow string
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				var err error
				previous, follow, err = bucketRepo().
					Neighbours(ctx, collection, c.before, c.moving)
				return err
			}); err != nil {
				t.Fatalf("reading the bounds: %v", err)
			}
			if previous != c.previous || follow != c.follow {
				t.Errorf("bounds (%q, %q), want (%q, %q)", previous, follow, c.previous, c.follow)
			}
		})
	}
}

// What DeleteBucket owes its items: the leftmost remaining column, and every item moved into it.
func TestFirstOtherAndMoveItems(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	first := seedBucket(ctx, t, tenantA, collection, "a0")
	doomed := seedBucket(ctx, t, tenantA, collection, "a1")

	task := seedTask(ctx, t, tenantA, authorA, collection)
	putInBucket(ctx, t, task, doomed.ID)

	var fallback work.Bucket
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		fallback, err = bucketRepo().FirstOther(ctx, collection, doomed.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the fallback bucket: %v", err)
	}
	if fallback.ID != first.ID {
		t.Fatalf("the fallback is %s, want the leftmost column %s", fallback.ID, first.ID)
	}

	var moved int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		moved, err = bucketRepo().MoveItems(ctx, doomed.ID, fallback.ID, changedAt)
		return err
	}); err != nil {
		t.Fatalf("moving the items: %v", err)
	}
	if moved != 1 {
		t.Errorf("moved %d items, want 1", moved)
	}
	if got := bucketOf(ctx, t, task); got != first.ID.String() {
		t.Errorf("the item is in bucket %s, want %s", got, first.ID)
	}

	// With the emptied column gone, the board is down to one. The last column has no fallback, and
	// that is not a failure: its items then carry no bucket, which is the state the collection was
	// in before anybody made a board.
	gone, _, err := doomed.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting the emptied bucket: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().SetDeleted(ctx, gone, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}

	err = read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := bucketRepo().FirstOther(ctx, collection, first.ID)
		return err
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("a board with one column reported a fallback: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := bucketRepo().MoveItems(ctx, first.ID, "", changedAt)
		return err
	}); err != nil {
		t.Fatalf("moving the items out of every bucket: %v", err)
	}
	if got := bucketOf(ctx, t, task); got != "" {
		t.Errorf("the item is still in bucket %s", got)
	}
}

// An update against a version somebody else has moved on is refused rather than applied.
func TestBucketWritesAreVersionLocked(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	bucket := seedBucket(ctx, t, tenantA, collection, "a0")

	renamed := bucket
	renamed.Name = freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return bucketRepo().SetAttributes(ctx, renamed, 1)
	}); err != nil {
		t.Fatalf("writing the attributes: %v", err)
	}
	if stored := findBucket(ctx, t, tenantA, bucket.ID); stored.Version != 2 ||
		stored.Name != renamed.Name {
		t.Errorf("unexpected bucket after the update: %+v", stored)
	}

	for _, c := range []struct {
		name  string
		write func(context.Context) error
	}{
		{name: "attributes", write: func(ctx context.Context) error {
			return bucketRepo().SetAttributes(ctx, renamed, 1)
		}},
		{name: "order key", write: func(ctx context.Context) error {
			moved := renamed
			moved.OrderKey = "a9"
			return bucketRepo().SetOrderKey(ctx, moved, 1)
		}},
		{name: "deletion", write: func(ctx context.Context) error {
			gone, _, err := renamed.Deleted(changedAt)
			if err != nil {
				return err
			}
			return bucketRepo().SetDeleted(ctx, gone, 1)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			err := write(ctx, t, tenantA, c.write)
			if !errors.Is(err, shared.ErrVersionConflict) {
				t.Fatalf("a stale version was accepted: %v", err)
			}
		})
	}
}

// The cross-tenant negative for every bucket method (gate SG-3). Reads see nothing, writes reach
// nothing, and neither can tell "it belongs to somebody else" from "it does not exist".
func TestBucketsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	bucket := seedBucket(ctx, t, tenantA, collection, "a0")

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := bucketRepo().Find(ctx, bucket.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B read tenant A's bucket: %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var board []work.Bucket
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			board, err = bucketRepo().List(ctx, collection)
			return err
		}); err != nil {
			t.Fatalf("listing from tenant B: %v", err)
		}
		if len(board) != 0 {
			t.Errorf("tenant B saw tenant A's board: %+v", board)
		}
	})

	t.Run("last order key", func(t *testing.T) {
		var key string
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			key, err = bucketRepo().LastOrderKey(ctx, collection)
			return err
		}); err != nil {
			t.Fatalf("reading the last rank from tenant B: %v", err)
		}
		if key != "" {
			t.Errorf("tenant B read tenant A's rank %q", key)
		}
	})

	t.Run("neighbours", func(t *testing.T) {
		var previous, follow string
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			previous, follow, err = bucketRepo().Neighbours(ctx, collection, "", bucket.ID)
			return err
		}); err != nil {
			t.Fatalf("reading the bounds from tenant B: %v", err)
		}
		if previous != "" || follow != "" {
			t.Errorf("tenant B read tenant A's bounds (%q, %q)", previous, follow)
		}
	})

	t.Run("first other", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := bucketRepo().FirstOther(ctx, collection, freshID(t))
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B found a bucket of tenant A's: %v", err)
		}
	})

	t.Run("attributes", func(t *testing.T) {
		renamed := bucket
		renamed.Name = freshName(t)
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return bucketRepo().SetAttributes(ctx, renamed, 1)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B wrote tenant A's bucket: %v", err)
		}
	})

	t.Run("order key", func(t *testing.T) {
		moved := bucket
		moved.OrderKey = "z9"
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return bucketRepo().SetOrderKey(ctx, moved, 1)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B reordered tenant A's bucket: %v", err)
		}
	})

	t.Run("deletion", func(t *testing.T) {
		gone, _, err := bucket.Deleted(changedAt)
		if err != nil {
			t.Fatalf("deleting the bucket: %v", err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			return bucketRepo().SetDeleted(ctx, gone, 1)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B deleted tenant A's bucket: %v", err)
		}
	})

	t.Run("move items", func(t *testing.T) {
		var moved int
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			moved, err = bucketRepo().MoveItems(ctx, bucket.ID, "", changedAt)
			return err
		}); err != nil {
			t.Fatalf("moving items from tenant B: %v", err)
		}
		if moved != 0 {
			t.Errorf("tenant B moved %d of tenant A's items", moved)
		}
	})

	t.Run("insert lands in the caller's own tenant", func(t *testing.T) {
		// The object claims tenant A, the transaction belongs to tenant B. The tenant comes from
		// the transaction, so the row cannot be smuggled across the boundary - it simply fails on
		// the foreign key, because tenant B has no such collection.
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return bucketRepo().Insert(ctx, bucketIn(tenantA, collection, freshID(t), freshName(t), "a0"))
		})
		if err == nil {
			t.Fatal("tenant B wrote a bucket into tenant A's collection")
		}
	})
}

func TestALabelIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	label := labelIn(tenantA, collection, freshID(t), freshName(t))
	label.Description = "Needs a decision today"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().Insert(ctx, label)
	}); err != nil {
		t.Fatalf("writing the label: %v", err)
	}

	var stored work.Label
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		stored, err = labelRepo().Find(ctx, label.ID)
		return err
	}); err != nil {
		t.Fatalf("reading the label: %v", err)
	}

	switch {
	case stored.Name != label.Name || stored.ColorToken != "accent.red":
		t.Errorf("unexpected label: %+v", stored)
	case stored.Description != "Needs a decision today":
		t.Errorf("the description did not survive: %q", stored.Description)
	case stored.TenantID != tenantA || stored.CollectionID != collection:
		t.Errorf("the row was written into the wrong place: %+v", stored)
	}
}

func TestALabelNameIsUniquePerCollectionIgnoringCaseAndAccents(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().Insert(ctx, labelIn(tenantA, collection, freshID(t), "Urgent"))
	}); err != nil {
		t.Fatalf("writing the first label: %v", err)
	}

	for _, name := range []string{"Urgent", "urgent", "ÜRGENT"} {
		t.Run(name, func(t *testing.T) {
			err := write(ctx, t, tenantA, func(ctx context.Context) error {
				return labelRepo().Insert(ctx, labelIn(tenantA, collection, freshID(t), name))
			})
			if !errors.Is(err, shared.ErrConflict) {
				t.Fatalf("%q was accepted beside Urgent: %v", name, err)
			}
		})
	}
}

// A collection's vocabulary comes back by name and without the deleted labels.
func TestListLabelsLeavesOutTheDeletedOnes(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	kept := seedLabel(ctx, t, tenantA, collection)
	dropped := seedLabel(ctx, t, tenantA, collection)

	gone, _, err := dropped.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting the label: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().SetDeleted(ctx, gone, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}

	var vocabulary []work.Label
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		vocabulary, err = labelRepo().List(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("listing the labels: %v", err)
	}

	if len(vocabulary) != 1 || vocabulary[0].ID != kept.ID {
		t.Fatalf("the vocabulary is %+v, want only the kept label", vocabulary)
	}
}

func TestLabelWritesAreVersionLocked(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)

	renamed := label
	renamed.Name = freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().SetAttributes(ctx, renamed, 1)
	}); err != nil {
		t.Fatalf("writing the attributes: %v", err)
	}

	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().SetAttributes(ctx, renamed, 1)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("a stale version was accepted: %v", err)
	}
}

// The cross-tenant negative for every label method (gate SG-3).
func TestLabelsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)

	t.Run("find", func(t *testing.T) {
		err := read(ctx, t, tenantB, func(ctx context.Context) error {
			_, err := labelRepo().Find(ctx, label.ID)
			return err
		})
		if !errors.Is(err, shared.ErrNotFound) {
			t.Fatalf("tenant B read tenant A's label: %v", err)
		}
	})

	t.Run("list", func(t *testing.T) {
		var vocabulary []work.Label
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			vocabulary, err = labelRepo().List(ctx, collection)
			return err
		}); err != nil {
			t.Fatalf("listing from tenant B: %v", err)
		}
		if len(vocabulary) != 0 {
			t.Errorf("tenant B saw tenant A's labels: %+v", vocabulary)
		}
	})

	t.Run("attributes", func(t *testing.T) {
		renamed := label
		renamed.Name = freshName(t)
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return labelRepo().SetAttributes(ctx, renamed, 1)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B wrote tenant A's label: %v", err)
		}
	})

	t.Run("deletion", func(t *testing.T) {
		gone, _, err := label.Deleted(changedAt)
		if err != nil {
			t.Fatalf("deleting the label: %v", err)
		}
		err = write(ctx, t, tenantB, func(ctx context.Context) error {
			return labelRepo().SetDeleted(ctx, gone, 1)
		})
		if !errors.Is(err, shared.ErrVersionConflict) {
			t.Fatalf("tenant B deleted tenant A's label: %v", err)
		}
	})

	t.Run("insert", func(t *testing.T) {
		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return labelRepo().Insert(ctx, labelIn(tenantA, collection, freshID(t), freshName(t)))
		})
		if err == nil {
			t.Fatal("tenant B wrote a label into tenant A's collection")
		}
	})
}

// The membership and the tag are written together, and the tag is what survives an offline merge
// (offline-sync.md §4.2). A membership without one would merge as last writer wins over the whole
// set, which is the loss the OR-set exists to prevent.
func TestAddingALabelWritesTheMembershipAndTheTag(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	// Twice, because a client that repeats a request after a lost response must not be refused.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label again: %v", err)
	}

	carried := itemLabels(ctx, t, tenantA, task)
	if len(carried) != 1 || carried[0] != label.ID {
		t.Fatalf("the item carries %v, want the label", carried)
	}

	elements := labelElements(ctx, t, tenantA, task)
	if len(elements) != 1 || !elements[0].IsPresent() {
		t.Fatalf("the tags say the label is absent: %+v", elements)
	}
	if elements[0].AddedAt.Compare(added) != 0 {
		t.Errorf("the addition tag is %v, want %v", elements[0].AddedAt, added)
	}
}

// A removal keeps the addition's tag. Erasing it would make a concurrent re-add on another device
// indistinguishable from an element that had never been added at all.
func TestRemovingALabelKeepsTheAdditionTag(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	removed, err := shared.NewHLC(changedAt, 0, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	var carriedIt bool
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		carriedIt, err = itemLabelRepo().Remove(ctx, task, label.ID, removed)
		return err
	}); err != nil {
		t.Fatalf("removing the label: %v", err)
	}
	if !carriedIt {
		t.Error("the removal reported that the item did not carry the label")
	}

	if carried := itemLabels(ctx, t, tenantA, task); len(carried) != 0 {
		t.Errorf("the item still carries %v", carried)
	}

	elements := labelElements(ctx, t, tenantA, task)
	if len(elements) != 1 || elements[0].IsPresent() {
		t.Fatalf("the tags say the label is present: %+v", elements)
	}
	if elements[0].AddedAt.IsZero() {
		t.Error("the addition tag was erased - a concurrent re-add could no longer be merged")
	}

	// Removing what the item does not carry is not a failure: the tag is still recorded, because a
	// device that removes something this replica never saw has made a decision to merge against.
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		carried, err := itemLabelRepo().Remove(ctx, task, label.ID, removed)
		if carried {
			t.Error("the second removal reported that the item carried the label")
		}
		return err
	}); err != nil {
		t.Fatalf("removing the label again: %v", err)
	}
}

// A deleted label is out of the collection's vocabulary, so it is out of the chips a client
// renders - without anything having had to rewrite the items that carried it.
func TestADeletedLabelStopsBeingCarried(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	gone, _, err := label.Deleted(changedAt)
	if err != nil {
		t.Fatalf("deleting the label: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return labelRepo().SetDeleted(ctx, gone, 1)
	}); err != nil {
		t.Fatalf("writing the deletion: %v", err)
	}

	if carried := itemLabels(ctx, t, tenantA, task); len(carried) != 0 {
		t.Errorf("the item still carries the deleted label: %v", carried)
	}
}

// The cross-tenant negative for the item label methods (gate SG-3).
func TestItemLabelsAreInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	label := seedLabel(ctx, t, tenantA, collection)
	task := seedTask(ctx, t, tenantA, authorA, collection)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	t.Run("list", func(t *testing.T) {
		if carried := itemLabels(ctx, t, tenantB, task); len(carried) != 0 {
			t.Errorf("tenant B read tenant A's labels: %v", carried)
		}
	})

	t.Run("elements", func(t *testing.T) {
		if elements := labelElements(ctx, t, tenantB, task); len(elements) != 0 {
			t.Errorf("tenant B read tenant A's tags: %+v", elements)
		}
	})

	t.Run("remove", func(t *testing.T) {
		if err := write(ctx, t, tenantB, func(ctx context.Context) error {
			carried, err := itemLabelRepo().Remove(ctx, task, label.ID, added)
			if carried {
				t.Error("tenant B removed a label of tenant A's")
			}
			return err
		}); err != nil {
			t.Fatalf("removing from tenant B: %v", err)
		}
		if carried := itemLabels(ctx, t, tenantA, task); !slices.Contains(carried, label.ID) {
			t.Error("tenant A's item lost its label to tenant B")
		}
	})

	// A label the item does not already carry, so that the write is a genuine insert rather than
	// one the primary key swallows: the foreign key is (tenant_id, item_id), and tenant B has no
	// such item to hang it on.
	t.Run("add", func(t *testing.T) {
		other := seedLabel(ctx, t, tenantA, collection)

		err := write(ctx, t, tenantB, func(ctx context.Context) error {
			return itemLabelRepo().Add(ctx, task, other.ID, added)
		})
		if err == nil {
			t.Fatal("tenant B put a label on tenant A's item")
		}
		if carried := itemLabels(ctx, t, tenantA, task); slices.Contains(carried, other.ID) {
			t.Error("tenant B's label reached tenant A's item")
		}
	})
}

func itemLabels(ctx context.Context, t *testing.T, tenant, item shared.ID) []shared.ID {
	t.Helper()

	var carried []shared.ID
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		carried, err = itemLabelRepo().List(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("listing the labels of the item: %v", err)
	}
	return carried
}

func labelElements(ctx context.Context, t *testing.T, tenant, item shared.ID) []work.SetElement {
	t.Helper()

	var elements []work.SetElement
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		elements, err = itemLabelRepo().Elements(ctx, item)
		return err
	}); err != nil {
		t.Fatalf("listing the tags of the item: %v", err)
	}
	return elements
}

// putInBucket sets a column the repositories do not write yet: putting an item into a bucket is its
// own use case (B-09, step 9), and a fixture that set the field on the struct would be dropped.
func putInBucket(ctx context.Context, t *testing.T, item, bucket shared.ID) {
	t.Helper()

	tag, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET bucket_id = $2 WHERE id = $1", item.String(), bucket.String())
	if err != nil {
		t.Fatalf("putting the item into the bucket: %v", err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("putting the item into the bucket matched %d rows, want 1", tag.RowsAffected())
	}
}

// bucketOf reads the column back, as the string it is stored as: the empty string is no bucket.
func bucketOf(ctx context.Context, t *testing.T, item shared.ID) string {
	t.Helper()

	var bucket *string
	if err := adminPool(ctx, t).QueryRow(ctx,
		"SELECT bucket_id::text FROM work_item WHERE id = $1", item.String()).
		Scan(&bucket); err != nil {
		t.Fatalf("reading the bucket of the item: %v", err)
	}
	if bucket == nil {
		return ""
	}
	return *bucket
}

// Invariant I-W6 against a real database: a subtree that moves to another collection loses the
// references the destination cannot resolve, and the answer names what was lost. Reported rather
// than discovered - a board that silently reassigned somebody's cards would be indistinguishable
// from one that lost them.
func TestAMoveToAnotherCollectionDropsWhatTheDestinationCannotResolve(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub, source := hubWithCollection(ctx, t, tenantA, authorA)

	destinationID := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return containerRepo().Insert(ctx,
			collectionIn(tenantA, authorA, destinationID, hub, freshName(t), "a1"))
	}); err != nil {
		t.Fatalf("seeding the destination: %v", err)
	}

	bucket := seedBucket(ctx, t, tenantA, source, "a0")
	label := seedLabel(ctx, t, tenantA, source)
	task := seedTask(ctx, t, tenantA, authorA, source)
	putInBucket(ctx, t, task, bucket.ID)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	moved := findWorkItem(ctx, t, tenantA, task)
	var dropped []work.DroppedReference
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		_, dropped, err = itemRepo().MoveSubtree(ctx, repository.Move{
			Item: moved, CollectionID: destinationID,
			OldPrefix: moved.Path, NewPrefix: moved.Path, DepthDelta: 0,
			OrderKey: "a0", UpdatedAt: changedAt, ExpectedVersion: moved.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("moving the subtree: %v", err)
	}

	kinds := map[work.ReferenceKind]shared.ID{}
	for _, reference := range dropped {
		if reference.ItemID != task {
			t.Errorf("a loss is attributed to %s", reference.ItemID)
		}
		kinds[reference.Kind] = reference.ID
	}
	if kinds[work.ReferenceLabel] != label.ID {
		t.Errorf("the label was not reported as lost: %+v", dropped)
	}
	if kinds[work.ReferenceBucket] != bucket.ID {
		t.Errorf("the bucket was not reported as lost: %+v", dropped)
	}

	if carried := itemLabels(ctx, t, tenantA, task); len(carried) != 0 {
		t.Errorf("the entry still carries a label of the collection it left: %v", carried)
	}
	if got := bucketOf(ctx, t, task); got != "" {
		t.Errorf("the entry is still in bucket %s", got)
	}
}

// A move inside one collection resolves everything, so nothing is dropped: the board and the
// vocabulary are the same ones.
func TestAMoveWithinOneCollectionDropsNothing(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	bucket := seedBucket(ctx, t, tenantA, collection, "a0")
	label := seedLabel(ctx, t, tenantA, collection)
	task := seedTask(ctx, t, tenantA, authorA, collection)
	putInBucket(ctx, t, task, bucket.ID)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, task, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	moved := findWorkItem(ctx, t, tenantA, task)
	var dropped []work.DroppedReference
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		_, dropped, err = itemRepo().MoveSubtree(ctx, repository.Move{
			Item: moved, CollectionID: collection,
			OldPrefix: moved.Path, NewPrefix: moved.Path, DepthDelta: 0,
			OrderKey: "a1", BucketID: bucket.ID,
			UpdatedAt: changedAt, ExpectedVersion: moved.Version,
		})
		return err
	}); err != nil {
		t.Fatalf("moving the subtree: %v", err)
	}

	if len(dropped) != 0 {
		t.Errorf("a move inside one collection dropped %+v", dropped)
	}
	if got := bucketOf(ctx, t, task); got != bucket.ID.String() {
		t.Errorf("the entry left its column: %s", got)
	}
	if carried := itemLabels(ctx, t, tenantA, task); len(carried) != 1 {
		t.Errorf("the entry lost its label: %v", carried)
	}
}

// The column an entry sits in is written and read back like any other field of its row.
func TestAnEntryCarriesItsColumn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	bucket := seedBucket(ctx, t, tenantA, collection, "a0")
	task := seedTask(ctx, t, tenantA, authorA, collection)

	stored := findWorkItem(ctx, t, tenantA, task)
	if !stored.BucketID.IsZero() {
		t.Fatalf("a new entry starts in column %s", stored.BucketID)
	}

	stored.BucketID = bucket.ID
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, stored, stored.Version)
	}); err != nil {
		t.Fatalf("writing the column: %v", err)
	}
	if after := findWorkItem(ctx, t, tenantA, task); after.BucketID != bucket.ID {
		t.Errorf("the entry is in column %s, want %s", after.BucketID, bucket.ID)
	}

	// And taking it off the board is a write like any other, rather than a state nothing can express.
	stored = findWorkItem(ctx, t, tenantA, task)
	stored.BucketID = ""
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemRepo().SetAttributes(ctx, stored, stored.Version)
	}); err != nil {
		t.Fatalf("clearing the column: %v", err)
	}
	if after := findWorkItem(ctx, t, tenantA, task); !after.BucketID.IsZero() {
		t.Errorf("the entry is still in column %s", after.BucketID)
	}
}

// The labels of a page, in one query: what `expand=labels` reads. An entry that carries none is
// absent from the map rather than present with an empty list - the caller writes the empty list,
// because the caller is the one that knows the page.
func TestTheLabelsOfAPageComeBackInOneQuery(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)

	label := seedLabel(ctx, t, tenantA, collection)
	labelled := seedTask(ctx, t, tenantA, authorA, collection)
	bare := seedTask(ctx, t, tenantA, authorA, collection)

	added, err := shared.NewHLC(created, 1, "server")
	if err != nil {
		t.Fatalf("building the tag: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return itemLabelRepo().Add(ctx, labelled, label.ID, added)
	}); err != nil {
		t.Fatalf("adding the label: %v", err)
	}

	var carried map[shared.ID][]shared.ID
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		carried, err = itemLabelRepo().ListFor(ctx, []shared.ID{labelled, bare})
		return err
	}); err != nil {
		t.Fatalf("listing the labels of the page: %v", err)
	}

	if len(carried[labelled]) != 1 || carried[labelled][0] != label.ID {
		t.Errorf("the labelled entry carries %v", carried[labelled])
	}
	if _, present := carried[bare]; present {
		t.Errorf("the bare entry is in the map: %v", carried[bare])
	}

	// The cross-tenant negative for ListFor (gate SG-3).
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		other, err := itemLabelRepo().ListFor(ctx, []shared.ID{labelled, bare})
		if len(other) != 0 {
			t.Errorf("tenant B read tenant A's labels: %v", other)
		}
		return err
	}); err != nil {
		t.Fatalf("listing from tenant B: %v", err)
	}
}
