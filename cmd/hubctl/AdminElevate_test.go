// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// demandingFor answers one privileged route the way the application layer does: refused for want
// of a proof, the proof minted, and the same request with the proof in X-Hubtask-Step-Up accepted.
func demandingFor(t *testing.T, method, path string, status int, answer string) (*installation, *[]string, *[]string) {
	t.Helper()
	var paths, proofs []string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == APIPath+stepUpPath:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"step_up_token":"` + stepUpToken + `",
			  "expires_at":"2026-10-12T09:05:00Z","method":"PASSWORD"}`))
		case r.Method == method && r.URL.Path == APIPath+path:
			proof := r.Header.Get(stepUpHeader)
			proofs = append(proofs, proof)
			if proof != stepUpToken {
				problemJSON(w, http.StatusForbidden, map[string]any{
					"status": 403, "code": "forbidden", "detail_code": "auth.step_up_required",
					"params": map[string]any{"methods": "PASSWORD"},
				})
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			_, _ = w.Write([]byte(answer))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`[]`))
		}
	})
	return stub, &paths, &proofs
}

// UC-ID-17 check 3 and UC-INS-06 check 2 at the terminal: a token with an administrative scope is
// refused for want of a proof, the person proves themselves, and the same mint goes again with the
// proof in the header.
func TestAnAdministrativeTokenIsMintedBehindAStepUp(t *testing.T) {
	stub, paths, proofs := demandingFor(t, http.MethodPost, tokensPath, http.StatusCreated, mintedTokenJSON())
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"token", "create", "--name", "the control plane", "--scope", "admin:tenants",
		"--days", "30", "--account", machineID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[0] != "" || (*proofs)[1] != stepUpToken {
		t.Fatalf("the mint was not retried with the proof: %v (paths %v)", *proofs, *paths)
	}
	if !strings.Contains(stub.body, machineID) {
		t.Errorf("the service account did not travel: %s", stub.body)
	}
	if !strings.Contains(out, mintedToken) {
		t.Errorf("the credential was not printed: %q", out)
	}
}

// A proof minted elsewhere travels on the first attempt, so a token profile can mint one too.
func TestAnAdministrativeTokenTakesAProofFromTheEnvironment(t *testing.T) {
	stub, paths, proofs := demandingFor(t, http.MethodPost, tokensPath, http.StatusCreated, mintedTokenJSON())

	env := signedIn(stub)
	env[envStepUp] = stepUpToken
	code, _, errOut := invokeAgainst(t, stub, env, "",
		"token", "create", "--name", "the control plane", "--scope", "admin:tenants", "--days", "30")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 1 || (*proofs)[0] != stepUpToken {
		t.Errorf("the carried proof did not travel at once: %v (paths %v)", *proofs, *paths)
	}
}

// UC-INS-04 check 6: a person at a terminal raises their own session the same way the web app does.
func TestElevatingRaisesThisSessionBehindAStepUp(t *testing.T) {
	until := time.Now().Add(time.Hour).UTC().Truncate(time.Second)
	stub, _, proofs := demandingFor(t, http.MethodPost, elevatePath, http.StatusOK,
		`{"elevated_until":"`+until.Format(time.RFC3339)+`","remaining_seconds":3600}`)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"admin", "elevate")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[1] != stepUpToken {
		t.Fatalf("the raise was not retried with the proof: %v", *proofs)
	}
	if !strings.Contains(out, until.Local().Format("2006-01-02 15:04")) || !strings.Contains(out, "60") {
		t.Errorf("the end and the minutes left are not printed: %q", out)
	}
}

// A running hour is not raised again: its end is the answer, and nothing is asked for.
func TestElevatingARaisedSessionPrintsTheRunningHour(t *testing.T) {
	until := time.Now().Add(25 * time.Minute).UTC().Truncate(time.Second)
	var asked int
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == APIPath+stepUpPath {
			asked++
		}
		problemJSON(w, http.StatusConflict, map[string]any{
			"status": 409, "code": "conflict", "detail_code": "auth.session_already_elevated",
			"params": map[string]any{"until": until.Format(time.RFC3339)},
		})
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, out, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "",
		"admin", "elevate")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if asked != 0 {
		t.Error("a proof was asked for a running hour")
	}
	if !strings.Contains(out, until.Local().Format("2006-01-02 15:04")) {
		t.Errorf("the running end is not printed: %q", out)
	}
}

// A personal access token cannot prove a person afresh, so there is nothing to raise.
func TestElevatingUnderAPersonalAccessTokenIsRefusedBeforeTheCall(t *testing.T) {
	var called bool
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "elevate")
	if code == exitOK || called {
		t.Fatalf("exit %d, called %v", code, called)
	}
	if !strings.Contains(errOut, "hubtask login") && !strings.Contains(errOut, "hubctl login") {
		t.Errorf("the refusal does not name the way: %q", errOut)
	}
}

// The register is listed by name, address and workspace, with the identifier `rm` takes last.
func TestTheOperatorListNamesItsPeople(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[
	  {"tenant_id":"`+itemID+`","account_id":"`+machineID+`","added_at":"2026-10-01T08:00:00Z",
	   "display_name":"Ada Lovelace","email":"ada@acme.example","kind":"USER",
	   "workspace_slug":"acme","workspace_name":"Acme"},
	  {"tenant_id":"`+itemID+`","account_id":"`+tokenID+`","added_at":"2026-10-02T08:00:00Z",
	   "display_name":"The shop","kind":"SERVICE_ACCOUNT","workspace_slug":"acme","workspace_name":"Acme"}]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "operator", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"Ada Lovelace", "ada@acme.example", "acme", "The shop", "service account", machineID} {
		if !strings.Contains(out, want) {
			t.Errorf("the list lacks %q: %q", want, out)
		}
	}
}

// UC-INS-06 check 1 at the terminal: a service account is chosen by its name from the caller's own
// workspace, and sent by its identifier; a name that matches none, or several, is refused here.
func TestAServiceAccountIsMadeAnOperatorByName(t *testing.T) {
	accounts := `[{"id":"` + machineID + `","kind":"SERVICE_ACCOUNT","display_name":"The shop","status":"ACTIVE"},
	  {"id":"` + tokenID + `","kind":"SERVICE_ACCOUNT","display_name":"Export","status":"ACTIVE"},
	  {"id":"` + itemID + `","kind":"SERVICE_ACCOUNT","display_name":"Export","status":"ACTIVE"}]`
	var added []string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == APIPath+serviceAccountPath:
			_, _ = w.Write([]byte(accounts))
		case r.Method == http.MethodPost && r.URL.Path == APIPath+adminOperatorsPath:
			var body map[string]any
			if err := json.Unmarshal([]byte(stub.body), &body); err != nil {
				t.Errorf("the body is not JSON: %v", err)
			}
			id, _ := body["account_id"].(string)
			added = append(added, id)
			w.WriteHeader(http.StatusNoContent)
		default:
			_, _ = w.Write([]byte(`[]`))
		}
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"admin", "operator", "add", "--service-account", "the shop")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(added) != 1 || added[0] != machineID {
		t.Fatalf("added %v, want the shop's identifier", added)
	}

	for _, name := range []string{"nobody", "Export"} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
			"admin", "operator", "add", "--service-account", name)
		if code == exitOK || len(added) != 1 {
			t.Errorf("%q was accepted (exit %d): %s", name, code, errOut)
		}
	}
}
