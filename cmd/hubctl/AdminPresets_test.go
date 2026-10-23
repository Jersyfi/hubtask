// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The installation's presets at the terminal (UC-INS-18 check 1): the verbs the API has.

const craftOrder = `{
  "id": "018f0000-0000-7000-8000-00000000c0de", "name": "Craft order", "version": 2,
  "is_default": true, "locks": ["views"],
  "settings": {"views": {"offered": ["KANBAN"], "default": "KANBAN"}}
}`

func TestTheInstallationsPresetsAreListed(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, `{"data": [`+craftOrder+`]}`)
	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "presets", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != APIPath+adminPresetsPath {
		t.Errorf("the read called %s", stub.request.URL.Path)
	}
	for _, expected := range []string{"Craft order", "default", "views"} {
		if !strings.Contains(out, expected) {
			t.Errorf("%s is missing from %q", expected, out)
		}
	}
}

func TestAnInstallationPresetIsCreatedFromHubtasksWithItsLocks(t *testing.T) {
	stub := serveJSON(t, http.StatusCreated, craftOrder)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "presets", "create",
		"--name", "Craft order", "--from", "flow", "--default", "--lock", "views, fields.TASK.DUE_DATE",
		"--settings", `{"levels": [{"type": "TASK", "name": "Order"}]}`)
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatal(err)
	}
	locks, _ := sent["locks"].([]any)
	settings, _ := sent["settings"].(map[string]any)
	if sent["name"] != "Craft order" || sent["preset"] != "FLOW" || sent["is_default"] != true ||
		len(locks) != 2 || locks[1] != "fields.TASK.DUE_DATE" || settings["levels"] == nil {
		t.Errorf("sent %s", stub.body)
	}
}

func TestAnInstallationPresetIsChangedWithTheVersionNamed(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, craftOrder)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "presets", "set",
		"--default", "no", "--expected-version", "2", "018f0000-0000-7000-8000-00000000c0de")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch || stub.request.Header.Get("If-Match") != `"2"` {
		t.Errorf("sent %s with If-Match %q", stub.request.Method, stub.request.Header.Get("If-Match"))
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["is_default"] != false || sent["locks"] != nil || sent["name"] != nil {
		t.Errorf("a change of the default alone sent %s", stub.body)
	}
	if code, _, _ := invokeAgainst(t, stub, signedIn(stub), "", "admin", "presets", "set", "--default", "maybe", "x"); code == exitOK {
		t.Error("--default maybe was accepted")
	}
}

func TestAnInstallationPresetIsWithdrawn(t *testing.T) {
	stub := serveJSON(t, http.StatusNoContent, ``)
	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "presets", "rm", "018f0000-0000-7000-8000-00000000c0de")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodDelete || !strings.HasSuffix(stub.request.URL.Path, "/admin/presets/018f0000-0000-7000-8000-00000000c0de") {
		t.Errorf("called %s %s", stub.request.Method, stub.request.URL.Path)
	}
}
