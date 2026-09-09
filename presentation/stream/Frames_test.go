// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package stream_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/presentation/stream"
)

// The framing both long-lived connections share. What is asked here is the protocol: the fields, the
// blank line that ends a frame, and the one thing that would be a vulnerability rather than a bug -
// a value carrying a newline out of its own field.

func framed(t *testing.T, write func(*stream.Writer) error) string {
	t.Helper()
	recorder := httptest.NewRecorder()
	if err := write(stream.NewWriter(recorder)); err != nil {
		t.Fatalf("writing the frame: %v", err)
	}
	return recorder.Body.String()
}

func TestAnEventCarriesItsIdentityItsNameAndItsData(t *testing.T) {
	frame := framed(t, func(w *stream.Writer) error {
		return w.Event("42", "work_item", `{"id":"x"}`)
	})

	if frame != "id: 42\nevent: work_item\ndata: {\"id\":\"x\"}\n\n" {
		t.Errorf("the frame is %q", frame)
	}
}

// A frame with nothing to identify it leaves the field out rather than sending an empty one, which
// is what a notification wants: it is not resumable and must not look as though it were.
func TestAnEventWithoutAnIdentityOmitsTheField(t *testing.T) {
	frame := framed(t, func(w *stream.Writer) error {
		return w.Event("", "", `{"method":"notifications/resources/list_changed"}`)
	})

	if strings.Contains(frame, "id:") || strings.Contains(frame, "event:") {
		t.Errorf("the frame carries empty fields: %q", frame)
	}
	if !strings.HasSuffix(frame, "\n\n") {
		t.Errorf("the frame does not end: %q", frame)
	}
}

// The one that would be a vulnerability rather than a bug. The separator of this protocol is a line
// break, so a value carrying one would end its own field and let whatever follows be read as the
// next - the same shape as a header injection.
func TestANewlineNeverEscapesItsField(t *testing.T) {
	for name, write := range map[string]func(*stream.Writer) error{
		"in the identifier": func(w *stream.Writer) error {
			return w.Event("1\nevent: forged", "real", "{}")
		},
		"in the name": func(w *stream.Writer) error {
			return w.Event("1", "real\ndata: forged", "{}")
		},
		"in the data": func(w *stream.Writer) error {
			return w.Event("1", "real", "{}\n\ndata: forged")
		},
		"as a carriage return": func(w *stream.Writer) error {
			return w.Event("1", "real\rdata: forged", "{}")
		},
		"in a comment": func(w *stream.Writer) error {
			return w.Comment("heartbeat\ndata: forged")
		},
	} {
		t.Run(name, func(t *testing.T) {
			frame := framed(t, write)

			// Counted per field rather than searched for the word: "forged" survives inside a
			// value, and that is correct - it was neutralised, not removed. What must not survive
			// is a *second line* claiming to be a field, which is what an escape produces.
			fields := map[string]int{}
			for _, line := range strings.Split(strings.TrimSuffix(frame, "\n\n"), "\n") {
				name, _, found := strings.Cut(line, ":")
				if !found {
					t.Errorf("the frame holds a line that is not a field: %q", line)
					continue
				}
				fields[name]++
			}
			for name, count := range fields {
				if count > 1 {
					t.Errorf("%d %q fields in one frame: %q", count, name, frame)
				}
			}
			// And one frame: a blank line ends a frame, so a value that produced a second one
			// produced a second event.
			if strings.Count(frame, "\n\n") != 1 {
				t.Errorf("the write produced more than one frame: %q", frame)
			}
		})
	}
}

func TestACommentIsIgnorableAndARetryIsANumber(t *testing.T) {
	if frame := framed(t, func(w *stream.Writer) error { return w.Comment("heartbeat") }); frame != ": heartbeat\n\n" {
		t.Errorf("the comment is %q", frame)
	}
	if frame := framed(t, func(w *stream.Writer) error { return w.Retry(3000) }); frame != "retry: 3000\n\n" {
		t.Errorf("the retry is %q", frame)
	}
}

// Every frame is flushed, because net/http buffers: an event written and not flushed is one the
// client gets when the next four kilobytes arrive, which for a quiet workspace is minutes later.
func TestEveryFrameIsFlushed(t *testing.T) {
	recorder := &countingFlusher{ResponseRecorder: httptest.NewRecorder()}
	writer := stream.NewWriter(recorder)

	for _, write := range []func() error{
		func() error { return writer.Retry(3000) },
		func() error { return writer.Event("1", "work_item", "{}") },
		func() error { return writer.Comment("heartbeat") },
	} {
		if err := write(); err != nil {
			t.Fatalf("writing: %v", err)
		}
	}

	if recorder.flushes != 3 {
		t.Errorf("%d flushes for three frames", recorder.flushes)
	}
}

// A write that fails is reported, which is how a connection that died without telling anybody is
// noticed at all.
func TestAFailedWriteIsReported(t *testing.T) {
	if err := stream.NewWriter(brokenPipe{httptest.NewRecorder()}).Comment("heartbeat"); err == nil {
		t.Error("a write to a dead connection reported success")
	}
}

// The headers a stream needs, and the two that are about intermediaries rather than about clients.
func TestTheStreamHeadersRefuseBufferingAndCaching(t *testing.T) {
	header := http.Header{}
	stream.Headers(header)

	for field, want := range map[string]string{
		"Content-Type":      "text/event-stream",
		"Cache-Control":     "no-store",
		"X-Accel-Buffering": "no",
	} {
		if header.Get(field) != want {
			t.Errorf("%s = %q, want %q", field, header.Get(field), want)
		}
	}
}

type countingFlusher struct {
	*httptest.ResponseRecorder
	flushes int
}

func (c *countingFlusher) Flush() { c.flushes++ }

type brokenPipe struct{ *httptest.ResponseRecorder }

func (brokenPipe) Write([]byte) (int, error) { return 0, http.ErrBodyNotAllowed }
