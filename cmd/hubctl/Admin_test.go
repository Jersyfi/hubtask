// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	acmeID    = "01936f2a-7c1e-7000-8000-0000000005a1"
	othersID  = "01936f2a-7c1e-7000-8000-0000000005a2"
	ownerID   = "01936f2a-7c1e-7000-8000-0000000005a3"
	seededHub = "01936f2a-7c1e-7000-8000-0000000005a4"
)

var redemptionToken = "hbt_" + "inv_theowners"

const twoTenants = `[
  {"id":"` + acmeID + `","slug":"acme","display_name":"Acme","status":"ACTIVE",
   "default_locale":"de","default_time_zone":"Europe/Berlin",
   "created_at":"2026-09-01T09:00:00Z","purge_after":null},
  {"id":"` + othersID + `","slug":"others","display_name":"Others","status":"PENDING_DELETION",
   "created_at":"2026-09-02T09:00:00Z","purge_after":"2026-10-02T09:00:00Z"}]`

func TestListingWorkspacesShowsTheirStatusAndTheGraceWhereOneRuns(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, twoTenants)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+adminTenantsPath {
		t.Errorf("the listing called %s", stub.request.URL.Path)
	}
	if !strings.Contains(out, "acme") || !strings.Contains(out, "PENDING_DELETION") {
		t.Errorf("the listing is not the table: %q", out)
	}
	// A workspace nobody has asked to end has no grace, and a dash says so rather than a date
	// that would read as one.
	if !strings.Contains(out, "-") {
		t.Errorf("a workspace with no grace should say so: %q", out)
	}
}

// The owner's way in is answered once. Same discipline as every other credential this client
// meets: readable by a script on standard output, with the warning beside it.
func TestProvisioningPrintsTheRedemptionTokenOnceAndWarnsBesideIt(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"id":"`+acmeID+`","slug":"acme",
	  "display_name":"Acme","status":"ACTIVE","created_at":"2026-09-01T09:00:00Z",
	  "owner_account_id":"`+ownerID+`","owner_redemption_token":"`+redemptionToken+`",
	  "default_hub_id":"`+seededHub+`","example_collection_id":"`+collectionID+`"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "create", "--slug", "acme", "--name", "Acme",
		"--owner-email", "eva@acme.example", "--locale", "de")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"slug":"acme"`) ||
		!strings.Contains(stub.body, `"owner_email":"eva@acme.example"`) {
		t.Errorf("the provisioning did not travel: %s", stub.body)
	}
	if !strings.Contains(out, redemptionToken) {
		t.Errorf("the owner's way in is not where a script can read it: %q", out)
	}
	if !strings.Contains(errOut, "shown once") {
		t.Errorf("nothing warns about the single showing: %q", errOut)
	}
	if strings.Contains(out, "shown once") {
		t.Error("the warning landed on standard output")
	}
	// The seeded structure is part of the answer: the workspace arrives with a hub and a
	// collection in it, and their identifiers are what the owner's first commands name.
	if !strings.Contains(out, seededHub) || !strings.Contains(out, collectionID) {
		t.Errorf("the seeded structure is not in the answer: %q", out)
	}
}

func TestSuspendingAndResumingAreOneWriteEach(t *testing.T) {
	for _, verb := range []string{"suspend", "resume"} {
		t.Run(verb, func(t *testing.T) {
			stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
				"admin", "tenant", verb, acmeID)
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}
			want := APIPath + adminTenantsPath + "/" + acmeID + ":" + verb
			if stub.request.Method != http.MethodPost || stub.request.URL.Path != want {
				t.Errorf("%s %s, want POST %s", stub.request.Method, stub.request.URL.Path, want)
			}
		})
	}
}

// The most irreversible act this API has: the name typed exactly, and a fresh proof on top of it.
// The proof travels in the header here, which is the one difference from the restore.
func TestEndingAWorkspaceProvesAgainAndCarriesTheProofInTheHeader(t *testing.T) {
	var proofHeader string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == APIPath+stepUpPath:
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"step_up_token":"` + stepUpToken + `",
			  "expires_at":"2026-09-05T09:05:00Z","method":"PASSWORD"}`))
		case r.Header.Get(stepUpHeader) == "":
			problemJSON(w, http.StatusForbidden, map[string]any{
				"status": 403, "code": "forbidden", "detail_code": "auth.step_up_required",
				"params": map[string]any{"methods": "PASSWORD TOTP"},
			})
		default:
			proofHeader = r.Header.Get(stepUpHeader)
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"tenant_id":"` + acmeID + `","purge_after":"2026-10-05T09:00:00Z"}`))
		}
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"admin", "tenant", "delete", acmeID, "--confirm", "Acme")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if proofHeader != stepUpToken {
		t.Errorf("the proof did not travel in %s: %q", stepUpHeader, proofHeader)
	}
	if !strings.Contains(stub.body, `"confirmation":"Acme"`) {
		t.Errorf("the typed name did not travel: %s", stub.body)
	}
	// The grace is the answer: the data is still there and the export still works until then.
	if !strings.Contains(out, "2026-10-05") {
		t.Errorf("the moment the grace runs out is not in the answer: %q", out)
	}
}

// Typing the name is the point of the confirmation; a client that offered to fill it in would be
// undoing the safeguard it is implementing.
func TestEndingAWorkspaceNeedsTheNameTyped(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made for a deletion nobody confirmed")
	})
	code, _, _ := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "delete", acmeID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}

// The archive is at the target, so the job carries no result to read back: what this command can
// say is that the export finished.
func TestExportingAWorkspaceFollowsTheJobItBecame(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasPrefix(r.URL.Path, APIPath+jobsPath) {
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":null,"error_code":null,
			  "created_at":"2026-09-05T09:00:00Z","finished_at":"2026-09-05T09:02:00Z"}`))
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED","result_url":null}`))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "export", acmeID, "--target", targetID, "--follow", "--wait", "10s")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "SUCCEEDED") {
		t.Errorf("the followed job is not in the answer: %q", out)
	}
}

// The archive goes to a configured target and nowhere else - E-09's one discipline for "bytes
// leave the installation" - so a request without one is refused before the round trip.
func TestExportingNeedsATarget(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("an export was requested with nowhere to write it")
	})
	code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "export", acmeID)
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
}

func TestTheQuotaStandingReadsUnlimitedRatherThanNought(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[
	  {"quota":"items","limit":1000,"used":250,"ratio":0.25,"configured":true},
	  {"quota":"api_requests_per_minute","limit":0,"used":null,"ratio":null,"configured":false}]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "quota", "show")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+quotasPath {
		t.Errorf("the standing called %s", stub.request.URL.Path)
	}
	// Nought means unlimited, and a column of numbers with a nought in it would read as the
	// opposite of what the contract says.
	if !strings.Contains(out, "unlimited") {
		t.Errorf("nought was printed as a limit: %q", out)
	}
	if !strings.Contains(out, "25%") {
		t.Errorf("the approach is not in the table: %q", out)
	}
}
