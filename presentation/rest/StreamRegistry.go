// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"sync"
)

// StreamLimits is how many streams may be open at once, at each of the three levels a long-lived
// connection has to be bounded at.
//
// Three rather than one, because they answer three different failures. Per credential stops one
// client reconnecting in a loop and holding a hundred sockets it never reads. Per tenant stops one
// busy workspace taking a whole pod's capacity from everybody else on it. Per process is the load
// shedding of observability-reliability.md §6 applied to the resource a stream actually consumes:
// above the threshold, new connections are refused with `503` and a `Retry-After` before latency
// tips over for everyone - the connections already open are never dropped to make room, because
// shedding is about not accepting more work, not about abandoning work in hand.
type StreamLimits struct {
	PerCredential int
	PerTenant     int
	PerProcess    int
}

// StreamRegistry counts what is open and hands out the right to open one more.
//
// Process-local, and deliberately so. The `api` role is stateless and horizontally scaled, so
// there is no shared count to keep - a limit per process is a limit per pod, which is exactly the
// resource being protected. A cluster-wide cap would need coordination on the request path for a
// number nobody can act on anyway.
type StreamRegistry struct {
	limits StreamLimits

	// mu guards everything below. Admit and the release run on request goroutines, CloseAll on
	// the shutdown path.
	mu            sync.Mutex
	perCredential map[string]int
	perTenant     map[string]int
	open          map[int64]*openStream
	nextID        int64
	// draining is set once the process has begun shutting down. A stream that asked to open after
	// that is refused rather than accepted and closed a moment later, which would look to a client
	// like a flapping server rather than one going away.
	draining bool
}

func NewStreamRegistry(limits StreamLimits) *StreamRegistry {
	return &StreamRegistry{
		limits:        limits,
		perCredential: map[string]int{},
		perTenant:     map[string]int{},
		open:          map[int64]*openStream{},
	}
}

// openStream is one held connection: the channel that asks it to end, and the guard that makes
// asking twice harmless. A shutdown path can run on two signals, and closing a closed channel
// panics.
type openStream struct {
	closing chan struct{}
	once    sync.Once
}

func (s *openStream) close() { s.once.Do(func() { close(s.closing) }) }

// StreamSlot is the right to hold one stream open.
type StreamSlot struct {
	// Closing is closed when the process wants the stream to end. The handler selects on it beside
	// the client's own context, so a shutdown ends the connection the same way a client leaving
	// does - by returning from the handler, which is what lets the server drain (§9).
	Closing <-chan struct{}
	release func()
}

// Release gives the slot back. Idempotent, and it must be called: a slot that outlives its
// connection is a stream this process will refuse to open for the rest of its life.
func (s StreamSlot) Release() {
	if s.release != nil {
		s.release()
	}
}

// StreamRefusal says which limit was reached, for the problem document and the metric. A closed
// set, because it is a label (observability-reliability.md §3.2).
type StreamRefusal string

const (
	// RefusedNone means the slot was granted.
	RefusedNone StreamRefusal = ""
	// RefusedCredential is one client holding too many.
	RefusedCredential StreamRefusal = "credential"
	// RefusedTenant is one workspace holding too many.
	RefusedTenant StreamRefusal = "tenant"
	// RefusedProcess is this pod holding too many, whoever they belong to.
	RefusedProcess StreamRefusal = "process"
	// RefusedDraining is the process shutting down.
	RefusedDraining StreamRefusal = "draining"
)

func (r StreamRefusal) String() string { return string(r) }

// Admit grants the right to open one stream, or says which limit refused it.
//
// The keys are opaque to this type: whoever calls it decides what "one credential" means, and it
// is a fingerprint rather than the credential itself - the same reasoning that keeps the rate
// limiter's bucket key a hash (security.md §9, rule 10).
func (r *StreamRegistry) Admit(credential, tenant string) (StreamSlot, StreamRefusal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch {
	case r.draining:
		return StreamSlot{}, RefusedDraining
	case r.limits.PerProcess > 0 && len(r.open) >= r.limits.PerProcess:
		return StreamSlot{}, RefusedProcess
	case r.limits.PerTenant > 0 && tenant != "" && r.perTenant[tenant] >= r.limits.PerTenant:
		return StreamSlot{}, RefusedTenant
	case r.limits.PerCredential > 0 && credential != "" &&
		r.perCredential[credential] >= r.limits.PerCredential:
		return StreamSlot{}, RefusedCredential
	}

	r.nextID++
	id := r.nextID
	held := &openStream{closing: make(chan struct{})}
	r.open[id] = held
	if tenant != "" {
		r.perTenant[tenant]++
	}
	if credential != "" {
		r.perCredential[credential]++
	}

	var once sync.Once
	return StreamSlot{
		Closing: held.closing,
		release: func() {
			once.Do(func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				delete(r.open, id)
				// The counters go back to absent rather than to zero. A process that served one
				// stream for every tenant it has ever seen would otherwise keep a map entry per
				// tenant for the life of the pod.
				decrement(r.perTenant, tenant)
				decrement(r.perCredential, credential)
			})
		},
	}, RefusedNone
}

// Open is how many streams this process is holding. The gauge (§3.2), and what the health report
// reads.
func (r *StreamRegistry) Open() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.open)
}

// CloseAll asks every open stream to end, and refuses new ones from now on.
//
// Called on `SIGTERM`, before the HTTP server is asked to shut down. Without it `Shutdown` would
// wait out the whole grace period on connections that are working exactly as designed - a stream
// has no natural end, so nothing but this tells it there is one (observability-reliability.md §9).
//
// It does not wait. The handlers return on their own and the server's own drain is what waits for
// them, which is the one place that already knows how long it may.
func (r *StreamRegistry) CloseAll() {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.draining = true
	for _, held := range r.open {
		held.close()
	}
	// The map is not emptied here, and that is the point: a stream stays counted until its handler
	// has actually returned and released its slot. A gauge that dropped to zero the moment the
	// signal arrived would say the drain was over while the connections were still finishing,
	// which is precisely the number an operator watches during a rollout.
}

// Draining reports whether the process has begun shutting down.
func (r *StreamRegistry) Draining() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.draining
}

func decrement(counts map[string]int, key string) {
	if key == "" {
		return
	}
	counts[key]--
	if counts[key] <= 0 {
		delete(counts, key)
	}
}
