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
