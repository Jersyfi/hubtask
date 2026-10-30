// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	StartViewAsName = "StartViewAs"
	GetViewAsName   = "GetViewAs"
	EndViewAsName   = "EndViewAs"

	// ViewedAsAction is an administrator's session showing the app as another person (UC-ID-29).
	// A warning: for half an hour one tab reads what that person reads.
	ViewedAsAction audit.Action = "account.viewed_as"
	// ViewAsEndedAction is the view ended by the viewer before its half hour ran out.
	ViewAsEndedAction audit.Action = "account.view_as_ended"
	// ViewAsReadAction is declared for the catalogue; reading the running view refuses nobody.
	ViewAsReadAction audit.Action = "account.view_as_read"
)

// ViewAs is a running view: the person shown, and until when.
type ViewAs struct {
	Active      bool
	AccountID   shared.ID
	DisplayName string
	Until       time.Time
}

// StartViewAs makes the caller's own session show the app as another person sees it, read only,
// for half an hour (UC-ID-29). It takes what issuing a way into the account takes - MANAGE_MEMBERS
// at the workspace, and nobody viewed holds a role above the viewer's (wayInCeiling) - because a
// view reads everything the person reads. A person in a private hub is not viewed: the hub is
// opened only by its owner's emergency access (ADR-0073 §1). Only a person's own session views; a
// token, an app's grant, an agent or a rule views as nobody.
type StartViewAs struct {
	Accounts   repository.Accounts
	Sessions   repository.Sessions
	Held       repository.HeldRoles
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute starts the view.
func (h StartViewAs) Execute(
	ctx context.Context, actor appshared.ActorContext, accountID shared.ID,
) (ViewAs, error) {
	if accountID.IsZero() {
		return ViewAs{}, shared.ErrValidation.WithDetail("validation.required").
			WithParams(map[string]string{"field": "account_id"})
	}
	request := access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     ViewedAsAction,
		TokenScope: membersWrite,
		TargetType: accountTarget,
		TargetID:   accountID,
	}
	if err := h.Authorizer.Authorize(ctx, actor, request); err != nil {
		return ViewAs{}, err
	}
	if actor.Kind != appshared.ActorUser || !actor.OnBehalfOf.IsZero() || !actor.RunsForRule.IsZero() ||
		!actor.APIClient.IsZero() || actor.TokenID.IsZero() {
		return ViewAs{}, viewAsNeedsASession()
	}
	if accountID == actor.AccountID {
		return ViewAs{}, shared.ErrConflict.WithDetail("access.view_as_self")
	}

	ceiling := wayInCeiling{Held: h.Held, Authorizer: h.Authorizer}
	var person domain.Account
	var held []domain.Role
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Accounts.Find(ctx, accountID)
		if err != nil {
			return err
		}
		person = read
		held, err = ceiling.heldBy(ctx, accountID)
		return err
	})
	if err != nil {
		return ViewAs{}, err
	}
	if person.Kind != domain.AccountUser || person.Verify() != nil {
		return ViewAs{}, shared.ErrConflict.WithDetail("access.view_as_not_a_person")
	}
	if err := ceiling.hold(ctx, actor, request, held); err != nil {
		return ViewAs{}, err
	}

	now := h.Clock.Now()
	until := now.Add(domain.ViewAsLifetime).UTC()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		started, err := h.Sessions.StartViewingAs(ctx, actor.TokenID, actor.AccountID, person.ID, until, now)
		if err != nil {
			return err
		}
		if !started {
			return viewAsNeedsASession()
		}
		// Read back the way authentication reads it, so that the refusal and the lapse are one
		// judgement: a person in a private hub, directly or through a group, is not viewed.
		credential, err := h.Sessions.FindForAuth(ctx, actor.TokenID)
		if err != nil {
			return err
		}
		if credential.ViewedReachesPrivateHub {
			return shared.ErrForbidden.WithDetail("access.view_as_private_hub")
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: now,
			Action:     ViewedAsAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityWarning,
			ActorKind:  actor.Kind,
			ActorID:    actor.AccountID,
			ActorLabel: actor.AccountName,
			TargetType: accountTarget,
			TargetID:   person.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "viewing_as_until", Classification: audit.Open, To: until.Format(time.RFC3339),
			}),
		})
	})
	if err != nil {
		return ViewAs{}, err
	}
	return ViewAs{Active: true, AccountID: person.ID, DisplayName: person.DisplayName, Until: until}, nil
}

