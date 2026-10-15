// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// finishedJobs is the queue's purge, as the leader sees it.
type finishedJobs struct {
	calls  int
	before time.Time
	batch  int
	purged map[string]int
	err    error
}

func (f *finishedJobs) PurgeFinished(_ context.Context, before time.Time, batch int) (map[string]int, error) {
	f.calls++
	f.before, f.batch = before, batch
	return f.purged, f.err
}

// The catalogue's seven days for a finished job, kept by the leader on the first tick of each hour,
// bounded per run, and counted per state - zeros included, so every state has a series.
func TestTheLeaderPurgesFinishedJobsHourly(t *testing.T) {
	purge := &finishedJobs{purged: map[string]int{"SUCCEEDED": 3, "DEAD_LETTER": 1}}
	signals := newSchedulerSignals()
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, signals)
	leader.FinishedJobs = purge

	leader.tick(t.Context(), false, now) // 12:00:00, the hour's first tick
	if purge.calls != 1 || !purge.before.Equal(now.Add(-7*24*time.Hour)) || purge.batch != 10_000 {
		t.Fatalf("purged %d times, before %s, batch %d", purge.calls, purge.before, purge.batch)
	}
	if signals.purged["SUCCEEDED"] != 3 || signals.purged["DEAD_LETTER"] != 1 {
		t.Errorf("counted %v", signals.purged)
	}
	for _, state := range []string{"FAILED", "CANCELLED"} {
		if _, reported := signals.purged[state]; !reported {
			t.Errorf("%s has no reading", state)
		}
	}

	leader.tick(t.Context(), true, now.Add(10*time.Second)) // the same hour
	if purge.calls != 1 {
		t.Errorf("purged %d times within one hour", purge.calls)
	}
}

// A purge that fails is logged and the tick goes on: the leader's other duties do not wait on it,
// and the next hour tries again.
func TestAFailedPurgeDoesNotStopTheLeader(t *testing.T) {
	signals := newSchedulerSignals()
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, signals)
	leader.FinishedJobs = &finishedJobs{err: shared.ErrUnavailable.WithDetail("postgres.query_failed")}

	if leading := leader.tick(t.Context(), false, now); !leading {
		t.Fatal("a failed purge cost the leadership")
	}
	if len(signals.purged) != 0 {
		t.Errorf("a failed purge was counted: %v", signals.purged)
	}
	if len(signals.depths) == 0 {
		t.Error("the leader's other duties did not run")
	}
}
