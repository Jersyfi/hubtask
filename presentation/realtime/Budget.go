// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"math"
	"sync"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
)

// BudgetRate is a byte budget: PerMinute bytes refill, Burst at most are held. A message larger
// than Burst could never pass, so a burst is at least a snapshot and an update.
type BudgetRate struct {
	PerMinute float64
	Burst     float64
}

// The defaults bound what a live document stores per process. A person typing writes a few
// kilobytes a minute; a pasted picture is one update of up to domain.MaxUpdateBytes. A connection
// may write its largest snapshot and update at once, then 2 MiB a minute; a workspace, across
// every connection of it on this process, 64 MiB at once, then 16 MiB a minute.
var (
	DefaultConnectionBytes = BudgetRate{PerMinute: 2 << 20, Burst: domain.MaxSnapshotBytes + domain.MaxUpdateBytes}
	DefaultWorkspaceBytes  = BudgetRate{PerMinute: 16 << 20, Burst: 64 << 20}
)

const (
	budgetIdleTTL       = 10 * time.Minute
	budgetSweepInterval = time.Minute
)

// Budget is a byte token bucket per key, per process - like the REST rate limiter, so a shared
// counter is not a write on the path of every update; the effective budget is this times the
// replicas (observability-reliability.md §6).
type Budget struct {
	rate      BudgetRate
	mutex     sync.Mutex
	buckets   map[string]*byteBucket
	lastSweep time.Time
}

type byteBucket struct {
	tokens float64
	seen   time.Time
}

func NewBudget(rate BudgetRate) *Budget {
	return &Budget{rate: rate, buckets: map[string]*byteBucket{}}
}

// Spend takes n bytes from the key's bucket; a wait above zero is the refusal, and how long until
// n bytes are there again.
func (b *Budget) Spend(key string, n int, now time.Time) time.Duration {
	b.mutex.Lock()
	defer b.mutex.Unlock()
	if now.Sub(b.lastSweep) >= budgetSweepInterval {
		for k, bucket := range b.buckets {
			if now.Sub(bucket.seen) > budgetIdleTTL {
				delete(b.buckets, k)
			}
		}
		b.lastSweep = now
	}
	bucket, known := b.buckets[key]
	if !known {
		bucket = &byteBucket{tokens: b.rate.Burst, seen: now}
		b.buckets[key] = bucket
	}
	perSecond := b.rate.PerMinute / 60
	bucket.tokens = math.Min(b.rate.Burst, bucket.tokens+now.Sub(bucket.seen).Seconds()*perSecond)
	bucket.seen = now
	need := float64(n)
	if need <= bucket.tokens {
		bucket.tokens -= need
		return 0
	}
	if perSecond <= 0 {
		return time.Hour
	}
	return time.Duration(math.Ceil((need-bucket.tokens)/perSecond)) * time.Second
}
