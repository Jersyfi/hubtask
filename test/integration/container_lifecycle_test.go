// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The container lifecycle against the real database (B-06): the four writes, the inherited archive
// the read computes, and a cross-tenant negative for every one of them (gate SG-3).

var changedAt = created.Add(time.Hour)

// collectionIn is the container fixture's collection counterpart: a collection needs a hub, and the
// hub is what the inherited archive is read from.
func collectionIn(tenant, author, id, parentID shared.ID, name, orderKey string) work.Container {
	container := containerIn(tenant, author, id, name, orderKey)
	container.Type = work.ContainerCollection
	container.ParentID = parentID
	return container
}

// hubWithCollection writes a hub and one collection in it, and hands back both identifiers.
func hubWithCollection(ctx context.Context, t *testing.T, tenant, author shared.ID) (shared.ID, shared.ID) {
	t.Helper()
	repo := containerRepo()
	hubID, collectionID := freshID(t), freshID(t)

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		if err := repo.Insert(ctx, containerIn(tenant, author, hubID, freshName(t), "a0")); err != nil {
			return err
		}
		return repo.Insert(ctx, collectionIn(tenant, author, collectionID, hubID, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("seeding the hub and its collection: %v", err)
	}
	return hubID, collectionID
}

func findContainer(ctx context.Context, t *testing.T, tenant, id shared.ID) work.Container {
	t.Helper()

	var stored work.Container
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		stored, err = containerRepo().Find(ctx, id)
		return err
	}); err != nil {
		t.Fatalf("reading the container: %v", err)
	}
	return stored
}

func TestSetContainerAttributesWritesEveryFieldAndBumpsTheVersion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()

	id := freshID(t)
	hub := containerIn(tenantA, authorA, id, freshName(t), "a0")
	hub.Description, hub.Icon, hub.ColorToken = "Everything personal", "home", "blue"
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, hub)
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	renamed := hub
	renamed.Name = freshName(t)
	renamed.Description, renamed.Icon, renamed.ColorToken = "", "house", ""
	renamed.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetAttributes(ctx, renamed, 1)
	}); err != nil {
		t.Fatalf("renaming the hub: %v", err)
	}

	stored := findContainer(ctx, t, tenantA, id)
	if stored.Name != renamed.Name || stored.Icon != "house" {
		t.Errorf("the rename did not land: %+v", stored)
	}
	// Every column is written, not only the ones that moved - which is what makes clearing a field
	// expressible at all.
	if stored.Description != "" || stored.ColorToken != "" {
		t.Errorf("the cleared fields survived: %+v", stored)
	}
	if stored.Version != 2 || !stored.UpdatedAt.Equal(changedAt) {
		t.Errorf("version %d at %v, want 2 at the new timestamp", stored.Version, stored.UpdatedAt)
	}
}

// The optimistic lock is in the WHERE clause, so the loser matches no row and is told rather than
// overwriting the winner (api-guidelines.md §5).
func TestSetContainerAttributesRefusesAStaleVersion(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()

	id := freshID(t)
	hub := containerIn(tenantA, authorA, id, freshName(t), "a0")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, hub)
	}); err != nil {
		t.Fatalf("writing the hub: %v", err)
	}

	renamed := hub
	renamed.Name = freshName(t)
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetAttributes(ctx, renamed, 7)
	})
	if !errors.Is(err, shared.ErrVersionConflict) {
		t.Fatalf("error %v, want a version conflict", err)
	}
	if code := shared.AsError(err).DetailCode; code != "containers.version_conflict" {
		t.Errorf("detail code %s, want containers.version_conflict", code)
	}
	if stored := findContainer(ctx, t, tenantA, id); stored.Version != 1 {
		t.Errorf("the losing write moved the row anyway: %+v", stored)
	}
}

// A rename into a name already taken at this level fails on the same index an insert fails on, so
// one condition keeps one answer.
func TestRenamingIntoATakenNameIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()

	taken, mover := freshID(t), freshID(t)
	takenName := freshName(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Insert(ctx, containerIn(tenantA, authorA, taken, takenName, "a0")); err != nil {
			return err
		}
		return repo.Insert(ctx, containerIn(tenantA, authorA, mover, freshName(t), "a1"))
	}); err != nil {
		t.Fatalf("seeding the two hubs: %v", err)
	}

	renamed := containerIn(tenantA, authorA, mover, takenName, "a1")
	renamed.UpdatedAt = changedAt
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetAttributes(ctx, renamed, 1)
	})
	if code := shared.AsError(err).DetailCode; code != "containers.name_taken" {
		t.Fatalf("detail code %s, want containers.name_taken (%v)", code, err)
	}
}

