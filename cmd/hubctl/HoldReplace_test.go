// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
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

// The temporary cluster is read for the workspace named, and every hold it holds goes to the
// operation as it was - its placer, its moment, its release.
func TestReplacingHoldsReadsTheTemporaryCluster(t *testing.T) {
	placed := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	var askedDSN string
	var askedTenant shared.ID
	recoveredHolds = func(_ context.Context, dsn string, tenantID shared.ID, timeout time.Duration) ([]domain.LegalHold, error) {
		askedDSN, askedTenant = dsn, tenantID
		if timeout <= 0 {
			t.Error("the read has no bound")
		}
		return []domain.LegalHold{
			{ID: holdID, Scope: domain.HoldContainer, ScopeID: collectionID, Reason: "the Meier proceedings",
				PlacedBy: itemID, PlacedAt: placed},
			{ID: replaceTenantID, Scope: domain.HoldTenant, Reason: "Inspection",
				PlacedBy: itemID, PlacedAt: placed, ReleasedBy: itemID, ReleasedAt: placed.Add(time.Hour)},
		}, nil
	}
	t.Cleanup(func() { recoveredHolds = postgres.ReadRecoveredHolds })
	stub := serveJSON(t, http.StatusOK, `{"holds":[]}`)

	const dsn = "postgres://hubtask_app@recovered.hubtask.svc:5432/hubtask"
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "replace-holds", replaceTenantID,
		"--recovery-point", "2026-08-16T00:00:00Z", "--from-cluster", dsn)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if askedDSN != dsn || askedTenant != replaceTenantID {
		t.Errorf("read %q for %q", askedDSN, askedTenant)
	}
	var sent struct {
		Holds []map[string]any `json:"holds"`
	}
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if len(sent.Holds) != 2 {
		t.Fatalf("sent %d holds", len(sent.Holds))
	}
	first, second := sent.Holds[0], sent.Holds[1]
	scope, _ := first["scope"].(map[string]any)
	if first["id"] != holdID || scope["kind"] != "CONTAINER" || scope["id"] != collectionID ||
		first["placed_by"] != itemID || first["placed_at"] != "2026-08-20T08:00:00Z" || first["released_at"] != nil {
		t.Errorf("the first hold went as %v", first)
	}
	scope, _ = second["scope"].(map[string]any)
	if _, named := scope["id"]; named || second["released_at"] != "2026-08-20T09:00:00Z" {
		t.Errorf("the released workspace hold went as %v", second)
	}
}

// The archive is named to the server, which reads it, and the answer's gap is said per workspace:
// from when the archive is, and that it lacked the trail.
func TestReplacingHoldsFromAnArchiveSaysWhereTheGapBegins(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"holds":[{"id":"`+holdID+`","outcome":"PLACED",`+
		`"target_present":true,"released_in_period":false}],"archive":{"path":"hubtask-backup-x",`+
		`"snapshot_at":"2026-08-15T02:00:00Z","audit_trail":false,"unreadable_entries":0}}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "replace-holds", replaceTenantID, "--recovery-point", "2026-08-16T00:00:00Z",
		"--from-archive", "hubtask-backup-x", "--target", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	archive, _ := sent["archive"].(map[string]any)
	if _, holds := sent["holds"]; holds || archive["path"] != "hubtask-backup-x" || archive["target_id"] != collectionID {
		t.Errorf("sent %v", sent)
	}
	if !strings.Contains(errOut, "2026-08-15T02:00:00Z") || !strings.Contains(errOut, "audit trail") {
		t.Errorf("the gap went unsaid: %q", errOut)
	}
	if !strings.Contains(out, "PLACED") {
		t.Errorf("output %q", out)
	}
}

// An archive without its target is refused before any call.
func TestReplacingHoldsFromAnArchiveNeedsItsTarget(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	for _, args := range [][]string{
		{"--from-archive", "hubtask-backup-x"},
		{"--from", "holds.json", "--target", collectionID},
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", append([]string{
			"admin", "tenant", "replace-holds", replaceTenantID, "--recovery-point", "2026-08-16T00:00:00Z",
		}, args...)...)
		if code != exitUsage {
			t.Errorf("%v: exit %d, want %d: %s", args, code, exitUsage, errOut)
		}
	}
}

// One source at a time: a cluster and a file together are refused before anything is read.
func TestReplacingHoldsReadsOneSource(t *testing.T) {
	recoveredHolds = func(context.Context, string, shared.ID, time.Duration) ([]domain.LegalHold, error) {
		t.Error("the cluster was read")
		return nil, nil
	}
	t.Cleanup(func() { recoveredHolds = postgres.ReadRecoveredHolds })
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "replace-holds", replaceTenantID, "--recovery-point", "2026-08-16T00:00:00Z",
		"--from-cluster", "postgres://recovered/hubtask", "--from", "holds.json")
	if code != exitUsage {
		t.Errorf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
}
