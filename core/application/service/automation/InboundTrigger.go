// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RotateInboundTriggerName = "RotateInboundTrigger"

	// InboundRotatedAction is a credential being minted or replaced. A warning rather than a
	// notice, for the reason a backup target's is: what this creates is an address on the public
	// internet that starts a rule, and both halves of that sentence are what a review looks for.
	InboundRotatedAction audit.Action = "automation.inbound_rotated"
)

// RotateInboundTrigger mints the address an INBOUND_WEBHOOK rule answers on, and replaces it
// (G-08, automation.md §1.1).
//
// One call for both, and that is the whole of "revocable by rotating": there is exactly one address
// per rule, minting a second one replaces the first in the same statement, and a rule with two live
// addresses is the state this design does not have. Somebody who wants to revoke without a
// replacement switches the rule off, which is the honest way to stop a rule acting.
//
// The token is answered **once**. It is hashed under its own purpose label and stored as a value
// nobody can present, so no read afterwards can produce it - D-08's discipline, applied to a
// credential that starts a run rather than one that reads a list.
type RotateInboundTrigger struct {
	Rules      repository.Rules
	Inbound    repository.InboundTriggers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Entropy is where the token's secret half comes from. A port, so that production draws from
	// crypto/rand and a test can fix the credential it asserts on (rule 4).
	Entropy clock.Entropy
}

// MintedInboundTrigger is what a rotation answers: the rule, when it happened, and the credential
// that will never be readable again.
type MintedInboundTrigger struct {
	RuleID    shared.ID
	Token     integration.InboundToken
	RotatedAt time.Time
}

// Execute mints the address.
func (h RotateInboundTrigger) Execute(
	ctx context.Context, actor appshared.ActorContext, ruleID shared.ID,
) (MintedInboundTrigger, error) {
	var rule domain.Rule
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var findErr error
		rule, findErr = h.Rules.Find(ctx, ruleID)
		return findErr
	})
	if err != nil {
		return MintedInboundTrigger{}, err
	}

	// The automation permission at the rule's own scope. Not the composition check writing a rule
	// needs: an address does not widen what the rule may do, and what the rule does was decided
	// when it was written and is decided again, per action, when it runs.
	if err := h.Authorizer.Authorize(
		ctx, actor, runRequest(rule.Scope, InboundRotatedAction, rule.ID),
	); err != nil {
		return MintedInboundTrigger{}, err
	}

	if rule.Trigger.Kind != domain.TriggerInboundWebhook {
		// Refused by name. An address on a rule nothing can reach through it would be a credential
		// that opens nothing, handed out as though it worked.
		return MintedInboundTrigger{}, shared.ErrValidation.
			WithDetail("automation.trigger_not_inbound").
			WithParams(map[string]string{"kind": rule.Trigger.Kind.String()})
	}

	secret, err := h.Entropy.Bytes(integration.InboundTokenSecretBytes)
	if err != nil {
		return MintedInboundTrigger{}, shared.ErrUnavailable.
			WithDetail("automation.inbound_token_undrawable")
	}
	token, err := integration.NewInboundToken(actor.TenantID, secret)
	if err != nil {
		return MintedInboundTrigger{}, err
	}

	now := h.Clock.Now()
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		changed, err := h.Inbound.SetToken(ctx, rule.ID, token, now)
		if err != nil {
			return err
		}
		if !changed {
			// Deleted between the read and the write. Not found rather than a silent success:
			// answering a token that opens nothing is worse than answering nothing.
			return shared.ErrNotFound.
				WithDetail("automation.rule_not_found").
				WithParams(map[string]string{"rule_id": rule.ID.String()})
		}
		return nil
	})
	if err != nil {
		return MintedInboundTrigger{}, err
	}

	h.record(ctx, actor, rule, now)
	return MintedInboundTrigger{RuleID: rule.ID, Token: token, RotatedAt: now}, nil
}

