// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) Releases(_ context.Context, after shared.ID, batch int) ([]planning.Release, error) {
	return pageAfter(s.releases, func(r planning.Release) string { return r.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) ReleaseEntries(_ context.Context, after shared.ID, batch int) ([]repository.ReleaseEntryOf, error) {
	return pageAfter(s.releaseOf, func(r repository.ReleaseEntryOf) string { return r.EntryID.String() }, after.String(), batch), nil
}

// UC-PLN-08 offline: a release and an entry's release travel to a device that may read the hub,
// and never to one that may not.
func TestTheWalkCarriesReleasesWhereTheHubIsRead(t *testing.T) {
	pull, f, store := walking(t)
	release, hiddenRelease, otherHub := idAt('8', 1), idAt('8', 2), idAt('8', 5)
	seen, hidden := idAt('8', 3), idAt('8', 4)
	store.releases = []planning.Release{
		{ID: release, HubID: hub, Name: "2.4", State: planning.ReleasePlanned, ReleaseOn: planning.DayFrom(2026, 11, 30), Version: 2},
		{ID: hiddenRelease, HubID: otherHub, Name: "Elsewhere", State: planning.ReleasePlanned},
	}
	store.releaseOf = []repository.ReleaseEntryOf{
		{EntryID: seen, ReleaseID: release, HubID: hub},
		{EntryID: hidden, ReleaseID: hiddenRelease, HubID: otherHub},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	got := map[string][]Record{}
	for _, record := range records {
		switch record.Entity {
		case "release", "release_entry":
			got[record.Entity] = append(got[record.Entity], record)
		}
	}
	if r := got["release"]; len(r) != 1 || r[0].EntityID != release || r[0].Payload["release_on"] != "2026-11-30" {
		t.Errorf("releases delivered %v, want only the readable one", r)
	}
	if m := got["release_entry"]; len(m) != 1 || m[0].EntityID != seen || m[0].Payload["release_id"] != release.String() {
		t.Errorf("memberships delivered %v, want the readable entry's", m)
	}
}
