// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"log/slog"
	"slices"
	"time"

	port "github.com/Jersyfi/hubtask/core/port/health"
)

const (
	// MigrationOK is a database holding every migration this binary embeds and none it does not.
	MigrationOK = "ok"
	// MigrationBehind is a database missing at least one of them: this binary may use a column
	// that is not there yet, so it is not ready (deployment.md §5).
	MigrationBehind = "behind"
	// MigrationAhead is a database migrated past this binary - the next release's expand step,
	// which an older binary runs against by design.
	MigrationAhead = "ahead"

	// migrationReadTimeout bounds one read of the ledger (rule 7).
	migrationReadTimeout = 2 * time.Second
	// migrationKept is how long a state that is not behind is reused. Readiness is polled every
	// few seconds; a schema only moves forward during a rollout, but a restore can move it back,
	// so never for longer. A behind state is read again on every probe, so the pod turns ready as
	// soon as its migration has run.
	migrationKept = 30 * time.Second
)

// MigrationLedger reads which migrations the database has applied. older is true when the
// database predates the reader itself - which is to say, it is behind any binary that has one.
// An error means the ledger could not be read, and says nothing about the schema.
type MigrationLedger func(ctx context.Context) (applied []int64, older bool, err error)

// SetMigration has readiness and the report compare the database's migrations with the ones this
// binary embeds. Without it, or with nothing embedded, neither compares anything.
func (r *Registry) SetMigration(embedded []int64, ledger MigrationLedger) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.embedded = slices.Clone(embedded)
	r.ledger = ledger
	r.migrationAt = time.Time{}
}

// Migration answers the current state, and false when there is none to tell: nothing is compared,
// or the ledger could not be read - the database probe speaks for the database then.
func (r *Registry) Migration(ctx context.Context) (port.MigrationState, bool) {
	r.mu.RLock()
	embedded, ledger := r.embedded, r.ledger
	cached, at := r.migrationState, r.migrationAt
	r.mu.RUnlock()
	if ledger == nil || len(embedded) == 0 {
		return port.MigrationState{}, false
	}
	if !at.IsZero() && cached.Status != MigrationBehind && r.now().Sub(at) < migrationKept {
		return cached, true
	}

	ctx, cancel := context.WithTimeout(ctx, migrationReadTimeout)
	defer cancel()
	applied, older, err := ledger(ctx)
	if err != nil {
		// A lost grant would otherwise switch the comparison off without a word.
		slog.WarnContext(ctx, "the migration ledger could not be read for readiness",
			slog.String("error", err.Error()))
		return port.MigrationState{}, false
	}
	state := compareMigrations(embedded, applied, older)

	r.mu.Lock()
	r.migrationState, r.migrationAt = state, r.now()
	r.mu.Unlock()
	return state, true
}

// compareMigrations is the rule: every embedded migration applied, or the binary is behind.
func compareMigrations(embedded, applied []int64, older bool) port.MigrationState {
	expected := slices.Max(embedded)
	if older {
		return port.MigrationState{Applied: 0, Expected: int(expected), Status: MigrationBehind}
	}
	highest := int64(0)
	held := make(map[int64]bool, len(applied))
	for _, version := range applied {
		held[version] = true
		highest = max(highest, version)
	}
	state := port.MigrationState{Applied: int(highest), Expected: int(expected), Status: MigrationOK}
	for _, version := range embedded {
		if !held[version] {
			state.Status = MigrationBehind
			return state
		}
	}
	if highest > expected {
		state.Status = MigrationAhead
	}
	return state
}
