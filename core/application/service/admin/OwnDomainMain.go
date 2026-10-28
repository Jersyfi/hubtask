// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// The own domain as the main address, and back (UC-INS-15 checks 6, 7, 8 and 11; multi-tenancy.md
// §3.1). Either way is a change of address in what it asks and costs - the lockout rule, the proof
// by password or provider, everybody signing in again, the notices - but it replaces no row: the
// main mark moves between the own domain and the address row, which both stay and both answer, the
// one not main sending browsers on. No cooldown holds it: the way back is there at any time.

const (
	MakeOwnDomainMainName       = "MakeOwnDomainMain"
	SwitchBackFromOwnDomainName = "SwitchBackFromOwnDomain"

	// OwnDomainMadeMainAction and OwnDomainSwitchedBackAction are the workspace's trail. WARNING:
	// each ends every session.
	OwnDomainMadeMainAction     audit.Action = "tenant.own_domain_made_main"
	OwnDomainSwitchedBackAction audit.Action = "tenant.own_domain_switched_back"
)

// MainAddressCommand is the input of either way.
type MainAddressCommand struct {
	DryRun bool
	// StepUpToken is the proof, by password or provider, consumed by this one request.
	StepUpToken string
}

// MainAddress is what both ways share: the own domain's store and the change of address whose
// lockout rule, proof, audience and notices they ask.
type MainAddress struct {
	OwnDomains
	Change     ChangeTenantAddress
	Authorizer WorkspaceAuthorizer
}

// MakeOwnDomainMain is the owner's *Make it the main address*.
type MakeOwnDomainMain struct{ MainAddress }

// SwitchBackFromOwnDomain is the owner's way back to the subdomain.
type SwitchBackFromOwnDomain struct{ MainAddress }

// Execute makes the active own domain the main address, or with DryRun answers what it would be.
func (h MakeOwnDomainMain) Execute(ctx context.Context, actor appshared.ActorContext, cmd MainAddressCommand) (AddressChange, error) {
	return h.move(ctx, actor, cmd, true)
}

// Execute makes the subdomain the main address again, or with DryRun answers what it would be.
func (h SwitchBackFromOwnDomain) Execute(ctx context.Context, actor appshared.ActorContext, cmd MainAddressCommand) (AddressChange, error) {
	return h.move(ctx, actor, cmd, false)
}

func (h MainAddress) move(
	ctx context.Context, actor appshared.ActorContext, cmd MainAddressCommand, toOwn bool,
) (AddressChange, error) {
	action := OwnDomainSwitchedBackAction
	if toOwn {
		action = OwnDomainMadeMainAction
	}
	if err := h.authorizeOwner(ctx, h.Authorizer, actor, action); err != nil {
		return AddressChange{}, err
	}
	// The way back is never held by the switch: a domain set up before the operator switched own
	// domains off may still be given up.
	if toOwn {
		rule, err := h.rule(ctx)
		if err != nil {
			return AddressChange{}, err
		}
		if err := rule.Refuse(); err != nil {
			return AddressChange{}, err
		}
	}

	var (
		answer           AddressChange
		mailed, unmailed []shared.ID
	)
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		from, to, err := h.moveNow(ctx, toOwn)
		if err != nil {
			return err
		}
		answer.From, answer.To = from, to
		mailed, unmailed, err = h.Change.Audience.PeopleToTell(ctx)
		return err
	})
	if err != nil {
		return AddressChange{}, err
	}
	answer.Mailed, answer.ToldAtSignIn, answer.DryRun = len(mailed), len(unmailed), cmd.DryRun

	// The lockout first, then the proof: a refused move must not burn a step-up.
	if err := h.Change.refuseLockout(ctx, actor, actor.TenantID); err != nil {
		return AddressChange{}, err
	}
	if cmd.DryRun {
		return answer, nil
	}
	if err := stepup.DemandOneOf(ctx, h.Change.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken,
		stepUpPurposeAddress, stepup.MethodPassword, stepup.MethodProvider); err != nil {
		return AddressChange{}, err
	}

	now := h.Clock.Now()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Asked again under the write: a check or a block between the read and here wins.
		from, to, err := h.moveNow(ctx, toOwn)
		if err != nil {
			return err
		}
		if toOwn {
			_, err = h.Domains.MainToOwnDomain(ctx)
		} else {
			_, err = h.Domains.MainToAddress(ctx, time.Time{})
		}
		if err != nil {
			return err
		}
		if _, err := h.Change.Addresses.EndSessions(ctx, now); err != nil {
			return err
		}
		if err := h.trail(ctx, actor.TenantID, actor, action, audit.SeverityWarning,
			audit.Change{Field: "host", Classification: audit.Open, From: from, To: to}, now); err != nil {
			return err
		}
		if h.Change.Notices != nil {
			if err := h.Change.Notices.AddressChanged(ctx, actor.TenantID, from, to, now, mailed, unmailed); err != nil {
				return err
			}
		}
		answer.From, answer.To = from, to
		return nil
	})
	if err != nil {
		return AddressChange{}, err
	}
	return answer, nil
}

