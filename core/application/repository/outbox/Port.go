// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package outbox declares how a domain event leaves the write path.
//
// A transactional outbox (ADR-0007): the event is written to a table in the same transaction as
// the change it describes, and a dispatcher delivers it afterwards. That is what makes "no change
// without its event, and no event without its change" true rather than hoped for - calling a
// webhook from inside the request would leave one of the two half done on any failure.
//
// It is a repository port rather than a bus port for the same reason: what happens here is an
// insert inside somebody else's transaction. The publish/subscribe side - the dispatcher, the
// retries, the dead letter - arrives with A-08 behind core/port/eventbus.
package outbox

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/event"
)

// Events records what happened, for delivery later.
type Events interface {
	// Append writes the event inside the caller's transaction. It fails the transaction rather
	// than swallowing the error: an event that was dropped quietly is a change automation and
	// webhooks never hear about, and nothing later can tell that it is missing.
	Append(ctx context.Context, envelope event.Envelope) error
}
