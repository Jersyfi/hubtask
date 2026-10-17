// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"bytes"
	"context"
	"encoding/base64"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// UC-ID-21 check 5: a passkey or security key as the second factor after the password, through the
// registry as the card reaches it.

// proofStore keeps the challenges a known account answers, spending them as the statement does.
type proofStore struct {
	issued []issuedProof
}

type issuedProof struct {
	challenge domain.PasskeyProofChallenge
	token     string
	spent     bool
}

var _ repository.PasskeyProofs = (*proofStore)(nil)

func (s *proofStore) Issue(_ context.Context, challenge domain.PasskeyProofChallenge, presented domain.Token) error {
	s.issued = append(s.issued, issuedProof{challenge: challenge, token: presented.Secret()})
	return nil
}

func (s *proofStore) Spend(
	_ context.Context, purpose domain.PendingPurpose, presented domain.Token,
	accountID, sessionID shared.ID, at time.Time,
) (domain.PasskeyProofChallenge, bool, error) {
	for i := range s.issued {
		held := &s.issued[i]
		if held.token == presented.Secret() && !held.spent && held.challenge.Purpose == purpose &&
			held.challenge.AccountID == accountID && held.challenge.SessionID == sessionID &&
			at.Before(held.challenge.ExpiresAt) {
			held.spent = true
			return held.challenge, true, nil
		}
	}
	return domain.PasskeyProofChallenge{}, false, nil
}

type keyFactorFixture struct {
	session  *sessionFixture
	keys     *passkeyStore
	proofs   *proofStore
	holders  *signInsStore
	verifier *verifierFake
	registry *usecase.Registry
	bert     domain.PasskeyCredential
}

var (
	bertsKey   = []byte{0x0b, 0xe7, 0x0b, 0xe7}
	strangeKey = []byte{0x05, 0x7a}
)

const (
	workspaceHost    = "acme.hubtask.example"
	installationHost = "hubtask.example"
)

// newKeyFactorFixture is the workspace Acme of a multi-mode installation at hubtask.example, and Bert,
// whose password is right and who holds a security key made at acme.hubtask.example - and no app. The
// key answers without verifying the person, as one without a PIN does.
func newKeyFactorFixture(t *testing.T, requirement domain.MfaRequirement) *keyFactorFixture {
	t.Helper()
	session := mfaFixture(now)
	session.withAccount("bert@example.org", "correct horse battery")
	session.writer.Rule = ruleFake{requirement: requirement}
	f := attachKeys(session)
	f.register(t)
	return f
}

// attachKeys gives the writer Bert's key and everything asking for one needs.
func attachKeys(session *sessionFixture) *keyFactorFixture {
	session.writer.Entropy = &steppingEntropy{}
	f := &keyFactorFixture{
		session: session, keys: &passkeyStore{}, proofs: &proofStore{}, holders: newSignInsStore(),
		verifier: &verifierFake{asserted: passkey.Asserted{UserVerified: false, SignCount: 8}},
	}
	f.bert = domain.PasskeyCredential{
		ID: "01936f2a-7c1e-7000-8000-00000000be47", TenantID: tenant, AccountID: account,
		CredentialID: bertsKey, PublicKey: []byte{0xa5}, Algorithm: -7, SignCount: 7,
		UserHandle: bytes.Repeat([]byte{0x44}, domain.PasskeyUserHandleBytes),
		Transports: []string{"usb"}, Name: "YubiKey", RPID: workspaceHost,
	}
	f.keys.held = append(f.keys.held, f.bert)
	f.holders.holders[string(bertsKey)] = repository.PasskeyHolder{Credential: f.bert}
	stranger := f.bert
	stranger.ID, stranger.AccountID, stranger.CredentialID = "01936f2a-7c1e-7000-8000-0000000057a0", "01936f2a-7c1e-7000-8000-0000000057a1", strangeKey
	f.holders.holders[string(strangeKey)] = repository.PasskeyHolder{Credential: stranger}
	party := PasskeyParty{
		Workspaces: &workspaceStore{row: domain.Workspace{
			Tenant: domain.Tenant{ID: tenant, Slug: "acme", DisplayName: "Acme"}, Version: 1,
		}},
		Installation: PasskeyInstallation{Host: installationHost, Scheme: "https", Multi: true},
	}
	session.writer.KeyProofs = &KeyProofs{
		Passkeys: f.keys, Proofs: f.proofs, Holders: f.holders, Verifier: f.verifier, Party: party,
	}
	session.writer.Keys = PasskeyKeys{Passkeys: f.keys, Party: party}
	return f
}

// register builds the registry from the writer as it stands: every use case holds a copy.
func (f *keyFactorFixture) register(t *testing.T) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		SignIn{Writer: f.session.writer}.Descriptor(), CompleteSignIn{Writer: f.session.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
}

