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

// UC-AI-06 check 1: `set` changes the model it names, whole, with the key from the environment;
// the key is never printed.
func TestAnOperatorChangesAnOfferedModelWithTheKeyFromTheEnvironment(t *testing.T) {
	var method, path, sent string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			method, path, sent = r.Method, r.URL.Path, stub.body
			_, _ = w.Write([]byte(strings.TrimPrefix(strings.TrimSuffix(offeredList, "]"), "[")))
			return
		}
		_, _ = w.Write([]byte(offeredList))
	})
	env := signedIn(stub)
	env[aiKeyVariable] = "the-changed-key"

	code, out, errOut := invokeAgainst(t, stub, env, "", "admin", "ai", "set", "0192f000-0000-7000-8000-0000000000e1",
		"--name", "Large (EU)", "--processor", "Another processor", "--kind", "OPENAI_COMPATIBLE",
		"--base-url", "https://offered.example/v2", "--completion-model", "larger", "--jurisdiction", "EEA")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if method != http.MethodPut || !strings.HasSuffix(path, "/admin/ai-providers/0192f000-0000-7000-8000-0000000000e1") {
		t.Fatalf("the change went to %s %s", method, path)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(sent), &body); err != nil {
		t.Fatalf("the change is not JSON: %v (%q)", err, sent)
	}
	if body["processor"] != "Another processor" || body["base_url"] != "https://offered.example/v2" ||
		body["completion_model"] != "larger" || body["api_key"] != "the-changed-key" {
		t.Errorf("changed %v", body)
	}
	if strings.Contains(out, "the-changed-key") || strings.Contains(errOut, "the-changed-key") {
		t.Error("the key was printed")
	}

	// Without the variable the change sends no key, and the server keeps the sealed one.
	code, _, errOut = invokeAgainst(t, stub, signedIn(stub), "", "admin", "ai", "set", "0192f000-0000-7000-8000-0000000000e1",
		"--name", "Large (EU)", "--processor", "A processor", "--kind", "OPENAI_COMPATIBLE",
		"--base-url", "https://offered.example/v1", "--jurisdiction", "EEA")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	body = map[string]any{}
	if err := json.Unmarshal([]byte(sent), &body); err != nil {
		t.Fatalf("the change is not JSON: %v (%q)", err, sent)
	}
	if _, sends := body["api_key"]; sends {
		t.Errorf("a change without the variable sent a key: %v", body)
	}

	code, _, _ = invokeAgainst(t, stub, signedIn(stub), "", "admin", "ai", "set",
		"--name", "Large (EU)", "--processor", "A processor", "--kind", "OPENAI_COMPATIBLE",
		"--base-url", "https://offered.example/v1", "--jurisdiction", "EEA")
	if code == exitOK {
		t.Error("a change naming no model was sent")
	}
}

// UC-AI-06 check 1: `rm` withdraws the model it names.
func TestAnOperatorWithdrawsAnOfferedModel(t *testing.T) {
	var method, path string
	stub := serve(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			method, path = r.Method, r.URL.Path
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "admin", "ai", "rm",
		"0192f000-0000-7000-8000-0000000000e1")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	if method != http.MethodDelete || !strings.HasSuffix(path, "/admin/ai-providers/0192f000-0000-7000-8000-0000000000e1") {
		t.Errorf("the withdrawal went to %s %s", method, path)
	}

	method = ""
	code, _, _ = invokeAgainst(t, stub, signedIn(stub), "", "admin", "ai", "rm")
	if code == exitOK || method != "" {
		t.Error("a withdrawal naming no model was sent")
	}
}

// UC-AI-05 check 6: among several offered models, the name picks the one it names.
func TestAWorkspaceChoosesOneOfSeveralOfferedModelsByName(t *testing.T) {
	const twoOffered = `{"source":"OWN","sources":"EITHER","installation_sources":"EITHER",
  "sources_origin":"INSTANCE","min_jurisdiction":"EEA","min_jurisdiction_origin":"INSTANCE","version":1,
  "offered":[{"id":"0192f000-0000-7000-8000-0000000000e1","display_name":"Large (EU)",
    "processor":"A processor","jurisdiction":"EEA"},
   {"id":"0192f000-0000-7000-8000-0000000000e2","display_name":"Small (EU)",
    "processor":"Another processor","jurisdiction":"EEA"}]}`
	var put string
	var stub *installation
	stub = serve(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPut {
			put = stub.body
		}
		_, _ = w.Write([]byte(twoOffered))
	})

	for name, id := range map[string]string{
		"small (eu)": "0192f000-0000-7000-8000-0000000000e2",
		"Large (EU)": "0192f000-0000-7000-8000-0000000000e1",
	} {
		code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "source", "set",
			"--source", "OFFERED", "--model", name)
		if code != exitOK {
			t.Fatalf("exit %d: %s", code, errOut)
		}
		var body map[string]any
		if err := json.Unmarshal([]byte(put), &body); err != nil {
			t.Fatalf("the choice is not JSON: %v", err)
		}
		if body["offered_model_id"] != id {
			t.Errorf("%q chose %v, want %s", name, body["offered_model_id"], id)
		}
	}
}
