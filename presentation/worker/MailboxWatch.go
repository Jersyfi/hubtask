// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"time"

	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// MailboxHolds is the application's side of the watch: which mailboxes live, and one hold of one.
type MailboxHolds interface {
	Targets(ctx context.Context, limit int) ([]mailservice.WatchTarget, error)
	// Wanted is how many mailboxes' reads live, beyond the limit.
	Wanted(ctx context.Context) (int64, error)
	Hold(ctx context.Context, target mailservice.WatchTarget, hold time.Duration) (string, error)
}

// MailboxWatchSignals is the slice of the metrics adapter the watch uses.
type MailboxWatchSignals interface {
	// MailboxWatchesHeld is how many mailboxes this process holds open now.
	MailboxWatchesHeld(ctx context.Context, held int64)
	// MailboxWatchesUnheld is how many mailboxes the limit leaves to the minute's read - A-38's
	// number: those workspaces lose the arrival in seconds, and nothing else says so.
	MailboxWatchesUnheld(ctx context.Context, unheld int64)
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

// MailboxWatch holds the connected IMAP mailboxes open (IDLE) so mail reaches the desk in seconds
// (UC-JUM-11 check 10). One process of the installation holds them all, by an advisory lock of
// its own - two would sign every mailbox in twice and providers count connections. Everything it
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
	// SchemaBehind stands the watch by while the database lacks migrations this build needs.
	SchemaBehind func(ctx context.Context) bool
	// Tick, Hold, Limit, Rest, Retry, Backoff, Ceiling, Release and Pace default to the constants
	// above; tests shorten them.
	Tick, Hold, Rest, Retry, Backoff, Ceiling, Release, Pace time.Duration
	Limit                                                    int
	// unsure is shared by the copies withDefaults hands out: since when the lock could not be asked.
	unsure *time.Time
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
		m.stopAll(holders)
		released, cancel := context.WithTimeout(context.WithoutCancel(ctx), bookkeepingTimeout)
		defer cancel()
		_ = m.Leadership.Release(released)
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
			m.stopAll(holders)
		}
		m.held(ctx, holders)
		return
	}
	*m.unsure = time.Time{}
	if !leading {
		m.stopAll(holders)
		m.held(ctx, holders)
		return
	}
	targets, err := m.Holds.Targets(bounded, m.Limit)
	if err != nil {
		// The holds that run keep running: a database that does not answer a list for a minute
		// is no reason to drop mailboxes that are held fine.
		slog.WarnContext(ctx, "listing the mailboxes to hold failed", slog.String("error", shared.AsError(err).Code))
		return
	}
	if m.Signals != nil {
		if total, err := m.Holds.Wanted(bounded); err == nil {
			m.Signals.MailboxWatchesUnheld(ctx, max(total-int64(len(targets)), 0))
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
		return false, true
	}
	leading, err := m.Leadership.Confirm(ctx)
	if err == nil && !leading {
		leading, err = m.Leadership.Acquire(ctx)
	}
	if err != nil {
		slog.WarnContext(ctx, "the mailbox watch's lock could not be asked", slog.String("error", shared.AsError(err).Code))
		return false, false
	}
	return leading, true
}

// slot is when the n-th sign-in of a tick starts: n paces on, at a random point within its pace.
func (m MailboxWatch) slot(n int) time.Duration {
	return time.Duration(n)*m.Pace + jitter(m.Pace)
}

// jitter is a random duration below d, so holds that failed together do not retry together.
func jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	return rand.N(d)
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

func (m MailboxWatch) stopAll(holders map[mailservice.WatchTarget]*mailboxHolder) {
	m.letGo(context.Background(), holders)
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
	m.Signals.MailboxWatchesHeld(ctx, int64(live))
}
