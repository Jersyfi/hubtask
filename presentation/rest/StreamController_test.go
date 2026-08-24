// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

var (
	streamTenant  = shared.MustParseID("0192f000-0000-7000-8000-0000000000f1")
	streamAccount = shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	streamNow     = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
)

// fakeStream stands in for the application service. What the controller owes is framing, the
// heartbeat, the caps and the shutdown; what the service decides is proved a layer down.
type fakeStream struct {
	mu       sync.Mutex
	batches  []syncservice.Batch
	resumeAt syncservice.Position
	resumeAs error
	rounds   int
}

func (f *fakeStream) Resume(context.Context, appshared.ActorContext, string) (syncservice.Position, error) {
	if f.resumeAs != nil {
		return syncservice.Position{}, f.resumeAs
	}
	return f.resumeAt, nil
}

func (f *fakeStream) Next(
	context.Context, appshared.ActorContext, syncservice.Position,
) (syncservice.Batch, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.rounds++
	if len(f.batches) == 0 {
		return syncservice.Batch{}, nil
	}
	batch := f.batches[0]
	f.batches = f.batches[1:]
	return batch, nil
}

func (f *fakeStream) Encode(position syncservice.Position) string {
	return "cursor-" + strconv.FormatInt(position.Seq, 10)
}

func streamRecord(seq int64, entity string) syncservice.Record {
	return syncservice.Record{
		Recorded: repository.Recorded{
			Change: repository.Change{
				TenantID: streamTenant, Entity: entity,
				EntityID:    shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
				Op:          repository.Upsert,
				ContainerID: shared.MustParseID("0192f000-0000-7000-8000-00000000000b"),
				ActorID:     streamAccount,
				Payload:     map[string]any{"title": "Review the quote"},
			},
			Seq: seq, OccurredAt: streamNow,
		},
		Cursor: syncservice.Position{Seq: seq, IssuedAt: streamNow},
	}
}

type streamSignals struct {
	mu      sync.Mutex
	opened  int
	closed  int
	refused []string
	records int
}

func (s *streamSignals) StreamOpened(context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opened++
}

func (s *streamSignals) StreamClosed(context.Context, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
}

func (s *streamSignals) StreamRefused(_ context.Context, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refused = append(s.refused, reason)
}

func (s *streamSignals) StreamRecords(_ context.Context, count int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.records += count
}

func (s *streamSignals) refusals() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.refused...)
}

func authenticated(r *http.Request) *http.Request {
	return r.WithContext(appshared.ContextWithActor(r.Context(), appshared.ActorContext{
		TenantID: streamTenant, AccountID: streamAccount, AccountName: "Anna",
		Kind: shared.ActorUser,
	}))
}

func streamController(t *testing.T, stream *fakeStream, limits StreamLimits) (
	StreamController, *StreamRegistry, *streamSignals,
) {
	t.Helper()
	registry := NewStreamRegistry(limits)
	signals := &streamSignals{}
	return StreamController{
		Stream: stream, Registry: registry, Signals: signals,
		Clock: func() time.Time { return streamNow },
	}, registry, signals
}

// serve runs the handler until it returns, and gives the test the body it wrote.
func serveStream(
	t *testing.T, controller StreamController, request *http.Request,
) (*httptest.ResponseRecorder, func()) {
	t.Helper()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	concurrency.Go(request.Context(), "test.stream", func(context.Context) {
		defer close(done)
		controller.StreamChanges(recorder, request, openapi.StreamChangesParams{})
	})
	return recorder, func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the handler did not return")
		}
	}
}

func TestTheStreamFramesEachRecordAsAnEvent(t *testing.T) {
	stream := &fakeStream{batches: []syncservice.Batch{{
		Records: []syncservice.Record{streamRecord(1, "work_item"), streamRecord(2, "comment")},
		Cursor:  syncservice.Position{Seq: 2, IssuedAt: streamNow},
	}}}
	controller, registry, signals := streamController(t, stream, StreamLimits{PerProcess: 4})

	ctx, cancel := context.WithCancel(context.Background())
	request := authenticated(httptest.NewRequestWithContext(ctx, http.MethodGet, "/stream", nil))

	recorder, wait := serveStream(t, controller, request)
	// The handler blocks once the batch is drained; the client leaving is what ends it.
	time.Sleep(50 * time.Millisecond)
	cancel()
	wait()

	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Errorf("content type %q", got)
	}
	// An intermediary that cached this would serve one client's records to another; one that
	// buffered it would hold every event until the connection ended.
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("cache control %q", got)
	}
	if got := recorder.Header().Get("X-Accel-Buffering"); got != "no" {
		t.Errorf("nginx would buffer this stream: %q", got)
	}

	body := recorder.Body.String()
	if !strings.Contains(body, "retry: ") {
		t.Errorf("the stream suggests no reconnection delay:\n%s", body)
	}

	events := parseEvents(t, body)
	if len(events) != 2 {
		t.Fatalf("%d events, want two:\n%s", len(events), body)
	}
	if events[0].id != "cursor-1" || events[1].id != "cursor-2" {
		t.Errorf("the ids are %q and %q, want the cursors", events[0].id, events[1].id)
	}
	// A client dispatches on the event name, so the entity is what it has to be: somebody
	// listening for comments should not have to parse every item change to learn it is not one.
	if events[0].name != "work_item" || events[1].name != "comment" {
		t.Errorf("the names are %q and %q", events[0].name, events[1].name)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(events[0].data), &payload); err != nil {
		t.Fatalf("the data is not JSON: %v\n%s", err, events[0].data)
	}
	for _, field := range []string{"seq", "entity", "entity_id", "op", "occurred_at", "hlc"} {
		if _, present := payload[field]; !present {
			t.Errorf("the payload has no %q: %v", field, payload)
		}
	}
	if signals.records != 2 || signals.opened != 1 || signals.closed != 1 {
		t.Errorf("signals: opened %d, closed %d, records %d",
			signals.opened, signals.closed, signals.records)
	}
	if registry.Open() != 0 {
		t.Errorf("%d streams still counted after the handler returned", registry.Open())
	}
}

