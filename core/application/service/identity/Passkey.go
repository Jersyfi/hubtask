// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Passkeys and security keys on the caller's own account (identity.md §23, UC-ID-22): listed with
// whether another can be added now, and added in two calls - a step-up and the options, then the
// browser's ceremony. The verifier is an adapter behind core/port/passkey; nothing here reads a byte
// of a ceremony.

const (
	ListOwnPasskeysName             = "ListOwnPasskeys"
	StartPasskeyRegistrationName    = "StartPasskeyRegistration"
	CompletePasskeyRegistrationName = "CompletePasskeyRegistration"
)

// PasskeyAddedAction is a new way into the account (identity.md §23.2): the act and its moment,
// never the name the person gave it or the credential's identifier (rule 10).
const PasskeyAddedAction audit.Action = "auth.passkey_added" //nolint:gosec // G101: an audit code

// PasskeyRegistrationStartedAction is the first half, recorded as the authenticator app's
// enrolment start is - without the name the person will give it, which is not chosen yet anyway.
const PasskeyRegistrationStartedAction audit.Action = "auth.passkey_registration_started" //nolint:gosec // G101: an audit code

// The reasons a passkey cannot be added now, as message codes: the list answers them for the profile
// to say, and starting answers the same code as its refusal (P-05).
const (
	codeLimitReached   = "auth.passkey_limit_reached"
	codeNoRelyingParty = "auth.passkey_unavailable"
	codeStepUpNoMethod = "auth.step_up_no_method"
	codePersonOnly     = "auth.passkey_person_only"
)

// PasskeyInstallation is where the installation is reached, from its public address: what a
// workspace's relying party is derived from (identity.md §23.3). Empty Host where no public address
// is configured, and then no passkey can be bound.
type PasskeyInstallation struct {
	Host   string
	Scheme string
	Port   string
	Multi  bool
}

// PasskeyWriter is what the passkey use cases share.
type PasskeyWriter struct {
	// Session carries the transaction, the clock, the randomness, the ids, the trail, the recovery
	// codes and the pending rows.
	Session    SessionWriter
	Passkeys   repository.PasskeyCredentials
	Challenges repository.PasskeyChallenges
	Verifier   passkey.Verifier
	Workspaces repository.Workspaces
	// Hosts is the workspace's hosts, the canonical one first. Optional: a workspace without a row
	// answers at the host its slug makes (TenantHost.go).
	Hosts        repository.TenantHosts
	Installation PasskeyInstallation
	// StepUp is the fresh proof adding asks for (identity.md §23.2). Nil refuses, as everywhere.
	StepUp stepup.Verifier
	// WayIn tells the person that a passkey was added or removed (ADR-0078 §6). Nil where the
	// installation has no mail server; set in the literal that builds the writer.
	WayIn WayInNotices
}

// told queues the notice of a changed way in, inside the caller's transaction.
func (w PasskeyWriter) told(ctx context.Context, actor appshared.ActorContext, event domain.WayInEvent, at time.Time) error {
	if w.WayIn == nil {
		return nil
	}
	return w.WayIn.WayInChanged(ctx, actor.TenantID, actor.AccountID, domain.WayInChange{Event: event, At: at})
}

// relyingParty is a workspace's relying party with the name its options carry.
type relyingParty struct {
	passkey.RelyingParty
	Name string
}

// Party is the writer's relying party, derived as the factor rules derive it.
func (w PasskeyWriter) Party() PasskeyParty { return w.party() }

// party is the writer's relying party, derived as the factor rules derive it.
func (w PasskeyWriter) party() PasskeyParty {
	return PasskeyParty{
		Workspaces: w.Workspaces, Hosts: w.Hosts, Installation: w.Installation, Domains: w.Session.Domains,
	}
}

// relyingParty answers the workspace's relying party, inside a transaction bound to it; false where
// the installation has no public address.
func (w PasskeyWriter) relyingParty(ctx context.Context) (relyingParty, bool, error) {
	return w.party().of(ctx)
}

// passkeyPerson admits a person acting for themselves, and nobody else: a machine holds tokens
// (identity.md §15.2), and an automation or an agent has no device to confirm on.
func passkeyPerson(actor appshared.ActorContext) error {
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if actor.Kind != appshared.ActorUser {
		return shared.ErrForbidden.WithDetail(codePersonOnly)
	}
	return nil
}

