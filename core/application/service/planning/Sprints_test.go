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
	"github.com/Jersyfi/hubtask/core/domain/service"
)

// sprintStore keeps sprints in memory, with the two indexes the table holds: one running sprint per
// collection, one open membership per entry.
type sprintStore struct {
	sprints   map[shared.ID]domain.Sprint
	members   map[shared.ID][]planrepo.SprintMember
	estimates map[shared.ID]domain.Estimate
	done      map[shared.ID]*time.Time
}

func newSprintStore() *sprintStore {
	return &sprintStore{
		sprints: map[shared.ID]domain.Sprint{}, members: map[shared.ID][]planrepo.SprintMember{},
		estimates: map[shared.ID]domain.Estimate{}, done: map[shared.ID]*time.Time{},
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
	if sprint, ok := s.sprints[id]; ok {
		return sprint, nil
	}
	return domain.Sprint{}, shared.ErrNotFound.WithDetail("sprints.not_found")
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

func (s *sprintStore) Delete(_ context.Context, id shared.ID) error {
	delete(s.sprints, id)
	delete(s.members, id)
	return nil
}

func (s *sprintStore) Entries(_ context.Context, id shared.ID, _ int) ([]planrepo.SprintMember, error) {
	out := slices.Clone(s.members[id])
	for i := range out {
		out[i].CompletedAt = s.done[out[i].EntryID]
		out[i].Done = out[i].CompletedAt != nil
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

// UC-PLN-06 check 3, UC-PLN-07 checks 1 to 3: starting records the commitment from the estimates,
// closing the result, the unfinished entries move into the next sprint, the burndown follows the
// days, and the velocity averages the closed sprints.
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

type fixedClock struct{ at time.Time }

func (c fixedClock) Now() time.Time { return c.at }

func fixedAt(at time.Time) fixedClock { return fixedClock{at: at} }
