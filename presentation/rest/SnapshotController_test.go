// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// fakeSnapshotter is the pull service with the whole walk: it hands out the records it was given,
// one at a time, and fails where it was told to.
type fakeSnapshotter struct {
	fakePuller
	snapshot syncservice.SnapshotRequest
	records  []syncservice.Record
	cursor   syncservice.Position
	failAt   int
	refuse   error
}

func (f *fakeSnapshotter) WalkAll(
	_ context.Context, _ appshared.ActorContext, request syncservice.SnapshotRequest,
	emit func(syncservice.Record) error,
) (syncservice.Position, error) {
	f.snapshot = request
	if f.refuse != nil {
		return syncservice.Position{}, f.refuse
	}
	for i, record := range f.records {
		if f.failAt > 0 && i == f.failAt {
			return syncservice.Position{}, errors.New("the reader went away")
		}
		if err := emit(record); err != nil {
			return syncservice.Position{}, err
		}
	}
	return f.cursor, nil
}

func snapshotRecords(n int) []syncservice.Record {
	records := make([]syncservice.Record, 0, n)
	for i := range n {
		id := shared.MustParseID("0192f000-0000-7000-8000-0000000000" + string(rune('a'+i%6)) + string(rune('0'+i%10)))
		records = append(records, syncservice.Record{Recorded: repository.Recorded{Change: repository.Change{
			Entity: "item", EntityID: id, Op: repository.Upsert, Payload: map[string]any{"title": "Entry"},
		}}})
	}
	return records
}

func snapshot(t *testing.T, snapshotter *fakeSnapshotter, registry *stream.Registry, body string, authenticate bool) (
	*httptest.ResponseRecorder, *pullSignals, *streamSignals,
) {
	t.Helper()
	signals, streamed := &pullSignals{}, &streamSignals{}
	controller := NewRestController()
	controller.Sync = &SyncController{Pull: snapshotter, Signals: signals, Registry: registry, StreamSignals: streamed}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/sync:snapshot", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer a-device-token")
	if authenticate {
		request = authenticated(request)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder, signals, streamed
}

func bufioWriter(size int) *bufio.Writer { return bufio.NewWriterSize(io.Discard, size) }

func lines(body string) []string {
	return strings.Split(strings.TrimSuffix(body, "\n"), "\n")
}

func TestASnapshotStreamsTheRecordsAndTheCursorLast(t *testing.T) {
	snapshotter := &fakeSnapshotter{records: snapshotRecords(5), cursor: syncservice.Position{Seq: 42}}
	registry := stream.NewRegistry(stream.Limits{PerCredential: 1, PerTenant: 1, PerProcess: 1})

	recorder, signals, streamed := snapshot(t, snapshotter, registry,
		`{"device_id":"0192f000-0000-7000-8000-0000000000d1","platform":"hubctl","scopes":[{"container_id":"0192f000-0000-7000-8000-0000000000c1","depth":"SELF"}]}`, true)

	if recorder.Code != http.StatusOK || recorder.Header().Get("Content-Type") != "application/x-ndjson" {
		t.Fatalf("answered %d %s: %s", recorder.Code, recorder.Header().Get("Content-Type"), recorder.Body.String())
	}
	got := lines(recorder.Body.String())
	if len(got) != 6 {
		t.Fatalf("%d lines, want five records and the cursor:\n%s", len(got), recorder.Body.String())
	}
	for _, line := range got[:5] {
		var change map[string]any
		if err := json.Unmarshal([]byte(line), &change); err != nil || change["entity"] != "item" || change["op"] != "UPSERT" {
			t.Errorf("a record line is %s (%v)", line, err)
		}
	}
	if got[5] != `{"cursor":"cursor-42"}` {
		t.Errorf("the last line is %s", got[5])
	}
	// The request travelled whole, and the connection was counted with the streams'.
	if snapshotter.snapshot.DeviceID.String() != "0192f000-0000-7000-8000-0000000000d1" || snapshotter.snapshot.Platform != "hubctl" ||
		len(snapshotter.snapshot.Scopes) != 1 || snapshotter.snapshot.Scopes[0].Depth != syncservice.DepthSelf {
		t.Errorf("the service was asked %+v", snapshotter.snapshot)
	}
	if signals.records != 5 || streamed.opened != 1 || streamed.closed != 1 {
		t.Errorf("counted %d records, %d opened, %d closed", signals.records, streamed.opened, streamed.closed)
	}
	if registry.Open() != 0 {
		t.Error("the slot was not released")
	}
}

