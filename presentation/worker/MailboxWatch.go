// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// MailboxHolds is the application's side of the watch: which mailboxes live, and one hold of one.
type MailboxHolds interface {
	Targets(ctx context.Context, shard queue.Shard, limit int) ([]mailservice.WatchTarget, error)
	// Wanted is how many of the shard's mailboxes' reads live, beyond the limit.
	Wanted(ctx context.Context, shard queue.Shard) (int64, error)
	Hold(ctx context.Context, target mailservice.WatchTarget, hold time.Duration) (string, error)
}

// MailboxWatchSignals is the slice of the metrics adapter the watch uses.
type MailboxWatchSignals interface {
	// MailboxWatchesHeld is how many of the shard's mailboxes this process holds open now.
	MailboxWatchesHeld(ctx context.Context, shard int, held int64)
	// MailboxWatchesUnheld is how many of the shard's mailboxes the limit leaves to the minute's
	// read - A-38's number: those workspaces lose the arrival in seconds, and nothing else says so.
	MailboxWatchesUnheld(ctx context.Context, shard int, unheld int64)
	// MailboxWatchShards is how many shards the watch is split into and how many this process
	// leads: a shard no process leads is A-38's too.
	MailboxWatchShards(ctx context.Context, shards, led int64)
	// MailboxWatchEnded counts one hold by what it came to (mailservice.Watch*).
	MailboxWatchEnded(ctx context.Context, outcome string)
}

const (
	mailboxWatchTick  = time.Minute
	mailboxWatchHold  = 25 * time.Minute
	mailboxWatchLimit = 1000
	// mailboxWatchRest bounds how often an arrival wakes a read: a server that announces mail on
	// every hold costs a read every few seconds, not a loop.
	mailboxWatchRest = 5 * time.Second
	// mailboxWatchRetry is when a mailbox that cannot be held (no IDLE, not IMAP) is tried again.
	mailboxWatchRetry   = time.Hour
	mailboxWatchBackoff = 5 * time.Second
	mailboxWatchCeiling = 5 * time.Minute
	// mailboxWatchRelease bounds the wait for holds to end once let go: a hold that does not hear
	// its cancellation is logged and left to its connection's deadline rather than stopping the
	// loop, a lost lock or a shutdown for up to a hold.
	mailboxWatchRelease = 10 * time.Second
	// mailboxWatchPace is the gap between two sign-ins the watch starts: 1,000 mailboxes take
	// 50 seconds rather than one, so a provider that hosts many of them is not asked for hundreds
	// of logins at once - the burst it throttles the source address for, minute's read included.
	mailboxWatchPace = 50 * time.Millisecond
)

// MailboxWatch holds the connected IMAP mailboxes of one shard of the workspaces open (IDLE) so mail
// reaches the desk in seconds (UC-JUM-11 check 10). One process of the installation holds a shard,
// by an advisory lock of the shard's own - two would sign every mailbox in twice and providers
// count connections; Seats bound how many shards one process leads, so they spread over workers. Everything it
// does is an acceleration: a hold that fails, a lost lock or a stopped process leave the minute's
// read, which never depends on it. Between a lost lock and the former holder's next tick both may
// hold a mailbox; that costs a connection, never a message, since a wake is the read's dedupe key.
// The same cost is why a lock that cannot be asked keeps the holds: a database failover of a
// second must not drop every connection and sign them all in again; only a lock known lost, or
// unknown for longer than a hold, lets go.
type MailboxWatch struct {
	Holds      MailboxHolds
	Leadership queue.Leadership
	Signals    MailboxWatchSignals
	// Shard is the part of the workspaces this watch holds; the zero shard is all of them.
	Shard queue.Shard
	// Seats, shared by a process's watches, bound how many shards it leads. Nil: no bound.
	Seats *MailboxWatchSeats
	// SchemaBehind stands the watch by while the database lacks migrations this build needs.
	SchemaBehind func(ctx context.Context) bool
	// Tick, Hold, Limit, Rest, Retry, Backoff, Ceiling, Release and Pace default to the constants
	// above; tests shorten them.
	Tick, Hold, Rest, Retry, Backoff, Ceiling, Release, Pace time.Duration
	Limit                                                    int
	// unsure is shared by the copies withDefaults hands out: since when the lock could not be asked.
	unsure *time.Time
	// seated, shared the same way, says this watch holds one of the Seats.
	seated *bool
}

// MailboxWatchSeats bound how many shards' locks one process leads at once: without a bound the
// first worker to start would take every shard, and a second would only stand by.
type MailboxWatchSeats struct {
	mu          sync.Mutex
	most, taken int
}

// NewMailboxWatchSeats bounds a process to most shards; most below one is one.
func NewMailboxWatchSeats(most int) *MailboxWatchSeats {
	return &MailboxWatchSeats{most: max(most, 1)}
}

