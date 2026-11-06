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
	// OwnDomains is the workspace's own domain (UC-INS-15); nil answers none.
	OwnDomains repository.OwnDomains
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
	domainsAllowed := false
	if multi && r.Settings != nil {
		err := unitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
			level, err := r.Settings.Read(ctx)
			rule = domain.OwnerAddressRule{Allowed: level.OwnersChangeAddress, CooldownDays: level.AddressCooldownDays}
			domainsAllowed = level.OwnersOwnDomains
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	var (
		owner, waiting bool
		out            = usecase.Output{
			"host": host, "owners_may_change": rule.Allowed, "may_change": false,
			"own_domains_allowed": domainsAllowed,
		}
	)
	err := unitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if multi && r.OwnDomains != nil {
			// Shown wherever the workspace has one, the switch off or not: a domain set up before
			// the operator switched it off still answers, and its owner may still remove it.
			own, err := ownDomainView(ctx, r.OwnDomains, r.InstallationHost)
			if err != nil {
				return err
			}
			if own != nil {
				out["own_domain"] = own
				// Beside a main own domain, the address row is the way back the screen offers.
				if main, _ := own["is_main"].(bool); main && own["state"] != string(repository.HostStateBlocked) {
					back, err := r.OwnDomains.AddressHost(ctx)
					if err != nil {
						return err
					}
					if back != "" {
						out["address_host"] = back
					}
				}
			}
		}
		last, err := r.Changes.LastAddressChange(ctx)
		if err != nil {
			return err
		}
		if !last.IsZero() {
			out["changed_at"] = last
		}
		if !multi {
			return nil
		}
		paths, err := r.Owners.OwnerPaths(ctx)
		owner = slices.ContainsFunc(paths, func(path repository.OwnerPath) bool {
			return path.AccountID == actor.AccountID
		})
		if !rule.Allowed {
			return err
		}
		if from := rule.ChangeableFrom(last, r.Clock.Now()); !from.IsZero() {
			out["changeable_from"] = from
			waiting = true
		}
		return err
	})
	if err != nil {
		return nil, err
	}
	out["may_manage_own_domain"] = owner
	out["may_change"] = owner && rule.Allowed && !waiting
	return out, nil
}
