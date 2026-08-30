// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The port carries no logic, so there is nothing here to measure - only something to hold in
// place. The doubles prove both interfaces can still be implemented by a fake, which is what the
// use case tests depend on.
type double struct{}

func (double) Insert(context.Context, domain.Entry) error { return nil }
func (double) Find(context.Context, shared.ID) (domain.Entry, error) {
	return domain.Entry{}, shared.ErrNotFound
}
func (double) List(context.Context, Query) (Page, error)          { return Page{}, nil }
func (double) Settle(context.Context, domain.Entry) (bool, error) { return false, nil }

var _ Entries = double{}

type intakeDouble struct{}

func (intakeDouble) SetToken(context.Context, integration.InboundToken, time.Time) error {
	return nil
}
func (intakeDouble) VerifyToken(context.Context, integration.InboundToken) (bool, error) {
	return false, nil
}
func (intakeDouble) RotatedAt(context.Context) (time.Time, error) {
	return time.Time{}, shared.ErrNotFound
}

var _ Intake = intakeDouble{}

// Three answers the use cases tell apart, so all three have to be expressible: an entry that is
// not there, a settlement somebody got to first, and an intake nobody minted.
func TestTheEmptyAnswersAreDistinguishable(t *testing.T) {
	if _, err := (double{}).Find(context.Background(), ""); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a missing entry answers %v", err)
	}
	if decided, _ := (double{}).Settle(context.Background(), domain.Entry{}); decided {
		t.Error("a lost settlement reads as decided")
	}
	if _, err := (intakeDouble{}).RotatedAt(context.Background()); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an unminted intake answers %v", err)
	}
}
