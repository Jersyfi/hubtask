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
	"github.com/Jersyfi/hubtask/core/port/stepup"
)

// Renaming and removing one's own passkeys (identity.md §23.2, UC-ID-22 checks 2, 4, 5).

const (
	RenameOwnPasskeyName = "RenameOwnPasskey"
	RemoveOwnPasskeyName = "RemoveOwnPasskey"
)

// PasskeyRemovedAction is a way into the account gone (identity.md §23.2): the act and its moment,
// never the name (rule 10).
const PasskeyRemovedAction audit.Action = "auth.passkey_removed" //nolint:gosec // G101: an audit code

const (
	codePasskeyNotFound = "auth.passkey_not_found" //nolint:gosec // G101: a message code
	codeFactorRequired  = "auth.mfa_required_by_tenant"
)

// PasskeyRenamedAction is a name changed: the act and its moment, never the old or the new name
// (rule 10). Info: a name opens nothing.
const PasskeyRenamedAction audit.Action = "auth.passkey_renamed" //nolint:gosec // G101: an audit code

// RenameOwnPasskey changes the name a person gave one of their passkeys.
type RenameOwnPasskey struct{ Writer PasskeyWriter }

// Execute renames. The session alone is enough, or a token that may write its holder's account: a
// name opens nothing, so no fresh proof is asked (identity.md §23.2).
func (h RenameOwnPasskey) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, raw string,
) (OwnPasskey, error) {
	if err := passkeyPerson(actor); err != nil {
		return OwnPasskey{}, err
	}
	if err := actor.RequireScope(accountRead); err != nil {
		return OwnPasskey{}, err
	}
	name, err := domain.PasskeyName(raw)
	if err != nil {
		return OwnPasskey{}, err
	}
	s := h.Writer.Session
	var renamed OwnPasskey
	err = s.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		credential, err := h.Writer.Passkeys.Rename(ctx, actor.AccountID, id, name)
		if err != nil {
			return err
		}
		renamed = ownPasskeyOf(credential)
		return s.recordMfaAudit(ctx, PasskeyRenamedAction, audit.SeverityInfo, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, s.Clock.Now())
	})
	if err != nil {
		return OwnPasskey{}, err
	}
	return renamed, nil
}

// Descriptor is the catalogue entry.
func (h RenameOwnPasskey) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RenameOwnPasskeyName,
		Summary: "Changes the name the caller gave one of their own passkeys or security keys, and " +
			"nothing else. No fresh proof: a name opens nothing.",
		SideEffects: "Changes the name; writes an audit entry without it.",
		TokenScope:  accountRead,
		Input: []usecase.Field{
			{Name: "passkey_id", Kind: usecase.KindID, Required: true,
				Description: "The passkey, as the list names it."},
			{Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What the person calls it: 1 to 64 characters."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PasskeyRenamedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			id, err := in.ID("passkey_id")
			if err != nil {
				return nil, err
			}
			renamed, err := h.Execute(ctx, actor, id, in.String("name"))
			if err != nil {
				return nil, err
			}
			return usecase.Output{"passkey": ownPasskeyOutput(renamed)}, nil
		}),
	}
}

// keys is the writer's passkeys as the factor rules count them: those of the workspace's address.
func (w PasskeyWriter) keys() PasskeyKeys {
	return PasskeyKeys{Passkeys: w.Passkeys, Party: w.party()}
}

// removal is what removing one of the account's passkeys is decided on.
type removal struct {
	factor   domain.SecondFactor
	required bool
	// target is the passkey asked about; zero where the question is about any of them.
	target  domain.PasskeyCredential
	current bool
}

// refusal is the rule's answer: the last factor the workspace requires of this person stays. A
// passkey of another address is no factor, and goes whatever the rule says (P-16).
func (r removal) refusal() error {
	if r.current && !r.factor.KeyRemovable(r.required) {
		return shared.ErrForbidden.WithDetail(codeFactorRequired)
	}
	return nil
}

