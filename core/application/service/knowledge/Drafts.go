// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"strconv"
	"time"

	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names of a space with drafts (UC-DOC-15).
const (
	PublishPageName      = "PublishPage"
	DiscardPageDraftName = "DiscardPageDraft"
	ReadSpaceDraftsName  = "ReadSpaceDrafts"
	SetSpaceDraftsName   = "SetSpaceDrafts"
)

// The stable audit codes of drafts and publishing (audit.md §2).
const (
	PagePublishedAction      audit.Action = "page.published"
	PageDraftDiscardedAction audit.Action = "page.draft_discarded"
	SpaceDraftsReadAction    audit.Action = "page_space.drafts_read"
	SpaceDraftsSetAction     audit.Action = "page_space.drafts_set"
)

const spaceTarget = "page_space"

// The fields a draft's save names in the trail, apart from the page's own.
const (
	draftTitleField = "draft.title"
	draftBodyField  = "draft.body"
)

// draftsWalk is where a space's switch resolves through: the workspace, then the hub.
func draftsWalk(tenant, hubID shared.ID) []settingrepo.Place {
	return []settingrepo.Place{
		{Level: setting.LevelWorkspace, ScopeID: tenant},
		{Level: setting.LevelHub, ScopeID: hubID},
	}
}

// draftsResolved resolves a space's switch inside the caller's transaction over the walk given;
// without a store every space is a live document.
func (s Space) draftsResolved(ctx context.Context, walk []settingrepo.Place) (setting.Resolution, error) {
	definition := domain.DraftsDefinition()
	if s.Drafting.Drafts == nil || s.Drafting.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := s.Drafting.Settings.Above(ctx, string(domain.DraftsKey))
	if err != nil {
		return setting.Resolution{}, err
	}
	stored, err := s.Drafting.Settings.At(ctx, walk, string(domain.DraftsKey))
	if err != nil {
		return setting.Resolution{}, err
	}
	return setting.Resolve(definition, append([]setting.Layer{instance, plan}, settingrepo.LayersOf(walk, stored)...)...), nil
}

// drafting reports whether a hub's space keeps drafts.
func (s Space) drafting(ctx context.Context, tenant, hubID shared.ID) (bool, error) {
	r, err := s.draftsResolved(ctx, draftsWalk(tenant, hubID))
	return domain.DraftsOf(r), err
}

// draftOf answers a page's draft, if the space keeps drafts.
func (s Space) draftOf(ctx context.Context, pageID shared.ID) (*domain.Draft, error) {
	if s.Drafting.Drafts == nil {
		return nil, nil
	}
	draft, found, err := s.Drafting.Drafts.Find(ctx, pageID)
	if err != nil || !found {
		return nil, err
	}
	return &draft, nil
}

// markUnpublished keeps a page just written in a space with drafts to its writer until it is
// published: a restriction naming them, so that every way a page is read - the tree, search,
// backlinks, the offline copy, a link - already leaves it out for everybody else.
func (s Space) markUnpublished(ctx context.Context, actor appshared.ActorContext, page domain.Page, now time.Time) (domain.Page, error) {
	next := page
	next.Restricted = true
	if err := s.Pages.Restrict(ctx, next, []shared.ID{actor.AccountID}, page.Version); err != nil {
		return domain.Page{}, err
	}
	next.Version = page.Version + 1
	err := s.Drafting.Drafts.Put(ctx, domain.Draft{
		TenantID: actor.TenantID, PageID: page.ID, Unpublished: true, UpdatedBy: actor.AccountID, UpdatedAt: now,
	})
	return next, err
}

