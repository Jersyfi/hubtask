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

// The people group (UC-ID-14 check 7, UC-ID-20 check 9): what each command sends, what it shows
// once, and that the server's refusals reach the person by name.

const (
	invitedID    = "01936f2a-7c1e-7000-8000-000000000d01"
	grantedID    = "01936f2a-7c1e-7000-8000-000000000d02"
	peopleHubID  = "01936f2a-7c1e-7000-8000-000000000d03"
	managedID    = "01936f2a-7c1e-7000-8000-000000000d04"
	invitedJSON  = `{"id":"` + invitedID + `","kind":"USER","email":"ada@example.org","display_name":"Ada","status":"INVITED"}`
	grantedJSON  = `{"id":"` + grantedID + `","account_id":"` + invitedID + `","role":"MEMBER","scope_type":"HUB","scope_id":"` + peopleHubID + `"}`
	invitedLink  = "https://hub.test.invalid/redeem#token=only-once"
	startSecret  = "a-start-password-shown-once"
	refusalLines = "memberships.last_owner"
)

// peopleStub answers the invitation and the grant, keeping what each was sent.
func peopleStub(t *testing.T, grant func(w http.ResponseWriter)) (*installation, map[string]map[string]any) {
	t.Helper()
	sent := map[string]map[string]any{}
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		// serve has read the body already, and keeps it.
		var body map[string]any
		_ = json.Unmarshal([]byte(stub.body), &body)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case APIPath + invitePath:
			sent["invite"] = body
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(strings.TrimSuffix(invitedJSON, "}") + `,"invitation_link":"` + invitedLink + `"}`))
		case APIPath + membershipsPath:
			sent["grant"] = body
			grant(w)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	return stub, sent
}

// The invitation's own body, then the grant on the account it answered - and a link answered where
// no mail is sent goes to standard output once, with the warning on standard error.
func TestAnInvitationGivesTheRoleAndShowsTheLinkOnce(t *testing.T) {
	stub, sent := peopleStub(t, func(w http.ResponseWriter) {
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(grantedJSON))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"people", "invite", "--email", "ada@example.org", "--name", "Ada",
		"--role", "member", "--scope", "hub", "--scope-id", peopleHubID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if sent["invite"]["email"] != "ada@example.org" || sent["invite"]["display_name"] != "Ada" {
		t.Errorf("the invitation sent %v", sent["invite"])
	}
	if g := sent["grant"]; g["account_id"] != invitedID || g["role"] != "MEMBER" ||
		g["scope_type"] != "HUB" || g["scope_id"] != peopleHubID {
		t.Errorf("the grant sent %v", g)
	}
	if strings.Count(out, invitedLink) != 1 || !strings.Contains(errOut, "shown once") {
		t.Errorf("the link: out %q, err %q", out, errOut)
	}
}

// A grant the server refuses after the invitation leaves the account invited, as on the screen,
// and the command says both and fails.
func TestARefusedGrantLeavesTheInvitationAndSaysSo(t *testing.T) {
	stub, _ := peopleStub(t, func(w http.ResponseWriter) {
		problemJSON(w, http.StatusForbidden, map[string]any{
			"status": 403, "code": "forbidden", "detail_code": "access.not_permitted",
		})
	})
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"people", "invite", "--email", "ada@example.org", "--role", "ADMIN")
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(out, invitedID) || !strings.Contains(errOut, "invited") {
		t.Errorf("out %q, err %q: the account and the half that failed are not both said", out, errOut)
	}
}

// Half a placement is named here rather than sent.
func TestAPlacementNeedsItsScopeAndOnlyThat(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, grantedJSON)
	for name, args := range map[string][]string{
		"a hub without its id":     {"people", "grant", invitedID, "--role", "MEMBER", "--scope", "HUB"},
		"the workspace with an id": {"people", "grant", invitedID, "--role", "MEMBER", "--scope-id", peopleHubID},
		"no role":                  {"people", "grant", invitedID},
	} {
		if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", args...); code != exitUsage {
			t.Errorf("%s: exit %d, want %d", name, code, exitUsage)
		}
	}
}

