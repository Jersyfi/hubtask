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
	bodies  []showcaseWrite
}

// showcaseWrite is one create the showcase sent, with its body.
type showcaseWrite struct {
	path string
	body map[string]any
}

const showcaseSelf = "0190a000-0000-7000-8000-00000000aaaa"
const showcaseSprint = "0190a000-0000-7000-8000-00000000bbbb"

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
		case r.Method == http.MethodGet && path == "/accounts/me":
			answer(map[string]any{"id": showcaseSelf, "display_name": "Jana Krüger"})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/sprints"):
			answer(map[string]any{"data": []any{map[string]any{"id": showcaseSprint, "state": "ACTIVE"}}})
		case r.Method == http.MethodGet && path == "/goal-periods":
			answer(map[string]any{"data": s.periods})
		case r.Method == http.MethodPost && path == "/goal-periods":
			var period map[string]any
			_ = json.Unmarshal(raw, &period)
			period["id"] = "0190a000-0000-7000-8000-0000000fffff"
			s.periods = append(s.periods, period)
			s.creates = append(s.creates, path)
			answer(period)
		case r.Method == http.MethodGet && (strings.HasPrefix(path, "/goals/") || strings.HasPrefix(path, "/dashboards/")):
			// The showcase's own check-ins moved it on since it was made at version 1.
			answer(map[string]any{"id": strings.Split(path, "/")[2], "version": 3})
		case r.Method == http.MethodGet:
			answer(map[string]any{"data": []any{}, "version": 0})
		case strings.HasSuffix(path, ":plan"):
			answer(map[string]any{"plan_hash": "h", "document": map[string]any{}, "template": "t", "mode": "MERGE"})
		default:
			if r.Method == http.MethodPost && !strings.Contains(path, ":") {
				s.creates = append(s.creates, path)
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				s.bodies = append(s.bodies, showcaseWrite{path: path, body: body})
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
		if (m["kind"] == "GOAL" || m["kind"] == "DASHBOARD") && m["version"] != float64(3) {
			t.Errorf("%s was marked at %v, not at the version its follow-ups left it at", key, m["version"])
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

// With --voices, what other people say is left to them in order - their comments and a
// requester's request - and the signed-in person's writes that wait for one follow it.
func TestTheShowcaseLeavesOtherPeoplesWordsToThem(t *testing.T) {
	s := serveShowcase(t)
	code, out, errOut := invoke(t, s.env(), "", "--json", "workspace", "demo", "--workspace", "nordlicht", "--voices")
	if code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	for _, path := range s.creates {
		if strings.HasSuffix(path, "/comments") || path == "/requests" {
			t.Errorf("%s was written by the signed-in person", path)
		}
	}
	var answer struct {
		Voices []demoVoice `json:"voices"`
	}
	if err := json.Unmarshal([]byte(out), &answer); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	speakers, requests, waiting, marks := map[string]bool{}, 0, 0, 0
	for i, v := range answer.Voices {
		speakers[v.Speaker] = true
		if v.Path == "/requests" {
			requests++
			if v.Mark == nil || v.Speaker == "" {
				t.Errorf("voice %d: a request without its requester or mark: %+v", i, v)
			}
		}
		if v.Speaker == "" && strings.Contains(v.Path, "{voice:") {
			waiting++
		}
		if v.Method == "MARK" {
			marks++
		}
		if v.Path == "/time-records" && v.Mark != nil {
			t.Errorf("voice %d: another person's time is marked, which nobody else could remove", i)
		}
	}
	for _, who := range []string{"Mehmet Yılmaz", "Amelia Hughes", "Hannah Albers", "Priya Natarajan"} {
		if !speakers[who] {
			t.Errorf("%s says nothing: %v", who, speakers)
		}
	}
	if requests != 4 || waiting == 0 || marks < 4 {
		t.Errorf("%d requests, %d waiting writes, %d marks after the voices", requests, waiting, marks)
	}
}

// user-experience.md §3.6: the showcase is how the owner sees what exists, so the person it is run
// as gets work of their own that Today shows (§3.3) - due today in the running sprint, overdue,
// coming up and a desk incident - and, with --voices too, their own time this week.
func TestTheShowcaseGivesThePersonItRunsAsWorkOfTheirOwn(t *testing.T) {
	for _, voices := range []bool{false, true} {
		s := serveShowcase(t)
		args := []string{"workspace", "demo", "--workspace", "nordlicht"}
		if voices {
			args = append(args, "--voices")
		}
		if code, out, errOut := invoke(t, s.env(), "", args...); code != exitOK {
			t.Fatalf("voices %v: exit %d: %s %s", voices, code, out, errOut)
		}
		mine, due, incident, sprint, time := 0, map[string]bool{}, false, false, 0
		for _, w := range s.bodies {
			switch {
			case w.path == "/items" && w.body["assignee_id"] == showcaseSelf:
				mine++
				if at, ok := w.body["due_at"].(string); ok {
					due[at[:10]] = true
				}
				if w.body["entry_type"] == "incident" {
					incident = true
				}
			case w.path == "/sprints/"+showcaseSprint+"/entries":
				sprint = true
			case w.path == "/time-records" && w.body["entry_id"] != nil:
				time++
			}
		}
		if mine < 4 || len(due) < 3 || !incident || !sprint || time == 0 {
			t.Errorf("voices %v: %d entries on the person, due on %v, incident %v, in the running sprint %v, %d own time records",
				voices, mine, due, incident, sprint, time)
		}
	}
}
