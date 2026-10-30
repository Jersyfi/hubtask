// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package planning

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	planrepo "github.com/Jersyfi/hubtask/core/application/repository/planning"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
)

var (
	notesPage  = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000201")
	otherHubID = shared.MustParseID("01936f2a-7c1e-7000-8000-000000000202")
)

// releaseStore keeps releases in memory with the table's index: one release per entry.
type releaseStore struct {
	releases map[shared.ID]domain.Release
	members  map[shared.ID][]planrepo.ReleaseMember
	done     map[shared.ID]bool
}

func newReleaseStore() *releaseStore {
	return &releaseStore{releases: map[shared.ID]domain.Release{}, members: map[shared.ID][]planrepo.ReleaseMember{}, done: map[shared.ID]bool{}}
}

func (s *releaseStore) Insert(_ context.Context, r domain.Release) error {
	s.releases[r.ID] = r
	return nil
}

func (s *releaseStore) Update(_ context.Context, r domain.Release, expected int) error {
	if s.releases[r.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	r.Version = expected + 1
	s.releases[r.ID] = r
	return nil
}

func (s *releaseStore) Find(_ context.Context, id shared.ID) (domain.Release, error) {
	if r, ok := s.releases[id]; ok {
		return r, nil
	}
	return domain.Release{}, shared.ErrNotFound.WithDetail("releases.not_found")
}

func (s *releaseStore) List(_ context.Context, hub shared.ID, state domain.ReleaseState, _ int) ([]domain.Release, error) {
	var out []domain.Release
	for _, r := range s.releases {
		if r.HubID == hub && (state == "" || r.State == state) {
			out = append(out, r)
		}
	}
	return out, nil
}

func (s *releaseStore) Delete(_ context.Context, id shared.ID) error {
	delete(s.releases, id)
	delete(s.members, id)
	return nil
}

func (s *releaseStore) Entries(_ context.Context, id shared.ID, _ int) ([]planrepo.ReleaseMember, error) {
	out := slices.Clone(s.members[id])
	for i := range out {
		out[i].Done = s.done[out[i].EntryID]
	}
	return out, nil
}

func (s *releaseStore) Count(_ context.Context, id shared.ID) (int, error) {
	return len(s.members[id]), nil
}

func (s *releaseStore) Of(_ context.Context, entries []shared.ID) ([]planrepo.EntryRelease, error) {
	var out []planrepo.EntryRelease
	for release, members := range s.members {
		for _, m := range members {
			if slices.Contains(entries, m.EntryID) {
				out = append(out, planrepo.EntryRelease{EntryID: m.EntryID, ReleaseID: release})
			}
		}
	}
	return out, nil
}

func (s *releaseStore) Add(_ context.Context, release, entry shared.ID, at time.Time) error {
	titles := map[shared.ID]string{design: "Approve the design", build: "Build", launch: "Launch"}
	s.members[release] = append(s.members[release], planrepo.ReleaseMember{
		EntryID: entry, AddedAt: at, Title: titles[entry], TypeName: "Story", CollectionID: collection, HubID: hubID,
	})
	return nil
}

func (s *releaseStore) Remove(_ context.Context, release, entry shared.ID) (bool, error) {
	for i, m := range s.members[release] {
		if m.EntryID == entry {
			s.members[release] = slices.Delete(s.members[release], i, i+1)
			return true, nil
		}
	}
	return false, nil
}

func releaseFixture() (fixture, *releaseStore) {
	f, _ := sprintFixture()
	store := newReleaseStore()
	f.planner.Releases = store
	f.planner.References = references{summaries: []workrepo.EntrySummary{
		{ID: design, CollectionID: collection, HubID: hubID, Title: "Approve the design"},
		{ID: build, CollectionID: collection, HubID: hubID, Title: "Build"},
		{ID: launch, CollectionID: collection, HubID: hubID, Title: "Launch"},
		{ID: secret, CollectionID: collection, HubID: hubID, Title: "Secret"},
		{ID: elsewhere, CollectionID: collection, HubID: otherHubID, Title: "Elsewhere"},
	}}
	return f, store
}

func planRelease(t *testing.T, f fixture, name string) domain.Release {
	t.Helper()
	out, err := CreateRelease{Planner: f.planner}.Descriptor().Handler.Invoke(context.Background(), actor(), usecase.Input{
		"hub_id": hubID.String(), "name": name, "description": "The checkout release.", "release_on": "2026-11-30",
	})
	if err != nil {
		t.Fatal(err)
	}
	r, err := f.planner.Releases.Find(context.Background(), shared.MustParseID(out.String("id")))
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// UC-PLN-08 checks 1, 2 and 5: a release is planned for a hub, entries of its collections join it,
// one release at a time, the reader sees only the entries they may, and only a planned one is
// deleted.
func TestAReleaseCollectsEntriesOfItsHub(t *testing.T) {
	f, store := releaseFixture()
	ctx := context.Background()
	release := planRelease(t, f, "2.4")
	if asked := f.gate.asked[0]; asked.Permission != service.PermissionWriteItems || asked.TargetID != hubID {
		t.Errorf("asked %+v, want WRITE_ITEMS at the hub", asked)
	}
	if _, err := (CreateRelease{Planner: f.planner}).Execute(ctx, actor(), collection, domain.ReleaseFields{Name: "x", ReleaseOn: 1}); detail(err) != "releases.hub_required" {
		t.Errorf("a collection got a release: %v", err)
	}
	add := AddReleaseEntries{Planner: f.planner}
	if _, err := add.Execute(ctx, actor(), release.ID, []shared.ID{elsewhere}); detail(err) != "releases.entry_elsewhere" {
		t.Errorf("an entry of another hub joined: %v", err)
	}
	if _, err := add.Execute(ctx, actor(), release.ID, []shared.ID{secret}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an entry out of reach joined: %v", err)
	}
	if _, err := add.Execute(ctx, actor(), release.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	other := planRelease(t, f, "2.5")
	if _, err := add.Execute(ctx, actor(), other.ID, []shared.ID{design}); detail(err) != "releases.entry_in_another_release" {
		t.Errorf("an entry joined two releases: %v", err)
	}
	f.gate.hidden[build] = true
	view, err := ReadRelease{Planner: f.planner}.Execute(ctx, actor(), release.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Entries) != 1 || view.Entries[0].EntryID != design {
		t.Errorf("the reader sees %+v, want only the design", view.Entries)
	}
	delete(f.gate.hidden, build)
	if _, err := (RemoveReleaseEntries{Planner: f.planner}).Execute(ctx, actor(), release.ID, []shared.ID{build}); err != nil || len(store.members[release.ID]) != 1 {
		t.Errorf("remove: %v %+v", err, store.members[release.ID])
	}
	if err := (DeleteRelease{Planner: f.planner}).Execute(ctx, actor(), other.ID); err != nil {
		t.Errorf("a planned release was not deleted: %v", err)
	}
}

// UC-PLN-08 checks 3 and 4: releasing it ships what is done, carries what is open into the next
// release, writes its notes as a page of the hub's space through CreatePage, and then it changes no
// more.
func TestAReleaseShipsItsDoneEntriesAndWritesItsNotes(t *testing.T) {
	f, store := releaseFixture()
	ctx := context.Background()
	release, next := planRelease(t, f, "2.4"), planRelease(t, f, "2.5")
	if _, err := (AddReleaseEntries{Planner: f.planner}).Execute(ctx, actor(), release.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	store.done[design] = true
	if _, err := (PublishRelease{Planner: f.planner}).Execute(ctx, actor(), PublishReleaseCommand{ReleaseID: release.ID, CarryTo: release.ID}); detail(err) != "releases.carry_target_invalid" {
		t.Errorf("carried into itself: %v", err)
	}
	out, err := PublishRelease{Planner: f.planner}.Execute(ctx, actor(), PublishReleaseCommand{ReleaseID: release.ID, WriteNotes: true, CarryTo: next.ID})
	if err != nil {
		t.Fatal(err)
	}
	if out.State != domain.ReleaseReleased || out.NotesPageID != notesPage || out.ReleasedAt == nil {
		t.Fatalf("release = %+v", out)
	}
	call := f.catalogue.calls[len(f.catalogue.calls)-1]
	body, _ := call["body"].(string)
	if call["hub_id"] != hubID.String() || call["title"] != "2.4" || !strings.Contains(body, "hubtask:entry/"+design.String()) || strings.Contains(body, build.String()) {
		t.Errorf("the notes page = %+v", call)
	}
	if held := store.members[release.ID]; len(held) != 1 || held[0].EntryID != design {
		t.Errorf("the release holds %+v, want the done design", held)
	}
	if held := store.members[next.ID]; len(held) != 1 || held[0].EntryID != build {
		t.Errorf("the next release holds %+v, want the open build", held)
	}
	if _, err := (AddReleaseEntries{Planner: f.planner}).Execute(ctx, actor(), release.ID, []shared.ID{launch}); detail(err) != "releases.released" {
		t.Errorf("an entry joined a released release: %v", err)
	}
	if err := (DeleteRelease{Planner: f.planner}).Execute(ctx, actor(), release.ID); detail(err) != "releases.released" {
		t.Errorf("a released release was deleted: %v", err)
	}
}

// UC-PLN-08 offline: a release and the release an entry is in reach the change log under the
// hub as each write leaves them - a released one's open entry in the next release.
func TestReleasesAreToldToOfflineDevices(t *testing.T) {
	f, store := releaseFixture()
	log := &changes{}
	f.planner.Changes, f.planner.HLC = log, hlc{}
	ctx := context.Background()
	release, next := planRelease(t, f, "2.4"), planRelease(t, f, "2.5")
	if got, ok := log.last(ReleaseEntity, release.ID); !ok || got.ContainerID != hubID || got.Payload["state"] != "PLANNED" {
		t.Fatalf("release change = %+v", got)
	}
	if _, err := (AddReleaseEntries{Planner: f.planner}).Execute(ctx, actor(), release.ID, []shared.ID{design, build}); err != nil {
		t.Fatal(err)
	}
	store.done[design] = true
	if _, err := (PublishRelease{Planner: f.planner}).Execute(ctx, actor(), PublishReleaseCommand{ReleaseID: release.ID, CarryTo: next.ID}); err != nil {
		t.Fatal(err)
	}
	if got, _ := log.last(ReleaseEntity, release.ID); got.Payload["state"] != "RELEASED" {
		t.Errorf("released change = %+v", got)
	}
	if got, _ := log.last(ReleaseEntryEntity, build); got.ContainerID != hubID || got.Payload["release_id"] != next.ID.String() {
		t.Errorf("the carried entry's change = %+v", got)
	}
	if got, _ := log.last(ReleaseEntryEntity, design); got.Payload["release_id"] != release.ID.String() {
		t.Errorf("the shipped entry's change = %+v", got)
	}
}
