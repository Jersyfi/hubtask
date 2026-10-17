// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"
	"errors"
	"slices"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// Signing in with a passkey alone (identity.md §23.4, UC-ID-21): the device offers the passkeys it
// holds for this workspace, the person confirms with a fingerprint, a face or the device's PIN, and
// the verified assertion is the whole proof - no password and no second factor, also where the
// workspace requires one (#1521's decision, §23.1). The card asks for the options when the address
// field gains focus or the button is pressed; the verifier checks the ceremony, the core decides.

const (
	StartPasskeySignInName    = "StartPasskeySignIn"
	CompletePasskeySignInName = "CompletePasskeySignIn"
)

// PasskeySignedInAction is a session a passkey opened - the act, the account, never the credential.
const PasskeySignedInAction audit.Action = "auth.passkey_signed_in" //nolint:gosec // G101: an audit code

// codeNotOffered is the refusal where the workspace's rules do not offer passkeys as a way in.
const codeNotOffered = "auth.passkey_not_offered"

// PasskeySignInWriter is what the two use cases share.
type PasskeySignInWriter struct {
	Session SessionWriter
	SignIns repository.PasskeySignIns
	// Verifier checks the ceremony; Party names the workspace's relying party.
	Verifier passkey.Verifier
	Party    PasskeyParty
	// Resolver reads the rule in force, whose `methods` decide whether passkeys are a way in here.
	Resolver interface {
		Resolve(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error)
	}
}

// PasskeySignInRequest is where a sign-in arrived: the workspace's sources, the host, the device.
type PasskeySignInRequest struct {
	TenantSlug   string
	TenantHeader string
	Host         string
	UserAgent    string
	RemoteAddr   string
}

// offeredAt resolves the workspace and answers its relying party, refusing where passkeys are no way
// in here: switched off, no public address, or a host that is not the relying party's (P-05).
func (w PasskeySignInWriter) offeredAt(
	ctx context.Context, request PasskeySignInRequest,
) (shared.ID, relyingParty, error) {
	s := w.Session
	tenantID, err := s.resolveTenant(ctx, request.TenantSlug, request.TenantHeader)
	if err != nil {
		return "", relyingParty{}, err
	}
	resolved, err := w.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return "", relyingParty{}, err
	}
	if !slices.Contains(resolved.Effective.Policy.Methods, domain.MethodPasskey) {
		return "", relyingParty{}, shared.ErrForbidden.WithDetail(codeNotOffered)
	}
	var party relyingParty
	var ok bool
	if err := s.UnitOfWork.WithinReadOnly(ctx, appshared.ActorContext{TenantID: tenantID}.PersistenceScope(),
		func(ctx context.Context) error {
			party, ok, err = w.Party.of(ctx)
			return err
		}); err != nil {
		return "", relyingParty{}, err
	}
	if !ok || !party.servesAt(request.Host) {
		return "", relyingParty{}, shared.ErrConflict.WithDetail(codeNoRelyingParty)
	}
	return tenantID, party, nil
}

// PasskeySignInOptions is what the browser's ceremony is asked with.
type PasskeySignInOptions struct {
	RPID      string
	Challenge []byte
	ExpiresAt time.Time
}

// StartPasskeySignIn answers the options for signing in with a passkey.
type StartPasskeySignIn struct{ Writer PasskeySignInWriter }

