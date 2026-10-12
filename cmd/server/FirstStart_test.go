// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

type issuerFake struct {
	issued identity.IssuedSetupCode
	ok     bool
	err    error
}

func (f issuerFake) Execute(context.Context) (identity.IssuedSetupCode, bool, error) {
	return f.issued, f.ok, f.err
}

// captureStderr runs fn with the process's standard error pointed at a file, and answers what was
// written there - the stream an operator reads through `docker compose logs`.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	file, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatalf("a file for standard error: %v", err)
	}
	saved := os.Stderr
	os.Stderr = file
	fn()
	os.Stderr = saved
	_ = file.Close()
	written, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatalf("reading standard error back: %v", err)
	}
	return string(written)
}

// UC-INS-01 check 1: on an empty installation the start prints the code on standard error, with
// where to enter it and until when; once a workspace exists it prints nothing like it.
func TestTheFirstStartPrintsTheSetupCodeOnStandardError(t *testing.T) {
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("the catalogue: %v", err)
	}
	render := func(code string, params map[string]string) string { return renderer.Render("en", code, params) }
	code, err := domain.NewSetupCode([]byte{1, 2, 3, 4, 5, 6, 7, 8})
	if err != nil {
		t.Fatalf("a code: %v", err)
	}
	issued := identity.IssuedSetupCode{Code: code, ExpiresAt: time.Date(2026, 10, 12, 14, 32, 0, 0, time.UTC)}

	printed := captureStderr(t, func() {
		firstStart(t.Context(), issuerFake{issued: issued, ok: true}, render, "https://tasks.example.eu")
	})
	for _, want := range []string{code.Display(), "https://tasks.example.eu", "14:32 UTC", "Set up Hubtask"} {
		if !strings.Contains(printed, want) {
			t.Errorf("standard error lacks %q:\n%s", want, printed)
		}
	}
	if strings.Contains(printed, "setup.console") {
		t.Errorf("a message code reached the console instead of its sentence:\n%s", printed)
	}

	set := captureStderr(t, func() {
		firstStart(t.Context(), issuerFake{ok: false}, render, "https://tasks.example.eu")
	})
	if strings.TrimSpace(set) != "" {
		t.Errorf("an installation that is set up printed:\n%s", set)
	}
}
