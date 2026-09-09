// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package mcp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// The server-initiated half (J-13). What is asked here is the handshake, the binding, the caps this
// stream shares with the browser's, and the two ways a stream ends that are not the client leaving.

var (
	streamTenant  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	streamAccount = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	streamNow     = time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
)

// sessions is the issuer as this server sees it: it mints an identifier per actor and accepts only
// that actor's back, which is the whole of what the transport does with one.
type sessions struct{ issued int }

func (s *sessions) Issue(tenantID, accountID shared.ID, _ time.Time) string {
	s.issued++
	return "session:" + tenantID.String() + ":" + accountID.String()
}

func (s *sessions) Validate(session string, tenantID, accountID shared.ID, _ time.Time) error {
	if session != "session:"+tenantID.String()+":"+accountID.String() {
		return errors.New("unknown MCP session")
	}
	return nil
}

// wakeups is the change listener, driven by the test rather than by a database.
type wakeups struct {
	mu           sync.Mutex
	channel      chan struct{}
	subscribed   int
	unsubscribed int
}

func newWakeups() *wakeups { return &wakeups{channel: make(chan struct{}, 4)} }

func (w *wakeups) Subscribe(shared.ID) (<-chan struct{}, func()) {
	w.mu.Lock()
	w.subscribed++
	w.mu.Unlock()
	return w.channel, func() {
		w.mu.Lock()
		w.unsubscribed++
		w.mu.Unlock()
	}
}

func (w *wakeups) wake() { w.channel <- struct{}{} }

func (w *wakeups) counts() (int, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.subscribed, w.unsubscribed
}

type signals struct {
	mu       sync.Mutex
	opened   int
	closed   int
	refusals []string
}

func (s *signals) StreamOpened(context.Context) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.opened++
}

func (s *signals) StreamClosed(context.Context, float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed++
}

func (s *signals) StreamRefused(_ context.Context, reason string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refusals = append(s.refusals, reason)
}

func (s *signals) read() (int, int, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.opened, s.closed, append([]string(nil), s.refusals...)
}

// streamingServer is the server with everything the streaming half needs.
func streamingServer(limits stream.Limits) (Server, *wakeups, *signals, *stream.Registry) {
	woken, reported := newWakeups(), &signals{}
	registry := stream.NewRegistry(limits)

	server := serverWith(&catalogue{})
	server.Sessions = &sessions{}
	server.Streams = registry
	server.Wakeups = woken
	server.Signals = reported
	server.Clock = func() time.Time { return streamNow }
	return server, woken, reported, registry
}

// streamRequest is a GET carrying an actor, a credential and a session.
func streamRequest(ctx context.Context, session string) *http.Request {
	actorCtx := appshared.ContextWithActor(ctx, appshared.ActorContext{
		Kind: appshared.ActorAIAgent, TenantID: streamTenant, AccountID: streamAccount,
	})
	request := httptest.NewRequestWithContext(actorCtx, http.MethodGet, Path, nil)
	request.Header.Set("Authorization", "Bearer a-token")
	if session != "" {
		request.Header.Set(SessionHeader, session)
	}
	return request
}

func sessionFor(tenant, account shared.ID) string {
	return "session:" + tenant.String() + ":" + account.String()
}

// serveStream runs the handler until it returns and hands the test what it wrote. The handler *is*
// the connection, so ending it is how the test ends the stream.
func serveStream(t *testing.T, server Server, request *http.Request) *httptest.ResponseRecorder {
	t.Helper()

	recorder := httptest.NewRecorder()
	done := make(chan struct{})
	concurrency.Go(request.Context(), "test.mcp.stream", func(context.Context) {
		defer close(done)
		server.ServeHTTP(recorder, request)
	})

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream handler did not return")
	}
	return recorder
}

// The handshake hands out a session, and it is the header MCP carries it in.
func TestTheHandshakeIssuesASession(t *testing.T) {
	server, _, _, _ := streamingServer(stream.Limits{PerProcess: 4})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, Path,
		strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	request = request.WithContext(appshared.ContextWithActor(request.Context(),
		appshared.ActorContext{
			Kind: appshared.ActorAIAgent, TenantID: streamTenant, AccountID: streamAccount,
		}))

	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Header().Get(SessionHeader) != sessionFor(streamTenant, streamAccount) {
		t.Errorf("the handshake answered the session %q", recorder.Header().Get(SessionHeader))
	}
}

