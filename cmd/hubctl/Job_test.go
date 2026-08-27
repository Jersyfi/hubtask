// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
)

const jobID = "01936f2a-7c1e-7000-8000-0000000000a1"

func TestShowingAJobAsksForItAndPrintsWhereTheResultWillBe(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{
	  "job_id":"`+jobID+`","status":"RUNNING","progress":0.25,
	  "result_url":"/v1/backups/`+jobID+`","error_code":null,
	  "created_at":"2026-08-27T09:00:00Z","finished_at":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "job", "show", jobID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + jobsPath + "/" + jobID; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "25%") {
		t.Errorf("the progress is not in the table: %q", out)
	}
	if !strings.Contains(out, "/v1/backups/") {
		t.Errorf("the result is not in the table: %q", out)
	}
}

// A job that cannot say how far along it is says so with a dash. A nought would read as "nothing
// has happened", which is a different statement.
func TestAJobWithoutProgressPrintsADashRatherThanZero(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{
	  "job_id":"`+jobID+`","status":"QUEUED","progress":null,"result_url":null,
	  "error_code":null,"created_at":"2026-08-27T09:00:00Z","finished_at":null}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "job", "show", jobID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if strings.Contains(out, "0%") {
		t.Errorf("a job that cannot say reported a number: %q", out)
	}
}

// --follow keeps asking, and what it reports while waiting goes to standard error - so that a
// script reading the answer out of a pipe reads one document and no percentages.
func TestFollowingAJobWaitsForTheEndAndKeepsProgressOffStandardOutput(t *testing.T) {
	var calls atomic.Int32
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		body := `{"job_id":"` + jobID + `","status":"RUNNING","progress":0.5,"result_url":null,
		  "error_code":null,"created_at":"2026-08-27T09:00:00Z","finished_at":null}`
		if calls.Add(1) > 1 {
			body = `{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":"/v1/backups/` + jobID + `","error_code":null,
			  "created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:01:00Z"}`
		}
		_, _ = w.Write([]byte(body))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"--json", "job", "show", jobID, "--follow")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if calls.Load() < 2 {
		t.Errorf("it stopped after %d call(s) rather than following", calls.Load())
	}
	if !strings.Contains(errOut, "RUNNING 50%") {
		t.Errorf("the progress was not reported while waiting: %q", errOut)
	}
	if strings.Contains(out, "RUNNING") {
		t.Errorf("standard output carries more than the answer: %q", out)
	}
	if !strings.Contains(out, "SUCCEEDED") {
		t.Errorf("the answer is not the finished job: %q", out)
	}
}

func TestCancellingAJobPostsToTheCancelPath(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{
	  "job_id":"`+jobID+`","status":"CANCELLED","progress":null,"result_url":null,
	  "error_code":null,"created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:02:00Z"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "job", "cancel", jobID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + jobsPath + "/" + jobID + ":cancel"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "CANCELLED") {
		t.Errorf("output %q", out)
	}
}

// An identifier that is not one is a mistake in the invocation: exit 2, and the catalogue's
// sentence rather than a second wording of it.
func TestAJobIdentifierThatIsNotOneIsAUsageError(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "job", "show", "not-a-uuid")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "not-a-uuid") {
		t.Errorf("the complaint does not name the value: %q", errOut)
	}
}

// `--timeout` is global and has to come before the command; `--wait` is the command's own, and it
// is the one a person reaches for after typing `--follow`. A command that watches has to accept it
// where it is typed, or the flag package would refuse the invocation the README shows.
func TestAFollowingCommandTakesItsWaitAfterTheVerb(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{
	  "job_id":"`+jobID+`","status":"SUCCEEDED","progress":1,"result_url":null,
	  "error_code":null,"created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:01:00Z"}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"job", "show", jobID, "--follow", "--wait", "2m")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
}

// A job that is being retried carries the code of its last failure and goes back to QUEUED between
// attempts. A watching command that printed the status alone would repeat "QUEUED" for the whole
// wait while the work failed six times - which is what it did against a stack with no keyring.
func TestAWatchedJobThatKeepsFailingSaysWhy(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{
	  "job_id":"`+jobID+`","status":"QUEUED","progress":null,"result_url":null,
	  "error_code":"crypto.no_encryption_key","created_at":"2026-08-27T09:00:00Z",
	  "finished_at":null}`)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"job", "show", jobID, "--follow", "--wait", "3s")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	if !strings.Contains(errOut, "retrying after crypto.no_encryption_key") {
		t.Errorf("the wait says nothing about the failures: %q", errOut)
	}
	if !strings.Contains(errOut, "last failure crypto.no_encryption_key") {
		t.Errorf("giving up says nothing about why: %q", errOut)
	}
}
