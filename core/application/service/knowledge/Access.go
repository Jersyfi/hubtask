// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	RestrictPageName       = "RestrictPage"
	ReadPageAccessName     = "ReadPageAccess"
	SharePageName          = "SharePage"
	UnsharePageName        = "UnsharePage"
	ListSharedPagesName    = "ListSharedPages"
	ReadPersonalSpaceName  = "ReadPersonalSpace"
	OpenDailyNoteName      = "OpenDailyNote"
	maxSharedPagesListed   = 200
	personalSpaceNameCode  = "pages.personal_space_name"
	PageRestrictedAction   = audit.Action("page.restricted")
	PageSharedAction       = audit.Action("page.shared")
	PageUnsharedAction     = audit.Action("page.unshared")
	shareTarget            = "page_share"
	restrictionChangeField = "restricted_to"
)

// PersonalHubs makes a private hub for the actor, its owner-member (ADR-0073 §2): the hub a
// personal space is. It is the work module's CreateContainer, so the hub is like any other.
type PersonalHubs interface {
	MakePrivateHub(ctx context.Context, actor appshared.ActorContext, name string) (shared.ID, error)
}

// admitted answers not found when a restriction keeps the actor from a page, inside the caller's
// transaction: a page one may not read is a page that is not there (rule 2, as a hub is).
func (s Space) admitted(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	readers, err := s.Pages.Readers(ctx, []shared.ID{id})
	if err != nil {
		return err
	}
	if !readers[id].Admits(actor.AccountID) {
		return shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return nil
}

// admit is admitted in a read-only transaction of its own.
func (s Space) admit(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	return s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return s.admitted(ctx, actor, id)
	})
}

// keepAdmitted leaves out the pages a restriction keeps from the actor, inside a transaction.
func (s Space) keepAdmitted(ctx context.Context, actor appshared.ActorContext, pages []domain.Page) ([]domain.Page, error) {
	if len(pages) == 0 {
		return pages, nil
	}
	ids := make([]shared.ID, 0, len(pages))
	for _, page := range pages {
		ids = append(ids, page.ID)
	}
	readers, err := s.Pages.Readers(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := pages[:0:0]
	for _, page := range pages {
		if readers[page.ID].Admits(actor.AccountID) {
			out = append(out, page)
		}
	}
	return out, nil
}

// sharedRead answers a page the actor reads by a share alone: shared with them, admitted by its
// restrictions, and in a hub they do not read. found false is every other case, which the hub
// decides as before.
func (s Space) sharedRead(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Page, bool, error) {
	var page domain.Page
	held := false
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if page, err = s.find(ctx, id, false); err != nil {
			return err
		}
		if held, err = s.Pages.SharedWith(ctx, page.ID, actor.AccountID); err != nil || !held {
			return err
		}
		return s.admitted(ctx, actor, page.ID)
	})
	if errors.Is(err, shared.ErrNotFound) {
		return domain.Page{}, false, nil
	}
	if err != nil || !held {
		return domain.Page{}, false, err
	}
	readsHub, err := s.Authorizer.Permits(ctx, actor, s.request(page.HubID, service.PermissionRead, PageReadAction, pagesRead, pageTarget, page.ID))
	if err != nil || readsHub {
		return domain.Page{}, false, err
	}
	return page, true, nil
}

// RestrictPageCommand is the input, typed.
type RestrictPageCommand struct {
	PageID shared.ID
	// AccountIDs are the people who may read the page; the caller is always among them.
	AccountIDs []shared.ID
	// Lift takes the page's own restriction away; AccountIDs are then ignored.
	Lift            bool
	ExpectedVersion int
}

// RestrictPage narrows who of a hub's readers may read a page and the pages under it, or lifts
// that again (UC-DOC-07). A writer the restriction admits sets it; somebody who manages the hub's
// members may lift one that shuts them out, so that no page is left that nobody can open.
type RestrictPage struct{ Space Space }

