// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package stream is the machinery a long-lived connection needs, shared by every adapter that
// holds one.
//
// It exists because there is more than one: `GET /stream` serves changes to a browser, and
// `GET /mcp` serves notifications to an agent. **An agent's stream is not a
// different kind of connection from a browser's and must not have a different kind of limit** - so
// the caps, the draining and the refusal reasons live here rather than in either adapter, and a
// pod's capacity is one number whoever is holding it.
package stream

import (
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/port/health"
)

// Limits is how many streams may be open at once, at each of the three levels a long-lived
// connection has to be bounded at.
//
// Three rather than one, because they answer three different failures. Per credential stops one
// client reconnecting in a loop and holding a hundred sockets it never reads. Per tenant stops one
// busy workspace taking a whole pod's capacity from everybody else on it. Per process is the load
// shedding of observability-reliability.md §6 applied to the resource a stream actually consumes:
// above the threshold, new connections are refused with `503` and a `Retry-After` before latency
// tips over for everyone - the connections already open are never dropped to make room, because
// shedding is about not accepting more work, not about abandoning work in hand.
//
// A side connection - a sign-in snapshot, a live document - counts towards the workspace's and the
// process's limits like a stream, and on top may hold at most half of each: without that share a
// sign-in wave or a room full of boards would leave no slot for the change stream every tab needs.
// A credential's side connections are counted apart from its streams, so a tab with a board and a
// page open still gets its stream.
type Limits struct {
	PerCredential int
	PerTenant     int
	PerProcess    int
}

// capacityWarningWindow is how long a refusal at the workspace's or the process's limit keeps
// the health report's warning up after the last one.
const capacityWarningWindow = 10 * time.Minute

// Registry counts what is open and hands out the right to open one more.
//
// Process-local, and deliberately so. The `api` role is stateless and horizontally scaled, so
// there is no shared count to keep - a limit per process is a limit per pod, which is exactly the
// resource being protected. A cluster-wide cap would need coordination on the request path for a
// number nobody can act on anyway.
type Registry struct {
	limits Limits

	// mu guards everything below. Admit and the release run on request goroutines, CloseInWaves
	// on the shutdown path.
	mu            sync.Mutex
	perCredential map[string]int
	perTenant     map[string]int
	open          map[int64]*openStream
	// The side connections' own counts (Limits).
	sideCredential map[string]int
	sideTenant     map[string]int
	sideOpen       int
	// capacityRefused is the last refusal at the workspace's or the process's limit.
	capacityRefused time.Time
	nextID          int64
	// draining is set once the process has begun shutting down. A stream that asked to open after
	// that is refused rather than accepted and closed a moment later, which would look to a client
	// like a flapping server rather than one going away.
	draining bool
}

func NewRegistry(limits Limits) *Registry {
	return &Registry{
		limits:         limits,
		perCredential:  map[string]int{},
		perTenant:      map[string]int{},
		open:           map[int64]*openStream{},
		sideCredential: map[string]int{},
		sideTenant:     map[string]int{},
	}
}

// openStream is one held connection: the channel that asks it to end, and the guard that makes
// asking twice harmless. A shutdown path can run on two signals, and closing a closed channel
// panics.
type openStream struct {
	closing chan struct{}
	once    sync.Once
	// tenant and accounts are what a revocation is matched against; recheck is its doorbell.
	tenant   string
	accounts []string
	recheck  chan struct{}
}

func (s *openStream) close() { s.once.Do(func() { close(s.closing) }) }

// Slot is the right to hold one stream open.
type Slot struct {
	// Closing is closed when the process wants the stream to end. The handler selects on it beside
	// the client's own context, so a shutdown ends the connection the same way a client leaving
	// does - by returning from the handler, which is what lets the server drain (§9).
	Closing <-chan struct{}
	// Recheck rings when a credential of the stream's account or its workspace's status changed on
	// any process (migration 0332): the handler asks its credential again now rather than at its
	// next periodic recheck. Room for one; a second ring before the first is read says nothing new.
	Recheck <-chan struct{}
	release func()
}

// Release gives the slot back. Idempotent, and it must be called: a slot that outlives its
// connection is a stream this process will refuse to open for the rest of its life.
func (s Slot) Release() {
	if s.release != nil {
		s.release()
	}
}

