// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

const (
	// meaningMemoLife is how long a search's vector is reused: long enough for the menu's peek, the
	// first page and the pages walked after it, short enough that nothing derived from content
	// outlives the search.
	meaningMemoLife = 5 * time.Minute
	// meaningMemoSize bounds the memory; the oldest vector goes first.
	meaningMemoSize = 1024
)

type meaningKey struct {
	tenant shared.ID
	model  string
	words  string
}

type meaningHeld struct {
	vector []float32
	until  time.Time
}

// MeaningMemo holds a search's vector for a few minutes per workspace, model and words, so that
// one search embeds its words once however many requests it sends (UC-AI-07). In memory only and
// never persisted: a vector is derived from what somebody typed.
type MeaningMemo struct {
	clock clock.Clock
	mu    sync.Mutex
	held  map[meaningKey]meaningHeld
	order []meaningKey
}

// NewMeaningMemo is an empty memo on the clock.
func NewMeaningMemo(c clock.Clock) *MeaningMemo {
	return &MeaningMemo{clock: c, held: map[meaningKey]meaningHeld{}}
}

func (m *MeaningMemo) get(key meaningKey) ([]float32, bool) {
	if m == nil {
		return nil, false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	held, ok := m.held[key]
	if !ok || !m.clock.Now().Before(held.until) {
		return nil, false
	}
	return held.vector, true
}

func (m *MeaningMemo) put(key meaningKey, vector []float32) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.clock.Now()
	if _, ok := m.held[key]; !ok {
		m.order = append(m.order, key)
	}
	m.held[key] = meaningHeld{vector: vector, until: now.Add(meaningMemoLife)}
	for len(m.order) > meaningMemoSize {
		delete(m.held, m.order[0])
		m.order = m.order[1:]
	}
}
