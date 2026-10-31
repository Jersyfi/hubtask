// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// DraftsKey switches a space from the live document to drafts and publishing (UC-DOC-15): its
// writers' changes wait in a draft until one of them publishes, and its readers read what was
// published. Off - the live document - by default; decided by each hub for its own space, which
// is where its writers and readers meet.
const DraftsKey setting.Key = "knowledge.drafts"

// DraftsDefinition is the setting's declaration.
func DraftsDefinition() setting.Definition {
	return setting.Definition{
		Key: DraftsKey, Kind: setting.KindFlag,
		Levels: []setting.Level{setting.LevelHub}, May: setting.MayReplace, Default: false,
		Writers: map[setting.Level]string{setting.LevelHub: "STRUCTURE"},
	}
}

// DraftsOf reads the resolved switch; anything but true is the live document.
func DraftsOf(r setting.Resolution) bool {
	on, _ := r.Effective.(bool)
	return on
}

// Draft is what waits to be published on a page of a space with drafts.
//
// Unpublished marks a page written in such a space and never published: it is restricted to its
// writer meanwhile, so its words are on the page itself and the draft holds none. Otherwise the
// draft holds the title and body its writers changed since the page was last published, and the
// page holds what readers read.
type Draft struct {
	TenantID    shared.ID
	PageID      shared.ID
	Unpublished bool
	Title       string
	Body        string
	UpdatedBy   shared.ID
	UpdatedAt   time.Time
}

// Pending reports whether the draft holds changes to a published page.
func (d Draft) Pending() bool { return !d.Unpublished }

// Drafted applies a change's title and body to a page's draft - the one it has, or the page as
// published - and reports whether anything differs from what is published. A change of anything
// else is the page's own and is not the draft's.
func Drafted(page Page, current *Draft, change Change, by shared.ID, now time.Time, norm text.Normalizer) (Draft, bool, error) {
	draft := Draft{TenantID: page.TenantID, PageID: page.ID, Title: page.Title, Body: page.Body}
	if current != nil && current.Pending() {
		draft.Title, draft.Body = current.Title, current.Body
	}
	if change.Title != nil {
		title, err := CheckTitle(*change.Title, norm)
		if err != nil {
			return Draft{}, false, err
		}
		draft.Title = title
	}
	if change.Body != nil {
		body, err := CheckBody(*change.Body, norm)
		if err != nil {
			return Draft{}, false, err
		}
		draft.Body = body
	}
	draft.UpdatedBy, draft.UpdatedAt = by, now
	return draft, draft.Title != page.Title || draft.Body != page.Body, nil
}

// Published is the page with its draft's title and body, as publishing writes it.
func (d Draft) Published(page Page, by shared.ID, now time.Time) Page {
	page.Title, page.Body = d.Title, d.Body
	page.UpdatedBy, page.UpdatedAt = by, now
	return page
}

// DraftOnly splits a change in a space with drafts: its title and body go to the draft, the rest
// to the page.
func DraftOnly(change Change) (draft, rest Change) {
	draft = Change{Title: change.Title, Body: change.Body}
	rest = change
	rest.Title, rest.Body = nil, nil
	return draft, rest
}

// Touches reports whether a change writes a title or a body.
func (c Change) Touches() bool { return c.Title != nil || c.Body != nil }
