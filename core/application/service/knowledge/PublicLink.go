// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"errors"
	"strconv"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The use case names and trail codes of a page's public link (UC-DOC-08 check 4).
const (
	CreatePagePublicLinkName  = "CreatePagePublicLink"
	DeletePagePublicLinkName  = "DeletePagePublicLink"
	ReadPublicLinksPolicyName = "ReadPublicLinksPolicy"
	SetPublicLinksPolicyName  = "SetPublicLinksPolicy"

	PagePublicLinkCreatedAction audit.Action = "page.public_link_created"
	PagePublicLinkDeletedAction audit.Action = "page.public_link_deleted"
	PublicLinksPolicyReadAction audit.Action = "page_public_links.policy_read"
	PublicLinksPolicySetAction  audit.Action = "page_public_links.policy_set"

	publicLinkTarget = "page_public_link"
	policyTarget     = "page_public_links"
	knowledgePrefix  = "knowledge."
)

// PublicLinking is what a page's public link needs beside the space. A nil Links in Space offers
// no public links at all.
type PublicLinking struct {
	Links    repository.PublicLinks
	Settings settingrepo.Settings
	Entropy  clock.Entropy
	Accounts identityrepo.Accounts
	// Modules answers whether the knowledge module is on at the page's hub: the unauthenticated
	// route passes no catalogue gate, so it asks itself.
	Modules ModuleStates
}

// ModuleStates answers the modules on at a place (setting.Modules).
type ModuleStates interface {
	On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error)
}

// publicLinksAllowed resolves the workspace's switch inside the caller's transaction; without a
// settings store it is off.
func (s Space) publicLinksAllowed(ctx context.Context, tenant shared.ID) (setting.Resolution, error) {
	definition := domain.PublicLinksDefinition()
	if s.Public.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := s.Public.Settings.Above(ctx, knowledgePrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := s.Public.Settings.At(ctx, []settingrepo.Place{workspace}, knowledgePrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, plan}, settingrepo.LayersOf([]settingrepo.Place{workspace}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

func (s Space) administer(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string) error {
	return s.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: scope, TargetType: policyTarget, TargetID: actor.TenantID,
	})
}

func policyOutput(r setting.Resolution) usecase.Output {
	out := usecase.Output{"allowed": domain.PublicLinksOf(r), "source": string(r.Source)}
	if r.Locked() {
		out["locked_by"] = string(r.LockedBy)
	}
	return out
}

// ReadPublicLinksPolicy answers whether the workspace lets a page be opened by a public link, the
// level that decided it and the level that locked it.
type ReadPublicLinksPolicy struct{ Space Space }

// Descriptor is the catalogue entry.
func (h ReadPublicLinksPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPublicLinksPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Answers whether the workspace lets a page be opened by a public link without " +
			"signing in, the level that decided it and the level that locked it.",
		SideEffects: "None. Reads only.",
		TokenScope:  pagesRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: PublicLinksPolicyReadAction, TargetType: policyTarget, Severity: audit.SeverityInfo},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Space.administer(ctx, actor, PublicLinksPolicyReadAction, pagesRead); err != nil {
				return nil, err
			}
			var r setting.Resolution
			err := h.Space.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				r, err = h.Space.publicLinksAllowed(ctx, actor.TenantID)
				return err
			})
			if err != nil {
				return nil, err
			}
			return policyOutput(r), nil
		}),
	})
}

// SetPublicLinksPolicy switches public links on or off for the workspace, inside what the
// installation and the plan leave. Switching off makes every link stop answering; the links stay
// and answer again when it is switched back on.
type SetPublicLinksPolicy struct{ Space Space }

// Execute switches.
func (h SetPublicLinksPolicy) Execute(ctx context.Context, actor appshared.ActorContext, allowed bool) (setting.Resolution, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return setting.Resolution{}, shared.ErrForbidden.WithDetail("pages.agent_decides_policy_not")
	}
	if err := h.Space.administer(ctx, actor, PublicLinksPolicySetAction, pagesWrite); err != nil {
		return setting.Resolution{}, err
	}
	if h.Space.Public.Settings == nil {
		return setting.Resolution{}, shared.ErrUnavailable.WithDetail("pages.public_links_unavailable")
	}
	definition := domain.PublicLinksDefinition()
	now := h.Space.Clock.Now()
	var r setting.Resolution
	err := h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		instance, plan, err := h.Space.Public.Settings.Above(ctx, knowledgePrefix)
		if err != nil {
			return err
		}
		entry := setting.Entry{Value: allowed, Hold: setting.HoldSet, By: actor.AccountID}
		if err := setting.Check(definition, setting.LevelWorkspace, setting.Resolve(definition, instance, plan), entry); err != nil {
			return err
		}
		place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
		if err := h.Space.Public.Settings.Put(ctx, h.Space.IDs.NewID(), place, domain.PublicLinksKey, entry, now); err != nil {
			return err
		}
		if r, err = h.Space.publicLinksAllowed(ctx, actor.TenantID); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PublicLinksPolicySetAction, policyTarget, actor.TenantID, []audit.Change{
			{Field: string(domain.PublicLinksKey), Classification: audit.Open, To: strconv.FormatBool(allowed)},
		}, now)
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetPublicLinksPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPublicLinksPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Switches whether a page of the workspace may be opened by a public link without " +
			"signing in. Off by default; the installation or the plan may lock it. Switched off, " +
			"every public link stops answering until it is switched on again.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  pagesWrite,
		Input: []usecase.Field{
			{Name: "allowed", Kind: usecase.KindBool, Required: true, Description: "Whether public links are allowed."},
		},
		Audit: usecase.AuditDeclaration{Action: PublicLinksPolicySetAction, TargetType: policyTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			r, err := h.Execute(ctx, actor, in.Bool("allowed"))
			if err != nil {
				return nil, err
			}
			return policyOutput(r), nil
		}),
	})
}

