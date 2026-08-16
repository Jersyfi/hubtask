// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package resilience

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Backoff retries an operation with exponential delays and full jitter
// (observability-reliability.md §6).
//
// It bounds nothing by itself: an attempt is as long as fn allows. Compose it with Do so that
// every attempt carries its own deadline - otherwise the first hanging attempt consumes the
// whole budget and the remaining attempts never happen.
//
// Only for idempotent operations. Retrying a create is how one request becomes two containers;
// what makes a retry safe is a dedup key or an Idempotency-Key on the operation itself, and the
// caller is the only one who knows whether it has one.
type Backoff struct {
	// Attempts is the total number of tries, the first one included. 1 means "no retry".
	Attempts int
	// Base is the delay after the first failure; each further delay doubles it.
	Base time.Duration
	// Max caps a single delay. Without it, six attempts on a base of a second put the last
	// one half a minute out.
	Max time.Duration
	// Retryable decides whether a failure is worth another attempt. Nil means Retryable below,
	// which is the right answer for every caller that has not thought about it specifically.
	Retryable func(error) bool
	// Random draws the jitter, in [0,1). Nil means crypto/rand. A test injects a fixed source
	// here; nothing else should.
	Random func() float64
}

// Do runs fn until it succeeds, until the attempts are used up, or until the error is not worth
// repeating. It returns the last error, so the caller sees why the final attempt failed rather
// than why the first one did.
func (b Backoff) Do(ctx context.Context, dependency string, fn func(context.Context) error) error {
	if b.Attempts < 1 || b.Base <= 0 || b.Max < b.Base {
		// The same reasoning as in Do: a nonsensical policy is a defect at the call site. A
		// zero base would turn the retry into a busy loop against a struggling dependency,
		// which is precisely the failure mode a backoff exists to prevent.
		return shared.Internalf("resilience: %s has an invalid backoff policy", dependency)
	}

	retryable := b.Retryable
	if retryable == nil {
		retryable = Retryable
	}

	var err error
	for attempt := 0; attempt < b.Attempts; attempt++ {
		if attempt > 0 {
			if waitErr := sleep(ctx, b.Delay(attempt)); waitErr != nil {
				// The context ended while waiting. The dependency's error is the interesting
				// one, not the fact that we stopped waiting for it.
				return err
			}
		}

		err = fn(ctx)
		if err == nil {
			return nil
		}
		if !retryable(err) {
			return err
		}
	}
	return err
}

// Delay is full jitter: a random point in [0, min(Max, Base·2^(attempt-1))], where attempt 1 is
// the delay after the first failure. The alternative - the full exponential delay for everyone -
// synchronises every client that failed at the same moment and lets them retry in the same
// moment too, which is how a recovering dependency gets knocked over a second time.
//
// Exported because a retry is not always a loop: a job that failed is rescheduled rather than
// slept on, and it needs the same policy to compute its next run.
func (b Backoff) Delay(attempt int) time.Duration {
	window := b.Base
	for i := 1; i < attempt && window < b.Max; i++ {
		window *= 2
	}
	if window > b.Max || window <= 0 {
		window = b.Max
	}

	random := b.Random
	if random == nil {
		random = randomFraction
	}
	return time.Duration(random() * float64(window))
}

// sleep waits out the delay but gives up when the context does. time.Sleep would keep a request
// waiting for a backoff that has already outlived it.
func sleep(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Retryable is the default policy: try again when the answer might differ next time.
//
// A typed error decides by its category. An error that is not typed - a driver's, a network
// stack's - is retried: at this layer an unclassified failure is almost always transport
// trouble, and the attempt count bounds how wrong that guess can be.
func Retryable(err error) bool {
	if err == nil {
		return false
	}
	// The caller went away, or the whole operation ran out of time. Either way there is nobody
	// left to succeed for.
	if errors.Is(err, context.Canceled) {
		return false
	}

	var typed *shared.Error
	if !errors.As(err, &typed) {
		return true
	}
	switch typed.Category {
	case shared.CategoryUnavailable, shared.CategoryRateLimited:
		return true
	case shared.CategoryValidation, shared.CategoryNotFound, shared.CategoryConflict,
		shared.CategoryForbidden, shared.CategoryUnauthenticated, shared.CategoryGone:
		// The same request produces the same answer. Repeating it only costs both sides.
		return false
	case shared.CategoryInternal:
		// Deliberately typed as a defect by the code that raised it. A defect does not heal
		// between two attempts.
		return false
	default:
		return false
	}
}

// randomFraction draws from crypto/rand, because math/rand is banned across the module
// (security.md §8) and the RandomSource port belongs to the core, which this adapter is not.
// A failed draw yields the full window rather than none: worse for the thundering herd, but a
// delay of zero would hammer the dependency that is already in trouble.
func randomFraction() float64 {
	var buf [8]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return 1
	}
	// 53 bits is the mantissa of a float64; taking all 64 would round and could produce 1.0.
	return float64(binary.BigEndian.Uint64(buf[:])>>11) / float64(1<<53)
}
