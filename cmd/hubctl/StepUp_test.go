// SPDX-License-Identifier: BUSL-1.1
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

var stepUpToken = "hbt_" + "sup_proof"

// destructiveRun is what the drill's closing scene answers.
const destructiveRun = `{"id":"` + restoreID + `","target_id":"` + targetID + `",
  "source_archive":"daily/2026-08-27.hubtask","mode":"REPLACE_TENANT","dry_run":false,
  "status":"SUCCEEDED","started_at":"2026-08-27T09:00:00Z",
  "report":{"new":0,"overwritten":42,"skipped":0,"duplicated":0,"conflicts":0,
            "deleted":0,"media":3,"entities":{"work_item":42}}}`

// demanding answers the destructive restore the way the application layer does: the first request
// is refused for want of a proof, the proof is minted, and the second request - the same request
// with the token in it - is accepted.
func demanding(t *testing.T) (*installation, *[]string, *string) {
	t.Helper()
	var paths []string
	var proved string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		switch {
		case r.URL.Path == APIPath+stepUpPath:
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"step_up_token":"` + stepUpToken + `",
			  "expires_at":"2026-08-27T09:05:00Z","method":"PASSWORD"}`))
		case r.Method == http.MethodPost && r.URL.Path == APIPath+restoresPath:
			if !strings.Contains(stub.body, stepUpToken) {
				problemJSON(w, http.StatusForbidden, map[string]any{
					"status": 403, "code": "forbidden", "detail_code": "auth.step_up_required",
					"params": map[string]any{"methods": "PASSWORD TOTP"},
				})
				return
			}
			proved = stub.body
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED",
			  "result_url":"` + restoresPath + `/` + restoreID + `"}`))
		case strings.HasPrefix(r.URL.Path, APIPath+jobsPath):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED","progress":1,
			  "result_url":null,"error_code":null,
			  "created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:03:00Z"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(destructiveRun))
		}
	})
	return stub, &paths, &proved
}

// The round trip that has been refused since 0.4.5, from a terminal: the destructive mode is
// refused for want of a proof, the person proves themselves, and the same call goes again with the
// token in the field the contract has carried all along.
func TestADestructiveRestoreIsProvedAgainAndRetried(t *testing.T) {
	stub, paths, proved := demanding(t)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"restore", "run", "--target", targetID, "--archive", "daily/2026-08-27.hubtask",
		"--mode", "REPLACE_TENANT", "--apply", "--confirm", "Acme")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	if len(*paths) < 3 || (*paths)[1] != APIPath+stepUpPath {
		t.Fatalf("the proof was not asked for between the two attempts: %v", *paths)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(*proved), &sent); err != nil {
		t.Fatalf("the retried body is not JSON: %v", err)
	}
	// In the field, not the header: that is the one thing this act does differently.
	if sent["step_up_token"] != stepUpToken {
		t.Errorf("the proof did not travel in the request: %v", sent["step_up_token"])
	}
	if sent["confirmation"] != "Acme" || sent["dry_run"] != false {
		t.Errorf("the retry is not the same request: %v", sent)
	}
	if !strings.Contains(errOut, "retrying") {
		t.Errorf("the pause nobody asked for was not reported: %q", errOut)
	}
}

// A step-up is a session's act. Said before the round trip, because the answer is about this
// machine's configuration: a person who reads it has to sign in rather than retype anything.
func TestAStepUpUnderAPersonalAccessTokenIsRefusedBeforeTheCall(t *testing.T) {
	stub, paths, _ := demanding(t)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "hunter2\n",
		"restore", "run", "--target", targetID, "--archive", "daily/2026-08-27.hubtask",
		"--mode", "REPLACE_TENANT", "--apply", "--confirm", "Acme")
	if code != exitError {
		t.Fatalf("exit %d, want %d: %s", code, exitError, errOut)
	}
	for _, path := range *paths {
		if path == APIPath+stepUpPath {
			t.Fatal("a personal access token was sent to prove a person afresh")
		}
	}
	// The catalogue's sentence, not a second wording of it.
	if !strings.Contains(errOut, "Sign in with your password first") {
		t.Errorf("the refusal is not the catalogue's: %q", errOut)
	}
}

