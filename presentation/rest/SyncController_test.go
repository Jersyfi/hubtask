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