// A tombstone carries no content by design, and `null` would say "the change set is empty", which
// is a different statement.
func TestADeletionCarriesNoPayloadField(t *testing.T) {
	record := streamRecord(1, "work_item")
	record.Op = repository.Delete
	record.Payload = nil

	stream := &fakeStream{batches: []syncservice.Batch{{
		Records: []syncservice.Record{record},
		Cursor:  syncservice.Position{Seq: 1, IssuedAt: streamNow},
	}}}
	controller, _, _ := streamController(t, stream, StreamLimits{PerProcess: 4})

	ctx, cancel := context.WithCancel(context.Background())
	request := authenticated(httptest.NewRequestWithContext(ctx, http.MethodGet, "/stream", nil))
	recorder, wait := serveStream(t, controller, request)
	time.Sleep(50 * time.Millisecond)
	cancel()
	wait()

	events := parseEvents(t, recorder.Body.String())
	if len(events) != 1 {
		t.Fatalf("%d events", len(events))
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(events[0].data), &payload); err != nil {
		t.Fatalf("the data is not JSON: %v", err)
	}
	if _, present := payload["payload"]; present {
		t.Errorf("a deletion carries a payload field: %v", payload)
	}
}

// The refusal is a problem document with a Retry-After, before a byte of stream is written: a
// `200` already sent cannot become a `503`.
func TestAStreamOverTheCapIsRefusedWithRetryAfter(t *testing.T) {
	stream := &fakeStream{}
	controller, registry, signals := streamController(t, stream, StreamLimits{PerProcess: 1})

	// Fill the one slot.
	if _, refusal := registry.Admit("somebody", streamTenant.String()); refusal != RefusedNone {
		t.Fatalf("the first slot was refused: %q", refusal)
	}

	recorder := httptest.NewRecorder()
	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
	controller.StreamChanges(recorder, request, openapi.StreamChangesParams{})

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status %d, want 503", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Error("a refused client is not told when to come back")
	}
	if got := recorder.Header().Get("Content-Type"); !strings.Contains(got, "problem+json") {
		t.Errorf("content type %q, want a problem document", got)
	}
	if refusals := signals.refusals(); len(refusals) != 1 || refusals[0] != "process" {
		t.Errorf("refusals %v", refusals)
	}
}

// A cursor the client cannot use is an ordinary problem document, because it is decided before the
// stream starts.
func TestAnUnusableCursorIsAProblemDocument(t *testing.T) {
	stream := &fakeStream{resumeAs: shared.ErrGone.WithDetail("sync.cursor_too_old")}
	controller, _, _ := streamController(t, stream, StreamLimits{PerProcess: 4})

	recorder := httptest.NewRecorder()
	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
	controller.StreamChanges(recorder, request, openapi.StreamChangesParams{})

	if recorder.Code != http.StatusGone {
		t.Errorf("status %d, want 410", recorder.Code)
	}
	if !strings.Contains(recorder.Body.String(), "sync.cursor_too_old") {
		t.Errorf("the body does not say why: %s", recorder.Body.String())
	}
}

func TestAnUnauthenticatedRequestGetsNoStream(t *testing.T) {
	controller, _, _ := streamController(t, &fakeStream{}, StreamLimits{PerProcess: 4})

	recorder := httptest.NewRecorder()
	controller.StreamChanges(recorder, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil),
		openapi.StreamChangesParams{})

	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", recorder.Code)
	}
}

// SIGTERM: the client is told the stream is closing rather than having its socket cut, and the
// handler returns so the server's own drain can finish.
func TestTheProcessDrainingEndsTheStream(t *testing.T) {
	controller, registry, _ := streamController(t, &fakeStream{}, StreamLimits{PerProcess: 4})

	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/stream", nil))
	recorder, wait := serveStream(t, controller, request)

	// Let the handler get as far as waiting.
	time.Sleep(50 * time.Millisecond)
	registry.CloseAll()
	wait()

	if !strings.Contains(recorder.Body.String(), ": closing") {
		t.Errorf("the client was not told the stream was closing:\n%s", recorder.Body.String())
	}
	if registry.Open() != 0 {
		t.Errorf("%d streams still counted after the drain", registry.Open())
	}
}

type sseEvent struct{ id, name, data string }

// parseEvents reads the frames back the way a client would.
func parseEvents(t *testing.T, body string) []sseEvent {
	t.Helper()

	var events []sseEvent
	var current sseEvent
	scanner := bufio.NewScanner(strings.NewReader(body))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case line == "":
			if current.data != "" {
				events = append(events, current)
			}
			current = sseEvent{}
		case strings.HasPrefix(line, ":"):
			// A comment. Discarded, which is exactly the contract a keep-alive needs.
		case strings.HasPrefix(line, "id: "):
			current.id = strings.TrimPrefix(line, "id: ")
		case strings.HasPrefix(line, "event: "):
			current.name = strings.TrimPrefix(line, "event: ")
		case strings.HasPrefix(line, "data: "):
			current.data = strings.TrimPrefix(line, "data: ")
		}
	}
	return events
}