// Execute restricts or lifts.
func (h RestrictPage) Execute(ctx context.Context, actor appshared.ActorContext, cmd RestrictPageCommand) (domain.Page, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return domain.Page{}, err
	}
	var place domain.Page
	var readers domain.Readers
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if place, err = h.Space.find(ctx, cmd.PageID, false); err != nil {
			return err
		}
		all, err := h.Space.Pages.Readers(ctx, []shared.ID{place.ID})
		readers = all[place.ID]
		return err
	})
	if err != nil {
		return domain.Page{}, err
	}
	switch {
	case readers.Admits(actor.AccountID):
		err = h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageRestrictedAction, pageTarget)
	case cmd.Lift:
		err = h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
			PageRestrictedAction, pagesWrite, pageTarget, place.ID))
	default:
		err = shared.ErrNotFound.WithDetail("pages.not_found")
	}
	if err != nil {
		return domain.Page{}, err
	}
	var accounts []shared.ID
	if !cmd.Lift {
		if accounts, err = domain.CheckRestriction(cmd.AccountIDs, actor.AccountID); err != nil {
			return domain.Page{}, err
		}
	}
	now := h.Space.Clock.Now()
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		next := current
		next.Restricted, next.UpdatedBy, next.UpdatedAt = !cmd.Lift, actor.AccountID, now
		if err := h.Space.Pages.Restrict(ctx, next, accounts, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		if err := h.Space.announceSubtree(ctx, actor, next.ID, false); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageRestrictedAction, pageTarget, next.ID, []audit.Change{
			{Field: "restricted", Classification: audit.Open, To: next.Restricted},
			{Field: restrictionChangeField, Classification: audit.Open, To: len(accounts)},
		}, now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h RestrictPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RestrictPageName, Permission: service.PermissionWriteItems,
		Summary: "Restricts who of a hub's readers may read a page and the pages under it to the people " +
			"named - the caller always among them - or lifts the page's own restriction. A restriction " +
			"never widens: a page under several is read by the people every one names. Somebody who " +
			"manages the hub's members may lift a restriction that shuts them out.",
		SideEffects: "Writes the page's restriction and an audit entry; offline copies drop a restricted page.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "account_ids", Kind: usecase.KindIDList, Description: "The people who may read the page."},
			{Name: "lift", Kind: usecase.KindBool, Description: "Take the page's own restriction away."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		},
		Audit: usecase.AuditDeclaration{Action: PageRestrictedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			accounts, err := in.IDList("account_ids")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, RestrictPageCommand{
				PageID: id, AccountIDs: accounts, Lift: in.Bool("lift"), ExpectedVersion: in.Int("expected_version"),
			})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, false, h.Space.Clock.Now()), nil
		}),
	})
}

// Access is who reads a page beyond its hub: its own restriction, whether one above narrows it,
// the people it then admits, and the people it is shared with.
type Access struct {
	Page         domain.Page
	RestrictedTo []shared.ID
	Readers      domain.Readers
	Shares       []domain.Share
	MayShare     bool
}

// AccessOutput is the shape every door returns (api/openapi.yaml, schema PageAccess).
func AccessOutput(a Access) usecase.Output {
	shares := make([]usecase.Output, 0, len(a.Shares))
	for _, share := range a.Shares {
		shares = append(shares, usecase.Output{
			"account_id": share.AccountID.String(), "granted_by": idOrNil(share.GrantedBy), "granted_at": share.GrantedAt,
		})
	}
	var readers any
	if a.Readers.Restricted {
		readers = idStrings(a.Readers.Accounts)
	}
	return usecase.Output{
		"page_id": a.Page.ID.String(), "restricted": a.Page.Restricted, "restricted_to": idStrings(a.RestrictedTo),
		"restricted_above": a.Readers.Restricted && !a.Page.Restricted, "readers": readers,
		"shares": shares, "may_share": a.MayShare,
	}
}

func idStrings(ids []shared.ID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, id.String())
	}
	return out
}

// ReadPageAccess answers who reads a page beyond its hub, for a reader of the page.
type ReadPageAccess struct{ Space Space }

// Execute reads the access.
func (h ReadPageAccess) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (Access, error) {
	page, err := h.Space.readable(ctx, actor, id, PageReadAction)
	if err != nil {
		return Access{}, err
	}
	out := Access{Page: page}
	err = h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if out.RestrictedTo, err = h.Space.Pages.RestrictedTo(ctx, page.ID); err != nil {
			return err
		}
		readers, err := h.Space.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		out.Readers = readers[page.ID]
		out.Shares, err = h.Space.Pages.Shares(ctx, page.ID, domain.MaxSharesPerPage)
		return err
	})
	if err != nil {
		return Access{}, err
	}
	if actor.RequireScope(pagesWrite) == nil {
		out.MayShare, err = h.Space.Authorizer.Permits(ctx, actor, h.Space.request(page.HubID,
			service.PermissionManageMembers, PageSharedAction, pagesWrite, pageTarget, page.ID))
	}
	return out, err
}

// Descriptor is the catalogue entry.
func (h ReadPageAccess) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPageAccessName, Permission: service.PermissionRead,
		Summary: "Answers who reads a page beyond its hub: the people its own restriction names, whether " +
			"a page above restricts it too, the people it then admits, the people it is shared with, " +
			"and whether the caller may share it.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			access, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return AccessOutput(access), nil
		}),
	})
}

