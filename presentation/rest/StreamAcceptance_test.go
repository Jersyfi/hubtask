// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The two acceptance criteria of C-10 that are measurements rather than assertions.

// countingStream records how often the log was read, which is the number the idle-cost measurement
// is about.
type countingStream struct {
	mu     sync.Mutex
	rounds int
}

func (c *countingStream) Resume(
	context.Context, appshared.ActorContext, string,
) (syncservice.Position, error) {
	return syncservice.Position{}, nil
}

func (c *countingStream) Next(
	context.Context, appshared.ActorContext, syncservice.Position,
) (syncservice.Batch, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rounds++
	return syncservice.Batch{}, nil
}

func (c *countingStream) Encode(position syncservice.Position) string {
	return "cursor-" + strconv.FormatInt(position.Seq, 10)
}

func (c *countingStream) reads() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.rounds
}

// "Several hundred idle connections cost no busy loop" — a measurement in the test, as the task
// asks, rather than an assertion in a comment.
//
// What is measured is reads of the change log. An idle stream is woken by LISTEN/NOTIFY or, when
// there is nothing to be woken by, by its idle timer at thirty seconds - so over a window far
// shorter than that, three hundred idle connections must read the log exactly three hundred times:
// once each, when they open. Any design with a poller per connection fails this by construction.
func TestSeveralHundredIdleStreamsDoNotPoll(t *testing.T) {
	const connections = 300
	const idle = 300 * time.Millisecond

	stream := &countingStream{}
	controller, registry, _ := streamController(t, nil, StreamLimits{PerProcess: connections + 10})
	controller.Stream = stream
	// No wake-ups at all: this measures what a stream costs when nothing is happening.
	controller.Wakeups = nil

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for range connections {
		wg.Add(1)
		concurrency.Go(ctx, "test.idle_stream", func(context.Context) {
			defer wg.Done()
			request := authenticated(
				httptest.NewRequestWithContext(ctx, http.MethodGet, "/stream", nil))
			controller.StreamChanges(httptest.NewRecorder(), request, openapi.StreamChangesParams{})
		})
	}

	// Wait until every connection has opened and done its first read.
	deadline := time.Now().Add(5 * time.Second)
	for registry.Open() < connections && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if open := registry.Open(); open != connections {
		t.Fatalf("%d of %d streams opened", open, connections)
	}
	settled := stream.reads()

	time.Sleep(idle)
	after := stream.reads()

	cancel()
	wg.Wait()

	if settled > connections {
		t.Errorf("%d log reads to open %d streams - a stream read more than once to start",
			settled, connections)
	}
	if after != settled {
		t.Errorf("%d idle connections read the change log %d further times in %v; "+
			"idle streams must cost nothing until something wakes them",
			connections, after-settled, idle)
	}
}

// "SIGTERM closes every stream inside the grace period and loses no record a client had not yet
// received."
//
// Nothing is held in memory for a client, which is what makes the second half true: a record not
// yet sent is still in the log, and the client's cursor is the one of the last record it actually
// received. The test proves exactly that - the cursor the client holds when the process goes away
// is the one that yields the unsent record on the next connection.
func TestAShutdownLosesNoRecordAClientHadNotReceived(t *testing.T) {
	first := streamRecord(1, "work_item")
	second := streamRecord(2, "work_item")

	// The first connection is given only the first record, then the process drains.
	stream := &fakeStream{batches: []syncservice.Batch{{
		Records: []syncservice.Record{first},
		Cursor:  syncservice.Position{Seq: 1, IssuedAt: streamNow},
	}}}
	controller, registry, _ := streamController(t, stream, StreamLimits{PerProcess: 4})

	request := authenticated(
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
	recorder, wait := serveStream(t, controller, request)

	waitUntil(t, func() bool { return registry.Open() == 1 })
	// Let the first record reach the wire before the drain.
	time.Sleep(50 * time.Millisecond)

	drained := make(chan struct{})
	concurrency.Go(t.Context(), "test.drain", func(context.Context) {
		defer close(drained)
		registry.CloseAll()
	})
	select {
	case <-drained:
	case <-time.After(5 * time.Second):
		t.Fatal("the drain did not finish")
	}
	wait()

	events := parseEvents(t, recorder.Body.String())
	if len(events) != 1 {
		t.Fatalf("%d events reached the client, want the one that was ready", len(events))
	}
	if events[0].id != "cursor-1" {
		t.Fatalf("the client's last cursor is %q", events[0].id)
	}
	if !strings.Contains(recorder.Body.String(), ": closing") {
		t.Error("the client was not told the stream was closing")
	}

	// The second record was never sent, and it was never held either: it is in the log, and the
	// cursor the client kept is what fetches it. A new process, a new connection, no gap.
	resumed := &fakeStream{
		resumeAt: syncservice.Position{Seq: 1, IssuedAt: streamNow},
		batches: []syncservice.Batch{{
			Records: []syncservice.Record{second},
			Cursor:  syncservice.Position{Seq: 2, IssuedAt: streamNow},
		}},
	}
	next, nextRegistry, _ := streamController(t, resumed, StreamLimits{PerProcess: 4})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cursor := events[0].id
	request = authenticated(httptest.NewRequestWithContext(ctx, http.MethodGet, "/stream", nil))
	secondRecorder := httptest.NewRecorder()
	done := make(chan struct{})
	concurrency.Go(ctx, "test.stream_resumed", func(context.Context) {
		defer close(done)
		next.StreamChanges(secondRecorder, request, openapi.StreamChangesParams{
			LastEventID: &cursor,
		})
	})

	waitUntil(t, func() bool { return nextRegistry.Open() == 1 })
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	resumedEvents := parseEvents(t, secondRecorder.Body.String())
	if len(resumedEvents) != 1 {
		t.Fatalf("%d events after resuming, want the one that was missed", len(resumedEvents))
	}
	if resumedEvents[0].id != "cursor-2" {
		t.Errorf("the resumed stream sent %q, want the record the first connection never reached",
			resumedEvents[0].id)
	}
}

func waitUntil(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("gave up waiting")
}