// Refusal says which limit was reached, for the problem document and the metric. A closed
// set, because it is a label (observability-reliability.md §3.2).
type Refusal string

const (
	// RefusedNone means the slot was granted.
	RefusedNone Refusal = ""
	// RefusedCredential is one client holding too many.
	RefusedCredential Refusal = "credential"
	// RefusedTenant is one workspace holding too many.
	RefusedTenant Refusal = "tenant"
	// RefusedProcess is this pod holding too many, whoever they belong to.
	RefusedProcess Refusal = "process"
	// RefusedDraining is the process shutting down.
	RefusedDraining Refusal = "draining"
)

func (r Refusal) String() string { return string(r) }

// Admit grants the right to open one stream, or says which limit refused it.
//
// The keys are opaque to this type: whoever calls it decides what "one credential" means, and it
// is a fingerprint rather than the credential itself - the same reasoning that keeps the rate
// limiter's bucket key a hash (security.md §9, rule 10). The accounts are those whose revocation
// must reach the stream: the actor's, and a viewer's where somebody views as the actor.
func (r *Registry) Admit(credential, tenant string, accounts ...string) (Slot, Refusal) {
	return r.admit(false, credential, tenant, accounts)
}

// AdmitSide grants the right to hold one side connection - a snapshot, a live document - within
// the share Limits leaves them.
func (r *Registry) AdmitSide(credential, tenant string, accounts ...string) (Slot, Refusal) {
	return r.admit(true, credential, tenant, accounts)
}

func (r *Registry) admit(side bool, credential, tenant string, accounts []string) (Slot, Refusal) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if refusal := r.refusal(side, credential, tenant); refusal != RefusedNone {
		if refusal == RefusedTenant || refusal == RefusedProcess {
			r.capacityRefused = time.Now()
		}
		return Slot{}, refusal
	}

	r.nextID++
	id := r.nextID
	held := &openStream{
		closing: make(chan struct{}), tenant: tenant, accounts: accounts, recheck: make(chan struct{}, 1),
	}
	r.open[id] = held
	if tenant != "" {
		r.perTenant[tenant]++
	}
	byCredential := r.perCredential
	if side {
		byCredential = r.sideCredential
		r.sideOpen++
		if tenant != "" {
			r.sideTenant[tenant]++
		}
	}
	if credential != "" {
		byCredential[credential]++
	}

	var once sync.Once
	return Slot{
		Closing: held.closing,
		Recheck: held.recheck,
		release: func() {
			once.Do(func() {
				r.mu.Lock()
				defer r.mu.Unlock()
				delete(r.open, id)
				// The counters go back to absent rather than to zero. A process that served one
				// stream for every tenant it has ever seen would otherwise keep a map entry per
				// tenant for the life of the pod.
				decrement(r.perTenant, tenant)
				decrement(byCredential, credential)
				if side {
					r.sideOpen--
					decrement(r.sideTenant, tenant)
				}
			})
		},
	}, RefusedNone
}

// refusal answers which limit a new connection would pass, RefusedNone when none.
func (r *Registry) refusal(side bool, credential, tenant string) Refusal {
	l := r.limits
	byCredential := r.perCredential
	if side {
		byCredential = r.sideCredential
	}
	switch {
	case r.draining:
		return RefusedDraining
	case l.PerProcess > 0 && len(r.open) >= l.PerProcess,
		side && l.PerProcess > 0 && r.sideOpen >= half(l.PerProcess):
		return RefusedProcess
	case l.PerTenant > 0 && tenant != "" && r.perTenant[tenant] >= l.PerTenant,
		side && l.PerTenant > 0 && tenant != "" && r.sideTenant[tenant] >= half(l.PerTenant):
		return RefusedTenant
	case l.PerCredential > 0 && credential != "" && byCredential[credential] >= l.PerCredential:
		return RefusedCredential
	}
	return RefusedNone
}

func half(limit int) int { return max(1, limit/2) }

