// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const policyID = "01936f2a-7c1e-7000-8000-0000000000d1"

const onePolicy = `{"id":"` + policyID + `","data_kind":"COMPLETED_ITEM",
  "scope":{"kind":"TENANT","id":null},"retain_days":90,"action":"TRASH",
  "then_after_days":30,"then_action":"HARD_DELETE","enabled":true,"in_force":true}`

func TestListingRetentionRulesPassesTheContainerAndTheEffectiveFlag(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[`+onePolicy+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"retention", "ls", "--container", collectionID, "--effective")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	query := stub.request.URL.Query()
	if query.Get("container_id") != collectionID || query.Get("effective") != "true" {
		t.Errorf("query %v", query)
	}
	// The chain is one rule and reads as one: what happens, and what happens after that.
	if !strings.Contains(out, "HARD_DELETE +30d") {
		t.Errorf("the second step is not shown: %q", out)
	}
}

func TestAddingARuleSendsTheScopeTheDaysAndTheAction(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, onePolicy)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"retention", "add", "--kind", "COMPLETED_ITEM", "--days", "90", "--action", "TRASH",
		"--scope", "COLLECTION", "--scope-id", collectionID, "--justification", "the audit asked")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the body is not JSON: %v", err)
	}
	scope, _ := sent["scope"].(map[string]any)
	if scope["kind"] != "COLLECTION" || scope["id"] != collectionID {
		t.Errorf("scope %v", sent["scope"])
	}
	if sent["retain_days"] != float64(90) || sent["action"] != "TRASH" {
		t.Errorf("the rule lost something: %v", sent)
	}
	if sent["justification"] != "the audit asked" {
		t.Errorf("the justification did not travel: %v", sent["justification"])
	}
}

func TestARuleWithoutItsThreeAnswersIsAUsageError(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, onePolicy)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"retention", "add", "--kind", "COMPLETED_ITEM")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "--days") {
		t.Errorf("the complaint does not say what is missing: %q", errOut)
	}
}

// The preview is what makes a rule safe to switch on: how much it takes, what stops it, and a few
// examples. "Nothing" is a sentence a person needs; an empty column is not.
func TestPreviewingARuleShowsWhatWouldGoAndWhatIsStoppingIt(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"matched":128,"blocked":{"legal_hold":3,"restriction":1},
	  "share_of_scope":0.42,
	  "samples":[{"id":"`+itemID+`","title":"Buy milk","effective_at":"2026-09-01T00:00:00Z"}]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "retention", "preview", policyID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + retentionPath + "/" + policyID + ":preview"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
	if !strings.Contains(out, "128") || !strings.Contains(out, "42.0%") {
		t.Errorf("the size of the effect is not shown: %q", out)
	}
	if !strings.Contains(out, "legal_hold=3 restriction=1") {
		t.Errorf("the reasons are not shown in a stable order: %q", out)
	}
	if !strings.Contains(out, "Buy milk") {
		t.Errorf("the samples are not shown: %q", out)
	}
}

func TestAPreviewWithNothingInTheWaySaysSo(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"matched":4,"blocked":{},"share_of_scope":0.01,"samples":[]}`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "retention", "preview", policyID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(out, "nothing") {
		t.Errorf("output %q", out)
	}
}

func TestRetainingAnEntryPostsToItsRetainPath(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneItem)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "retention", "retain", itemID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if want := APIPath + "/items/" + itemID + ":retain"; stub.request.URL.Path != want {
		t.Errorf("path %q, want %q", stub.request.URL.Path, want)
	}
}
