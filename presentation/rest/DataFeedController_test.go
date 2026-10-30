// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func fetchData(t *testing.T, reader CalendarFeedReader, path string) *httptest.ResponseRecorder {
	t.Helper()
	controller := NewRestController()
	controller.DataFeeds = reader
	controller.BaseURL = "https://hubtask.example.com"
	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet, APIBasePath+path, nil)
	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// A reporting tool reads the service, the metadata and the rows without a bearer, an entry's notes
// never among them (UC-INT-15 checks 2, 3).
func TestADataFeedAnswersAnODataService(t *testing.T) {
	token := wellFormedToken(t)
	fixture := feedFixture()
	fixture.Items[0].Notes = "the landlord's private number"
	reader := &feedReader{out: fixture}
	root := "/odata/" + token

	service := fetchData(t, reader, root)
	if service.Code != http.StatusOK || service.Header().Get("OData-Version") != "4.0" {
		t.Fatalf("service %d %v: %s", service.Code, service.Header(), service.Body)
	}
	if !strings.Contains(service.Body.String(), `"https://hubtask.example.com/api/v1/odata/`+token+`/$metadata"`) {
		t.Errorf("service %s", service.Body)
	}

	metadata := fetchData(t, reader, root+"/$metadata")
	if metadata.Code != http.StatusOK || !strings.HasPrefix(metadata.Header().Get("Content-Type"), "application/xml") ||
		!strings.Contains(metadata.Body.String(), `EntitySet Name="Entries"`) {
		t.Fatalf("metadata %d: %s", metadata.Code, metadata.Body)
	}

	entries := fetchData(t, reader, root+"/Entries?$top=2")
	if entries.Code != http.StatusOK {
		t.Fatalf("entries %d: %s", entries.Code, entries.Body)
	}
	var answer struct {
		Value []map[string]any `json:"value"`
	}
	if err := json.Unmarshal(entries.Body.Bytes(), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Value) != 2 || answer.Value[0]["Title"] != "Call the landlord" || answer.Value[0]["DueAt"] != "2026-10-22T14:30:00Z" {
		t.Errorf("entries %v", answer.Value)
	}
	if strings.Contains(entries.Body.String(), "private number") {
		t.Error("an entry's notes reached the feed")
	}
}

// Every refusal is the calendar's one 404, and a malformed token costs no lookup (UC-INT-15
// check 4).
func TestADataFeedRefusesAsTheCalendarDoes(t *testing.T) {
	reader := &feedReader{err: shared.ErrNotFound.WithDetail("calendar.feed_not_found")}
	for _, path := range []string{"/odata/" + wellFormedToken(t), "/odata/" + wellFormedToken(t) + "/Entries"} {
		if got := fetchData(t, reader, path); got.Code != http.StatusNotFound {
			t.Errorf("%s answered %d", path, got.Code)
		}
	}
	quiet := &feedReader{}
	if got := fetchData(t, quiet, "/odata/not-a-token/$metadata"); got.Code != http.StatusNotFound || quiet.invoked {
		t.Errorf("a malformed token answered %d, looked up %v", got.Code, quiet.invoked)
	}
}
