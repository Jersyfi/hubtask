// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func names(steps []step) string {
	var out []string
	for _, s := range steps {
		out = append(out, s.name)
	}
	return strings.Join(out, ", ")
}

func TestADocumentationChangeRunsVerifyAlone(t *testing.T) {
	steps, notes := selectSteps(map[string]bool{"docs": true})
	if got := names(steps); got != "tools, make verify" {
		t.Errorf("a documentation change runs %q, want make verify alone", got)
	}
	if len(notes) != 3 {
		t.Errorf("notes = %v, want the three jobs that run on every pull request and only in CI", notes)
	}
}

func TestAGoChangeRunsTheGoGatesAndTheContainersLast(t *testing.T) {
	steps, _ := selectSteps(map[string]bool{"go": true})
	got := names(steps)
	for _, want := range []string{"gate-integration", "gate-selftest", "build (linux/arm64)", "tokens without a diff", "gate-e2e"} {
		if !strings.Contains(got, want) {
			t.Errorf("a Go change does not run %q: %s", want, got)
		}
	}
	if strings.Contains(got, "workspace (") {
		t.Errorf("a Go change runs a workspace package: %s", got)
	}
	seenContainer := false
	for _, s := range steps {
		if s.container {
			seenContainer = true
		} else if seenContainer {
			t.Errorf("%q runs after a container gate - the lock would be held for it", s.name)
		}
	}
}

func TestAWebappChangeRunsItsPackageAndTheEngines(t *testing.T) {
	got := names(func() []step { s, _ := selectSteps(map[string]bool{"webapp": true}); return s }())
	for _, want := range []string{"workspace (webapp)", "the three engines", "gate-compose"} {
		if !strings.Contains(got, want) {
			t.Errorf("a webapp change does not run %q: %s", want, got)
		}
	}
	if strings.Contains(got, "workspace (website)") || strings.Contains(got, "gate-integration") {
		t.Errorf("a webapp change runs more than CI would: %s", got)
	}
}

func TestAStaleLockIsTakenOver(t *testing.T) {
	dir := filepath.Join(t.TempDir(), lockName)
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	// A pid far above any the system hands out: nobody holds it.
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte("2147483646\n/elsewhere\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	release, err := acquireLock(ctx, dir, "/here")
	if err != nil {
		t.Fatalf("a stale lock was not taken over: %v", err)
	}
	if owner, _ := os.ReadFile(filepath.Join(dir, "owner")); !strings.Contains(string(owner), "/here") {
		t.Errorf("the lock does not name its new owner: %q", owner)
	}
	release()
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Error("release left the lock behind")
	}
}

func TestALiveLockIsWaitedFor(t *testing.T) {
	dir := filepath.Join(t.TempDir(), lockName)
	if err := os.Mkdir(dir, 0o750); err != nil {
		t.Fatal(err)
	}
	// This test's own process: alive for as long as the question is asked.
	if err := os.WriteFile(filepath.Join(dir, "owner"), []byte(strconv.Itoa(os.Getpid())+"\n/other\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	if _, err := acquireLock(ctx, dir, "/here"); err == nil {
		t.Fatal("a lock held by a live process was taken")
	}
}
