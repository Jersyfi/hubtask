// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// fakePuller stands in for the application service: it remembers the request it was handed and
// answers the page it was given. What the controller owes is the mapping in both directions.
type fakePuller struct {
	request syncservice.PullRequest
	page    syncservice.Page
	err     error
}

func (f *fakePuller) Pull(
	_ context.Context, _ appshared.ActorContext, request syncservice.PullRequest,
) (syncservice.Page, error) {
	f.request = request
	return f.page, f.err
}

func (f *fakePuller) Encode(position syncservice.Position) string {
	return "cursor-" + strconv.FormatInt(position.Seq, 10)
}

type pullSignals struct{ records int }

func (s *pullSignals) PullRecords(_ context.Context, count int) { s.records += count }

func pull(t *testing.T, puller *fakePuller, body string, authenticate bool) (
	*httptest.ResponseRecorder, *pullSignals,
) {
	t.Helper()
	signals := &pullSignals{}
	controller := NewRestController()
	controller.Sync = &SyncController{Pull: puller, Signals: signals}

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/sync:pull", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if authenticate {
		request = authenticated(request)
	}
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder, signals
}

func TestAPullMapsTheRequestAndThePage(t *testing.T) {
	record := streamRecord(7, "work_item")
	record.DeviceID = shared.MustParseID("0192f000-0000-7000-8000-0000000000d1")
	deletion := streamRecord(8, "label")
	deletion.Op = "DELETE"
	deletion.Payload = nil
	puller := &fakePuller{page: syncservice.Page{
		Records:    []syncservice.Record{record, deletion},
		Cursor:     syncservice.Position{Seq: 8, IssuedAt: streamNow},
		More:       true,
		ServerTime: streamNow,
		Window:     90 * 24 * time.Hour,
	}}

	recorder, signals := pull(t, puller, `{
		"device_id": "0192f000-0000-7000-8000-0000000000d1",
		"cursor": "cursor-3",
		"limit": 2,
		"scopes": [{"container_id": "0192f000-0000-7000-8000-00000000000b", "depth": "SELF"},
		           {"container_id": "0192f000-0000-7000-8000-00000000000c"}]
	}`, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}

	// The request, mapped: nothing defaulted here - the depth left empty reaches the service empty.
	if puller.request.DeviceID != "0192f000-0000-7000-8000-0000000000d1" ||
		puller.request.Cursor != "cursor-3" || puller.request.Limit != 2 {
		t.Errorf("request mapped as %+v", puller.request)
	}
	if len(puller.request.Scopes) != 2 ||
		puller.request.Scopes[0].Depth != syncservice.DepthSelf || puller.request.Scopes[1].Depth != "" {
		t.Errorf("scopes mapped as %+v", puller.request.Scopes)
	}

	var body struct {
		Changes             []map[string]any `json:"changes"`
		Cursor              string           `json:"cursor"`
		HasMore             bool             `json:"has_more"`
		ServerTime          string           `json:"server_time"`
		TombstoneWindowDays int              `json:"tombstone_window_days"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Cursor != "cursor-8" || !body.HasMore || body.TombstoneWindowDays != 90 ||
		body.ServerTime != "2026-08-24T09:00:00Z" {
		t.Errorf("page envelope %+v", body)
	}
	if len(body.Changes) != 2 {
		t.Fatalf("%d changes, want two", len(body.Changes))
	}
	first := body.Changes[0]
	if first["op"] != "UPSERT" || first["entity"] != "work_item" ||
		first["container_id"] != "0192f000-0000-7000-8000-00000000000b" ||
		first["actor_id"] != streamAccount.String() ||
		first["device_id"] != "0192f000-0000-7000-8000-0000000000d1" ||
		first["occurred_at"] != "2026-08-24T09:00:00Z" {
		t.Errorf("first change %v", first)
	}
	if payload, ok := first["payload"].(map[string]any); !ok || payload["title"] != "Review the quote" {
		t.Errorf("first payload %v", first["payload"])
	}
	// A deletion carries no payload, and the key is absent rather than null.
	if _, present := body.Changes[1]["payload"]; present {
		t.Errorf("the deletion carries a payload key: %v", body.Changes[1])
	}
	if signals.records != 2 {
		t.Errorf("%d records counted, want two", signals.records)
	}
}

func TestAPullRefusesAnUnauthenticatedCaller(t *testing.T) {
	recorder, _ := pull(t, &fakePuller{}, `{"device_id": "0192f000-0000-7000-8000-0000000000d1"}`, false)
	if recorder.Code != http.StatusUnauthorized {
		t.Errorf("status %d, want 401", recorder.Code)
	}
}

func TestAPullPassesTheServicesRefusalThrough(t *testing.T) {
	puller := &fakePuller{err: shared.ErrGone.WithDetail("sync.cursor_too_old")}
	recorder, signals := pull(t, puller, `{"device_id": "0192f000-0000-7000-8000-0000000000d1", "cursor": "x"}`, true)
	if recorder.Code != http.StatusGone {
		t.Errorf("status %d, want 410: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "sync.cursor_too_old") {
		t.Errorf("the problem does not carry the code: %s", recorder.Body.String())
	}
	if signals.records != 0 {
		t.Errorf("a refusal counted records")
	}
}

func TestAPullRefusesAnUnknownField(t *testing.T) {
	recorder, _ := pull(t, &fakePuller{}, `{"device_id": "0192f000-0000-7000-8000-0000000000d1", "cursors": "x"}`, true)
	if recorder.Code != http.StatusUnprocessableEntity {
		t.Errorf("status %d, want 422: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "usecase.field_unknown") {
		t.Errorf("the refusal does not name the field: %s", recorder.Body.String())
	}
}

// Without the controller wired, the route answers as it did before N-01: pending.
func TestAnInstallationWithoutThePullAnswersPending(t *testing.T) {
	controller := NewRestController()
	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/sync:pull", strings.NewReader(`{}`)))
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status %d, want the pending 404", recorder.Code)
	}
}

func TestTheDeviceListAndForgettingGoThroughTheCatalogue(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{{
		"id": "0192f000-0000-7000-8000-0000000000d1", "platform": "ios", "display_name": nil,
		"last_seen_at": streamNow, "last_cursor": "cursor-42", "blocked": false, "created_at": streamNow,
	}}}}
	controller := NewRestController()
	controller.UseCases = registry

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, authenticated(
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+"/sync/devices", nil)))
	if recorder.Code != http.StatusOK || registry.name != "ListSyncDevices" {
		t.Fatalf("status %d via %q: %s", recorder.Code, registry.name, recorder.Body.String())
	}
	var devices []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &devices); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if len(devices) != 1 || devices[0]["platform"] != "ios" || devices[0]["last_cursor"] != "cursor-42" ||
		devices[0]["blocked"] != false || devices[0]["display_name"] != nil {
		t.Errorf("the list is %v", devices)
	}

	registry.out = usecase.Output{}
	recorder = httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, authenticated(httptest.NewRequestWithContext(
		t.Context(), http.MethodDelete, APIBasePath+"/sync/devices/0192f000-0000-7000-8000-0000000000d1", nil)))
	if recorder.Code != http.StatusNoContent || registry.name != "ForgetSyncDevice" ||
		registry.in["device_id"] != "0192f000-0000-7000-8000-0000000000d1" {
		t.Errorf("status %d via %q with %v", recorder.Code, registry.name, registry.in)
	}
}

func TestAPullPassesWhatTheDeviceSaysAboutItself(t *testing.T) {
	puller := &fakePuller{}
	recorder, _ := pull(t, puller, `{"device_id": "0192f000-0000-7000-8000-0000000000d1", "cursor": "c",
		"platform": "hubctl", "display_name": "Anna's laptop"}`, true)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}
	if puller.request.Platform != "hubctl" || puller.request.DisplayName != "Anna's laptop" {
		t.Errorf("the request reached the service as %+v", puller.request)
	}
}

type fakePusher struct {
	request  syncservice.PushRequest
	response syncservice.PushResponse
	err      error
}

func (f *fakePusher) Push(_ context.Context, _ appshared.ActorContext, request syncservice.PushRequest) (
	syncservice.PushResponse, error,
) {
	f.request = request
	return f.response, f.err
}

func (f *fakePusher) Encode(position syncservice.Position) string {
	return "cursor-" + strconv.FormatInt(position.Seq, 10)
}

type pushSignals struct{ results []string }

func (s *pushSignals) PushResult(_ context.Context, result string) {
	s.results = append(s.results, result)
}

func TestAPushMapsTheQueueAndTheResults(t *testing.T) {
	itemID := shared.MustParseID("0192f000-0000-7000-8000-0000000000e1")
	pusher := &fakePusher{response: syncservice.PushResponse{
		Results: []syncservice.Result{
			{OpID: shared.MustParseID("0192f000-0000-7000-8000-000000000f01"), Result: "APPLIED",
				EntityID: itemID, ServerState: map[string]any{"title": "Fix the tap"}},
			{OpID: shared.MustParseID("0192f000-0000-7000-8000-000000000f02"), Result: "REJECTED",
				Error: &syncservice.ResultError{Code: "gone", MessageCode: "sync.gone"}},
		},
		Cursor: syncservice.Position{Seq: 9, IssuedAt: streamNow}, ServerTime: streamNow,
	}}
	signals := &pushSignals{}
	controller := NewRestController()
	controller.Sync = &SyncController{Push: pusher, PushSignals: signals}

	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/sync:push", strings.NewReader(`{
			"device_id": "0192f000-0000-7000-8000-0000000000d1",
			"platform": "hubctl",
			"mutations": [
				{"op_id": "0192f000-0000-7000-8000-000000000f01", "kind": "ITEM_CREATE",
				 "item_id": "0192f000-0000-7000-8000-0000000000e1", "hlc": "1757937600000:00001:dev-a",
				 "payload": {"type": "TASK", "title": "Fix the tap"}},
				{"op_id": "0192f000-0000-7000-8000-000000000f02", "kind": "ITEM_PATCH",
				 "item_id": "0192f000-0000-7000-8000-0000000000e1", "base_version": 3,
				 "fields": {"title": {"value": "x", "hlc": "1757937600000:00002:dev-a"}}}
			]}`)))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status %d: %s", recorder.Code, recorder.Body.String())
	}

	// The queue, mapped field for field.
	if pusher.request.DeviceID != "0192f000-0000-7000-8000-0000000000d1" || pusher.request.Platform != "hubctl" ||
		len(pusher.request.Mutations) != 2 {
		t.Fatalf("the request reached the service as %+v", pusher.request)
	}
	first, second := pusher.request.Mutations[0], pusher.request.Mutations[1]
	if first.Kind != "ITEM_CREATE" || first.ItemID != itemID || first.HLC != "1757937600000:00001:dev-a" ||
		first.Payload["title"] != "Fix the tap" {
		t.Errorf("the first mutation is %+v", first)
	}
	if second.Kind != "ITEM_PATCH" || second.BaseVersion == nil || *second.BaseVersion != 3 ||
		second.Fields["title"].Value != "x" || second.Fields["title"].HLC != "1757937600000:00002:dev-a" {
		t.Errorf("the second mutation is %+v", second)
	}

	var body struct {
		Results []map[string]any `json:"results"`
		Cursor  string           `json:"cursor"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decoding: %v", err)
	}
	if body.Cursor != "cursor-9" || len(body.Results) != 2 {
		t.Fatalf("the answer is %s", recorder.Body.String())
	}
	if body.Results[0]["result"] != "APPLIED" || body.Results[0]["entity_id"] != itemID.String() ||
		body.Results[0]["server_state"].(map[string]any)["title"] != "Fix the tap" {
		t.Errorf("the first result is %v", body.Results[0])
	}
	if failure, ok := body.Results[1]["error"].(map[string]any); !ok || failure["message_code"] != "sync.gone" {
		t.Errorf("the second result is %v", body.Results[1])
	}
	if len(signals.results) != 2 || signals.results[1] != "REJECTED" {
		t.Errorf("counted %v", signals.results)
	}
}

func TestAnInstallationWithoutThePushAnswersPending(t *testing.T) {
	controller := NewRestController()
	controller.Sync = &SyncController{Pull: &fakePuller{}}
	request := authenticated(httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/sync:push", strings.NewReader(`{}`)))
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNotFound {
		t.Errorf("status %d, want the pending 404", recorder.Code)
	}
}
