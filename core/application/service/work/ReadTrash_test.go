// SPDX-License-Identifier: Apache-2.0
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
	"github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
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
	// members are the rows of the deletions asked for; askedBatches what was asked.
	members      []repository.TrashMember
	askedBatches []shared.ID
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
func (s *trashStore) KeptBelow(context.Context, []shared.ID, []shared.ID) (map[shared.ID]int, error) {
	return map[shared.ID]int{}, nil
}

func (s *trashStore) BatchMembers(_ context.Context, batches []shared.ID) ([]repository.TrashMember, error) {
	s.askedBatches = append(s.askedBatches, batches...)
	return s.members, nil
}

var deletedEarlier = now.Add(-time.Hour)

// trashHolds answers the holds in force, and how often it was asked.
type trashHolds struct {
	holds []lifecycle.LegalHold
	asked int
}

func (h *trashHolds) List(_ context.Context, includeReleased bool) ([]lifecycle.LegalHold, error) {
	h.asked++
	if includeReleased {
		return nil, errors.New("the trash asked for released holds too")
	}
	return h.holds, nil
}

// trashContributors answers whose data each entry carries, and what it was asked.
type trashContributors struct {
	by    map[shared.ID][]shared.ID
	asked []shared.ID
}

func (c *trashContributors) Contributors(_ context.Context, ids []shared.ID) (map[shared.ID][]shared.ID, error) {
	c.asked = append(c.asked, ids...)
	return c.by, nil
}

// trashPolicies answers the trash period the retention pass reads, or a failure.
type trashPolicies struct {
	policy lifecycle.Policy
	err    error
}

func (p trashPolicies) Ensure(context.Context, []lifecycle.Policy) error { return nil }

func (p trashPolicies) Find(_ context.Context, kind lifecycle.DataKind) (lifecycle.Policy, error) {
	if kind != lifecycle.KindTrash {
		return lifecycle.Policy{}, shared.ErrNotFound
	}
	return p.policy, p.err
}

