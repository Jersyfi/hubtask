// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

const (
	erasureCaseID = "01936f2a-7c1e-7000-8000-0000000000e1"
	unnamedCaseID = "01936f2a-7c1e-7000-8000-0000000000e2"
)

func withProof(stub *installation) map[string]string {
	env := signedIn(stub)
	env[envStepUp] = stepUpToken
	return env
}

// The cases read from the temporary cluster go to the workspace's operation with the recovery
// point and the step-up proof; a case whose subject could not be named is reported, not sent.
func TestReapplyingErasuresSendsTheClustersCasesBehindAStepUp(t *testing.T) {
	completed := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	var askedDSN string
	var askedTenant shared.ID
	var askedPoint time.Time
	recoveredErasures = func(
		_ context.Context, dsn string, tenantID shared.ID, point time.Time, timeout time.Duration,
	) ([]postgres.RecoveredErasure, error) {
		askedDSN, askedTenant, askedPoint = dsn, tenantID, point
		if timeout <= 0 {
			t.Error("the read has no bound")
		}
		return []postgres.RecoveredErasure{
			{Case: domain.Request{
				ID: erasureCaseID, ErasureMode: domain.ModeFullDelete, SubjectAccountID: itemID,
				ReceivedAt: completed.Add(-time.Hour), DueAt: completed.Add(720 * time.Hour), CompletedAt: completed,
			}},
			{Case: domain.Request{ID: unnamedCaseID, ErasureMode: domain.ModeFullDelete, CompletedAt: completed},
				Unresolved: postgres.SubjectAmbiguous},
		}, nil
	}
	t.Cleanup(func() { recoveredErasures = postgres.ReadRecoveredErasures })
	stub := serveJSON(t, http.StatusOK,
		`{"erasures":[{"id":"`+erasureCaseID+`","outcome":"REAPPLIED","kept":false}]}`)

	const dsn = "postgres://hubtask_app@recovered.hubtask.svc:5432/hubtask"
	code, out, errOut := invokeAgainst(t, stub, withProof(stub), "",
		"admin", "tenant", "reapply-erasures", replaceTenantID,
		"--recovery-point", "2026-08-16T00:00:00Z", "--from-cluster", dsn)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if askedDSN != dsn || askedTenant != replaceTenantID || !askedPoint.Equal(time.Date(2026, 8, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("read %q for %q from %s", askedDSN, askedTenant, askedPoint)
	}
	if want := "/admin/tenants/" + replaceTenantID + ":reapply-erasures"; !strings.HasSuffix(stub.request.URL.Path, want) {
		t.Errorf("posted to %s", stub.request.URL.Path)
	}
	if proof := stub.request.Header.Get(stepUpHeader); proof != stepUpToken {
		t.Errorf("the proof did not travel: %q", proof)
	}
	var sent struct {
		RecoveryPoint string           `json:"recovery_point"`
		Erasures      []map[string]any `json:"erasures"`
	}
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if len(sent.Erasures) != 1 || sent.Erasures[0]["id"] != erasureCaseID ||
		sent.Erasures[0]["subject_account_id"] != string(itemID) || sent.Erasures[0]["erasure_mode"] != "FULL_DELETE" {
		t.Fatalf("sent %v", sent.Erasures)
	}
	if !strings.Contains(out, "REAPPLIED") || !strings.Contains(out, postgres.SubjectAmbiguous) {
		t.Errorf("output %q", out)
	}
	if !strings.Contains(errOut, unnamedCaseID) {
		t.Errorf("the case set aside was not named: %q", errOut)
	}
}

// A file in the operation's own shape is the other source.
func TestReapplyingErasuresReadsAFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "erasures.json")
	content := `{"erasures":[{"id":"` + erasureCaseID + `","erasure_mode":"ANONYMIZE",` +
		`"subject_account_id":"` + string(itemID) + `","received_at":"2026-08-17T08:00:00Z",` +
		`"due_at":"2026-09-16T08:00:00Z","completed_at":"2026-08-18T09:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := serveJSON(t, http.StatusOK,
		`{"erasures":[{"id":"`+erasureCaseID+`","outcome":"ALREADY_COMPLETED","kept":false}]}`)

	code, out, errOut := invokeAgainst(t, stub, withProof(stub), "",
		"admin", "tenant", "reapply-erasures", replaceTenantID,
		"--recovery-point", "2026-08-16T00:00:00Z", "--from", path)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"erasure_mode":"ANONYMIZE"`) || !strings.Contains(out, "ALREADY_COMPLETED") {
		t.Errorf("sent %s, printed %q", stub.body, out)
	}
}

// Without a recovery point, without a source, or with two, nothing is read and nothing is sent.
func TestReapplyingErasuresRefusesAnIncompleteCommand(t *testing.T) {
	recoveredErasures = func(context.Context, string, shared.ID, time.Time, time.Duration) ([]postgres.RecoveredErasure, error) {
		t.Error("the cluster was read")
		return nil, nil
	}
	t.Cleanup(func() { recoveredErasures = postgres.ReadRecoveredErasures })
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		t.Error("a call was made")
		w.WriteHeader(http.StatusOK)
	})

	for name, args := range map[string][]string{
		"no recovery point": {"--from", "erasures.json"},
		"no source":         {"--recovery-point", "2026-08-16T00:00:00Z"},
		"two sources": {"--recovery-point", "2026-08-16T00:00:00Z",
			"--from-cluster", "postgres://recovered/hubtask", "--from", "erasures.json"},
		"a moment that is not one": {"--recovery-point", "yesterday", "--from", "erasures.json"},
	} {
		t.Run(name, func(t *testing.T) {
			code, _, _ := invokeAgainst(t, stub, withProof(stub), "",
				append([]string{"admin", "tenant", "reapply-erasures", replaceTenantID}, args...)...)
			if code != exitUsage {
				t.Errorf("exit %d, want %d", code, exitUsage)
			}
		})
	}
}
