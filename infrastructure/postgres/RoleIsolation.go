// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"log/slog"

	port "github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// IsolationBypassedCode is the report's warning for a database role row level security does not
// bind: every workspace's rows are then open to every query, which no gate in this process sees.
const IsolationBypassedCode = "database.isolation_bypassed"

// RoleIsolation is a warning source: whether the role this process connects as gets past row level
// security (multi-tenancy.md §2.1). Asked per report - a backfill's leftover policy comes and goes
// with the migrator - and silent when it cannot be asked, since the database probe says that.
type RoleIsolation struct {
	UnitOfWork persistence.UnitOfWork
}

// Warnings answers the warning, critical, naming each cause.
func (r RoleIsolation) Warnings(ctx context.Context) []port.Warning {
	var row sqlc.DatabaseRoleIsolationRow
	err := r.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		queries, err := queriesFrom(ctx)
		if err != nil {
			return err
		}
		row, err = queries.DatabaseRoleIsolation(ctx)
		return err
	})
	if err != nil {
		slog.WarnContext(ctx, "reading the database role's isolation failed", slog.String("error", err.Error()))
		return nil
	}
	return isolationWarnings(row)
}

// isolationWarnings is the rule: one warning per cause.
func isolationWarnings(row sqlc.DatabaseRoleIsolationRow) []port.Warning {
	var causes []string
	if row.Superuser != nil && *row.Superuser {
		causes = append(causes, "superuser")
	}
	if row.BypassRls != nil && *row.BypassRls {
		causes = append(causes, "bypass_rls")
	}
	if row.OwnsTables {
		causes = append(causes, "table_owner")
	}
	if row.BackfillPolicies > 0 {
		causes = append(causes, "backfill_policy")
	}
	warnings := make([]port.Warning, 0, len(causes))
	for _, cause := range causes {
		warnings = append(warnings, port.Warning{
			Code: IsolationBypassedCode, Severity: "critical", Params: map[string]string{"cause": cause},
		})
	}
	return warnings
}
