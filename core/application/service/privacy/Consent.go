// SPDX-License-Identifier: Apache-2.0
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
	WithdrawConsentName = "WithdrawConsent"

	consentTarget = "consent_record"

	// ConsentWithdrawnAction is Art. 21 as this system can carry it out: the optional processing
	// stops and the core features keep working.
	ConsentWithdrawnAction audit.Action = "dsr.consent_withdrawn"

	GiveConsentName = "GiveConsent"

	// ConsentGivenAction is a person agreeing again - for AI, letting their own content back in.
	ConsentGivenAction audit.Action = "dsr.consent_given"
)

// Reindexer seeds the embedding pass of the transaction's workspace. A person's AI objection
// moves whose entries may be embedded, so the write that moved it seeds the pass. Nil seeds
// nothing.
type Reindexer interface {
	Reseed(ctx context.Context, tenantID shared.ID) error
}

// WithdrawConsent takes back an agreement to an optional processing purpose.
//
// Self-service for one's own consent, and an administrator's act for somebody else's - which is
// the same line `UpdateAccountPreferences` draws, and for the same reason: a preference and a
// consent are both the person's own, and an administrator recording one on their behalf is
// recording a decision that person took.
type WithdrawConsent struct {
	Consents   repository.Consents
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Reindex    Reindexer
}

// WithdrawCommand is the input, typed.
type WithdrawCommand struct {
	AccountID shared.ID
	Purpose   string
	Reason    string
}

// Execute records the withdrawal.
//
// A withdrawal for a purpose nobody granted is **not** an error. The person said no, and the
// record says they said no - which is what an operator showing which processing was lawful when
// needs, and a gap would leave them guessing.
func (h WithdrawConsent) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd WithdrawCommand,
) (domain.Consent, error) {
	subject := cmd.AccountID
	if subject.IsZero() {
		subject = actor.AccountID
	}

	source := domain.SourceUser
	if subject != actor.AccountID {
		// Somebody else's consent. The administrator's line, and the record says who wrote it.
		source = domain.SourceTenantAdmin
		if err := h.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionManageMembers,
			Path:       []identity.Scope{identity.TenantScope()},
			Action:     ConsentWithdrawnAction,
			TokenScope: privacyManage,
			TargetType: consentTarget,
			TargetID:   subject,
		}); err != nil {
			return domain.Consent{}, err
		}
	} else if !actor.IsAuthenticated() {
		return domain.Consent{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}

	withdrawal, err := domain.NewWithdrawal(
		h.IDs.NewID(), subject, cmd.Purpose, source, h.Clock.Now())
	if err != nil {
		return domain.Consent{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		ended, err := h.Consents.Withdraw(ctx, subject, withdrawal.Purpose, withdrawal.RevokedAt)
		if err != nil {
			return err
		}
		if err := h.Consents.Record(ctx, withdrawal); err != nil {
			return err
		}
		if err := reseedForAi(ctx, h.Reindex, actor.TenantID, withdrawal.Purpose); err != nil {
			return err
		}

		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: ConsentWithdrawnAction, Outcome: audit.OutcomeSuccess,
			Severity:  audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: consentTarget, TargetID: subject,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{
					Field: "purpose", Classification: audit.Open, To: withdrawal.Purpose,
				},
				// How many standing consents this ended. Zero is a legitimate answer and worth
				// recording: it says the person objected to something they had never agreed to,
				// which is a different fact from a consent being taken back.
				audit.Change{Field: "ended", Classification: audit.Open, To: ended},
				audit.Change{Field: "reason", Classification: audit.Open, To: cmd.Reason},
			),
			LegalBasis: LegalBasisOf(domain.KindObjection),
		})
	})
	if err != nil {
		return domain.Consent{}, err
	}
	return withdrawal, nil
}

// Descriptor registers the withdrawal in all three channels.
func (h WithdrawConsent) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: WithdrawConsentName,
		Summary: "Takes back an agreement to an optional processing purpose - AI, metering, " +
			"notification content. The optional processing stops and the core features keep " +
			"working. Withdrawing something that was never granted is recorded too: the person " +
			"said no, and the record says so rather than leaving a gap.",
		SideEffects: "Ends every standing consent for that purpose, records the withdrawal, and " +
			"writes an audit entry. Nothing else about the account changes.",
		TokenScope: privacyManage,
		Input: []usecase.Field{
			{
				Name: "purpose", Kind: usecase.KindString, Required: true,
				Description: "What the consent is about, e.g. `ai_processing`.",
			},
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose consent. Omitted means the caller's own.",
			},
			{Name: "reason", Kind: usecase.KindString, Description: "Why, for the audit trail."},
		},
		Audit: usecase.AuditDeclaration{
			Action: ConsentWithdrawnAction, TargetType: consentTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h WithdrawConsent) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}

	consent, err := h.Execute(ctx, actor, WithdrawCommand{
		AccountID: accountID, Purpose: in.String("purpose"), Reason: in.String("reason"),
	})
	if err != nil {
		return nil, err
	}
	return ConsentOutput(consent), nil
}

