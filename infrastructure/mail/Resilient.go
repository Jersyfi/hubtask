// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"

	port "github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/infrastructure/resilience"
)

// ResilientSender composes a Sender with the A-05 blocks (ADR-0016): a breaker so a dead mail
// server costs an immediate answer instead of a blocked worker for every queued message, and a
// bulkhead so the delivery of notifications cannot take more of the process than its share.
//
// It matters more here than anywhere else that the breaker exists. A tenant with a busy morning
// has a queue full of notifications, and without a breaker each of them would spend the dial
// timeout finding out what the one before it already knew.
type ResilientSender struct {
	inner    port.Sender
	breaker  *resilience.Breaker
	bulkhead *resilience.Bulkhead
}

// NewResilientSender wraps the sender. One breaker and one bulkhead per dependency, owned by the
// composition root - the health probe reads the same breaker.
func NewResilientSender(
	inner port.Sender, breaker *resilience.Breaker, bulkhead *resilience.Bulkhead,
) ResilientSender {
	return ResilientSender{inner: inner, breaker: breaker, bulkhead: bulkhead}
}

var _ port.Sender = ResilientSender{}

// Send runs the inner sender behind the bulkhead and the breaker, in that order: a call that
// cannot get a slot must not count against the breaker, because a full pool is this process's
// state and not the mail server's.
func (r ResilientSender) Send(ctx context.Context, message port.Message) error {
	return r.bulkhead.Do(ctx, func(ctx context.Context) error {
		return r.breaker.Do(ctx, func(ctx context.Context) error {
			return r.inner.Send(ctx, message)
		})
	})
}
