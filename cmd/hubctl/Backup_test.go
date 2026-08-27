// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const (
	targetID = "01936f2a-7c1e-7000-8000-0000000000b1"
	runID    = "01936f2a-7c1e-7000-8000-0000000000b2"
)

const oneTarget = `{"id":"` + targetID + `","name":"nightly","kind":"LOCAL","scope":"TENANT",
  "config":{"path":"daily"},"encryption_mode":"AES256_GCM","enabled":true,
  "last_test_at":null,"last_test_ok":null,"warnings":["backup.target_unencrypted"]}`

const oneRun = `{"id":"` + runID + `","target_id":"` + targetID + `","trigger":"MANUAL",
  "mode":"FULL","status":"SUCCEEDED","archive_path":"daily/2026-08-27.hubtask",
  "size_bytes":5242880,"item_count":42,"media_count":3,
  "started_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:01:00Z",
  "verified_at":null,"verify_ok":null}`

func TestAddingATargetSendsTheConfigurationAsPairsAndTheModeItWasAskedFor(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneTarget)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "target", "add", "--name", "nightly", "--kind", "LOCAL", "--config", "path=daily")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+backupTargetsPath {
		t.Errorf("path %q", stub.request.URL.Path)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	config, _ := sent["config"].(map[string]any)
	if config["path"] != "daily" {
		t.Errorf("the configuration did not travel: %v", sent["config"])
	}
	if sent["encryption_mode"] != "AES256_GCM" {
		t.Errorf("encryption mode %v", sent["encryption_mode"])
	}
	// The contract has an `encryption_passphrase` and this version refuses it: the key is derived
	// from the installation's master key (E-02). Sending one would have every target creation
	// answer 400.
	if _, sentAnyway := sent["encryption_passphrase"]; sentAnyway {
		t.Error("a passphrase was sent to a version that refuses the field")
	}
	if !strings.Contains(out, "nightly") {
		t.Errorf("output %q", out)
	}
}

func TestAConfigurationThatIsNotPairsIsRefusedWhole(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneTarget)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "target", "add", "--name", "n", "--kind", "LOCAL", "--config", "path")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "k=v") {
		t.Errorf("the complaint does not say what a pair is: %q", errOut)
	}
}

// An unencrypted target is the one shape that needs saying out loud, and the acknowledgement
// travels with it rather than being assumed.
func TestAnUnencryptedTargetSendsTheModeAndTheAcknowledgement(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneTarget)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "target", "add", "--name", "n", "--kind", "LOCAL", "--config", "path=daily",
		"--unencrypted", "--insecure-acknowledged")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["encryption_mode"] != "NONE" || sent["insecure_acknowledged"] != true {
		t.Errorf("the request does not say what was asked for: %v", sent)
	}
}

