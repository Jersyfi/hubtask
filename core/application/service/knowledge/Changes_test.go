// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type changeLog struct{ changes []changelog.Change }

func (l *changeLog) Record(_ context.Context, change changelog.Change) error {
	l.changes = append(l.changes, change)
	return nil
}

type hlcs struct{ reading shared.HLC }

func (h *hlcs) Next() shared.HLC {
	next, _ := h.reading.Tick(start, "server")
	h.reading = next
	return next
}

// A page is read offline (M3, offline-sync.md §4.3): every write of a page tells the change log
// the page as it now stands, body included and filed under its hub; a trash tells it of every page
// that went with it.
func TestEveryWriteOfAPageReachesTheChangeLog(t *testing.T) {
	f := newFixture()
	log := &changeLog{}
	f.space.Changes, f.space.HLC = log, &hlcs{}
	parent := f.create(t, anna, hub, "", "Handbook", "Travel")
	child := f.create(t, anna, hub, parent.ID, "Trains", "Book early")
	if _, err := (UpdatePage{Space: f.space}).Execute(context.Background(), as(anna), UpdatePageCommand{
		PageID: child.ID, Change: domain.Change{Body: ptr("Book a week early")}, ExpectedVersion: child.Version,
	}); err != nil {
		t.Fatalf("UpdatePage: %v", err)
	}
	if len(log.changes) != 3 {
		t.Fatalf("%d changes after two pages and an edit, want 3", len(log.changes))
	}
	edited := log.changes[2]
	if edited.Entity != "page" || edited.EntityID != child.ID || edited.ContainerID != hub ||
		edited.Op != changelog.Upsert || edited.Payload["body"] != "Book a week early" {
		t.Errorf("the edit was recorded as %+v", edited)
	}

	if _, err := (TrashPage{Space: f.space}).Execute(context.Background(), as(anna), parent.ID); err != nil {
		t.Fatalf("TrashPage: %v", err)
	}
	trashed := map[shared.ID]bool{}
	for _, change := range log.changes[3:] {
		if change.Payload["trashed_at"] != nil {
			trashed[change.EntityID] = true
		}
	}
	if !trashed[parent.ID] || !trashed[child.ID] {
		t.Errorf("the trash told the log of %v, want both pages", trashed)
	}
}
