// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
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

// The two G-03 ports, held in place the same way. A signature change that breaks the use case
// tests breaks this first, with a clearer message.
type subscriptions struct{}

func (subscriptions) Insert(context.Context, StoredSubscription) error { return nil }
func (subscriptions) Find(context.Context, shared.ID) (StoredSubscription, error) {
	return StoredSubscription{}, shared.ErrNotFound
}
func (subscriptions) List(context.Context) ([]domain.WebhookSubscription, error) { return nil, nil }
func (subscriptions) WantingEvent(context.Context, event.Type) ([]StoredSubscription, error) {
	return nil, nil
}
func (subscriptions) Update(context.Context, domain.WebhookSubscription, int) (bool, error) {
	return false, nil
}
func (subscriptions) Rotate(context.Context, shared.ID, SealedSecret, time.Time, int) (bool, error) {
	return false, nil
}
func (subscriptions) Delete(context.Context, shared.ID) (bool, error) { return false, nil }

type deliveries struct{}

func (deliveries) Insert(context.Context, domain.WebhookDelivery) error { return nil }
func (deliveries) Find(context.Context, shared.ID) (domain.WebhookDelivery, error) {
	return domain.WebhookDelivery{}, shared.ErrNotFound
}
func (deliveries) List(context.Context, DeliveryQuery) ([]domain.WebhookDelivery, error) {
	return nil, nil
}
func (deliveries) RecordOutcome(context.Context, DeliveryOutcome) error { return nil }

var (
	_ WebhookSubscriptions = subscriptions{}
	_ WebhookDeliveries    = deliveries{}
)

// A sealed value is a pair, and the zero value has to say "there is none" - a subscription that
// has never been rotated carries an empty previous secret, and the sweep of a rotation that
// retired its grace deliberately carries one too.
func TestTheZeroSealedSecretIsRecognisable(t *testing.T) {
	if !(SealedSecret{}).IsZero() {
		t.Error("the zero sealed secret does not report itself empty")
	}
	if (SealedSecret{Ciphertext: []byte("x")}).IsZero() {
		t.Error("a ciphertext without a key identifier reports itself empty")
	}
	if (SealedSecret{KeyID: "k1"}).IsZero() {
		t.Error("a key identifier without a ciphertext reports itself empty")
	}
}
