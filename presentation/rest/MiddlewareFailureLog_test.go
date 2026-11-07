// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// capturedLine is one log record as the correlating handler would see it: the request ID read from
// the context the line was written with.
type capturedLine struct {
	level     slog.Level
	message   string
	requestID string
	attrs     map[string]string
}

type capturingHandler struct {
	mu    *sync.Mutex
	lines *[]capturedLine
}

func (h capturingHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h capturingHandler) Handle(ctx context.Context, record slog.Record) error {
	line := capturedLine{level: record.Level, message: record.Message,
		requestID: correlation.RequestIDFrom(ctx), attrs: map[string]string{}}
	record.Attrs(func(a slog.Attr) bool {
		line.attrs[a.Key] = a.Value.String()
		return true
	})
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.lines = append(*h.lines, line)
	return nil
}

func (h capturingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h capturingHandler) WithGroup(string) slog.Handler      { return h }

func captureLogs(t *testing.T) func() []capturedLine {
	t.Helper()
	var mu sync.Mutex
	var lines []capturedLine
	previous := slog.Default()
	slog.SetDefault(slog.New(capturingHandler{mu: &mu, lines: &lines}))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return func() []capturedLine {
		mu.Lock()
		defer mu.Unlock()
		return append([]capturedLine(nil), lines...)
	}
}

// wrappingWriter stands for a middleware between Observed and the handler that wraps the writer.
type wrappingWriter struct{ http.ResponseWriter }

func (w wrappingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// A route that calls its service directly - sync push, SAML, SCIM, media, public forms - and
// answers an internal error through WriteProblem writes one ERROR line carrying the request ID the
// client shows, the route and the cause, even through a wrapping writer.
func TestADelegatedRoutesInternalErrorIsLoggedWithTheRequestID(t *testing.T) {
	lines := captureLogs(t)
	cause := shared.ErrInternal.WithDetail("postgres.query_failed").WithCause(errors.New("driver: broken pipe"))
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sync:push", func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(wrappingWriter{w}, cause, correlation.RequestIDFrom(r.Context()))
	})
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/sync:push", nil)
	req.Header.Set(RequestIDHeader, "req-42")

	rec, _ := serve(t, mux, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", rec.Code)
	}
	got := lines()
	if len(got) != 1 {
		t.Fatalf("%d lines, want exactly one: %+v", len(got), got)
	}
	line := got[0]
	if line.level != slog.LevelError || line.requestID != "req-42" {
		t.Errorf("line %+v, want ERROR with request_id req-42", line)
	}
	if line.attrs["route"] != "POST /sync:push" || line.attrs["error_code"] != shared.ErrInternal.Code ||
		line.attrs["error"] != cause.Error() {
		t.Errorf("line attributes %v, want the route, the code and the cause", line.attrs)
	}
}

// A failure a registry use case already logged is not logged a second time.
func TestAFailureTheUseCaseLoggedIsNotLoggedAgain(t *testing.T) {
	lines := captureLogs(t)
	mux := http.NewServeMux()
	mux.HandleFunc("POST /items", func(w http.ResponseWriter, r *http.Request) {
		slog.ErrorContext(r.Context(), "the use case failed")
		correlation.NoteFailureLogged(r.Context())
		WriteProblem(w, shared.ErrInternal, correlation.RequestIDFrom(r.Context()))
	})
	serve(t, mux, httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/items", nil))

	if got := lines(); len(got) != 1 {
		t.Fatalf("%d lines, want only the use case's: %+v", len(got), got)
	}
}

// A refusal (4xx) is the API working as designed and writes no line; a dependency's 503 is a WARN.
func TestARefusalIsNotLoggedAndAnOutageIsAWarning(t *testing.T) {
	lines := captureLogs(t)
	mux := http.NewServeMux()
	mux.HandleFunc("GET /refused", func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, shared.ErrValidation, correlation.RequestIDFrom(r.Context()))
	})
	mux.HandleFunc("GET /later", func(w http.ResponseWriter, r *http.Request) {
		WriteProblem(w, shared.ErrUnavailable, correlation.RequestIDFrom(r.Context()))
	})
	serve(t, mux, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/refused", nil))
	if got := lines(); len(got) != 0 {
		t.Fatalf("a refusal wrote %+v", got)
	}
	serve(t, mux, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/later", nil))
	if got := lines(); len(got) != 1 || got[0].level != slog.LevelWarn {
		t.Fatalf("an outage wrote %+v, want one WARN", got)
	}
}

// A credential that could not be read - the database saying "later", or the client gone while it
// was asked - answers 503 through the authentication's own writers; the line is a WARN with the
// cause like any other outage, not an ERROR that names none.
func TestAnOutageWhileAuthenticatingIsAWarningWithItsCause(t *testing.T) {
	cause := shared.ErrUnavailable.WithDetail("postgres.begin_failed").WithCause(context.Canceled)
	writers := map[string]func(http.ResponseWriter, error, string){
		"bearer": WriteUnauthenticated,
		"basic":  writeBasicChallenge,
	}
	for name, write := range writers {
		t.Run(name, func(t *testing.T) {
			lines := captureLogs(t)
			mux := http.NewServeMux()
			mux.HandleFunc("GET /memberships", func(w http.ResponseWriter, r *http.Request) {
				write(wrappingWriter{w}, cause, correlation.RequestIDFrom(r.Context()))
			})
			rec, _ := serve(t, mux, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/memberships", nil))
			if rec.Code != http.StatusServiceUnavailable {
				t.Fatalf("status %d, want 503", rec.Code)
			}
			got := lines()
			if len(got) != 1 || got[0].level != slog.LevelWarn ||
				got[0].attrs["error_code"] != shared.ErrUnavailable.Code || got[0].attrs["error"] != cause.Error() {
				t.Fatalf("lines %+v, want one WARN with the code and the cause", got)
			}
		})
	}
}
