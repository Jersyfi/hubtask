// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PlatformState is where an installation subscription stands. There is no `DISABLED`: unlike a
// workspace's webhook, a purchase platform's subscription is never switched off for being
// unreachable (multi-tenancy.md §4.3) - its dead letters wait, and the operator is told.
type PlatformState string

const (
	PlatformActive PlatformState = "ACTIVE"
	PlatformPaused PlatformState = "PAUSED"
)

// PlatformSubscription is an address every installation event is pushed to (UC-INS-14). It takes
// every installation event, with no type list, filter or scope: the platform keeps the workspaces'
// whole state.
type PlatformSubscription struct {
	ID        shared.ID
	TargetURL string
	State     PlatformState
	// LastError is the message code of the last failed attempt, empty after an arrival.
	LastError      string
	LastDeliveryAt time.Time
	// DeadLetters are the deliveries that ended without arriving and wait to be redelivered.
	DeadLetters int64
	// FromFile marks one the instance file declares: changed there, not through the API.
	FromFile  bool
	CreatedAt time.Time
	CreatedBy shared.ID
	Version   int
}

// NewPlatformSubscription builds one, its target checked as a webhook's is. Whether the target
// may be dialled is the guard's question at each delivery.
func NewPlatformSubscription(id shared.ID, target string, fromFile bool, createdBy shared.ID, now time.Time,
) (PlatformSubscription, error) {
	if id.IsZero() {
		return PlatformSubscription{}, shared.ErrInternal.WithDetail("admin.subscription_incomplete")
	}
	checked, err := TargetURL(target)
	if err != nil {
		return PlatformSubscription{}, err
	}
	return PlatformSubscription{
		ID: id, TargetURL: checked, State: PlatformActive, FromFile: fromFile,
		CreatedAt: now.UTC(), CreatedBy: createdBy, Version: 1,
	}, nil
}

// Retargeted is the subscription sent somewhere else.
func (s PlatformSubscription) Retargeted(target string) (PlatformSubscription, error) {
	checked, err := TargetURL(target)
	if err != nil {
		return PlatformSubscription{}, err
	}
	s.TargetURL = checked
	return s, nil
}

// InState is the subscription paused or resumed. Only the two states exist, and both are set by
// hand.
func (s PlatformSubscription) InState(state PlatformState) (PlatformSubscription, error) {
	if state != PlatformActive && state != PlatformPaused {
		return PlatformSubscription{}, fieldError("/state", "admin.subscription_state_invalid")
	}
	s.State = state
	return s, nil
}

// PlatformSecretPurposeFor is what a platform subscription's sealed secret is bound to, as
// SecretPurposeFor binds a webhook's.
func PlatformSecretPurposeFor(id shared.ID) string {
	return "platform_subscription.secret:" + id.String()
}