// addability is whether another passkey can be added now, and where not, why: the one condition the
// list answers and starting enforces (P-05).
type addability struct {
	account domain.Account
	party   relyingParty
	held    []domain.PasskeyCredential
	reason  string
}

func (a addability) possible() bool { return a.reason == "" }

// addable reads the account's credentials and the relying party, inside a transaction bound to the
// workspace, and decides everything but the step-up's methods, which the caller asks outside it.
func (w PasskeyWriter) addable(ctx context.Context, actor appshared.ActorContext) (addability, error) {
	account, err := w.Session.People.Find(ctx, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	if account.Kind != domain.AccountUser {
		return addability{}, shared.ErrForbidden.WithDetail(codePersonOnly)
	}
	held, err := w.Passkeys.ListOf(ctx, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	party, ok, err := w.relyingParty(ctx)
	if err != nil {
		return addability{}, err
	}
	result := addability{account: account, party: party, held: held}
	switch {
	case !ok:
		result.reason = codeNoRelyingParty
	case len(held) >= domain.MaxPasskeys:
		result.reason = codeLimitReached
	}
	return result, nil
}

// withMethods completes the decision with the step-up's methods: an account that can give no fresh
// proof cannot add one, and the button that would ask is not drawn.
func (w PasskeyWriter) withMethods(
	ctx context.Context, actor appshared.ActorContext, decided addability,
) (addability, error) {
	if !decided.possible() || w.StepUp == nil || !w.StepUp.Available() {
		if decided.possible() {
			decided.reason = codeStepUpNoMethod
		}
		return decided, nil
	}
	methods, err := w.StepUp.Methods(ctx, actor.TenantID, actor.AccountID)
	if err != nil {
		return addability{}, err
	}
	if len(methods) == 0 {
		decided.reason = codeStepUpNoMethod
	}
	return decided, nil
}

// provable is whether the account can give the fresh proof adding and removing ask for.
func (w PasskeyWriter) provable(ctx context.Context, actor appshared.ActorContext) (bool, error) {
	if w.StepUp == nil || !w.StepUp.Available() {
		return false, nil
	}
	methods, err := w.StepUp.Methods(ctx, actor.TenantID, actor.AccountID)
	return len(methods) > 0, err
}

// refusal is the reason as starting answers it.
func (a addability) refusal() error {
	switch a.reason {
	case codeLimitReached:
		return shared.ErrConflict.WithDetail(codeLimitReached).WithParams(limitParams())
	case codeNoRelyingParty:
		return shared.ErrConflict.WithDetail(codeNoRelyingParty)
	default:
		return nil
	}
}

// limitParams is what the limit's refusal and the profile's sentence count with.
func limitParams() map[string]string { return map[string]string{"limit": "10"} }

// OwnPasskeys is the list as the profile shows it.
type OwnPasskeys struct {
	Passkeys []OwnPasskey
	CanAdd   bool
	// CannotAddReason is the message code saying why not, where CanAdd is false.
	CannotAddReason string
	// RPID is the host the workspace's passkeys belong to, empty where there is none.
	RPID string
	// CanRemove is whether removing one of the passkeys of the workspace's address would be accepted
	// now, and CannotRemoveReason the message code saying why not - the condition RemoveOwnPasskey
	// asks before the proof (P-05). One of another address always goes.
	CanRemove          bool
	CannotRemoveReason string
}

// OwnPasskey is one credential as its holder sees it: never its identifier, key or handle.
type OwnPasskey struct {
	ID         shared.ID
	Name       string
	CreatedAt  time.Time
	LastUsedAt time.Time
	// RPID is the host it was made for; empty where it was not recorded.
	RPID string
}

func ownPasskeyOf(credential domain.PasskeyCredential) OwnPasskey {
	return OwnPasskey{
		ID: credential.ID, Name: credential.Name,
		CreatedAt: credential.CreatedAt, LastUsedAt: credential.LastUsedAt, RPID: credential.RPID,
	}
}

// ListOwnPasskeys answers the caller's passkeys and whether another can be added.
type ListOwnPasskeys struct{ Writer PasskeyWriter }

// Execute reads them.
func (h ListOwnPasskeys) Execute(ctx context.Context, actor appshared.ActorContext) (OwnPasskeys, error) {
	if err := passkeyPerson(actor); err != nil {
		return OwnPasskeys{}, err
	}
	// A read of one's own account, bounded as GetOwnAccount is: a token minted for items reads no
	// list of its holder's ways in.
	if err := actor.RequireScope(accountsRead); err != nil {
		return OwnPasskeys{}, err
	}
	w := h.Writer
	verdict, err := w.Session.ownVerdict(ctx, actor)
	if err != nil {
		return OwnPasskeys{}, err
	}
	var (
		decided  addability
		removing removal
	)
	err = w.Session.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if decided, err = w.addable(ctx, actor); err != nil {
			return err
		}
		removing, err = w.decideRemoval(ctx, actor, verdict, "")
		return err
	})
	if err != nil {
		return OwnPasskeys{}, err
	}
	decided, err = w.withMethods(ctx, actor, decided)
	if err != nil {
		return OwnPasskeys{}, err
	}
	provable, err := w.provable(ctx, actor)
	if err != nil {
		return OwnPasskeys{}, err
	}
	listed := OwnPasskeys{
		Passkeys: make([]OwnPasskey, 0, len(decided.held)),
		CanAdd:   decided.possible(), CannotAddReason: decided.reason, RPID: decided.party.ID,
		CanRemove: true,
	}
	switch {
	case removing.factor.Keys > 0 && removing.refusal() != nil:
		listed.CanRemove, listed.CannotRemoveReason = false, codeFactorRequired
	case !provable:
		listed.CanRemove, listed.CannotRemoveReason = false, codeStepUpNoMethod
	}
	for _, credential := range decided.held {
		listed.Passkeys = append(listed.Passkeys, ownPasskeyOf(credential))
	}
	return listed, nil
}

