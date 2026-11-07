// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func checkFile(t *testing.T, content string, env map[string]string) (int, string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "instance.json")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	render := func(code string, params map[string]string) string {
		return code + " " + params["area"] + params["variable"]
	}
	lookup := func(name string) (string, bool) { value, ok := env[name]; return value, ok }
	readFile := func(string) ([]byte, error) { return nil, errors.New("no such file") }
	code := instanceCommand([]string{"check", path}, &stdout, &stderr, render, lookup, readFile)
	return code, stdout.String(), stderr.String()
}

// A typo in the instance file refuses the start; the check finds it before the restart does.
func TestInstanceCheckFindsWhatWouldRefuseTheStart(t *testing.T) {
	cases := []struct {
		name, content, want string
	}{
		{"a trailing comma", `{"business_calendars": [],}`, "config.instance_file_malformed"},
		{"a misspelt area", `{"bussiness_calendars": []}`, "config.instance_file_area_unknown bussiness_calendars"}, //nolint:misspell // the misspelling is the case under test
		{"an AI key nobody set", `{"ai_models": [{"display_name": "M", "api_key_env": "MODEL_KEY"}]}`,
			"config.instance_file_ai_key_missing MODEL_KEY"},
		{"a calendar without its fields", `{"business_calendars": [{"key": ""}]}`, ""},
		{"a secret file that is not there", `{"platform_subscriptions": [{"target_url": "https://x.example", "secret_file": "/nowhere"}]}`,
			"config.instance_file_unreadable"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, _, stderr := checkFile(t, tc.content, nil)
			if code != 1 {
				t.Fatalf("exit %d, want 1; stderr %q", code, stderr)
			}
			if !strings.Contains(stderr, tc.want) {
				t.Fatalf("stderr %q does not name %q", stderr, tc.want)
			}
		})
	}
}

func TestInstanceCheckAcceptsAValidFile(t *testing.T) {
	code, stdout, stderr := checkFile(t, `{"ai_models": [{"display_name": "M", "api_key_env": "MODEL_KEY"}]}`,
		map[string]string{"MODEL_KEY": "k"})
	if code != 0 || !strings.Contains(stdout, "cli.instance_file_valid") {
		t.Fatalf("exit %d, stdout %q, stderr %q", code, stdout, stderr)
	}
}

func TestInstanceCheckNamesAMissingFile(t *testing.T) {
	var stdout, stderr bytes.Buffer
	render := func(code string, _ map[string]string) string { return code }
	code := instanceCommand([]string{"check", filepath.Join(t.TempDir(), "absent.json")}, &stdout, &stderr, render,
		func(string) (string, bool) { return "", false }, os.ReadFile)
	if code != 1 || !strings.Contains(stderr.String(), "config.instance_file_unreadable") {
		t.Fatalf("exit %d, stderr %q", code, stderr.String())
	}
	if code := instanceCommand(nil, &stdout, &stderr, render, func(string) (string, bool) { return "", false }, os.ReadFile); code != 2 {
		t.Fatalf("no arguments: exit %d, want 2", code)
	}
}
