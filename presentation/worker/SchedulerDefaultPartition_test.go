// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	streams "github.com/Jersyfi/hubtask/core/application/repository/streams"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

// inDefault answers every month as already living in an existing partition - the state the
// stream migration leaves the current month in - and counts the asks.
type inDefault struct {
	mu    sync.Mutex
	asked int
}

func (p *inDefault) Ensure(context.Context, string, time.Time) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked++
	return "", nil
}

func (p *inDefault) DropAged(context.Context, string, int) ([]streams.Dropped, error) {
	return nil, nil
}

func (p *inDefault) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asked
}

// The state is reported once per stream and month, not on every tick: three lines every ten
// seconds bury every other line of the log for a whole month.
func TestTheDefaultPartitionIsReportedOncePerStreamAndMonth(t *testing.T) {
	var lines bytes.Buffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&lines, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })

	partitions := &inDefault{}
	leader := scheduler(&leadership{lock: &lock{}, name: "a"}, &depthQueue{}, newSchedulerSignals())
	leader.StreamPartitions = partitions
	leader.TickInterval = time.Millisecond

	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	concurrency.Go(ctx, "test.scheduler.loop", func(context.Context) {
		leader.Run(ctx)
		close(done)
	})
	perTick := 2 * len(streams.Tables())
	deadline := time.Now().Add(2 * time.Second)
	for partitions.count() < 5*perTick && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done

	if partitions.count() < 5*perTick {
		t.Fatalf("the loop made only %d asks in time", partitions.count())
	}
	got := strings.Count(lines.String(), "the stream's month is in the default partition")
	if got != perTick {
		t.Errorf("%d notices over %d ticks, want %d - one per stream and month",
			got, partitions.count()/perTick, perTick)
	}
}
