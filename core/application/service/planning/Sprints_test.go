// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// sprintStore keeps sprints in memory, with the two indexes the table holds: one running sprint per
// collection, one open membership per entry.
type sprintStore struct {
	sprints   map[shared.ID]domain.Sprint
	members   map[shared.ID][]planrepo.SprintMember
	estimates map[shared.ID]domain.Estimate
	done      map[shared.ID]*time.Time
	// stale is what a read outside a transaction answers: the sprint as another request saw it
	// before a concurrent change; a locked read answers the row as it stands.
	stale map[shared.ID]domain.Sprint
	// trashed entries are left out of a sprint's entries, as the store's read leaves them out.
	trashed map[shared.ID]bool
	// mine is what Mine answers, by the account asked for.
	mine    map[shared.ID][]planrepo.MySprint
	mineMax int
}

func newSprintStore() *sprintStore {
	return &sprintStore{
		sprints: map[shared.ID]domain.Sprint{}, members: map[shared.ID][]planrepo.SprintMember{},
		estimates: map[shared.ID]domain.Estimate{}, done: map[shared.ID]*time.Time{},
		stale: map[shared.ID]domain.Sprint{}, trashed: map[shared.ID]bool{},
	}
}

func (s *sprintStore) Insert(_ context.Context, sprint domain.Sprint) error {
	s.sprints[sprint.ID] = sprint
	return nil
}

