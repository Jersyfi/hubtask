// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// PublicLinksKey is the workspace's switch letting a page be opened by a link without signing in
// (products.md M3: a public link only where the workspace allows it, lock-able per plan).
const PublicLinksKey setting.Key = "knowledge.public_links"

// PublicLinksDefinition is its declaration: off by default; the installation and a plan may set
// it and lock it - a plan that offers no public pages locks it off -, the workspace switches it
// freely inside that.
func PublicLinksDefinition() setting.Definition {
	return setting.Definition{
		Key: PublicLinksKey, Kind: setting.KindFlag,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayReplace, Default: false,
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "MANAGE_MEMBERS",
		},
	}
}

// PublicLinksOf reads the resolved switch; anything but true is off.
func PublicLinksOf(r setting.Resolution) bool {
	on, _ := r.Effective.(bool)
	return on
}

// PublicLink is a page opened read-only to whoever holds its link. One per page: a new link
// replaces the old one, which stops answering. The token itself is never kept, only its hash.
//
// It opens the page alone - not the pages under it, its comments or its history - and is read as
// the person who made it, evaluated at every opening: once they no longer read the page, the
// workspace switches public links off or the page is restricted, the link answers nothing.
type PublicLink struct {
	TenantID  shared.ID
	PageID    shared.ID
	CreatedBy shared.ID
	CreatedAt time.Time
}

// NewPublicLink checks a link: never to a page behind a restriction, since a link reaches
// everybody and a restriction names a few.
func NewPublicLink(tenant, page, by shared.ID, readers Readers, now time.Time) (PublicLink, error) {
	if tenant.IsZero() || page.IsZero() || by.IsZero() {
		return PublicLink{}, shared.ErrInternal.WithDetail("pages.identity_incomplete")
	}
	if readers.Restricted {
		return PublicLink{}, shared.ErrConflict.WithDetail("pages.public_link_restricted")
	}
	return PublicLink{TenantID: tenant, PageID: page, CreatedBy: by, CreatedAt: now}, nil
}