// SharePage shares a page read-only with one person beside its hub's memberships - a guest, a
// colleague outside the hub (UC-DOC-08). Who manages the hub's members shares, as they would grant
// a membership; a restricted page is shared only with somebody it admits.
type SharePage struct{ Space Space }

// Execute shares.
func (h SharePage) Execute(ctx context.Context, actor appshared.ActorContext, pageID, accountID shared.ID) (domain.Share, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return domain.Share{}, err
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return domain.Share{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PageSharedAction, pagesWrite, shareTarget, place.ID)); err != nil {
		return domain.Share{}, err
	}
	now := h.Space.Clock.Now()
	var share domain.Share
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		page, err := h.Space.find(ctx, pageID, true)
		if err != nil {
			return err
		}
		readers, err := h.Space.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		if share, err = domain.NewShare(actor.TenantID, page.ID, accountID, actor.AccountID, readers[page.ID], now); err != nil {
			return err
		}
		added, err := h.Space.Pages.InsertShare(ctx, share)
		if err != nil || !added {
			return err
		}
		return h.Space.record(ctx, actor, PageSharedAction, pageTarget, page.ID, []audit.Change{
			{Field: "account_id", Classification: audit.Open, To: accountID.String()},
		}, now)
	})
	return share, err
}

// Descriptor is the catalogue entry.
func (h SharePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SharePageName, Permission: service.PermissionManageMembers,
		Summary: "Shares a page read-only with one person of the workspace beside its hub's memberships " +
			"- a guest, or somebody outside the hub. The page alone opens, not the pages under it, its " +
			"comments or its history. A restricted page is shared only with somebody it admits.",
		SideEffects: "Writes the share and an audit entry. Sharing twice changes nothing.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "account_id", Kind: usecase.KindID, Required: true, Description: "The person it is shared with."},
		},
		Audit: usecase.AuditDeclaration{Action: PageSharedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			ids := map[string]shared.ID{}
			for _, field := range []string{"page_id", "account_id"} {
				id, err := in.ID(field)
				if err != nil {
					return nil, err
				}
				ids[field] = id
			}
			share, err := h.Execute(ctx, actor, ids["page_id"], ids["account_id"])
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"page_id": share.PageID.String(), "account_id": share.AccountID.String(),
				"granted_by": idOrNil(share.GrantedBy), "granted_at": share.GrantedAt,
			}, nil
		}),
	})
}

// UnsharePage takes a share away: by somebody who manages the hub's members, or by the person it
// was shared with, who may leave it.
type UnsharePage struct{ Space Space }

// Execute takes the share away.
func (h UnsharePage) Execute(ctx context.Context, actor appshared.ActorContext, pageID, accountID shared.ID) error {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return err
	}
	var place domain.Page
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		place, err = h.Space.find(ctx, pageID, false)
		return err
	})
	if err != nil {
		return err
	}
	if accountID != actor.AccountID {
		if err := h.Space.admit(ctx, actor, place.ID); err != nil {
			return err
		}
		if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
			PageUnsharedAction, pagesWrite, shareTarget, place.ID)); err != nil {
			return err
		}
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Space.Pages.DeleteShare(ctx, place.ID, accountID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("pages.share_not_found")
		}
		return h.Space.record(ctx, actor, PageUnsharedAction, pageTarget, place.ID, []audit.Change{
			{Field: "account_id", Classification: audit.Open, To: accountID.String()},
		}, now)
	})
}

// Descriptor is the catalogue entry.
func (h UnsharePage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: UnsharePageName, Permission: service.PermissionManageMembers,
		Summary: "Takes a page's share with one person away. Somebody who manages the hub's members " +
			"may take any; the person it was shared with may leave their own.",
		SideEffects: "Removes the share and writes an audit entry.",
		TokenScope:  pagesWrite,
		Destructive: true,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "account_id", Kind: usecase.KindID, Required: true, Description: "The person whose share goes."},
		},
		Audit: usecase.AuditDeclaration{Action: PageUnsharedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			page, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			account, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, page, account)
		}),
	})
}

// ListSharedPages answers the pages shared with the caller, the list a guest starts from.
type ListSharedPages struct{ Space Space }

// Execute lists them.
func (h ListSharedPages) Execute(ctx context.Context, actor appshared.ActorContext) ([]repository.Summary, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return nil, err
	}
	var listed []repository.Summary
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		listed, err = h.Space.Pages.PagesSharedWith(ctx, actor.AccountID, maxSharedPagesListed)
		return err
	})
	out := listed[:0:0]
	for _, summary := range listed {
		if summary.Readers.Admits(actor.AccountID) {
			out = append(out, summary)
		}
	}
	return out, err
}

