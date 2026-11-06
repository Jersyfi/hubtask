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
	"time"
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
	// cis is what the hub's inventory lists; changes are the PUTs and PATCHes the showcase sent.
	cis     []any
	changes []showcaseWrite
}

// showcaseWrite is one create the showcase sent, with its body.
type showcaseWrite struct {
	path string
	body map[string]any
	// id is what the installation answered the write with.
	id string
}

const showcaseSelf = "0190a000-0000-7000-8000-00000000aaaa"
const showcaseSprint = "0190a000-0000-7000-8000-00000000bbbb"
const showcaseMacro = "0190a000-0000-7000-8000-00000000cccc"

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
		case r.Method == http.MethodGet && path == "/automation/rules":
			// What the ITIL template leaves: an event rule and the acknowledgement macro.
			answer(map[string]any{"data": []any{
				map[string]any{"id": "0190a000-0000-7000-8000-00000000cccb", "enabled": true, "trigger": map[string]any{"kind": "EVENT"},
					"actions": []any{map[string]any{"kind": "ADD_COMMENT"}}},
				map[string]any{"id": showcaseMacro, "enabled": true, "trigger": map[string]any{"kind": "MANUAL"},
					"actions": []any{map[string]any{"kind": "ADD_COMMENT"}}},
			}})
		case r.Method == http.MethodGet && strings.HasSuffix(path, "/cis"):
			answer(map[string]any{"data": s.cis})
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
			if r.Method == http.MethodPost {
				if !strings.Contains(path, ":") {
					s.creates = append(s.creates, path)
				}
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				s.bodies = append(s.bodies, showcaseWrite{path: path, body: body, id: fmt.Sprintf("0190a000-0000-7000-8000-%012d", s.next+1)})
			}
			if r.Method == http.MethodPut || r.Method == http.MethodPatch {
				var body map[string]any
				_ = json.Unmarshal(raw, &body)
				s.changes = append(s.changes, showcaseWrite{path: r.Method + " " + path, body: body})
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

// The classic project's plan is a Gantt worth showing (user-experience.md §3.6): every entry the
// showcase makes in its hub - phases, their work packages and activities, risks and change
// requests - has dates, so none stands in the plan's tray asking to be given them.
func TestTheShowcasesProjectPlanHasNoUndatedEntry(t *testing.T) {
	s := serveShowcase(t)
	if code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht"); code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	inHub := map[string]bool{}
	for _, w := range s.bodies {
		if w.path == "/containers" && w.body["type"] == "HUB" && strings.HasPrefix(str(w.body["name"]), "Neubau Logistikzentrum") {
			inHub[w.id] = true
		}
	}
	if len(inHub) != 1 {
		t.Fatalf("the classic project's hub was not made: %v", inHub)
	}
	for _, w := range s.bodies {
		if w.path == "/containers" && inHub[str(w.body["parent_id"])] {
			inHub[w.id] = true
		}
	}
	undated, entries := []string{}, 0
	for _, w := range s.bodies {
		if w.path != "/items" || (!inHub[str(w.body["collection_id"])] && !inHub[str(w.body["parent_id"])]) {
			continue
		}
		inHub[w.id] = true
		entries++
		if str(w.body["start_at"]) == "" && str(w.body["due_at"]) == "" {
			undated = append(undated, str(w.body["title"]))
		}
	}
	if entries < 20 || len(undated) > 0 {
		t.Errorf("%d entries in the project's hub, these without dates: %q", entries, undated)
	}
}

// The desk is walked from its portal and its automation as well (user-experience.md §3.6): the
// knowledge base's released articles are published to the intake's portal, the desk acknowledges
// a request with the template's macro so the run log holds a run, and one change still waits for
// the CAB, so its approvals open with something to decide.
func TestTheShowcasesDeskOpensWithArticlesARunAndAWaitingChange(t *testing.T) {
	s := serveShowcase(t)
	if code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht"); code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	intake, request := "", ""
	for _, w := range s.bodies {
		if w.path == "/containers" && w.body["name"] == "Störungen & Anfragen" {
			intake = w.id
		}
		if w.path == "/items" && w.body["collection_id"] == intake && w.body["entry_type"] == "service_request" && request == "" {
			request = w.id
		}
	}
	articles, pressed, asked := 0, false, 0
	for _, w := range s.bodies {
		switch {
		case strings.HasSuffix(w.path, "/articles") && strings.HasPrefix(w.path, "/pages/"):
			if w.body["collection_id"] != intake {
				t.Errorf("an article went to %v, not the intake %s", w.body["collection_id"], intake)
			}
			articles++
		case w.path == "/automation/rules/"+showcaseMacro+":trigger":
			pressed = w.body["item_id"] == request && request != ""
		case strings.HasSuffix(w.path, ":step") && w.body["step"] == "approve":
			asked++
		}
	}
	// The CAB decides one; the other approval stays waiting.
	if articles < 2 || !pressed || asked < 2 {
		t.Errorf("%d articles on the portal, macro pressed on the request %v, %d changes asking the CAB", articles, pressed, asked)
	}
}

// The desk's inventory is kept (user-experience.md §3.6): every CI has who answers for it and a
// next review, the hardware its class's attributes, and health finds only two believable things
// wanting - so the Assets head does not flag every CI.
func TestTheShowcasesInventoryIsKept(t *testing.T) {
	s := serveShowcase(t)
	if code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht"); code != exitOK {
		t.Fatalf("exit %d: %s %s", code, out, errOut)
	}
	made := map[string]showcaseWrite{}
	for _, w := range s.bodies {
		if strings.HasPrefix(w.path, "/hubs/") && strings.HasSuffix(w.path, "/cis") {
			made[w.id] = w
		}
	}
	if len(made) != len(demoCIs) {
		t.Fatalf("%d CIs made, want %d", len(made), len(demoCIs))
	}
	kept, overdue := map[string]bool{}, 0
	for _, w := range s.changes {
		id, ok := strings.CutSuffix(strings.TrimPrefix(w.path, "PUT /cis/"), "/compliance")
		if !ok || made[id].path == "" {
			continue
		}
		if w.body["owner_account_id"] == nil && w.body["owner_group_id"] == nil {
			t.Errorf("%v has nobody answering for it", made[id].body["name"])
		}
		due, _ := w.body["review_due"].(string)
		if due == "" {
			t.Errorf("%v has no next review", made[id].body["name"])
		} else if due < time.Now().UTC().Format(time.DateOnly) {
			overdue++
		}
		kept[id] = true
	}
	if len(kept) != len(made) || overdue != 1 {
		t.Errorf("%d of %d CIs kept, %d reviews overdue, want all and one", len(kept), len(made), overdue)
	}
	for _, w := range made {
		if attributes, _ := w.body["attributes"].(map[string]any); (w.body["class"] == "server" || w.body["class"] == "laptop") &&
			(attributes["hostname"] == nil || attributes["serial_number"] == nil || attributes["operating_system"] == nil) {
			t.Errorf("%v lacks its class's attributes: %v", w.body["name"], attributes)
		}
	}
}