func viewAsNeedsASession() error {
	return shared.ErrForbidden.WithDetail("access.view_as_needs_a_session")
}

// Descriptor is the catalogue entry.
func (h StartViewAs) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       StartViewAsName,
		Permission: service.PermissionManageMembers,
		Summary: "Shows the app on the caller's own session as another person of the workspace sees " +
			"it, read only and online only, for half an hour. Takes MANAGE_MEMBERS at the workspace " +
			"and nobody holding a role above the caller's; a person in a private hub is not viewed. " +
			"A personal access token, an app's grant, an agent or a rule views as nobody.",
		SideEffects: "Marks the session and writes an audit entry naming the person and the end.",
		TokenScope:  membersWrite,
		Input: []usecase.Field{
			{Name: "account_id", Kind: usecase.KindID, Required: true, Description: "The person viewed."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ViewedAsAction, TargetType: accountTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "A view changes no entry."},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			accountID, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			view, err := h.Execute(ctx, actor, accountID)
			if err != nil {
				return nil, err
			}
			return viewAsOutput(view), nil
		}),
	}
}

// GetViewAs answers whether the caller's session shows the app as somebody else right now, and
// whom - what the web app draws its banner from. Asked as the person viewed, it answers them.
type GetViewAs struct{}

// Execute answers the running view.
func (GetViewAs) Execute(actor appshared.ActorContext) (ViewAs, error) {
	if !actor.IsAuthenticated() {
		return ViewAs{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if !actor.IsViewingAs() {
		return ViewAs{}, nil
	}
	return ViewAs{Active: true, AccountID: actor.AccountID, DisplayName: actor.AccountName,
		Until: actor.ViewingAsUntil}, nil
}

// Descriptor is the catalogue entry.
func (h GetViewAs) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        GetViewAsName,
		Summary:     "Answers whether the caller's session shows the app as another person, whom and until when.",
		SideEffects: "None. Reads only.",
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: ViewAsReadAction, TargetType: sessionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			_ context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			view, err := h.Execute(actor)
			if err != nil {
				return nil, err
			}
			return viewAsOutput(view), nil
		}),
	}
}

// EndViewAs ends the caller's view and gives the session back to its own person. It needs no scope
// beyond the session: a view reads only, and ending it is the one act a view may make. Ending
// where no view runs - or one that lapsed - is not an error.
type EndViewAs struct {
	Sessions   repository.Sessions
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute ends the view.
func (h EndViewAs) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if !actor.IsAuthenticated() || actor.TokenID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	holder := actor.AccountID
	if actor.IsViewingAs() {
		holder = actor.ViewedBy
	}
	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		ended, err := h.Sessions.EndViewingAs(ctx, actor.TokenID, holder)
		if err != nil || !ended || !actor.IsViewingAs() {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID:   actor.TenantID,
			OccurredAt: h.Clock.Now(),
			Action:     ViewAsEndedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityInfo,
			ActorKind:  appshared.ActorUser,
			ActorID:    actor.ViewedBy,
			ActorLabel: actor.ViewedByName,
			TargetType: accountTarget,
			TargetID:   actor.AccountID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// Descriptor is the catalogue entry.
func (h EndViewAs) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        EndViewAsName,
		Summary:     "Ends the caller's view as another person; the session is its own person's again.",
		SideEffects: "Clears the session's view and writes an audit entry where one ran.",
		Audit: usecase.AuditDeclaration{
			Action: ViewAsEndedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "A view changes no entry."},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			return nil, h.Execute(ctx, actor)
		}),
	}
}

func viewAsOutput(view ViewAs) usecase.Output {
	out := usecase.Output{"active": view.Active}
	if view.Active {
		out["account_id"] = view.AccountID.String()
		out["display_name"] = view.DisplayName
		out["until"] = view.Until.UTC()
	}
	return out
}