// Execute issues a challenge that names nobody, five minutes, single use. The workspace's retention
// sweep is seeded here, so expired challenges of a workspace nobody signs in to still go.
func (h StartPasskeySignIn) Execute(
	ctx context.Context, request PasskeySignInRequest,
) (PasskeySignInOptions, error) {
	w := h.Writer
	s := w.Session
	tenantID, party, err := w.offeredAt(ctx, request)
	if err != nil {
		return PasskeySignInOptions{}, err
	}
	challenge, err := s.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return PasskeySignInOptions{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	token, err := domain.NewPasskeySignInToken(tenantID, challenge)
	if err != nil {
		return PasskeySignInOptions{}, err
	}
	now := s.Clock.Now()
	pending := domain.PendingCredential{
		ID: s.IDs.NewID(), TenantID: tenantID, Purpose: domain.PendingPasskeySignIn,
		UserAgent: request.UserAgent, IPClass: domain.IPClass(request.RemoteAddr),
		CreatedAt: now.UTC(), ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
	}
	scope := appshared.ActorContext{TenantID: tenantID}.PersistenceScope()
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		if err := w.SignIns.IssueChallenge(ctx, pending, token); err != nil {
			return err
		}
		if err := s.seedRetention(ctx, tenantID, now); err != nil {
			return err
		}
		// Somebody began a passkey sign-in for this workspace: nobody is known yet, so the system
		// records it, as a provider's sign-in's start is (identity.md §20).
		return s.Audit.Append(ctx, audit.Entry{
			TenantID: tenantID, OccurredAt: now, Action: PasskeySignInStartedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo, ActorKind: shared.ActorSystem,
			TargetType: workspaceTarget, TargetID: tenantID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}); err != nil {
		return PasskeySignInOptions{}, err
	}
	return PasskeySignInOptions{RPID: party.ID, Challenge: challenge, ExpiresAt: pending.ExpiresAt}, nil
}

// PasskeySignInCommand is the browser's answer, decoded, and where it arrived.
type PasskeySignInCommand struct {
	Request           PasskeySignInRequest
	RawID             []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
	UserHandle        []byte
}

// CompletePasskeySignIn opens the session a verified passkey proves.
type CompletePasskeySignIn struct{ Writer PasskeySignInWriter }

// Execute judges the ceremony and opens the session, or answers the step still owed.
//
// Every refusal is the sign-in's one answer and counts on the network's subject, in its own
// transaction after the refusing one (§4.3): an unknown or removed passkey, one made at another
// address, an account that cannot act, a device that did not verify the person, a signature or a
// counter that does not hold. No subject per account: nobody typed one, and a subject keyed on the
// credential would let anyone who learnt its identifier lock its owner out. A challenge that is
// stale is not a guess and counts nothing - a tab left open answers it, and the card asks again.
func (h CompletePasskeySignIn) Execute(ctx context.Context, cmd PasskeySignInCommand) (SignInResult, error) {
	w := h.Writer
	s := w.Session
	tenantID, party, err := w.offeredAt(ctx, cmd.Request)
	if err != nil {
		return SignInResult{}, err
	}
	scope := appshared.ActorContext{TenantID: tenantID}.PersistenceScope()
	var subjects []string
	if class := domain.IPClass(cmd.Request.RemoteAddr); class != "" {
		subjects = append(subjects, "ip:"+class)
	}
	refuse := func(cause error) (SignInResult, error) {
		s.failure(ctx, FailureWrongCredential)
		return SignInResult{}, errors.Join(s.recordFailure(ctx, scope, subjects),
			shared.ErrUnauthenticated.WithDetail(passkey.CodeSignInFailed).WithCause(cause))
	}

	if err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		return s.checkLocked(ctx, subjects, s.Clock.Now())
	}); err != nil {
		return SignInResult{}, err
	}

	challenge, err := w.Verifier.ChallengeOf(cmd.ClientDataJSON)
	if err != nil {
		return refuse(err)
	}
	token, err := domain.NewPasskeySignInToken(tenantID, challenge)
	if err != nil {
		return SignInResult{}, shared.ErrConflict.WithDetail(codeChallengeStale)
	}
	// Spent first, in its own transaction: whatever follows, this challenge has had its one try.
	var spent bool
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		var err error
		spent, err = w.SignIns.SpendChallenge(ctx, token, s.Clock.Now())
		return err
	}); err != nil {
		return SignInResult{}, err
	}
	if !spent {
		return SignInResult{}, shared.ErrConflict.WithDetail(codeChallengeStale)
	}

	var holder repository.PasskeyHolder
	err = s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		holder, err = w.SignIns.HolderOf(ctx, cmd.RawID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		// Unknown, removed, or another workspace's - row security answers the same for all three.
		return refuse(err)
	}
	if err != nil {
		return SignInResult{}, err
	}
	credential := holder.Credential
	if !credential.BelongsTo(party.ID) || string(cmd.UserHandle) != string(credential.UserHandle) {
		return refuse(errors.New("passkey: another address's credential, or another handle"))
	}

	asserted, err := w.Verifier.VerifyAssertion(passkey.Assertion{
		ClientDataJSON: cmd.ClientDataJSON, AuthenticatorData: cmd.AuthenticatorData,
		Signature: cmd.Signature, RawID: cmd.RawID, Challenge: challenge,
		RelyingParty: party.RelyingParty, CredentialID: credential.CredentialID,
		PublicKey: credential.PublicKey, Algorithm: credential.Algorithm,
		BackupEligible: credential.BackupEligible,
	})
	if err != nil {
		return refuse(err)
	}
	// This door opens the account alone, so it asks for the person verified on the device (P-02);
	// a key that proves presence only is the second factor after the password, never a way in.
	if !asserted.UserVerified {
		return refuse(errors.New("passkey: the device did not verify the person"))
	}
	account := holder.Account
	if account.Kind != domain.AccountUser || account.Verify() != nil {
		return refuse(errors.New("passkey: an account that cannot sign in"))
	}
	// The workspace's own state keeps the sentence it has at every door (UC-INS-08 check 2): the
	// signature has proved the person, so saying the workspace is paused tells nobody else anything.
	if err := holder.TenantStatus.Verify(); err != nil {
		return SignInResult{}, err
	}

	var marked bool
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		var err error
		marked, err = w.SignIns.MarkUsed(ctx, credential.ID, asserted.SignCount, asserted.BackedUp, s.Clock.Now())
		return err
	}); err != nil {
		return SignInResult{}, err
	}
	if !marked {
		// The counter went back: a cloned authenticator, or a replay (identity.md §23.3).
		return refuse(errors.New("passkey: the counter did not rise"))
	}

	// A start password Hubtask drew is never kept (identity.md §13): the change step follows, and the
	// session it opens says it was a passkey that proved the person.
	if !holder.StartPasswordAt.IsZero() {
		challenge, err := s.owedChange(ctx, scope, account, domain.PendingCredential{
			UserAgent: cmd.Request.UserAgent, IPClass: domain.IPClass(cmd.Request.RemoteAddr),
			SignedInWith: domain.SignedInWithPasskey,
		})
		if err != nil {
			return SignInResult{}, err
		}
		challenge.Email = nameOf(account)
		return SignInResult{Challenge: challenge}, nil
	}

	bounds, err := s.sessionBounds(ctx, tenantID, account)
	if err != nil {
		return SignInResult{}, err
	}
	result, err := s.openOrAgree(ctx, scope, account, SessionHint{
		UserAgent: cmd.Request.UserAgent, RemoteAddr: cmd.Request.RemoteAddr,
		Method: domain.SignedInWithPasskey, Subjects: subjects,
	}, bounds)
	if err == nil && result.Challenge != nil {
		// Nobody typed a name: the TERMS step names the account (UC-ID-02 check 1's reasoning).
		result.Challenge.Email = nameOf(account)
	}
	return result, err
}