func (s *sprintStore) Update(_ context.Context, sprint domain.Sprint, expected int) error {
	if s.sprints[sprint.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	for _, other := range s.sprints {
		if other.ID != sprint.ID && other.CollectionID == sprint.CollectionID && other.State == domain.SprintActive && sprint.State == domain.SprintActive {
			return shared.ErrConflict.WithDetail("sprints.another_running")
		}
	}
	sprint.Version = expected + 1
	s.sprints[sprint.ID] = sprint
	return nil
}

func (s *sprintStore) Find(_ context.Context, id shared.ID) (domain.Sprint, error) {
	if sprint, ok := s.stale[id]; ok {
		return sprint, nil
	}
	if sprint, ok := s.sprints[id]; ok {
		return sprint, nil
	}
	return domain.Sprint{}, shared.ErrNotFound.WithDetail("sprints.not_found")
}

func (s *sprintStore) Lock(_ context.Context, id shared.ID) (domain.Sprint, error) {
	if sprint, ok := s.sprints[id]; ok {
		return sprint, nil
	}
	return domain.Sprint{}, shared.ErrNotFound.WithDetail("sprints.not_found")
}

func (s *sprintStore) Count(_ context.Context, id shared.ID) (int, error) {
	return len(s.members[id]), nil
}

func (s *sprintStore) ReturnOpen(_ context.Context, id shared.ID) error {
	for i, m := range s.members[id] {
		if m.RemovedAt == nil && m.Outcome == "" {
			s.members[id][i].Outcome = domain.OutcomeReturned
		}
	}
	return nil
}

func (s *sprintStore) List(_ context.Context, c shared.ID, state domain.SprintState, _ int) ([]domain.Sprint, error) {
	var out []domain.Sprint
	for _, sprint := range s.sprints {
		if sprint.CollectionID == c && (state == "" || sprint.State == state) {
			out = append(out, sprint)
		}
	}
	slices.SortFunc(out, func(a, b domain.Sprint) int { return int(b.StartOn - a.StartOn) })
	return out, nil
}

func (s *sprintStore) Mine(_ context.Context, account shared.ID, limit int) ([]planrepo.MySprint, error) {
	s.mineMax = limit
	return s.mine[account], nil
}

func (s *sprintStore) Delete(_ context.Context, id shared.ID) error {
	delete(s.sprints, id)
	delete(s.members, id)
	return nil
}

func (s *sprintStore) Entries(_ context.Context, id shared.ID, _ int) ([]planrepo.SprintMember, error) {
	out := make([]planrepo.SprintMember, 0, len(s.members[id]))
	for _, m := range s.members[id] {
		if s.trashed[m.EntryID] {
			continue
		}
		m.CompletedAt = s.done[m.EntryID]
		m.Done = m.CompletedAt != nil
		out = append(out, m)
	}
	return out, nil
}

func (s *sprintStore) OpenMemberships(_ context.Context, entries []shared.ID) ([]planrepo.OpenMembership, error) {
	var out []planrepo.OpenMembership
	for sprint, members := range s.members {
		for _, m := range members {
			if slices.Contains(entries, m.EntryID) && m.RemovedAt == nil && m.Outcome == "" {
				out = append(out, planrepo.OpenMembership{EntryID: m.EntryID, SprintID: sprint})
			}
		}
	}
	return out, nil
}

func (s *sprintStore) Add(_ context.Context, sprint, entry shared.ID, at time.Time, committed bool) error {
	s.members[sprint] = append(s.members[sprint], planrepo.SprintMember{
		SprintEntry:  domain.SprintEntry{SprintID: sprint, EntryID: entry, AddedAt: at, Committed: committed},
		CollectionID: collection, HubID: hubID,
	})
	return nil
}

func (s *sprintStore) Remove(_ context.Context, sprint, entry shared.ID, at time.Time) (bool, error) {
	for i, m := range s.members[sprint] {
		if m.EntryID == entry && m.RemovedAt == nil && m.Outcome == "" {
			s.members[sprint][i].RemovedAt = &at
			return true, nil
		}
	}
	return false, nil
}

func (s *sprintStore) Commit(_ context.Context, sprint shared.ID) error {
	for i := range s.members[sprint] {
		s.members[sprint][i].Committed = true
	}
	return nil
}

func (s *sprintStore) SetOutcomes(_ context.Context, sprint shared.ID, outcome domain.SprintOutcome, entries []shared.ID) error {
	for i, m := range s.members[sprint] {
		if slices.Contains(entries, m.EntryID) && m.Outcome == "" {
			s.members[sprint][i].Outcome = outcome
		}
	}
	return nil
}

func (s *sprintStore) SetEstimate(_ context.Context, e domain.Estimate) error {
	s.estimates[e.EntryID] = e
	return nil
}

func (s *sprintStore) ClearEstimate(_ context.Context, id shared.ID) (bool, error) {
	_, ok := s.estimates[id]
	delete(s.estimates, id)
	return ok, nil
}

func (s *sprintStore) Estimates(_ context.Context, ids []shared.ID) (map[shared.ID]domain.Estimate, error) {
	out := map[shared.ID]domain.Estimate{}
	for _, id := range ids {
		if e, ok := s.estimates[id]; ok {
			out[id] = e
		}
	}
	return out, nil
}

var elsewhere = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000105")

func sprintFixture() (fixture, *sprintStore) {
	f := newFixture()
	store := newSprintStore()
	f.planner.Sprints = store
	f.planner.References = references{summaries: []workrepo.EntrySummary{
		{ID: design, CollectionID: collection, HubID: hubID, Title: "Approve the design"},
		{ID: build, CollectionID: collection, HubID: hubID, Title: "Build"},
		{ID: launch, CollectionID: collection, HubID: hubID, Title: "Launch"},
		{ID: secret, CollectionID: collection, HubID: hubID, Title: "Secret"},
		{ID: elsewhere, CollectionID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000012"), HubID: hubID, Title: "Elsewhere"},
	}}
	return f, store
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func planSprint(t *testing.T, f fixture, name, start string) domain.Sprint {
	t.Helper()
	first, _ := time.Parse(time.DateOnly, start)
	out, err := CreateSprint{Planner: f.planner}.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"collection_id": collection.String(), "name": name, "goal": "Ship the beta", "start_on": start,
		"end_on": first.AddDate(0, 0, 11).Format(time.DateOnly),
	})
	if err != nil {
		t.Fatal(err)
	}
	sprint, err := f.planner.Sprints.Find(context.Background(), shared.MustParseID(out.String("id")))
	if err != nil {
		t.Fatal(err)
	}
	return sprint
}

// UC-PLN-06 checks 1, 2 and 4: a sprint is planned for a collection, entries of that collection
// join it, one open sprint at a time, and the reader sees only the entries they may.
func TestASprintIsPlannedWithEntriesOfItsCollection(t *testing.T) {
	f, _ := sprintFixture()
	ctx := context.Background()
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	if asked := f.gate.asked[0]; asked.Permission != service.PermissionWriteItems || asked.TargetID != collection {
		t.Errorf("asked %+v, want WRITE_ITEMS at the collection", asked)
	}
	if sprint.State != domain.SprintPlanned || sprint.Goal != "Ship the beta" || sprint.Unit != domain.Points {
		t.Fatalf("sprint = %+v", sprint)
	}
	if _, err := (CreateSprint{Planner: f.planner}).Execute(ctx, actor(), hubID, domain.SprintFields{Name: "S", StartOn: 1, EndOn: 2}); detail(err) != "sprints.collection_required" {
		t.Errorf("a hub got a sprint: %v", err)
	}
	add := AddSprintEntries{Planner: f.planner}
	if _, err := add.Execute(ctx, actor(), sprint.ID, []shared.ID{elsewhere}); detail(err) != "sprints.entry_elsewhere" {
		t.Errorf("an entry of another collection joined: %v", err)
	}
	if _, err := add.Execute(ctx, actor(), sprint.ID, []shared.ID{secret}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an entry out of reach joined: %v", err)
	}
	if _, err := add.Execute(ctx, actor(), sprint.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	other := planSprint(t, f, "Sprint 2", "2026-11-16")
	if _, err := add.Execute(ctx, actor(), other.ID, []shared.ID{design}); detail(err) != "sprints.entry_in_another_sprint" {
		t.Errorf("an entry joined two open sprints: %v", err)
	}
	f.gate.hidden[build] = true
	view, err := ReadSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Entries) != 1 || view.Entries[0].EntryID != design || view.Entries[0].Title != "Approve the design" {
		t.Errorf("the reader sees %+v, want only the design", view.Entries)
	}
}

// UC-PLN-06 checks 3 and 5, UC-PLN-07 checks 1, 2 and 3: starting records the commitment from the estimates,
// closing the result, the unfinished entries move into the next sprint, a running sprint is not
// deleted, the burndown follows the days, and the velocity averages the closed sprints.
func TestASprintRunsAndCloses(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	estimate := SetEntryEstimate{Planner: f.planner}
	if _, err := estimate.Execute(ctx, actor(), SetEntryEstimateCommand{ItemID: design, Unit: domain.Points, Value: 3}); err != nil {
		t.Fatal(err)
	}
	if asked := f.gate.asked[len(f.gate.asked)-1]; asked.Permission != service.PermissionWriteItems || asked.TargetID != design {
		t.Errorf("asked %+v, want WRITE_ITEMS on the entry", asked)
	}
	if e, err := estimate.Execute(ctx, actor(), SetEntryEstimateCommand{ItemID: build, Unit: domain.Size, Size: "L"}); err != nil || e.Value != 5 {
		t.Fatalf("an L counts %v (%v), want 5 points", e.Value, err)
	}
	if _, err := estimate.Execute(ctx, actor(), SetEntryEstimateCommand{ItemID: build, Unit: domain.Points, Value: -2}); detail(err) != "estimates.value_out_of_range" {
		t.Errorf("a negative estimate: %v", err)
	}
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	next := planSprint(t, f, "Sprint 2", "2026-11-16")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	f.planner.Clock = fixedAt(time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC))
	started, err := StartSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.CommittedValue != 8 || started.CommittedCount != 2 || started.State != domain.SprintActive {
		t.Fatalf("commitment = %+v", started)
	}
	if _, err := (StartSprint{Planner: f.planner}).Execute(ctx, actor(), next.ID); detail(err) != "sprints.another_running" {
		t.Errorf("a second sprint ran: %v", err)
	}
	if err := (DeleteSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); detail(err) != "sprints.not_planned" {
		t.Errorf("a running sprint was deleted: %v", err)
	}
	doneAt := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	store.done[design] = &doneAt
	f.planner.Clock = fixedAt(time.Date(2026, 11, 5, 10, 0, 0, 0, time.UTC))
	view, err := ReadSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Burndown) != 4 || view.Burndown[1].Remaining != 8 || view.Burndown[2].Remaining != 5 || view.Burndown[0].Ideal != 7.2 {
		t.Errorf("burndown = %+v", view.Burndown)
	}
	if _, err := (CloseSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, sprint.ID); detail(err) != "sprints.carry_to_itself" {
		t.Errorf("carried into itself: %v", err)
	}
	result, err := CloseSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID, next.ID)
	if err != nil {
		t.Fatal(err)
	}
	if result.Sprint.CompletedValue != 3 || result.Sprint.CompletedCount != 1 || len(result.Carried) != 1 || result.Carried[0] != build {
		t.Fatalf("close = %+v", result)
	}
	held, _ := store.OpenMemberships(ctx, []shared.ID{build, design})
	if len(held) != 1 || held[0].SprintID != next.ID || held[0].EntryID != build {
		t.Errorf("open memberships = %+v, want build in the next sprint", held)
	}
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{launch}); detail(err) != "sprints.closed" {
		t.Errorf("an entry joined a closed sprint: %v", err)
	}
	velocity, err := ReadVelocity{Planner: f.planner}.Execute(ctx, actor(), collection)
	if err != nil || velocity.Average != 3 || len(velocity.Sprints) != 1 {
		t.Errorf("velocity = %+v, %v", velocity, err)
	}
	backlog, err := ReadSprintBacklog{Planner: f.planner}.Execute(ctx, actor(), collection)
	if err != nil {
		t.Fatal(err)
	}
	var names []shared.ID
	for _, b := range backlog {
		names = append(names, b.ID)
	}
	if !slices.Contains(names, launch) || slices.Contains(names, build) || slices.Contains(names, secret) {
		t.Errorf("backlog = %v, want the launch, not the carried build nor the secret", names)
	}
}

