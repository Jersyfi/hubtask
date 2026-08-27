// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// JobChannel is the `LISTEN` channel the queue's trigger announces on
// (db/migrations/0048_job_notify.sql). A constant on both sides of a name that has to agree, for
// ChangeChannel's reason: a channel nobody listens on is a queue that never wakes early, and
// nothing would say so.
const JobChannel = "hubtask_job"

// JobListener is the wake-up for the worker: one connection per process holding a `LISTEN`, and a
// single doorbell out of it (ADR-0007's first countermeasure).
//
// # Why this is a second listener rather than a second channel on the first
//
// The two live in different processes. `ChangeListener` runs where streams are served - the API
// role - and this runs where jobs are executed - the worker role. A single listener would mean the
// API process holding a subscription for notifications nobody in it is waiting for, and the worker
// process holding one for streams it does not serve. Two small listeners, each in the role that
// needs it, is what the role split is for.
//
// # Why holding a connection is not a hole in rule 3
//
// ChangeListener's reasoning, unchanged: the only statement is `LISTEN hubtask_job`, it touches no
// table, and nothing else can reach this connection - it is unexported and this type offers no way
// to run anything on it. What arrives back is a doorbell with an empty payload. Every claim still
// goes through the transaction wrapper.
//
// # What it is not
//
// It is not the delivery guarantee. A missed notification costs latency and nothing else: the
// runner's poll is the fallback, and it is what a process whose listener has never connected falls
// back to. That is why this can be optional at the composition root and why a failure here is a
// warning rather than an error.
type JobListener struct {
	pool *pgxpool.Pool

	// woken has room for one, and a send that would block is dropped. The correct semantics for a
	// doorbell: a runner with an unread wake-up already knows there is work, and a second ring
	// tells it nothing new. It is also what stops a busy queue from blocking the listening loop.
	woken chan struct{}

	mu        sync.Mutex
	connected bool
}

func NewJobListener(pool *pgxpool.Pool) *JobListener {
	return &JobListener{pool: pool, woken: make(chan struct{}, 1)}
}

// Woken is the channel the runner selects on beside its poll timer.
func (l *JobListener) Woken() <-chan struct{} { return l.woken }

// Connected reports whether a listening connection is currently held. What the health report
// reads: a worker without one still runs every job, and runs them at the poll interval.
func (l *JobListener) Connected() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.connected
}

// Run holds the listening connection for as long as the context lives, reconnecting when it drops.
//
// Started through SafeGo by the composition root like every other loop (rule 5). It returns when
// the context is cancelled and not before: a listener that gave up after one failure would leave
// the worker at its poll interval for the rest of the process's life, which is a degradation
// nothing else reports.
func (l *JobListener) Run(ctx context.Context) {
	slog.InfoContext(ctx, "job listener ready", slog.String("channel", JobChannel))

	for ctx.Err() == nil {
		if err := l.listen(ctx); err != nil && ctx.Err() == nil {
			// Warning rather than error: the queue keeps working, more slowly.
			slog.WarnContext(ctx, "the job listener lost its connection",
				slog.String("error", err.Error()))
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(listenerReconnectDelay):
		}
	}
}

// listen holds one connection until it fails or the context ends.
func (l *JobListener) listen(ctx context.Context) error {
	conn, err := l.pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()

	if _, err := conn.Exec(ctx, "LISTEN "+JobChannel); err != nil {
		return err
	}

	l.setConnected(true)
	defer l.setConnected(false)

	// One ring on connecting. A job enqueued while this process had no listener would otherwise
	// wait for the poll, and the moment a listener attaches is exactly the moment that is most
	// likely - a restart, a failover, a rolling update.
	l.ring()

	for {
		// No deadline of its own, for ChangeListener's reason: waiting for a notification is the
		// loop's whole purpose, and the bound is the context the shutdown cancels.
		if _, err := conn.Conn().WaitForNotification(ctx); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				return nil
			}
			return err
		}
		// The payload is empty by construction, so there is nothing to read and nothing to
		// validate: anything arriving on this channel means "there may be work".
		l.ring()
	}
}

func (l *JobListener) ring() {
	select {
	case l.woken <- struct{}{}:
	default:
	}
}

func (l *JobListener) setConnected(connected bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.connected = connected
}
