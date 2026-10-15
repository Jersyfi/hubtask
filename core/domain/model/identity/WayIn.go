// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// WayInEvent is what changed about a way into an account, as the person is told of it (ADR-0078
// §6): a change nobody asked for is noticed by the one person who would notice.
type WayInEvent string

// The changes a person is told of. A first way in made by the person themselves on an account that
// had none - an invitation redeemed, an invited account activated, a newcomer provisioned - is not
// among them: there was nothing to change, and the person is the one acting.
const (
	// WayInPasswordChanged is a password changed while signed in or at the PASSWORD_CHANGE step.
	WayInPasswordChanged WayInEvent = "PASSWORD_CHANGED"
	// WayInPasswordReset is a password set through a reset link, the first one included.
	WayInPasswordReset WayInEvent = "PASSWORD_RESET"
	// WayInProviderConnected is a provider identity connected to an account that already existed.
	WayInProviderConnected WayInEvent = "PROVIDER_CONNECTED"
	// WayInProviderDisconnected is a provider identity gone because its provider was removed.
	WayInProviderDisconnected WayInEvent = "PROVIDER_DISCONNECTED"
)

// WayInChange is one change, as the notice carries it: what, when, and for a provider which one by
// identifier and by the name on its button - configuration, not anything a person wrote.
type WayInChange struct {
	Event        WayInEvent
	At           time.Time
	ProviderID   shared.ID
	ProviderName string
}
