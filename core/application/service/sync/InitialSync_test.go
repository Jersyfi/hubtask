// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// snapshotStore is the current state, in memory, paged the way the port pages it: by identifier,
// after a key. Every kind is sorted at construction so that a page after a key is deterministic.
type snapshotStore struct {
	containers  []work.Container
	buckets     []work.Bucket
	labels      []work.Label
	items       []work.WorkItem
	elements    []repository.ItemSetElement
	comments    []repository.InCollection[work.Comment]
	reminders   []repository.InCollection[work.Reminder]
	recurrences []repository.InCollection[work.RecurrenceRule]
	templates   []work.Template
	// reads counts the pages asked for, so a test can say how much of the store a walk touched.
	reads int
}

func pageAfter[T any](rows []T, idOf func(T) string, after string, batch int) []T {
	var out []T
	for _, row := range rows {
		if idOf(row) > after {
			out = append(out, row)
		}
		if len(out) == batch {
			break
		}
	}
	return out
}

func (s *snapshotStore) Containers(_ context.Context, after shared.ID, batch int) ([]work.Container, error) {
	s.reads++
	return pageAfter(s.containers, func(c work.Container) string { return c.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Buckets(_ context.Context, after shared.ID, batch int) ([]work.Bucket, error) {
	s.reads++
	return pageAfter(s.buckets, func(b work.Bucket) string { return b.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Labels(_ context.Context, after shared.ID, batch int) ([]work.Label, error) {
	s.reads++
	return pageAfter(s.labels, func(l work.Label) string { return l.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) Items(_ context.Context, after shared.ID, batch int) ([]work.WorkItem, error) {
	s.reads++
	return pageAfter(s.items, func(i work.WorkItem) string { return i.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) SetElements(
	_ context.Context, after repository.SetElementKey, batch int,
) ([]repository.ItemSetElement, error) {
	s.reads++
	keyOf := func(e repository.ItemSetElement) string {
		return e.ItemID.String() + "|" + string(e.Set) + "|" + e.Element.ElementID.String()
	}
	afterKey := ""
	if !after.ItemID.IsZero() {
		afterKey = after.ItemID.String() + "|" + string(after.Set) + "|" + after.ElementID.String()
	}
	return pageAfter(s.elements, keyOf, afterKey, batch), nil
}

func (s *snapshotStore) Comments(
	_ context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.Comment], error) {
	s.reads++
	return pageAfter(s.comments,
		func(c repository.InCollection[work.Comment]) string { return c.Value.ID.String() },
		after.String(), batch), nil
}

func (s *snapshotStore) Reminders(
	_ context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.Reminder], error) {
	s.reads++
	return pageAfter(s.reminders,
		func(r repository.InCollection[work.Reminder]) string { return r.Value.ID.String() },
		after.String(), batch), nil
}

func (s *snapshotStore) Recurrences(
	_ context.Context, after shared.ID, batch int,
) ([]repository.InCollection[work.RecurrenceRule], error) {
	s.reads++
	return pageAfter(s.recurrences,
		func(r repository.InCollection[work.RecurrenceRule]) string { return r.Value.ID.String() },
		after.String(), batch), nil
}

func (s *snapshotStore) Templates(_ context.Context, after shared.ID, batch int) ([]work.Template, error) {
	s.reads++
	return pageAfter(s.templates, func(t work.Template) string { return t.ID.String() }, after.String(), batch), nil
}

// idAt mints a sortable identifier for the fixture: the kind's letter and a number, so that the
// order within a kind is the order of the numbers.
func idAt(kind byte, n int) shared.ID {
	return shared.MustParseID("01936f2a-7c1e-7000-8000-" + strings.Repeat(string(kind), 4) + pad(int64(n))[4:])
}

// workspace is a small, complete workspace: one hub with two collections, and one of everything
// under the first collection, plus an entry under the second. The second collection is what a
// partial-access test hides.
func workspace() *snapshotStore {
	first, second := collectionA, collectionB
	itemA, itemB := idAt('a', 1), idAt('a', 2)
	labelID := idAt('b', 1)
	removed := idAt('b', 2)
	added, _ := shared.NewHLC(now.Add(-time.Hour), 1, "dev-a")
	taken, _ := shared.NewHLC(now.Add(-time.Minute), 1, "dev-b")

	return &snapshotStore{
		containers: []work.Container{
			{ID: hub, TenantID: tenant, Type: work.ContainerHub, Name: "Home"},
			{ID: first, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub, Name: "Kitchen"},
			{ID: second, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub, Name: "Garage"},
		},
		buckets: []work.Bucket{{ID: idAt('c', 1), TenantID: tenant, CollectionID: first, Name: "Todo"}},
		labels:  []work.Label{{ID: labelID, TenantID: tenant, CollectionID: first, Name: "urgent"}},
		items: []work.WorkItem{
			{ID: itemA, TenantID: tenant, CollectionID: first, Type: work.ItemTask, Title: "Fix the tap"},
			{ID: itemB, TenantID: tenant, CollectionID: second, Type: work.ItemTask, Title: "Oil the door"},
		},
		elements: []repository.ItemSetElement{
			{ItemID: itemA, CollectionID: first, Set: work.SetLabels,
				Element: work.SetElement{ElementID: labelID, AddedAt: added}},
			{ItemID: itemA, CollectionID: first, Set: work.SetLabels,
				Element: work.SetElement{ElementID: removed, AddedAt: added, RemovedAt: taken}},
		},
		comments: []repository.InCollection[work.Comment]{{
			Value:        work.Comment{ID: idAt('d', 1), TenantID: tenant, ItemID: itemA, AuthorID: account, Body: "Done?"},
			CollectionID: first,
		}},
		reminders: []repository.InCollection[work.Reminder]{{
			Value:        work.Reminder{ID: idAt('e', 1), TenantID: tenant, ItemID: itemA},
			CollectionID: first,
		}},
		recurrences: []repository.InCollection[work.RecurrenceRule]{{
			Value:        work.RecurrenceRule{ID: idAt('f', 1), TenantID: tenant, ItemID: itemA, RRULE: "FREQ=DAILY"},
			CollectionID: first,
		}},
		templates: []work.Template{{ID: idAt('0', 1), TenantID: tenant, Scope: work.TemplateScopeCollection, ScopeID: first, Name: "Move"}},
	}
}

func walking(t *testing.T, entries ...repository.Recorded) (PullChanges, fixture, *snapshotStore) {
	t.Helper()
	pull, f := pulling(t, entries...)
	store := workspace()
	pull.Snapshot = store
	return pull, f, store
}

// wholeWalk pulls from nothing to the delta cursor and returns every record in order, and the
// cursor the walk ended on.
func wholeWalk(t *testing.T, pull PullChanges, limit int, scopes ...Scope) ([]Record, Position) {
	t.Helper()
	var records []Record
	request := PullRequest{DeviceID: device, Limit: limit, Scopes: scopes}
	for pages := 0; ; pages++ {
		page, err := pull.Pull(t.Context(), actor(), request)
		if err != nil {
			t.Fatalf("pulling page %d: %v", pages, err)
		}
		records = append(records, page.Records...)
		if !page.More {
			if page.Cursor.Walking() {
				t.Fatalf("the walk ended on a walk cursor: %+v", page.Cursor)
			}
			return records, page.Cursor
		}
		if !page.Cursor.Walking() {
			t.Fatalf("a page with more ended on a delta cursor: %+v", page.Cursor)
		}
		request.Cursor = pull.Encode(page.Cursor)
		if pages > 50 {
			t.Fatalf("the walk does not end")
		}
	}
}

func entities(records []Record) []string {
	out := make([]string, 0, len(records))
	for _, record := range records {
		out = append(out, record.Entity)
	}
	return out
}

func TestTheWalkDeliversEveryKindInOrderAndEndsOnTheDeltaCursor(t *testing.T) {
	pull, _, _ := walking(t, entry(1, collectionA), entry(2, collectionA))

	records, cursor := wholeWalk(t, pull, 4)

	want := []string{
		"container", "container", "container", "bucket", "label", "item", "item",
		"item", "item", // the two set elements, as records of their entry
		"comment", "reminder", "recurrence_rule", "template",
	}
	if got := entities(records); !slices.Equal(got, want) {
		t.Errorf("the walk delivered %v, want %v", got, want)
	}
	for _, record := range records {
		if record.Op != repository.Upsert || record.Payload == nil {
			t.Errorf("a walk record is not an UPSERT with a payload: %+v", record)
		}
	}
	// The cursor is the log's position *before* the first page: seq 2 was the head when the walk
	// began, and the delta resumes from there.
	if cursor.Seq != 2 || !cursor.IssuedAt.Equal(now) {
		t.Errorf("the walk ended on %+v, want the position taken at its start", cursor)
	}
}

// The acceptance criterion: a change that lands mid-walk is on the first delta, not lost between
// two pages.
func TestAChangeLandingMidWalkIsOnTheFirstDelta(t *testing.T) {
	pull, f, _ := walking(t, entry(1, collectionA))

	first, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Limit: 3})
	if err != nil {
		t.Fatalf("pulling the first page: %v", err)
	}
	if !first.More {
		t.Fatalf("the workspace fit in one page; the test needs a walk")
	}
	// Somebody edits while the device is still walking.
	f.changes.entries = append(f.changes.entries, entry(2, collectionA))

	cursor := first.Cursor
	for cursor.Walking() {
		page, err := pull.Pull(t.Context(), actor(),
			PullRequest{DeviceID: device, Limit: 3, Cursor: pull.Encode(cursor)})
		if err != nil {
			t.Fatalf("continuing the walk: %v", err)
		}
		cursor = page.Cursor
	}
	delta, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: pull.Encode(cursor)})
	if err != nil {
		t.Fatalf("pulling the delta: %v", err)
	}
	if !sameSeqs(seqs(delta.Records), []int64{2}) {
		t.Errorf("the first delta carries %v, want the change that landed mid-walk", seqs(delta.Records))
	}
}

// A member who may read one of two collections is told about that one and about nothing under
// the other - checked per record, by the container each row belongs to.
func TestAMemberWithPartialAccessGetsExactlyWhatTheyMayRead(t *testing.T) {
	pull, f, _ := walking(t)
	f.auth.allowed[collectionB] = false

	records, _ := wholeWalk(t, pull, 100)

	for _, record := range records {
		if record.ContainerID == collectionB {
			t.Errorf("a record under the hidden collection was delivered: %+v", record)
		}
	}
	items := 0
	for _, record := range records {
		if record.Entity == "item" && record.Payload["title"] != nil {
			items++
		}
	}
	if items != 1 {
		t.Errorf("%d entries delivered, want the one in the readable collection", items)
	}
}

func TestAWalkCursorRoundTripsAndIsRefusedWhenForged(t *testing.T) {
	pull, _, _ := walking(t)

	first, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Limit: 2})
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if first.Cursor.Kind != "container" || first.Cursor.After == "" {
		t.Errorf("the first page's cursor is %+v, want a walk position in the containers", first.Cursor)
	}

	// The next page resumes after exactly the last row handed out.
	second, err := pull.Pull(t.Context(), actor(),
		PullRequest{DeviceID: device, Limit: 2, Cursor: pull.Encode(first.Cursor)})
	if err != nil {
		t.Fatalf("pulling the second page: %v", err)
	}
	if second.Records[0].EntityID.String() <= first.Cursor.After {
		t.Errorf("the second page starts at %s, not after %s", second.Records[0].EntityID, first.Cursor.After)
	}

	// A kind this build does not walk is not resumable.
	_, err = pull.Pull(t.Context(), actor(),
		PullRequest{DeviceID: device, Cursor: pull.Encode(Position{Seq: 0, IssuedAt: now, Kind: "sticker"})})
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_invalid" {
		t.Errorf("a forged kind was refused with %q", got)
	}
}

// The stream cannot resume a walk: a device that streamed from the middle of one would believe
// itself complete with half its state missing.
func TestTheStreamRefusesAWalkCursor(t *testing.T) {
	pull, f, _ := walking(t)
	first, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Limit: 2})
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}

	_, err = f.stream.Resume(t.Context(), actor(), pull.Encode(first.Cursor))
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_invalid" {
		t.Errorf("the stream took a walk cursor: %v", err)
	}
}