func (f *keyFactorFixture) password(t *testing.T, host string) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), SignInName, appshared.ActorContext{}, usecase.Input{
		"email": "bert@example.org", "password": "correct horse battery", "host": host,
		"user_agent": "Firefox", "remote_addr": "203.0.113.7:51234",
	})
}

// step answers the password and hands back the factor's step.
func (f *keyFactorFixture) step(t *testing.T) usecase.Output {
	t.Helper()
	out, err := f.password(t, workspaceHost)
	if err != nil {
		t.Fatalf("the password was refused: %v", err)
	}
	if out["mfa_required"] != true {
		t.Fatalf("no factor's step: %v", out)
	}
	return out
}

func challengeOf(step usecase.Output) string {
	return step["passkey_options"].(usecase.Output)["public_key"].(usecase.Output).String("challenge")
}

// answer presents a key's answer whose client data is the challenge itself (the verifier fake's shape).
func (f *keyFactorFixture) answer(t *testing.T, pending, challenge string, key []byte) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), CompleteSignInName, appshared.ActorContext{}, usecase.Input{
		"pending_token": pending, "raw_id": base64.RawURLEncoding.EncodeToString(key),
		"client_data_json": challenge, "authenticator_data": "AQ", "signature": "AQ",
	})
}

func TestAKeyWithoutPinCompletesTheFactorsStep(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForEveryone)
	step := f.step(t)
	methods := step["methods"].([]any)
	if len(methods) != 2 || methods[0] != methodPasskey || methods[1] != methodRecovery {
		t.Errorf("methods %v, want the key and the recovery code", methods)
	}
	publicKey := step["passkey_options"].(usecase.Output)["public_key"].(usecase.Output)
	allowed := publicKey["allowCredentials"].([]any)
	if publicKey.String("rpId") != workspaceHost || publicKey.String("userVerification") != "preferred" ||
		len(allowed) != 1 || allowed[0].(usecase.Output).String("id") != base64.RawURLEncoding.EncodeToString(bertsKey) {
		t.Errorf("options %v", publicKey)
	}
	if len(f.proofs.issued) != 1 || f.proofs.issued[0].challenge.Purpose != domain.PendingPasskeyFactor ||
		f.proofs.issued[0].challenge.AccountID != account {
		t.Fatalf("issued %+v", f.proofs.issued)
	}

	out, err := f.answer(t, step.String("pending_token"), challengeOf(step), bertsKey)
	if err != nil {
		t.Fatalf("a key without the person verified was refused as the factor: %v", err)
	}
	if out.String("access_token") == "" {
		t.Fatalf("no pair: %v", out)
	}
	opened := f.session.sessions.inserted
	if len(opened) != 1 || opened[0].SignedInWith != domain.SignedInWithPasswordPasskey {
		t.Errorf("sessions %+v, want one opened as %q", opened, domain.SignedInWithPasswordPasskey)
	}
	asked := f.verifier.assertions
	if len(asked) != 1 || asked[0].RelyingParty.ID != workspaceHost ||
		asked[0].RelyingParty.Origin != "https://"+workspaceHost {
		t.Errorf("asked %+v", asked)
	}
	if !f.proofs.issued[0].spent || len(f.holders.used) != 1 ||
		f.holders.holders[string(bertsKey)].Credential.SignCount != 8 {
		t.Error("the challenge was not spent, or the key's use not recorded")
	}
}

// Where a key cannot sign - the installation's host - the account meets what it met before keys:
// the setup the rule demands, and no setup offered beside it; nothing demanded, no step at all.
func TestAKeyCountsOnlyWhereItCanSign(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForEveryone)
	out, err := f.password(t, installationHost)
	if err != nil {
		t.Fatal(err)
	}
	methods := out["methods"].([]any)
	if len(methods) != 1 || methods[0] != methodEnroll || out["passkey_setup"] != nil || out["passkey_options"] != nil {
		t.Errorf("at the installation's host %v, want the setup alone", out)
	}

	open := newKeyFactorFixture(t, domain.MfaForNobody)
	out, err = open.password(t, installationHost)
	if err != nil || out["mfa_required"] == true || out.String("access_token") == "" {
		t.Errorf("nothing demanded, a key that cannot sign here answered %v, %v", out, err)
	}
	if len(open.proofs.issued) != 0 {
		t.Error("a challenge was issued where no key can answer it")
	}
}

// The setup the rule demands offers a key beside the app only at the workspace's host.
func TestTheSetupOffersAKeyAtTheWorkspacesHost(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForEveryone)
	f.keys.held = nil
	out, err := f.password(t, workspaceHost)
	if err != nil {
		t.Fatal(err)
	}
	if methods := out["methods"].([]any); len(methods) != 1 || methods[0] != methodEnroll || out["passkey_setup"] != true {
		t.Errorf("the setup %v, want ENROLL with a key offered", out)
	}
}