// moveNow reads the own domain and the address row in the caller's transaction and answers where
// the main address would leave and go.
func (h MainAddress) moveNow(ctx context.Context, toOwn bool) (string, string, error) {
	own, err := h.Domains.Find(ctx)
	if errors.Is(err, shared.ErrNotFound) {
		return "", "", shared.ErrNotFound.WithDetail("admin.own_domain_none")
	}
	if err != nil {
		return "", "", err
	}
	address, err := h.Domains.AddressHost(ctx)
	if err != nil {
		return "", "", err
	}
	return domain.MainAddressMove(own, address, toOwn)
}

// Descriptor is the catalogue entry.
func (h MakeOwnDomainMain) Descriptor() usecase.Descriptor {
	return mainAddressDescriptor(MakeOwnDomainMainName, OwnDomainMadeMainAction,
		"The workspace's owner makes its active own domain the main address, where the installation "+
			"allows own domains: the confirmation, lockout rule and proof of a change of address, "+
			"and its cost - everybody signs in again and passkeys made at the old address stop "+
			"working. The subdomain stays, sends browsers on and still answers the API. With "+
			"dry_run it checks and answers what the change would be.",
		func(ctx context.Context, actor appshared.ActorContext, cmd MainAddressCommand) (AddressChange, error) {
			return h.Execute(ctx, actor, cmd)
		})
}

// Descriptor is the catalogue entry.
func (h SwitchBackFromOwnDomain) Descriptor() usecase.Descriptor {
	return mainAddressDescriptor(SwitchBackFromOwnDomainName, OwnDomainSwitchedBackAction,
		"The workspace's owner makes its subdomain the main address again, at any time and at the "+
			"cost of a change of address. The own domain stays set up and sends browsers on. With "+
			"dry_run it checks and answers what the change would be.",
		func(ctx context.Context, actor appshared.ActorContext, cmd MainAddressCommand) (AddressChange, error) {
			return h.Execute(ctx, actor, cmd)
		})
}

func mainAddressDescriptor(
	name string, action audit.Action, summary string,
	execute func(context.Context, appshared.ActorContext, MainAddressCommand) (AddressChange, error),
) usecase.Descriptor {
	return usecase.Descriptor{
		Name:    name,
		Summary: summary,
		SideEffects: "Moves the workspace's main address, ends every person's session, mails every " +
			"member with an address and tells the others at their next sign-in; records it in the " +
			"workspace's trail.",
		TokenScope: workspaceManageScope,
		StepUp:     "always, by password or provider - a passkey belongs to the address that ends",
		Input: []usecase.Field{
			{Name: "dry_run", Kind: usecase.KindBool,
				Description: "Check and answer without changing anything, and without a step-up."},
			{Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof POST /auth/step-up answered, by password or provider."},
		},
		Audit: usecase.AuditDeclaration{
			Action: action, TargetType: tenantTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "a workspace's address is not an entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			change, err := execute(ctx, actor, MainAddressCommand{
				DryRun: in.Bool("dry_run"), StepUpToken: in.String("step_up_token"),
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"from": change.From, "to": change.To, "members_mailed": change.Mailed,
				"members_told_at_sign_in": change.ToldAtSignIn, "dry_run": change.DryRun,
			}, nil
		}),
	}
}
