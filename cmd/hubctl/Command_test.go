// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"context"
	"net/http"
	"os"
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

// The CLI speaks the language of the person running it, from the same catalogues the server
// renders email from (M-01): HUBTASK_LOCALE first, then the shell's own variables.
func TestTheCLISpeaksThePersonsLanguage(t *testing.T) {
	stub := serve(t, func(http.ResponseWriter, *http.Request) {
		t.Error("a call was made with an unreadable identifier")
	})

	for _, tc := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"HUBTASK_LOCALE", map[string]string{envLocale: "de"}, "ist keine gültige Kennung"},
		{"LANG in POSIX form", map[string]string{"LANG": "de_AT.UTF-8"}, "ist keine gültige Kennung"},
		{"LC_ALL wins over LANG", map[string]string{"LC_ALL": "en_GB.UTF-8", "LANG": "de_DE.UTF-8"}, "is not a valid identifier"},
		{"C is no language", map[string]string{"LANG": "C"}, "is not a valid identifier"},
		{"nothing set", nil, "is not a valid identifier"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := signedIn(stub)
			for name, value := range tc.env {
				env[name] = value
			}
			code, _, errOut := invokeAgainst(t, stub, env, "",
				"remind", "add", itemID, "--at", "-PT30M", "--to", "not-an-id")
			if code != exitUsage {
				t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
			}
			if !strings.Contains(errOut, tc.want) {
				t.Errorf("the refusal reads %q, want it to contain %q", errOut, tc.want)
			}
		})
	}
}

// An operator's directory is read by the CLI too, under the server's own variable.
func TestTheCLIReadsTheOverrideDirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "en.json"),
		[]byte(`{"shared.id_malformed": "Not an identifier the operator would accept: {value}"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	stub := serve(t, func(http.ResponseWriter, *http.Request) {})
	env := signedIn(stub)
	env[envLocaleDir] = dir

	code, _, errOut := invokeAgainst(t, stub, env, "",
		"remind", "add", itemID, "--at", "-PT30M", "--to", "not-an-id")
	if code != exitUsage {
		t.Fatalf("exit %d, want %d: %s", code, exitUsage, errOut)
	}
	if !strings.Contains(errOut, "the operator would accept") {
		t.Errorf("the override was not read: %q", errOut)
	}
}
