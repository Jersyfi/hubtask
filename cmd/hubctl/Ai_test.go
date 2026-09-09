// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// The provider configuration as an operator meets it (J-02, J-16). What is asked here is the one
// property that matters: no path through this command prints a key, and no path sends one that
// somebody did not put in the environment.

const configuredProvider = `{"kind":"OPENAI_COMPATIBLE","base_url":"https://api.example.org/v1",
  "completion_model":"gpt-4o-mini","embedding_model":"text-embedding-3-small",
  "jurisdiction":"THIRD_COUNTRY","has_api_key":true,"processing_allowed":true,
  "created_at":"2026-09-09T08:00:00Z","version":3}`

// The decision, printed: which provider, which models, where it processes, and whether a key is
// stored - which is the whole of what this surface says about one.
func TestTheProviderShowsItsDecisionAndNeverItsKey(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, configuredProvider)

	code, out, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "config", "show")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}
	for _, want := range []string{"OPENAI_COMPATIBLE", "gpt-4o-mini", "THIRD_COUNTRY"} {
		if !strings.Contains(out, want) {
			t.Errorf("the table does not say %q: %q", want, out)
		}
	}
	// The API answers no key, so there is nothing to print - and `has_api_key` is what tells an
	// operator "configured with a key" from "configured without one".
	if !strings.Contains(out, "yes") {
		t.Errorf("the table does not say whether a key is stored: %q", out)
	}
}

// The key comes from the environment and never from a flag: a flag lands in the shell's history,
// in `ps`, and in whatever captures a terminal.
func TestTheProviderKeyIsTakenFromTheEnvironmentAndNotEchoed(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, configuredProvider)
	environment := signedIn(stub)
	environment[aiKeyVariable] = "sk-the-secret-itself"

	code, out, errOut := invokeAgainst(t, stub, environment, "",
		"ai", "config-set", "--kind", "OPENAI_COMPATIBLE", "--jurisdiction", "EEA",
		"--base-url", "https://api.example.org/v1", "--allow-processing")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the request is not JSON: %v", err)
	}
	if sent["api_key"] != "sk-the-secret-itself" {
		t.Errorf("the key did not reach the request: %v", sent["api_key"])
	}
	if sent["processing_allowed"] != true {
		t.Error("consent is its own decision and did not travel")
	}
	// And nowhere else. Both streams, because a mistake here would most likely be a debug line.
	if strings.Contains(out, "sk-the-secret") || strings.Contains(errOut, "sk-the-secret") {
		t.Errorf("the key was printed:\nout: %q\nerr: %q", out, errOut)
	}
}

// An unset variable sends no key at all rather than an empty one: the contract reads an omitted key
// as "keep the one already sealed" and an empty string as "clear it", and sending the second for
// the first would silently unconfigure a working provider.
func TestWithoutTheVariableNoKeyIsSentAtAll(t *testing.T) {
	stub := serveJSON(t, http.StatusOK, configuredProvider)

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "",
		"ai", "config-set", "--kind", "OLLAMA", "--jurisdiction", "SELF_HOSTED",
		"--base-url", "http://localhost:11434")
	if code != exitOK {
		t.Fatalf("exit %d: %s", code, errOut)
	}

	var sent map[string]any
	if err := json.Unmarshal([]byte(stub.body), &sent); err != nil {
		t.Fatalf("the request is not JSON: %v", err)
	}
	if _, present := sent["api_key"]; present {
		t.Errorf("an unset variable sent a key anyway: %v", sent["api_key"])
	}
}

// Both halves of the decision are required, because a provider set with neither is a provider
// nobody can reason about.
func TestConfiguringNeedsTheKindAndTheJurisdiction(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made without the required flags")
	})

	code, _, errOut := invokeAgainst(t, stub, signedIn(stub), "", "ai", "config-set", "--kind", "OLLAMA")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "--jurisdiction") {
		t.Errorf("the message %q does not name what is missing", errOut)
	}
}
