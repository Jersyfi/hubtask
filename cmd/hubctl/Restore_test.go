// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const restoreID = "01936f2a-7c1e-7000-8000-0000000000c9"

// restoring answers the three calls every restore command makes: the request, the job, and the run
// that carries the report. The request's body is kept here rather than read off the stub, because
// the two calls that follow it overwrite what the stub last saw.
func restoring(t *testing.T, run string) (*installation, *string) {
	t.Helper()
	var asked string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == APIPath+restoresPath:
			// `serve` has already drained the body into the stub; this keeps the request's own
			// copy, because the two calls that follow overwrite it.
			asked = stub.body
			w.WriteHeader(http.StatusAccepted)
			// The job and the restore are two identifiers, and the 202 is the only answer that
			// carries both.
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED",
			  "result_url":"` + restoresPath + `/` + restoreID + `"}`))
		case strings.HasPrefix(r.URL.Path, APIPath+jobsPath):
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":null,"error_code":null,
			  "created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:03:00Z"}`))
		default:
			_, _ = w.Write([]byte(run))
		}
	})
	return stub, &asked
}

const inspectedRun = `{"id":"` + restoreID + `","target_id":"` + targetID + `",
  "source_archive":"daily/2026-08-27.hubtask","mode":"INSPECT","dry_run":true,
  "status":"SUCCEEDED","started_at":"2026-08-27T09:00:00Z",
  "report":{"new":42,"overwritten":0,"skipped":0,"duplicated":0,"conflicts":0,
            "deleted":1,"media":3,"entities":{"work_item":42,"container":4}}}`

func TestInspectingAnArchiveAsksForTheInspectModeAndPrintsTheReport(t *testing.T) {
	stub, asked := restoring(t, inspectedRun)

	env := signedIn(stub)
	env[envBackupPassphrase] = "a long passphrase"
	code, out, errOut := invokeAgainst(t, stub, env, "",
		"restore", "inspect", "--target", targetID, "--archive", "daily/2026-08-27.hubtask")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(*asked), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["mode"] != "INSPECT" {
		t.Errorf("mode %v", sent["mode"])
	}
	if sent["archive_id"] != "daily/2026-08-27.hubtask" {
		t.Errorf("the archive did not travel: %v", sent["archive_id"])
	}
	if sent["decryption_passphrase"] != "a long passphrase" {
		t.Error("the passphrase was not read from the environment")
	}
	// The report is the whole point: the counts and what each entity contributed.
	if !strings.Contains(out, "42") || !strings.Contains(out, "work_item") {
		t.Errorf("the report is not in the answer: %q", out)
	}
}

// A restore is a dry run unless somebody says otherwise. The flag is `--apply` rather than
// `--dry-run=false`, so that the dangerous half is the one that has to be typed.
func TestARestoreIsADryRunUntilApplyIsGiven(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		dryRun any
	}{
		{"without --apply", nil, nil},
		{"with --apply", []string{"--apply"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stub, asked := restoring(t, inspectedRun)
			args := append([]string{
				"restore", "run", "--target", targetID,
				"--archive", "daily/2026-08-27.hubtask", "--mode", "NEW_TENANT",
			}, tc.args...)

			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}

			var sent map[string]any
			if err := json.Unmarshal([]byte(*asked), &sent); err != nil {
				t.Fatalf("the body is not JSON: %v", err)
			}
			if sent["dry_run"] != tc.dryRun {
				t.Errorf("dry_run %v, want %v", sent["dry_run"], tc.dryRun)
			}
			if sent["mode"] != "NEW_TENANT" {
				t.Errorf("mode %v", sent["mode"])
			}
		})
	}
}

func TestARestoreWithoutAnArchiveSaysWhereToFindOne(t *testing.T) {
	stub, _ := restoring(t, inspectedRun)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"restore", "run", "--target", targetID, "--mode", "MERGE")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "hubctl backup ls") {
		t.Errorf("the complaint does not say where an archive comes from: %q", errOut)
	}
}

// The confirmation a destructive mode needs is passed on rather than invented, and so is the
// decision to skip the safety copy.
func TestADestructiveRestorePassesOnWhatTheServerWillAskFor(t *testing.T) {
	stub, asked := restoring(t, inspectedRun)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"restore", "run", "--target", targetID, "--archive", "a.hubtask",
		"--mode", "REPLACE_TENANT", "--confirm", "Acme", "--no-safety-backup", "--apply",
		"--conflict", "OVERWRITE", "--tenant", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(*asked), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["confirmation"] != "Acme" || sent["create_safety_backup"] != false ||
		sent["conflict_rule"] != "OVERWRITE" || sent["target_tenant_id"] != collectionID {
		t.Errorf("the request lost something: %v", sent)
	}
}

// Under --json it is one document, so that `hubctl --json restore inspect … | jq .report` works.
func TestARestoreUnderJSONIsOneDocument(t *testing.T) {
	stub, _ := restoring(t, inspectedRun)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "restore", "inspect", "--target", targetID, "--archive", "a.hubtask")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var document map[string]any
	if err := json.Unmarshal([]byte(out), &document); err != nil {
		t.Fatalf("standard output is not one document: %v\n%s", err, out)
	}
	if document["source_archive"] != "daily/2026-08-27.hubtask" {
		t.Errorf("document %v", document)
	}
}

func TestShowingARestoreReadsItsRun(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, inspectedRun)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "restore", "show", restoreID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + restoresPath + "/" + restoreID; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "SUCCEEDED") {
		t.Errorf("output %q", out)
	}
}
