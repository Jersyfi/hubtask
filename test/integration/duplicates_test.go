// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// K-04's half that only a real database can answer: what pgvector calls near, what the statement
// refuses to call near, and whether one workspace's neighbourhood can reach into another's.
//
// The vectors are the hybrid fixture's synthetic axes rather than a model's output, for that
// file's reason: two vectors on one axis are identical, two on different axes are orthogonal, and
// a mixture sits at a similarity this file can state exactly. What is under test is the statement,
// not a model.

type neighbourhood struct {
	collection shared.ID
	subject    shared.ID
	// twin is the near-duplicate, cousin a distant relative below the floor, child a work package
	// under the subject, and stranger an entry pointing somewhere else entirely.
	twin, cousin, child, stranger shared.ID
}

func seedNeighbourhood(ctx context.Context, t *testing.T) neighbourhood {
	t.Helper()
	requirePgvector(ctx, t)
	seedContainerTenants(ctx, t)

	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	built := neighbourhood{
		collection: collection,
		subject:    freshID(t), twin: freshID(t), cousin: freshID(t),
		child: freshID(t), stranger: freshID(t),
	}
	axis := freshAxis()

	seed := []struct {
		id     shared.ID
		title  string
		vector []float32
		child  bool
	}{
		{built.subject, "Renew the domain registration", meaningOf(axis), false},
		{built.twin, "Renew the domain name before it lapses", meaningNear(axis, 0.97), false},
		// Related enough to be found by a search and not enough to be called the same thing.
		{built.cousin, "Move the mailboxes to the new host", meaningNear(axis, 0.55), false},
		// Under the subject, and as near as a piece of one thing is to the thing.
		{built.child, "Pay the invoice", meaningNear(axis, 0.99), true},
		{built.stranger, "Buy a new kettle", meaningOf(freshAxis()), false},
	}

	previous := ""
	items := itemRepo()
	embeddings := postgres.NewEmbeddingRepository()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for _, entry := range seed {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			previous = key

			item := taskIn(tenantA, authorA, collection, entry.id, entry.title, key)
			if entry.child {
				item.Type = work.ItemWorkPackage
				item.ParentID = built.subject
				item.Path = work.RootPath(built.subject) + entry.id.String() + work.PathSeparator
				item.Depth = 2
			}
			if err := items.Insert(ctx, item); err != nil {
				return err
			}
			if err := embeddings.Store(ctx, repository.StoredEmbedding{
				ItemID: entry.id, Model: "test-embed-1", Vector: entry.vector,
				SourceDigest: suggestion.Digest(entry.title, ""),
				UpdatedAt:    time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the neighbourhood: %v", err)
	}
	return built
}

// The sentence the task is for: an entry that is a near-duplicate of another is found, and one that
// merely shares a subject is not.
func TestANearDuplicateIsFoundAndAMereRelativeIsNot(t *testing.T) {
	ctx := context.Background()
	f := seedNeighbourhood(ctx, t)
	embeddings := postgres.NewEmbeddingRepository()

	var near repository.Nearby
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		near, err = embeddings.Near(ctx, f.subject, 0.9, 10)
		return err
	}); err != nil {
		t.Fatalf("reading the neighbourhood: %v", err)
	}

	if !near.Embedded || near.Model != "test-embed-1" {
		t.Fatalf("the subject came back as %+v, want it embedded and naming its model", near)
	}
	if len(near.Candidates) != 1 {
		t.Fatalf("%d neighbours, want the twin alone: %+v", len(near.Candidates), near.Candidates)
	}
	found := near.Candidates[0]
	if found.ItemID != f.twin {
		t.Errorf("the neighbour is %s, want the twin", found.ItemID)
	}
	if found.Similarity < 0.9 || found.Similarity > 1.0001 {
		t.Errorf("the similarity is %v, want it above the floor and no more than one",
			found.Similarity)
	}
	// The path the permission question is asked against travels with it, or the narrowing above
	// could only ask about the collection.
	if found.CollectionID != f.collection || found.HubID.IsZero() {
		t.Errorf("the neighbour carries %s / %s", found.CollectionID, found.HubID)
	}
}

// A work package under an entry is as near as a piece of a thing is to the thing, and it is not a
// duplicate of it. Neither is the entry a duplicate of its own child.
func TestAnEntrysOwnBranchIsNeverItsDuplicate(t *testing.T) {
	ctx := context.Background()
	f := seedNeighbourhood(ctx, t)
	embeddings := postgres.NewEmbeddingRepository()

	for _, testCase := range []struct {
		name   string
		of     shared.ID
		absent shared.ID
	}{
		{"the child of the subject", f.subject, f.child},
		{"the parent of the child", f.child, f.subject},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var near repository.Nearby
			if err := read(ctx, t, tenantA, func(ctx context.Context) error {
				var err error
				near, err = embeddings.Near(ctx, testCase.of, 0.5, 20)
				return err
			}); err != nil {
				t.Fatalf("reading the neighbourhood: %v", err)
			}
			for _, candidate := range near.Candidates {
				if candidate.ItemID == testCase.absent {
					t.Errorf("%s was proposed as a duplicate of %s", testCase.absent, testCase.of)
				}
				if candidate.ItemID == testCase.of {
					t.Error("an entry was proposed as its own duplicate")
				}
			}
		})
	}
}

// Gate SG-3 for the one statement this task adds. The neighbourhood is read from next door, with
// the neighbour's own identifiers, and answers nothing at all - not "no neighbours", but "there is
// no such entry to have any".
func TestOneWorkspacesNeighbourhoodIsInvisibleNextDoor(t *testing.T) {
	ctx := context.Background()
	f := seedNeighbourhood(ctx, t)
	embeddings := postgres.NewEmbeddingRepository()

	var near repository.Nearby
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		near, err = embeddings.Near(ctx, f.subject, 0.5, 20)
		return err
	}); err != nil {
		t.Fatalf("B's read: %v", err)
	}
	if near.Embedded || len(near.Candidates) != 0 {
		t.Errorf("B read A's neighbourhood: %+v", near)
	}
}

// An entry the embedding pass has not reached is found by nobody and finds nobody, and that is an
// answer rather than an error (J-10's third degradation).
func TestAnEntryWithNoVectorHasNoNeighbourhood(t *testing.T) {
	ctx := context.Background()
	f := seedNeighbourhood(ctx, t)
	embeddings := postgres.NewEmbeddingRepository()

	unembedded := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		key, err := service.OrderKeyAfter("")
		if err != nil {
			return err
		}
		return itemRepo().Insert(ctx, taskIn(tenantA, authorA, f.collection, unembedded,
			"Renew the domain registration", key))
	}); err != nil {
		t.Fatalf("writing an entry the pass has not reached: %v", err)
	}

	var near repository.Nearby
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		near, err = embeddings.Near(ctx, unembedded, 0.5, 20)
		return err
	}); err != nil {
		t.Fatalf("reading the neighbourhood: %v", err)
	}
	if near.Embedded || len(near.Candidates) != 0 {
		t.Errorf("an entry with no vector answered %+v", near)
	}
}