// UC-PLN-06 check 5: a planned sprint is deleted and its entries go back to the backlog as they
// were - estimated as before, free to join another sprint.
func TestAPlannedSprintIsDeletedAndItsEntriesGoBack(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	if _, err := (SetEntryEstimate{Planner: f.planner}).Execute(ctx, actor(), SetEntryEstimateCommand{ItemID: design, Unit: domain.Points, Value: 3}); err != nil {
		t.Fatal(err)
	}
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	if err := (DeleteSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); err != nil {
		t.Fatalf("deleting a planned sprint: %v", err)
	}
	if _, kept := store.sprints[sprint.ID]; kept {
		t.Error("the sprint is still there")
	}
	if open, _ := store.OpenMemberships(ctx, []shared.ID{design, build}); len(open) != 0 {
		t.Errorf("the entries are still in a sprint: %+v", open)
	}
	if e := store.estimates[design]; e.Unit != domain.Points || e.Value != 3 {
		t.Errorf("the entry's estimate changed: %+v", e)
	}
	next := planSprint(t, f, "Sprint 2", "2026-11-16")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), next.ID, []shared.ID{design, build}); err != nil {
		t.Errorf("the entries back in the backlog did not join the next sprint: %v", err)
	}
}

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

func fixedAt(at time.Time) fixedClock { return fixedClock{at: at} }