// A session belonging to somebody else is refused, and refused as a not-found: saying "forbidden"
// would confirm that the session exists for somebody.
func TestASessionThatIsNotThisActorsIsANotFound(t *testing.T) {
	server, _, _, _ := streamingServer(stream.Limits{PerProcess: 4})

	for name, session := range map[string]string{
		"another account's":   sessionFor(streamTenant, shared.MustParseID("0192f000-0000-7000-8000-00000000000e")),
		"another workspace's": sessionFor(shared.MustParseID("0192f000-0000-7000-8000-00000000000b"), streamAccount),
		"a forgery":           "session:made-up",
	} {
		t.Run(name, func(t *testing.T) {
			// On a call...
			request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, Path,
				strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
			request.Header.Set(SessionHeader, session)
			request = request.WithContext(appshared.ContextWithActor(request.Context(),
				appshared.ActorContext{
					Kind: appshared.ActorAIAgent, TenantID: streamTenant, AccountID: streamAccount,
				}))
			recorder := httptest.NewRecorder()
			server.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Errorf("a call with somebody else's session answered %d", recorder.Code)
			}

			// ...and on the stream.
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			opened := serveStream(t, server, streamRequest(ctx, session))
			if opened.Code != http.StatusNotFound {
				t.Errorf("a stream with somebody else's session answered %d", opened.Code)
			}
		})
	}
}

// A stream carrying no session is not part of a conversation, and is refused rather than opened.
func TestAStreamWithoutASessionIsRefused(t *testing.T) {
	server, _, _, _ := streamingServer(stream.Limits{PerProcess: 4})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if recorder := serveStream(t, server, streamRequest(ctx, "")); recorder.Code != http.StatusNotFound {
		t.Errorf("an unbound stream answered %d", recorder.Code)
	}
}

// A change in the workspace becomes a notification, and the notification carries no payload: the
// client re-reads what it cares about, which is also what keeps a workspace's content out of a
// frame nothing has authorised the reading of.
func TestAChangeBecomesAListChangedNotification(t *testing.T) {
	server, woken, reported, _ := streamingServer(stream.Limits{PerProcess: 4})

	ctx, cancel := context.WithCancel(t.Context())
	request := streamRequest(ctx, sessionFor(streamTenant, streamAccount))

	recorder := newLockedRecorder()
	done := make(chan struct{})
	concurrency.Go(request.Context(), "test.mcp.stream", func(context.Context) {
		defer close(done)
		server.ServeHTTP(recorder, request)
	})

	// Woken twice, then the client leaves.
	woken.wake()
	woken.wake()
	waitFor(t, func() bool {
		return strings.Count(recorder.body(), "list_changed") == 2
	}, "the notifications were not written")
	cancel()
	<-done

	body := recorder.body()
	if !strings.Contains(body, `"method":"notifications/resources/list_changed"`) {
		t.Fatalf("the stream carried %q", body)
	}
	if strings.Contains(body, "params") {
		t.Errorf("the notification carries a payload: %q", body)
	}
	if opened, closed, _ := reported.read(); opened != 1 || closed != 1 {
		t.Errorf("the stream reported %d opened and %d closed", opened, closed)
	}
	if subscribed, unsubscribed := woken.counts(); subscribed != 1 || unsubscribed != 1 {
		t.Errorf("%d subscriptions and %d releases", subscribed, unsubscribed)
	}
}

// The caps are the change stream's caps. An agent that opened more than its share is refused by the
// same rule and counted in the same metric.
func TestTheAgentStreamIsBoundedByTheSameCaps(t *testing.T) {
	server, _, reported, registry := streamingServer(stream.Limits{PerProcess: 1})

	// One slot taken by anything at all - it does not matter what, which is the point.
	taken, refusal := registry.Admit("somebody-else", "another-tenant")
	if refusal != stream.RefusedNone {
		t.Fatalf("the first slot was refused: %s", refusal)
	}
	defer taken.Release()

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	recorder := serveStream(t, server, streamRequest(ctx, sessionFor(streamTenant, streamAccount)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("a stream over the cap answered %d", recorder.Code)
	}
	if recorder.Header().Get("Retry-After") == "" {
		t.Error("a refused stream does not say when to come back")
	}
	if _, _, refusals := reported.read(); len(refusals) != 1 || refusals[0] != "process" {
		t.Errorf("the refusal was reported as %v", refusals)
	}
}

// A drain closes an agent's stream the way it closes a browser's, and tells it rather than cutting
// the socket: it reconnects to another pod, and the difference between the two is a visible error
// in somebody's console.
func TestADrainClosesTheAgentStream(t *testing.T) {
	server, _, _, registry := streamingServer(stream.Limits{PerProcess: 4})

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	recorder := newLockedRecorder()
	done := make(chan struct{})
	drained := streamRequest(ctx, sessionFor(streamTenant, streamAccount))
	concurrency.Go(ctx, "test.mcp.stream", func(context.Context) {
		defer close(done)
		server.ServeHTTP(recorder, drained)
	})

	waitFor(t, func() bool { return registry.Open() == 1 }, "the stream never opened")
	registry.CloseAll()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the stream did not end when the process drained")
	}
	if !strings.Contains(recorder.body(), ": closing") {
		t.Errorf("the client was not told: %q", recorder.body())
	}
	if registry.Open() != 0 {
		t.Errorf("%d streams still counted after the handler returned", registry.Open())
	}
}

