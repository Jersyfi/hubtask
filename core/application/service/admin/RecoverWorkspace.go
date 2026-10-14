// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"strings"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// RecoverWorkspace is the rest of the way back into a locked installation (UC-INS-03): run on the
// machine as `hubtask workspace resume|cancel-deletion --workspace <short name>`, it makes the same
// transition as ResumeTenant and CancelTenantDeletion - the same guards, the same trail entry, the
// same journal entry - for a workspace whose every operator is locked out by the very state it is
// in. A single-mode installation's only workspace suspended, or set for deletion, before the server
// refused both is that case (multi-tenancy.md §5).
//
// The door is access to the machine, as for RestoreOperator: the actor it hands the two use cases
// holds `admin:tenants` for that one workspace and is recorded as the machine. No register check -
// the register's accounts are the ones the state locks out - and no route: it has no Descriptor,
// because a registered use case is reachable from outside, and this one mints its own actor.
type RecoverWorkspace struct {
	Tenants    adminrepo.Tenants
	UnitOfWork persistence.UnitOfWork
	Resume     ResumeTenant
	Cancel     CancelTenantDeletion
}

// ResumeBySlug reactivates the suspended workspace a short name names. An active one is left as it
// is; one pending deletion is refused as the API refuses it - the cancel comes first.
func (h RecoverWorkspace) ResumeBySlug(ctx context.Context, slug string) error {
	actor, err := h.machine(ctx, slug)
	if err != nil {
		return err
	}
	return h.Resume.Execute(ctx, actor, actor.TenantID)
}

// CancelDeletionBySlug calls off the pending deletion of the workspace a short name names, and
// answers the status the workspace is in afterwards - SUSPENDED where the request found it so.
func (h RecoverWorkspace) CancelDeletionBySlug(ctx context.Context, slug string) (domain.TenantStatus, error) {
	actor, err := h.machine(ctx, slug)
	if err != nil {
		return "", err
	}
	return h.Cancel.Execute(ctx, actor, actor.TenantID)
}

// machine resolves the short name through the installation's enumerator, in a read of its own that
// is closed before the workspace's transaction opens (a nested scope may not switch tenant), and
// answers the actor the machine acts as. Short names are stored in lower case, and `operator add`
// finds `Demo` as `demo`; so does this.
func (h RecoverWorkspace) machine(ctx context.Context, slug string) (appshared.ActorContext, error) {
	slug = strings.ToLower(strings.TrimSpace(slug))
	if slug == "" {
		return appshared.ActorContext{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	var found adminrepo.TenantRecord
	err := h.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		records, err := h.Tenants.List(ctx)
		if err != nil {
			return err
		}
		for _, record := range records {
			if record.Slug == slug {
				found = record
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return appshared.ActorContext{}, err
	}
	if found.ID.IsZero() {
		return appshared.ActorContext{}, shared.ErrNotFound.WithDetail("admin.tenant_not_found")
	}
	return appshared.ActorContext{
		Kind: appshared.ActorSystem, TenantID: found.ID, TenantSlug: found.Slug,
		AccountName: machineLabel, Scopes: []string{adminTenantsScope},
	}, nil
}
