// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"strconv"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// The read side's two list methods against a real database (B-04).
//
// The pagination tests are the point of this file. A keyset walk is correct or it is not, and the way
// it is wrong - one row visited twice, or one never visited at all - is invisible in a unit test
// against a fake that returns whatever it was handed.
//
// Every test here works inside a level of its own: a hub it just created, or a collection it just
// created. The package shares one database, so a test that listed the hubs would be listing every
// other test's fixtures too, and could then only assert "mine are in there somewhere". Owning the
// level is what lets these assert the page exactly - which is the only way a duplicate or a missing
// row is visible at all.

// hubFor creates one empty hub, to be used as a level of collections.
func hubFor(ctx context.Context, t *testing.T, tenant, author shared.ID) shared.ID {
	t.Helper()
	seedContainerTenants(ctx, t)

	id := freshID(t)
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		return containerRepo().Insert(ctx, containerIn(tenant, author, id, freshName(t), "a0"))
	}); err != nil {
		t.Fatalf("seeding the hub: %v", err)
	}
	return id
}

// collectionsIn writes n collections into a hub, ranked the way the domain ranks them, and returns
// their identifiers in that order. Through OrderKeyAfter rather than with keys written out here, so
// that the fixture and production agree about what "next" is.
func collectionsIn(ctx context.Context, t *testing.T, tenant, author, hub shared.ID, n int) []shared.ID {
	t.Helper()

	containers := containerRepo()
	ids := make([]shared.ID, 0, n)
	previous := ""

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		for range n {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			id := freshID(t)
			collection := containerIn(tenant, author, id, freshName(t), key)
			collection.Type = work.ContainerCollection
			collection.ParentID = hub
			if err := containers.Insert(ctx, collection); err != nil {
				return err
			}
			ids, previous = append(ids, id), key
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %d collections: %v", n, err)
	}
	return ids
}

// tasksIn does the same for the items directly in a collection.
func tasksIn(ctx context.Context, t *testing.T, tenant, author, collection shared.ID, n int) []shared.ID {
	t.Helper()

	items := itemRepo()
	ids := make([]shared.ID, 0, n)
	previous := ""

	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		for range n {
			key, err := service.OrderKeyAfter(previous)
			if err != nil {
				return err
			}
			id := freshID(t)
			if err := items.Insert(ctx, taskIn(tenant, author, collection, id, freshName(t), key)); err != nil {
				return err
			}
			ids, previous = append(ids, id), key
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding %d tasks: %v", n, err)
	}
	return ids
}

// containerPage reads one page of a level.
func containerPage(
	ctx context.Context, t *testing.T, tenant shared.ID, query repository.ContainerQuery,
) repository.ContainerPage {
	t.Helper()

	var page repository.ContainerPage
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		page, err = containerRepo().List(ctx, query)
		return err
	}); err != nil {
		t.Fatalf("listing the containers: %v", err)
	}
	return page
}

// walkContainers pages a level to the end and returns what it saw, plus how many pages it took.
func walkContainers(
	ctx context.Context, t *testing.T, tenant shared.ID, query repository.ContainerQuery,
) ([]shared.ID, int) {
	t.Helper()

	var seen []shared.ID
	for pages := 1; ; pages++ {
		page := containerPage(ctx, t, tenant, query)
		seen = append(seen, idsOf(page.Containers)...)

		if !page.Info.HasMore {
			return seen, pages
		}
		if page.Info.NextCursor == "" {
			t.Fatal("a page reported more rows and carried no cursor to reach them")
		}
		query.Page.Cursor = page.Info.NextCursor
		if pages > 100 {
			t.Fatal("the walk did not terminate")
		}
	}
}

// walkItems is the same for one level of items.
func walkItems(ctx context.Context, t *testing.T, tenant shared.ID, query repository.ItemQuery) []shared.ID {
	t.Helper()

	repo := itemRepo()
	var seen []shared.ID

	for walked := 0; ; walked++ {
		var page repository.ItemPage
		if err := read(ctx, t, tenant, func(ctx context.Context) error {
			var err error
			page, err = repo.List(ctx, query)
			return err
		}); err != nil {
			t.Fatalf("listing the items: %v", err)
		}

		for _, item := range page.Items {
			seen = append(seen, item.ID)
		}
		if !page.Info.HasMore {
			return seen
		}
		query.Page.Cursor = page.Info.NextCursor
		if walked > 100 {
			t.Fatal("the walk did not terminate")
		}
	}
}