// A client ending its session closes cleanly rather than abandoning one it thinks still exists.
func TestASessionCanBeEnded(t *testing.T) {
	server, _, _, _ := streamingServer(stream.Limits{PerProcess: 4})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete, Path, nil)
	request.Header.Set(SessionHeader, sessionFor(streamTenant, streamAccount))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNoContent {
		t.Errorf("ending a session answered %d", recorder.Code)
	}
}

// An idle stream holds one goroutine - its own handler - and accumulates nothing. The stream
// spawns no writer, which is what makes this true rather than merely observed.
func TestIdleAgentStreamsDoNotAccumulateGoroutines(t *testing.T) {
	const connections = 30

	server, _, _, registry := streamingServer(stream.Limits{PerProcess: connections + 10})

	ctx, cancel := context.WithCancel(t.Context())
	var running sync.WaitGroup
	for range connections {
		running.Add(1)
		held := streamRequest(ctx, sessionFor(streamTenant, streamAccount))
		concurrency.Go(ctx, "test.mcp.stream", func(context.Context) {
			defer running.Done()
			server.ServeHTTP(httptest.NewRecorder(), held)
		})
	}
	waitFor(t, func() bool { return registry.Open() == connections }, "the streams never opened")

	before := runtime.NumGoroutine()
	// Idle for long enough that anything per-round would have run several times over.
	time.Sleep(200 * time.Millisecond)
	if after := runtime.NumGoroutine(); after > before+connections/2 {
		t.Errorf("goroutines grew from %d to %d while the streams were idle", before, after)
	}

	cancel()
	running.Wait()
	if registry.Open() != 0 {
		t.Errorf("%d streams still counted after every handler returned", registry.Open())
	}
}

// The declared capability and what the server can actually do have to agree, or a client waits for
// a message that never comes.
func TestListChangedIsClaimedOnlyWhereItCanBeSent(t *testing.T) {
	streaming, _, _, _ := streamingServer(stream.Limits{PerProcess: 4})

	for name, c := range map[string]struct {
		server Server
		claims bool
	}{
		"a server that streams and listens": {streaming, true},
		"a server with no stream":           {serverWith(&catalogue{}), false},
		"a server with no listener": func() struct {
			server Server
			claims bool
		} {
			deaf := streaming
			deaf.Wakeups = nil
			return struct {
				server Server
				claims bool
			}{deaf, false}
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			answer := rpc(t, c.server, `{"jsonrpc":"2.0","id":1,"method":"initialize"}`, true)
			result, _ := answer["result"].(map[string]any)
			capabilities, _ := result["capabilities"].(map[string]any)
			resources, _ := capabilities["resources"].(map[string]any)

			if resources["listChanged"] != c.claims {
				t.Errorf("listChanged is %v, want %v", resources["listChanged"], c.claims)
			}
		})
	}
}

// lockedRecorder is a recorder a test may read while the handler is still writing.
//
// httptest.ResponseRecorder is not safe for that, and the handler under test holds the connection
// for as long as the test wants it to - so a test that watched the body would be racing the thing
// it is watching. This is the recorder, with a lock over the two operations that meet.
type lockedRecorder struct {
	mu       sync.Mutex
	recorder *httptest.ResponseRecorder
}

func newLockedRecorder() *lockedRecorder {
	return &lockedRecorder{recorder: httptest.NewRecorder()}
}

func (l *lockedRecorder) Header() http.Header { return l.recorder.Header() }

func (l *lockedRecorder) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.recorder.Write(b)
}

func (l *lockedRecorder) WriteHeader(status int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.recorder.WriteHeader(status)
}

func (l *lockedRecorder) body() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.recorder.Body.String()
}

// waitFor polls a condition rather than sleeping for a guess, so a slow machine does not decide
// whether the test passes.
func waitFor(t *testing.T, until func() bool, complaint string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if until() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatal(complaint)
}