// The policies column holds four documented keys and this write owns one. A key nothing here wrote
// has to survive, or the first feature that stores one loses it to the next rename of a policy.
func TestSetPoliciesLeavesTheOtherKeysAlone(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	_, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	admin := adminPool(ctx, t)
	if _, err := admin.Exec(ctx,
		`UPDATE container SET policies = jsonb_set(policies, '{default_bucket_id}', '"b1"', true) WHERE id = $1`,
		collectionID.String()); err != nil {
		t.Fatalf("seeding a second policy key: %v", err)
	}

	configured := findContainer(ctx, t, tenantA, collectionID)
	configured.CompletionPolicy = work.CompletionRollup
	configured.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetPolicies(ctx, configured, configured.Version)
	}); err != nil {
		t.Fatalf("writing the policies: %v", err)
	}

	stored := findContainer(ctx, t, tenantA, collectionID)
	if stored.CompletionPolicy != work.CompletionRollup {
		t.Errorf("policy %q, want ROLLUP", stored.CompletionPolicy)
	}
	var other string
	if err := admin.QueryRow(ctx,
		`SELECT policies->>'default_bucket_id' FROM container WHERE id = $1`,
		collectionID.String()).Scan(&other); err != nil {
		t.Fatalf("reading the other key back: %v", err)
	}
	if other != "b1" {
		t.Errorf("the other key is now %q - writing one policy discarded it", other)
	}
}

// I-C3 as the read computes it: the hub's stamp travels with the collection's row, and nothing is
// written onto the collection itself.
func TestArchivingAHubIsInheritedByItsCollectionWithoutStampingIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	hub := findContainer(ctx, t, tenantA, hubID)
	hub.ArchivedAt = &changedAt
	hub.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetArchived(ctx, hub, hub.Version)
	}); err != nil {
		t.Fatalf("archiving the hub: %v", err)
	}

	collection := findContainer(ctx, t, tenantA, collectionID)
	if collection.ArchivedAt != nil {
		t.Errorf("archiving the hub stamped the collection: %v", collection.ArchivedAt)
	}
	if collection.ParentArchivedAt == nil || !collection.ParentArchivedAt.Equal(changedAt) {
		t.Fatalf("the hub's stamp did not travel with the collection: %v", collection.ParentArchivedAt)
	}
	if !collection.IsEffectivelyArchived() || collection.IsArchived() {
		t.Errorf("the two predicates disagree with the row: %+v", collection)
	}
	if collection.Version != 1 {
		t.Errorf("the collection's version moved: %d", collection.Version)
	}

	// And unarchiving the hub gives the collection its writability back, without ever having touched
	// it: the acceptance criterion, read off the database.
	hub = findContainer(ctx, t, tenantA, hubID)
	hub.ArchivedAt = nil
	hub.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetArchived(ctx, hub, hub.Version)
	}); err != nil {
		t.Fatalf("unarchiving the hub: %v", err)
	}
	if restored := findContainer(ctx, t, tenantA, collectionID); restored.IsEffectivelyArchived() {
		t.Errorf("the collection is still read-only after the hub came back: %+v", restored)
	}
}

// A collection archived in its own right stays archived when its hub is unarchived. That is the
// distinction the inherited stamp exists to keep, and it only holds because nothing is written down.
func TestACollectionArchivedInItsOwnRightSurvivesTheHubsReturn(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	collection := findContainer(ctx, t, tenantA, collectionID)
	collection.ArchivedAt = &changedAt
	collection.UpdatedAt = changedAt
	hub := findContainer(ctx, t, tenantA, hubID)
	hub.ArchivedAt = &changedAt
	hub.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.SetArchived(ctx, collection, collection.Version); err != nil {
			return err
		}
		return repo.SetArchived(ctx, hub, hub.Version)
	}); err != nil {
		t.Fatalf("archiving both: %v", err)
	}

	hub = findContainer(ctx, t, tenantA, hubID)
	hub.ArchivedAt = nil
	hub.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetArchived(ctx, hub, hub.Version)
	}); err != nil {
		t.Fatalf("unarchiving the hub: %v", err)
	}

	stored := findContainer(ctx, t, tenantA, collectionID)
	if !stored.IsArchived() {
		t.Errorf("the collection lost its own archiving when the hub came back: %+v", stored)
	}
	if stored.ParentArchivedAt != nil {
		t.Errorf("the hub's stamp is still there: %v", stored.ParentArchivedAt)
	}
}

