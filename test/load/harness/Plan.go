// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package harness is the load generator the resilience and load suites share.
//
// It is the RT-8 command's own generator, lifted out of it and given a ramp. Three properties are
// the reason it exists rather than a library being pulled in, and every one of them is a decision
// somebody has to be able to read:
//
//   - It counts from the client. What a server reports about itself under overload is what it
//     managed to record, and a request that never got a worker never appears in it.
//   - It paces independently of the responses. A generator whose next request waits for the last
//     answer measures itself: when the installation slows down, the offered load falls with it and
//     the numbers stay beautiful. That is coordinated omission, and it is the single most common
//     way a load test lies.
//   - It records a timeline and the class of every call. "P95 held over ten minutes" and "P95 held
//     while the shedding was engaged" are different claims, and only the second one is evidence
//     for RT-6.
package harness

import (
	"context"
	"time"
)

// Class is what a request is worth waiting for, and it is the same two-way split the load shedder
// makes (infrastructure/resilience). The harness has to know it because the whole of RT-6 is a
// statement about one class while the other is being refused.
type Class string

const (
	// ClassInteractive is a person waiting. Its latency is the number RT-6 is about.
	ClassInteractive Class = "interactive"
	// ClassDeferrable is bulk, export, search and the query shapes: the work that is shed.
	ClassDeferrable Class = "deferrable"
)

// Stage is one step of a ramp: an offered rate, held for a while.
type Stage struct {
	// PerSecond is the offered rate over the whole generator, not per worker.
	PerSecond int
	// For is how long this step is held.
	For time.Duration
}

// Plan is a ramp. One stage is a flat run, which is what RT-8 has always done; several stages are
// how capacity is found - the rate is raised until the answers stop keeping up, and the stage in
// which that happened is visible in the timeline.
//
// Expressed as stages rather than as a start, an end and a slope because a ramp with a plateau is
// the useful shape: rise, hold, look. A slope alone never holds anything long enough for a P95 to
// mean something.
type Plan []Stage

// FlatPlan is the one-stage plan: a rate held for a duration.
func FlatPlan(perSecond int, duration time.Duration) Plan {
	return Plan{{PerSecond: perSecond, For: duration}}
}

// Duration is how long the whole plan takes.
func (p Plan) Duration() time.Duration {
	var total time.Duration
	for _, stage := range p {
		total += stage.For
	}
	return total
}

// Peak is the highest rate the plan offers. It is what a report calls the run's ceiling.
func (p Plan) Peak() int {
	peak := 0
	for _, stage := range p {
		if stage.PerSecond > peak {
			peak = stage.PerSecond
		}
	}
	return peak
}

// RateAt is the offered rate this far into the run, and zero once the plan is over.
//
// A step function rather than an interpolation between stages: a stage is a measurement, and a
// rate that is still moving while the percentile is being taken is a percentile of nothing in
// particular.
func (p Plan) RateAt(elapsed time.Duration) int {
	if elapsed < 0 {
		return 0
	}
	var boundary time.Duration
	for _, stage := range p {
		boundary += stage.For
		if elapsed < boundary {
			return stage.PerSecond
		}
	}
	return 0
}

// Pacer hands out permission to make a request, at the plan's current rate, shared by every
// worker.
//
// A rate held by the workers themselves would drift with latency: eight workers waiting 200 ms
// each is forty requests a second only while every answer is instant. This is the coordinated
// omission discipline, and it is why the permits are produced by the clock rather than by the
// answers.
type Pacer struct {
	permits chan struct{}
	plan    Plan
}

// NewPacer starts producing permits at the plan's rate until the context ends or the plan does.
//
// The goroutine is bare, and it may be: this package is a test harness, and CLAUDE.md rule 5
// governs the production tree. Everything it owns ends with the context.
func NewPacer(ctx context.Context, plan Plan, started time.Time) *Pacer {
	p := &Pacer{permits: make(chan struct{}), plan: plan}
	go p.produce(ctx, started)
	return p
}

func (p *Pacer) produce(ctx context.Context, started time.Time) {
	defer close(p.permits)

	rate := p.plan.RateAt(0)
	if rate < 1 {
		return
	}
	ticker := time.NewTicker(time.Second / time.Duration(rate))
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// The rate is re-read on every tick rather than on a stage boundary, because a
			// boundary needs a second timer and a second timer is a second thing that can be
			// wrong. A tick's worth of lag at a stage change costs nothing a percentile notices.
			if next := p.plan.RateAt(time.Since(started)); next != rate {
				if next < 1 {
					return
				}
				rate = next
				ticker.Reset(time.Second / time.Duration(rate))
			}
			select {
			case p.permits <- struct{}{}:
			case <-ctx.Done():
				return
			}
		}
	}
}

// Wait blocks until this request may be made, and reports false when the run is over.
func (p *Pacer) Wait(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return false
	case _, open := <-p.permits:
		return open
	}
}