// saveDraft writes a change's title and body to the page's draft instead of the page; a draft
// back at what is published is removed. It answers the draft that now waits, if any.
func (s Space) saveDraft(
	ctx context.Context, actor appshared.ActorContext, page domain.Page, current *domain.Draft, change domain.Change,
) (*domain.Draft, error) {
	now := s.Clock.Now()
	draft, differs, err := domain.Drafted(page, current, change, actor.AccountID, now, s.Text)
	if err != nil {
		return nil, err
	}
	if !differs {
		if current != nil {
			if _, err := s.Drafting.Drafts.Delete(ctx, page.ID); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}
	if current != nil && current.Title == draft.Title && current.Body == draft.Body {
		return current, nil
	}
	if err := s.Drafting.Drafts.Put(ctx, draft); err != nil {
		return nil, err
	}
	var changes []audit.Change
	if current == nil || current.Title != draft.Title {
		changes = append(changes, audit.Change{Field: draftTitleField, Classification: audit.Sensitive, To: draft.Title})
	}
	if current == nil || current.Body != draft.Body {
		changes = append(changes, audit.Change{Field: draftBodyField, Classification: audit.Sensitive, To: draft.Body})
	}
	return &draft, s.record(ctx, actor, PageChangedAction, pageTarget, page.ID, changes, now)
}

// DraftOutput is a draft as the doors answer it (schema PageDraft); nil is no draft.
func DraftOutput(d *domain.Draft) any {
	if d == nil || !d.Pending() {
		return nil
	}
	return usecase.Output{
		"title": d.Title, "body": d.Body, "updated_by": d.UpdatedBy.String(), "updated_at": d.UpdatedAt,
	}
}

// PublishPage publishes what waits on a page (UC-DOC-15): a page never published opens to its
// space, the restriction that kept it to its writer lifted; a draft's title and body become the
// page's, kept as a version of its history. Who may write the page publishes it.
type PublishPage struct{ Space Space }

// PublishPageCommand is the input, typed.
type PublishPageCommand struct {
	PageID          shared.ID
	ExpectedVersion int
}

// Execute publishes.
func (h PublishPage) Execute(ctx context.Context, actor appshared.ActorContext, cmd PublishPageCommand) (domain.Page, error) {
	place, err := h.Space.placeOf(ctx, actor, cmd.PageID)
	if err != nil {
		return domain.Page{}, err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PagePublishedAction, pageTarget); err != nil {
		return domain.Page{}, err
	}
	if h.Space.Drafting.Drafts == nil {
		return domain.Page{}, shared.ErrConflict.WithDetail("pages.nothing_to_publish")
	}
	var page domain.Page
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		current, err := h.Space.find(ctx, cmd.PageID, true)
		if err != nil {
			return err
		}
		if cmd.ExpectedVersion != 0 && cmd.ExpectedVersion != current.Version {
			return shared.ErrVersionConflict.WithDetail("pages.version_conflict")
		}
		draft, err := h.Space.draftOf(ctx, current.ID)
		if err != nil {
			return err
		}
		if draft == nil {
			return shared.ErrConflict.WithDetail("pages.nothing_to_publish")
		}
		now := h.Space.Clock.Now()
		if _, err := h.Space.Drafting.Drafts.Delete(ctx, current.ID); err != nil {
			return err
		}
		if draft.Unpublished {
			next := current
			next.Restricted, next.UpdatedBy, next.UpdatedAt = false, actor.AccountID, now
			if err := h.Space.Pages.Restrict(ctx, next, nil, current.Version); err != nil {
				return err
			}
			next.Version = current.Version + 1
			page = next
			if err := h.Space.announceSubtree(ctx, actor, next.ID, false); err != nil {
				return err
			}
			return h.Space.record(ctx, actor, PagePublishedAction, pageTarget, next.ID, []audit.Change{
				{Field: "restricted", Classification: audit.Open, To: false},
			}, now)
		}
		next := draft.Published(current, actor.AccountID, now)
		if err := h.Space.Pages.Update(ctx, next, current.Version); err != nil {
			return err
		}
		next.Version = current.Version + 1
		page = next
		if err := h.Space.snapshot(ctx, next, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Space.announce(ctx, actor, next); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PagePublishedAction, pageTarget, next.ID,
			changed([]string{domain.FieldTitle, domain.FieldBody}, next), now)
	})
	return page, err
}

