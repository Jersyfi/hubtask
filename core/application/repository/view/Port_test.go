// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package view

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Find(context.Context, shared.ID) (view.SavedView, error) {
	return view.SavedView{}, shared.ErrNotFound
}
func (double) ListOwned(context.Context, shared.ID) ([]view.SavedView, error) { return nil, nil }
func (double) ListReachable(context.Context, shared.ID, []shared.ID) ([]view.SavedView, error) {
	return nil, nil
}
func (double) Insert(context.Context, view.SavedView) error             { return nil }
func (double) SetAttributes(context.Context, view.SavedView, int) error { return nil }
func (double) SetSharing(context.Context, view.SavedView, int) error    { return nil }
func (double) Delete(context.Context, view.SavedView, int) error        { return nil }

var _ SavedViews = double{}

// Two answers the use case tells apart, so both have to be expressible: a view that is not
// there, and a shelf that holds none yet.
func TestTheTwoEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(t.Context(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing view is reported as %v", err)
	}

	views, err := (double{}).ListOwned(t.Context(), "")
	if err != nil {
		t.Fatalf("an empty shelf is reported as an error: %v", err)
	}
	if len(views) != 0 {
		t.Errorf("an empty shelf answered %d views", len(views))
	}
}