func (s *MailboxWatchSeats) take() bool {
	if s == nil {
		return true
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.taken >= s.most {
		return false
	}
	s.taken++
	return true
}

func (s *MailboxWatchSeats) give() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.taken = max(s.taken-1, 0)
}

func (s *MailboxWatchSeats) held() int64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return int64(s.taken)
}

type mailboxHolder struct {
	cancel context.CancelFunc
	done   chan struct{}
	// endedAt is written before done closes, so it is read only after.
	endedAt time.Time
}

// Run is the loop. It returns when the context ends, letting every mailbox and the lock go.
func (m MailboxWatch) Run(ctx context.Context) {
	if m.Holds == nil || m.Leadership == nil {
		slog.ErrorContext(ctx, "the mailbox watch did not start", slog.String("error", "queue.mailbox_watch_incomplete"))
		return
	}
	m = m.withDefaults()
	holders := map[mailservice.WatchTarget]*mailboxHolder{}
	defer func() {
		m.stopAll(ctx, holders)
		released, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
		defer cancel()
		_ = m.Leadership.Release(released)
		m.unseat()
	}()
	for {
		m.tick(ctx, holders)
		if !wait(ctx, m.Tick) {
			return
		}
	}
}

func (m MailboxWatch) withDefaults() MailboxWatch {
	defaults := []struct {
		value    *time.Duration
		fallback time.Duration
	}{
		{&m.Tick, mailboxWatchTick}, {&m.Hold, mailboxWatchHold}, {&m.Rest, mailboxWatchRest},
		{&m.Retry, mailboxWatchRetry}, {&m.Backoff, mailboxWatchBackoff}, {&m.Ceiling, mailboxWatchCeiling},
		{&m.Release, mailboxWatchRelease}, {&m.Pace, mailboxWatchPace},
	}
	for _, d := range defaults {
		if *d.value <= 0 {
			*d.value = d.fallback
		}
	}
	if m.Limit <= 0 {
		m.Limit = mailboxWatchLimit
	}
	if m.unsure == nil {
		m.unsure = new(time.Time)
	}
	if m.seated == nil {
		m.seated = new(bool)
	}
	return m
}

// tick takes or confirms the lock, then holds what lives and lets go of what does not.
func (m MailboxWatch) tick(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	bounded, cancel := context.WithTimeout(ctx, bookkeepingTimeout)
	defer cancel()
	leading, known := m.leading(bounded)
	if !known {
		if m.unsure.IsZero() {
			*m.unsure = time.Now()
		}
		if time.Since(*m.unsure) >= m.Hold {
			m.stopAll(ctx, holders)
		}
		m.held(ctx, holders)
		return
	}
	*m.unsure = time.Time{}
	m.shards(ctx, leading)
	if !leading {
		m.stopAll(ctx, holders)
		m.held(ctx, holders)
		if m.Signals != nil {
			// The leader says what it leaves; a former one says nothing stale.
			m.Signals.MailboxWatchesUnheld(ctx, m.Shard.Index, 0)
		}
		return
	}
	targets, err := m.Holds.Targets(bounded, m.Shard, m.Limit)
	if err != nil {
		// The holds that run keep running: a database that does not answer a list for a minute
		// is no reason to drop mailboxes that are held fine.
		slog.WarnContext(ctx, "listing the mailboxes to hold failed", slog.String("error", shared.AsError(err).Code))
		return
	}
	if m.Signals != nil {
		if total, err := m.Holds.Wanted(bounded, m.Shard); err == nil {
			m.Signals.MailboxWatchesUnheld(ctx, m.Shard.Index, max(total-int64(len(targets)), 0))
		}
	}
	wanted := make(map[mailservice.WatchTarget]bool, len(targets))
	for _, target := range targets {
		wanted[target] = true
	}
	gone := map[mailservice.WatchTarget]*mailboxHolder{}
	for target, holder := range holders {
		if !wanted[target] {
			gone[target] = holder
			delete(holders, target)
		}
	}
	m.letGo(ctx, gone)
	now := time.Now()
	started := 0
	for _, target := range targets {
		if holder, ok := holders[target]; ok {
			select {
			case <-holder.done:
				if now.Sub(holder.endedAt) < m.Retry {
					continue
				}
			default:
				continue
			}
		}
		holders[target] = m.start(ctx, target, m.slot(started))
		started++
	}
	m.held(ctx, holders)
}

// leading answers whether this process holds the lock, and whether that is known: an error asking
// is not an answer.
func (m MailboxWatch) leading(ctx context.Context) (leading, known bool) {
	if m.SchemaBehind != nil && m.SchemaBehind(ctx) {
		_ = m.Leadership.Release(ctx)
		m.unseat()
		return false, true
	}
	leading, err := m.Leadership.Confirm(ctx)
	if err == nil && !leading {
		m.unseat()
		// A process at its bound does not ask: the shard is left to a worker with a seat free.
		if m.Seats.take() {
			leading, err = m.Leadership.Acquire(ctx)
			if err == nil && leading {
				*m.seated = true
			} else {
				m.Seats.give()
			}
		}
	}
	if err != nil {
		slog.WarnContext(ctx, "the mailbox watch's lock could not be asked", slog.String("error", shared.AsError(err).Code))
		return false, false
	}
	return leading, true
}

