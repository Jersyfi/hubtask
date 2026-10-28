// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const bugType = `{"key":"bug","level":"TASK","name":"Bug","icon":"bug","color_token":"red",
  "completable":true,"workflow":false,"record":false,"schedulable":true,"estimable":false,
  "allowed_children":[],"position":1,"system":false,"archived":false,"version":2}`

// UC-FLW-01 check 7 from the command line: a type is listed, created, changed, archived and
// deleted, and an entry's type changed, through the same REST use cases every other door reaches.
func TestEntryTypesAreShapedFromTheCommandLine(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data":[`+bugType+`]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodGet || stub.request.URL.Path != APIPath+entryTypesPath {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}
	if !strings.Contains(out, "bug") || !strings.Contains(out, "completable,schedulable") {
		t.Errorf("the answer is not the table: %q", out)
	}

	stub = serveJSON(t, http.StatusCreated, bugType)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "create",
		"--key", "server", "--level", "TASK", "--name", "Server", "--icon", "server", "--color", "slate",
		"--record", "--schedulable=false"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPost || !strings.Contains(stub.body, `"record":true`) ||
		!strings.Contains(stub.body, `"schedulable":false`) || strings.Contains(stub.body, `"workflow"`) {
		t.Errorf("%s with %s", stub.request.Method, stub.body)
	}

	stub = serveJSON(t, http.StatusOK, bugType)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "set", "bug",
		"--name", "Defect", "--expect-version", "2"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch || stub.request.URL.Path != APIPath+entryTypesPath+"/bug" ||
		stub.body != `{"name":"Defect"}` || stub.request.Header.Get("If-Match") != `"2"` {
		t.Errorf("%s %s with %s, If-Match %q", stub.request.Method, stub.request.URL.Path, stub.body,
			stub.request.Header.Get("If-Match"))
	}

	stub = serveJSON(t, http.StatusOK, bugType)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "archive", "bug"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.body != `{"archived":true}` {
		t.Errorf("archiving sent %s", stub.body)
	}

	stub = serveJSON(t, http.StatusNoContent, "")
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "rm", "bug"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete || stub.request.URL.Path != APIPath+entryTypesPath+"/bug" {
		t.Errorf("%s %s", stub.request.Method, stub.request.URL.Path)
	}

	item := "0190a000-0000-7000-8000-000000000001"
	stub = serveJSON(t, http.StatusOK, `{"id":"`+item+`","type":"TASK","title":"Crash","version":3}`)
	if code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "type", "assign", item, "bug"); code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPut || stub.request.URL.Path != APIPath+itemsPath+"/"+item+"/entry-type" ||
		stub.body != `{"entry_type":"bug"}` {
		t.Errorf("%s %s with %s", stub.request.Method, stub.request.URL.Path, stub.body)
	}

	stub = serveJSON(t, http.StatusCreated, bugType)
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "type", "create", "--key", "bug"); code == exitOK {
		t.Error("a type without its name, level, icon and colour was sent")
	}
}
