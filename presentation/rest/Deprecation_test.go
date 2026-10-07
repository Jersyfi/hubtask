// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The manifest lists every generated row, with its day: what a client reads before a field goes.
func TestTheManifestListsEveryDeprecatedField(t *testing.T) {
	listed := *deprecationManifest()
	if len(listed) != len(deprecatedFields) || len(listed) == 0 {
		t.Fatalf("the manifest lists %d of %d deprecated fields", len(listed), len(deprecatedFields))
	}
	for _, entry := range listed {
		if entry.Since.IsZero() || entry.RemovedIn == "" || len(entry.ReplacedBy) == 0 {
			t.Errorf("%s %s %s is listed without its day, version or what replaces it: %+v", entry.Method, entry.Path, entry.Field, entry)
		}
	}
}

// Every row is announced on its own route, the body reaches the handler as it was sent, and a body
// the announcement does not read passes untouched.
func TestARequestThatSendsADeprecatedFieldIsTold(t *testing.T) {
	send := func(t *testing.T, template string, body io.Reader, length int64) (http.Header, string) {
		t.Helper()
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", body)
		request.ContentLength = length
		response := httptest.NewRecorder()
		announceDeprecation(response, request, template)
		received, err := io.ReadAll(request.Body)
		if err != nil {
			t.Fatalf("the handler could not read the body: %v", err)
		}
		return response.Header(), string(received)
	}

	for _, row := range deprecatedFields {
		template := row.Method + " " + APIBasePath + row.Path
		since, err := time.Parse(time.DateOnly, row.Since)
		if err != nil {
			t.Fatalf("%s: %v", template, err)
		}
		want := "@" + strconv.FormatInt(since.Unix(), 10)
		for _, key := range []string{row.Field, strings.ToUpper(row.Field[:1]) + row.Field[1:]} {
			body := `{"` + key + `":true}`
			header, received := send(t, template, strings.NewReader(body), int64(len(body)))
			if header.Get("Deprecation") != want {
				t.Errorf("%s with %q answered Deprecation %q", template, key, header.Get("Deprecation"))
			}
			if received != body {
				t.Errorf("%s: the handler received %q, want %q", template, received, body)
			}
		}
		if header, _ := send(t, template, strings.NewReader(`{}`), 2); header.Get("Deprecation") != "" {
			t.Errorf("%s without the field answered Deprecation", template)
		}
	}

	template := "POST " + APIBasePath + "/auth/mfa:disable"
	large := `{"password":"` + strings.Repeat("x", deprecationBodyLimit) + `"}`
	if header, received := send(t, template, strings.NewReader(large), int64(len(large))); header.Get("Deprecation") != "" || received != large {
		t.Error("a body over the limit was read, or not passed on whole")
	}
	if header, received := send(t, template, strings.NewReader(large), -1); header.Get("Deprecation") != "" || received != large {
		t.Error("a chunked body over the limit was read, or not passed on whole")
	}
	for _, body := range []string{`["password"]`, `{"password":`, ``} {
		if header, received := send(t, template, strings.NewReader(body), -1); header.Get("Deprecation") != "" || received != body {
			t.Errorf("a body that is no object (%q) was announced, or changed", body)
		}
	}
}

// Once a day is set, Sunset says it in the HTTP date format (RFC 8594).
func TestASetDayIsSentAsSunset(t *testing.T) {
	saved := deprecatedByTemplate
	t.Cleanup(func() { deprecatedByTemplate = saved })
	deprecatedByTemplate = map[string][]deprecatedField{
		"POST /x": {{Field: "old", Since: "2026-10-03", Sunset: "2027-04-01"}},
	}
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/", strings.NewReader(`{"old":1}`))
	response := httptest.NewRecorder()
	announceDeprecation(response, request, "POST /x")
	if got := response.Header().Get("Sunset"); got != "Thu, 01 Apr 2027 00:00:00 GMT" {
		t.Errorf("Sunset is %q", got)
	}
}
