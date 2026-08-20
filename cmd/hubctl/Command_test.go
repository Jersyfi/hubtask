// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// invoke runs the CLI the way the process does, with the streams and the environment in hand.
func invoke(t *testing.T, env map[string]string, stdin string, args ...string) (int, string, string) {
	t.Helper()
	if env == nil {
		env = map[string]string{}
	}
	if _, set := env[envProfile]; !set {
		// Never the real one. A test that wrote into somebody's configuration directory would be
		// a test that changed the machine it ran on.
		env[envProfile] = filepath.Join(t.TempDir(), "profile.json")
	}

	var out, errOut bytes.Buffer
	code := Run(context.Background(), args,
		Streams{In: strings.NewReader(stdin), Out: &out, Err: &errOut}, environment(env))
	return code, out.String(), errOut.String()
}

func TestTheVersionIsPrintedAndNothingElseHappens(t *testing.T) {
	code, out, _ := invoke(t, nil, "", "--version")
	if code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
	if !strings.HasPrefix(out, "hubctl "+version) {
		t.Errorf("output %q does not name the version", out)
	}
}

func TestWithoutArgumentsTheUsageIsPrintedOnStandardOutput(t *testing.T) {
	code, out, errOut := invoke(t, nil, "")
	if code != exitOK {
		t.Fatalf("exit %d, want %d", code, exitOK)
	}
	if !strings.Contains(out, "Usage:") {
		t.Errorf("no usage on standard output: %q", out)
	}
	// Asking for help is not an error, so nothing belongs on standard error - a shell that pipes
	// stdout should get the whole answer.
	if errOut != "" {
		t.Errorf("standard error carried %q", errOut)
	}
}

func TestAnUnknownCommandIsAUsageError(t *testing.T) {
	code, _, errOut := invoke(t, nil, "", "nonsense")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if !strings.Contains(errOut, "no such command") {
		t.Errorf("standard error %q does not say what was wrong", errOut)
	}
}

func TestAnUnknownFlagIsAUsageError(t *testing.T) {
	code, _, errOut := invoke(t, nil, "", "--nonsense")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d", code, exitUsage)
	}
	if errOut == "" {
		t.Error("standard error said nothing about the flag")
	}
}

// A URL that is not one is refused before any call is made, and the message says which part of it
// is wrong rather than reporting a failed request.
func TestAnImpossibleURLIsRefusedBeforeAnythingIsCalled(t *testing.T) {
	code, _, errOut := invoke(t, nil, "", "--url", "localhost:8080", "container", "ls")
	if code != exitError {
		t.Fatalf("exit %d, want %d", code, exitError)
	}
	if !strings.Contains(errOut, "scheme") {
		t.Errorf("standard error %q does not name the missing scheme", errOut)
	}
}