// OwnPasskeysReadAction names the read for the trail's refusals.
const OwnPasskeysReadAction audit.Action = "auth.own_passkeys_read" //nolint:gosec // G101: an audit code

// Descriptor is the catalogue entry.
func (h ListOwnPasskeys) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListOwnPasskeysName,
		Summary: "Lists the passkeys and security keys of the caller's own account - each one's " +
			"name, when it was added and when it was last used, never its identifier or key - and " +
			"whether another can be added now, with the reason where not.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			// The caller's own records: nothing to refuse beyond the credential, nothing to record.
			Action: OwnPasskeysReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(func(
			ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
		) (usecase.Output, error) {
			listed, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(listed.Passkeys))
			for _, each := range listed.Passkeys {
				data = append(data, ownPasskeyOutput(each))
			}
			out := usecase.Output{
				"data": data, "limit": domain.MaxPasskeys, "can_add": listed.CanAdd, "rp_id": listed.RPID,
				"can_remove": listed.CanRemove,
			}
			if listed.CannotAddReason != "" {
				out["cannot_add_reason"] = listed.CannotAddReason
			}
			if listed.CannotRemoveReason != "" {
				out["cannot_remove_reason"] = listed.CannotRemoveReason
			}
			return out, nil
		}),
	}
}

func ownPasskeyOutput(each OwnPasskey) usecase.Output {
	out := usecase.Output{
		"id": each.ID.String(), "name": each.Name, "created_at": each.CreatedAt.UTC(),
	}
	if !each.LastUsedAt.IsZero() {
		out["last_used_at"] = each.LastUsedAt.UTC()
	}
	if each.RPID != "" {
		out["rp_id"] = each.RPID
	}
	return out
}

// PasskeyRegistrationOptions is what the browser's ceremony is asked with.
type PasskeyRegistrationOptions struct {
	RelyingParty passkey.RelyingParty
	PartyName    string
	UserHandle   []byte
	// UserName is what the browser shows to tell accounts apart: the address, or the sign-in name of
	// an account without one.
	UserName    string
	DisplayName string
	Challenge   []byte
	// Exclude is the account's credentials, so an authenticator that holds one says so rather than
	// making a second.
	Exclude   []domain.PasskeyCredential
	ExpiresAt time.Time
}

