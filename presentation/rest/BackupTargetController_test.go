// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The backup targets over REST (E-03). What this layer owes is the shape of the answer and, above
// all, what it does not write: the response schema has no credential in it, and a use case that
// grew one must not be able to leak it through this mapper.

const backupTargetUUID = "0192f000-0000-7000-8000-0000000000c1"

func backupTargetOutput() usecase.Output {
	return usecase.Output{
		"id":              backupTargetUUID,
		"name":            "Off-site bucket",
		"kind":            "S3",
		"scope":           "TENANT",
		"config":          map[string]any{"bucket": "hubtask-backups"},
		"encryption_mode": "AES256_GCM",
		"enabled":         true,
		"warnings":        []string{},
	}
}

func callBackup(
	t *testing.T, registry UseCaseRegistry, method, path, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.UseCases = registry

	ctx := appshared.ContextWithActor(t.Context(), appshared.ActorContext{
		Kind:      appshared.ActorUser,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
	})
	request := httptest.NewRequestWithContext(ctx, method, APIBasePath+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

func TestTheTargetsAreServedAsAnArray(t *testing.T) {
	registry := &catalogue{out: usecase.Output{"data": []usecase.Output{backupTargetOutput()}}}

	recorder := callBackup(t, registry, http.MethodGet, "/backup-targets", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "ListBackupTargets" {
		t.Errorf("use case = %q", registry.name)
	}

	var body []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not an array: %v", err)
	}
	if len(body) != 1 || body[0]["name"] != "Off-site bucket" {
		t.Fatalf("the answer is %v", body)
	}
	// An empty list is an empty array rather than a null: a client renders a list either way, and
	// a null would make it check first.
	empty := callBackup(t, &catalogue{out: usecase.Output{"data": []usecase.Output{}}},
		http.MethodGet, "/backup-targets", "")
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatalf("an empty list reads as %s", empty.Body)
	}
}

// The requirement of the whole task, enforced at the boundary: a credential cannot reach a client
// even if a use case hands one over.
func TestNoCredentialReachesTheResponse(t *testing.T) {
	out := backupTargetOutput()
	out["credentials"] = map[string]any{"secret_key": "the-secret-access-key"}
	out["credential_enc"] = "sealed-bytes"

	recorder := callBackup(t, &catalogue{out: usecase.Output{"data": []usecase.Output{out}}},
		http.MethodGet, "/backup-targets", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if strings.Contains(recorder.Body.String(), "the-secret-access-key") ||
		strings.Contains(recorder.Body.String(), "credential") {
		t.Fatalf("a credential reached the client: %s", recorder.Body)
	}
}

func TestATargetIsCreatedAndAnsweredWithItsWarnings(t *testing.T) {
	out := backupTargetOutput()
	out["encryption_mode"] = "NONE"
	out["warnings"] = []string{"backup.target_unencrypted"}
	registry := &catalogue{out: out}

	recorder := callBackup(t, registry, http.MethodPost, "/backup-targets", `{
		"name": "Off-site bucket",
		"kind": "S3",
		"config": {"bucket": "hubtask-backups"},
		"credentials": {"access_key": "AKIAEXAMPLE", "secret_key": "the-secret"},
		"encryption_mode": "NONE",
		"insecure_acknowledged": true
	}`)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "CreateBackupTarget" {
		t.Errorf("use case = %q", registry.name)
	}
	if registry.in["insecure_acknowledged"] != true {
		t.Errorf("the acknowledgement reached the catalogue as %v", registry.in["insecure_acknowledged"])
	}
	credentials, _ := registry.in["credentials"].(map[string]any)
	if credentials["secret_key"] != "the-secret" {
		t.Errorf("the credentials reached the catalogue as %v", credentials)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	warnings, _ := body["warnings"].([]any)
	if len(warnings) != 1 || warnings[0] != "backup.target_unencrypted" {
		t.Fatalf("the warnings are %v", body["warnings"])
	}
}

// The one field of the contract this installation does not serve yet, and it is refused rather
// than ignored: a passphrase with no effect would leave somebody believing their archives are
// protected by it.
func TestAnEncryptionPassphraseIsRefusedRatherThanIgnored(t *testing.T) {
	registry := &catalogue{out: backupTargetOutput()}

	recorder := callBackup(t, registry, http.MethodPost, "/backup-targets", `{
		"name": "Off-site bucket", "kind": "S3", "config": {"bucket": "b"},
		"encryption_passphrase": "correct horse battery staple"
	}`)

	if recorder.Code != http.StatusUnprocessableEntity && recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.invoked {
		t.Fatal("the catalogue was called with a passphrase it cannot use")
	}
	if !strings.Contains(recorder.Body.String(), "backup.encryption_passphrase_not_available") {
		t.Fatalf("the refusal says %s", recorder.Body)
	}
}

// A target that could not be reached is a 200 with a reason, not a 502: "it does not work and
// here is the code" is what the caller asked to find out.
func TestAnUnreachableTargetIsAnAnswerRatherThanAnError(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"ok": false, "latency_ms": int64(1200), "writable": false,
		"error_code": "backup.target_unreachable",
	}}

	recorder := callBackup(t, registry,
		http.MethodPost, "/backup-targets/"+backupTargetUUID+":test", "")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body)
	}
	if registry.name != "TestBackupTarget" || registry.in["target_id"] != backupTargetUUID {
		t.Errorf("the catalogue was called as %q with %v", registry.name, registry.in)
	}

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	switch {
	case body["ok"] != false:
		t.Errorf("ok = %v", body["ok"])
	case body["error_code"] != "backup.target_unreachable":
		t.Errorf("error code = %v", body["error_code"])
	// A target that cannot say how much room is left says null, which the contract's nullable
	// field is for - and it is present rather than absent, so a client reads it unconditionally.
	case body["free_bytes"] != nil:
		t.Errorf("free bytes = %v", body["free_bytes"])
	}
	if _, present := body["free_bytes"]; !present {
		t.Error("free_bytes is absent rather than null")
	}
}

func TestAProbeThatWorkedCarriesWhatItMeasured(t *testing.T) {
	registry := &catalogue{out: usecase.Output{
		"ok": true, "latency_ms": int64(42), "writable": true, "free_bytes": int64(1 << 30),
	}}

	recorder := callBackup(t, registry,
		http.MethodPost, "/backup-targets/"+backupTargetUUID+":test", "")

	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	switch {
	case body["ok"] != true || body["writable"] != true:
		t.Errorf("the answer is %v", body)
	case body["latency_ms"] != float64(42):
		t.Errorf("latency = %v", body["latency_ms"])
	case body["free_bytes"] != float64(1<<30):
		t.Errorf("free bytes = %v", body["free_bytes"])
	case body["error_code"] != nil:
		t.Errorf("a probe that worked carries the reason %v", body["error_code"])
	}
}

func TestATargetsLastProbeIsAnsweredWhereThereIsOne(t *testing.T) {
	out := backupTargetOutput()
	out["last_test_at"] = time.Date(2026, 8, 26, 8, 0, 0, 0, time.UTC)
	out["last_test_ok"] = false
	out["region_note"] = "Frankfurt"

	recorder := callBackup(t, &catalogue{out: usecase.Output{"data": []usecase.Output{out}}},
		http.MethodGet, "/backup-targets", "")

	var body []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	switch {
	case body[0]["last_test_ok"] != false:
		t.Errorf("last probe = %v", body[0]["last_test_ok"])
	case body[0]["region_note"] != "Frankfurt":
		t.Errorf("region note = %v", body[0]["region_note"])
	}
}