// UC-PLN-06: the backlog a sprint is planned from lists the collection's top-level entries; a
// work package under one of them travels with its parent and is not offered on its own.
func TestTheBacklogListsTopLevelEntries(t *testing.T) {
	f, _ := sprintFixture()
	child := shared.MustParseID("01936f2a-7c1e-7000-8000-000000000107")
	f.plans.entries = append(f.plans.entries, planrepo.PlanEntry{ID: child, ParentID: build, CollectionID: collection, HubID: hubID, Title: "Build the header"})
	backlog, err := ReadSprintBacklog{Planner: f.planner}.Execute(context.Background(), actor(), collection)
	if err != nil {
		t.Fatal(err)
	}
	var ids []shared.ID
	for _, b := range backlog {
		ids = append(ids, b.ID)
	}
	if slices.Contains(ids, child) || !slices.Contains(ids, build) {
		t.Errorf("backlog = %v, want the build without its work package", ids)
	}
}

// taken names the templates each place took.
type taken map[shared.ID][]string

func (t taken) TemplatesTaken(_ context.Context, place shared.ID) ([]string, error) {
	return t[place], nil
}

// user-experience.md §2.4: a collection runs sprints where a sprint was planned there - whatever the
// state asked for - or where it or its hub took the Scrum team; a Kanban hub's collection does not.
func TestACollectionRunsSprintsWhereOneWasPlannedOrItsTemplateBringsThem(t *testing.T) {
	f, _ := sprintFixture()
	ctx := context.Background()
	runs := func(state string) any {
		t.Helper()
		in := usecase.Input{"collection_id": collection.String()}
		if state != "" {
			in["state"] = state
		}
		out, err := ListSprints{Planner: f.planner}.Descriptor().Handler.Invoke(ctx, actor(), in)
		if err != nil {
			t.Fatal(err)
		}
		return out["runs_sprints"]
	}
	if got := runs(""); got != false {
		t.Errorf("a collection without sprints or template runs sprints: %v", got)
	}
	f.planner.Templates = taken{hubID: {"kanban_team"}}
	if got := runs(""); got != false {
		t.Errorf("a Kanban hub's collection runs sprints: %v", got)
	}
	f.planner.Templates = taken{hubID: {"kanban_team", "scrum_team"}}
	if got := runs(""); got != true {
		t.Errorf("a Scrum hub's collection does not run sprints: %v", got)
	}
	f.planner.Templates = taken{collection: {"scrum_team"}}
	if got := runs(""); got != true {
		t.Errorf("a collection that took the Scrum team does not run sprints: %v", got)
	}
	f.planner.Templates = nil
	planSprint(t, f, "Sprint 1", "2026-11-02")
	if got := runs("CLOSED"); got != true {
		t.Errorf("a collection with a planned sprint does not run sprints when closed ones are asked: %v", got)
	}
}