func TestListingTargetsShowsWhetherAnybodyHasEverTestedThem(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[`+oneTarget+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "backup", "target", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "never") {
		t.Errorf("a target nobody has tested does not say so: %q", out)
	}
	if !strings.Contains(out, "backup.target_unencrypted") {
		t.Errorf("the warning is not shown: %q", out)
	}
}

// A probe answers what happened rather than the row, and a probe that failed fails the command:
// `backup target test` is what somebody runs to find out whether a target works.
func TestTestingATargetReportsTheProbeAndFailsWhenItDidNot(t *testing.T) {
	for _, tc := range []struct {
		name    string
		payload string
		code    int
		shows   string
	}{
		{
			"a target that answers",
			`{"ok":true,"writable":true,"latency_ms":12,"free_bytes":1048576,"error_code":null}`,
			exitOK, "1.0 MiB",
		},
		{
			"a target that cannot be written",
			`{"ok":false,"writable":false,"latency_ms":8,"free_bytes":null,
			  "error_code":"backup.target_unreachable"}`,
			exitError, "backup.target_unreachable",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub := serveJSON(t, http.StatusOK, tc.payload)

			code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"backup", "target", "test", targetID)
			if code != tc.code {
				t.Fatalf("exit %d, want %d: %s", code, tc.code, errOut)
			}
			if want := APIPath + backupTargetsPath + "/" + targetID + ":test"; stub.request.URL.Path != want {
				t.Errorf("path %q, want %q", stub.request.URL.Path, want)
			}
			if !strings.Contains(out+errOut, tc.shows) {
				t.Errorf("output %q / %q does not carry %q", out, errOut, tc.shows)
			}
		})
	}
}

// Without --follow a started run answers the job it became: a script can then follow it itself,
// or not, and either way it has the identifier.
func TestStartingABackupAnswersTheJob(t *testing.T) {
	stub := serveJSON(t, http.StatusAccepted,
		`{"job_id":"`+runID+`","status":"QUEUED","result_url":"/v1/backups/`+runID+`"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "run", "--target", targetID, "--no-media")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["target_id"] != targetID || sent["mode"] != "FULL" || sent["include_media"] != false {
		t.Errorf("the request does not say what was asked for: %v", sent)
	}
	if !strings.Contains(out, "QUEUED") {
		t.Errorf("output %q", out)
	}
}

// With --follow it waits, and then reads the run rather than the job: what a person asked for is
// what the archive is, not that a job finished.
// A job identifier is not a run identifier. The 202 carries both, and only its `result_url` says
// which run the job is making - the job resource itself carries none, because nothing writes one.
func TestFollowingABackupEndsWithTheRunTheJobNamed(t *testing.T) {
	var calls atomic.Int32
	var read string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED",
			  "result_url":"` + backupsPath + `/` + runID + `"}`))
		case strings.HasPrefix(r.URL.Path, APIPath+jobsPath):
			calls.Add(1)
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":null,"error_code":null,
			  "created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:01:00Z"}`))
		default:
			read = r.URL.Path
			_, _ = w.Write([]byte(oneRun))
		}
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "run", "--target", targetID, "--follow")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if calls.Load() == 0 {
		t.Error("the job was never asked about")
	}
	if want := APIPath + backupsPath + "/" + runID; read != want {
		t.Errorf("it read %q rather than the run the job named, %q", read, want)
	}
	if !strings.Contains(out, "daily/2026-08-27.hubtask") {
		t.Errorf("the archive is not in the answer: %q", out)
	}
	if !strings.Contains(out, "5.0 MiB") {
		t.Errorf("the size does not read like a size: %q", out)
	}
}

// The listing at the target answers **archives** rather than runs: a path, what wrote it, whether
// it is complete, whether it is encrypted. There is no run row behind it - that is the database's
// reading - and the archive's path is what a restore names.
func TestListingBackupsReadsTheArchivesLyingAtTheTarget(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[{
	  "archive_id":"01936f2a-7c1e-7000-8000-0000000000b2",
	  "path":"daily/hubtask-backup-20260827T004305Z-full","created_at":"2026-08-27T09:00:00Z",
	  "mode":"FULL","item_count":42,"media_count":3,"size_bytes":5242880,
	  "complete":true,"encrypted":true}]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "ls", "--target", targetID, "--refresh")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + backupTargetsPath + "/" + targetID + "/backups"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if stub.request.URL.Query().Get("refresh") != "true" {
		t.Errorf("the cache was not bypassed: %v", stub.request.URL.Query())
	}
	if !strings.Contains(out, "hubtask-backup-20260827T004305Z-full") {
		t.Errorf("the archive a restore would name is not in the listing: %q", out)
	}
	if !strings.Contains(out, "5.0 MiB") {
		t.Errorf("the size does not read like a size: %q", out)
	}
}

// A verification that comes back negative is a failure of the command, not a table with a "no" in
// it: a script that ran `hubctl backup verify` and read exit 0 would file the archive as good.
func TestAnArchiveThatDoesNotVerifyFailsTheCommand(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED"}`))
		case strings.HasPrefix(r.URL.Path, APIPath+jobsPath):
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":null,"error_code":null,"created_at":"2026-08-27T09:00:00Z",
			  "finished_at":"2026-08-27T09:01:00Z"}`))
		default:
			_, _ = w.Write([]byte(`{"id":"` + runID + `","target_id":"` + targetID + `",
			  "trigger":"MANUAL","mode":"FULL","status":"SUCCEEDED",
			  "started_at":"2026-08-27T09:00:00Z","verified_at":"2026-08-27T09:02:00Z",
			  "verify_ok":false}`))
		}
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"backup", "verify", runID, "--follow")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	if !strings.Contains(errOut, "did not verify") {
		t.Errorf("the failure does not say what happened: %q", errOut)
	}
}

func TestBackupTargetWithoutAVerbSaysWhichOnesThereAre(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[]`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "backup", "target")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "add, ls, test") {
		t.Errorf("the complaint does not list the verbs: %q", errOut)
	}
}
