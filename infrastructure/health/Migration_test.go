// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"errors"
	"testing"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

// ledgerAnswer is a ledger a test sets, and counts how often it was read.
type ledgerAnswer struct {
	applied []int64
	older   bool
	err     error
	reads   int
}

func (l *ledgerAnswer) read(context.Context) ([]int64, bool, error) {
	l.reads++
	return l.applied, l.older, l.err
}

func migratedRegistry(embedded []int64, ledger *ledgerAnswer) *Registry {
	registry := startedRegistry(&probe{name: "postgres", required: true, status: port.StatusOK})
	registry.SetMigration(embedded, ledger.read)
	return registry
}

// deployment.md §5: a binary serves only against a database holding every migration it embeds. A
// newer schema is the next release's expand step and is fine; an older one is not ready.
func TestReadinessComparesTheMigrationsWithTheBinarys(t *testing.T) {
	embedded := []int64{1, 2, 3, 5}
	cases := []struct {
		name      string
		ledger    ledgerAnswer
		ready     bool
		reason    string
		status    port.Status
		migration port.MigrationState
	}{
		{name: "every migration applied", ledger: ledgerAnswer{applied: []int64{0, 1, 2, 3, 5}},
			ready: true, status: port.StatusOK,
			migration: port.MigrationState{Applied: 5, Expected: 5, Status: MigrationOK}},
		{name: "a newer schema", ledger: ledgerAnswer{applied: []int64{1, 2, 3, 5, 6}},
			ready: true, status: port.StatusOK,
			migration: port.MigrationState{Applied: 6, Expected: 5, Status: MigrationAhead}},
		{name: "the binary's newest not yet run", ledger: ledgerAnswer{applied: []int64{1, 2, 3}},
			ready: false, reason: "migration:behind", status: port.StatusDown,
			migration: port.MigrationState{Applied: 3, Expected: 5, Status: MigrationBehind}},
		// Migrations apply out of order: the highest number is there, an older one is not.
		{name: "one in the middle missing", ledger: ledgerAnswer{applied: []int64{1, 2, 5, 6}},
			ready: false, reason: "migration:behind", status: port.StatusDown,
			migration: port.MigrationState{Applied: 6, Expected: 5, Status: MigrationBehind}},
		{name: "a database older than the reader", ledger: ledgerAnswer{older: true},
			ready: false, reason: "migration:behind", status: port.StatusDown,
			migration: port.MigrationState{Applied: 0, Expected: 5, Status: MigrationBehind}},
		// An unreadable ledger says nothing about the schema: the database probe decides.
		{name: "the ledger cannot be read", ledger: ledgerAnswer{err: errors.New("permission denied")},
			ready: true, status: port.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ledger := tc.ledger
			registry := migratedRegistry(embedded, &ledger)

			ready, reason := registry.Ready(t.Context())
			if ready != tc.ready || reason != tc.reason {
				t.Errorf("ready = %v %q, want %v %q", ready, reason, tc.ready, tc.reason)
			}
			report := registry.Report(t.Context())
			if report.Status != tc.status || report.Migration != tc.migration {
				t.Errorf("report %q %+v, want %q %+v", report.Status, report.Migration, tc.status, tc.migration)
			}
		})
	}
}

// Without a comparison - nothing embedded, or no ledger wired - the report says nothing about the
// migration, rather than a zero that reads like a fact.
func TestNoComparisonMeansNoMigrationInTheReport(t *testing.T) {
	ledger := &ledgerAnswer{applied: []int64{1}}
	for _, registry := range []*Registry{
		startedRegistry(&probe{name: "postgres", required: true, status: port.StatusOK}),
		migratedRegistry(nil, ledger),
	} {
		if ready, _ := registry.Ready(t.Context()); !ready {
			t.Error("not ready without a comparison")
		}
		if report := registry.Report(t.Context()); report.Migration != (port.MigrationState{}) {
			t.Errorf("a migration was reported without a comparison: %+v", report.Migration)
		}
	}
	if ledger.reads != 0 {
		t.Error("the ledger was read with nothing to compare it with")
	}
}

// Readiness is polled every few seconds. A state that is not behind is kept for a while; a behind
// one is read again every time, so the pod turns ready as soon as its migration has run.
func TestTheLedgerIsReadAgainOnlyWhileBehind(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	ledger := &ledgerAnswer{applied: []int64{1}}
	registry := migratedRegistry([]int64{1, 2}, ledger)
	registry.now = func() time.Time { return now }

	for range 3 {
		if ready, _ := registry.Ready(t.Context()); ready {
			t.Fatal("ready while behind")
		}
	}
	if ledger.reads != 3 {
		t.Errorf("a behind state was read %d times in 3 probes, want 3", ledger.reads)
	}

	ledger.applied = []int64{1, 2}
	if ready, _ := registry.Ready(t.Context()); !ready {
		t.Fatal("not ready once the migration ran")
	}
	for range 3 {
		registry.Ready(t.Context())
	}
	if ledger.reads != 4 {
		t.Errorf("a state that is ok was read %d times, want it kept", ledger.reads)
	}

	now = now.Add(migrationKept)
	registry.Ready(t.Context())
	if ledger.reads != 5 {
		t.Errorf("a kept state was not read again after %s", migrationKept)
	}
}
