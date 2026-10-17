// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"encoding/base64"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
)

// A passkey or security key as the second factor after the password and as a fresh proof
// (identity.md §23.2, §16.1). Presence on the device suffices at both: the password, or the session,
// came first, so the key is the second thing the person holds, never the only one (P-02).
//
// A key counts as a factor only where it can be asked for: the account holds one made for the
// workspace's relying party, and the request arrived at that party's very host - a browser runs a
// ceremony for the host it is on and no other. Everywhere else the account meets what it met before
// keys existed: the app's step, or the setup the rule demands.

// KeyProofs is what asking for a key needs. Set in the literal that builds the writer, never later:
// every door is a copy. Nil asks for none, an installation wired before keys.
type KeyProofs struct {
	Passkeys repository.PasskeyCredentials
	Party    PasskeyParty
	Proofs   repository.PasskeyProofs
	// Holders reads the credential a browser names and records its use, as the passkey sign-in does.
	Holders  repository.PasskeySignIns
	Verifier passkey.Verifier
}

// heldFactor is what a factor step can ask an account for at one host.
type heldFactor struct {
	App bool
	// Keys are the account's credentials of the relying party, empty where none can sign here.
	Keys  []domain.PasskeyCredential
	Party relyingParty
}

func (h heldFactor) any() bool { return h.App || len(h.Keys) > 0 }

// factorHeldAt is the one answer to "does this account hold a second factor here", read inside the
// caller's transaction by every door that hands on to the factor's step.
func (w SessionWriter) factorHeldAt(ctx context.Context, accountID shared.ID, host string) (heldFactor, error) {
	var held heldFactor
	if w.Enrollments != nil {
		enrollment, err := w.Enrollments.Find(ctx, accountID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return heldFactor{}, err
		}
		held.App = err == nil && !enrollment.ConfirmedAt.IsZero()
	}
	party, ok, err := w.keyPartyAt(ctx, host)
	if err != nil || !ok {
		return held, err
	}
	keys, err := w.KeyProofs.Passkeys.ListOf(ctx, accountID)
	if err != nil {
		return heldFactor{}, err
	}
	for _, key := range keys {
		if key.BelongsTo(party.ID) {
			held.Keys = append(held.Keys, key)
		}
	}
	held.Party = party
	return held, nil
}

// keyPartyAt answers the workspace's relying party where a key can be asked for at host.
func (w SessionWriter) keyPartyAt(ctx context.Context, host string) (relyingParty, bool, error) {
	if w.KeyProofs == nil || host == "" {
		return relyingParty{}, false, nil
	}
	party, ok, err := w.KeyProofs.Party.of(ctx)
	if err != nil || !ok || !party.servesAt(host) {
		return relyingParty{}, false, err
	}
	return party, true, nil
}

// factorMethods is what the factor's step offers: the app's code, the key, and the recovery code.
func factorMethods(held heldFactor) []string {
	var methods []string
	if held.App {
		methods = append(methods, methodTotp)
	}
	if len(held.Keys) > 0 {
		methods = append(methods, methodPasskey)
	}
	return append(methods, methodRecovery)
}

// KeyOptions is what a browser's ceremony for a known account is asked with.
type KeyOptions struct {
	RPID      string
	Challenge []byte
	ExpiresAt time.Time
	Allow     []domain.PasskeyCredential
}

// keyTokenOf is a challenge as the token its row is found by, under its purpose's own prefix.
func keyTokenOf(purpose domain.PendingPurpose, tenantID shared.ID, challenge []byte) (domain.Token, error) {
	if purpose == domain.PendingPasskeyStepUp {
		return domain.NewPasskeyStepUpToken(tenantID, challenge)
	}
	return domain.NewPasskeyFactorToken(tenantID, challenge)
}

// issueKeyChallenge writes a challenge for the account's keys in the caller's transaction and answers
// the options; sessionID only for a fresh proof.
func (w SessionWriter) issueKeyChallenge(
	ctx context.Context, tenantID shared.ID, purpose domain.PendingPurpose,
	accountID, sessionID shared.ID, held heldFactor,
) (*KeyOptions, error) {
	challenge, err := w.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return nil, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	token, err := keyTokenOf(purpose, tenantID, challenge)
	if err != nil {
		return nil, err
	}
	now := w.Clock.Now()
	issued := domain.PasskeyProofChallenge{
		ID: w.IDs.NewID(), Purpose: purpose, AccountID: accountID, SessionID: sessionID,
		CreatedAt: now.UTC(), ExpiresAt: now.Add(domain.PendingLifetime).UTC(),
	}
	if err := w.KeyProofs.Proofs.Issue(ctx, issued, token); err != nil {
		return nil, err
	}
	return &KeyOptions{RPID: held.Party.ID, Challenge: challenge, ExpiresAt: issued.ExpiresAt, Allow: held.Keys}, nil
}

// KeyAnswer is a browser's answer to a ceremony for a known account, decoded.
type KeyAnswer struct {
	RawID             []byte
	ClientDataJSON    []byte
	AuthenticatorData []byte
	Signature         []byte
}

func (a KeyAnswer) given() bool {
	return len(a.RawID) > 0 || len(a.ClientDataJSON) > 0 || len(a.AuthenticatorData) > 0 || len(a.Signature) > 0
}