// A connection that dies mid-walk ends without the cursor line: what was written is a device's
// problem to start over from, never something it may resume.
func TestASnapshotCutShortAnswersNoCursor(t *testing.T) {
	snapshotter := &fakeSnapshotter{records: snapshotRecords(5), cursor: syncservice.Position{Seq: 42}, failAt: 3}
	registry := stream.NewRegistry(stream.Limits{PerCredential: 1, PerTenant: 1, PerProcess: 1})

	recorder, _, streamed := snapshot(t, snapshotter, registry, `{"device_id":"0192f000-0000-7000-8000-0000000000d1"}`, true)

	if recorder.Code != http.StatusOK {
		t.Fatalf("answered %d", recorder.Code)
	}
	got := lines(recorder.Body.String())
	if len(got) != 3 || strings.Contains(recorder.Body.String(), "cursor") {
		t.Errorf("a cut snapshot answered:\n%s", recorder.Body.String())
	}
	if streamed.closed != 1 || registry.Open() != 0 {
		t.Error("the connection was not closed and released")
	}
}

// A refusal before the first record is an ordinary problem document.
func TestASnapshotRefusedBeforeTheFirstRecordIsAProblem(t *testing.T) {
	snapshotter := &fakeSnapshotter{refuse: shared.ErrUnavailable.WithDetail("sync.initial_sync_unavailable")}
	registry := stream.NewRegistry(stream.Limits{PerCredential: 1, PerTenant: 1, PerProcess: 1})

	recorder, _, _ := snapshot(t, snapshotter, registry, `{"device_id":"0192f000-0000-7000-8000-0000000000d1"}`, true)
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), "sync.initial_sync_unavailable") {
		t.Errorf("answered %d: %s", recorder.Code, recorder.Body.String())
	}
	recorder, _, _ = snapshot(t, snapshotter, registry, `{"device_id":"0192f000-0000-7000-8000-0000000000d1"}`, false)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("an unauthenticated caller was answered %d", recorder.Code)
	}
}

// A snapshot is bounded the way a stream is: the same registry, the same refusal.
func TestASnapshotIsRefusedWhereTheStreamsHoldTheirComplement(t *testing.T) {
	snapshotter := &fakeSnapshotter{records: snapshotRecords(1)}
	registry := stream.NewRegistry(stream.Limits{PerCredential: 1, PerTenant: 1, PerProcess: 1})
	held, refusal := registry.Admit("a-device-token", streamTenant.String())
	if refusal != stream.RefusedNone {
		t.Fatal(refusal)
	}
	defer held.Release()

	recorder, _, streamed := snapshot(t, snapshotter, registry, `{"device_id":"0192f000-0000-7000-8000-0000000000d1"}`, true)
	if recorder.Code != http.StatusServiceUnavailable || recorder.Header().Get("Retry-After") == "" ||
		!strings.Contains(recorder.Body.String(), "sync.stream_unavailable") {
		t.Errorf("answered %d %s: %s", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Body.String())
	}
	if len(streamed.refusals()) != 1 || streamed.opened != 0 {
		t.Errorf("counted %v refusals and %d opened", streamed.refusals(), streamed.opened)
	}
}

// The byte budget ends a snapshot without the cursor.
func TestASnapshotPastItsByteBudgetEndsWithoutTheCursor(t *testing.T) {
	sink := &snapshotSink{buffer: bufioWriter(64), budget: 40}
	if err := sink.line(map[string]any{"a": "twelve chars"}); err != nil {
		t.Fatalf("the first line: %v", err)
	}
	if err := sink.line(map[string]any{"b": "twelve chars"}); !errors.Is(err, errSnapshotBudget) {
		t.Errorf("the second line: %v", err)
	}
}
