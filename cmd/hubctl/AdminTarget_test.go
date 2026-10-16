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

const installationTargetJSON = `{"id":"01936f2a-7c1e-7000-8000-000000000e51","name":"exports","kind":"S3",
  "scope":"INSTANCE","encryption_mode":"AES256_GCM","enabled":true,"warnings":[],
  "config":{"bucket":"installation-exports"}}`

// The installation's target is configured with a workspace target's flags at the control plane's
// path, behind a fresh step-up: refused for want of a proof, proven, and sent again with it.
func TestAnInstallationTargetIsAddedBehindAStepUp(t *testing.T) {
	stub, paths, proofs := demandingFor(t, http.MethodPost, adminTargetsPath, http.StatusCreated, installationTargetJSON)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"admin", "target", "add", "--name", "exports", "--kind", "S3",
		"--config", "bucket=installation-exports,region=eu-central-1",
		"--credentials", "access_key=AKIA,secret_key=the-secret")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[0] != "" || (*proofs)[1] != stepUpToken {
		t.Fatalf("the configuration was not retried with the proof: %v (paths %v)", *proofs, *paths)
	}
	if !strings.Contains(stub.body, `"bucket":"installation-exports"`) || !strings.Contains(stub.body, "the-secret") {
		t.Errorf("the body read %s", stub.body)
	}
	if !strings.Contains(out, "exports") || strings.Contains(out, "the-secret") {
		t.Errorf("printed %q", out)
	}
}

// Removing one demands the proof too.
func TestAnInstallationTargetIsRemovedBehindAStepUp(t *testing.T) {
	const id = "01936f2a-7c1e-7000-8000-000000000e51"
	stub, _, proofs := demandingFor(t, http.MethodDelete, adminTargetsPath+"/"+id, http.StatusNoContent, ``)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"admin", "target", "rm", id)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[1] != stepUpToken {
		t.Fatalf("the removal was not retried with the proof: %v", *proofs)
	}
}