// MintedLink is a public link with its token, answered once: only the hash is kept.
type MintedLink struct {
	Link  domain.PublicLink
	Token integration.PageLinkToken
}

// CreatePagePublicLink makes a page's public link, replacing the one it had: who manages the hub's
// members makes it, as they would share the page, where the workspace allows public links and the
// page is behind no restriction.
type CreatePagePublicLink struct{ Space Space }

// Execute makes the link.
func (h CreatePagePublicLink) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) (MintedLink, error) {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return MintedLink{}, err
	}
	if h.Space.Public.Links == nil {
		return MintedLink{}, shared.ErrUnavailable.WithDetail("pages.public_links_unavailable")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return MintedLink{}, err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PagePublicLinkCreatedAction, pagesWrite, publicLinkTarget, place.ID)); err != nil {
		return MintedLink{}, err
	}
	secret, err := h.Space.Public.Entropy.Bytes(integration.PageLinkTokenSecretBytes)
	if err != nil {
		return MintedLink{}, shared.ErrInternal.WithDetail("pages.link_unmintable").WithCause(err)
	}
	token, err := integration.NewPageLinkToken(actor.TenantID, secret)
	if err != nil {
		return MintedLink{}, err
	}
	now := h.Space.Clock.Now()
	var link domain.PublicLink
	err = h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		r, err := h.Space.publicLinksAllowed(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if !domain.PublicLinksOf(r) {
			return shared.ErrForbidden.WithDetail("pages.public_links_off")
		}
		page, err := h.Space.find(ctx, pageID, true)
		if err != nil {
			return err
		}
		readers, err := h.Space.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		if link, err = domain.NewPublicLink(actor.TenantID, page.ID, actor.AccountID, readers[page.ID], now); err != nil {
			return err
		}
		if err := h.Space.Public.Links.Put(ctx, link, token); err != nil {
			return err
		}
		return h.Space.record(ctx, actor, PagePublicLinkCreatedAction, pageTarget, page.ID, nil, now)
	})
	if err != nil {
		return MintedLink{}, err
	}
	return MintedLink{Link: link, Token: token}, nil
}

// Descriptor is the catalogue entry.
func (h CreatePagePublicLink) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePagePublicLinkName, Permission: service.PermissionManageMembers,
		Summary: "Makes a page's public link and answers its token, once: whoever holds it reads the " +
			"page alone, without signing in - not the pages under it, its comments or its history. " +
			"A new link replaces the page's previous one. Only where the workspace allows public " +
			"links, and never for a page behind a restriction. The link reads as the person who made " +
			"it: once they no longer read the page, it answers nothing.",
		SideEffects: "Writes the link's hash and an audit entry; the previous link stops answering.",
		TokenScope:  pagesWrite,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PagePublicLinkCreatedAction, TargetType: pageTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			minted, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			out := LinkOutput(minted.Link)
			out["token"] = minted.Token.Secret()
			return out, nil
		}),
	})
}

// LinkOutput is a link without its token (api/openapi.yaml, schema PagePublicLink).
func LinkOutput(link domain.PublicLink) usecase.Output {
	return usecase.Output{
		"page_id": link.PageID.String(), "created_by": idOrNil(link.CreatedBy), "created_at": link.CreatedAt,
	}
}

// DeletePagePublicLink takes a page's public link away, by somebody who manages the hub's members.
type DeletePagePublicLink struct{ Space Space }