// unseat gives back this watch's seat, if it holds one.
func (m MailboxWatch) unseat() {
	if m.seated != nil && *m.seated {
		*m.seated = false
		m.Seats.give()
	}
}

// shards reports the split and how many shards this process leads.
func (m MailboxWatch) shards(ctx context.Context, leading bool) {
	if m.Signals == nil {
		return
	}
	led := int64(0)
	switch {
	case m.Seats != nil:
		led = m.Seats.held()
	case leading:
		led = 1
	}
	m.Signals.MailboxWatchShards(ctx, int64(max(m.Shard.Count, 1)), led)
}

// slot is when the n-th sign-in of a tick starts: n paces on, at a random point within its pace.
func (m MailboxWatch) slot(n int) time.Duration {
	return time.Duration(n)*m.Pace + jitter(m.Pace)
}

// jitter is a random duration below d, so holds that failed together do not retry together.
func jitter(d time.Duration) time.Duration {
	var raw [8]byte
	if d <= 0 {
		return 0
	}
	if _, err := rand.Read(raw[:]); err != nil {
		return 0
	}
	//nolint:gosec // G115: the remainder is below d, a positive duration
	return time.Duration(binary.BigEndian.Uint64(raw[:]) % uint64(d))
}

// start holds one mailbox, after delay, until its context ends or it cannot be held.
func (m MailboxWatch) start(ctx context.Context, target mailservice.WatchTarget, delay time.Duration) *mailboxHolder {
	holdCtx, cancel := context.WithCancel(ctx)
	holder := &mailboxHolder{cancel: cancel, done: make(chan struct{})}
	concurrency.Go(holdCtx, "worker.mailbox_watch", func(ctx context.Context) {
		defer close(holder.done)
		defer func() { holder.endedAt = time.Now() }()
		if !wait(ctx, delay) {
			return
		}
		failures := 0
		for ctx.Err() == nil {
			outcome, err := m.Holds.Hold(ctx, target, m.Hold)
			if ctx.Err() != nil {
				return
			}
			if m.Signals != nil {
				m.Signals.MailboxWatchEnded(ctx, outcome)
			}
			pause := time.Duration(0)
			switch outcome {
			case mailservice.WatchGone, mailservice.WatchUnsupported:
				return
			case mailservice.WatchArrived:
				failures, pause = 0, m.Rest
			case mailservice.WatchQuiet:
				failures = 0
			default:
				failures++
				pause = m.backoff(failures)
				pause = pause/2 + jitter(pause/2)
				slog.WarnContext(ctx, "holding a mailbox failed", slog.String("error", shared.AsError(err).DetailCode),
					slog.Int("failures", failures))
			}
			if !wait(ctx, pause) {
				return
			}
		}
	})
	return holder
}

// backoff doubles from Backoff up to Ceiling, so a server that is down is asked a few times an
// hour rather than every few seconds.
func (m MailboxWatch) backoff(failures int) time.Duration {
	pause := m.Backoff
	for range failures - 1 {
		pause *= 2
		if pause >= m.Ceiling {
			return m.Ceiling
		}
	}
	return pause
}

func (m MailboxWatch) stopAll(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	// Not cancelled with the watch: letting go waits for the holds, Release at most.
	m.letGo(context.WithoutCancel(ctx), holders)
	clear(holders)
}

// letGo cancels every hold, then waits for them together, for Release at most. A hold still running
// then is counted in the log and left: its context is cancelled, so it ends at its own deadline.
func (m MailboxWatch) letGo(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	for _, holder := range holders {
		holder.cancel()
	}
	timer := time.NewTimer(m.Release)
	defer timer.Stop()
	stuck := 0
	for _, holder := range holders {
		if stuck > 0 {
			select {
			case <-holder.done:
			default:
				stuck++
			}
			continue
		}
		select {
		case <-holder.done:
		case <-timer.C:
			stuck++
		}
	}
	if stuck > 0 {
		slog.WarnContext(ctx, "mailbox holds did not end when let go",
			slog.String("error", "queue.mailbox_watch_hold_stuck"), slog.Int("holds", stuck))
	}
}

func (m MailboxWatch) held(ctx context.Context, holders map[mailservice.WatchTarget]*mailboxHolder) {
	if m.Signals == nil {
		return
	}
	live := 0
	for _, holder := range holders {
		select {
		case <-holder.done:
		default:
			live++
		}
	}
	m.Signals.MailboxWatchesHeld(ctx, m.Shard.Index, int64(live))
}
