// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package goal

import (
	"context"
	"errors"
	"strconv"

	goalrepo "github.com/Jersyfi/hubtask/core/application/repository/goal"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/goal"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The use case names and trail codes of a portfolio roadmap's public link (UC-GOL-03 check 15).
const (
	ReadPortfolioPublicLinkName   = "ReadPortfolioPublicLink"
	CreatePortfolioPublicLinkName = "CreatePortfolioPublicLink"
	DeletePortfolioPublicLinkName = "DeletePortfolioPublicLink"
	ReadPublicRoadmapsPolicyName  = "ReadPublicRoadmapsPolicy"
	SetPublicRoadmapsPolicyName   = "SetPublicRoadmapsPolicy"

	PortfolioPublicLinkReadAction    audit.Action = "portfolio.public_link_read"
	PortfolioPublicLinkCreatedAction audit.Action = "portfolio.public_link_created"
	PortfolioPublicLinkDeletedAction audit.Action = "portfolio.public_link_deleted"
	PublicRoadmapsPolicyReadAction   audit.Action = "public_roadmaps.policy_read"
	PublicRoadmapsPolicySetAction    audit.Action = "public_roadmaps.policy_set"

	roadmapPolicyTarget = "public_roadmaps"
	goalsPrefix         = "goals."
)

// RoadmapLinking is what a roadmap's public link needs beside the steward. A nil Links offers no
// public roadmaps at all.
type RoadmapLinking struct {
	Links    goalrepo.RoadmapLinks
	Settings settingrepo.Settings
	Entropy  clock.Entropy
	Accounts identityrepo.Accounts
	// Modules answers whether the goals module is on at the workspace: the unauthenticated route
	// passes no catalogue gate, so it asks itself.
	Modules ModuleStates
}

// ModuleStates answers the modules on at a place (setting.Modules); a zero hub is the workspace.
type ModuleStates interface {
	On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error)
}

// publicRoadmapsAllowed resolves the workspace's switch inside the caller's transaction; without a
// settings store it is off.
func (s Steward) publicRoadmapsAllowed(ctx context.Context, tenant shared.ID) (setting.Resolution, error) {
	definition := domain.PublicRoadmapsDefinition()
	if s.Public.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := s.Public.Settings.Above(ctx, goalsPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := s.Public.Settings.At(ctx, []settingrepo.Place{place}, goalsPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, plan}, settingrepo.LayersOf([]settingrepo.Place{place}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

// shareAuthority is who may hand the roadmap to people outside: who manages the workspace's
// members, as for a page's public link - a portfolio spans the hubs, so its link is the
// workspace's to give.
func (s Steward) shareAuthority(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope, target string, id shared.ID) error {
	if err := person(actor); err != nil {
		return err
	}
	return s.authorize(ctx, actor, service.PermissionManageMembers, workspace, action, scope, target, id)
}

func roadmapPolicyOutput(r setting.Resolution) usecase.Output {
	out := usecase.Output{"allowed": domain.PublicRoadmapsOf(r), "source": string(r.Source)}
	if r.Locked() {
		out["locked_by"] = string(r.LockedBy)
	}
	return out
}

// ReadPublicRoadmapsPolicy answers whether the workspace lets a roadmap be opened by a public link.
type ReadPublicRoadmapsPolicy struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadPublicRoadmapsPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPublicRoadmapsPolicyName, Permission: service.PermissionManageMembers, ReadOnly: true,
		Summary: "Answers whether the workspace lets a portfolio's roadmap be opened by a public link " +
			"without signing in, the level that decided it and the level that locked it.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Audit: usecase.AuditDeclaration{Action: PublicRoadmapsPolicyReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Steward.shareAuthority(ctx, actor, PublicRoadmapsPolicyReadAction, itemsRead, roadmapPolicyTarget, actor.TenantID); err != nil {
				return nil, err
			}
			var r setting.Resolution
			err := h.Steward.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				r, err = h.Steward.publicRoadmapsAllowed(ctx, actor.TenantID)
				return err
			})
			if err != nil {
				return nil, err
			}
			return roadmapPolicyOutput(r), nil
		}),
	}, roadmapPolicyTarget)
}

