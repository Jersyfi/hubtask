// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"net/http"
	"strings"
	"testing"
)

const presetsAnswer = `[
  {"key":"LIST","settings":{"levels":[{"type":"TASK","name":null}],
    "views":{"offered":["LIST_COLLAPSED"],"default":"LIST_COLLAPSED"}}},
  {"key":"FLOW","settings":{"levels":[{"type":"TASK","name":"Card"}],
    "views":{"offered":["KANBAN","LIST_COLLAPSED"],"default":"KANBAN"}}}
]`

// `--preset` names the preset a container starts from; a key that is not one is refused before
// anything is sent.
func TestCreatingAContainerFromAPresetSendsItsKey(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"id":"01936f2a-7c1e-7000-8000-0000000000a1","type":"HUB",
			"name":"Groceries","version":1}`))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "create", "--type", "HUB", "--name", "Groceries", "--preset", "LIST")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if !strings.Contains(stub.body, `"preset":"LIST"`) {
		t.Errorf("the body %q does not name the preset", stub.body)
	}

	code, _, _ = invokeAgainst(t, stub, signedIn(stub), "",
		"container", "create", "--type", "HUB", "--name", "Groceries", "--preset", "SHOPPING")
	if code == exitOK {
		t.Error("an unknown preset was sent")
	}
}

func TestListingPresetsShowsTheirLevelsAndViews(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(presetsAnswer))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "preset", "ls")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.URL.Path != "/api/v1/presets" {
		t.Errorf("asked %s", stub.request.URL.Path)
	}
	for _, want := range []string{"LIST", "TASK", "FLOW", "TASK (Card)", "KANBAN*, LIST_COLLAPSED"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table %q lacks %q", out, want)
		}
	}
}

const structuredContainer = `{"id":"01936f2a-7c1e-7000-8000-0000000000a1","type":"COLLECTION","name":"Groceries",
  "version":4,"structure":{"preset":"LIST","settings":{"completion":"MANUAL"},
  "sources":{"completion":"HUB","views":"COLLECTION"},
  "locks":{"views":{"origin":"WORKSPACE","locked_by":null}}},"kept_by_lock":["views"]}`

// UC-WRK-23 check 8 through hubctl: a change sends the preset, the settings and the resets by
// name, with the version read; the answer's table names each setting's source and lock.
func TestChangingAStructureSendsTheChangeAndShowsTheSources(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(structuredContainer))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "structure", "01936f2a-7c1e-7000-8000-0000000000a1",
		"--preset", "LIST", "--settings", `{"celebration":"ALL"}`, "--reset", "views", "--version", "3")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if stub.request.Method != http.MethodPatch ||
		stub.request.URL.Path != "/api/v1/containers/01936f2a-7c1e-7000-8000-0000000000a1/structure" ||
		stub.request.Header.Get("If-Match") != `"3"` {
		t.Errorf("sent %s %s %q", stub.request.Method, stub.request.URL.Path, stub.request.Header.Get("If-Match"))
	}
	for _, fragment := range []string{`"preset":"LIST"`, `"celebration":"ALL"`, `"reset":["views"]`} {
		if !strings.Contains(stub.body, fragment) {
			t.Errorf("the body %q lacks %s", stub.body, fragment)
		}
	}
	if !strings.Contains(out, "HUB") || !strings.Contains(out, "WORKSPACE") {
		t.Errorf("the table %q does not name sources and locks", out)
	}
	if !strings.Contains(errOut, "kept by a lock: views") {
		t.Errorf("standard error %q does not name what a lock kept", errOut)
	}
}

// The server's refusal comes back in the catalogue's words, as on every other door; a misspelt
// setting or reset is refused before anything is sent.
func TestAStructureRefusalIsTheServersAndATypoNeverLeaves(t *testing.T) {
	stub := serve(t, func(w http.ResponseWriter, _ *http.Request) {
		problemJSON(w, http.StatusUnprocessableEntity, map[string]any{
			"code": "capability_not_supported", "status": 422, "detail_code": "structure.widens",
			"params": map[string]any{"setting": "automation"},
		})
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"container", "structure", "01936f2a-7c1e-7000-8000-0000000000a1", "--settings", `{"automation":["X"]}`)
	if code == exitOK || !strings.Contains(errOut, "more than this installation offers") {
		t.Errorf("exit %d, standard error %q", code, errOut)
	}

	for _, args := range [][]string{
		{"--settings", `{"colour":"red"}`},
		{"--reset", "colour"},
	} {
		stub.request = nil
		code, _, _ = invokeAgainst(t, stub, signedIn(stub), "",
			append([]string{"container", "structure", "01936f2a-7c1e-7000-8000-0000000000a1"}, args...)...)
		if code != exitUsage || stub.request != nil {
			t.Errorf("%v: exit %d, sent %v", args, code, stub.request != nil)
		}
	}
}
