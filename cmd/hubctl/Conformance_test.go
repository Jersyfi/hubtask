// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// conformingServer is a server that keeps §9 the way the real one does, in memory, with one
// switch per requirement to break it - so that the runner is proved to name the requirement a
// broken server breaks, and no other.
type conformingServer struct {
	mu      sync.Mutex
	broken  map[int]bool
	items   map[string]map[string]any
	ops     map[string]map[string]any
	revoked bool
	hub     string
}

const (
	stubHub        = "01936f2a-7c1e-7000-8000-0000000000f1"
	stubCollection = "01936f2a-7c1e-7000-8000-0000000000f2"
	stubAccount    = "01936f2a-7c1e-7000-8000-0000000000f3"
	stubToken      = "01936f2a-7c1e-7000-8000-0000000000f4"
	stubMembership = "01936f2a-7c1e-7000-8000-0000000000f5"
	secondToken    = "hbt_pat_second"
)

func newConformingServer(broken ...int) *conformingServer {
	s := &conformingServer{broken: map[int]bool{}, items: map[string]map[string]any{}, ops: map[string]map[string]any{}, hub: stubHub}
	for _, requirement := range broken {
		s.broken[requirement] = true
	}
	return s
}

func (s *conformingServer) handle(w http.ResponseWriter, r *http.Request, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	write := func(status int, v any) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(v)
	}
	path := strings.TrimPrefix(r.URL.Path, APIPath)
	second := r.Header.Get("Authorization") == "Bearer "+secondToken
	switch {
	case r.Method == http.MethodPost && path == containersPath:
		var in map[string]any
		_ = json.Unmarshal([]byte(body), &in)
		id := stubHub
		if in["type"] == "COLLECTION" {
			id = stubCollection
		}
		write(201, map[string]any{"id": id, "type": in["type"], "name": in["name"], "order_key": "a0", "created_at": "2026-09-16T08:00:00Z", "updated_at": "2026-09-16T08:00:00Z", "version": 1, "completion_policy": "MANUAL", "created_by": stubAccount})
	case r.Method == http.MethodPost && path == serviceAccountPath:
		write(201, map[string]any{"id": stubAccount, "kind": "SERVICE_ACCOUNT", "display_name": "second", "status": "ACTIVE", "created_at": "2026-09-16T08:00:00Z"})
	case r.Method == http.MethodPost && path == tokensPath:
		write(201, map[string]any{"id": stubToken, "name": "t", "account_id": stubAccount, "scopes": []string{"items:read"}, "created_at": "2026-09-16T08:00:00Z", "expires_at": "2026-09-17T08:00:00Z", "token": secondToken})
	case r.Method == http.MethodPost && path == membershipsPath:
		write(201, map[string]any{"id": stubMembership, "account_id": stubAccount, "scope_type": "HUB", "scope_id": stubHub, "role": "MEMBER"})
	case r.Method == http.MethodDelete:
		if strings.HasPrefix(path, membershipsPath+"/") {
			s.revoked = true
		}
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodGet && strings.HasPrefix(path, itemsPath+"/"):
		item, found := s.items[strings.TrimPrefix(path, itemsPath+"/")]
		if !found {
			problemJSON(w, 404, map[string]any{"status": 404, "code": "not_found", "detail_code": "items.not_found"})
			return
		}
		write(200, item)
	case r.Method == http.MethodPost && path == syncPullPath:
		var in map[string]any
		_ = json.Unmarshal([]byte(body), &in)
		cursor, _ := in["cursor"].(string)
		if cursor == "not-a-cursor" {
			problemJSON(w, 422, map[string]any{"status": 422, "code": "validation", "detail_code": "sync.cursor_invalid"})
			return
		}
		changes := []map[string]any{}
		if cursor == "" {
			changes = append(changes,
				map[string]any{"seq": 1, "entity": "container", "entity_id": stubHub, "op": "UPSERT", "container_id": stubHub},
				map[string]any{"seq": 2, "entity": "container", "entity_id": stubCollection, "op": "UPSERT", "container_id": stubCollection})
			if s.broken[4] {
				changes = changes[:1]
			}
		}
		if second && s.revoked && !s.broken[3] {
			changes = append(changes, map[string]any{"seq": 3, "entity": "container", "entity_id": stubHub, "op": "ACCESS_REVOKED", "container_id": stubHub, "actor_id": stubAccount})
		}
		write(200, map[string]any{"changes": changes, "cursor": "c-" + cursor, "has_more": false})
	case r.Method == http.MethodPost && path == syncSnapshotPath:
		// The snapshot is the walk as one stream: the same two containers, the cursor last -
		// unless broken into cutting one record, or the cursor line, short.
		w.Header().Set("Content-Type", "application/x-ndjson")
		w.WriteHeader(http.StatusOK)
		lines := []string{
			`{"entity":"container","entity_id":"` + stubHub + `","op":"UPSERT","container_id":"` + stubHub + `"}`,
			`{"entity":"container","entity_id":"` + stubCollection + `","op":"UPSERT","container_id":"` + stubCollection + `"}`,
			`{"cursor":"c-snapshot"}`,
		}
		if s.broken[4] {
			lines = lines[:1]
		}
		_, _ = w.Write([]byte(strings.Join(lines, "\n") + "\n"))
	case r.Method == http.MethodPost && path == syncPushPath:
		var in struct {
			Mutations []map[string]any `json:"mutations"`
		}
		_ = json.Unmarshal([]byte(body), &in)
		// The frame refuses a field it does not know by name, the REST layer's rule - unless
		// broken into swallowing it.
		for _, m := range in.Mutations {
			if _, unknown := m["a_field_of_a_later_version"]; unknown && !s.broken[7] {
				problemJSON(w, 422, map[string]any{"status": 422, "code": "validation", "detail_code": "usecase.input_invalid",
					"field_errors": []map[string]any{{"path": "/a_field_of_a_later_version", "code": "usecase.field_unknown"}}})
				return
			}
		}
		results := []map[string]any{}
		for _, m := range in.Mutations {
			results = append(results, s.apply(m, second))
		}
		write(200, map[string]any{"results": results, "cursor": "c9"})
	default:
		problemJSON(w, 404, map[string]any{"status": 404, "code": "not_found", "detail_code": "route.unknown"})
	}
}