func TestAListReturnsOneLevelInItsRankOrder(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	written := collectionsIn(ctx, t, tenantA, authorA, hub, 4)

	seen, pages := walkContainers(ctx, t, tenantA, repository.ContainerQuery{
		ParentID: hub, Page: repository.Page{Size: 50},
	})
	if pages != 1 {
		t.Errorf("four collections took %d pages at a size of 50", pages)
	}
	assertExactly(t, seen, written)
}

// The hub level is reached by naming no parent at all, which is a different SQL path - IS NOT
// DISTINCT FROM against NULL rather than against a value. The hub written here has to be in it.
func TestNamingNoParentListsTheHubs(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)

	seen, _ := walkContainers(ctx, t, tenantA, repository.ContainerQuery{
		Page: repository.Page{Size: 200},
	})
	assertPresent(t, seen, hub, "the hub just written")
}

// The acceptance criterion of B-04: no skipped and no repeated rows. Walked at every page size from
// one row upwards, because one row at a time is what turns an off-by-one at the boundary into a wrong
// answer rather than a lucky one.
func TestPaginationVisitsEveryRowExactlyOnce(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	written := collectionsIn(ctx, t, tenantA, authorA, hub, 7)

	for _, size := range []int{1, 2, 3, 6, 7, 8, 50} {
		t.Run(sizeName(size), func(t *testing.T) {
			seen, pages := walkContainers(ctx, t, tenantA, repository.ContainerQuery{
				ParentID: hub, Page: repository.Page{Size: size},
			})
			assertExactly(t, seen, written)

			// A page that is exactly full and has no successor must still end the walk, rather than
			// costing one more request that returns nothing. size=7 over seven rows is that case.
			if wanted := (len(written) + size - 1) / size; pages != wanted {
				t.Errorf("the walk took %d pages, want %d", pages, wanted)
			}
		})
	}
}

// The other half of the criterion: a row inserted while a client is halfway through the walk must
// not disturb the rows it has not reached. Inserted *behind* the cursor, which is the case an offset
// gets wrong - with an offset every later row shifts one place and one is skipped entirely.
func TestAConcurrentInsertDoesNotDisturbTheRestOfTheWalk(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	written := collectionsIn(ctx, t, tenantA, authorA, hub, 4)

	query := repository.ContainerQuery{ParentID: hub, Page: repository.Page{Size: 2}}
	first := containerPage(ctx, t, tenantA, query)
	if len(first.Containers) != 2 || !first.Info.HasMore {
		t.Fatalf("the first page holds %d rows, has_more=%v", len(first.Containers), first.Info.HasMore)
	}

	// A rank below everything already there, so the new row sorts ahead of the cursor.
	key, err := service.OrderKeyBetween("", first.Containers[0].OrderKey)
	if err != nil {
		t.Fatalf("ranking the intruder: %v", err)
	}
	intruder := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		collection := containerIn(tenantA, authorA, intruder, freshName(t), key)
		collection.Type = work.ContainerCollection
		collection.ParentID = hub
		return containerRepo().Insert(ctx, collection)
	}); err != nil {
		t.Fatalf("inserting during the walk: %v", err)
	}

	query.Page.Cursor = first.Info.NextCursor
	rest, _ := walkContainers(ctx, t, tenantA, query)

	seen := append(idsOf(first.Containers), rest...)
	assertExactly(t, seen, written)
	assertAbsent(t, seen, intruder, "a row inserted behind the cursor")
}

// Gate SG-3 for ContainerRepository.List. The tenant boundary is row level security underneath, so
// the only way to know it applies to a new query is to reach across it - and the interesting form of
// the question is not "does B see its own rows" but "does naming A's hub reach A's rows".
func TestAContainerListIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	collectionsIn(ctx, t, tenantA, authorA, hub, 3)

	page := containerPage(ctx, t, tenantB, repository.ContainerQuery{
		ParentID: hub, Page: repository.Page{Size: 50},
	})
	if len(page.Containers) != 0 {
		t.Errorf("naming another tenant's hub returned %d collections", len(page.Containers))
	}
}