// UC-PLN-06: an entry trashed while its sprint runs is not read at the close; its membership is
// closed all the same, so that once restored it is back in the backlog and may join another
// sprint rather than being held for ever by a closed one.
func TestAnEntryTrashedDuringASprintIsFreeAfterItsClose(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	if _, err := (StartSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); err != nil {
		t.Fatal(err)
	}
	store.trashed[build] = true
	if _, err := (CloseSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, ""); err != nil {
		t.Fatal(err)
	}
	delete(store.trashed, build)
	if open, _ := store.OpenMemberships(ctx, []shared.ID{build}); len(open) != 0 {
		t.Errorf("the restored entry is still held by the closed sprint: %+v", open)
	}
	next := planSprint(t, f, "Sprint 2", "2026-11-16")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), next.ID, []shared.ID{build}); err != nil {
		t.Errorf("the restored entry did not join the next sprint: %v", err)
	}
}

// UC-PLN-06: a sprint holds at most 1000 entries, those once in it among them - what its start and
// close read; one more is refused rather than left out of the commitment and held open at the close.
func TestASprintHoldsAtMostItsEntries(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	for range domain.MaxSprintEntries {
		store.members[sprint.ID] = append(store.members[sprint.ID], planrepo.SprintMember{
			SprintEntry: domain.SprintEntry{SprintID: sprint.ID, EntryID: shared.ID("01936f2a-7c1e-7000-8000-00000000ffff")},
		})
	}
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design}); detail(err) != "sprints.full" {
		t.Errorf("the 1001st entry joined: %v", err)
	}
}

// UC-PLN-06: what changes a sprint's entries or state reads the sprint again under its lock, so a
// close, start or delete that another request made meanwhile is not undone by a stale read.
func TestASprintChangedMeanwhileIsReadUnderItsLock(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design}); err != nil {
		t.Fatal(err)
	}
	planned := store.sprints[sprint.ID]
	if _, err := (StartSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); err != nil {
		t.Fatal(err)
	}
	store.stale[sprint.ID] = planned
	if err := (DeleteSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); detail(err) != "sprints.not_planned" {
		t.Errorf("a sprint started meanwhile was deleted: %v", err)
	}
	running := store.sprints[sprint.ID]
	if _, err := (CloseSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, ""); err != nil {
		t.Fatal(err)
	}
	store.stale[sprint.ID] = running
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{build}); detail(err) != "sprints.closed" {
		t.Errorf("an entry joined a sprint closed meanwhile: %v", err)
	}
	if _, err := (RemoveSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design}); detail(err) != "sprints.closed" {
		t.Errorf("an entry left a sprint closed meanwhile: %v", err)
	}
	if open, _ := store.OpenMemberships(ctx, []shared.ID{build}); len(open) != 0 {
		t.Errorf("the entry is held by a closed sprint: %+v", open)
	}
}

// UC-PLN-06: unfinished entries are carried only into a sprint that can hold them; a full one
// refuses the close, which leaves the sprint running for its closer to choose another.
func TestUnfinishedEntriesAreNotCarriedIntoAFullSprint(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	sprint, next := planSprint(t, f, "Sprint 1", "2026-11-02"), planSprint(t, f, "Sprint 2", "2026-11-16")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{build}); err != nil {
		t.Fatal(err)
	}
	if _, err := (StartSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID); err != nil {
		t.Fatal(err)
	}
	for range domain.MaxSprintEntries {
		store.members[next.ID] = append(store.members[next.ID], planrepo.SprintMember{
			SprintEntry: domain.SprintEntry{SprintID: next.ID, EntryID: shared.ID("01936f2a-7c1e-7000-8000-00000000ffff")},
		})
	}
	if _, err := (CloseSprint{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, next.ID); detail(err) != "sprints.full" {
		t.Errorf("carried into a full sprint: %v", err)
	}
}

