// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package activity

import (
	"context"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves both halves can still be implemented by a fake, which is what the use
// case tests depend on.
type double struct{ recorded []domain.Entry }

func (d *double) Record(_ context.Context, entry domain.Entry) error {
	d.recorded = append(d.recorded, entry)
	return nil
}

func (d *double) List(context.Context, shared.ID, Page) (EntryPage, error) {
	return EntryPage{Entries: d.recorded}, nil
}

var (
	_ Journal = (*double)(nil)
	_ History = (*double)(nil)
)

// A page that is not full has no cursor: that is what lets a client stop on has_more alone rather
// than paging once more to discover the end (api-guidelines.md §4).
func TestAnExhaustedWalkCarriesNoCursor(t *testing.T) {
	journal := &double{}
	if err := journal.Record(t.Context(), domain.Entry{Verb: domain.ItemCreated}); err != nil {
		t.Fatalf("recording failed: %v", err)
	}

	page, err := journal.List(t.Context(), "", Page{Size: 50})
	if err != nil {
		t.Fatalf("reading failed: %v", err)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("%d entries, want the one recorded", len(page.Entries))
	}
	if page.Info.HasMore || page.Info.NextCursor != "" {
		t.Errorf("the walk reports %+v, want an exhausted one", page.Info)
	}
}
