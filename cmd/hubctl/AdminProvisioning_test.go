// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A purchase platform's credential at the terminal (UC-INS-06 check 6, UC-INS-08 check 7): the
// operator mints the narrow scope for the platform's service account behind their own proof, and
// the platform's token then ends a workspace with the typed name alone - no prompt, no proof.

func TestTheProvisioningScopeIsMintedForAServiceAccountBehindAStepUp(t *testing.T) {
	stub, _, proofs := demandingFor(t, http.MethodPost, tokensPath, http.StatusCreated, mintedTokenJSON())
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"token", "create", "--name", "the shop", "--scope", "admin:provisioning",
		"--days", "30", "--account", machineID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[1] != stepUpToken {
		t.Fatalf("the mint was not retried with the proof: %v", *proofs)
	}
	if !strings.Contains(stub.body, `"admin:provisioning"`) || !strings.Contains(stub.body, machineID) {
		t.Errorf("the scope or the service account did not travel: %s", stub.body)
	}
}

func TestAPlatformsTokenEndsAWorkspaceWithTheTypedNameAlone(t *testing.T) {
	var proofHeader string
	calls := 0
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.URL.Path == APIPath+stepUpPath {
			t.Error("a proof was asked for although the server demanded none")
		}
		proofHeader = r.Header.Get(stepUpHeader)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"tenant_id":"` + acmeID + `","purge_after":"2026-11-16T09:00:00Z"}`))
	})

	// No stdin: a prompt would have nothing to read, and the command must not need one.
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "tenant", "delete", acmeID, "--confirmation", "Acme")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if calls != 1 || proofHeader != "" {
		t.Errorf("%d calls, proof %q; want the one request without a proof", calls, proofHeader)
	}
	if !strings.Contains(stub.body, `"confirmation":"Acme"`) || !strings.Contains(out, "2026-11-16") {
		t.Errorf("the typed name or the grace is missing: body %s, out %q", stub.body, out)
	}
}