// The app and a key: both are offered, the code still works.
func TestTheAppAndAKeyAreBothOffered(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForNobody)
	material := enrolled(t, f.session)
	step := f.step(t)
	methods := step["methods"].([]any)
	if len(methods) != 3 || methods[0] != methodTotp || methods[1] != methodPasskey || methods[2] != methodRecovery {
		t.Errorf("methods %v", methods)
	}
	out, err := f.registry.Invoke(t.Context(), CompleteSignInName, appshared.ActorContext{}, usecase.Input{
		"pending_token": step.String("pending_token"),
		"code":          domain.TotpCode(material, domain.TotpStep(now)+1),
	})
	if err != nil || out.String("access_token") == "" {
		t.Errorf("the code beside a key: %v, %v", out, err)
	}
}

// Check 5's other half and §4.3: every key that does not prove the account is the code's refusal,
// counted on the account's subject, and the step stands for another try.
func TestARefusedKeyIsTheCodesRefusalAndTheStepStands(t *testing.T) {
	for name, tamper := range map[string]func(*keyFactorFixture, string) (string, []byte){
		"another account's key": func(_ *keyFactorFixture, challenge string) (string, []byte) {
			return challenge, strangeKey
		},
		"an unknown key": func(_ *keyFactorFixture, challenge string) (string, []byte) {
			return challenge, []byte{0x99}
		},
		"a refused signature": func(f *keyFactorFixture, challenge string) (string, []byte) {
			f.verifier.refuseSign = true
			return challenge, bertsKey
		},
		"a challenge never issued": func(_ *keyFactorFixture, _ string) (string, []byte) {
			return base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x77}, 32)), bertsKey
		},
		"a counter that went back": func(f *keyFactorFixture, challenge string) (string, []byte) {
			f.verifier.asserted.SignCount = 3
			return challenge, bertsKey
		},
		"a key made at another address": func(f *keyFactorFixture, challenge string) (string, []byte) {
			holder := f.holders.holders[string(bertsKey)]
			holder.Credential.RPID = "elsewhere.example"
			f.holders.holders[string(bertsKey)] = holder
			return challenge, bertsKey
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := newKeyFactorFixture(t, domain.MfaForEveryone)
			step := f.step(t)
			challenge, key := tamper(f, challengeOf(step))
			_, err := f.answer(t, step.String("pending_token"), challenge, key)
			if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
				t.Fatalf("answered %v, want the code's refusal", err)
			}
			if f.session.attempts.standing[mfaSubject(account)].Failures != 1 {
				t.Errorf("the refusal was not counted on the account's subject: %+v", f.session.attempts.standing)
			}
			if len(f.session.sessions.inserted) != 0 {
				t.Error("a session was opened")
			}
			// The pending credential stands: the right key, on a fresh challenge, still completes it.
			f.verifier.refuseSign, f.verifier.asserted.SignCount = false, 9
			f.holders.holders[string(bertsKey)] = repository.PasskeyHolder{Credential: f.bert}
			// The refusing transaction rolled back, the challenge's spend with it (the fake keeps it).
			f.proofs.issued[0].spent = false
			if _, err := f.answer(t, step.String("pending_token"), challengeOf(step), bertsKey); err != nil {
				t.Errorf("the step did not stand for another try: %v", err)
			}
		})
	}
}

// A sign-in's challenge, or a fresh proof's, opens nothing at the factor's step: a challenge is
// spent only by its own door.
func TestAnotherDoorsChallengeIsNoFactor(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForEveryone)
	step := f.step(t)
	raw := bytes.Repeat([]byte{0x5c}, 32)
	stepUp, _ := domain.NewPasskeyStepUpToken(tenant, raw)
	f.proofs.issued = append(f.proofs.issued, issuedProof{
		challenge: domain.PasskeyProofChallenge{
			Purpose: domain.PendingPasskeyStepUp, AccountID: account, ExpiresAt: now.Add(time.Hour),
		},
		token: stepUp.Secret(),
	})
	if _, err := f.answer(t, step.String("pending_token"), base64.RawURLEncoding.EncodeToString(raw), bertsKey); err == nil {
		t.Error("a fresh proof's challenge completed the factor's step")
	}
}

