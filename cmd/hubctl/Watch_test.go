// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// oneEvent frames one change record the way presentation/rest/ServerSentEvents.go does.
const oneEvent = "id: cursor-1\nevent: item\ndata: {\"seq\":1,\"entity\":\"item\"," +
	"\"entity_id\":\"" + itemID + "\",\"op\":\"UPSERT\"," +
	"\"occurred_at\":\"2026-08-25T10:00:00Z\",\"container_id\":\"" + collectionID + "\"}\n\n"

// TestWatchingPrintsEventsAndResumesWithTheCursor holds the whole contract of the consumer: the
// event arrives as a line, the reconnect carries the last event's id, the server's retry pace is
// honoured, and a refusal on reconnect ends the command with the catalogue's sentence.
func TestWatchingPrintsEventsAndResumesWithTheCursor(t *testing.T) {
	var resumedFrom string
	connects := 0
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		connects++
		switch connects {
		case 1:
			if r.Header.Get("Accept") != "text/event-stream" {
				t.Errorf("Accept %q", r.Header.Get("Accept"))
			}
			w.Header().Set("Content-Type", "text/event-stream")
			// A short pace so the test does not sit out the default reconnect delay.
			_, _ = w.Write([]byte("retry: 10\n\n: heartbeat\n\n" + oneEvent))
			w.(http.Flusher).Flush()
		default:
			resumedFrom = r.Header.Get("Last-Event-ID")
			problemJSON(w, http.StatusGone, map[string]any{
				"status": http.StatusGone, "code": "gone", "detail_code": "sync.cursor_too_old",
			})
		}
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "watch")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	if resumedFrom != "cursor-1" {
		t.Errorf("the reconnect resumed from %q, want the last event's id", resumedFrom)
	}
	if !strings.Contains(out, itemID) || !strings.Contains(out, "UPSERT") {
		t.Errorf("the event was not printed: %q", out)
	}
	if !strings.Contains(out, "ENTITY") {
		t.Errorf("no header over the table: %q", out)
	}
	expected, _ := catalogue(t).Message("sync.cursor_too_old", nil)
	if !strings.Contains(errOut, expected) {
		t.Errorf("the refusal %q is not the catalogue's sentence %q", errOut, expected)
	}
}

func TestWatchingUnderJSONPipesOneDocumentPerEvent(t *testing.T) {
	connects := 0
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		connects++
		if connects == 1 {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("retry: 10\n\n" + oneEvent))
			return
		}
		problemJSON(w, http.StatusGone, map[string]any{
			"status": http.StatusGone, "code": "gone", "detail_code": "sync.cursor_too_old",
		})
	})

	_, out, _ := invokeAgainst(t, stub, signedIn(stub), "", "--json", "watch")
	line, _, _ := strings.Cut(out, "\n")
	if !strings.HasPrefix(line, `{"seq":1,`) || !strings.Contains(line, `"op":"UPSERT"`) {
		t.Errorf("the first line %q is not the record itself", line)
	}
	if strings.Contains(out, "ENTITY") {
		t.Errorf("a table header landed in the pipe: %q", out)
	}
}

// Cancelling the context - which is what Ctrl-C does in main - ends the watch cleanly even while
// it is blocked in a read, and nothing calls that an error.
func TestCancellingTheWatchWhileItIsBlockedExitsClean(t *testing.T) {
	connected := make(chan struct{})
	hold := make(chan struct{})
	defer close(hold)
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte(": heartbeat\n\n"))
		w.(http.Flusher).Flush()
		close(connected)
		<-hold
	})

	ctx, cancel := context.WithCancel(context.Background())
	cli := &CLI{
		Streams:   Streams{In: strings.NewReader(""), Out: io.Discard, Err: io.Discard},
		Env:       environment(map[string]string{}),
		Profile:   Profile{BaseURL: stub.server.URL, Token: secret.New(validToken)},
		Catalogue: catalogue(t),
		Timeout:   2 * time.Second,
	}
	done := make(chan error, 1)
	concurrency.Go(ctx, "test.watch", func(ctx context.Context) {
		done <- watchRun(ctx, cli, nil)
	})

	<-connected
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a cancelled watch returned %v, want a clean end", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the watch did not end after the cancel")
	}
}

// The parser follows the specification where the server gives it room to: several data lines are
// one event, an unknown field is ignored, and one leading space of a value is stripped.
func TestTheEventParserCutsFieldsTheWayTheSpecificationSays(t *testing.T) {
	var event sseEvent
	for _, line := range []string{"id: abc", "event: item", "data: first", "data:  second", "unknown: x"} {
		event.field(line)
	}
	if event.id != "abc" || event.name != "item" {
		t.Errorf("id %q, name %q", event.id, event.name)
	}
	// Exactly one space goes; the second one belongs to the value.
	if got := strings.Join(event.data, "\n"); got != "first\n second" {
		t.Errorf("data %q", got)
	}
}
