// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func (s *snapshotStore) Portfolios(_ context.Context, after shared.ID, batch int) ([]goal.Portfolio, error) {
	return pageAfter(s.portfolios, func(p goal.Portfolio) string { return p.ID.String() }, after.String(), batch), nil
}

func projectsOf(t *testing.T, payload map[string]any) []string {
	t.Helper()
	items, _ := payload["items"].([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.(map[string]any)["container_id"].(string))
	}
	return out
}

// UC-GOL-03 offline: the walk hands a portfolio to a reader of the workspace with the projects
// they may read alone, as ReadPortfolio answers it, and to nobody who may not read there.
func TestTheWalkNarrowsAPortfoliosProjectsToTheReader(t *testing.T) {
	pull, f, store := walking(t)
	f.auth.workspace = true
	store.portfolios = []goal.Portfolio{{
		ID: idAt('f', 1), Name: "Programme", Status: goal.StatusOnTrack,
		Items: []goal.PortfolioItem{{ContainerID: readable}, {ContainerID: hidden}},
	}}
	records, _ := wholeWalk(t, pull, 100)
	var found map[string]any
	for _, record := range records {
		if record.Entity == entityPortfolio {
			found = record.Payload
		}
	}
	if found == nil {
		t.Fatal("the walk handed out no portfolio")
	}
	if got := projectsOf(t, found); len(got) != 1 || got[0] != readable.String() {
		t.Errorf("the projects = %v, want the readable one alone", got)
	}
	if items := store.portfolios[0].Items; len(items) != 2 {
		t.Error("narrowing changed the store's portfolio")
	}

	f.auth.workspace = false
	records, _ = wholeWalk(t, pull, 100)
	for _, record := range records {
		if record.Entity == entityPortfolio {
			t.Errorf("a portfolio reached a reader who may not read the workspace: %+v", record)
		}
	}
}

// The delta delivers a portfolio narrowed the same way, a removal as it is.
func TestTheDeltaNarrowsAPortfoliosProjectsToTheReader(t *testing.T) {
	upsert := entry(1, "")
	upsert.Entity, upsert.Payload = entityPortfolio, map[string]any{
		"id": idAt('f', 1).String(), "name": "Programme",
		"items": []any{map[string]any{"container_id": hidden.String()}, map[string]any{"container_id": readable.String()}},
	}
	removal := entry(2, "")
	removal.Entity, removal.Op = entityPortfolio, "DELETE"
	pull, f := pulling(t, upsert, removal)
	f.auth.workspace = true

	page, err := pull.Pull(t.Context(), actor(), PullRequest{DeviceID: device, Cursor: cursorAt(f, 0), Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Records) != 2 {
		t.Fatalf("the delta delivered %d records, want both", len(page.Records))
	}
	if got := projectsOf(t, page.Records[0].Payload); len(got) != 1 || got[0] != readable.String() {
		t.Errorf("the projects = %v, want the readable one alone", got)
	}
}
