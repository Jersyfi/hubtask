// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"log/slog"
	"strconv"
	"sync"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/concurrency"
)

const (
	// NotConfiguredCode is an installation with no enabled backup target (backup-restore.md §10).
	NotConfiguredCode = "config.backup_not_configured"
	// UnencryptedCode is an installation with enabled targets that store archives unencrypted.
	UnencryptedCode = "config.backup_unencrypted"
	// SingleTargetCode is an installation with exactly one enabled target - a pointer to 3-2-1.
	SingleTargetCode = "config.backup_single_target"
	// CoverageUncheckedCode says the count behind the three above could not be taken, or not yet:
	// never read as "a backup is configured" (P-11).
	CoverageUncheckedCode = "config.backup_coverage_unchecked"

	targetWarningsComponent = "backup-target-warnings"
	// TargetWarningsEvery is how old a count may be before a report asks for a new one. A target
	// is configured far less often than a report is read, and the count is one transaction per
	// workspace.
	TargetWarningsEvery = 15 * time.Minute
	// TargetWarningsTimeout bounds one count (rule 7).
	TargetWarningsTimeout = 30 * time.Second
)

// WorkspaceLister enumerates the workspaces the count visits.
type WorkspaceLister interface {
	List(ctx context.Context) ([]adminrepo.TenantRecord, error)
}

// TargetWarnings is the health report's `config.backup_*` source: whether the installation has a
// backup target at all, only one, or some that store unencrypted (backup-restore.md §10).
//
// A target is a workspace's row behind row level security and FORCE, so the count is taken the way
// the control plane touches every workspace - one bounded read each, as the application role - in
// the background, and served from the last count; a report must answer at once.
//
// Only in single-tenant operation: there the installation's backups are its workspaces' targets.
// In provider operation the installation is backed up by its operator at the database level
// (backup-restore.md §1, §8.6), and one workspace's target says nothing about another's.
type TargetWarnings struct {
	Tenancy    env.TenancyMode
	Workspaces WorkspaceLister
	Targets    repository.Targets
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock

	mu        sync.Mutex
	counted   bool
	failed    bool
	running   bool
	countedAt time.Time
	coverage  repository.Coverage
}

// Warnings is the report's source: the last count's warnings, and a new count started when the
// last one is older than TargetWarningsEvery.
func (w *TargetWarnings) Warnings(ctx context.Context) []health.Warning {
	if w.Tenancy != env.TenancySingle {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running && (!w.counted || w.Clock.Now().Sub(w.countedAt) >= TargetWarningsEvery) {
		w.startLocked(ctx)
	}
	if !w.counted {
		severity := "info"
		if w.failed {
			severity = "warn"
		}
		return []health.Warning{{Code: CoverageUncheckedCode, Severity: severity}}
	}
	warnings := coverageWarnings(w.coverage)
	if w.failed {
		warnings = append(warnings, health.Warning{Code: CoverageUncheckedCode, Severity: "warn"})
	}
	return warnings
}

// Refresh starts a count now, unless one is running. Called at start.
func (w *TargetWarnings) Refresh(ctx context.Context) {
	if w.Tenancy != env.TenancySingle {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.running {
		w.startLocked(ctx)
	}
}

// startLocked runs one count in the background, detached from the report that asked for it and
// bounded by its own deadline.
func (w *TargetWarnings) startLocked(ctx context.Context) {
	w.running = true
	concurrency.Go(context.WithoutCancel(ctx), targetWarningsComponent, func(ctx context.Context) {
		ctx, cancel := context.WithTimeout(ctx, TargetWarningsTimeout)
		defer cancel()
		coverage, err := w.count(ctx)
		at := w.Clock.Now()

		w.mu.Lock()
		defer w.mu.Unlock()
		w.running = false
		if err != nil {
			slog.WarnContext(ctx, "the backup targets could not be counted for the health report",
				slog.String("error", err.Error()))
			w.failed = true
			return
		}
		w.counted, w.failed, w.coverage, w.countedAt = true, false, coverage, at
	})
}

func (w *TargetWarnings) count(ctx context.Context) (repository.Coverage, error) {
	var workspaces []adminrepo.TenantRecord
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			listed, err := w.Workspaces.List(ctx)
			workspaces = listed
			return err
		})
	if err != nil {
		return repository.Coverage{}, err
	}
	var total repository.Coverage
	for _, workspace := range workspaces {
		err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: workspace.ID},
			func(ctx context.Context) error {
				coverage, err := w.Targets.Coverage(ctx)
				total.Configured += coverage.Configured
				total.Unencrypted += coverage.Unencrypted
				return err
			})
		if err != nil {
			return repository.Coverage{}, err
		}
	}
	return total, nil
}

// coverageWarnings turns a count into the codes it states. Counts only - never which workspace.
func coverageWarnings(coverage repository.Coverage) []health.Warning {
	var warnings []health.Warning
	switch coverage.Configured {
	case 0:
		warnings = append(warnings, health.Warning{Code: NotConfiguredCode, Severity: "warn"})
	case 1:
		warnings = append(warnings, health.Warning{Code: SingleTargetCode, Severity: "info"})
	}
	if coverage.Unencrypted > 0 {
		warnings = append(warnings, health.Warning{
			Code: UnencryptedCode, Severity: "warn",
			Params: map[string]string{"count": strconv.Itoa(coverage.Unencrypted)},
		})
	}
	return warnings
}