// ConsentOutput is one consent record as every channel answers it.
func ConsentOutput(consent domain.Consent) usecase.Output {
	out := usecase.Output{
		"id":         consent.ID.String(),
		"purpose":    consent.Purpose,
		"granted":    consent.Granted,
		"granted_at": consent.GrantedAt.UTC(),
	}
	if !consent.AccountID.IsZero() {
		out["account_id"] = consent.AccountID.String()
	}
	if !consent.RevokedAt.IsZero() {
		out["revoked_at"] = consent.RevokedAt.UTC()
	}
	if consent.Source != "" {
		out["source"] = consent.Source
	}
	return out
}

func reseedForAi(ctx context.Context, reindex Reindexer, tenantID shared.ID, purpose string) error {
	if reindex == nil || purpose != domain.PurposeAi {
		return nil
	}
	return reindex.Reseed(ctx, tenantID)
}

// GiveConsent records a person agreeing to an optional processing purpose - for AI, letting the
// content they authored back into prompts after they kept it out.
//
// The person's own act only. An administrator records a withdrawal on somebody's behalf, because
// a person may object by mail; nobody agrees in another's place (P-14), so a consent for somebody
// else is refused, and a withdrawal recorded by mistake is undone by the person.
type GiveConsent struct {
	Consents   repository.Consents
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Reindex    Reindexer
}

// GiveCommand is the input, typed.
type GiveCommand struct {
	AccountID shared.ID
	Purpose   string
}

// Execute records the consent.
func (h GiveConsent) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd GiveCommand,
) (domain.Consent, error) {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return domain.Consent{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(privacyManage); err != nil {
		return domain.Consent{}, err
	}
	if !cmd.AccountID.IsZero() && cmd.AccountID != actor.AccountID {
		return domain.Consent{}, shared.ErrForbidden.WithDetail(domain.CodeConsentOwnOnly)
	}
	given, err := domain.NewGrant(h.IDs.NewID(), actor.AccountID, cmd.Purpose, h.Clock.Now())
	if err != nil {
		return domain.Consent{}, err
	}
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Consents.Record(ctx, given); err != nil {
			return err
		}
		if err := reseedForAi(ctx, h.Reindex, actor.TenantID, given.Purpose); err != nil {
			return err
		}
		return h.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: h.Clock.Now(),
			Action: ConsentGivenAction, Outcome: audit.OutcomeSuccess,
			Severity:  audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: consentTarget, TargetID: actor.AccountID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "purpose", Classification: audit.Open, To: given.Purpose},
			),
		})
	})
	if err != nil {
		return domain.Consent{}, err
	}
	return given, nil
}

// Descriptor registers giving consent in every channel.
func (h GiveConsent) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GiveConsentName,
		Summary: "Records the caller agreeing to an optional processing purpose - for " +
			"`ai_processing`, letting the content they authored back into AI after they kept it " +
			"out. The caller's own act only: nobody agrees in another's place.",
		SideEffects: "Records the consent and writes an audit entry; for `ai_processing`, queues " +
			"rebuilding search by meaning. An earlier withdrawal stays on record for the period " +
			"it covered.",
		TokenScope: privacyManage,
		Input: []usecase.Field{
			{
				Name: "purpose", Kind: usecase.KindString, Required: true,
				Description: "What the consent is about, e.g. `ai_processing`.",
			},
			{
				Name: "account_id", Kind: usecase.KindID,
				Description: "Whose consent - only ever the caller's own. Anybody else's is refused.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ConsentGivenAction, TargetType: consentTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GiveConsent) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	accountID, err := in.ID("account_id")
	if err != nil {
		return nil, err
	}
	consent, err := h.Execute(ctx, actor, GiveCommand{AccountID: accountID, Purpose: in.String("purpose")})
	if err != nil {
		return nil, err
	}
	return ConsentOutput(consent), nil
}
