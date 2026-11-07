// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package calendarpush is the port a calendar push writes through: one event at a time into the
// owner's own calendar at Microsoft 365 or Google (UC-INT-16).
package calendarpush

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Calendar writes and removes events. Refusals are coded: a grant the provider no longer takes is
// `calendar_push.grant_revoked`, a provider that cannot be reached or refused for now
// `calendar_push.provider_unreachable`.
type Calendar interface {
	// Put creates the event, or replaces the one remoteID names; it answers the event's identifier
	// at the provider. An event the provider no longer has is created anew.
	Put(ctx context.Context, provider integration.PushProvider, access secret.Secret, remoteID string, event integration.PushEvent) (string, error)
	// Delete removes an event; one the provider no longer has is removed already.
	Delete(ctx context.Context, provider integration.PushProvider, access secret.Secret, remoteID string) error
}