// The list carries the inherited stamp too, so a client paging a hub's collections can grey out
// what it may not write to without reading each one.
func TestTheListCarriesTheInheritedArchive(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	hub := findContainer(ctx, t, tenantA, hubID)
	hub.ArchivedAt = &changedAt
	hub.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetArchived(ctx, hub, hub.Version)
	}); err != nil {
		t.Fatalf("archiving the hub: %v", err)
	}

	var page repository.ContainerPage
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		page, err = repo.List(ctx, repository.ContainerQuery{
			ParentID: hubID, Page: repository.Page{Size: 10},
		})
		return err
	}); err != nil {
		t.Fatalf("listing the hub's collections: %v", err)
	}

	if len(page.Containers) != 1 || page.Containers[0].ID != collectionID {
		t.Fatalf("unexpected page: %+v", page.Containers)
	}
	// The collection is still in the level: the client asking for an archived hub's collections has
	// just been told the hub is archived, and hiding its contents would make them unreachable.
	if !page.Containers[0].IsEffectivelyArchived() {
		t.Errorf("the listed collection does not report the hub's archiving: %+v", page.Containers[0])
	}
}

func TestSetPlacementMovesACollectionBetweenHubs(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	_, collectionID := hubWithCollection(ctx, t, tenantA, authorA)
	targetID := freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.Insert(ctx, containerIn(tenantA, authorA, targetID, freshName(t), "a1"))
	}); err != nil {
		t.Fatalf("writing the destination hub: %v", err)
	}

	collection := findContainer(ctx, t, tenantA, collectionID)
	collection.ParentID = targetID
	collection.OrderKey = "a5"
	collection.UpdatedAt = changedAt
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetPlacement(ctx, collection, collection.Version)
	}); err != nil {
		t.Fatalf("moving the collection: %v", err)
	}

	stored := findContainer(ctx, t, tenantA, collectionID)
	if stored.ParentID != targetID || stored.OrderKey != "a5" || stored.Version != 2 {
		t.Errorf("unexpected placement: %+v", stored)
	}
}

// The name check at the destination is the same unique index that decides an insert.
func TestMovingIntoATakenNameIsAConflict(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	_, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	targetID := freshID(t)
	collection := findContainer(ctx, t, tenantA, collectionID)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Insert(ctx, containerIn(tenantA, authorA, targetID, freshName(t), "a1")); err != nil {
			return err
		}
		// A collection at the destination already carrying the name the mover has.
		return repo.Insert(ctx,
			collectionIn(tenantA, authorA, freshID(t), targetID, collection.Name, "a0"))
	}); err != nil {
		t.Fatalf("seeding the destination: %v", err)
	}

	collection.ParentID = targetID
	collection.OrderKey = "a5"
	collection.UpdatedAt = changedAt
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return repo.SetPlacement(ctx, collection, collection.Version)
	})
	if code := shared.AsError(err).DetailCode; code != "containers.name_taken" {
		t.Fatalf("detail code %s, want containers.name_taken (%v)", code, err)
	}
}

// Neighbours reports the two ranks a position sits between, and excludes the moving container from
// its own level - which is what makes a repeated move land on the rank it already has.
func TestNeighboursReportsTheBoundsAndExcludesTheMover(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	hubID := freshID(t)
	first, second, mover := freshID(t), freshID(t), freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := repo.Insert(ctx, containerIn(tenantA, authorA, hubID, freshName(t), "a0")); err != nil {
			return err
		}
		for id, key := range map[shared.ID]string{first: "a1", second: "a3", mover: "a5"} {
			if err := repo.Insert(ctx,
				collectionIn(tenantA, authorA, id, hubID, freshName(t), key)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the level: %v", err)
	}

	var previous, next string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		previous, next, err = repo.Neighbours(ctx, hubID, second, mover)
		return err
	}); err != nil {
		t.Fatalf("reading the bounds: %v", err)
	}
	if previous != "a1" || next != "a3" {
		t.Errorf("bounds (%q, %q), want (a1, a3)", previous, next)
	}

	// Appending: no sibling named, so the previous bound is the last rank at the level with the
	// mover left out - a5 is the mover's own and must not be the answer.
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		previous, next, err = repo.Neighbours(ctx, hubID, "", mover)
		return err
	}); err != nil {
		t.Fatalf("reading the append bounds: %v", err)
	}
	if previous != "a3" || next != "" {
		t.Errorf("bounds (%q, %q), want (a3, \"\") - the mover was counted as its own neighbour",
			previous, next)
	}
}

