// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RotateJumbleIntakeName = "RotateJumbleIntake"

	// automationManageScope is the automation surface's token scope: pointing outside input at
	// the workspace is that surface's power wherever the input lands.
	automationManageScope = "automation:manage"

	// IntakeRotatedAction is the intake credential being minted or replaced. A warning for the
	// inbound trigger's reason: what this creates is an address on the public internet that puts
	// content into the workspace, and both halves of that sentence are what a review looks for.
	IntakeRotatedAction audit.Action = "jumble.intake_rotated"
)

// RotateJumbleIntake mints the address the jumble accepts webhooks on, and replaces it (G-10).
//
// The inbound trigger's discipline applied to the inbox: one address per tenant, minted and
// replaced in a single statement, answered once, stored as a hash under the intake's own purpose
// label. Somebody who wants to revoke without a replacement has nothing to switch off - they
// rotate, and the old address is dead the same instant.
type RotateJumbleIntake struct {
	Intake     repository.Intake
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Entropy is where the token's secret half comes from (rule 4).
	Entropy clock.Entropy
}

// MintedIntake is what a rotation answers: when, and the credential that will never be readable
// again.
type MintedIntake struct {
	Token     integration.InboundToken
	RotatedAt time.Time
}

// Execute mints the address.
func (h RotateJumbleIntake) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (MintedIntake, error) {
	// The automation permission at the tenant: pointing outside input at the workspace is the
	// same power wherever the input lands, and the inbound trigger and the webhook subscriptions
	// already ask this question with this vocabulary.
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     IntakeRotatedAction,
		TokenScope: automationManageScope,
		TargetType: entryTarget,
	}); err != nil {
		return MintedIntake{}, err
	}

	material, err := h.Entropy.Bytes(integration.InboundTokenSecretBytes)
	if err != nil {
		return MintedIntake{}, shared.ErrUnavailable.
			WithDetail("jumble.intake_token_undrawable")
	}
	token, err := integration.NewInboundToken(actor.TenantID, material)
	if err != nil {
		return MintedIntake{}, err
	}

	now := h.Clock.Now()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Intake.SetToken(ctx, token, now)
	})
	if err != nil {
		return MintedIntake{}, err
	}

	h.record(ctx, actor, now)
	return MintedIntake{Token: token, RotatedAt: now}, nil
}

// record writes the trail entry: that the address changed and when - never anything of the token.
func (h RotateJumbleIntake) record(
	ctx context.Context, actor appshared.ActorContext, at time.Time,
) {
	if h.Audit == nil {
		return
	}
	_ = h.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     IntakeRotatedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityWarning,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: entryTarget,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// Descriptor is the catalogue entry.
func (h RotateJumbleIntake) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotateJumbleIntakeName,
		Summary: "Mints the address the jumble accepts webhooks on, replacing any previous one " +
			"in the same statement - the old token and the new one never both open the intake. " +
			"The token is answered once and stored hashed; every later read shows only when it " +
			"was minted.",
		SideEffects: "Replaces the tenant's intake credential and writes an audit entry.",
		TokenScope:  automationManageScope,
		Audit: usecase.AuditDeclaration{
			Action: IntakeRotatedAction, TargetType: entryTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A credential is about the workspace's ingress rather than about one entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RotateJumbleIntake) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	minted, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		// The one read of the credential, by its named accessor: String() masks (T-18).
		"token":      minted.Token.Secret(),
		"rotated_at": minted.RotatedAt,
	}, nil
}

// IntakeJumbleEntry is the unauthenticated half: a delivery arrives on the tenant's address and
// becomes an entry (G-10).
//
// Not a catalogue entry, for StartInboundRun's reason: there is no actor to authorise - the token
// authenticates the tenant, never a person - and the registry is the vocabulary of what a
// credential's owner may ask for.
type IntakeJumbleEntry struct {
	Intake     repository.Intake
	Entries    repository.Entries
	Events     outboxEvents
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// IntakeDelivery is one arrival on the wire: the credential and the small shape it carried.
type IntakeDelivery struct {
	Token   integration.InboundToken
	Sender  string
	Subject string
	Body    string
}

// Execute stores the delivery as an entry.
//
// Every refusal is the same one: an unknown token, a rotated one and a tenant that never minted
// one all answer not found with the same body - distinguishing them would answer questions for
// whoever is trying tokens (T-21).
func (h IntakeJumbleEntry) Execute(
	ctx context.Context, delivery IntakeDelivery,
) (domain.Entry, error) {
	if delivery.Token.IsZero() {
		return domain.Entry{}, intakeGone()
	}

	// The tenant the token names, and the only honest source of one on a route with no
	// authentication (multi-tenancy.md §2.2).
	scope := persistence.Scope{TenantID: delivery.Token.TenantID()}
	now := h.Clock.Now()

	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: h.IDs.NewID(), TenantID: delivery.Token.TenantID(), Channel: domain.ChannelWebhook,
		Sender: delivery.Sender, RawSubject: delivery.Subject, RawBody: delivery.Body,
		Now: now,
	})
	if err != nil {
		return domain.Entry{}, err
	}

	err = h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		opens, err := h.Intake.VerifyToken(ctx, delivery.Token)
		if err != nil {
			return err
		}
		if !opens {
			return intakeGone()
		}

		if err := h.Entries.Insert(ctx, entry); err != nil {
			return err
		}
		if h.Events == nil {
			return nil
		}
		// The system as the actor: the token authenticates the tenant, and naming an account
		// would invent an author for something nobody did.
		envelope, err := event.NewJumbleEntryReceived(
			h.IDs.NewID(), entry, event.Actor{Kind: shared.ActorSystem}, now, event.Cause{})
		if err != nil {
			return err
		}
		return h.Events.Append(ctx, envelope)
	})
	if err != nil {
		return domain.Entry{}, err
	}
	return entry, nil
}

// outboxEvents is the one append the intake makes.
type outboxEvents interface {
	Append(ctx context.Context, envelope event.Envelope) error
}

// intakeGone is the one refusal every failed delivery gets.
func intakeGone() error {
	return shared.ErrNotFound.WithDetail("jumble.inbound_not_found")
}