// Granting OWNER is changing the OWNER role: refused for want of a proof, proven, and sent again.
func TestAnOwnerGrantIsSentAgainWithAFreshProof(t *testing.T) {
	stub, paths, proofs := demandingFor(t, http.MethodPost, membershipsPath, http.StatusCreated, grantedJSON)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))

	code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"people", "grant", invitedID, "--role", "OWNER")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if len(*proofs) != 2 || (*proofs)[0] != "" || (*proofs)[1] != stepUpToken {
		t.Fatalf("the grant was not retried with the proof: %v (paths %v)", *proofs, *paths)
	}
}

// Revoking: an OWNER membership behind a proof, and the last owner refused by name.
func TestRevokingTakesTheProofAndNamesTheLastOwner(t *testing.T) {
	stub, _, proofs := demandingFor(t, http.MethodDelete, membershipsPath+"/"+grantedID, http.StatusNoContent, ``)
	profile := filepath.Join(t.TempDir(), "profile.json")
	saveSession(t, profile, stub.server.URL, time.Now().Add(10*time.Minute))
	if code, _, errOut := invokeAgainst(t, stub, map[string]string{envProfile: profile}, "hunter2\n",
		"people", "revoke", grantedID); code != exitOK || len(*proofs) != 2 {
		t.Fatalf("exit %d (%s), proofs %v", code, errOut, *proofs)
	}

	last := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusConflict, map[string]any{
			"status": 409, "code": "conflict", "detail_code": refusalLines,
		})
	})
	code, _, errOut := invokeAgainst(t, last, signedIn(last), "", "people", "revoke", grantedID)
	if code != exitError || !strings.Contains(errOut, "at least one owner") {
		t.Errorf("the last owner: exit %d, err %q", code, errOut)
	}
}

// A managed account: its body, and the start password once on standard output.
func TestAManagedAccountShowsItsStartPasswordOnce(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"account":{"id":"`+managedID+`","kind":"USER","display_name":"Lena","status":"ACTIVE"},`+
		`"membership":{"id":"`+grantedID+`","account_id":"`+managedID+`","role":"MEMBER","scope_type":"TENANT"},`+
		`"start_password":"`+startSecret+`"}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"people", "add-managed", "--name", "Lena", "--sign-in", "lena", "--role", "MEMBER")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	if sent["display_name"] != "Lena" || sent["sign_in_name"] != "lena" || sent["role"] != "MEMBER" || sent["scope_type"] != "TENANT" {
		t.Errorf("the account sent %v", sent)
	}
	if strings.Count(out, startSecret) != 1 || strings.Contains(errOut, startSecret) || !strings.Contains(errOut, "shown once") {
		t.Errorf("the start password: out %q, err %q", out, errOut)
	}
}

// The server's refusals of a managed account reach the person as its sentences.
func TestAManagedAccountsRefusalsAreNamed(t *testing.T) {
	for code, sentence := range map[string]string{
		"accounts.managed_accounts_off":   "does not allow accounts without a mail address",
		"accounts.sign_in_name_taken":     "already signs in with that name",
		"accounts.managed_needs_a_person": "created by a person",
	} {
		stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
			problemJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"status": 422, "code": "validation", "detail_code": code,
			})
		})
		exit, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
			"people", "add-managed", "--name", "Lena", "--sign-in", "lena", "--role", "MEMBER")
		if exit != exitError || !strings.Contains(errOut, sentence) {
			t.Errorf("%s: exit %d, err %q", code, exit, errOut)
		}
	}
}

// The listing reads one scope and names the holder of each role.
func TestTheListingNamesWhoHoldsWhat(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[`+grantedJSON+`],"page":{"has_more":false},"grantable_roles":["MEMBER"]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"people", "ls", "--scope", "HUB", "--scope-id", peopleHubID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if q := stub.request.URL.Query(); q.Get("scope_type") != "HUB" || q.Get("scope_id") != peopleHubID {
		t.Errorf("asked %v", q)
	}
	if !strings.Contains(out, "account "+invitedID) || !strings.Contains(out, "MEMBER") {
		t.Errorf("listed %q", out)
	}
}
