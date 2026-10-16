// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// Plans at the terminal (UC-INS-13 check 7): the same verbs the API has.

const familyPlan = `{
  "name": "family", "display_name": "Family", "version": 3, "workspaces": 2,
  "created_at": "2026-10-16T09:00:00Z", "updated_at": "2026-10-16T09:00:00Z",
  "quotas": {"items": {"set": true, "value": 5000, "locked": true, "kind": "COUNT"},
             "media_bytes": {"set": false, "locked": false, "kind": "COUNT"}},
  "sign_in": {"min_length": {"set": false, "locked": false, "kind": "COUNT"}},
  "left_out": ["WEBHOOKS"]
}`

func TestThePlansAreListed(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data": [`+familyPlan+`]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "plan", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+adminPlansPath {
		t.Errorf("the read called %s", stub.request.URL.Path)
	}
	for _, expected := range []string{"family", "Family", "WEBHOOKS"} {
		if !strings.Contains(out, expected) {
			t.Errorf("%s is missing from %q", expected, out)
		}
	}
}

func TestAPlanIsCreatedWithTheFeaturesItLeavesOut(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, familyPlan)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "plan", "create",
		"--name", "family", "--display-name", "Family", "--leave-out", "webhooks")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["name"] != "family" || sent["left_out"].([]any)[0] != "WEBHOOKS" {
		t.Errorf("sent %s", stub.body)
	}
}

// One key changed, the rest sent back as read, with the version read: a PUT replaces, and a save
// on a plan somebody else changed is refused rather than undoing theirs.
func TestAPlanKeyIsSetWithTheVersionRead(t *testing.T) {
	var put *http.Request
	var putBody string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			put = r.Clone(r.Context())
		}
		_, _ = w.Write([]byte(familyPlan))
	})
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "plan", "set",
		"--locked", "family", "quotas.media_bytes", "1000000")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	putBody = stub.body
	if put == nil || put.Header.Get("If-Match") != `"3"` {
		t.Fatalf("the change was not sent with the version read: %+v", put)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(putBody), &sent); err != nil {
		t.Fatal(err)
	}
	quotas, _ := sent["quotas"].(map[string]any)
	media, _ := quotas["media_bytes"].(map[string]any)
	items, _ := quotas["items"].(map[string]any)
	if media["value"] != float64(1000000) || media["locked"] != true || items["value"] != float64(5000) {
		t.Errorf("sent %s", putBody)
	}
	if sent["left_out"].([]any)[0] != "WEBHOOKS" {
		t.Errorf("the features left out were not sent back: %s", putBody)
	}
}

func TestAWorkspaceIsMovedFromTheTerminal(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"tenant": {"id": "0192f000-0000-7000-8000-0000000000e1",
	  "slug": "maria", "display_name": "Maria", "status": "ACTIVE", "created_at": "2026-10-01T08:00:00Z",
	  "plan": "family", "version": 5}, "features_resting": ["WEBHOOKS"], "exceptions_kept": ["items"]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "move-plan",
		"0192f000-0000-7000-8000-0000000000e1", "--plan", "family", "--expected-version", "4")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.HasSuffix(stub.request.URL.Path, ":move-plan") || !strings.Contains(stub.body, `"expected_version":4`) {
		t.Errorf("called %s with %s", stub.request.URL.Path, stub.body)
	}
	if !strings.Contains(out, "family") || !strings.Contains(out, "items") {
		t.Errorf("printed %q", out)
	}
}

func TestProvisioningNamesThePlan(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, `{"id": "0192f000-0000-7000-8000-0000000000e1", "slug": "maria",
	  "display_name": "Maria", "status": "ACTIVE", "created_at": "2026-10-01T08:00:00Z",
	  "owner_account_id": "0192f000-0000-7000-8000-0000000000e2", "owner_redemption_token": "hbt_red_x",
	  "default_hub_id": "0192f000-0000-7000-8000-0000000000e3", "example_collection_id": "0192f000-0000-7000-8000-0000000000e4"}`)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "tenant", "create",
		"--slug", "maria", "--name", "Maria", "--owner-email", "maria@example.org", "--plan", "family")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"plan":"family"`) {
		t.Errorf("sent %s", stub.body)
	}
}