// SetPublicRoadmapsPolicy switches public roadmaps on or off for the workspace, inside what the
// installation and the plan leave. Switched off, every link stops answering; the links stay.
type SetPublicRoadmapsPolicy struct{ Steward Steward }

// Execute switches.
func (h SetPublicRoadmapsPolicy) Execute(ctx context.Context, actor appshared.ActorContext, allowed bool) (setting.Resolution, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return setting.Resolution{}, shared.ErrForbidden.WithDetail("portfolios.agent_decides_policy_not")
	}
	if err := h.Steward.shareAuthority(ctx, actor, PublicRoadmapsPolicySetAction, itemsWrite, roadmapPolicyTarget, actor.TenantID); err != nil {
		return setting.Resolution{}, err
	}
	if h.Steward.Public.Settings == nil {
		return setting.Resolution{}, shared.ErrUnavailable.WithDetail("portfolios.public_links_unavailable")
	}
	definition := domain.PublicRoadmapsDefinition()
	now := h.Steward.Clock.Now()
	var r setting.Resolution
	err := h.Steward.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		instance, plan, err := h.Steward.Public.Settings.Above(ctx, goalsPrefix)
		if err != nil {
			return err
		}
		entry := setting.Entry{Value: allowed, Hold: setting.HoldSet, By: actor.AccountID}
		if err := setting.Check(definition, setting.LevelWorkspace, setting.Resolve(definition, instance, plan), entry); err != nil {
			return err
		}
		place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
		if err := h.Steward.Public.Settings.Put(ctx, h.Steward.IDs.NewID(), place, domain.PublicRoadmapsKey, entry, now); err != nil {
			return err
		}
		if r, err = h.Steward.publicRoadmapsAllowed(ctx, actor.TenantID); err != nil {
			return err
		}
		return h.Steward.record(ctx, actor, PublicRoadmapsPolicySetAction, roadmapPolicyTarget, actor.TenantID, now,
			audit.Change{Field: string(domain.PublicRoadmapsKey), Classification: audit.Open, To: strconv.FormatBool(allowed)})
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetPublicRoadmapsPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPublicRoadmapsPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Switches whether a portfolio's roadmap may be opened by a public link without signing " +
			"in. Off by default; the installation or the plan may lock it. Switched off, every roadmap " +
			"link stops answering until it is switched on again.",
		SideEffects: "Stores the setting and writes an audit entry.", TokenScope: itemsWrite,
		Input: []usecase.Field{
			{Name: "allowed", Kind: usecase.KindBool, Required: true, Description: "Whether public roadmaps are allowed."},
		},
		Audit: usecase.AuditDeclaration{Action: PublicRoadmapsPolicySetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			r, err := h.Execute(ctx, actor, in.Bool("allowed"))
			if err != nil {
				return nil, err
			}
			return roadmapPolicyOutput(r), nil
		}),
	}, roadmapPolicyTarget)
}

// RoadmapLinkOutput is a link without its token (api/openapi.yaml, schema PortfolioPublicLink).
func RoadmapLinkOutput(link domain.RoadmapLink) usecase.Output {
	return usecase.Output{
		"portfolio_id": link.PortfolioID.String(), "created_by": link.CreatedBy.String(), "created_at": link.CreatedAt,
	}
}

// ReadPortfolioPublicLink answers whether a portfolio's roadmap has a public link, never its token.
type ReadPortfolioPublicLink struct{ Steward Steward }