// Descriptor is the catalogue entry.
func (h PublishPage) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: PublishPageName, Permission: service.PermissionWriteItems,
		Summary: "Publishes what waits on a page of a space with drafts: a page never published opens to " +
			"the space's readers, and a draft's title and body become the page's, kept as a version.",
		SideEffects: "Writes the page, a version and an audit entry, and removes the draft.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version the caller last read; a newer one is a conflict."},
		},
		Audit: usecase.AuditDeclaration{Action: PagePublishedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			page, err := h.Execute(ctx, actor, PublishPageCommand{PageID: id, ExpectedVersion: in.Int("expected_version")})
			if err != nil {
				return nil, err
			}
			return PageOutput(page, true, h.Space.Clock.Now()), nil
		}),
	})
}

// DiscardPageDraft throws a page's draft away, so the page stays as published. A page never
// published has no draft to discard: it is trashed instead.
type DiscardPageDraft struct{ Space Space }

// Execute discards.
func (h DiscardPageDraft) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) error {
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.authorizeWrite(ctx, actor, place.HubID, place.ID, PageDraftDiscardedAction, pageTarget); err != nil {
		return err
	}
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		draft, err := h.Space.draftOf(ctx, pageID)
		if err != nil {
			return err
		}
		if draft == nil || draft.Unpublished {
			return shared.ErrConflict.WithDetail("pages.nothing_to_discard")
		}
		if _, err := h.Space.Drafting.Drafts.Delete(ctx, pageID); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PageDraftDiscardedAction, pageTarget, pageID, nil, h.Space.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h DiscardPageDraft) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DiscardPageDraftName, Permission: service.PermissionWriteItems,
		Summary: "Throws away the draft waiting on a page of a space with drafts; the page stays as " +
			"published. A page never published is trashed instead.",
		SideEffects: "Removes the draft and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PageDraftDiscardedAction, TargetType: pageTarget, Severity: audit.SeverityInfo, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, id)
		}),
	})
}

// SpaceDrafts is a space's switch with where it comes from and how many drafts wait.
type SpaceDrafts struct {
	Resolution setting.Resolution
	Pending    int
}

// Output is the shape every door answers (schema PageSpaceDrafts).
func (v SpaceDrafts) Output() usecase.Output {
	out := usecase.Output{"drafts": domain.DraftsOf(v.Resolution), "source": string(v.Resolution.Source), "pending": v.Pending}
	if v.Resolution.Locked() {
		out["locked_by"] = string(v.Resolution.LockedBy)
	}
	return out
}

func (s Space) spaceDrafts(ctx context.Context, tenant, hubID shared.ID) (SpaceDrafts, error) {
	r, err := s.draftsResolved(ctx, draftsWalk(tenant, hubID))
	if err != nil || s.Drafting.Drafts == nil {
		return SpaceDrafts{Resolution: r}, err
	}
	pending, err := s.Drafting.Drafts.Pending(ctx, hubID)
	return SpaceDrafts{Resolution: r, Pending: pending}, err
}

// ReadSpaceDrafts answers whether a hub's space keeps drafts, the level that decided it and how
// many drafts wait. Whoever reads the hub may know how its pages are written.
type ReadSpaceDrafts struct{ Space Space }

// Execute reads.
func (h ReadSpaceDrafts) Execute(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (SpaceDrafts, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return SpaceDrafts{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(hubID, service.PermissionRead,
		SpaceDraftsReadAction, pagesRead, spaceTarget, hubID)); err != nil {
		return SpaceDrafts{}, err
	}
	var view SpaceDrafts
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, hubID); err != nil {
			return err
		}
		var err error
		view, err = h.Space.spaceDrafts(ctx, actor.TenantID, hubID)
		return err
	})
	return view, err
}

