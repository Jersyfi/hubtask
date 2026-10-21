// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const workspaceStructureAnswer = `{"preset":null,"settings":{"celebration":"SUBTLE"},
  "sources":{"celebration":"WORKSPACE"},
  "locks":{"fields.TASK.DUE_DATE":{"origin":"WORKSPACE","locked_by":null}},"default_preset":"TASKS"}`

// The administrator's door to the lock: the whole set of locks, the default preset, the version.
func TestLockingTheWorkspacesStructureSendsTheWholeSet(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(workspaceStructureAnswer))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"workspace", "structure", "--lock", "fields.TASK.DUE_DATE", "--lock", "views",
		"--default-preset", "TASKS", "--version", "4")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch || stub.request.URL.Path != "/api/v1/tenant/structure" ||
		stub.request.Header.Get("If-Match") != `"4"` {
		t.Errorf("sent %s %s", stub.request.Method, stub.request.URL.Path)
	}
	for _, fragment := range []string{`"locks":["fields.TASK.DUE_DATE","views"]`, `"default_preset":"TASKS"`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q lacks %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "default preset") || !strings.Contains(out, "TASKS") {
		t.Errorf("the table %q", out)
	}
}

func TestClearingTheWorkspacesLocksAndDefaultSendsThemEmpty(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(workspaceStructureAnswer))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"workspace", "structure", "--no-locks", "--no-default-preset")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, fragment := range []string{`"locks":[]`, `"default_preset":null`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q lacks %s", stub.body, fragment)
		}
	}

	stub.request = nil
	code, _, _ = invokeAgainst(t, stub, signedIn(stub), "", "workspace", "structure", "--no-locks", "--lock", "views")
	if code != exitUsage || stub.request != nil {
		t.Errorf("contradicting flags: exit %d", code)
	}
}
