// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"sync"

	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// relayBuffer is how many awareness messages a slow reader may fall behind by before the next is
// dropped for it. Awareness is the latest cursor, not a log: a dropped one is replaced by the next.
const relayBuffer = 16

// Relay hands awareness from one connection to the other connections of the same document in this
// process. Process-local: awareness is never stored, and a reader on another process sees it only
// once the document's connections share one (offline-sync.md §13, open point).
type Relay struct {
	mu     sync.Mutex
	next   int64
	joined map[relayKey]map[int64]chan []byte
}

type relayKey struct {
	tenant shared.ID
	ref    domain.Ref
}

// NewRelay answers an empty relay.
func NewRelay() *Relay { return &Relay{joined: map[relayKey]map[int64]chan []byte{}} }

// Join adds a connection to the document's readers; leave takes it out again and may be called
// more than once.
func (r *Relay) Join(tenant shared.ID, ref domain.Ref) (id int64, messages <-chan []byte, leave func()) {
	key := relayKey{tenant: tenant, ref: ref}
	inbox := make(chan []byte, relayBuffer)
	r.mu.Lock()
	r.next++
	id = r.next
	if r.joined[key] == nil {
		r.joined[key] = map[int64]chan []byte{}
	}
	r.joined[key][id] = inbox
	r.mu.Unlock()

	var once sync.Once
	return id, inbox, func() {
		once.Do(func() {
			r.mu.Lock()
			defer r.mu.Unlock()
			delete(r.joined[key], id)
			if len(r.joined[key]) == 0 {
				delete(r.joined, key)
			}
		})
	}
}

// Publish hands a message to every other reader of the document, never blocking: a reader whose
// buffer is full misses this one.
func (r *Relay) Publish(tenant shared.ID, ref domain.Ref, from int64, message []byte) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, inbox := range r.joined[relayKey{tenant: tenant, ref: ref}] {
		if id == from {
			continue
		}
		select {
		case inbox <- message:
		default:
		}
	}
}
