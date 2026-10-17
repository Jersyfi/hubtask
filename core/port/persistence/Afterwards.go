// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package persistence

import (
	"context"
	"sync"
	"time"
)

// Afterwards holds work that has to run once the outermost transaction has ended, whatever its
// outcome - a write a refusal owes that the refused transaction cannot hold, since it rolls back
// with the refusal (known trap: a write on a refusal path), and that a second transaction beside it
// must not attempt either, since the first may hold a lock the second waits for.
//
// A helper beside the unit of work rather than a method on it: the adapter that opens the outermost
// transaction holds the work in the context and runs it when that transaction ends, and a unit of
// work that holds nothing - a test's fake - runs the work at once, which is what its caller can see.
// The access package's held refusals are the same shape, owned by one use case instead.
type Afterwards struct {
	mu      sync.Mutex
	pending []func(context.Context)
}

type afterwardsKey struct{}

// afterwardsTimeout bounds the held work as a whole. Short: it is bookkeeping a request owes, not
// the request itself, and it runs on a context detached from the request's cancellation.
const afterwardsTimeout = 10 * time.Second

// HoldAfterwards returns a context that holds work for later, and the holder - or, when the context
// holds already, the context unchanged and nil: the outer holder runs it.
func HoldAfterwards(ctx context.Context) (context.Context, *Afterwards) {
	if held, ok := ctx.Value(afterwardsKey{}).(*Afterwards); ok && held != nil {
		return ctx, nil
	}
	held := &Afterwards{}
	return context.WithValue(ctx, afterwardsKey{}, held), held
}

// After runs fn once the outermost transaction ctx belongs to has ended, or at once when nothing
// holds work for later. fn opens whatever transaction it needs: it runs outside every transaction.
func After(ctx context.Context, fn func(context.Context)) {
	held, ok := ctx.Value(afterwardsKey{}).(*Afterwards)
	if !ok || held == nil {
		fn(ctx)
		return
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	held.pending = append(held.pending, fn)
}

// Run runs the held work in the order it was held, on ctx detached from its cancellation: the
// client may have hung up on the refusal, and what the request owes is still owed. It runs before
// the request is answered, once its transaction has ended. A nil holder runs nothing.
func (a *Afterwards) Run(ctx context.Context) {
	if a == nil {
		return
	}
	a.mu.Lock()
	pending := a.pending
	a.pending = nil
	a.mu.Unlock()
	if len(pending) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), afterwardsTimeout)
	defer cancel()
	// Each runs on a context that holds nothing for later, so a transaction it opens runs its own
	// held work when it ends rather than into a holder that has already been emptied.
	ctx = context.WithValue(ctx, afterwardsKey{}, (*Afterwards)(nil))
	for _, fn := range pending {
		fn(ctx)
	}
}