// StartPasskeyRegistration is the first half of adding a passkey: the fresh proof, and the options.
type StartPasskeyRegistration struct{ Writer PasskeyWriter }

// Execute checks the account, demands the step-up, and issues a challenge bound to the session.
//
// The step-up is asked here rather than when the ceremony's answer comes back: the person proves it
// is them, then confirms on the device (UC-ID-22 check 2), and the challenge carries the proof - bound
// to this session, five minutes, single use. What would refuse anyway is asked before the proof, so
// a refusal does not spend it.
func (h StartPasskeyRegistration) Execute(
	ctx context.Context, actor appshared.ActorContext, stepUpToken string,
) (PasskeyRegistrationOptions, error) {
	if err := passkeyPerson(actor); err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	w := h.Writer
	s := w.Session
	session, err := s.stepUpSession(ctx, actor)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	scope := actor.PersistenceScope()
	var decided addability
	if err := s.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		decided, err = w.addable(ctx, actor)
		return err
	}); err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	if refused := decided.refusal(); refused != nil {
		return PasskeyRegistrationOptions{}, refused
	}
	if err := stepup.Demand(ctx, w.StepUp, actor.TenantID, actor.AccountID, stepUpToken); err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	challenge, err := s.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return PasskeyRegistrationOptions{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	minted, err := s.Entropy.Bytes(domain.PasskeyUserHandleBytes)
	if err != nil {
		return PasskeyRegistrationOptions{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	token, err := domain.NewPasskeyChallengeToken(actor.TenantID, challenge)
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}

	options := PasskeyRegistrationOptions{
		RelyingParty: decided.party.RelyingParty, PartyName: decided.party.Name,
		UserName: decided.account.Email, DisplayName: decided.account.DisplayName, Challenge: challenge,
	}
	if options.UserName == "" {
		options.UserName = decided.account.SignInName
	}
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		// Under the account's lock: the count is read again where the proof may have taken a while,
		// and the handle is the account's own wherever it holds one.
		if err := w.Passkeys.LockAccount(ctx, actor.AccountID); err != nil {
			return err
		}
		held, err := w.Passkeys.ListOf(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if len(held) >= domain.MaxPasskeys {
			return shared.ErrConflict.WithDetail(codeLimitReached).WithParams(limitParams())
		}
		// One handle per account: the one its credentials carry, or a new one. Two starts on an
		// account without a credential cannot both finish - Issue spends the older challenge - so a
		// second handle never lands.
		options.UserHandle = minted
		if len(held) > 0 {
			options.UserHandle = held[0].UserHandle
		}
		options.Exclude = held
		now := s.Clock.Now()
		options.ExpiresAt = now.Add(domain.PasskeyRegistrationLifetime)
		if err := w.Challenges.Issue(ctx, domain.PasskeyChallenge{
			ID: s.IDs.NewID(), AccountID: actor.AccountID, SessionID: session.ID,
			UserHandle: options.UserHandle, CreatedAt: now, ExpiresAt: options.ExpiresAt,
		}, token); err != nil {
			return err
		}
		return s.recordMfaAudit(ctx, PasskeyRegistrationStartedAction, audit.SeverityInfo, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, now)
	})
	if err != nil {
		return PasskeyRegistrationOptions{}, err
	}
	return options, nil
}

