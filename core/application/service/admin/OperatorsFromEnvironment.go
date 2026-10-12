// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"log/slog"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/health"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// operatorsVariable is who the journal names for an entry the environment put in.
const operatorsVariable = "HUBTASK_OPERATORS"

// RestoreCommand is the command the health report names for an installation nobody can run
// (UC-INS-03 check 4). Not a sentence: the catalogue says what to do with it.
const RestoreCommand = "hubtask operator add --workspace <short name> --email <address>"

// SingleOperatorResolver finds an address in a single-mode installation's one workspace.
type SingleOperatorResolver interface {
	ResolveSingle(ctx context.Context, email string) (shared.ID, error)
}

// RegisterState answers how many workspaces and operators the installation has.
type RegisterState interface {
	State(ctx context.Context) (workspaces, operators int64, err error)
}

// OperatorsFromEnvironment is HUBTASK_OPERATORS (UC-INS-02 checks 4, 5): every entry whose account
// exists is put into the register, at start and again as accounts come to exist; an entry whose
// account does not exist yet waits, and the health report names it.
//
// The environment adds and never removes: an operator added through the screen stays when the
// variable no longer names them, and one the variable names stays when somebody removes it from
// the variable. Authorisation is the configuration itself - whoever sets the server's environment
// runs the installation - so nothing here asks for a scope, and nothing here is on a route.
type OperatorsFromEnvironment struct {
	Entries    []env.OperatorEntry
	Operators  identityrepo.Operators
	Single     SingleOperatorResolver
	Journal    adminrepo.Journal
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// Apply adds every entry whose account exists, records each one that was not in the register yet,
// and answers the entries still waiting for their account.
func (o OperatorsFromEnvironment) Apply(ctx context.Context) ([]env.OperatorEntry, error) {
	if len(o.Entries) == 0 {
		return nil, nil
	}
	var waiting []env.OperatorEntry
	err := o.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		waiting = nil
		now := o.Clock.Now()
		for _, entry := range o.Entries {
			accountID, err := o.resolve(ctx, entry)
			if err != nil {
				return err
			}
			if accountID.IsZero() {
				waiting = append(waiting, entry)
				continue
			}
			added, err := o.Operators.Add(ctx, accountID, shared.ID(""))
			if err != nil {
				return err
			}
			if !added {
				continue
			}
			if err := o.Journal.Record(ctx, adminrepo.InstanceEvent{
				ID: o.IDs.NewID(), OccurredAt: now, Action: journalOperatorAdded,
				ActorLabel: operatorsVariable,
				Details:    map[string]any{"account_id": accountID.String()},
			}); err != nil {
				return err
			}
			slog.InfoContext(ctx, "an operator named in the environment was added to the register")
		}
		return nil
	})
	return waiting, err
}

// Pending answers, without writing, the entries whose account does not exist yet.
func (o OperatorsFromEnvironment) Pending(ctx context.Context) ([]env.OperatorEntry, error) {
	var waiting []env.OperatorEntry
	err := o.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		waiting = nil
		for _, entry := range o.Entries {
			accountID, err := o.resolve(ctx, entry)
			if err != nil {
				return err
			}
			if accountID.IsZero() {
				waiting = append(waiting, entry)
			}
		}
		return nil
	})
	return waiting, err
}

func (o OperatorsFromEnvironment) resolve(ctx context.Context, entry env.OperatorEntry) (shared.ID, error) {
	if entry.Workspace == "" {
		if o.Single == nil {
			return shared.ID(""), nil
		}
		return o.Single.ResolveSingle(ctx, entry.Email)
	}
	return o.Operators.Resolve(ctx, entry.Workspace, entry.Email)
}

// healthReadTimeout bounds the report's database read (rule 7): the report is read by a person
// waiting for it and by the sampler, and neither should wait on a slow register.
const healthReadTimeout = 2 * time.Second

// RegisterWarnings is the health report's view of who runs the installation (UC-INS-02 check 4,
// UC-INS-03 check 4), read live for each report:
//
//   - an installation with more than one workspace and nobody in the register - counted as
//     `is_operator` counts, so exactly where nobody can reach the control plane - names the
//     command that restores an operator from the machine;
//   - each HUBTASK_OPERATORS entry whose account does not exist yet;
//   - and a register that could not be read says so, rather than reading as "fine" (P-11).
type RegisterWarnings struct {
	State       RegisterState
	Environment OperatorsFromEnvironment
	UnitOfWork  persistence.UnitOfWork
}

// Warnings answers the warnings that hold now.
func (r RegisterWarnings) Warnings(ctx context.Context) []health.Warning {
	ctx, cancel := context.WithTimeout(ctx, healthReadTimeout)
	defer cancel()

	var workspaces, operators int64
	err := r.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		workspaces, operators, err = r.State.State(ctx)
		return err
	})
	if err != nil {
		return []health.Warning{{Code: "config.operator_register_unchecked", Severity: "warn"}}
	}

	var warnings []health.Warning
	if workspaces > 1 && operators == 0 {
		warnings = append(warnings, health.Warning{
			Code: "config.operator_register_empty", Severity: "warn",
			Params: map[string]string{"command": RestoreCommand},
		})
	}
	pending, err := r.Environment.Pending(ctx)
	if err != nil {
		return append(warnings, health.Warning{Code: "config.operator_register_unchecked", Severity: "warn"})
	}
	for _, entry := range pending {
		warnings = append(warnings, health.Warning{
			Code: "config.operator_pending", Severity: "info",
			Params: map[string]string{"entry": entry.String()},
		})
	}
	return warnings
}