// nameOf is how a card names an account nobody typed: its address, or a managed account's sign-in
// name.
func nameOf(account domain.Account) string {
	if account.Email != "" {
		return account.Email
	}
	return account.SignInName
}

// passkeySignInFields are the request's inputs, which only the adapter has.
var passkeySignInFields = []usecase.Field{
	{Name: "host", Kind: usecase.KindString,
		Description: "The host the request arrived at: a ceremony is only offered at the relying party's."},
	{Name: "tenant_slug", Kind: usecase.KindString,
		Description: "The subdomain the request arrived under, in multi mode."},
	{Name: "tenant_header", Kind: usecase.KindString,
		Description: "The X-Hubtask-Tenant header, when sent. It may confirm the resolution, never overrule it."},
	{Name: "user_agent", Kind: usecase.KindString,
		Description: "The client as it introduced itself; recorded on the session as a hint."},
	{Name: "remote_addr", Kind: usecase.KindString,
		Description: "The peer's address as the adapter saw it; only its network class is recorded."},
}

func requestOf(in usecase.Input) PasskeySignInRequest {
	return PasskeySignInRequest{
		TenantSlug: in.String("tenant_slug"), TenantHeader: in.String("tenant_header"),
		Host: in.String("host"), UserAgent: in.String("user_agent"), RemoteAddr: in.String("remote_addr"),
	}
}