const trashDay = 24 * time.Hour

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
	return ListTrash{
		Trash: store, Reader: permitted, UnitOfWork: &unitOfWork{},
		Policies:        trashPolicies{policy: lifecycle.Policy{DataKind: lifecycle.KindTrash, RetainDays: 30}},
		TombstoneWindow: 90 * trashDay,
		Holds:           &trashHolds{}, Contributors: &trashContributors{},
	}, store, permitted
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
	// Two reads: the page, and - after the narrowing, so nothing of a hidden deletion is read -
	// the legal holds that may keep what is left.
	if uow.writes != 0 || uow.reads != 2 {
		t.Errorf("%d writes and %d reads, want none and two", uow.writes, uow.reads)
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
		"kind", "id", "trash_batch_id", "deleted_at", "purge_after", "title", "subtype",
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

// Each entry says when the automatic pass may take it, by the pass's own two numbers: the trash
// period it reads and the offline window, whichever ends later (UC-LIF-02 check 6). A tenant the pass
// has not reached yet counts with the documented default, and a failed read fails the list rather
// than guessing a date.
func TestEachTrashEntrySaysWhenTheAutomaticPassMayTakeIt(t *testing.T) {
	trash := func(days int) trashPolicies {
		return trashPolicies{policy: lifecycle.Policy{DataKind: lifecycle.KindTrash, RetainDays: days}}
	}
	for _, c := range []struct {
		name     string
		policies trashPolicies
		window   time.Duration
		want     time.Time
	}{
		{"the window outlasts the period", trash(30), 90 * trashDay, deletedEarlier.Add(90 * trashDay)},
		{"the period outlasts the window", trash(120), 90 * trashDay, deletedEarlier.AddDate(0, 0, 120)},
		{"no period stored yet", trashPolicies{err: shared.ErrNotFound}, 0, deletedEarlier.AddDate(0, 0, 30)},
	} {
		t.Run(c.name, func(t *testing.T) {
			handler, _, _ := newTrashReader(trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"))
			handler.Policies, handler.TombstoneWindow = c.policies, c.window

			out, err := handler.Descriptor().Handler.Invoke(t.Context(), itemActor(), nil)
			if err != nil {
				t.Fatalf("invoking through the catalogue: %v", err)
			}
			rows, _ := out["data"].([]usecase.Output)
			if len(rows) != 1 {
				t.Fatalf("the answer holds %d rows, want 1", len(rows))
			}
			if got, _ := rows[0]["purge_after"].(time.Time); !got.Equal(c.want) {
				t.Errorf("purge_after is %v, want %v", rows[0]["purge_after"], c.want)
			}
		})
	}

	handler, _, _ := newTrashReader(trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK"))
	handler.Policies = trashPolicies{err: errors.New("the database went away")}
	if _, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{}); err == nil {
		t.Error("a failed read of the trash period answered a list with guessed dates")
	}
}

// A deletion a legal hold keeps says so (UC-LIF-02 check 6): its rows are judged as the retention
// pass judges them, and one held row keeps the whole deletion, because the pass removes a row only
// with everything below it. The cases are the scopes a hold can name, against a task deleted with
// its work package and activity, and a collection deleted with an entry in it.
func TestATrashEntrySaysWhenALegalHoldKeepsIt(t *testing.T) {
	var (
		taskBatch  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
		collBatch  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
		pkgID      = shared.MustParseID("0192f000-0000-7000-8000-0000000004a2")
		activityID = shared.MustParseID("0192f000-0000-7000-8000-0000000004a3")
		parentTask = shared.MustParseID("0192f000-0000-7000-8000-0000000004a0")
		trashedCol = shared.MustParseID("0192f000-0000-7000-8000-0000000004c1")
		inColl     = shared.MustParseID("0192f000-0000-7000-8000-0000000004c2")
		author     = shared.MustParseID("0192f000-0000-7000-8000-0000000004d1")
		stranger   = shared.MustParseID("0192f000-0000-7000-8000-0000000004d2")
		elsewhere  = shared.MustParseID("0192f000-0000-7000-8000-0000000004e1")
	)
	taskPath := "/" + parentTask.String() + "/" + taskID.String() + "/"
	members := []repository.TrashMember{
		{BatchID: taskBatch, Kind: domain.TrashItemKind, ID: taskID, HubID: hubID, CollectionID: collectionID, Path: taskPath},
		{BatchID: taskBatch, Kind: domain.TrashItemKind, ID: pkgID, HubID: hubID, CollectionID: collectionID, Path: taskPath + pkgID.String() + "/"},
		{BatchID: taskBatch, Kind: domain.TrashItemKind, ID: activityID, HubID: hubID, CollectionID: collectionID, Path: taskPath + pkgID.String() + "/" + activityID.String() + "/"},
		{BatchID: collBatch, Kind: domain.TrashContainerKind, ID: trashedCol, HubID: hubID},
		{BatchID: collBatch, Kind: domain.TrashItemKind, ID: inColl, HubID: hubID, CollectionID: trashedCol, Path: "/" + inColl.String() + "/"},
	}
	hold := func(scope lifecycle.HoldScope, id shared.ID) lifecycle.LegalHold {
		return lifecycle.LegalHold{ID: shared.MustParseID("0192f000-0000-7000-8000-0000000004f1"), Scope: scope, ScopeID: id}
	}

	for _, c := range []struct {
		name          string
		holds         []lifecycle.LegalHold
		task, coll    bool
		asksWhoseData bool
	}{
		{name: "no hold"},
		{name: "the workspace", holds: []lifecycle.LegalHold{hold(lifecycle.HoldTenant, shared.ID(""))}, task: true, coll: true},
		{name: "the hub", holds: []lifecycle.LegalHold{hold(lifecycle.HoldContainer, hubID)}, task: true, coll: true},
		{name: "the task's collection", holds: []lifecycle.LegalHold{hold(lifecycle.HoldContainer, collectionID)}, task: true},
		{name: "the deleted collection itself", holds: []lifecycle.LegalHold{hold(lifecycle.HoldContainer, trashedCol)}, coll: true},
		{name: "the deleted task", holds: []lifecycle.LegalHold{hold(lifecycle.HoldItem, taskID)}, task: true},
		{name: "the task above the deleted one", holds: []lifecycle.LegalHold{hold(lifecycle.HoldItem, parentTask)}, task: true},
		{name: "an activity deleted with it", holds: []lifecycle.LegalHold{hold(lifecycle.HoldItem, activityID)}, task: true},
		{name: "somebody whose data is in it", holds: []lifecycle.LegalHold{hold(lifecycle.HoldAccount, author)}, coll: true, asksWhoseData: true},
		{name: "somebody whose data is not in it", holds: []lifecycle.LegalHold{hold(lifecycle.HoldAccount, stranger)}, asksWhoseData: true},
		{name: "something else entirely", holds: []lifecycle.LegalHold{hold(lifecycle.HoldItem, elsewhere), hold(lifecycle.HoldContainer, elsewhere)}},
	} {
		t.Run(c.name, func(t *testing.T) {
			task := trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK")
			task.BatchID = taskBatch
			coll := trashEntry(domain.TrashContainerKind, trashedCol, hubID, shared.ID(""), "COLLECTION")
			coll.BatchID = collBatch
			handler, store, _ := newTrashReader(task, coll)
			store.members = members
			holds := &trashHolds{holds: c.holds}
			whose := &trashContributors{by: map[shared.ID][]shared.ID{inColl: {author}}}
			handler.Holds, handler.Contributors = holds, whose

			out, err := handler.Descriptor().Handler.Invoke(t.Context(), itemActor(), nil)
			if err != nil {
				t.Fatalf("invoking through the catalogue: %v", err)
			}
			rows, _ := out["data"].([]usecase.Output)
			if len(rows) != 2 {
				t.Fatalf("the answer holds %d rows, want 2", len(rows))
			}
			for _, want := range []struct {
				row  usecase.Output
				held bool
				what string
			}{{rows[0], c.task, "the task"}, {rows[1], c.coll, "the collection"}} {
				got, present := want.row["held"].(bool)
				if !present {
					t.Fatalf("%s carries no held field: %v", want.what, want.row)
				}
				if got != want.held {
					t.Errorf("%s: held is %v, want %v", want.what, got, want.held)
				}
			}
			if len(c.holds) == 0 && len(store.askedBatches) != 0 {
				t.Error("the deletions' rows were read while no hold is in force")
			}
			if (len(whose.asked) > 0) != c.asksWhoseData {
				t.Errorf("whose data the entries carry was asked: %v, want %v", len(whose.asked) > 0, c.asksWhoseData)
			}
		})
	}
}

// Only what the reader may see is judged: a deletion narrowed away is not read on their behalf.
func TestOnlyTheVisibleDeletionsAreJudged(t *testing.T) {
	hidden := trashEntry(domain.TrashItemKind, taskID, hubID, collectionID, "TASK")
	handler, store, permitted := newTrashReader(hidden)
	permitted.permit = map[shared.ID]bool{}
	handler.Holds = &trashHolds{holds: []lifecycle.LegalHold{{Scope: lifecycle.HoldTenant}}}

	if _, err := handler.Execute(t.Context(), itemActor(), ListTrashQuery{}); err != nil {
		t.Fatalf("reading the trash was refused: %v", err)
	}
	if len(store.askedBatches) != 0 {
		t.Errorf("the rows of %d hidden deletions were read", len(store.askedBatches))
	}
}