// Exactly one proof: a code and a key at once is refused before either is judged.
func TestTwoProofsAtOnceAreRefused(t *testing.T) {
	f := newKeyFactorFixture(t, domain.MfaForEveryone)
	step := f.step(t)
	_, err := f.registry.Invoke(t.Context(), CompleteSignInName, appshared.ActorContext{}, usecase.Input{
		"pending_token": step.String("pending_token"), "code": "123456",
		"raw_id": base64.RawURLEncoding.EncodeToString(bertsKey), "client_data_json": challengeOf(step),
		"authenticator_data": "AQ", "signature": "AQ",
	})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_required") {
		t.Errorf("answered %v, want exactly one proof asked for", err)
	}
	if len(f.verifier.assertions) != 0 {
		t.Error("the key was judged")
	}
}

// One helper decides at every door (D1 of the record): the provider's LINK step hands on to a key
// where one can sign, and connects as before where none can.
func TestTheLinkStepHandsOnToAKeyWhereItCanSign(t *testing.T) {
	keys := newKeyFactorFixture(t, domain.MfaForNobody)
	for host, wantKey := range map[string]bool{workspaceHost: true, installationHost: false} {
		f := linkFixture(t, false)
		f.writer.Session.KeyProofs = keys.session.writer.KeyProofs
		f.session.writer.KeyProofs = keys.session.writer.KeyProofs
		result, err := arrive(t, f)
		if err != nil || result.Challenge == nil {
			t.Fatalf("arriving: (%+v, %v)", result, err)
		}
		next, err := CompleteLink{Writer: f.writer}.Execute(t.Context(), CompleteLinkCommand{
			PendingToken: result.Challenge.Token, Password: secret.New("correct horse battery"), Host: host,
		})
		if err != nil {
			t.Fatalf("%s: the right password was refused: %v", host, err)
		}
		if !wantKey {
			if next.Pair == nil || len(f.external.links) != 1 {
				t.Errorf("%s: where no key can sign the password is the whole proof, got %+v", host, next)
			}
			continue
		}
		if next.Challenge == nil || next.Challenge.Methods[0] != methodPasskey || next.Challenge.KeyOptions == nil {
			t.Fatalf("%s: answered %+v, want the key's step", host, next.Challenge)
		}
		if len(f.external.links) != 0 {
			t.Fatal("the provider was connected before the key")
		}
		completed, _, err := CompleteSignIn{Writer: f.session.writer}.Execute(t.Context(), CompleteSignInCommand{
			PendingToken: next.Challenge.Token,
			Key: KeyAnswer{RawID: bertsKey, ClientDataJSON: next.Challenge.KeyOptions.Challenge,
				AuthenticatorData: []byte{1}, Signature: []byte{1}},
		})
		if err != nil || completed.Pair == nil || len(f.external.links) != 1 {
			t.Errorf("the key's step: (%+v, %v), links %v", completed, err, f.external.links)
		}
		keys.holders.holders[string(bertsKey)] = repository.PasskeyHolder{Credential: keys.bert}
	}
}

// UC-ID-04: the reset's kept factor is a key where one can sign.
func TestAResetOfAnAccountWithAKeyAnswersTheKeysStep(t *testing.T) {
	keys := newKeyFactorFixture(t, domain.MfaForNobody)
	fixture := newResetFixture(now)
	fixture.writer.Session.KeyProofs = keys.session.writer.KeyProofs
	fixture.writer.Session.Entropy = &steppingEntropy{}
	fixture.writer.Session.Enrollments, fixture.writer.Session.Pending = newEnrollments(), newPending()
	token := fixture.mintedFor(t, now, domain.PendingReset)
	result, err := ResetPassword{Writer: fixture.writer}.Execute(t.Context(), ResetPasswordCommand{
		Token: secret.New(token), Password: secret.New("seven blue lanterns"), Host: workspaceHost,
	})
	if err != nil {
		t.Fatalf("the reset was refused: %v", err)
	}
	if result.Pair != nil || result.Challenge == nil || result.Challenge.Methods[0] != methodPasskey ||
		result.Challenge.KeyOptions == nil {
		t.Fatalf("the reset answered %+v, want the key's step", result)
	}
}

// Check 8's other half: with `PASSKEY` switched off as a way in, a passkey alone opens nothing, and
// the same account's key still completes the step after the password - the switch is about ways in,
// not about factors (identity.md §23.4).
func TestSwitchedOffPasskeysStillServeAsTheFactor(t *testing.T) {
	signIn := newPasskeySignInFixture(t)
	signIn.writer.Resolver = methodsResolver{methods: []string{domain.MethodDirect}}
	signIn.register(t)
	if _, err := signIn.options(t, "acme.hubtask.example"); err == nil || !strings.Contains(err.Error(), codeNotOffered) {
		t.Fatalf("switched off, the passkey door answered %v", err)
	}

	f := newKeyFactorFixture(t, domain.MfaForNobody)
	step := f.step(t)
	if _, err := f.answer(t, step.String("pending_token"), challengeOf(step), bertsKey); err != nil {
		t.Errorf("the key as the factor: %v", err)
	}
}