func TestAWalkOlderThanTheWindowStartsAgain(t *testing.T) {
	pull, _, _ := walking(t)
	stale := pull.Encode(Position{Seq: 0, IssuedAt: now.Add(-window - time.Hour), Kind: "item"})

	_, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: stale})
	if got := shared.AsError(err).DetailCode; got != "sync.cursor_too_old" {
		t.Errorf("a stale walk was refused with %q", got)
	}
}

func TestSetElementsTravelAsTheDeltaSpellsThem(t *testing.T) {
	pull, _, _ := walking(t)
	records, _ := wholeWalk(t, pull, 100)

	var sets []Record
	for _, record := range records {
		if record.Entity == "item" && record.Payload["set"] != nil {
			sets = append(sets, record)
		}
	}
	if len(sets) != 2 {
		t.Fatalf("%d set records, want two", len(sets))
	}
	present, absent := sets[0], sets[1]
	if present.Payload["op"] != "add" || present.Payload["set"] != "labels" ||
		present.Payload["element_id"] != idAt('b', 1).String() || present.HLC.Device != "dev-a" {
		t.Errorf("the present element travels as %+v under %s", present.Payload, present.HLC)
	}
	// The removed element travels as a removal under its removal tag, so that a device merging a
	// later re-add has the removal to compare it against.
	if absent.Payload["op"] != "remove" || absent.HLC.Device != "dev-b" {
		t.Errorf("the removed element travels as %+v under %s", absent.Payload, absent.HLC)
	}
}

