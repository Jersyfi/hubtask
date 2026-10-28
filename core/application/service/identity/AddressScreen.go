// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"slices"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// WorkspaceAddressReader answers the workspace's main address and who may change it, for the
// workspace's *Address* screen (UC-INS-19 check 3): the screen draws *Change address* only where the
// reader may, and names the day where the installation's cooldown runs. The change itself decides
// again (admin.ChangeWorkspaceAddress); this answer only spares a control that would be refused.
type WorkspaceAddressReader struct {
	Tenancy env.TenancyMode
	// InstallationHost names the address of a workspace without a main row: the installation's
	// own host in single mode, the subdomain its short name makes in multi mode.
	InstallationHost string
	Settings         repository.InstanceSettings
	Changes          repository.AddressChanges
	Owners           repository.Owners
	Clock            clock.Clock
}

// addressOutput is the contract's WorkspaceAddress. Nil where the reader is not wired.
func (r WorkspaceAddressReader) addressOutput(
	ctx context.Context, unitOfWork persistence.UnitOfWork, actor appshared.ActorContext,
	slug string, hosts []domain.TenantHost,
) (usecase.Output, error) {
	if r.Changes == nil || r.Owners == nil || r.Clock == nil {
		return nil, nil
	}
	multi := r.Tenancy == env.TenancyMulti
	host := domain.CanonicalHostOf(slug, r.InstallationHost, multi)
	for _, held := range hosts {
		if held.Canonical {
			host = held.Host
		}
	}

	rule := domain.OwnerAddressRule{}
	if multi && r.Settings != nil {
		err := unitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			level, err := r.Settings.Read(ctx)
			rule = domain.OwnerAddressRule{Allowed: level.OwnersChangeAddress, CooldownDays: level.AddressCooldownDays}
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	var (
		owner bool
		out   = usecase.Output{"host": host, "owners_may_change": rule.Allowed, "may_change": false}
	)
	err := unitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		last, err := r.Changes.LastAddressChange(ctx)
		if err != nil {
			return err
		}
		if !last.IsZero() {
			out["changed_at"] = last
		}
		if !rule.Allowed {
			return nil
		}
		if from := rule.ChangeableFrom(last, r.Clock.Now()); !from.IsZero() {
			out["changeable_from"] = from
			return nil
		}
		paths, err := r.Owners.OwnerPaths(ctx)
		owner = slices.ContainsFunc(paths, func(path repository.OwnerPath) bool {
			return path.AccountID == actor.AccountID
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	out["may_change"] = owner
	return out, nil
}
