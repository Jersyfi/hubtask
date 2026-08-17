// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package clock

import (
	"sync"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/clock"
)

// HybridClock stamps server-side changes with a hybrid logical clock (offline-sync.md §4.1).
//
// One instance per process, because the state it keeps is what makes two changes in the same
// millisecond orderable: the counter advances when the wall clock has not. Two instances in one
// process would hand out the same reading twice and the two changes would then be ordered
// differently on different clients - the one failure a merge rule must not have.
type HybridClock struct {
	clock  port.Clock
	device string

	mu   sync.Mutex
	last shared.HLC
}

// NewHybridClock refuses a device identifier that cannot be read back out of the stored form, so
// that Next never has to fail.
func NewHybridClock(clock port.Clock, device string) (*HybridClock, error) {
	if _, err := shared.NewHLC(clock.Now(), 0, device); err != nil {
		return nil, err
	}
	return &HybridClock{clock: clock, device: device}, nil
}

var _ port.HLCSource = (*HybridClock)(nil)

// Next returns the reading for the change being written now. Safe for concurrent use: two
// requests stamping at once get two readings, and the second one sorts after the first.
func (h *HybridClock) Next() shared.HLC {
	h.mu.Lock()
	defer h.mu.Unlock()

	next, err := h.last.Tick(h.clock.Now(), h.device)
	if err != nil {
		// Unreachable: Tick can only report a malformed device, which the constructor refused,
		// and an exhausted counter, which it resolves by borrowing a millisecond. Returning the
		// previous reading keeps the ordering monotonic if it ever does happen - a repeated
		// reading is wrong, a smaller one would be worse.
		return h.last
	}
	h.last = next
	return next
}
