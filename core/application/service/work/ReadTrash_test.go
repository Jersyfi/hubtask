// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// trashStore answers what is in the trash, and records what it was asked.
type trashStore struct {
	page       repository.TrashPage
	asked      repository.Page
	listErr    error
	subtree    []shared.ID
	purgedItem []shared.ID
	purgedCont []shared.ID
}

func (s *trashStore) List(_ context.Context, page repository.Page) (repository.TrashPage, error) {
	s.asked = page
	if s.listErr != nil {
		return repository.TrashPage{}, s.listErr
	}
	return s.page, nil
}

func (s *trashStore) SubtreeIDs(context.Context, string) ([]shared.ID, error) {
	return s.subtree, nil
}

func (s *trashStore) PurgeItems(_ context.Context, ids []shared.ID) (int, error) {
	s.purgedItem = append(s.purgedItem, ids...)
	return len(ids), nil
}

func (s *trashStore) PurgeContainers(_ context.Context, ids []shared.ID) (int, error) {
	s.purgedCont = append(s.purgedCont, ids...)
	return len(ids), nil
}

var deletedEarlier = now.Add(-time.Hour)

func trashEntry(kind domain.TrashKind, id, hub, collection shared.ID, subtype string) domain.TrashEntry {
	return domain.TrashEntry{
		Kind: kind, ID: id, BatchID: shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		DeletedAt: deletedEarlier, Title: "Something", Subtype: subtype,
		HubID: hub, CollectionID: collection, Version: 2,
	}
}

func newTrashReader(entries ...domain.TrashEntry) (ListTrash, *trashStore, *reader) {
	store := &trashStore{page: repository.TrashPage{Entries: entries}}
	permitted := &reader{}
	return ListTrash{Trash: store, Reader: permitted, UnitOfWork: &unitOfWork{}}, store, permitted
}

// The read side of the trash, and the two things it owes: the page as stored, and a read-only
// transaction - a read that opened a writable one would pin every list in the product to the primary
// (multi-tenancy.md §7).
func TestListingTheTrashReadsWithoutWriting(t *testing.T) {
	handler, store, _ := newTrashReader(
		trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"),
	)
	uow := handler.UnitOfWork.(*unitOfWork)

	page, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{Size: 25})
	if err != nil {
		t.Fatalf("reading the trash was refused: %v", err)
	}

	if len(page.Entries) != 1 {
		t.Errorf("%d entries, want 1", len(page.Entries))
	}
	if store.asked.Size != 25 {
		t.Errorf("the repository was asked for %d rows, want 25", store.asked.Size)
	}
	if uow.writes != 0 || uow.reads != 1 {
		t.Errorf("%d writes and %d reads, want none and one", uow.writes, uow.reads)
	}
}

// A size nobody named is the default, and one beyond the ceiling is clamped rather than refused: a
// client asking for 500 rows wants as many as it can have (api-guidelines.md §4).
func TestTheTrashPageSizeIsClamped(t *testing.T) {
	for _, c := range []struct{ asked, want int }{
		{0, DefaultPageSize}, {-1, DefaultPageSize}, {500, MaxPageSize}, {25, 25},
	} {
		handler, store, _ := newTrashReader()
		if _, err := handler.Execute(
			t.Context(), itemActor(), ListTrashQuery{Size: c.asked}); err != nil {
			t.Fatalf("reading the trash was refused: %v", err)
		}
		if store.asked.Size != c.want {
			t.Errorf("a size of %d reached the repository as %d, want %d", c.asked, store.asked.Size, c.want)
		}
	}
}

// The trash spans hubs, so it is narrowed to what the actor may see rather than refused. "What did I
// delete" is answered with what they may see, not with a 403.
func TestTheTrashIsNarrowedToWhatTheActorMaySee(t *testing.T) {
	otherHub := shared.MustParseID("0192f000-0000-7000-8000-00000000001b")
	otherCollection := shared.MustParseID("0192f000-0000-7000-8000-00000000001c")

	handler, _, permitted := newTrashReader(
		trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"),
		trashEntry(domain.TrashItemKind, packageID, otherHub, otherCollection, "WORK_PACKAGE"),
	)
	// The double keys on the last scope of each path, which is the collection for an entry.
	permitted.permit = map[shared.ID]bool{collectionID: true}

	page, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{})
	if err != nil {
		t.Fatalf("reading the trash was refused: %v", err)
	}

	if len(page.Entries) != 1 || page.Entries[0].ID != taskID {
		t.Errorf("the page holds %v, want only the entry in the readable hub", page.Entries)
	}
}

