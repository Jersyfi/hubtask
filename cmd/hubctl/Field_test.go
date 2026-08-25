// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const fieldID = "01936f2a-7c1e-7000-8000-0000000000f1"

const oneDefinition = `{"id":"` + fieldID + `","key":"effort","kind":"NUMBER",
  "collection_id":"` + collectionID + `","applies_to":["TASK","WORK_PACKAGE"],
  "is_required":false,"options":[],"version":1}`

func TestListingFieldDefinitionsPassesTheCollectionOn(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `[`+oneDefinition+`]`)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"field", "ls", "--collection", collectionID)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if got := stub.request.URL.Query().Get("collection_id"); got != collectionID {
		t.Errorf("collection_id %q", got)
	}
	if !strings.Contains(out, "effort") || !strings.Contains(out, "NUMBER") {
		t.Errorf("output %q is not the table", out)
	}
}

func TestDefiningAFieldSendsTheDocumentedPayload(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, oneDefinition)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"field", "define", "--key", "effort", "--kind", "NUMBER",
		"--collection", collectionID, "--applies-to", "TASK, WORK_PACKAGE")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, fragment := range []string{
		`"key":"effort"`, `"kind":"NUMBER"`,
		`"collection_id":"` + collectionID + `"`, `"applies_to":["TASK","WORK_PACKAGE"]`,
	} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q is missing %s", stub.body, fragment)
		}
	}
}

func TestAFieldKindThatIsNotOneIsRefusedBeforeTheCall(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with a kind that does not exist")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"field", "define", "--key", "effort", "--kind", "DECIMAL")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	expected, _ := catalogue(t).Message("fields.kind_unknown", map[string]string{"value": "DECIMAL"})
	if !strings.Contains(errOut, expected) {
		t.Errorf("the message %q is not the catalogue's: %q", errOut, expected)
	}
}

func TestSettingAFieldPutsTheValueUnderTheKey(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneItem)

	for _, values := range []struct {
		flag, sent string
	}{
		// What reads as JSON goes as JSON, anything else as text.
		{"3", `{"value":3}`},
		{"high", `{"value":"high"}`},
		{`["a","b"]`, `{"value":["a","b"]}`},
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
			"field", "set", itemID, "effort", "--value", values.flag)
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		if want := APIPath + "/items/" + itemID + "/custom-fields/effort"; stub.request.URL.Path != want {
			t.Errorf("path %q, want %q", stub.request.URL.Path, want)
		}
		if stub.request.Method != http.MethodPut {
			t.Errorf("method %s", stub.request.Method)
		}
		if stub.body != values.sent {
			t.Errorf("--value %s travelled as %q, want %q", values.flag, stub.body, values.sent)
		}
	}
}

func TestClearingAFieldSendsNull(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, oneItem)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"field", "set", itemID, "effort", "--clear")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.body != `{"value":null}` {
		t.Errorf("--clear travelled as %q, want the null that removes the key", stub.body)
	}
}

func TestSettingAFieldNeedsExactlyOneOfValueAndClear(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an ambiguous request")
	})

	for _, args := range [][]string{
		{"field", "set", itemID, "effort"},
		{"field", "set", itemID, "effort", "--value", "3", "--clear"},
		{"field", "set", itemID, "--value", "3"},
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", args...)
		if code != exitUsage {
			t.Fatalf("%v: exit %d, want %d: %s", args, code, exitUsage, errOut)
		}
	}
}