// And for ItemRepository.List, separately: the two tables carry separate policies, and one being
// right says nothing about the other.
func TestAnItemListIsInvisibleFromAnotherTenant(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	tasksIn(ctx, t, tenantA, authorA, collection, 3)

	var page repository.ItemPage
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		var err error
		page, err = itemRepo().List(ctx, repository.ItemQuery{
			CollectionID: collection,
			Page:         repository.Page{Size: 50},
		})
		return err
	}); err != nil {
		t.Fatalf("listing as tenant B: %v", err)
	}
	if len(page.Items) != 0 {
		t.Errorf("naming another tenant's collection returned %d items", len(page.Items))
	}
}

func TestAnItemListReturnsTheLevelAsked(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	tasks := tasksIn(ctx, t, tenantA, authorA, collection, 3)

	// A work package under the first task. The level of tasks must not contain it, and the level of
	// that task's children must contain nothing else.
	child := freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		item := taskIn(tenantA, authorA, collection, child, "A work package", "a0")
		item.Type = work.ItemWorkPackage
		item.ParentID = tasks[0]
		item.Path = work.RootPath(tasks[0]) + child.String() + work.PathSeparator
		item.Depth = 2
		return itemRepo().Insert(ctx, item)
	}); err != nil {
		t.Fatalf("seeding the work package: %v", err)
	}

	t.Run("the items directly in the collection", func(t *testing.T) {
		seen := walkItems(ctx, t, tenantA, repository.ItemQuery{
			CollectionID: collection, Page: repository.Page{Size: 50},
		})
		assertExactly(t, seen, tasks)
	})

	t.Run("the children of one item", func(t *testing.T) {
		seen := walkItems(ctx, t, tenantA, repository.ItemQuery{
			CollectionID: collection, ParentID: tasks[0], Page: repository.Page{Size: 50},
		})
		assertExactly(t, seen, []shared.ID{child})
	})
}

// The type filter composes with the level rather than replacing it, so the two impossible
// combinations return nothing at all. Worth a test because "returns an empty page" is also what a
// broken filter does, and the third case here is what tells them apart.
func TestTheTypeFilterComposesWithTheLevel(t *testing.T) {
	ctx := context.Background()
	hub := hubFor(ctx, t, tenantA, authorA)
	written := collectionsIn(ctx, t, tenantA, authorA, hub, 2)

	t.Run("the collections of a hub", func(t *testing.T) {
		seen, _ := walkContainers(ctx, t, tenantA, repository.ContainerQuery{
			ParentID: hub, Type: work.ContainerCollection, Page: repository.Page{Size: 50},
		})
		assertExactly(t, seen, written)
	})

	t.Run("hubs inside a hub, of which there are none", func(t *testing.T) {
		page := containerPage(ctx, t, tenantA, repository.ContainerQuery{
			ParentID: hub, Type: work.ContainerHub, Page: repository.Page{Size: 50},
		})
		if len(page.Containers) != 0 {
			t.Errorf("a hub inside a hub returned %d rows", len(page.Containers))
		}
	})
}

