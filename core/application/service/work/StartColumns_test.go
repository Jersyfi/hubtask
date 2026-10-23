// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

type startedColumns struct {
	inserted []domain.Bucket
	err      error
}

func (b *startedColumns) Insert(_ context.Context, bucket domain.Bucket) error {
	if b.err != nil {
		return b.err
	}
	b.inserted = append(b.inserted, bucket)
	return nil
}

type workspaces struct {
	locale string
	err    error
}

func (w workspaces) Find(context.Context) (identity.Workspace, error) {
	return identity.Workspace{Tenant: identity.Tenant{ID: tenantID, DefaultLocale: w.locale}}, w.err
}

// Answers the locale and the code it was asked for, so a test reads which catalogue a name came from.
type echoRenderer struct{}

func (echoRenderer) Render(locale, code string, _ map[string]string) string {
	return locale + ":" + code
}

func boardOn(defaultView domain.ViewLayout) domain.StructureValues {
	return domain.StructureValues{Views: &domain.Views{
		Offered: []domain.ViewLayout{domain.ViewListCollapsed, domain.ViewKanban}, Default: defaultView,
	}}
}

func collectionUnder() domain.Container {
	return domain.Container{ID: "01a1699d-0000-7000-8000-00000000c011", TenantID: tenantID,
		Type: domain.ContainerCollection, ParentID: hubID}
}

func TestANewCollectionStartsWithItsColumnsInTheWorkspacesLanguage(t *testing.T) {
	tests := []struct {
		name      string
		values    domain.StructureValues
		wantLimit int
	}{
		{"the board as the default view limits the work in progress", boardOn(domain.ViewKanban), domain.StartingWipLimit},
		{"the board offered beside the list limits nothing", boardOn(domain.ViewListCollapsed), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			store := &startedColumns{}
			h.handler.Buckets, h.handler.Workspaces, h.handler.Renderer = store, workspaces{locale: "de"}, echoRenderer{}

			if err := h.handler.startColumns(context.Background(), collectionUnder(), actor(), tc.values, now); err != nil {
				t.Fatalf("starting the columns failed: %v", err)
			}

			if len(store.inserted) != 3 {
				t.Fatalf("%d columns, want to do, in progress and done", len(store.inserted))
			}
			if got := store.inserted[0].Name; got != "de:seed.bucket.todo" {
				t.Errorf("the first column is named %q, want the workspace's words for to do", got)
			}
			// No limit is stored as none, never as a limit of zero.
			doing := store.inserted[1]
			if tc.wantLimit == 0 && doing.WipLimit != nil || tc.wantLimit > 0 && (doing.WipLimit == nil || *doing.WipLimit != tc.wantLimit) {
				t.Errorf("in progress carries the limit %v, want %d", doing.WipLimit, tc.wantLimit)
			}
			if !store.inserted[2].IsDoneBucket {
				t.Error("the last column does not complete what enters it")
			}
			if store.inserted[0].OrderKey >= store.inserted[1].OrderKey || store.inserted[1].OrderKey >= store.inserted[2].OrderKey {
				t.Errorf("the columns are not ranked in order: %q %q %q",
					store.inserted[0].OrderKey, store.inserted[1].OrderKey, store.inserted[2].OrderKey)
			}
			if len(h.events.appended) != 3 || h.events.appended[0].Type != event.BucketCreated {
				t.Errorf("%d events, want one BucketCreated per column", len(h.events.appended))
			}
			if len(h.changes.recorded) != 3 || h.changes.recorded[0].ContainerID != collectionUnder().ID {
				t.Errorf("%d change entries, want one per column under the collection", len(h.changes.recorded))
			}
		})
	}
}

func TestACollectionWithoutABoardStartsWithNoColumn(t *testing.T) {
	h := newHarness()
	store := &startedColumns{}
	h.handler.Buckets, h.handler.Workspaces, h.handler.Renderer = store, workspaces{locale: "de"}, echoRenderer{}
	values := domain.StructureValues{Views: &domain.Views{
		Offered: []domain.ViewLayout{domain.ViewListCollapsed}, Default: domain.ViewListCollapsed,
	}}

	if err := h.handler.startColumns(context.Background(), collectionUnder(), actor(), values, now); err != nil {
		t.Fatalf("a list without a board failed: %v", err)
	}
	if len(store.inserted) != 0 || len(h.events.appended) != 0 {
		t.Errorf("a list without a board got %d columns", len(store.inserted))
	}
}

// A build without the startedColumns, the workspace or the renderer creates the collection with no column
// rather than failing it.
func TestABuildWithoutTheColumnsWiringCreatesNone(t *testing.T) {
	h := newHarness()
	if err := h.handler.startColumns(context.Background(), collectionUnder(), actor(), boardOn(domain.ViewKanban), now); err != nil {
		t.Fatalf("an unwired build failed: %v", err)
	}
	if len(h.events.appended) != 0 {
		t.Errorf("an unwired build announced %d columns", len(h.events.appended))
	}
}

func TestAFailureWhileStartingTheColumnsIsReturned(t *testing.T) {
	refused := errors.New("refused")
	tests := []struct {
		name           string
		startedColumns *startedColumns
		workspaces     workspaces
	}{
		{"the workspace cannot be read", &startedColumns{}, workspaces{err: refused}},
		{"a column cannot be written", &startedColumns{err: refused}, workspaces{locale: "en"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h := newHarness()
			h.handler.Buckets, h.handler.Workspaces, h.handler.Renderer = tc.startedColumns, tc.workspaces, echoRenderer{}
			err := h.handler.startColumns(context.Background(), collectionUnder(), actor(), boardOn(domain.ViewKanban), now)
			if !errors.Is(err, refused) {
				t.Errorf("got %v, want the failure returned so the transaction rolls back", err)
			}
		})
	}
}
