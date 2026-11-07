// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"sync"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// slots bounds how many jobs of one runner run at once. Only the runner's loop takes a slot, so a
// count read by it stays a lower bound until it takes them.
type slots struct {
	tokens  chan struct{}
	running sync.WaitGroup
}

func newSlots(n int) *slots {
	s := &slots{tokens: make(chan struct{}, n)}
	for range n {
		s.tokens <- struct{}{}
	}
	return s
}

func (s *slots) size() int { return cap(s.tokens) }

func (s *slots) free() int { return len(s.tokens) }

// awaitFree waits until a slot is free, and reports false when the context ends first.
func (s *slots) awaitFree(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case token := <-s.tokens:
		s.tokens <- token
		return true
	}
}

// run takes a slot and runs fn in it under SafeGo (rule 5); the slot is given back when fn ends,
// a panic included.
func (s *slots) run(ctx context.Context, component string, fn func(context.Context)) {
	<-s.tokens
	s.running.Add(1)
	concurrency.Go(ctx, component, func(ctx context.Context) {
		defer func() {
			s.tokens <- struct{}{}
			s.running.Done()
		}()
		fn(ctx)
	})
}

// drain waits for every running job.
func (s *slots) drain() { s.running.Wait() }