// PasskeySignInStartedAction is a passkey sign-in's start, recorded by the system for the workspace:
// the challenge names nobody.
const PasskeySignInStartedAction audit.Action = "auth.passkey_sign_in_started" //nolint:gosec // G101: an audit code

// Descriptor is the catalogue entry.
func (h StartPasskeySignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartPasskeySignInName,
		Summary: "Answers the options for signing in to the workspace the request's host names with a " +
			"passkey and no identifier: the relying party, a single-use challenge that lives five " +
			"minutes, and user verification required. Refused where the workspace's rules do not " +
			"offer passkeys, or where the request did not arrive at the workspace's own host.",
		SideEffects: "Writes a short-lived challenge that names nobody and an audit entry.",
		Input:       passkeySignInFields,
		Audit: usecase.AuditDeclaration{
			Action: PasskeySignInStartedAction, TargetType: workspaceTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, _ appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			options, err := h.Execute(ctx, requestOf(in))
			if err != nil {
				return nil, err
			}
			return usecase.Output{
				"public_key": usecase.Output{
					"rpId":             options.RPID,
					"challenge":        base64.RawURLEncoding.EncodeToString(options.Challenge),
					"timeout":          int(domain.PendingLifetime / time.Millisecond),
					"userVerification": "required",
				},
				"expires_at": options.ExpiresAt.UTC(),
			}, nil
		}),
	}
}

// Descriptor is the catalogue entry.
func (h CompletePasskeySignIn) Descriptor() usecase.Descriptor {
	fields := append([]usecase.Field{
		{Name: "raw_id", Kind: usecase.KindString, Required: true,
			Description: "The credential's rawId, base64url without padding."},
		{Name: "client_data_json", Kind: usecase.KindString, Required: true,
			Description: "The response's clientDataJSON, base64url without padding."},
		{Name: "authenticator_data", Kind: usecase.KindString, Required: true,
			Description: "The response's authenticatorData, base64url without padding."},
		{Name: "signature", Kind: usecase.KindString, Required: true,
			Description: "The response's signature, base64url without padding."},
		{
			// Not required here: a ceremony without one has to reach the use case, which answers
			// the sign-in's one refusal and counts it, rather than a 422 that tells it apart.
			Name: "user_handle", Kind: usecase.KindString,
			Description: "The response's userHandle, base64url without padding.",
		},
	}, passkeySignInFields...)
	return usecase.Descriptor{
		Name: CompletePasskeySignInName,
		Summary: "Signs a person in with a passkey: the ceremony's answer, checked against the " +
			"challenge this workspace issued and the passkey it holds, with the person verified on the " +
			"device. A verified passkey is the whole proof - no second factor is asked. Every refusal " +
			"is the sign-in's one answer and counts against the attempt ledger; a challenge that ran " +
			"out is refused apart, uncounted, so the client asks for new options.",
		SideEffects: "Spends the challenge; records the passkey's use; opens a session or a step still " +
			"owed; writes an audit entry; advances the attempt ledger on failure.",
		Input: fields,
		Audit: usecase.AuditDeclaration{
			Action: PasskeySignedInAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, _ appshared.ActorContext, in usecase.Input,
		) (usecase.Output, error) {
			cmd := PasskeySignInCommand{Request: requestOf(in)}
			for _, field := range []struct {
				name string
				into *[]byte
			}{
				{"raw_id", &cmd.RawID}, {"client_data_json", &cmd.ClientDataJSON},
				{"authenticator_data", &cmd.AuthenticatorData}, {"signature", &cmd.Signature},
				{"user_handle", &cmd.UserHandle},
			} {
				decoded, err := base64.RawURLEncoding.DecodeString(in.String(field.name))
				if err != nil {
					return nil, domain.ErrSignInFailed()
				}
				*field.into = decoded
			}
			result, err := h.Execute(ctx, cmd)
			if err != nil {
				return nil, err
			}
			if result.Challenge != nil {
				return challengeOutput(*result.Challenge), nil
			}
			return pairOutput(*result.Pair), nil
		}),
	}
}