// One of the two, never both - and the authenticator's code wins where there is one to hand.
func TestTheProofIsACodeWhereThereIsOneAndOtherwiseThePassword(t *testing.T) {
	for _, tc := range []struct {
		name    string
		env     map[string]string
		stdin   string
		wantKey string
	}{
		{"with an authenticator", map[string]string{envTotp: "123456"}, "", "code"},
		{"without one", nil, "hunter2\n", "password"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var request string
			var stub *installation
			stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.URL.Path == APIPath+stepUpPath:
					request = stub.body
					w.WriteHeader(http.StatusCreated)
					_, _ = w.Write([]byte(`{"step_up_token":"` + stepUpToken + `",
					  "expires_at":"2026-08-27T09:05:00Z","method":"TOTP"}`))
				case r.Method == http.MethodPost && r.URL.Path == APIPath+restoresPath:
					if !strings.Contains(stub.body, stepUpToken) {
						problemJSON(w, http.StatusForbidden, map[string]any{
							"status": 403, "code": "forbidden",
							"detail_code": "auth.step_up_required",
							"params":      map[string]any{"methods": "PASSWORD TOTP"},
						})
						return
					}
					w.WriteHeader(http.StatusAccepted)
					_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"QUEUED",
					  "result_url":"` + restoresPath + `/` + restoreID + `"}`))
				case strings.HasPrefix(r.URL.Path, APIPath+jobsPath):
					_, _ = w.Write([]byte(`{"job_id":"` + jobID + `","status":"SUCCEEDED",
					  "progress":1,"result_url":null,"error_code":null,
					  "created_at":"2026-08-27T09:00:00Z","finished_at":"2026-08-27T09:03:00Z"}`))
				default:
					_, _ = w.Write([]byte(destructiveRun))
				}
			})

			profile := filepath.Join(t.TempDir(), "profile.json")
			saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))
			env := map[string]string{envProfile: profile}
			for name, value := range tc.env {
				env[name] = value
			}

			code, _, errOut := invokeAgainst(t, stub, env, tc.stdin,
				"restore", "run", "--target", targetID, "--archive", "a", "--mode", "REPLACE_TENANT",
				"--apply", "--confirm", "Acme")
			if code != exitOK {
				t.Fatalf("exit %d: %s", code, errOut)
			}

			var sent map[string]any
			if err := json.Unmarshal([]byte(request), &sent); err != nil {
				t.Fatalf("the proof is not JSON: %v", err)
			}
			if _, given := sent[tc.wantKey]; !given {
				t.Errorf("the proof carried %v, want a %s", sent, tc.wantKey)
			}
			if len(sent) != 1 {
				t.Errorf("the proof carried both halves: %v", sent)
			}
		})
	}
}

// Once more, not in a loop: a second refusal means the proof was rejected rather than missing, and
// asking for a password again on the same command would be a client arguing with a server.
func TestAProofTheInstallationRejectsIsNotAskedForTwice(t *testing.T) {
	attempts := 0
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == APIPath+stepUpPath {
			attempts++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"step_up_token":"` + stepUpToken + `",
			  "expires_at":"2026-08-27T09:05:00Z","method":"PASSWORD"}`))
			return
		}
		problemJSON(w, http.StatusForbidden, map[string]any{
			"status": 403, "code": "forbidden", "detail_code": "auth.step_up_required",
			"params": map[string]any{"methods": "PASSWORD TOTP"},
		})
	})
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, _ := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"restore", "run", "--target", targetID, "--archive", "a", "--mode", "REPLACE_TENANT",
		"--apply", "--confirm", "Acme")
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if attempts != 1 {
		t.Errorf("the person was asked to prove themselves %d times", attempts)
	}
}
