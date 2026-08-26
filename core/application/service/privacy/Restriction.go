// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"

	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RestrictProcessingName = "RestrictProcessing"

	accountTarget = "account"

	// RestrictedAction and UnrestrictedAction are the two ends of Art. 18. Both warnings: one
	// stops this system deciding anything about a person automatically, and the other starts it
	// again - and which of the two happened, and when, is what a supervisory authority asks about.
	RestrictedAction   audit.Action = "dsr.processing_restricted"
	UnrestrictedAction audit.Action = "dsr.processing_resumed"
)

// RestrictProcessing puts an account under Art. 18, or takes it back out.
//
// A technical state rather than a lock. The account stays readable, the person keeps working, and
// what stops is the *processing*: no automatic decision is made about them (the assignment policy
// leaves them out of its draw) and no AI is shown their content. Treating a restriction as a
// lockout would punish somebody for exercising a right.
type RestrictProcessing struct {
	Subjects   repository.Subjects
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// RestrictCommand is the input, typed.
type RestrictCommand struct {
	AccountID  shared.ID
	Restricted bool
	Reason     string
}

// Execute writes the state.
//
// `MANAGE_MEMBERS`, because it is a state of an account and the administrator who grants and
// revokes access is the one who answers for it. It is not the owner's line: nothing is destroyed
// and nothing leaves - the workspace simply stops deciding things about one person by machine.
func (h RestrictProcessing) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd RestrictCommand,
) error {
	if cmd.AccountID.IsZero() {
		return shared.ErrValidation.
			WithDetail("accounts.not_found").
			WithFields(shared.FieldError{Path: "/account_id", Code: "accounts.not_found"})
	}

	action := RestrictedAction
	status := identity.AccountRestricted
	if !cmd.Restricted {
		action, status = UnrestrictedAction, identity.AccountActive
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     action,
		TokenScope: privacyManage,
		TargetType: accountTarget,
		TargetID:   cmd.AccountID,
	}); err != nil {
		return err
	}

	return h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		written, err := h.Subjects.SetStatus(ctx, cmd.AccountID, string(status), h.Clock.Now())
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrNotFound.WithDetail("accounts.not_found").
				WithParams(map[string]string{"account_id": cmd.AccountID.String()})
		}

		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: action, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: accountTarget, TargetID: cmd.AccountID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "status", Classification: audit.Open, To: string(status)},
				// The reason is the operator's own words about a decision they took, which is what
				// an auditor reading the entry months later has to go on.
				audit.Change{Field: "reason", Classification: audit.Open, To: cmd.Reason},
			),
			LegalBasis: LegalBasisOf(domain.KindRestriction),
		})
	})
}

// Descriptor registers the restriction in all three channels.
func (h RestrictProcessing) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RestrictProcessingName,
		Summary: "Puts an account under a restriction of processing, or lifts one. The account " +
			"stays readable and the person keeps working; what stops is this system deciding " +
			"anything about them by machine - the assignment policy leaves them out of its draw, " +
			"and no AI is shown their content. It is not a lock, and lifting it is the same call.",
		SideEffects: "Writes the account's state and an audit entry carrying the reason. Nothing " +
			"of the person's content is changed or removed.",
		TokenScope: privacyManage,
		Input: []usecase.Field{
			{
				Name: "account_id", Kind: usecase.KindID, Required: true,
				Description: "Whose processing is restricted.",
			},
			{
				Name: "restricted", Kind: usecase.KindBool, Required: true,
				Description: "True to restrict, false to lift.",
			},
			{
				Name: "reason", Kind: usecase.KindString,
				Description: "Why, for the audit trail. The restriction is a decision somebody took.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: RestrictedAction, TargetType: accountTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RestrictProcessing) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}

	restricted := in.Bool("restricted")
	if err := h.Execute(ctx, actor, RestrictCommand{
		AccountID: accountID, Restricted: restricted, Reason: in.String("reason"),
	}); err != nil {
		return nil, err
	}

	status := string(identity.AccountActive)
	if restricted {
		status = string(identity.AccountRestricted)
	}
	return usecase.Output{"id": accountID.String(), "status": status}, nil
}
