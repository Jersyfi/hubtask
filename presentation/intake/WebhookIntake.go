// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package intake is the inbound adapter for content that arrives from outside without a person
// behind it (G-10): the jumble's doors. The webhook intake is here; the mail intake joins it
// (G-11). An intake differs from the REST API in who is asking - a bridge with a token, not an
// account with a role - and in nothing else: everything it stores goes through the same
// application services and the same bounds.
package intake

import (
	"context"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// WebhookIntake turns one posted delivery into a jumble entry (G-10, automation.md §1.1's
// credential discipline applied to the inbox).
//
// It authenticates the tenant, never a person: the token is the whole credential, and every
// reason not to serve answers the same not-found - a malformed token included, because "that is
// not the right shape" tells somebody guessing that the shape is what to fix (T-21).
type WebhookIntake struct {
	Deliveries jumbleservice.IntakeJumbleEntry
}

// Deliver parses the credential and stores the delivery.
func (i WebhookIntake) Deliver(
	ctx context.Context, presented, sender, subject, body string,
) (domain.Entry, error) {
	token, err := integration.ParseInboundToken(presented)
	if err != nil {
		return domain.Entry{}, shared.ErrNotFound.WithDetail("jumble.inbound_not_found")
	}
	return i.Deliveries.Execute(ctx, jumbleservice.IntakeDelivery{
		Token: token, Sender: sender, Subject: subject, Body: body,
	})
}