// Descriptor is the catalogue entry.
func (h StartPasskeyRegistration) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: StartPasskeyRegistrationName,
		Summary: "Begins adding a passkey or a security key to the caller's own account: a fresh " +
			"proof, then the options for the browser's ceremony - the workspace as relying party, the " +
			"account's opaque user handle, a single-use challenge bound to the session for five " +
			"minutes, the accepted algorithms and the credentials to exclude. Refused before the " +
			"proof at ten credentials and where the installation has no public address.",
		SideEffects: "Consumes the step-up; spends the account's older challenge and writes a new one; " +
			"writes an audit entry.",
		Input: []usecase.Field{
			{
				// Declared but not required: a caller without a proof needs the 403 that names the
				// methods rather than a 422 about a missing field.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The fresh proof this demands. It is consumed by this call.",
			},
		},
		StepUp: "adding a passkey or a security key",
		Audit: usecase.AuditDeclaration{
			Action: PasskeyRegistrationStartedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A way in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h StartPasskeyRegistration) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	options, err := h.Execute(ctx, actor, in.String("step_up_token"))
	if err != nil {
		return nil, err
	}
	exclude := make([]any, 0, len(options.Exclude))
	for _, held := range options.Exclude {
		transports := make([]any, 0, len(held.Transports))
		for _, transport := range held.Transports {
			transports = append(transports, transport)
		}
		exclude = append(exclude, usecase.Output{
			"type": "public-key", "id": base64.RawURLEncoding.EncodeToString(held.CredentialID),
			"transports": transports,
		})
	}
	return usecase.Output{
		"public_key": usecase.Output{
			"rp": usecase.Output{"id": options.RelyingParty.ID, "name": options.PartyName},
			"user": usecase.Output{
				"id":   base64.RawURLEncoding.EncodeToString(options.UserHandle),
				"name": options.UserName, "displayName": options.DisplayName,
			},
			"challenge": base64.RawURLEncoding.EncodeToString(options.Challenge),
			"pubKeyCredParams": []any{
				usecase.Output{"type": "public-key", "alg": int(passkeyES256)},
				usecase.Output{"type": "public-key", "alg": int(passkeyEdDSA)},
				usecase.Output{"type": "public-key", "alg": int(passkeyRS256)},
			},
			"timeout":            int(domain.PasskeyRegistrationLifetime / time.Millisecond),
			"excludeCredentials": exclude,
			"authenticatorSelection": usecase.Output{
				"residentKey": "preferred", "requireResidentKey": false, "userVerification": "preferred",
			},
			"attestation": "none",
		},
		"expires_at": options.ExpiresAt.UTC(),
	}, nil
}

// The algorithms the options offer, in order of preference (identity.md §23.3): COSE's numbers for
// ES256, EdDSA and RS256.
const (
	passkeyES256 int64 = -7
	passkeyEdDSA int64 = -8
	passkeyRS256 int64 = -257
)

// The completion's own refusals.
const (
	codeChallengeStale    = "auth.passkey_challenge_stale"
	codeAlreadyRegistered = "auth.passkey_already_registered"
)

// CompletePasskeyRegistrationCommand is the browser's answer, decoded, and the person's name for it.
type CompletePasskeyRegistrationCommand struct {
	Name              string
	RawID             []byte
	ClientDataJSON    []byte
	AttestationObject []byte
	Transports        []string
}

// AddedPasskey is the credential now held, and the recovery codes where they were minted.
type AddedPasskey struct {
	Passkey       OwnPasskey
	RecoveryCodes []secret.Secret
}

// CompletePasskeyRegistration is the second half: the ceremony checked and the credential kept.
type CompletePasskeyRegistration struct{ Writer PasskeyWriter }

