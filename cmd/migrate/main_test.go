// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package main

import (
	"context"
	"database/sql"
	"testing"
	"time"
)

func TestConnectWaitReadsADurationAndFallsBackToOneAttempt(t *testing.T) {
	cases := map[string]struct {
		value string
		want  time.Duration
	}{
		"unset":      {"", 0},
		"a duration": {"90s", 90 * time.Second},
		"nonsense":   {"soon", 0},
		"negative":   {"-1m", 0},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HUBTASK_DB_CONNECT_WAIT", test.value)
			if got := connectWait(); got != test.want {
				t.Errorf("connectWait() = %s, want %s", got, test.want)
			}
		})
	}
}

// A port nothing listens on: the wait has to end at its deadline rather than at the process's,
// and a zero wait has to be exactly one attempt.
func TestWaitForDatabaseGivesUpAtItsDeadline(t *testing.T) {
	pool, err := sql.Open("pgx", "postgres://nobody@127.0.0.1:1/nothing?connect_timeout=1")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer func() { _ = pool.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	started := time.Now()
	if err := waitForDatabase(ctx, pool, 0); err == nil {
		t.Fatal("a closed port answered")
	}
	if time.Since(started) > 10*time.Second {
		t.Errorf("one attempt took %s", time.Since(started))
	}

	started = time.Now()
	if err := waitForDatabase(ctx, pool, 6*time.Second); err == nil {
		t.Fatal("a closed port answered")
	}
	if elapsed := time.Since(started); elapsed < 5*time.Second || elapsed > 20*time.Second {
		t.Errorf("the wait lasted %s, want about six seconds", elapsed)
	}
}
