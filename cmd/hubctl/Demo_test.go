// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// showcaseInstallation answers the showcase's calls as an installation would in shape: every
// create an identifier and a version, the marks kept and listed back.
type showcaseInstallation struct {
	mu      sync.Mutex
	server  *httptest.Server
	next    int
	marks   map[string]map[string]any
	creates []string
	removed int
	periods []any
}

func serveShowcase(t *testing.T) *showcaseInstallation {
	t.Helper()
	s := &showcaseInstallation{marks: map[string]map[string]any{}}
	s.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		defer s.mu.Unlock()
		raw, _ := io.ReadAll(r.Body)
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		w.Header().Set("Content-Type", "application/json")
		answer := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch {
		case r.Method == http.MethodGet && path == "/tenant":
			answer(map[string]any{"slug": "nordlicht", "display_name": "Nordlicht"})
		case r.Method == http.MethodGet && path == "/tenant/samples":
			rows := []any{}
			for _, m := range s.marks {
				rows = append(rows, m)
			}
			answer(map[string]any{"samples": rows})
		case r.Method == http.MethodPost && path == "/tenant/samples":
			var mark map[string]any
			_ = json.Unmarshal(raw, &mark)
			s.marks[mark["key"].(string)] = mark
			w.WriteHeader(http.StatusCreated)
			answer(mark)
		case r.Method == http.MethodPost && path == "/tenant/samples:remove":
			s.removed++
			answer(map[string]any{"removed": len(s.marks), "kept": []any{}})
		case r.Method == http.MethodGet && path == "/goal-periods":
			answer(map[string]any{"data": s.periods})
		case r.Method == http.MethodPost && path == "/goal-periods":
			var period map[string]any
			_ = json.Unmarshal(raw, &period)
			period["id"] = "0190a000-0000-7000-8000-0000000fffff"
			s.periods = append(s.periods, period)
			s.creates = append(s.creates, path)
			answer(period)
		case r.Method == http.MethodGet:
			answer(map[string]any{"data": []any{}, "version": 0})
		case strings.HasSuffix(path, ":plan"):
			answer(map[string]any{"plan_hash": "h", "document": map[string]any{}, "template": "t", "mode": "MERGE"})
		default:
			if r.Method == http.MethodPost && !strings.Contains(path, ":") {
				s.creates = append(s.creates, path)
			}
			s.next++
			answer(map[string]any{"id": fmt.Sprintf("0190a000-0000-7000-8000-%012d", s.next), "version": 1,
				"upload": map[string]any{"method": "PUT", "url": "/upload"}})
		}
	}))
	t.Cleanup(s.server.Close)
	return s
}

func (s *showcaseInstallation) env() map[string]string {
	return map[string]string{envURL: s.server.URL, envToken: validToken}
}

// user-experience.md §3.6: the showcase guards against the wrong workspace, marks everything it
// makes, makes nothing again on a second run, and is removed in one act.
func TestTheShowcaseMarksWhatItMakesAndMakesItOnce(t *testing.T) {
	s := serveShowcase(t)
	code, _, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "other")
	if code == exitOK || len(s.creates) != 0 {
		t.Fatalf("another workspace was filled: exit %d, %d creates, %s", code, len(s.creates), errOut)
	}

	code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht")
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	kinds := map[string]int{}
	for _, m := range s.marks {
		kinds[m["kind"].(string)]++
	}
	for kind, least := range map[string]int{"CONTAINER": 12, "ENTRY": 59, "PAGE": 10, "WHITEBOARD": 3, "GOAL": 4,
		"PORTFOLIO": 2, "DASHBOARD": 2, "TIME_RECORD": 15} {
		if kinds[kind] < least {
			t.Errorf("%d marks of %s, want at least %d (%v)", kinds[kind], kind, least, kinds)
		}
	}
	for key, m := range s.marks {
		if m["kind"] == "GOAL" || m["kind"] == "TIME_RECORD" || m["kind"] == "PORTFOLIO" || m["kind"] == "DASHBOARD" {
			if m["version"] == nil {
				t.Errorf("%s has no trash and was marked without its version", key)
			}
		}
	}
	made := len(s.creates)

	code, _, errOut = invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht")
	if code != exitOK {
		t.Fatalf("second run exit %d: %s", code, errOut)
	}
	if again := s.creates[made:]; len(again) != 0 {
		t.Errorf("a second run made %d objects again: %v", len(again), again)
	}

	code, _, errOut = invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht", "--remove")
	if code != exitOK || s.removed != 1 {
		t.Errorf("remove: exit %d, %d removals, %s", code, s.removed, errOut)
	}
}