func (s *conformingServer) apply(m map[string]any, second bool) map[string]any {
	op, _ := m["op_id"].(string)
	kind, _ := m["kind"].(string)
	item, _ := m["item_id"].(string)
	rejected := func(code, message string) map[string]any {
		return map[string]any{"op_id": op, "result": "REJECTED", "error": map[string]any{"code": code, "message_code": message}}
	}
	if op == "" {
		if s.broken[2] {
			return map[string]any{"op_id": "01936f2a-7c1e-7000-8000-0000000000f9", "result": "APPLIED"}
		}
		return rejected("validation", "sync.op_id_required")
	}
	if seen, found := s.ops[op]; found {
		if s.broken[2] {
			s.items[item]["version"] = 2
		}
		return seen
	}
	var result map[string]any
	switch kind {
	case "ITEM_CREATE":
		payload, _ := m["payload"].(map[string]any)
		title, _ := payload["title"].(string)
		_, unknownInPayload := payload["a_field_of_a_later_version"]
		switch {
		case unknownInPayload && !s.broken[7]:
			result = rejected("validation", "usecase.field_unknown")
		case second && s.revoked:
			result = rejected("forbidden", "access.not_permitted")
			if s.broken[3] {
				result = map[string]any{"op_id": op, "result": "APPLIED", "entity_id": item}
			}
		case title == "":
			result = rejected("validation", "items.title_required")
			if s.broken[5] {
				result = map[string]any{"op_id": op, "result": "REJECTED"}
			}
		default:
			stored := map[string]any{"id": item, "type": "TASK", "title": title, "version": 1, "collection_id": stubCollection, "completion": map[string]any{"is_completed": false}, "created_at": "2026-09-16T08:00:00Z", "updated_at": "2026-09-16T08:00:00Z", "path": "/" + item + "/", "depth": 1, "order_key": "a0", "created_by": stubAccount}
			if s.broken[1] {
				// Once: the first creation is requirement one's, and a server that renamed
				// every entry would fail every check that reads one back, which proves less.
				item = "01936f2a-7c1e-7000-8000-0000000000fa"
				stored["id"] = item
				s.broken[1] = false
			}
			s.items[item] = stored
			result = map[string]any{"op_id": op, "result": "APPLIED", "entity_id": item, "server_state": stored}
		}
	case "ITEM_PATCH":
		// Last writer wins per field: a reading later than the one the title holds applies,
		// an older one is merged away - unless the server is broken into taking it.
		state := s.items[item]
		fields, _ := m["fields"].(map[string]any)
		title, _ := fields["title"].(map[string]any)
		reading, _ := title["hlc"].(string)
		held, _ := state["title_hlc"].(string)
		result = map[string]any{"op_id": op, "result": "MERGED", "entity_id": item, "server_state": state}
		if reading > held || s.broken[5] {
			state["title"], state["title_hlc"] = title["value"], reading
			result["result"] = "APPLIED"
		}
	default:
		result = rejected("validation", "sync.kind_unknown")
		if s.broken[8] {
			result = map[string]any{"op_id": op, "result": "APPLIED"}
		}
	}
	s.ops[op] = result
	return result
}

func runConformance(t *testing.T, server *conformingServer, args ...string) (int, string, string) {
	t.Helper()
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) { server.handle(w, r, stub.body) })
	env := signedIn(stub)
	env[envProfile] = filepath.Join(t.TempDir(), "profile.json")
	return invokeAgainst(t, stub, env, "", append([]string{"sync-conformance"}, args...)...)
}

// A conforming server passes every check the runner can make, and the report says which one it
// cannot; a server that breaks one requirement fails exactly that check.
func TestTheConformanceRunnerNamesExactlyTheRequirementAServerBreaks(t *testing.T) {
	report := filepath.Join(t.TempDir(), "report.md")
	code, out, errOut := runConformance(t, newConformingServer(), "--report", report)
	if code != exitOK {
		t.Fatalf("a conforming server failed: exit %d\n%s\n%s", code, out, errOut)
	}
	for _, want := range []string{"| 1 |", "| 8 |", "**not-tested**", "| Checks | 8, 0 failed"} {
		if !strings.Contains(out, want) {
			t.Errorf("the report lacks %q:\n%s", want, out)
		}
	}
	if written, err := os.ReadFile(report); err != nil || string(written) != out {
		t.Errorf("the report file differs from standard output (%v)", err)
	}

	for _, requirement := range []int{1, 2, 3, 4, 5, 7, 8} {
		code, out, _ := runConformance(t, newConformingServer(requirement))
		if code != exitError {
			t.Errorf("requirement %d broken: exit %d, want a failure", requirement, code)
		}
		failed := 0
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, "| **fail** |") {
				failed++
				if !strings.HasPrefix(line, "| "+string(rune('0'+requirement))+" |") {
					t.Errorf("requirement %d broken: the report fails another check: %s", requirement, line)
				}
			}
		}
		if failed != 1 {
			t.Errorf("requirement %d broken: %d checks failed, want exactly one", requirement, failed)
		}
	}
}