// Execute spends the session's challenge, has the verifier check the ceremony, and keeps the
// credential under the account's lock.
//
// The challenge is spent first and in a transaction of its own, so a refused ceremony has used its
// one try (known traps: a write on a refusal path). A refusal here counts nothing in the attempt
// ledger: the step-up already proved the person, and a malformed attestation guesses nothing.
func (h CompletePasskeyRegistration) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CompletePasskeyRegistrationCommand,
) (AddedPasskey, error) {
	if err := passkeyPerson(actor); err != nil {
		return AddedPasskey{}, err
	}
	w := h.Writer
	s := w.Session
	session, err := s.stepUpSession(ctx, actor)
	if err != nil {
		return AddedPasskey{}, err
	}
	// The name first: a person who typed one too long fixes it and tries again with the same
	// challenge, which nothing has spent yet.
	name, err := domain.PasskeyName(cmd.Name)
	if err != nil {
		return AddedPasskey{}, err
	}
	challenge, err := w.Verifier.ChallengeOf(cmd.ClientDataJSON)
	if err != nil {
		return AddedPasskey{}, err
	}
	token, err := domain.NewPasskeyChallengeToken(actor.TenantID, challenge)
	if err != nil {
		// A challenge of another length was never one of ours.
		return AddedPasskey{}, shared.ErrConflict.WithDetail(codeChallengeStale)
	}
	scope := actor.PersistenceScope()

	var spent domain.PasskeyChallenge
	if err := s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, ok, err := w.Challenges.Consume(ctx, token, session.ID, actor.AccountID, s.Clock.Now())
		if err != nil {
			return err
		}
		if !ok {
			// Unknown, spent, superseded, expired, another session's or account's: one answer. A 409
			// rather than a 401, which a client would read as a session to renew.
			return shared.ErrConflict.WithDetail(codeChallengeStale)
		}
		spent = found
		return nil
	}); err != nil {
		return AddedPasskey{}, err
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
		return AddedPasskey{}, err
	}
	verified, err := w.Verifier.VerifyRegistration(passkey.Registration{
		ClientDataJSON: cmd.ClientDataJSON, AttestationObject: cmd.AttestationObject, RawID: cmd.RawID,
		Transports: cmd.Transports, Challenge: challenge, RelyingParty: party.RelyingParty,
	})
	if err != nil {
		return AddedPasskey{}, err
	}

	codeMaterial, err := s.Entropy.Bytes(domain.RecoveryCodeCount * domain.RecoveryCodeBytes)
	if err != nil {
		return AddedPasskey{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	codes, err := domain.NewRecoveryCodes(codeMaterial)
	if err != nil {
		return AddedPasskey{}, err
	}

	var added AddedPasskey
	err = s.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		// Under the account's lock: the limit's count, and whether the codes are there, read where no
		// other completion can change them before this one writes.
		if err := w.Passkeys.LockAccount(ctx, actor.AccountID); err != nil {
			return err
		}
		count, err := w.Passkeys.CountOf(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		if count >= domain.MaxPasskeys {
			return shared.ErrConflict.WithDetail(codeLimitReached).WithParams(limitParams())
		}
		remaining, err := s.Recovery.Remaining(ctx, actor.AccountID)
		if err != nil {
			return err
		}
		now := s.Clock.Now()
		credential := domain.PasskeyCredential{
			ID: s.IDs.NewID(), TenantID: actor.TenantID, AccountID: actor.AccountID,
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
		added.Passkey = ownPasskeyOf(credential)
		// The first factor of an account without a code answers ten (identity.md §9, §23.2): they back
		// every factor the account holds.
		if remaining == 0 {
			ids := make([]shared.ID, 0, len(codes))
			for range codes {
				ids = append(ids, s.IDs.NewID())
			}
			if err := s.Recovery.Replace(ctx, actor.AccountID, ids, codes, now); err != nil {
				return err
			}
			for _, code := range codes {
				added.RecoveryCodes = append(added.RecoveryCodes, secret.New(code))
			}
		}
		if err := s.recordMfaAudit(ctx, PasskeyAddedAction, audit.SeverityNotice, domain.Account{
			ID: actor.AccountID, TenantID: actor.TenantID, DisplayName: actor.AccountName,
		}, now); err != nil {
			return err
		}
		return w.told(ctx, actor, domain.WayInPasskeyAdded, now)
	})
	if err != nil {
		return AddedPasskey{}, err
	}
	return added, nil
}

// Descriptor is the catalogue entry.
func (h CompletePasskeyRegistration) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CompletePasskeyRegistrationName,
		Summary: "Finishes adding a passkey or a security key: the browser's ceremony, checked " +
			"against the challenge the same session started and the workspace's address, and the " +
			"name the person chose. The challenge is spent whatever the outcome. Where the account " +
			"held no unused recovery code, the answer carries ten, shown this once.",
		SideEffects: "Spends the challenge; keeps the credential; may replace the recovery codes; " +
			"writes an audit entry without the name.",
		Input: []usecase.Field{
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

func (h CompletePasskeyRegistration) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	transports, err := in.StringList("transports")
	if err != nil {
		return nil, err
	}
	cmd := CompletePasskeyRegistrationCommand{Name: in.String("name"), Transports: transports}
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
	added, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	out := usecase.Output{"passkey": ownPasskeyOutput(added.Passkey)}
	if len(added.RecoveryCodes) > 0 {
		codes := make([]any, 0, len(added.RecoveryCodes))
		for _, code := range added.RecoveryCodes {
			codes = append(codes, code.Reveal())
		}
		// The one place these are answered (T-18's "shown once" as code).
		out["recovery_codes"] = codes
	}
	return out, nil
}