// Warnings is the health report's hint that connections were refused at the workspace's or the
// process's limit in the last ten minutes: the server is fine, and clients wait without live
// updates until a slot frees.
func (r *Registry) Warnings(context.Context) []health.Warning {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.capacityRefused.IsZero() || time.Since(r.capacityRefused) > capacityWarningWindow {
		return nil
	}
	return []health.Warning{{
		Code: "stream.capacity_reached", Severity: "warn",
		Params: map[string]string{
			"per_tenant": strconv.Itoa(r.limits.PerTenant), "per_process": strconv.Itoa(r.limits.PerProcess),
			"tenant_variable": "HUBTASK_STREAM_PER_TENANT", "process_variable": "HUBTASK_STREAM_PER_PROCESS",
		},
	}}
}

// Open is how many streams this process is holding. The gauge (§3.2), and what the health report
// reads.
func (r *Registry) Open() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.open)
}

// CloseAll asks every open stream to end at once, and refuses new ones from now on. It is
// CloseInWaves with no window.
func (r *Registry) CloseAll() { r.CloseInWaves(0) }

// drainWaves is how many waves CloseInWaves splits the open streams into.
const drainWaves = 10

// CloseInWaves refuses new streams from now on and asks the open ones to end in waves spread over
// window, each wave at a random moment within its share of it. It returns after the last wave.
//
// Called on `SIGTERM`, before the HTTP server is asked to shut down. Without it `Shutdown` would
// wait out the whole grace period on connections that are working exactly as designed - a stream
// has no natural end, so nothing but this tells it there is one (deployment.md §5). In waves, not
// at once: every client of a pod reconnecting to the remaining pods in the same second is the
// thundering herd a rolling update would otherwise cause on each pod it replaces.
//
// It does not wait for the handlers. They return on their own and the server's own drain is what
// waits for them, which is the one place that already knows how long it may.
func (r *Registry) CloseInWaves(window time.Duration) {
	r.closeInWaves(window, func(n time.Duration) time.Duration { return rand.N(n) }, time.Sleep)
}

func (r *Registry) closeInWaves(window time.Duration, jitter func(time.Duration) time.Duration, sleep func(time.Duration)) {
	r.mu.Lock()
	r.draining = true
	held := make([]*openStream, 0, len(r.open))
	for _, s := range r.open {
		held = append(held, s)
	}
	r.mu.Unlock()
	// The map is not emptied, and that is the point: a stream stays counted until its handler has
	// actually returned and released its slot. A gauge that dropped to zero on the signal would say
	// the drain was over while the connections were still finishing, which is precisely the number
	// an operator watches during a rollout.

	elapsed := time.Duration(0)
	for _, wave := range drainSchedule(held, window, jitter) {
		if wave.at > elapsed {
			sleep(wave.at - elapsed)
			elapsed = wave.at
		}
		for _, s := range wave.streams {
			s.close()
		}
	}
}

type drainWave struct {
	at      time.Duration
	streams []*openStream
}

// drainSchedule cuts the streams into at most drainWaves waves of near-equal size. Wave i falls at
// i·step plus a jitter below step, where step is window/waves, so every wave lands inside the
// window and in order. No window, or one stream, is a single wave now.
func drainSchedule(held []*openStream, window time.Duration, jitter func(time.Duration) time.Duration) []drainWave {
	if len(held) == 0 {
		return nil
	}
	waves := min(drainWaves, len(held))
	if window <= 0 || waves == 1 {
		return []drainWave{{streams: held}}
	}
	step := window / time.Duration(waves)
	out := make([]drainWave, 0, waves)
	for i := range waves {
		from, to := i*len(held)/waves, (i+1)*len(held)/waves
		at := time.Duration(i) * step
		if step > 0 {
			at += jitter(step)
		}
		out = append(out, drainWave{at: at, streams: held[from:to]})
	}
	return out
}

// Recheck rings the streams of the workspace's account, or every stream of the workspace when the
// account is empty. It never blocks: a ring already unread is enough.
func (r *Registry) Recheck(tenant, account string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, held := range r.open {
		if held.tenant != tenant || (account != "" && !slices.Contains(held.accounts, account)) {
			continue
		}
		select {
		case held.recheck <- struct{}{}:
		default:
		}
	}
}

// Draining reports whether the process has begun shutting down.
func (r *Registry) Draining() bool {
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
