// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Setting up a passkey or security key during sign-in, where the workspace demands a second factor of
// somebody who holds none (identity.md §8, §23.2, UC-ID-22 check 8). The ENROLL credential is the
// proof, as for the app: it exists only where no factor is held, so no step-up is asked - there is
// nothing to step up with. Everything else is adding a key: the same verifier, limit, codes, audit and
// notice. It ends as confirming the app does: the session, or the step still owed.

const (
	StartPasskeyEnrollmentName    = "StartPasskeyEnrollment"
	CompletePasskeyEnrollmentName = "CompletePasskeyEnrollment"
)

// StartPasskeyEnrollment answers the options for the ceremony.
type StartPasskeyEnrollment struct{ Writer PasskeyWriter }

// Execute issues a challenge carrying the handle the new credential will hold: five minutes, single
// use, for the account the ENROLL credential names.
func (h StartPasskeyEnrollment) Execute(
	ctx context.Context, pendingToken secret.Secret, host string,
) (PasskeyRegistrationOptions, error) {
	w := h.Writer
	s := w.Session
	if s.KeyProofs == nil {
		return PasskeyRegistrationOptions{}, shared.ErrConflict.WithDetail(codeNoRelyingParty)
	}
	caller, err := s.resolveMfaCaller(ctx, appshared.ActorContext{}, pendingToken)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	if caller.account.Kind != domain.AccountUser {
		return PasskeyRegistrationOptions{}, shared.ErrForbidden.WithDetail(codePersonOnly)
	}
	challenge, err := s.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return PasskeyRegistrationOptions{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	minted, err := s.Entropy.Bytes(domain.PasskeyUserHandleBytes)
	if err != nil {
		return PasskeyRegistrationOptions{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	token, err := domain.NewPasskeyEnrollToken(caller.tenantID, challenge)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	account := caller.account
	options := PasskeyRegistrationOptions{
		UserName: account.Email, DisplayName: account.DisplayName, Challenge: challenge,
	}
	if options.UserName == "" {
		options.UserName = account.SignInName
	}
	err = s.UnitOfWork.Within(ctx, persistence.Scope{TenantID: caller.tenantID}, func(ctx context.Context) error {
		party, ok, err := s.keyPartyAt(ctx, host)
		if err != nil {
			return err
		}
		if !ok {
			// A browser makes a key only for the host it is on: at another, the app is the way.
			return shared.ErrConflict.WithDetail(codeNoRelyingParty)
		}
		options.RelyingParty, options.PartyName = party.RelyingParty, party.Name
		if err := w.Passkeys.LockAccount(ctx, account.ID); err != nil {
			return err
		}
		held, err := w.Passkeys.ListOf(ctx, account.ID)
		if err != nil {
			return err
		}
		if len(held) >= domain.MaxPasskeys {
			return shared.ErrConflict.WithDetail(codeLimitReached).WithParams(limitParams())
		}
		options.UserHandle = minted
		if len(held) > 0 {
			options.UserHandle = held[0].UserHandle
		}
		options.Exclude = held
		now := s.Clock.Now()
		options.ExpiresAt = now.Add(domain.PasskeyRegistrationLifetime)
		if err := s.KeyProofs.Proofs.Issue(ctx, domain.PasskeyProofChallenge{
			ID: s.IDs.NewID(), Purpose: domain.PendingPasskeyEnroll, AccountID: account.ID,
			UserHandle: options.UserHandle, CreatedAt: now, ExpiresAt: options.ExpiresAt,
		}, token); err != nil {
			return err
		}
		return s.recordMfaAudit(ctx, PasskeyRegistrationStartedAction, audit.SeverityInfo, account, now)
	})
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	return options, nil
}

// CompletePasskeyEnrollmentCommand is the browser's answer, decoded, the name, and the credential.
type CompletePasskeyEnrollmentCommand struct {
	PendingToken secret.Secret
	CompletePasskeyRegistrationCommand
}

// EnrolledPasskey is the key now held, the codes where minted, and what the sign-in does next.
type EnrolledPasskey struct {
	AddedPasskey
	Result SignInResult
}

// CompletePasskeyEnrollment keeps the key and finishes the sign-in.
type CompletePasskeyEnrollment struct{ Writer PasskeyWriter }

// Execute spends the challenge in a transaction of its own, has the verifier check the ceremony, and
// keeps the credential under the account's lock - refused there where a factor appeared meanwhile,
// since the ENROLL credential was minted for an account without one.
func (h CompletePasskeyEnrollment) Execute(
	ctx context.Context, cmd CompletePasskeyEnrollmentCommand,
) (EnrolledPasskey, error) {
	w := h.Writer
	s := w.Session
	if s.KeyProofs == nil {
		return EnrolledPasskey{}, shared.ErrConflict.WithDetail(codeNoRelyingParty)
	}
	caller, err := s.resolveMfaCaller(ctx, appshared.ActorContext{}, cmd.PendingToken)
	if err != nil {
		return EnrolledPasskey{}, err
	}
	account := caller.account
	name, err := domain.PasskeyName(cmd.Name)
	if err != nil {
		return EnrolledPasskey{}, err
	}
	challenge, err := w.Verifier.ChallengeOf(cmd.ClientDataJSON)
	if err != nil {
		return EnrolledPasskey{}, err
	}
	token, err := domain.NewPasskeyEnrollToken(caller.tenantID, challenge)
	if err != nil {
		return EnrolledPasskey{}, shared.ErrConflict.WithDetail(codeChallengeStale)
	}
	scope := persistence.Scope{TenantID: caller.tenantID}

	var spent domain.PasskeyProofChallenge
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, ok, err := s.KeyProofs.Proofs.Spend(ctx, domain.PendingPasskeyEnroll, token, account.ID, "", s.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			return shared.ErrConflict.WithDetail(codeChallengeStale)
		}
		spent = found
		return nil
	}); err != nil {
		return EnrolledPasskey{}, err
	}

	var party relyingParty
	if err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		found, ok, err := w.relyingParty(ctx)
		if err != nil {
			return err
		}
		if !ok {
			return shared.ErrConflict.WithDetail(codeNoRelyingParty)
		}
		party = found
		return nil
	}); err != nil {
		return EnrolledPasskey{}, err
	}
	verified, err := w.Verifier.VerifyRegistration(passkey.Registration{
		ClientDataJSON: cmd.ClientDataJSON, AttestationObject: cmd.AttestationObject, RawID: cmd.RawID,
		Transports: cmd.Transports, Challenge: challenge, RelyingParty: party.RelyingParty,
	})
	if err != nil {
		return EnrolledPasskey{}, err
	}
	codeMaterial, err := s.Entropy.Bytes(domain.RecoveryCodeCount * domain.RecoveryCodeBytes)
	if err != nil {
		return EnrolledPasskey{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	codes, err := domain.NewRecoveryCodes(codeMaterial)
	if err != nil {
		return EnrolledPasskey{}, err
	}

	var enrolled EnrolledPasskey
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := w.Passkeys.LockAccount(ctx, account.ID); err != nil {
			return err
		}
		factor, _, err := secondFactorOf(ctx, s.Enrollments, s.Keys, account.ID)
		if err != nil {
			return err
		}
		if factor.App || factor.Keys > 0 {
			return shared.ErrConflict.WithDetail("auth.mfa_already_armed")
		}
		count, err := w.Passkeys.CountOf(ctx, account.ID)
		if err != nil {
			return err
		}
		if count >= domain.MaxPasskeys {
			return shared.ErrConflict.WithDetail(codeLimitReached).WithParams(limitParams())
		}
		remaining, err := s.Recovery.Remaining(ctx, account.ID)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		credential := domain.PasskeyCredential{
			ID: s.IDs.NewID(), TenantID: caller.tenantID, AccountID: account.ID,
			CredentialID: verified.ID, PublicKey: verified.PublicKey, Algorithm: verified.Algorithm,
			SignCount: verified.SignCount, UserHandle: spent.UserHandle,
			BackupEligible: verified.BackupEligible, BackedUp: verified.BackedUp,
			Transports: verified.Transports, Name: name, CreatedAt: now, RPID: party.ID,
		}
		written, err := w.Passkeys.Insert(ctx, credential)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.WithDetail(codeAlreadyRegistered)
		}
		enrolled.Passkey = ownPasskeyOf(credential)
		if remaining == 0 {
			ids := make([]shared.ID, 0, len(codes))
			for range codes {
				ids = append(ids, s.IDs.NewID())
			}
			if err := s.Recovery.Replace(ctx, account.ID, ids, codes, now); err != nil {
				return err
			}
			for _, code := range codes {
				enrolled.RecoveryCodes = append(enrolled.RecoveryCodes, secret.New(code))
			}
		}
		consumed, err := s.Pending.Consume(ctx, caller.pending.ID, now)
		if err != nil {
			return err
		}
		if !consumed {
			return challengeRefused()
		}
		if err := s.recordMfaAudit(ctx, PasskeyAddedAction, audit.SeverityNotice, account, now); err != nil {
			return err
		}
		return w.told(ctx, appshared.ActorContext{TenantID: caller.tenantID, AccountID: account.ID},
			domain.WayInPasskeyAdded, now)
	})
	if err != nil {
		return EnrolledPasskey{}, err
	}

	// The key just made is this sign-in's second factor; the session, or the change step's, says so.
	if caller.pending.PasswordChangeOwed {
		owed := caller.pending
		owed.SignedInWith = domain.SignedInWithPasswordPasskey
		challenge, err := s.owedChange(ctx, scope, account, owed)
		if err != nil {
			return EnrolledPasskey{}, err
		}
		enrolled.Result = SignInResult{Challenge: challenge}
		return enrolled, nil
	}
	bounds, err := s.sessionBounds(ctx, caller.tenantID, account)
	if err != nil {
		return EnrolledPasskey{}, err
	}
	enrolled.Result, err = s.openOrAgree(ctx, scope, account, SessionHint{
		UserAgent: caller.pending.UserAgent, IPClass: caller.pending.IPClass,
		Method: domain.SignedInWithPasswordPasskey,
	}, bounds)
	if err != nil {
		return EnrolledPasskey{}, err
	}
	return enrolled, nil
}

