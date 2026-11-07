// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PublicRoadmapsKey is the workspace's switch letting a portfolio's roadmap be opened by a link
// without signing in (UC-GOL-03 check 15; the owner's fourth answers, item 9).
const PublicRoadmapsKey setting.Key = "goals.public_roadmaps"

// PublicRoadmapsDefinition is its declaration, a page's public links' again: off by default; the
// installation and a plan may set and lock it, the workspace switches it inside that.
func PublicRoadmapsDefinition() setting.Definition {
	return setting.Definition{
		Key: PublicRoadmapsKey, Kind: setting.KindFlag,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayReplace, Default: false,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
	}
}

// PublicRoadmapsOf reads the resolved switch; anything but true is off.
func PublicRoadmapsOf(r setting.Resolution) bool {
	on, _ := r.Effective.(bool)
	return on
}

// RoadmapLink is a portfolio's roadmap opened read-only to whoever holds its link. One per
// portfolio: a new link replaces the old one, which stops answering; only the token's hash is kept.
//
// It is read as the person who made it, evaluated at every opening: the roadmap shows only what
// they may read now, and once they may no longer read the workspace, or the workspace switches
// public roadmaps off, the link answers nothing.
type RoadmapLink struct {
	TenantID    shared.ID
	PortfolioID shared.ID
	CreatedBy   shared.ID
	CreatedAt   time.Time
}

// NewRoadmapLink checks a link's identity.
func NewRoadmapLink(tenant, portfolio, by shared.ID, now time.Time) (RoadmapLink, error) {
	if tenant.IsZero() || portfolio.IsZero() || by.IsZero() {
		return RoadmapLink{}, shared.ErrInternal.WithDetail("portfolios.identity_incomplete")
	}
	return RoadmapLink{TenantID: tenant, PortfolioID: portfolio, CreatedBy: by, CreatedAt: now}, nil
}