// A trashed row is not part of a level, and an archived one only when the caller asks. The two are
// deliberately different: the trash is its own view (B-10), an archive is still the workspace.
func TestTheLifecycleStateDecidesWhetherARowIsInTheLevel(t *testing.T) {
	ctx := context.Background()
	collection := collectionFor(ctx, t, tenantA, authorA)
	visible := tasksIn(ctx, t, tenantA, authorA, collection, 1)[0]

	archived, trashed := freshID(t), freshID(t)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		items := itemRepo()
		if err := items.Insert(ctx, taskIn(tenantA, authorA, collection, archived, "Archived", "b0")); err != nil {
			return err
		}
		return items.Insert(ctx, taskIn(tenantA, authorA, collection, trashed, "Trashed", "c0"))
	}); err != nil {
		t.Fatalf("seeding the lifecycle fixtures: %v", err)
	}

	// Stamped through the superuser rather than through the repository. InsertWorkItem writes neither
	// column - archiving and trashing are use cases of their own (B-06, B-10) - so a fixture that set
	// the field on the struct would be silently dropped, and this test would then be asserting the
	// absence of rows that were never in the state it claims.
	stamp(ctx, t, archived, "archived_at")
	stamp(ctx, t, trashed, "deleted_at")

	t.Run("by default neither is there", func(t *testing.T) {
		seen := walkItems(ctx, t, tenantA, repository.ItemQuery{
			CollectionID: collection, Page: repository.Page{Size: 50},
		})
		assertExactly(t, seen, []shared.ID{visible})
	})

	t.Run("include_archived reaches the archived one and not the trashed one", func(t *testing.T) {
		seen := walkItems(ctx, t, tenantA, repository.ItemQuery{
			CollectionID: collection, IncludeArchived: true, Page: repository.Page{Size: 50},
		})
		assertPresent(t, seen, archived, "an archived item")
		assertAbsent(t, seen, trashed, "a trashed item")
	})
}

// A cursor this installation did not issue is refused rather than ignored. Ignoring it would restart
// the walk silently, which for a client paging a long list is a loop it cannot see.
func TestAForgedCursorIsRefused(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := containerRepo().List(ctx, repository.ContainerQuery{
			Page: repository.Page{Cursor: "not-a-cursor-this-server-issued", Size: 10},
		})
		return err
	})
	if !errors.Is(err, shared.ErrValidation) {
		t.Errorf("a forged cursor was answered with %v, want a validation error", err)
	}
}

func idsOf(containers []work.Container) []shared.ID {
	ids := make([]shared.ID, 0, len(containers))
	for _, container := range containers {
		ids = append(ids, container.ID)
	}
	return ids
}

// assertExactly compares a walk with the level it walked: the same identifiers, in the same order,
// and nothing else. Exact rather than "contains", which is what each test owning its own level buys -
// a duplicated or a missing row is only visible against an exact expectation.
func assertExactly(t *testing.T, seen, wanted []shared.ID) {
	t.Helper()

	if len(seen) != len(wanted) {
		t.Fatalf("the walk saw %d rows, want %d: %v against %v", len(seen), len(wanted), seen, wanted)
	}
	for i := range wanted {
		if seen[i] != wanted[i] {
			t.Fatalf("row %d of the walk is %s, want %s: %v against %v", i, seen[i], wanted[i], seen, wanted)
		}
	}
}

func assertPresent(t *testing.T, seen []shared.ID, wanted shared.ID, what string) {
	t.Helper()
	for _, id := range seen {
		if id == wanted {
			return
		}
	}
	t.Errorf("%s (%s) is missing from the level", what, wanted)
}

func assertAbsent(t *testing.T, seen []shared.ID, unwanted shared.ID, what string) {
	t.Helper()
	for _, id := range seen {
		if id == unwanted {
			t.Errorf("%s (%s) is in the level and should not be", what, unwanted)
		}
	}
}

func sizeName(size int) string {
	if size == 1 {
		return "one row at a time"
	}
	return "pages of " + strconv.Itoa(size)
}

// stamp sets a lifecycle column the repository does not write, and insists the row was there to be
// stamped: an UPDATE matching nothing would leave a test asserting the absence of a row that was
// never in the state it claims.
func stamp(ctx context.Context, t *testing.T, id shared.ID, column string) {
	t.Helper()

	// The column name is a constant of this test and never a value from anywhere else, which is what
	// keeps CLAUDE.md rule 9 intact here, where sqlc cannot express "either of two columns".
	switch column {
	case "archived_at", "deleted_at":
	default:
		t.Fatalf("stamp was asked for the column %q, which it does not know", column)
	}

	tag, err := adminPool(ctx, t).Exec(ctx,
		"UPDATE work_item SET "+column+" = $2 WHERE id = $1", id.String(), created)
	if err != nil {
		t.Fatalf("stamping %s: %v", column, err)
	}
	if tag.RowsAffected() != 1 {
		t.Fatalf("stamping %s matched %d rows, want 1", column, tag.RowsAffected())
	}
}