// A workspace an earlier showcase filled with bare CIs has them kept on the next run; a CI
// someone has kept since is left as it is.
func TestTheShowcaseKeepsTheBareCIsOfAnEarlierRun(t *testing.T) {
	s := serveShowcase(t)
	if code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht"); code != exitOK {
		t.Fatalf("first run exit %d: %s %s", code, out, errOut)
	}
	const bare, owned = "0190a000-0000-7000-8000-0000000c0001", "0190a000-0000-7000-8000-0000000c0002"
	s.cis = []any{
		map[string]any{"id": bare, "name": "db-01.hh.intern", "attributes": map[string]any{}},
		map[string]any{"id": owned, "name": "app-01.hh.intern", "attributes": map[string]any{}, "owner_group_id": "0190a000-0000-7000-8000-0000000c0003"},
	}
	s.changes = nil
	if code, out, errOut := invoke(t, s.env(), "", "workspace", "demo", "--workspace", "nordlicht"); code != exitOK {
		t.Fatalf("second run exit %d: %s %s", code, out, errOut)
	}
	patched, compliance := false, false
	for _, w := range s.changes {
		switch w.path {
		case "PATCH /cis/" + bare:
			attributes, _ := w.body["attributes"].(map[string]any)
			patched = attributes["hostname"] == "db-01"
		case "PUT /cis/" + bare + "/compliance":
			compliance = w.body["review_due"] != nil
		case "PATCH /cis/" + owned, "PUT /cis/" + owned + "/compliance":
			t.Errorf("the kept CI was changed: %s", w.path)
		}
	}
	if !patched || !compliance {
		t.Errorf("the bare CI: attributes %v, compliance %v", patched, compliance)
	}
}
