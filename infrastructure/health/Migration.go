// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package health

import (
	"context"
	"log/slog"
	"math"
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
	// MigrationTooOld is a database past a contract step this binary predates: its compatibility
	// floor is above the binary's newest migration, so the binary would name a dropped column. It
	// is not ready and does not start (deployment.md §5).
	MigrationTooOld = "too_old"

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

// CompatibilityFloor reads the oldest binary, by its newest migration, that works on the schema.
type CompatibilityFloor func(ctx context.Context) (int64, error)

// SetCompatibilityFloor has the comparison also hold the binary to the schema's floor.
func (r *Registry) SetCompatibilityFloor(floor CompatibilityFloor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.floor = floor
	r.migrationAt = time.Time{}
}

// Unserviceable reports whether the schema state forbids this binary to serve: behind it, or past
// a contract step it predates.
func Unserviceable(state port.MigrationState) bool {
	return state.Status == MigrationBehind || state.Status == MigrationTooOld
}

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
	embedded, ledger, floorOf := r.embedded, r.ledger, r.floor
	cached, at := r.migrationState, r.migrationAt
	r.mu.RUnlock()
	if ledger == nil || len(embedded) == 0 {
		return port.MigrationState{}, false
	}
	if !at.IsZero() && !Unserviceable(cached) && r.now().Sub(at) < migrationKept {
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
	floor := int64(0)
	if floorOf != nil && !older {
		if floor, err = floorOf(ctx); err != nil {
			// Unread, the floor holds nothing back: the ledger's comparison still stands.
			slog.WarnContext(ctx, "the compatibility floor could not be read for readiness",
				slog.String("error", err.Error()))
			floor = 0
		}
	}
	state := compareMigrations(embedded, applied, older, floor)

	r.mu.Lock()
	r.migrationState, r.migrationAt = state, r.now()
	r.mu.Unlock()
	return state, true
}

// versionNumber narrows a migration version for the report. A version is a four-digit file number;
// one outside int32 is not a version this build could have written, and reads as none.
func versionNumber(version int64) int {
	if version < 0 || version > math.MaxInt32 {
		return 0
	}
	return int(version)
}

// compareMigrations is the rule: every embedded migration applied, or the binary is behind; and the
// binary's newest at least the schema's floor, or it is too old.
func compareMigrations(embedded, applied []int64, older bool, floor int64) port.MigrationState {
	expected := slices.Max(embedded)
	if older {
		return port.MigrationState{Applied: 0, Expected: versionNumber(expected), Status: MigrationBehind}
	}
	highest := int64(0)
	held := make(map[int64]bool, len(applied))
	for _, version := range applied {
		held[version] = true
		highest = max(highest, version)
	}
	state := port.MigrationState{Applied: versionNumber(highest), Expected: versionNumber(expected),
		CompatibleFrom: versionNumber(floor), Status: MigrationOK}
	if floor > expected {
		state.Status = MigrationTooOld
		return state
	}
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
