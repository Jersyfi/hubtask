// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The double proves the interface can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Insert(context.Context, domain.CalendarFeed, domain.FeedToken) error { return nil }
func (double) FindByToken(context.Context, domain.FeedToken) (domain.CalendarFeed, error) {
	return domain.CalendarFeed{}, shared.ErrNotFound
}
func (double) Find(context.Context, shared.ID) (domain.CalendarFeed, error) {
	return domain.CalendarFeed{}, shared.ErrNotFound
}
func (double) ListForAccount(context.Context, shared.ID) ([]domain.CalendarFeed, error) {
	return nil, nil
}
func (double) Revoke(context.Context, shared.ID, time.Time) (bool, error) { return false, nil }

var _ CalendarFeeds = double{}

// Three answers the use case tells apart, so all three have to be expressible: a token that names
// nothing, a person with no feeds, and a revocation that changed nothing because it had already
// happened.
func TestTheEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).FindByToken(t.Context(), domain.FeedToken{}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unknown token is reported as %v", err)
	}

	feeds, err := (double{}).ListForAccount(t.Context(), "")
	if err != nil {
		t.Fatalf("an empty shelf is reported as an error: %v", err)
	}
	if len(feeds) != 0 {
		t.Errorf("an empty shelf answered %d feeds", len(feeds))
	}

	changed, err := (double{}).Revoke(t.Context(), "", time.Time{})
	if err != nil || changed {
		t.Errorf("revoking nothing answered (%v, %v)", changed, err)
	}
}
