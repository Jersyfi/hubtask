// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/infrastructure/security"
	"github.com/Jersyfi/hubtask/presentation/mcp"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// J-13's acceptance, with a real database under it: a client completes the handshake, opens the
// stream, and is told when the workspace changed - by a change made through another channel
// entirely, which is what says the notification comes from the workspace rather than from the
// connection that caused it.

// agentStream is a recorder a test may read while the handler is still writing to it.
type agentStream struct {
	mu       sync.Mutex
	recorder *httptest.ResponseRecorder
}

func newAgentStream() *agentStream {
	return &agentStream{recorder: httptest.NewRecorder()}
}

func (a *agentStream) Header() http.Header { return a.recorder.Header() }

func (a *agentStream) Write(b []byte) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recorder.Write(b)
}

func (a *agentStream) WriteHeader(status int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recorder.WriteHeader(status)
}

func (a *agentStream) body() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recorder.Body.String()
}

func (a *agentStream) status() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.recorder.Code
}

// streamingMcp is the server as the composition root builds it: the real session issuer, the real
// registry the change stream shares, and the real listener behind LISTEN/NOTIFY.
func streamingMcp(t *testing.T, limits stream.Limits) (
	mcp.Server, *usecase.Registry, *stream.Registry,
) {
	t.Helper()

	// Its own background context rather than the caller's, as every other helper in this package
	// takes one: what these open outlives no single request, and is ended by t.Cleanup.
	ctx := context.Background()
	useCases := catalogueFor(t)

	listener := postgres.NewChangeListener(appPool(ctx, t))
	listening, stopListening := context.WithCancel(ctx)
	t.Cleanup(stopListening)
	concurrency.Go(listening, "test.mcp_change_listener", listener.Run)
	waitFor(t, 5*time.Second, "the listener to connect", listener.Connected)

	registry := stream.NewRegistry(limits)
	return mcp.Server{
		Catalogue: useCases,
		Sessions:  security.NewMcpSessionIssuer(secret.New("integration test installation secret")),
		Streams:   registry,
		Wakeups:   listener,
		Name:      "hubtask",
		Version:   "test",
	}, useCases, registry
}

// handshake completes `initialize` and answers the session the server minted.
func handshake(ctx context.Context, t *testing.T, server mcp.Server, actor appshared.ActorContext) string {
	t.Helper()

	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		http.MethodPost, mcp.Path, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`))
	recorder := httptest.NewRecorder()
	server.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("the handshake answered %d: %s", recorder.Code, recorder.Body)
	}
	session := recorder.Header().Get(mcp.SessionHeader)
	if session == "" {
		t.Fatal("the handshake handed out no session")
	}
	return session
}

// openStream holds the server-initiated stream open and answers what it has written so far.
func openStream(
	ctx context.Context, t *testing.T, server mcp.Server,
	actor appshared.ActorContext, session string,
) (*agentStream, func()) {
	t.Helper()

	request := httptest.NewRequestWithContext(appshared.ContextWithActor(ctx, actor),
		http.MethodGet, mcp.Path, nil)
	request.Header.Set("Authorization", "Bearer an-agent-token")
	if session != "" {
		request.Header.Set(mcp.SessionHeader, session)
	}

	held := newAgentStream()
	done := make(chan struct{})
	concurrency.Go(ctx, "test.mcp_stream", func(context.Context) {
		defer close(done)
		server.ServeHTTP(held, request)
	})
	return held, func() {
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("the stream handler did not return")
		}
	}
}

// The whole lifecycle, in the order a client performs it: handshake, stream, a change made
// elsewhere, a notification, a clean close.
func TestAnAgentIsToldWhenTheWorkspaceChanges(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	server, useCases, registry := streamingMcp(t, stream.Limits{PerProcess: 8})
	agent := agentFor(tenantA, authorA)

	session := handshake(ctx, t, server, agent)

	streaming, stopStreaming := context.WithCancel(ctx)
	held, wait := openStream(streaming, t, server, agent, session)
	waitFor(t, 5*time.Second, "the stream to open", func() bool { return registry.Open() == 1 })

	// The change is made through another channel entirely - a REST request, by a person - which is
	// what says the notification comes from the workspace rather than from the connection that
	// caused it.
	throughREST(ctx, t, useCases, freshName(t))

	waitFor(t, 10*time.Second, "the notification to arrive", func() bool {
		return strings.Contains(held.body(), "notifications/resources/list_changed")
	})

	// And a clean close: the client leaves, the handler returns, and the slot goes back.
	stopStreaming()
	wait()
	if registry.Open() != 0 {
		t.Errorf("%d streams still counted after the client left", registry.Open())
	}
}

// The session is checked against the actor presenting it, with the real issuer: a stream opened
// with the workspace next door's session is a not-found, whatever token it carries.
func TestAStreamCannotBorrowAnotherWorkspacesSession(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	server, _, _ := streamingMcp(t, stream.Limits{PerProcess: 8})

	// A session minted for the workspace next door, presented by this one.
	borrowed := handshake(ctx, t, server, agentFor(tenantB, authorA))

	streaming, stopStreaming := context.WithCancel(ctx)
	defer stopStreaming()
	held, wait := openStream(streaming, t, server, agentFor(tenantA, authorA), borrowed)
	wait()

	if held.status() != http.StatusNotFound {
		t.Errorf("a borrowed session opened a stream: %d", held.status())
	}
	if strings.Contains(held.body(), "list_changed") {
		t.Error("a borrowed session received a notification")
	}
}

// A change in one workspace is not a notification in another. The listener is per workspace and the
// stream subscribes to the actor's own, so this is the tenant boundary on the streaming half.
func TestAChangeNextDoorIsNotAnAgentsNotification(t *testing.T) {
	ctx := context.Background()
	seedMemberships(ctx, t)

	server, useCases, registry := streamingMcp(t, stream.Limits{PerProcess: 8})
	agent := agentFor(tenantB, authorA)
	session := handshake(ctx, t, server, agent)

	streaming, stopStreaming := context.WithCancel(ctx)
	defer stopStreaming()
	held, wait := openStream(streaming, t, server, agent, session)
	waitFor(t, 5*time.Second, "the stream to open", func() bool { return registry.Open() == 1 })

	// A change in the workspace next door.
	throughREST(ctx, t, useCases, freshName(t))

	// Long enough that a notification meant for the other workspace would have arrived.
	time.Sleep(500 * time.Millisecond)
	if strings.Contains(held.body(), "list_changed") {
		t.Errorf("a change next door woke this workspace's agent: %q", held.body())
	}

	stopStreaming()
	wait()
}
