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