func TestAScopeNarrowsTheWalkToo(t *testing.T) {
	pull, _, _ := walking(t)
	records, _ := wholeWalk(t, pull, 100, Scope{ContainerID: collectionB, Depth: DepthSelf})

	if len(records) != 2 {
		t.Fatalf("%d records, want the collection and its one entry: %v", len(records), entities(records))
	}
	for _, record := range records {
		if record.ContainerID != collectionB {
			t.Errorf("a record outside the scope: %+v", record)
		}
	}
}

func TestThePageSizeIsHonouredAcrossKinds(t *testing.T) {
	pull, _, store := walking(t)
	records, _ := wholeWalk(t, pull, 5)
	if len(records) != 13 {
		t.Fatalf("%d records, want the whole workspace", len(records))
	}
	// Three full pages and a short last one, each filled across kind boundaries.
	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Limit: 5})
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	if got := entities(page.Records); !slices.Equal(got, []string{"container", "container", "container", "bucket", "label"}) {
		t.Errorf("the first page is %v", got)
	}
	if store.reads == 0 {
		t.Errorf("the store was never read")
	}
}

func TestAnInstallationWithoutASnapshotRefusesANullCursor(t *testing.T) {
	pull, _ := pulling(t)
	_, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device})
	if got := shared.AsError(err).DetailCode; got != "sync.initial_sync_unavailable" {
		t.Errorf("refused with %q", got)
	}
}

func TestAWalkRecordDescribesTheWholeObject(t *testing.T) {
	pull, _, _ := walking(t)
	records, _ := wholeWalk(t, pull, 100)

	for _, record := range records {
		switch record.Entity {
		case "container":
			if record.Payload["id"] != record.EntityID.String() || record.Payload["name"] == nil {
				t.Errorf("the container record is not the whole object: %v", record.Payload)
			}
		case "item":
			if record.Payload["set"] != nil {
				continue
			}
			if record.Payload["title"] == nil || record.Payload["collection_id"] == nil ||
				record.Payload["completion"] == nil {
				t.Errorf("the entry record is not the whole object: %v", record.Payload)
			}
		case "template":
			if record.Payload["nodes"] == nil {
				t.Errorf("the template record has no tree: %v", record.Payload)
			}
		}
		if record.Seq != 0 || !record.OccurredAt.Equal(now) {
			t.Errorf("a walk record is dated %v at seq %d, want the walk's start", record.OccurredAt, record.Seq)
		}
	}
	if !strings.HasPrefix(records[0].Payload["id"].(string), "01936f2a") {
		t.Errorf("the first record is %v", records[0].Payload)
	}
}