// Descriptor is the catalogue entry.
func (h ListSharedPages) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListSharedPagesName,
		Summary: "Answers the pages shared with the caller, newest share first: each one's identifier, " +
			"title and icon.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(listed))
			for _, summary := range listed {
				data = append(data, usecase.Output{
					"id": summary.ID.String(), "hub_id": summary.HubID.String(), "title": summary.Title,
					"icon": nilIfEmpty(summary.Icon),
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	})
}

// personalSpace answers the caller's own notes' hub, making it first when asked to.
func (s Space) personalSpace(ctx context.Context, actor appshared.ActorContext, create bool) (shared.ID, error) {
	var hub shared.ID
	found := false
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		hub, found, err = s.Pages.PersonalSpace(ctx, actor.AccountID)
		return err
	})
	if err != nil || found || !create {
		return hub, err
	}
	if s.PersonalHubs == nil || s.Words == nil {
		return "", shared.ErrConflict.WithDetail("pages.personal_space_unavailable")
	}
	made, err := s.PersonalHubs.MakePrivateHub(ctx, actor, s.Words.Render(actor.Locale, personalSpaceNameCode, nil))
	if err != nil {
		return "", err
	}
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		hub, err = s.Pages.ClaimPersonalSpace(ctx, actor.AccountID, made, s.Clock.Now())
		return err
	})
	return hub, err
}

// ReadPersonalSpace answers the hub that is the caller's own notes, or none yet.
type ReadPersonalSpace struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReadPersonalSpace) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPersonalSpaceName,
		Summary: "Answers the private hub that is the caller's own notes, or none when it has not been " +
			"made yet; it is made with the first daily note.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: PageReadAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := actor.RequireScope(pagesRead); err != nil {
				return nil, err
			}
			hub, err := h.Space.personalSpace(ctx, actor, false)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"hub_id": idOrNil(hub)}, nil
		}),
	})
}

// OpenDailyNote opens the caller's note of a day in their personal space - today in their time
// zone unless a day is named - making the space and the note on first use (UC-DOC-07).
type OpenDailyNote struct{ Space Space }

// Execute opens the note.
func (h OpenDailyNote) Execute(ctx context.Context, actor appshared.ActorContext, rawDay string) (domain.Page, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return domain.Page{}, err
	}
	zone, err := time.LoadLocation(actor.TimeZone)
	if err != nil || actor.TimeZone == "" {
		zone = time.UTC
	}
	now := h.Space.Clock.Now()
	day, err := domain.DailyNoteDay(rawDay, now, zone)
	if err != nil {
		return domain.Page{}, err
	}
	hub, err := h.Space.personalSpace(ctx, actor, true)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, hub, hub, PageCreatedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		id, found, err := h.Space.Pages.DailyNote(ctx, hub, day)
		if err != nil {
			return err
		}
		if found {
			page, err = h.Space.find(ctx, id, false)
			return err
		}
		last, err := h.Space.Pages.LastOrderKey(ctx, hub, "")
		if err != nil {
			return err
		}
		key, err := service.OrderKeyAfter(last)
		if err != nil {
			return err
		}
		if page, err = domain.NewPage(domain.NewPageInput{
			ID: h.Space.IDs.NewID(), TenantID: actor.TenantID, HubID: hub, Title: domain.DailyNoteTitle(day),
			OrderKey: key, CreatedBy: actor.AccountID, Now: now, Text: h.Space.Text,
		}); err != nil {
			return err
		}
		page.DailyFor = &day
		if err := h.Space.Pages.Insert(ctx, page); err != nil {
			return err
		}
		if err := h.Space.snapshot(ctx, page, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Space.announce(ctx, actor, page); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageCreatedAction, pageTarget, page.ID,
			changed([]string{domain.FieldTitle}, page), now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h OpenDailyNote) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: OpenDailyNoteName,
		Summary: "Opens the caller's note of a day in their personal space - a private hub only they " +
			"read - today in their time zone unless a day is named. The space and the note are made on " +
			"first use; the note is titled with its date.",
		SideEffects: "May create the personal space (a private hub) and the note, with an audit entry each.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "day", Kind: usecase.KindString, Description: "The day, YYYY-MM-DD; empty for today."},
		},
		Audit: usecase.AuditDeclaration{Action: PageCreatedAction, TargetType: pageTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			page, err := h.Execute(ctx, actor, in.String("day"))
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}
