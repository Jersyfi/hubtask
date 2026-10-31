// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	realtimeservice "github.com/Jersyfi/hubtask/core/application/service/realtime"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	tenant = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b001")
	doc    = domain.Ref{Kind: "page", ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000b002")}
)

// documents is the service in memory: a snapshot at 1, one update after it, and whatever is
// appended. Read-only when writable is false.
type documents struct {
	mu       sync.Mutex
	writable bool
	updates  []domain.Update
	ended    bool
	asked    int
}

func (d *documents) Open(context.Context, appshared.ActorContext, domain.Ref) (realtimeservice.Opened, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return realtimeservice.Opened{
		Head:     domain.Head{Seq: int64(len(d.updates)) + 1, SnapshotSeq: 1},
		Snapshot: &domain.Snapshot{UpToSeq: 1, State: []byte("snap")},
		Updates:  append([]domain.Update(nil), d.updates...),
		Writable: d.writable,
	}, nil
}

func (d *documents) Since(_ context.Context, _ appshared.ActorContext, _ domain.Ref, after int64) ([]domain.Update, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.asked++
	if d.ended {
		return nil, shared.ErrForbidden.WithDetail("realtime.access_ended")
	}
	var out []domain.Update
	for _, u := range d.updates {
		if u.Seq > after {
			out = append(out, u)
		}
	}
	return out, nil
}

func (d *documents) Append(_ context.Context, _ appshared.ActorContext, _ domain.Ref, data []byte) (realtimeservice.Appended, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	seq := int64(len(d.updates)) + 2
	d.updates = append(d.updates, domain.Update{Seq: seq, Data: data})
	return realtimeservice.Appended{Seq: seq}, nil
}

func (d *documents) Compact(context.Context, appshared.ActorContext, domain.Ref, int64, []byte) error {
	return nil
}

func (d *documents) Relayable(data []byte) error { return domain.CheckAwareness(data) }

func serving(t *testing.T, docs *documents) (*httptest.Server, *Relay) {
	t.Helper()
	relay := NewRelay()
	handler := Handler{Documents: docs, Relay: relay}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		actor := appshared.ActorContext{TenantID: tenant, Kind: shared.ActorUser}
		if err := handler.Serve(w, r, actor, doc, nil); err != nil {
			http.Error(w, "refused", http.StatusForbidden)
		}
	}))
	t.Cleanup(server.Close)
	return server, relay
}

func dial(t *testing.T, server *httptest.Server) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, "ws"+strings.TrimPrefix(server.URL, "http"),
		&websocket.DialOptions{Subprotocols: []string{Subprotocol}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func next(t *testing.T, conn *websocket.Conn) []byte {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return data
}

func send(t *testing.T, conn *websocket.Conn, message []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageBinary, message); err != nil {
		t.Fatal(err)
	}
}

func TestAConnectionStartsFromTheSnapshotAndTheLogAfterIt(t *testing.T) {
	docs := &documents{writable: true, updates: []domain.Update{{Seq: 2, Data: []byte("u2")}}}
	server, _ := serving(t, docs)
	conn := dial(t, server)

	if got := next(t, conn); !bytes.Equal(got, SyncStep2([]byte("snap"))) {
		t.Fatalf("first message %v, want the snapshot as step two", got)
	}
	if got := next(t, conn); !bytes.Equal(got, SyncUpdate([]byte("u2"))) {
		t.Fatalf("second message %v, want the update after the snapshot", got)
	}
	if got := next(t, conn); !bytes.Equal(got, SyncStep1()) {
		t.Fatalf("third message %v, want the server asking a writer for its state", got)
	}
}

