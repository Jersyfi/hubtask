// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strings"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// machineLabel is who the journal names for an operator restored from the machine.
const machineLabel = "machine"

// RestoreOperator is the way back into an installation nobody can run (UC-INS-03): run on the
// machine as `hubtask operator add --workspace <short name> --email <address>`, with the server's
// own configuration and database connection.
//
// No scope and no register check, and deliberately so: the door is access to the machine, which is
// what the person who could otherwise fix this with a database shell already has. That is also why
// it is not in the registry: nothing registered is reachable without a route, and this must have
// none (UC-INS-03 *Where it ends*).
type RestoreOperator struct{ Writer InstanceWriter }

// RestoredOperator says which account it was and whether it was added just now.
type RestoredOperator struct {
	AccountID shared.ID
	Added     bool
}

// Execute adds the account an address names in a workspace, and records it as done from the
// machine. An address that matches no account is the one refusal the screen gives too, which says
// nothing about other workspaces.
func (h RestoreOperator) Execute(ctx context.Context, slug, email string) (RestoredOperator, error) {
	w := h.Writer
	if strings.TrimSpace(slug) == "" || strings.TrimSpace(email) == "" {
		return RestoredOperator{}, shared.ErrValidation.WithDetail("admin.operator_incomplete")
	}

	var restored RestoredOperator
	err := w.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		found, err := w.Operators.Resolve(ctx, slug, email)
		if err != nil {
			return err
		}
		if found.IsZero() {
			return shared.ErrValidation.WithDetail("admin.operator_unknown_account")
		}
		restored.AccountID = found
		added, err := w.Operators.Add(ctx, found, shared.ID(""))
		if err != nil {
			return err
		}
		restored.Added = added
		if !added {
			// Already an operator: nothing changed, and nothing is recorded as if it had.
			return nil
		}
		return w.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: w.IDs.NewID(), OccurredAt: w.Clock.Now(), Action: journalOperatorAdded,
			ActorLabel: machineLabel,
			Details:    map[string]any{"account_id": found.String(), "door": machineLabel},
		})
	})
	return restored, err
}
