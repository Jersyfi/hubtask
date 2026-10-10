// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const replaceTenantID = "01936f2a-7c1e-7000-8000-0000000000a1"

func holdRecordJSON(id string) string {
	return `{"id":"` + id + `","scope":{"kind":"CONTAINER","id":"` + collectionID + `"},` +
		`"reason":"the Meier proceedings","placed_by":"` + itemID + `",` +
		`"placed_at":"2026-08-20T08:00:00Z"}`
}

func writeHoldFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "holds.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The holds go to the workspace's operation with the recovery point, and the answer says per hold
// what happened - the target and the release being what the operator acts on.
func TestReplacingHoldsSendsTheFileToTheWorkspace(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"holds":[{"id":"`+holdID+`","outcome":"PLACED",`+
		`"target_present":false,"released_in_period":true}]}`)
	path := writeHoldFile(t, `[`+holdRecordJSON(holdID)+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "replace-holds", replaceTenantID,
		"--recovery-point", "2026-08-16T00:00:00Z", "--from", path)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := "/admin/tenants/" + replaceTenantID + ":replace-legal-holds"; !strings.HasSuffix(stub.request.URL.Path, want) {
		t.Errorf("posted to %s", stub.request.URL.Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	holds, _ := sent["holds"].([]any)
	if sent["recovery_point"] != "2026-08-16T00:00:00Z" || len(holds) != 1 {
		t.Errorf("sent %v", sent)
	}
	if !strings.Contains(out, "PLACED") || !strings.Contains(out, "true") {
		t.Errorf("output %q", out)
	}
}

// More holds than one call takes go in several, and every answer reaches the output.
func TestReplacingHoldsGoesInBatches(t *testing.T) {
	var mu sync.Mutex
	var calls []int
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, _ *http.Request) {
		// The stub has read the body already; it keeps it for the handler.
		var body struct {
			Holds []map[string]any `json:"holds"`
		}
		_ = json.Unmarshal([]byte(stub.body), &body)
		mu.Lock()
		calls = append(calls, len(body.Holds))
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"holds":[]}`))
	})
	records := make([]string, 0, replaceBatch+1)
	for i := range replaceBatch + 1 {
		records = append(records, holdRecordJSON(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012d", i)))
	}
	path := writeHoldFile(t, `{"holds":[`+strings.Join(records, ",")+`]}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "replace-holds", replaceTenantID,
		"--recovery-point", "2026-08-16T00:00:00Z", "--from", path)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(calls) != 2 || calls[0] != replaceBatch || calls[1] != 1 {
		t.Errorf("calls carried %v holds", calls)
	}
}

// What is missing is refused here, before any call.
func TestReplacingHoldsNeedsAMomentAndASource(t *testing.T) {
	var called bool
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	for _, args := range [][]string{
		{"admin", "tenant", "replace-holds", replaceTenantID, "--from", "holds.json"},
		{"admin", "tenant", "replace-holds", replaceTenantID, "--recovery-point", "2026-08-16T00:00:00Z"},
		{"admin", "tenant", "replace-holds", replaceTenantID, "--recovery-point", "Sunday", "--from", "holds.json"},
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d: %s", args, code, exitUsage, errOut)
		}
	}
	if called {
		t.Error("a refused command reached the server")
	}
}
