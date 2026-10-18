// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// A fresh proof with a passkey or security key (identity.md §16.1, §23.2): the options call issues a
// challenge bound to the session that asked, and StepUp takes the key's answer to it. The step-up's
// methods cannot see the host a prompt runs at, so the options call is where another host is refused,
// and the prompt then offers the other ways.

const StartPasskeyStepUpName = "StartPasskeyStepUp"

// PasskeyStepUpStartedAction is the first half, recorded as the provider's step-up start is.
const PasskeyStepUpStartedAction audit.Action = "auth.step_up_passkey_started" //nolint:gosec // G101: an audit code

// StartPasskeyStepUp answers the options for proving the caller afresh with a key.
type StartPasskeyStepUp struct{ Writer SessionWriter }

// Execute issues the challenge, five minutes, single use, bound to the session and its account.
func (h StartPasskeyStepUp) Execute(
	ctx context.Context, actor appshared.ActorContext, host string,
) (KeyOptions, error) {
	w := h.Writer
	session, err := w.stepUpSession(ctx, actor)
	if err != nil {
		return KeyOptions{}, err
	}
	var options *KeyOptions
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := w.factorHeldAt(ctx, actor.AccountID, host)
		if err != nil {
			return err
		}
		if len(held.Keys) == 0 {
			return shared.ErrConflict.WithDetail(codeNoRelyingParty)
		}
		options, err = w.issueKeyChallenge(ctx, actor.TenantID, domain.PendingPasskeyStepUp,
			actor.AccountID, session.ID, held)
		if err != nil {
			return err
		}
		return w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: w.Clock.Now(), Action: PasskeyStepUpStartedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: sessionTarget, TargetID: session.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
	if err != nil {
		return KeyOptions{}, err
	}
	return *options, nil
}

// Descriptor is the catalogue entry.
func (h StartPasskeyStepUp) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartPasskeyStepUpName,
		Summary: "Answers the options for proving the caller afresh with a passkey or security key: " +
			"the account's keys and a single-use challenge bound to this session, five minutes; " +
			"presence on the device suffices. Refused where the account holds no key that can sign at " +
			"the host the request arrived at, and for anything but a session.",
		SideEffects: "Writes a short-lived challenge bound to the session and an audit entry.",
		Input:       []usecase.Field{hostField},
		Audit: usecase.AuditDeclaration{
			Action: PasskeyStepUpStartedAction, TargetType: sessionTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A proof is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			options, err := h.Execute(ctx, actor, in.String("host"))
			if err != nil {
				return nil, err
			}
			return keyOptionsOutput(options), nil
		}),
	}
}
