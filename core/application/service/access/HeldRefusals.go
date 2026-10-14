// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package access

import (
	"context"
	"sync"
	"time"
)

// heldRecordTimeout bounds writing the held entries once the caller's transaction has ended.
const heldRecordTimeout = 10 * time.Second

// HeldRefusals carries the DENIED entries recorded under a caller's read-write transaction until
// that transaction has ended, and then writes them.
//
// Authorize's rule is to be asked before the caller's transaction. A use case that performs other
// use cases inside its own transaction - accepting a proposal applies it through the write use
// case - cannot keep it for the use cases it calls: their refusal is recorded inside the outer
// transaction, which that very refusal rolls back. Writing the entry beside it instead is no way
// out, because the outer transaction may hold the audit chain's lock and the second one would wait
// for its own caller (persistence.Writable). So the entry waits, and is written once nothing holds
// that lock.
type HeldRefusals struct {
	mu      sync.Mutex
	pending []func(context.Context)
	// owner is false for a holder taken under another one: the outer holder writes what both hold,
	// once the outermost transaction has ended.
	owner bool
}

type heldKey struct{}

// HoldRefusals answers a context under which the authoriser holds back the refusals it records,
// and the holder that writes them. The caller runs its transaction under the returned context and
// calls Record after the transaction has returned, whether it committed or not.
func HoldRefusals(ctx context.Context) (context.Context, *HeldRefusals) {
	if _, held := ctx.Value(heldKey{}).(*HeldRefusals); held {
		return ctx, &HeldRefusals{}
	}
	held := &HeldRefusals{owner: true}
	return context.WithValue(ctx, heldKey{}, held), held
}

// hold keeps write for later when ctx carries a holder, and reports whether it did.
func hold(ctx context.Context, write func(context.Context)) bool {
	held, ok := ctx.Value(heldKey{}).(*HeldRefusals)
	if !ok {
		return false
	}
	held.mu.Lock()
	defer held.mu.Unlock()
	held.pending = append(held.pending, write)
	return true
}

// Record writes the held entries, each in a transaction of its own. It is detached from the
// request's cancellation: the refusal has been answered already, and a client that hung up on it
// does not take the trail's entry with it.
func (h *HeldRefusals) Record(ctx context.Context) {
	if !h.owner {
		return
	}
	h.mu.Lock()
	pending := h.pending
	h.pending = nil
	h.mu.Unlock()
	if len(pending) == 0 {
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), heldRecordTimeout)
	defer cancel()
	for _, write := range pending {
		write(ctx)
	}
}
