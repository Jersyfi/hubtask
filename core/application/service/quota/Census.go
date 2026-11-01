// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ApproachingRatio is where a workspace counts as approaching a ceiling (A-18).
const ApproachingRatio = 0.9

// Band is how many workspaces stand at or above ApproachingRatio of one quota, and how many at
// its limit.
type Band struct {
	Approaching int64
	AtLimit     int64
}

// Census counts, for the scheduler leader, how many workspaces stand near or at each bounded
// quota, from the same standings a workspace reads (Standings). The guard's ratio is recorded on a
// create only, so it carries whichever workspace created last - and nothing for one that stopped
// at the wall; this reads every workspace on its own clock (A-18).
type Census struct {
	Tenants    TenantList
	Store      repository.Store
	Usage      repository.Usage
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Tenancy    env.TenancyMode
}

// TenantList enumerates the workspaces (adminrepo.Tenants.List).
type TenantList interface {
	List(ctx context.Context) ([]adminrepo.TenantRecord, error)
}

// Count answers a band per quota that carries a count, every one present even when zero, and
// the workspaces at or above the ratio of any quota, by identifier only.
func (c Census) Count(ctx context.Context) (map[string]Band, []shared.ID, error) {
	var tenants []adminrepo.TenantRecord
	err := c.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(), func(ctx context.Context) error {
		var err error
		tenants, err = c.Tenants.List(ctx)
		return err
	})
	if err != nil {
		return nil, nil, err
	}
	bands := map[string]Band{}
	var near []shared.ID
	now := c.Clock.Now()
	for _, tenant := range tenants {
		if tenant.Status != identity.TenantActive {
			continue
		}
		var standings []Standing
		err := c.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenant.ID}, func(ctx context.Context) error {
			var err error
			standings, err = Standings(ctx, c.Store, c.Usage, c.Tenancy, now)
			return err
		})
		if err != nil {
			return nil, nil, err
		}
		approaching := false
		for _, standing := range standings {
			if standing.Used == nil {
				continue
			}
			band := bands[standing.Quota]
			if ratio := standing.Ratio(); ratio != nil {
				if *ratio >= ApproachingRatio {
					band.Approaching++
					approaching = true
				}
				if *standing.Used >= standing.Limit {
					band.AtLimit++
				}
			}
			bands[standing.Quota] = band
		}
		if approaching {
			near = append(near, tenant.ID)
		}
	}
	return bands, near, nil
}
