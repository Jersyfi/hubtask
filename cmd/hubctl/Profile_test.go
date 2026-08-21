// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// environment builds the lookup the CLI is handed, so that no test touches the real one.
func environment(pairs map[string]string) func(string) string {
	return func(name string) string { return pairs[name] }
}

func TestAProfileSurvivesARoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "profile.json")
	written := Profile{BaseURL: "https://hub.example.com", Token: secret.New("hbt_pat_secret")}

	if err := SaveProfile(path, written); err != nil {
		t.Fatalf("saving: %v", err)
	}
	read, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if read.BaseURL != written.BaseURL {
		t.Errorf("base URL %q, want %q", read.BaseURL, written.BaseURL)
	}
	if read.Token.Reveal() != written.Token.Reveal() {
		t.Error("the token did not survive the round trip")
	}
}

// The file holds a credential. A token in a world-readable file is the same mistake as a
// world-readable private key, and it is the kind of mistake that is only ever noticed afterwards.
func TestTheProfileIsReadableOnlyByItsOwner(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission bits do not describe what Windows enforces")
	}
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := SaveProfile(path, Profile{BaseURL: "https://h", Token: secret.New("t")}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if mode := info.Mode().Perm(); mode != profileFileMode {
		t.Errorf("mode %o, want %o", mode, profileFileMode)
	}
}

// Overwriting is what `hubctl auth login` does a second time, and it must not leave two profiles
// or half of one.
func TestSavingOverAnExistingProfileReplacesIt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "profile.json")
	for _, url := range []string{"https://first.example.com", "https://second.example.com"} {
		if err := SaveProfile(path, Profile{BaseURL: url, Token: secret.New("t")}); err != nil {
			t.Fatalf("saving %s: %v", url, err)
		}
	}

	read, err := LoadProfile(path)
	if err != nil {
		t.Fatalf("loading: %v", err)
	}
	if read.BaseURL != "https://second.example.com" {
		t.Errorf("base URL %q, want the second one", read.BaseURL)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	if len(entries) != 1 {
		t.Errorf("%d files in the profile directory, want only the profile", len(entries))
	}
}

func TestAProfileThatIsNotThereIsNotAnError(t *testing.T) {
	profile, err := LoadProfile(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("loading an absent profile: %v", err)
	}
	if profile.BaseURL != "" || !profile.Token.IsEmpty() {
		t.Error("an absent profile came back with something in it")
	}
}

func TestAProfileThatIsNotAProfileIsAnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	if err := os.WriteFile(path, []byte("not json"), profileFileMode); err != nil {
		t.Fatalf("writing: %v", err)
	}
	if _, err := LoadProfile(path); err == nil {
		t.Error("a file that is not a profile loaded without complaint")
	}
}

func TestForgettingAProfileThatIsNotThereSucceeds(t *testing.T) {
	if err := ForgetProfile(filepath.Join(t.TempDir(), "absent.json")); err != nil {
		t.Errorf("forgetting an absent profile: %v", err)
	}
}

func TestResolutionPutsTheEnvironmentOverTheFileAndTheFlagOverBoth(t *testing.T) {
	path := filepath.Join(t.TempDir(), "profile.json")
	stored := Profile{BaseURL: "https://stored.example.com", Token: secret.New("stored-token")}
	if err := SaveProfile(path, stored); err != nil {
		t.Fatalf("saving: %v", err)
	}

	for _, tc := range []struct {
		name      string
		env       map[string]string
		flagURL   string
		wantURL   string
		wantToken string
	}{
		{"the file alone", nil, "", "https://stored.example.com", "stored-token"},
		{"the environment over the file",
			map[string]string{envURL: "https://env.example.com", envToken: "env-token"}, "",
			"https://env.example.com", "env-token"},
		{"the flag over the environment",
			map[string]string{envURL: "https://env.example.com"}, "https://flag.example.com",
			"https://flag.example.com", "stored-token"},
		{"a trailing slash is not part of the address", nil, "https://flag.example.com/",
			"https://flag.example.com", "stored-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			profile, err := ResolveProfile(environment(tc.env), path, tc.flagURL)
			if err != nil {
				t.Fatalf("resolving: %v", err)
			}
			if profile.BaseURL != tc.wantURL {
				t.Errorf("base URL %q, want %q", profile.BaseURL, tc.wantURL)
			}
			if profile.Token.Reveal() != tc.wantToken {
				t.Errorf("token %q, want %q", profile.Token.Reveal(), tc.wantToken)
			}
		})
	}
}

func TestABaseURLIsAnAddress(t *testing.T) {
	for _, tc := range []struct {
		name string
		raw  string
		want string
	}{
		{"a host and a scheme", "http://localhost:8080", "http://localhost:8080"},
		{"a trailing slash", "https://hub.example.com/", "https://hub.example.com"},
		{"a path prefix is kept", "https://example.com/hubtask", "https://example.com/hubtask"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := NormaliseBaseURL(tc.raw)
			if err != nil {
				t.Fatalf("normalising %q: %v", tc.raw, err)
			}
			if got != tc.want {
				t.Errorf("%q, want %q", got, tc.want)
			}
		})
	}

	for _, tc := range []struct{ name, raw string }{
		{"no scheme", "localhost:8080"},
		{"a scheme that is not HTTP", "file:///etc/passwd"},
		{"no host", "https://"},
		{"a query", "https://example.com?tenant=a"},
		{"a fragment", "https://example.com#top"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NormaliseBaseURL(tc.raw); err == nil {
				t.Errorf("%q was accepted as an installation address", tc.raw)
			}
		})
	}
}