// Descriptor is the catalogue entry.
func (h ReadPortfolioPublicLink) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPortfolioPublicLinkName, Permission: service.PermissionManageMembers, ReadOnly: true,
		Summary:     "Answers a portfolio roadmap's public link - who made it and when, never its token - or none.",
		SideEffects: "None. Reads only.", TokenScope: itemsRead,
		Input: []usecase.Field{{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."}},
		Audit: usecase.AuditDeclaration{Action: PortfolioPublicLinkReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			if err := h.Steward.shareAuthority(ctx, actor, PortfolioPublicLinkReadAction, itemsRead, portfolioTarget, id); err != nil {
				return nil, err
			}
			out := usecase.Output{"portfolio_id": id.String(), "link": nil}
			if h.Steward.Public.Links == nil {
				return out, nil
			}
			err = h.Steward.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if _, err := h.Steward.Store.Portfolio(ctx, id); err != nil {
					return err
				}
				link, found, err := h.Steward.Public.Links.Find(ctx, id)
				if err == nil && found {
					out["link"] = RoadmapLinkOutput(link)
				}
				return err
			})
			if err != nil {
				return nil, err
			}
			return out, nil
		}),
	}, portfolioTarget)
}

// MintedRoadmapLink is a link with its token, answered once: only the hash is kept.
type MintedRoadmapLink struct {
	Link  domain.RoadmapLink
	Token integration.RoadmapLinkToken
}

// CreatePortfolioPublicLink makes a portfolio roadmap's public link, replacing the one it had.
type CreatePortfolioPublicLink struct{ Steward Steward }

// Execute makes the link.
func (h CreatePortfolioPublicLink) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) (MintedRoadmapLink, error) {
	if err := h.Steward.shareAuthority(ctx, actor, PortfolioPublicLinkCreatedAction, itemsWrite, portfolioTarget, id); err != nil {
		return MintedRoadmapLink{}, err
	}
	if h.Steward.Public.Links == nil || h.Steward.Public.Entropy == nil {
		return MintedRoadmapLink{}, shared.ErrUnavailable.WithDetail("portfolios.public_links_unavailable")
	}
	secret, err := h.Steward.Public.Entropy.Bytes(integration.RoadmapLinkTokenSecretBytes)
	if err != nil {
		return MintedRoadmapLink{}, shared.ErrInternal.WithDetail("portfolios.link_unmintable").WithCause(err)
	}
	token, err := integration.NewRoadmapLinkToken(actor.TenantID, secret)
	if err != nil {
		return MintedRoadmapLink{}, err
	}
	now := h.Steward.Clock.Now()
	var link domain.RoadmapLink
	err = h.Steward.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		r, err := h.Steward.publicRoadmapsAllowed(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if !domain.PublicRoadmapsOf(r) {
			return shared.ErrForbidden.WithDetail("portfolios.public_links_off")
		}
		if _, err := h.Steward.Store.Portfolio(ctx, id); err != nil {
			return err
		}
		if link, err = domain.NewRoadmapLink(actor.TenantID, id, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Steward.Public.Links.Put(ctx, link, token); err != nil {
			return err
		}
		return h.Steward.record(ctx, actor, PortfolioPublicLinkCreatedAction, portfolioTarget, id, now)
	})
	if err != nil {
		return MintedRoadmapLink{}, err
	}
	return MintedRoadmapLink{Link: link, Token: token}, nil
}

// Descriptor is the catalogue entry.
func (h CreatePortfolioPublicLink) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePortfolioPublicLinkName, Permission: service.PermissionManageMembers,
		Summary: "Makes a portfolio roadmap's public link and answers its token, once: whoever holds it " +
			"reads the roadmap without signing in, read-only. A new link replaces the previous one. Only " +
			"where the workspace allows public roadmaps. The link reads as the person who made it: it " +
			"shows what they may read, and nothing once they no longer read the workspace.",
		SideEffects: "Writes the link's hash and an audit entry; the previous link stops answering.",
		TokenScope:  itemsWrite,
		Input:       []usecase.Field{{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."}},
		Audit:       usecase.AuditDeclaration{Action: PortfolioPublicLinkCreatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			minted, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			out := RoadmapLinkOutput(minted.Link)
			out["token"] = minted.Token.Secret()
			return out, nil
		}),
	}, portfolioTarget)
}

// DeletePortfolioPublicLink takes a portfolio roadmap's public link away.
type DeletePortfolioPublicLink struct{ Steward Steward }

