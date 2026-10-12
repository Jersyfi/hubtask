// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// `hubctl admin ai` and `hubctl ai source` (UC-AI-06 check 1, UC-AI-05 check 6): the key from the
// environment and never printed, an offered model chosen by its name.

const offeredList = `[{"id":"0192f000-0000-7000-8000-0000000000e1","display_name":"Large (EU)",
  "processor":"A processor","kind":"OPENAI_COMPATIBLE","jurisdiction":"EEA","has_api_key":true,
  "created_at":"2026-10-12T09:00:00Z","version":1}]`

const sourceAnswer = `{"source":"OWN","sources":"EITHER","installation_sources":"EITHER",
  "sources_origin":"INSTANCE","min_jurisdiction":"EEA","min_jurisdiction_origin":"INSTANCE","version":1,
  "offered":[{"id":"0192f000-0000-7000-8000-0000000000e1","display_name":"Large (EU)",
    "processor":"A processor","jurisdiction":"EEA"}]}`

func TestAnOperatorOffersAModelWithTheKeyFromTheEnvironment(t *testing.T) {
	var posted string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posted = stub.body
			_, _ = w.Write([]byte(strings.TrimPrefix(strings.TrimSuffix(offeredList, "]"), "[")))
			return
		}
		_, _ = w.Write([]byte(offeredList))
	})
	env := signedIn(stub)
	env[aiKeyVariable] = "the-offered-key"

	code, out, errOut := invokeAgainst(t, stub, env, "", "admin", "ai", "add",
		"--name", "Large (EU)", "--processor", "A processor", "--kind", "OPENAI_COMPATIBLE",
		"--base-url", "https://offered.example/v1", "--completion-model", "large", "--jurisdiction", "EEA")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(posted), &body); err != nil {
		t.Fatalf("the offer is not JSON: %v (%q)", err, posted)
	}
	if body["display_name"] != "Large (EU)" || body["api_key"] != "the-offered-key" || body["processor"] != "A processor" {
		t.Errorf("offered %v", body)
	}
	if strings.Contains(out, "the-offered-key") {
		t.Error("the key was printed")
	}
	if !strings.Contains(out, "Large (EU)") {
		t.Errorf("the list is not printed: %q", out)
	}
}

func TestAWorkspaceChoosesAnOfferedModelByName(t *testing.T) {
	var put string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			put = stub.body
		}
		_, _ = w.Write([]byte(sourceAnswer))
	})

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "source", "set",
		"--source", "OFFERED", "--model", "large (eu)")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(put), &body); err != nil {
		t.Fatalf("the choice is not JSON: %v", err)
	}
	if body["source"] != "OFFERED" || body["offered_model_id"] != "0192f000-0000-7000-8000-0000000000e1" {
		t.Errorf("chose %v", body)
	}
	if !strings.Contains(out, "Large (EU) - A processor, EEA") {
		t.Errorf("the offered model is not named with its processor and jurisdiction: %q", out)
	}

	code, _, _ = invokeAgainst(t, stub, signedIn(stub), "", "ai", "source", "set", "--source", "OFFERED", "--model", "Nobody's")
	if code == exitOK {
		t.Error("a model nobody offers was chosen")
	}
}