func TestAWritersUpdateIsStoredAndReachesTheOtherReaderNotItself(t *testing.T) {
	docs := &documents{writable: true}
	server, _ := serving(t, docs)
	writer, reader := dial(t, server), dial(t, server)
	for _, conn := range []*websocket.Conn{writer, reader} {
		next(t, conn) // the snapshot
		next(t, conn) // step one
	}

	send(t, writer, SyncUpdate([]byte("typed")))

	deadline := time.Now().Add(5 * time.Second)
	for {
		docs.mu.Lock()
		stored := len(docs.updates)
		docs.mu.Unlock()
		if stored == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if got := next(t, reader); !bytes.Equal(got, SyncUpdate([]byte("typed"))) {
		t.Fatalf("the other reader got %v, want the writer's update", got)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 3*pollInterval)
	defer cancel()
	if _, data, err := writer.Read(ctx); err == nil {
		t.Errorf("the writer was handed %v; its own update is never echoed", data)
	}
}

func TestAReaderWhoMayNotWriteIsClosedOnAnUpdate(t *testing.T) {
	docs := &documents{writable: false}
	server, _ := serving(t, docs)
	conn := dial(t, server)
	next(t, conn) // the snapshot; no step one for a reader

	send(t, conn, SyncUpdate([]byte("typed")))

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != closeForbidden {
		t.Errorf("closed with %v, want %d", err, closeForbidden)
	}
	if len(docs.updates) != 0 {
		t.Errorf("a reader's update was stored")
	}
}

func TestAccessEndedWhileConnectedClosesTheConnection(t *testing.T) {
	docs := &documents{writable: true}
	server, _ := serving(t, docs)
	conn := dial(t, server)
	next(t, conn)
	next(t, conn)

	docs.mu.Lock()
	docs.ended = true
	docs.mu.Unlock()

	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	_, _, err := conn.Read(ctx)
	if websocket.CloseStatus(err) != closeForbidden {
		t.Errorf("closed with %v, want %d once access ended", err, closeForbidden)
	}
}

func TestAwarenessIsRelayedToTheOtherReaders(t *testing.T) {
	docs := &documents{writable: true}
	server, _ := serving(t, docs)
	first, second := dial(t, server), dial(t, server)
	for _, conn := range []*websocket.Conn{first, second} {
		next(t, conn)
		next(t, conn)
	}

	send(t, first, Awareness([]byte("cursor")))

	if got := next(t, second); !bytes.Equal(got, Awareness([]byte("cursor"))) {
		t.Errorf("the other reader got %v, want the awareness", got)
	}
}

func TestTheFramingRoundTrips(t *testing.T) {
	for _, raw := range [][]byte{SyncStep2([]byte("s")), SyncUpdate(make([]byte, 300)), Awareness([]byte("a"))} {
		message, err := Decode(raw)
		if err != nil {
			t.Fatal(err)
		}
		if message.Type == messageSync && message.Sync == syncUpdate && len(message.Payload) != 300 {
			t.Errorf("a long update decoded to %d bytes", len(message.Payload))
		}
	}
	compact := appendBytes(appendUvarint(appendUvarint(nil, messageCompact), 700), []byte("state"))
	message, err := Decode(compact)
	if err != nil || message.Seq != 700 || string(message.Payload) != "state" {
		t.Errorf("a snapshot decoded to %+v, %v", message, err)
	}
	if _, err := Decode([]byte{messageSync, syncUpdate, 5, 1}); err == nil {
		t.Error("a payload shorter than its length decoded")
	}
}

// bell is a wake-up rung by hand, for a workspace or for one document.
type bell struct{ ring chan struct{} }

func (b bell) Subscribe(shared.ID) (<-chan struct{}, func()) { return b.ring, func() {} }

func (b bell) SubscribeDocument(shared.ID, string, shared.ID) (<-chan struct{}, func()) {
	return b.ring, func() {}
}

// A busy workspace does not cost a query per open document per change (SY-E): with a document's
// own wake-up, the workspace's changes re-ask the reader's access at most every accessRecheck, and
// the document's own update catches up at once.
func TestAWorkspacesChangesDoNotReaskAccessEachTime(t *testing.T) {
	docs := &documents{writable: true}
	workspace, own := bell{make(chan struct{})}, bell{make(chan struct{})}
	handler := Handler{Documents: docs, Relay: NewRelay(), Wakeups: workspace, DocumentWakeups: own}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = handler.Serve(w, r, appshared.ActorContext{TenantID: tenant, Kind: shared.ActorUser}, doc, nil)
	}))
	t.Cleanup(server.Close)
	conn := dial(t, server)
	next(t, conn)
	next(t, conn)
	for range 20 {
		workspace.ring <- struct{}{}
	}
	docs.mu.Lock()
	asked := docs.asked
	docs.updates = append(docs.updates, domain.Update{Seq: 2, Data: []byte("theirs")})
	docs.mu.Unlock()
	if asked != 0 {
		t.Errorf("twenty changes of the workspace asked access %d times, want none within the interval", asked)
	}
	own.ring <- struct{}{}
	if got := next(t, conn); !bytes.Equal(got, SyncUpdate([]byte("theirs"))) {
		t.Errorf("the document's own wake-up handed out %v, want the update", got)
	}
}

// relayBus hands what one process publishes to another process's relay, as the listener does.
type relayBus struct{ to *Relay }

func (b relayBus) Publish(_ context.Context, tenantID shared.ID, kind string, documentID shared.ID, message []byte) error {
	b.to.Deliver(tenantID, kind, documentID, message)
	return nil
}

// SY-E: two readers of one document on two processes see each other's awareness through the bus.
func TestAwarenessCrossesToAnotherProcess(t *testing.T) {
	docs := &documents{writable: true}
	first, second := NewRelay(), NewRelay()
	first.Bus, second.Bus = relayBus{to: second}, relayBus{to: first}
	open := func(relay *Relay) *websocket.Conn {
		handler := Handler{Documents: docs, Relay: relay}
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = handler.Serve(w, r, appshared.ActorContext{TenantID: tenant, Kind: shared.ActorUser}, doc, nil)
		}))
		t.Cleanup(server.Close)
		conn := dial(t, server)
		next(t, conn)
		next(t, conn)
		return conn
	}
	here, there := open(first), open(second)
	send(t, here, Awareness([]byte("cursor-1")))
	send(t, here, Awareness([]byte("cursor-2")))
	got := next(t, there)
	if !bytes.Equal(got, Awareness([]byte("cursor-1"))) && !bytes.Equal(got, Awareness([]byte("cursor-2"))) {
		t.Fatalf("the reader on the other process got %v", got)
	}
	if !bytes.Equal(got, Awareness([]byte("cursor-2"))) {
		if latest := next(t, there); !bytes.Equal(latest, Awareness([]byte("cursor-2"))) {
			t.Errorf("the latest cursor did not follow: %v", latest)
		}
	}
}