// The hub level, which is the case F2-04 rests on and the reason the query compares the parent with
// IS NOT DISTINCT FROM rather than with `=`: a null parent has to mean "the hubs" and not "no
// filter at all". A plain `parent_id = NULL` is never true, so this level would come back empty and
// every hub would rank as though it were the only one.
//
// The keys sit in a band of their own because a hub level is the whole tenant, unlike a collection
// level which is one hub: another test's hub is a sibling here, so the assertion is written to hold
// whatever else the tenant contains rather than to require a level nobody else touched.
func TestNeighboursReadsTheHubLevelWhereTheParentIsAbsent(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	below, anchor, mover := freshID(t), freshID(t), freshID(t)

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for id, key := range map[shared.ID]string{below: "z1", anchor: "z3", mover: "z5"} {
			if err := repo.Insert(ctx, containerIn(tenantA, authorA, id, freshName(t), key)); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding the hub level: %v", err)
	}

	var previous, next string
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		// The empty parent is the hub level, which is what the port promises and what this proves.
		previous, next, err = repo.Neighbours(ctx, "", anchor, mover)
		return err
	}); err != nil {
		t.Fatalf("reading the hub bounds: %v", err)
	}
	if previous != "z1" || next != "z3" {
		t.Errorf("bounds (%q, %q), want (z1, z3) - the hub level did not resolve", previous, next)
	}
}

// The cross-tenant negative test for all four writes and for Neighbours (gate SG-3): row level
// security removes the row from the statement's reach, so the write matches nothing and the read
// sees an empty level.
func TestTheLifecycleWritesCannotReachAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	repo := containerRepo()
	hubID, collectionID := hubWithCollection(ctx, t, tenantA, authorA)

	collection := findContainer(ctx, t, tenantA, collectionID)
	collection.UpdatedAt = changedAt

	writes := map[string]func(context.Context) error{
		"attributes": func(ctx context.Context) error {
			renamed := collection
			renamed.Name = freshName(t)
			return repo.SetAttributes(ctx, renamed, collection.Version)
		},
		"policies": func(ctx context.Context) error {
			configured := collection
			configured.CompletionPolicy = work.CompletionRollup
			return repo.SetPolicies(ctx, configured, collection.Version)
		},
		"archived": func(ctx context.Context) error {
			archived := collection
			archived.ArchivedAt = &changedAt
			return repo.SetArchived(ctx, archived, collection.Version)
		},
		"placement": func(ctx context.Context) error {
			moved := collection
			moved.OrderKey = "a9"
			return repo.SetPlacement(ctx, moved, collection.Version)
		},
	}

	for name, attempt := range writes {
		t.Run(name, func(t *testing.T) {
			// The transaction belongs to tenant B; the row belongs to tenant A.
			err := write(ctx, t, tenantB, attempt)
			if !errors.Is(err, shared.ErrVersionConflict) {
				t.Fatalf("tenant B's %s write on tenant A's row answered %v, want a version conflict",
					name, err)
			}
		})
	}

	t.Run("neighbours", func(t *testing.T) {
		var previous, next string
		if err := read(ctx, t, tenantB, func(ctx context.Context) error {
			var err error
			previous, next, err = repo.Neighbours(ctx, hubID, "", freshID(t))
			return err
		}); err != nil {
			t.Fatalf("reading another tenant's level: %v", err)
		}
		if previous != "" || next != "" {
			t.Errorf("tenant B read tenant A's ranks: (%q, %q)", previous, next)
		}
	})

	// Nothing moved, which is the other half of the claim: the writes were refused rather than
	// applied somewhere else.
	if stored := findContainer(ctx, t, tenantA, collectionID); stored.Version != 1 {
		t.Errorf("a write from tenant B moved tenant A's row: %+v", stored)
	}
}