// errKeyRefused is every way a key's answer fails, which each door answers in its own words.
var errKeyRefused = errors.New("passkey: the answer does not prove the account")

// proveWithKey judges a key's answer for the account inside the caller's transaction: one of its own
// credentials, of the relying party, signing this purpose's challenge, the counter rising. The
// challenge is spent and the use recorded only where everything holds - on a refusal the transaction
// rolls back and the challenge stands for another try, as the code's step does. Presence suffices.
func (w SessionWriter) proveWithKey(
	ctx context.Context, tenantID shared.ID, purpose domain.PendingPurpose,
	accountID, sessionID shared.ID, answer KeyAnswer,
) error {
	if w.KeyProofs == nil {
		return errKeyRefused
	}
	keys := w.KeyProofs
	party, ok, err := keys.Party.of(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errKeyRefused
	}
	challenge, err := keys.Verifier.ChallengeOf(answer.ClientDataJSON)
	if err != nil {
		return errors.Join(errKeyRefused, err)
	}
	token, err := keyTokenOf(purpose, tenantID, challenge)
	if err != nil {
		return errors.Join(errKeyRefused, err)
	}
	holder, err := keys.Holders.HolderOf(ctx, answer.RawID)
	if errors.Is(err, shared.ErrNotFound) {
		return errKeyRefused
	}
	if err != nil {
		return err
	}
	credential := holder.Credential
	if credential.AccountID != accountID || !credential.BelongsTo(party.ID) {
		return errKeyRefused
	}
	asserted, err := keys.Verifier.VerifyAssertion(passkey.Assertion{
		ClientDataJSON: answer.ClientDataJSON, AuthenticatorData: answer.AuthenticatorData,
		Signature: answer.Signature, RawID: answer.RawID, Challenge: challenge,
		RelyingParty: party.RelyingParty, CredentialID: credential.CredentialID,
		PublicKey: credential.PublicKey, Algorithm: credential.Algorithm,
		BackupEligible: credential.BackupEligible,
	})
	if err != nil {
		return errors.Join(errKeyRefused, err)
	}
	now := w.Clock.Now()
	_, spent, err := keys.Proofs.Spend(ctx, purpose, token, accountID, sessionID, now)
	if err != nil {
		return err
	}
	if !spent {
		return errKeyRefused
	}
	marked, err := keys.Holders.MarkUsed(ctx, credential.ID, asserted.SignCount, asserted.BackedUp, now)
	if err != nil {
		return err
	}
	if !marked {
		// The counter went back: a cloned authenticator, or a replay (identity.md §23.3).
		return errKeyRefused
	}
	return nil
}

// keyAnswerOf decodes the four answer fields; false where one is no base64url.
func keyAnswerOf(in usecase.Input) (KeyAnswer, bool) {
	var answer KeyAnswer
	for _, field := range []struct {
		name string
		into *[]byte
	}{
		{"raw_id", &answer.RawID}, {"client_data_json", &answer.ClientDataJSON},
		{"authenticator_data", &answer.AuthenticatorData}, {"signature", &answer.Signature},
	} {
		decoded, err := base64.RawURLEncoding.DecodeString(in.String(field.name))
		if err != nil {
			return KeyAnswer{}, false
		}
		*field.into = decoded
	}
	return answer, true
}

// hostField is the request's host, for every door whose answer may ask for a key: only the adapter
// has it.
var hostField = usecase.Field{
	Name: "host", Kind: usecase.KindString,
	Description: "The host the request arrived at: a key is asked for only at the workspace's own.",
}

// keyAnswerFields are the answer's inputs, for every door that takes one beside other proofs.
var keyAnswerFields = []usecase.Field{
	{Name: "raw_id", Kind: usecase.KindString,
		Description: "A passkey or security key's answer: the credential's rawId, base64url without padding."},
	{Name: "client_data_json", Kind: usecase.KindString,
		Description: "The answer's clientDataJSON, base64url without padding."},
	{Name: "authenticator_data", Kind: usecase.KindString,
		Description: "The answer's authenticatorData, base64url without padding."},
	{Name: "signature", Kind: usecase.KindString,
		Description: "The answer's signature, base64url without padding."},
}

// keyOptionsOutput is the options in the WebAuthn JSON form.
func keyOptionsOutput(options KeyOptions) usecase.Output {
	allow := make([]any, 0, len(options.Allow))
	for _, held := range options.Allow {
		transports := make([]any, 0, len(held.Transports))
		for _, transport := range held.Transports {
			transports = append(transports, transport)
		}
		allow = append(allow, usecase.Output{
			"type": "public-key", "id": base64.RawURLEncoding.EncodeToString(held.CredentialID),
			"transports": transports,
		})
	}
	return usecase.Output{
		"public_key": usecase.Output{
			"rpId":             options.RPID,
			"challenge":        base64.RawURLEncoding.EncodeToString(options.Challenge),
			"timeout":          int(domain.PendingLifetime / time.Millisecond),
			"userVerification": "preferred",
			"allowCredentials": allow,
		},
		"expires_at": options.ExpiresAt.UTC(),
	}
}