// Descriptor is the catalogue entry.
func (h StartPasskeyEnrollment) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartPasskeyEnrollmentName,
		Summary: "Begins setting up a passkey or a security key during sign-in, where the workspace " +
			"demands a second factor of somebody who holds none: the ENROLL step's credential is the " +
			"proof, and the answer is the options for the browser's ceremony. Only where the request " +
			"arrived at the workspace's own host.",
		SideEffects: "Writes a short-lived challenge and an audit entry.",
		Input: []usecase.Field{
			{Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The ENROLL step's credential."},
			hostField,
		},
		Audit: usecase.AuditDeclaration{
			Action: PasskeyRegistrationStartedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, _ appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			options, err := h.Execute(ctx, secret.New(in.String("pending_token")), in.String("host"))
			if err != nil {
				return nil, err
			}
			return registrationOptionsOutput(options), nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h CompletePasskeyEnrollment) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompletePasskeyEnrollmentName,
		Summary: "Finishes setting up a passkey or a security key during sign-in: the ceremony checked " +
			"against the challenge the ENROLL step started, the key kept, ten recovery codes shown once " +
			"where the account held none, and - the second factor proved - the session, or the step " +
			"still owed.",
		SideEffects: "Spends the challenge and the ENROLL credential; keeps the credential; may replace " +
			"the recovery codes; opens a session; writes audit entries; tells the person of the new way in.",
		Input: []usecase.Field{
			{Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The ENROLL step's credential. It dies on use."},
			{Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What the person calls it: 1 to 64 characters."},
			{Name: "raw_id", Kind: usecase.KindString, Required: true,
				Description: "The credential's rawId, base64url without padding."},
			{Name: "client_data_json", Kind: usecase.KindString, Required: true,
				Description: "The response's clientDataJSON, base64url without padding."},
			{Name: "attestation_object", Kind: usecase.KindString, Required: true,
				Description: "The response's attestationObject, base64url without padding."},
			{Name: "transports", Kind: usecase.KindList,
				Description: "The response's getTransports(), where the browser offers it."},
		},
		Audit: usecase.AuditDeclaration{
			Action: PasskeyAddedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CompletePasskeyEnrollment) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	transports, err := in.StringList("transports")
	if err != nil {
		return nil, err
	}
	cmd := CompletePasskeyEnrollmentCommand{PendingToken: secret.New(in.String("pending_token"))}
	cmd.Name, cmd.Transports = in.String("name"), transports
	for _, field := range []struct {
		name string
		into *[]byte
	}{
		{"raw_id", &cmd.RawID}, {"client_data_json", &cmd.ClientDataJSON},
		{"attestation_object", &cmd.AttestationObject},
	} {
		decoded, err := base64.RawURLEncoding.DecodeString(in.String(field.name))
		if err != nil {
			return nil, shared.ErrValidation.WithDetail(passkey.CodeRegistrationInvalid)
		}
		*field.into = decoded
	}
	enrolled, err := h.Execute(ctx, cmd)
	if err != nil {
		return nil, err
	}
	out := usecase.Output{"passkey": ownPasskeyOutput(enrolled.Passkey)}
	if len(enrolled.RecoveryCodes) > 0 {
		codes := make([]any, 0, len(enrolled.RecoveryCodes))
		for _, code := range enrolled.RecoveryCodes {
			codes = append(codes, code.Reveal())
		}
		out["recovery_codes"] = codes
	}
	if enrolled.Result.Pair != nil {
		out["tokens"] = pairOutput(*enrolled.Result.Pair)
	}
	if enrolled.Result.Challenge != nil {
		out["challenge"] = challengeOutput(*enrolled.Result.Challenge)
	}
	return out, nil
}