// Descriptor is the catalogue entry.
func (h ReadSpaceDrafts) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadSpaceDraftsName, Permission: service.PermissionRead,
		Summary: "Answers whether a hub's space keeps its writers' changes in drafts until they are " +
			"published, the level that decided it, the level that locked it and how many drafts wait.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose space is read."}},
		Audit: usecase.AuditDeclaration{Action: SpaceDraftsReadAction, TargetType: spaceTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	})
}

// SetSpaceDrafts switches a hub's space between the live document and drafts with publishing,
// inside what the workspace leaves. Back to the live document it waits until no draft does: a
// draft left behind would be words nobody sees and the next write would pass by.
type SetSpaceDrafts struct{ Space Space }

// SetSpaceDraftsCommand is the input, typed.
type SetSpaceDraftsCommand struct {
	HubID  shared.ID
	Drafts bool
}

// Execute switches.
func (h SetSpaceDrafts) Execute(ctx context.Context, actor appshared.ActorContext, cmd SetSpaceDraftsCommand) (SpaceDrafts, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return SpaceDrafts{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(cmd.HubID, service.PermissionStructure,
		SpaceDraftsSetAction, pagesWrite, spaceTarget, cmd.HubID)); err != nil {
		return SpaceDrafts{}, err
	}
	if h.Space.Drafting.Drafts == nil || h.Space.Drafting.Settings == nil {
		return SpaceDrafts{}, shared.ErrUnavailable.WithDetail("pages.drafts_unavailable")
	}
	definition := domain.DraftsDefinition()
	now := h.Space.Clock.Now()
	var view SpaceDrafts
	err := h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if _, err := h.Space.hub(ctx, cmd.HubID); err != nil {
			return err
		}
		walk := draftsWalk(actor.TenantID, cmd.HubID)
		above, err := h.Space.draftsResolved(ctx, walk[:1])
		if err != nil {
			return err
		}
		entry := setting.Entry{Value: cmd.Drafts, Hold: setting.HoldSet, By: actor.AccountID}
		if err := setting.Check(definition, setting.LevelHub, above, entry); err != nil {
			return err
		}
		if !cmd.Drafts {
			pending, err := h.Space.Drafting.Drafts.Pending(ctx, cmd.HubID)
			if err != nil {
				return err
			}
			if pending > 0 {
				return shared.ErrConflict.WithDetail("pages.drafts_pending").
					WithParams(map[string]string{"count": strconv.Itoa(pending)})
			}
		}
		if err := h.Space.Drafting.Settings.Put(ctx, h.Space.IDs.NewID(), walk[1], domain.DraftsKey, entry, now); err != nil {
			return err
		}
		if view, err = h.Space.spaceDrafts(ctx, actor.TenantID, cmd.HubID); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, SpaceDraftsSetAction, spaceTarget, cmd.HubID, []audit.Change{
			{Field: string(domain.DraftsKey), Classification: audit.Open, To: strconv.FormatBool(cmd.Drafts)},
		}, now)
	})
	return view, err
}

// Descriptor is the catalogue entry.
func (h SetSpaceDrafts) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetSpaceDraftsName, Permission: service.PermissionStructure,
		Summary: "Switches a hub's space between the live document, where every change is read at once, " +
			"and drafts, where its writers' changes wait until one of them publishes. Back to the live " +
			"document only once no draft waits.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub whose space is switched."},
			{Name: "drafts", Kind: usecase.KindBool, Required: true, Description: "Whether changes wait in drafts until published."},
		},
		Audit: usecase.AuditDeclaration{Action: SpaceDraftsSetAction, TargetType: spaceTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, SetSpaceDraftsCommand{HubID: id, Drafts: in.Bool("drafts")})
			if err != nil {
				return nil, err
			}
			return view.Output(), nil
		}),
	})
}