// Execute takes it away.
func (h DeletePortfolioPublicLink) Execute(ctx context.Context, actor appshared.ActorContext, id shared.ID) error {
	if err := h.Steward.shareAuthority(ctx, actor, PortfolioPublicLinkDeletedAction, itemsWrite, portfolioTarget, id); err != nil {
		return err
	}
	if h.Steward.Public.Links == nil {
		return roadmapLinkGone()
	}
	now := h.Steward.Clock.Now()
	return h.Steward.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Steward.Public.Links.Delete(ctx, id)
		if err != nil {
			return err
		}
		if !removed {
			return roadmapLinkGone()
		}
		return h.Steward.record(ctx, actor, PortfolioPublicLinkDeletedAction, portfolioTarget, id, now)
	})
}

// Descriptor is the catalogue entry.
func (h DeletePortfolioPublicLink) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeletePortfolioPublicLinkName, Permission: service.PermissionManageMembers,
		Summary:     "Takes a portfolio roadmap's public link away; it answers nothing from then on.",
		SideEffects: "Removes the link and writes an audit entry.", TokenScope: itemsWrite, Destructive: true,
		Input: []usecase.Field{{Name: "portfolio_id", Kind: usecase.KindID, Required: true, Description: "The portfolio."}},
		Audit: usecase.AuditDeclaration{Action: PortfolioPublicLinkDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("portfolio_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, id)
		}),
	}, portfolioTarget)
}

// ReadPublicRoadmap answers what a roadmap's public link shows. Not a use case and not in the
// catalogue, for the calendar feed's reason: it answers a credential nobody in the system holds.
// The tenant comes from the token alone, and every question is asked here (ADR-0005).
//
// The roadmap is read as the link's maker, now: a link whose workspace switched public roadmaps or
// the goals module off, whose maker is gone or no longer reads the workspace, or whose portfolio
// was deleted answers what an unknown token answers.
type ReadPublicRoadmap struct{ Steward Steward }

// Execute answers the roadmap, or that there is none.
func (h ReadPublicRoadmap) Execute(ctx context.Context, token integration.RoadmapLinkToken, zone string) (usecase.Output, error) {
	s := h.Steward
	if token.IsZero() || s.Public.Links == nil || s.Public.Accounts == nil {
		return nil, roadmapLinkGone()
	}
	var (
		link  domain.RoadmapLink
		maker appshared.ActorContext
	)
	err := s.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: token.TenantID()}, func(ctx context.Context) error {
		var err error
		if link, err = s.Public.Links.FindByToken(ctx, token); err != nil {
			return err
		}
		r, err := s.publicRoadmapsAllowed(ctx, token.TenantID())
		if err != nil {
			return err
		}
		if !domain.PublicRoadmapsOf(r) {
			return roadmapLinkGone()
		}
		account, err := s.Public.Accounts.Find(ctx, link.CreatedBy)
		if err != nil {
			return err
		}
		if err := account.Verify(); err != nil {
			return roadmapLinkGone()
		}
		maker = appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: token.TenantID(), AccountID: account.ID,
			AccountName: account.DisplayName, Locale: account.Locale, TimeZone: account.TimeZone,
			// The one read a link performs, and nothing else.
			Scopes: []string{itemsRead},
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, roadmapLinkGone()
		}
		return nil, err
	}
	if s.Public.Modules != nil {
		on, err := s.Public.Modules.On(ctx, maker, "")
		if err != nil {
			return nil, err
		}
		if !on.Has(module.Goals) {
			return nil, roadmapLinkGone()
		}
	}
	out, err := s.roadmap(ctx, maker, link.PortfolioID, zone)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) || errors.Is(err, shared.ErrForbidden) {
			return nil, roadmapLinkGone()
		}
		return nil, err
	}
	return out, nil
}

// roadmapLinkGone is the one answer every refusal on the public route gives (T-21).
func roadmapLinkGone() error { return shared.ErrNotFound.WithDetail("portfolios.link_not_found") }