type sprintWorkflows struct {
	workrepo.Workflows
	rows []work.Workflow
}

func (w sprintWorkflows) List(context.Context) ([]work.Workflow, error) { return w.rows, nil }

// user-experience.md §7.5: the sprint's board has an *In progress* lane - an open entry whose
// status a workflow of the workspace counts as begun is said to be started; a done one and one
// without a status are not.
func TestASprintSaysWhichOfItsEntriesAreStarted(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	f.planner.References = references{summaries: []workrepo.EntrySummary{
		{ID: design, CollectionID: collection, HubID: hubID, Title: "Approve the design", Status: "in_progress"},
		{ID: build, CollectionID: collection, HubID: hubID, Title: "Build", Status: "review"},
		{ID: launch, CollectionID: collection, HubID: hubID, Title: "Launch"},
	}}
	f.planner.Workflows = sprintWorkflows{rows: []work.Workflow{{Key: "desk", Published: work.WorkflowShape{Statuses: []work.WorkflowStatus{
		{Key: "review", Category: work.CategoryInProgress},
	}}}}}
	sprint := planSprint(t, f, "Sprint 1", "2026-11-02")
	if _, err := (AddSprintEntries{Planner: f.planner}).Execute(ctx, actor(), sprint.ID, []shared.ID{design, build, launch}); err != nil {
		t.Fatal(err)
	}
	doneAt := time.Date(2026, 11, 4, 15, 0, 0, 0, time.UTC)
	store.done[design] = &doneAt
	view, err := ReadSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	started := map[shared.ID]bool{}
	for _, e := range view.Entries {
		started[e.EntryID] = e.Started
	}
	want := map[shared.ID]bool{design: false, build: true, launch: false}
	for id, w := range want {
		if started[id] != w {
			t.Errorf("entry %s started = %v, want %v", id, started[id], w)
		}
	}
	entries, _ := view.Output()["entries"].([]usecase.Output)
	if len(entries) == 0 || entries[0]["started"] == nil {
		t.Errorf("the output carries no started: %+v", entries)
	}

	f.planner.Workflows = nil
	view, err = ReadSprint{Planner: f.planner}.Execute(ctx, actor(), sprint.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range view.Entries {
		if e.EntryID == build && e.Started {
			t.Error("without the workspace's workflows a status only another workflow names counted as begun")
		}
	}
}

// UC-WRK-34 check 8: *Today*'s Sprint band lists the running sprints holding the reader's entries,
// and only where the reader may read the entry the sprint is judged on.
func TestTheRunningSprintsIAmInAreThoseICanRead(t *testing.T) {
	f, store := sprintFixture()
	ctx := context.Background()
	running := domain.Sprint{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000201"), CollectionID: collection,
		Name: "Sprint 7", State: domain.SprintActive, StartOn: 1, EndOn: 12, CommittedCount: 5, CompletedCount: 2}
	unseen := domain.Sprint{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-000000000202"), CollectionID: collection,
		Name: "Theirs", State: domain.SprintActive, StartOn: 1, EndOn: 14}
	store.mine = map[shared.ID][]planrepo.MySprint{accountID: {
		{Sprint: running, HubID: hubID, EntryID: design, Entries: 2},
		{Sprint: unseen, HubID: hubID, EntryID: secret, Entries: 1},
	}}
	out, err := ListMyRunningSprints{Planner: f.planner}.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want the one sprint whose entry the reader may read", rows)
	}
	sprint, _ := rows[0]["sprint"].(usecase.Output)
	if sprint["id"] != running.ID.String() || rows[0]["hub_id"] != hubID.String() || rows[0]["my_entries"] != 2 {
		t.Errorf("row = %+v", rows[0])
	}
	if store.mineMax != maxMySprints {
		t.Errorf("asked for %d, want the bound %d", store.mineMax, maxMySprints)
	}
	store.mine = nil
	out, err = ListMyRunningSprints{Planner: f.planner}.Descriptor().Handler.Invoke(ctx, actor(), usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	if rows, _ := out["data"].([]usecase.Output); rows == nil || len(rows) != 0 {
		t.Errorf("no sprint answered %+v, want an empty list", out["data"])
	}
}