// The cursor is a boundary in what was *scanned*, not in what came back. Narrowing it to the last
// visible row would skip everything between it and the last row actually read - the walk would then
// lose entries silently rather than showing shorter pages.
func TestNarrowingLeavesTheCursorAlone(t *testing.T) {
	handler, store, permitted := newTrashReader(
		trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"),
	)
	store.page.Info = repository.PageInfo{NextCursor: "opaque", HasMore: true}
	permitted.permit = map[shared.ID]bool{}

	page, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{})
	if err != nil {
		t.Fatalf("reading the trash was refused: %v", err)
	}
	if len(page.Entries) != 0 {
		t.Errorf("%d entries survived a permit-nothing reader", len(page.Entries))
	}
	if page.Info.NextCursor != "opaque" || !page.Info.HasMore {
		t.Errorf("the walk's state was narrowed with the page: %+v", page.Info)
	}
}

// The path each entry is judged against. A membership held at a hub applies downwards, so an entry
// that named only its collection could not be shown to somebody whose right sits above it - which is
// what the hub on the projection is for.
func TestTheTrashEntryIsJudgedAgainstItsWholePath(t *testing.T) {
	for _, c := range []struct {
		name  string
		entry domain.TrashEntry
		want  []identity.ScopeType
	}{
		{
			"a hub is its own level",
			trashEntry(domain.TrashContainerKind, hubID, "", "", "HUB"),
			[]identity.ScopeType{identity.ScopeTenant, identity.ScopeHub},
		},
		{
			"a collection sits in its hub",
			trashEntry(domain.TrashContainerKind, collectionID, hubID, "", "COLLECTION"),
			[]identity.ScopeType{identity.ScopeTenant, identity.ScopeHub, identity.ScopeCollection},
		},
		{
			"an entry sits in its collection, in its hub",
			trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"),
			[]identity.ScopeType{identity.ScopeTenant, identity.ScopeHub, identity.ScopeCollection},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			handler, _, permitted := newTrashReader(c.entry)

			if _, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{}); err != nil {
				t.Fatalf("reading the trash was refused: %v", err)
			}
			if len(permitted.asked) != 1 {
				t.Fatalf("%d paths were judged, want 1", len(permitted.asked))
			}

			path := permitted.asked[0]
			if len(path) != len(c.want) {
				t.Fatalf("the path is %v, want %v levels", path, len(c.want))
			}
			for i, want := range c.want {
				if path[i].Type != want {
					t.Errorf("level %d is a %s, want a %s", i, path[i].Type, want)
				}
			}
		})
	}
}

// An empty page asks nothing of the authorisation service: there is nothing to narrow, and a call
// per empty read is a round trip for an answer nobody needs.
func TestAnEmptyTrashAsksNoPermissionQuestion(t *testing.T) {
	handler, _, permitted := newTrashReader()

	if _, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{}); err != nil {
		t.Fatalf("reading an empty trash was refused: %v", err)
	}
	if permitted.asked != nil {
		t.Errorf("an empty page was judged: %v", permitted.asked)
	}
}

func TestAFailingReaderFailsTheRead(t *testing.T) {
	handler, _, permitted := newTrashReader(
		trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"),
	)
	permitted.err = errors.New("the authorisation service is unreachable")

	if _, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{}); err == nil {
		t.Error("a failing reader produced a page")
	}
}

// The catalogue entry, and the projection it renders: the optional identifiers travel as explicit
// nulls, because a field that appeared only for some kinds is one a client cannot read
// unconditionally - and this list mixes the two kinds by design.
func TestTheTrashProjectionCarriesEveryFieldEvenWhenEmpty(t *testing.T) {
	handler, _, _ := newTrashReader(trashEntry(domain.TrashContainerKind, hubID, "", "", "HUB"))

	out, err := handler.Descriptor().Handler.Invoke(t.Context(), itemActor(), nil)
	if err != nil {
		t.Fatalf("invoking through the catalogue: %v", err)
	}

	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("the answer holds %d rows, want 1", len(rows))
	}
	for _, field := range []string{
		"kind", "id", "trash_batch_id", "deleted_at", "title", "subtype",
		"hub_id", "collection_id", "parent_id", "version",
	} {
		if _, present := rows[0][field]; !present {
			t.Errorf("the projection omits %s", field)
		}
	}
	if rows[0]["hub_id"] != nil {
		t.Errorf("a hub carries a hub of its own: %v", rows[0]["hub_id"])
	}
	if !handler.Descriptor().ReadOnly {
		t.Error("ListTrash is not declared read-only")
	}
}
