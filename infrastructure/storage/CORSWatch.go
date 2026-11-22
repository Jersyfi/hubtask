// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package storage

import (
	"context"
	"sync"
	"time"

	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

const (
	// CORSMissingCode is a bucket that refuses the browser's upload from an origin of the
	// interface (deployment.md §6.1).
	CORSMissingCode = "config.storage_cors_missing"

	// CORSCheckEvery is how often the bucket is asked again. A rule set while the server runs is
	// seen at the latest after this long - the command that sets it runs in another process and
	// cannot reach the result held here.
	CORSCheckEvery = 15 * time.Minute

	corsWatchComponent = "storage.cors-watch"
)

// preflighter is the one call the watch makes.
type preflighter interface {
	Preflight(ctx context.Context, origin string) Preflight
}

// CORSWatch is the health report's `config.storage_cors_missing` source: the bucket is asked in
// the background, once at start and then every CORSCheckEvery, and the report is served from the
// last answer per origin - a report never dials, for the reason Probe gives. Per process: every
// replica asks for itself.
type CORSWatch struct {
	store   preflighter
	bucket  string
	origins []string
	clock   clock.Clock

	mu      sync.Mutex
	answers map[string]heldAnswer
}

type heldAnswer struct {
	refused   bool
	checkedAt time.Time
}

// NewCORSWatch watches the bucket for the origins the upload comes from.
func NewCORSWatch(store *S3Storage, origins []string, clk clock.Clock) *CORSWatch {
	return newCORSWatch(store, store.bucket, origins, clk)
}

func newCORSWatch(store preflighter, bucket string, origins []string, clk clock.Clock) *CORSWatch {
	return &CORSWatch{
		store: store, bucket: bucket, origins: origins, clock: clk,
		answers: map[string]heldAnswer{},
	}
}

// Start asks now and then on the timer, until ctx ends.
func (w *CORSWatch) Start(ctx context.Context) {
	concurrency.Go(ctx, corsWatchComponent, func(ctx context.Context) {
		ticker := time.NewTicker(CORSCheckEvery)
		defer ticker.Stop()
		for {
			w.Check(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	})
}

// Check asks once for every origin. An origin the bucket could not be asked about keeps its last
// answer: an unreachable bucket is the breaker's to report, not a missing rule.
func (w *CORSWatch) Check(ctx context.Context) {
	for _, origin := range w.origins {
		answer := w.store.Preflight(ctx, origin)
		if answer.Outcome == PreflightUnreachable {
			continue
		}
		at := w.clock.Now()
		w.mu.Lock()
		w.answers[origin] = heldAnswer{refused: answer.Outcome == PreflightRefused, checkedAt: at}
		w.mu.Unlock()
	}
}

// Warnings is the report's source: one warning per origin whose last answer was a refusal.
func (w *CORSWatch) Warnings(context.Context) []health.Warning {
	w.mu.Lock()
	defer w.mu.Unlock()
	var warnings []health.Warning
	for _, origin := range w.origins {
		answer, ok := w.answers[origin]
		if !ok || !answer.refused {
			continue
		}
		warnings = append(warnings, health.Warning{
			Code: CORSMissingCode, Severity: "warn",
			Params: map[string]string{
				"bucket":     w.bucket,
				"origin":     origin,
				"checked_at": answer.checkedAt.UTC().Format(time.RFC3339),
			},
		})
	}
	return warnings
}
