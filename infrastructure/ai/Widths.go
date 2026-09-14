// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"sync"
	"time"
)

// WidthPool remembers, per endpoint and model, how wide the vectors are - what a model description
// answered, or what the first batch this process embedded turned out to be (#569).
//
// Per process and never stored, the way breaker state is: what it holds is what this process has
// learned, and a restart learns it again for the price of one description. It exists so that a
// model the index cannot hold is refused before a batch is paid for, and so that the capability
// report and the health probe can say so without a network call on the request path - both read
// the pool; the embedding pass and the adapters' own batches write it.
//
// **An entry has to keep being confirmed, or it stops counting.** Nothing can tell the pool that a
// workspace switched models - a switch is a new key, and nothing enumerates tenants to find the
// old one orphaned - so a width that nobody has asked for lately is treated as one nobody is
// configured with. The embedding pass asks every pass, which is what confirms an entry; the health
// probe counts only entries confirmed within `StaleAfter`. That is the self-healing a breaker has
// by closing again, arrived at differently: a breaker sees the endpoint recover, and this sees the
// question stop being asked.
//
// Bounded the way the breaker pool is, for the same reason: the key comes from configuration and
// configuration comes from tenants. Two workspaces on the same endpoint and model name share an
// entry, which is an accepted limitation written down rather than a surprise - a gateway that
// serves two backends under one alias by key is a configuration this pool cannot tell apart, and
// the breaker pool cannot either.
type WidthPool struct {
	// Cap is how many entries the pool holds before it starts again. Zero means 256.
	Cap int
	// StaleAfter is how long an entry counts for the health probe after it was last confirmed.
	// Zero means three hours: three of the embedding pass's hourly intervals, so one missed pass
	// does not clear a real degradation and a switched workspace clears within the afternoon.
	StaleAfter time.Duration

	mu     sync.Mutex
	widths map[string]width
}

type width struct {
	dimensions int
	// since is when the width was first learned; confirmed is when it was last asked for.
	since, confirmed time.Time
}

const (
	defaultWidthCap        = 256
	defaultWidthStaleAfter = 3 * time.Hour
)

func widthKey(endpoint, model string) string { return endpoint + "\x00" + model }

// Known answers the width this process has learned for one endpoint and model, or zero. It
// confirms nothing: it is what Capabilities reads, on request paths, and a manifest being read is
// not a workspace embedding.
func (p *WidthPool) Known(endpoint, model string) int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.widths[widthKey(endpoint, model)].dimensions
}

// Confirm is Known for the embedding pass: the same answer, and the entry marked as asked for
// now, which is what keeps it counting for the health probe.
func (p *WidthPool) Confirm(endpoint, model string, now time.Time) int {
	if p == nil {
		return 0
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	key := widthKey(endpoint, model)
	held, seen := p.widths[key]
	if !seen {
		return 0
	}
	held.confirmed = now
	p.widths[key] = held
	return held.dimensions
}

// Record keeps a width, and confirms it. Zero is not recorded: it is the absence of an answer,
// not one. The first answer for a key is kept - a model re-tagged under the same name keeps its
// old width until the entry goes stale or the process restarts, which is stated in deployment.md.
func (p *WidthPool) Record(endpoint, model string, dimensions int, now time.Time) {
	if p == nil || dimensions <= 0 {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	key := widthKey(endpoint, model)
	if held, seen := p.widths[key]; seen {
		held.confirmed = now
		p.widths[key] = held
		return
	}
	limit := p.Cap
	if limit <= 0 {
		limit = defaultWidthCap
	}
	if p.widths == nil || len(p.widths) >= limit {
		p.widths = make(map[string]width)
	}
	p.widths[key] = width{dimensions: dimensions, since: now, confirmed: now}
}

// Wider answers how many widths confirmed within StaleAfter exceed the given one, and since when
// the earliest has been known. For the health probe: "some configured model is wider than the
// index", and no endpoint or model named, for the reason the breaker probe names none (rule 10).
func (p *WidthPool) Wider(than int, now time.Time) (int, time.Time) {
	if p == nil {
		return 0, time.Time{}
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	staleAfter := p.StaleAfter
	if staleAfter <= 0 {
		staleAfter = defaultWidthStaleAfter
	}
	count, earliest := 0, time.Time{}
	for _, w := range p.widths {
		if w.dimensions <= than || now.Sub(w.confirmed) > staleAfter {
			continue
		}
		count++
		if earliest.IsZero() || w.since.Before(earliest) {
			earliest = w.since
		}
	}
	return count, earliest
}