// decideRemoval reads, inside the caller's transaction, what the account holds, whether the rule
// demands a factor of the person, and the passkey asked about - ErrNotFound where it is not theirs.
func (w PasskeyWriter) decideRemoval(
	ctx context.Context, actor appshared.ActorContext, verdict SignInVerdict, id shared.ID,
) (removal, error) {
	s := w.Session
	factor, _, err := secondFactorOf(ctx, s.Enrollments, w.keys(), actor.AccountID)
	if err != nil {
		return removal{}, err
	}
	required, err := s.factorDemandedOf(ctx, ownAccount(actor), verdict)
	if err != nil {
		return removal{}, err
	}
	decided := removal{factor: factor, required: required, current: true}
	if id.IsZero() {
		return decided, nil
	}
	held, err := w.Passkeys.ListOf(ctx, actor.AccountID)
	if err != nil {
		return removal{}, err
	}
	party, ok, err := w.relyingParty(ctx)
	if err != nil {
		return removal{}, err
	}
	for _, credential := range held {
		if credential.ID == id {
			decided.target = credential
			decided.current = ok && credential.BelongsTo(party.ID)
			return decided, nil
		}
	}
	return removal{}, shared.ErrNotFound.WithDetail(codePasskeyNotFound)
}

// RemoveOwnPasskey removes one of the caller's passkeys, behind a fresh proof, at once.
type RemoveOwnPasskey struct{ Writer PasskeyWriter }

// Execute removes.
//
// What would refuse anyway - not the caller's, the last factor the rule requires - is asked before
// the proof, so a refusal does not spend it, and asked again under the account's lock, which turning
// the app off takes as well: two removals of the last two factors are decided one after the other.
func (h RemoveOwnPasskey) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID, stepUpToken string,
) error {
	if err := passkeyPerson(actor); err != nil {
		return err
	}
	w := h.Writer
	s := w.Session
	if _, err := s.stepUpSession(ctx, actor); err != nil {
		return err
	}
	verdict, err := s.ownVerdict(ctx, actor)
	if err != nil {
		return err
	}
	scope := actor.PersistenceScope()
	if err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		decided, err := w.decideRemoval(ctx, actor, verdict, id)
		if err != nil {
			return err
		}
		return decided.refusal()
	}); err != nil {
		return err
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return err
	}

	return s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := w.Passkeys.LockAccount(ctx, actor.AccountID); err != nil {
			return err
		}
		decided, err := w.decideRemoval(ctx, actor, verdict, id)
		if err != nil {
			return err
		}
		if err := decided.refusal(); err != nil {
			return err
		}
		removed, err := w.Passkeys.Remove(ctx, actor.AccountID, id)
		if err != nil {
			return err
		}
		if !removed {
			return shared.ErrNotFound.WithDetail(codePasskeyNotFound)
		}
		// With the last factor of either kind the codes go; beside the app or another key they stay.
		if err := s.Recovery.DiscardUnbacked(ctx, actor.AccountID); err != nil {
			return err
		}
		return s.recordMfaAudit(ctx, PasskeyRemovedAction, audit.SeverityNotice, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, s.Clock.Now())
	})
}

// Descriptor is the catalogue entry.
func (h RemoveOwnPasskey) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RemoveOwnPasskeyName,
		Summary: "Removes one of the caller's own passkeys or security keys, behind a fresh proof; it " +
			"stops working at once. Refused before the proof where it is the last second factor the " +
			"account holds and the workspace's rule requires one of this person. With the last " +
			"factor of any kind, the recovery codes go too.",
		SideEffects: "Consumes the step-up; deletes the credential, and the recovery codes where no " +
			"factor remains; writes an audit entry without the name.",
		Destructive: true,
		Input: []usecase.Field{
			{Name: "passkey_id", Kind: usecase.KindID, Required: true,
				Description: "The passkey, as the list names it."},
			{
				// Declared but not required: a caller without a proof needs the 403 that names the
				// methods rather than a 422 about a missing field.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof this demands. It is consumed by this call.",
			},
		},
		StepUp: "removing a passkey or a security key",
		Audit: usecase.AuditDeclaration{
			Action: PasskeyRemovedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			id, err := in.ID("passkey_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, id, in.String("step_up_token")); err != nil {
				return nil, err
			}
			return usecase.Output{}, nil
		}),
	}
}