// record writes the entry an auditor is looking for: an address on the public internet that starts
// this rule now exists, and whatever address existed before it does not. Never the token (rule 10):
// an audit trail carrying a live credential is a second place to steal one from.
func (h RotateInboundTrigger) record(
	ctx context.Context, actor appshared.ActorContext, rule domain.Rule, now time.Time,
) {
	if h.Audit == nil {
		return
	}
	_ = h.Audit.Append(ctx, audit.Entry{
		TenantID: rule.TenantID, OccurredAt: now,
		Action: InboundRotatedAction, Outcome: audit.OutcomeSuccess,
		Severity:  audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		OnBehalfOf: rule.RunAs,
		TargetType: ruleTarget, TargetID: rule.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// Descriptor is the catalogue entry.
func (h RotateInboundTrigger) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotateInboundTriggerName,
		Summary: "Mints the address an INBOUND_WEBHOOK rule answers on, and replaces whatever " +
			"address it had. The token is answered once and stored hashed, so no later read can " +
			"produce it - rotating is how one is revoked, because a rule has exactly one address.",
		SideEffects: "Replaces the rule's address and writes an audit entry. Any system holding " +
			"the previous token stops being able to start the rule.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{Name: "rule_id", Kind: usecase.KindID, Required: true, Description: "Which rule."},
		},
		Audit: usecase.AuditDeclaration{
			Action: InboundRotatedAction, TargetType: ruleTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "The same reason writing a rule is exempt: a rule is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RotateInboundTrigger) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("rule_id")
	if err != nil {
		return nil, err
	}
	minted, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"rule_id": minted.RuleID.String(),
		// The one time this value exists in an answer. Reading it is a deliberate act with a name
		// on the domain type, which is what stops it reaching a log by accident.
		"token":      minted.Token.Secret(),
		"rotated_at": minted.RotatedAt,
	}, nil
}

// StartInboundRun is the unauthenticated route's way into the engine (G-08).
//
// It is **not** a use case and is deliberately not in the catalogue, for ReadCalendarFeed's reason:
// a catalogue entry is something a person, an agent or a rule may ask for, and this is a route
// answering a credential nobody in the system holds. What it shares with a use case is the rule
// that matters - the tenant comes from the credential and from nowhere else (multi-tenancy.md
// §2.2) - and the rule that matters more: it can do exactly one thing, which is start that one
// rule's run.
//
// **It authenticates the rule, never a person.** There is no actor to record, which is why the run
// it produces carries no `triggered_by`: naming one would invent an author for something nobody
// did. What the run may then do is the rule's `run_as` account's business, checked per action.
type StartInboundRun struct {
	Inbound    repository.InboundTriggers
	Jobs       Queue
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// InboundDelivery is one arrival: the credential, and the body it carried.
type InboundDelivery struct {
	Token integration.InboundToken
	// Payload is the parsed body, which the CEL environment reads as `payload`. Untrusted from end
	// to end: it is data under one name and is never rendered as an instruction to anything
	// (ai-first.md §4).
	Payload map[string]any
}

// Execute starts the rule the address names, and answers the run it will produce.
//
// Every refusal is the same one. An unknown address, a rotated one, a rule that has been deleted, a
// rule that is switched off and a rule whose trigger is no longer an inbound webhook all answer not
// found, with the same body: distinguishing them would answer questions for whoever is trying
// tokens (T-21, the discipline D-08 established).
func (h StartInboundRun) Execute(
	ctx context.Context, delivery InboundDelivery,
) (TriggerRuleResult, error) {
	if delivery.Token.IsZero() {
		return TriggerRuleResult{}, inboundGone()
	}
	if delivery.Payload == nil {
		// An empty document rather than a missing one, so a condition naming `payload` sees
		// something rather than failing to resolve.
		delivery.Payload = map[string]any{}
	}

	// The tenant the token names, and the only source of one on a route with no authentication.
	scope := persistence.Scope{TenantID: delivery.Token.TenantID()}
	runID := h.IDs.NewID()

	var rule domain.Rule
	err := h.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, err := h.Inbound.FindByToken(ctx, delivery.Token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return inboundGone()
			}
			return err
		}
		if !found.Enabled || found.Trigger.Kind != domain.TriggerInboundWebhook {
			return inboundGone()
		}
		rule = found

		_, err = h.Jobs.Enqueue(ctx, queue.Request{
			Kind:     queue.KindAutomationRun,
			TenantID: scope.TenantID,
			Payload: map[string]any{
				"rule_id": rule.ID.String(),
				"trigger": string(domain.TriggerInboundWebhook),
				"run_id":  runID.String(),
				// Each delivery is its own occasion, so a system that posts twice gets two runs -
				// which is what an inbound webhook means. A caller that wants the two collapsed
				// says so with the rule's own dedupe expression.
				"occasion": "inbound:" + runID.String(),
				"payload":  delivery.Payload,
			},
			DedupeKey: ConsumerName + ":inbound:" + runID.String(),
		})
		return err
	})
	if err != nil {
		return TriggerRuleResult{}, err
	}
	return TriggerRuleResult{RunID: runID, RuleID: rule.ID}, nil
}

// inboundGone is the one answer this route ever gives when it will not serve.
func inboundGone() error {
	return shared.ErrNotFound.WithDetail("automation.inbound_not_found")
}
