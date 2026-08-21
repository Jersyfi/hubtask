// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package idempotency declares what the idempotency guard needs from storage.
package idempotency

import "context"

// Key identifies one attempt: the client's key, scoped to the operation it was sent to.
//
// The endpoint is part of the identity on purpose. The same key sent to two different operations
// is two different attempts, and folding them together would replay a container's response to a
// request that asked for an item.
type Key struct {
	Key      string
	Endpoint string
}

// Record is what a previous attempt left behind.
type Record struct {
	// RequestHash is what the first attempt was made of. A repeat that hashes differently is the
	// same key used for a different request, which is a client bug worth reporting rather than
	// silently answering (api-guidelines.md §5).
	RequestHash []byte
	// Status is zero while the first attempt is still running.
	Status int
	Body   []byte
}

// InProgress reports an attempt that was reserved and has not finished. A second request arriving
// during that window is not a repeat to answer - it is a race, and answering it from an empty
// record would invent a response.
func (r Record) InProgress() bool { return r.Status == 0 }

// Store keeps the answers of completed attempts.
//
// Every method runs inside the caller's transaction and inside its tenant; the rows carry a
// tenant_id and row level security applies it, so nothing here takes one as an argument
// (ADR-0010).
type Store interface {
	// Reserve claims the key for a first attempt. It returns the existing record and false when
	// somebody claimed it first - the two happen in one round trip on the common path, which is
	// the path where the key is new.
	Reserve(ctx context.Context, key Key, requestHash []byte) (Record, bool, error)

	// Complete stores the answer of a finished attempt.
	Complete(ctx context.Context, key Key, status int, body []byte) error
}