// Execute takes it away.
func (h DeletePagePublicLink) Execute(ctx context.Context, actor appshared.ActorContext, pageID shared.ID) error {
	if err := actor.RequireScope(pagesWrite); err != nil {
		return err
	}
	if h.Space.Public.Links == nil {
		return shared.ErrNotFound.WithDetail("pages.link_not_found")
	}
	place, err := h.Space.placeOf(ctx, actor, pageID)
	if err != nil {
		return err
	}
	if err := h.Space.Authorizer.Authorize(ctx, actor, h.Space.request(place.HubID, service.PermissionManageMembers,
		PagePublicLinkDeletedAction, pagesWrite, publicLinkTarget, place.ID)); err != nil {
		return err
	}
	now := h.Space.Clock.Now()
	return h.Space.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Space.Public.Links.Delete(ctx, place.ID)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail("pages.link_not_found")
		}
		return h.Space.record(ctx, actor, PagePublicLinkDeletedAction, pageTarget, place.ID, nil, now)
	})
}

// Descriptor is the catalogue entry.
func (h DeletePagePublicLink) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeletePagePublicLinkName, Permission: service.PermissionManageMembers,
		Summary:     "Takes a page's public link away; it answers nothing from then on.",
		SideEffects: "Removes the link and writes an audit entry.",
		TokenScope:  pagesWrite,
		Destructive: true,
		Input:       []usecase.Field{{Name: "page_id", Kind: usecase.KindID, Required: true, Description: "The page."}},
		Audit:       usecase.AuditDeclaration{Action: PagePublicLinkDeletedAction, TargetType: pageTarget, Severity: audit.SeverityNotice, Required: true},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("page_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, id)
		}),
	})
}

// PublicPage is a page as its public link shows it: the page alone, as it stands now.
type PublicPage struct {
	Page domain.Page
}

// ReadPublicPage answers what a public link shows. Not a use case and not in the catalogue, for
// the calendar feed's reason: it answers a credential nobody in the system holds. The tenant comes
// from the token alone, and every question is asked here (ADR-0005).
//
// The link reads as the person who made it, evaluated now: a link whose maker no longer reads the
// page, whose workspace switched public links off or whose knowledge module is off, or whose page
// was trashed or restricted since, answers what an unknown token answers.
type ReadPublicPage struct{ Space Space }

// Execute answers the page, or that there is none.
func (h ReadPublicPage) Execute(ctx context.Context, token integration.PageLinkToken) (PublicPage, error) {
	if token.IsZero() || h.Space.Public.Links == nil || h.Space.Public.Accounts == nil {
		return PublicPage{}, linkGone()
	}
	scope := persistence.Scope{TenantID: token.TenantID()}
	var (
		page  domain.Page
		maker appshared.ActorContext
	)
	err := h.Space.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		link, err := h.Space.Public.Links.FindByToken(ctx, token)
		if err != nil {
			return err
		}
		r, err := h.Space.publicLinksAllowed(ctx, token.TenantID())
		if err != nil {
			return err
		}
		if !domain.PublicLinksOf(r) {
			return linkGone()
		}
		if page, err = h.Space.find(ctx, link.PageID, false); err != nil {
			return err
		}
		readers, err := h.Space.Pages.Readers(ctx, []shared.ID{page.ID})
		if err != nil {
			return err
		}
		if readers[page.ID].Restricted {
			return linkGone()
		}
		account, err := h.Space.Public.Accounts.Find(ctx, link.CreatedBy)
		if err != nil {
			return err
		}
		if err := account.Verify(); err != nil {
			return linkGone()
		}
		maker = appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: token.TenantID(), AccountID: account.ID,
			AccountName: account.DisplayName, Locale: account.Locale, TimeZone: account.TimeZone,
			// The one read a link performs, and nothing else.
			Scopes: []string{pagesRead},
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return PublicPage{}, linkGone()
		}
		return PublicPage{}, err
	}
	if h.Space.Public.Modules != nil {
		on, err := h.Space.Public.Modules.On(ctx, maker, page.HubID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return PublicPage{}, err
		}
		if !on.Has(module.Knowledge) {
			return PublicPage{}, linkGone()
		}
	}
	reads, err := h.Space.Authorizer.Permits(ctx, maker, h.Space.request(page.HubID, service.PermissionRead,
		PageReadAction, pagesRead, pageTarget, page.ID))
	if err != nil {
		return PublicPage{}, err
	}
	if !reads {
		return PublicPage{}, linkGone()
	}
	return PublicPage{Page: page}, nil
}

// linkGone is the one answer every refusal on the public route gives (T-21).
func linkGone() error { return shared.ErrNotFound.WithDetail("pages.link_not_found") }

// publicLinkOf answers a page's link for its access panel; none without a store.
func (s Space) publicLinkOf(ctx context.Context, pageID shared.ID) (*domain.PublicLink, error) {
	if s.Public.Links == nil {
		return nil, nil
	}
	link, found, err := s.Public.Links.Find(ctx, pageID)
	if err != nil || !found {
		return nil, err
	}
	return &link, nil
}
