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
)

// UC-ID-21: signing in with a passkey alone, through the registry as the card reaches it.

// signInsStore keeps sign-in challenges, credentials with their holders, and counters as the
// statements do.
type signInsStore struct {
	challenges map[string]time.Time // token -> expiry; absent once spent
	issued     []domain.PendingCredential
	holders    map[string]repository.PasskeyHolder
	used       []time.Time
}

func newSignInsStore() *signInsStore {
	return &signInsStore{challenges: map[string]time.Time{}, holders: map[string]repository.PasskeyHolder{}}
}

func (s *signInsStore) IssueChallenge(_ context.Context, challenge domain.PendingCredential, presented domain.Token) error {
	s.issued = append(s.issued, challenge)
	s.challenges[presented.Secret()] = challenge.ExpiresAt
	return nil
}

func (s *signInsStore) SpendChallenge(_ context.Context, presented domain.Token, now time.Time) (bool, error) {
	expires, held := s.challenges[presented.Secret()]
	if !held || !now.Before(expires) {
		return false, nil
	}
	delete(s.challenges, presented.Secret())
	return true, nil
}

func (s *signInsStore) HolderOf(_ context.Context, credentialID []byte) (repository.PasskeyHolder, error) {
	holder, held := s.holders[string(credentialID)]
	if !held {
		return repository.PasskeyHolder{}, shared.ErrNotFound.WithDetail("auth.sign_in_failed")
	}
	return holder, nil
}

func (s *signInsStore) MarkUsed(_ context.Context, id shared.ID, count uint32, backedUp bool, at time.Time) (bool, error) {
	for key, holder := range s.holders {
		if holder.Credential.ID != id {
			continue
		}
		stored := holder.Credential.SignCount
		if count != 0 && stored != 0 && count <= stored {
			return false, nil
		}
		holder.Credential.SignCount = max(stored, count)
		holder.Credential.BackedUp = backedUp
		holder.Credential.LastUsedAt = at
		s.holders[key] = holder
		s.used = append(s.used, at)
		return true, nil
	}
	return false, nil
}

// methodsResolver answers the rule in force with the methods given.
type methodsResolver struct{ methods []string }

func (r methodsResolver) Resolve(context.Context, shared.ID) (ResolvedPolicy, error) {
	policy := domain.DefaultSignInPolicy()
	policy.Methods = r.methods
	return ResolvedPolicy{Effective: domain.EffectivePolicy{Policy: policy}}, nil
}

type passkeySignInFixture struct {
	session  *sessionFixture
	store    *signInsStore
	verifier *verifierFake
	writer   PasskeySignInWriter
	registry *usecase.Registry
	ada      repository.PasskeyHolder
}

var adaCredential = []byte{0x0a, 0xda, 0x0a, 0xda}

// newPasskeySignInFixture is the workspace Acme of a multi-mode installation at hubtask.example,
// passkeys on, and Ada, who holds a passkey made at acme.hubtask.example; her workspace requires a
// second factor of everyone.
func newPasskeySignInFixture(t *testing.T) *passkeySignInFixture {
	t.Helper()
	session := mfaFixture(now)
	session.writer.Entropy = &steppingEntropy{}
	session.writer.Rule = ruleFake{requirement: domain.MfaForEveryone}
	f := &passkeySignInFixture{
		session: session, store: newSignInsStore(),
		verifier: &verifierFake{asserted: passkey.Asserted{UserVerified: true, SignCount: 8}},
	}
	f.ada = repository.PasskeyHolder{
		Credential: domain.PasskeyCredential{
			ID: "01936f2a-7c1e-7000-8000-00000000a0da", TenantID: tenant, AccountID: account,
			CredentialID: adaCredential, PublicKey: []byte{0xa5}, Algorithm: -7, SignCount: 7,
			UserHandle:     bytes.Repeat([]byte{0x33}, domain.PasskeyUserHandleBytes),
			BackupEligible: true, Name: "iPhone", RPID: "acme.hubtask.example",
		},
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser, Email: "ada@example.org",
			DisplayName: "Ada", Status: domain.AccountActive,
		},
		TenantStatus: domain.TenantActive,
	}
	f.store.holders[string(adaCredential)] = f.ada
	f.writer = PasskeySignInWriter{
		Session: session.writer, SignIns: f.store, Verifier: f.verifier,
		Party: PasskeyParty{
			Workspaces: &workspaceStore{row: domain.Workspace{
				Tenant: domain.Tenant{ID: tenant, Slug: "acme", DisplayName: "Acme"}, Version: 1,
			}},
			Installation: PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true},
		},
		Resolver: methodsResolver{methods: []string{domain.MethodDirect, domain.MethodPasskey}},
	}
	f.register(t)
	return f
}

func (f *passkeySignInFixture) register(t *testing.T) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		StartPasskeySignIn{Writer: f.writer}.Descriptor(), CompletePasskeySignIn{Writer: f.writer}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
}

func requestInput(host string) usecase.Input {
	return usecase.Input{
		"host": host, "user_agent": "Firefox", "remote_addr": "203.0.113.7:51234",
	}
}

func (f *passkeySignInFixture) options(t *testing.T, host string) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), StartPasskeySignInName, appshared.ActorContext{}, requestInput(host))
}

// started answers a challenge the workspace issued, base64url as a client data carries it.
func (f *passkeySignInFixture) started(t *testing.T) string {
	t.Helper()
	out, err := f.options(t, "acme.hubtask.example")
	if err != nil {
		t.Fatalf("the options were refused: %v", err)
	}
	return out["public_key"].(usecase.Output).String("challenge")
}

// signIn presents a ceremony whose client data is the challenge itself (the verifier fake's shape).
func (f *passkeySignInFixture) signIn(t *testing.T, challenge string, credential, handle []byte) (usecase.Output, error) {
	t.Helper()
	in := requestInput("acme.hubtask.example")
	in["raw_id"] = base64.RawURLEncoding.EncodeToString(credential)
	in["client_data_json"] = challenge
	in["authenticator_data"] = "AQ"
	in["signature"] = "AQ"
	in["user_handle"] = base64.RawURLEncoding.EncodeToString(handle)
	return f.registry.Invoke(t.Context(), CompletePasskeySignInName, appshared.ActorContext{}, in)
}

func (f *passkeySignInFixture) signInAsAda(t *testing.T) (usecase.Output, error) {
	t.Helper()
	return f.signIn(t, f.started(t), adaCredential, f.ada.Credential.UserHandle)
}

// UC-ID-21 checks 2 and 7: a verified passkey opens a session without password or code, also where
// the workspace requires a second factor of everyone, and the session says it was a passkey.
func TestAVerifiedPasskeyOpensASessionAlone(t *testing.T) {
	f := newPasskeySignInFixture(t)
	options, err := f.options(t, "acme.hubtask.example")
	if err != nil {
		t.Fatal(err)
	}
	publicKey := options["public_key"].(usecase.Output)
	if publicKey.String("rpId") != "acme.hubtask.example" || publicKey.String("userVerification") != "required" ||
		publicKey["timeout"] != 300000 {
		t.Errorf("options %v", publicKey)
	}
	if len(f.store.issued) != 1 || !f.store.issued[0].AccountID.IsZero() ||
		f.store.issued[0].Purpose != domain.PendingPasskeySignIn || f.store.issued[0].IPClass != "203.0.113.0/24" {
		t.Errorf("issued %+v", f.store.issued)
	}

	out, err := f.signIn(t, publicKey.String("challenge"), adaCredential, f.ada.Credential.UserHandle)
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	if out["mfa_required"] == true || out.String("access_token") == "" {
		t.Fatalf("answered %v, want the pair", out)
	}
	if len(f.session.sessions.inserted) != 1 || f.session.sessions.inserted[0].SignedInWith != domain.SignedInWithPasskey {
		t.Errorf("sessions %+v", f.session.sessions.inserted)
	}
	asked := f.verifier.assertions[0]
	if asked.RelyingParty.ID != "acme.hubtask.example" || asked.RelyingParty.Origin != "https://acme.hubtask.example" ||
		!bytes.Equal(asked.PublicKey, f.ada.Credential.PublicKey) || !asked.BackupEligible {
		t.Errorf("verified against %+v", asked)
	}
	// The use and the counter are recorded.
	if held := f.store.holders[string(adaCredential)]; held.Credential.SignCount != 8 || held.Credential.LastUsedAt.IsZero() {
		t.Errorf("the credential after %+v", held.Credential)
	}
	var signedIn bool
	for _, entry := range f.session.audit.entries {
		if entry.Action == PasskeySignedInAction {
			signedIn = true
		}
		if strings.Contains(entry.ActorLabel+string(entry.TargetID), "iPhone") {
			t.Errorf("an entry carries the passkey's name: %+v", entry)
		}
	}
	if !signedIn {
		t.Errorf("no %s entry: %+v", PasskeySignedInAction, f.session.audit.entries)
	}
}

// UC-ID-21 check 4: every refusal is the wrong password's sentence and counts.
func TestEveryRefusalOfAPasskeyIsTheOneAnswerAndCounts(t *testing.T) {
	cases := map[string]func(f *passkeySignInFixture) (usecase.Output, error){
		"an unknown passkey": func(f *passkeySignInFixture) (usecase.Output, error) {
			return f.signIn(t, f.started(t), []byte{9, 9, 9}, f.ada.Credential.UserHandle)
		},
		"a passkey made at another address": func(f *passkeySignInFixture) (usecase.Output, error) {
			held := f.store.holders[string(adaCredential)]
			held.Credential.RPID = "old.acme.example"
			f.store.holders[string(adaCredential)] = held
			return f.signInAsAda(t)
		},
		"another user handle": func(f *passkeySignInFixture) (usecase.Output, error) {
			return f.signIn(t, f.started(t), adaCredential, bytes.Repeat([]byte{1}, 32))
		},
		"no user handle": func(f *passkeySignInFixture) (usecase.Output, error) {
			return f.signIn(t, f.started(t), adaCredential, nil)
		},
		"a refused ceremony": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.verifier.refuseSign = true
			return f.signInAsAda(t)
		},
		"no user verification": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.verifier.asserted.UserVerified = false
			return f.signInAsAda(t)
		},
		"a counter that went back": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.verifier.asserted.SignCount = 3
			return f.signInAsAda(t)
		},
		"a disabled account": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.holderStatus(domain.AccountDisabled)
			return f.signInAsAda(t)
		},
		"an invited account": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.holderStatus(domain.AccountInvited)
			return f.signInAsAda(t)
		},
		"an anonymised account": func(f *passkeySignInFixture) (usecase.Output, error) {
			f.holderStatus(domain.AccountAnonymized)
			return f.signInAsAda(t)
		},
		"a service account": func(f *passkeySignInFixture) (usecase.Output, error) {
			held := f.store.holders[string(adaCredential)]
			held.Account.Kind = domain.AccountServiceAccount
			f.store.holders[string(adaCredential)] = held
			return f.signInAsAda(t)
		},
		"unreadable client data": func(f *passkeySignInFixture) (usecase.Output, error) {
			return f.signIn(t, base64.RawURLEncoding.EncodeToString([]byte("unreadable")), adaCredential, f.ada.Credential.UserHandle)
		},
	}
	for name, attempt := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPasskeySignInFixture(t)
			_, err := attempt(f)
			refusedWith(t, err, "auth.sign_in_failed")
			if f.session.attempts.standing["ip:203.0.113.0/24"].Failures != 1 {
				t.Errorf("the refusal was not counted: %+v", f.session.attempts.standing)
			}
			if len(f.session.sessions.inserted) != 0 {
				t.Error("a session was opened")
			}
		})
	}
}

func (f *passkeySignInFixture) holderStatus(status domain.AccountStatus) {
	held := f.store.holders[string(adaCredential)]
	held.Account.Status = status
	f.store.holders[string(adaCredential)] = held
}

// A restricted person signs in, as at every door (Art. 18 restricts processing, not the person).
func TestARestrictedPersonSignsInWithAPasskey(t *testing.T) {
	f := newPasskeySignInFixture(t)
	f.holderStatus(domain.AccountRestricted)
	if _, err := f.signInAsAda(t); err != nil {
		t.Errorf("a restricted person was refused: %v", err)
	}
}

// UC-INS-08 check 2: a paused workspace keeps its sentence at this door too, once the passkey proved
// the person.
func TestASuspendedWorkspaceSaysSoAtThePasskeyDoor(t *testing.T) {
	f := newPasskeySignInFixture(t)
	held := f.store.holders[string(adaCredential)]
	held.TenantStatus = domain.TenantSuspended
	f.store.holders[string(adaCredential)] = held
	_, err := f.signInAsAda(t)
	refusedWith(t, err, "access.tenant_suspended")
}

// A challenge that ran out, was spent or never issued is not a guess: refused apart, uncounted, so
// the card asks again without the wrong password's sentence.
func TestAStaleChallengeIsRefusedApartAndUncounted(t *testing.T) {
	f := newPasskeySignInFixture(t)
	challenge := f.started(t)
	if _, err := f.signIn(t, challenge, adaCredential, f.ada.Credential.UserHandle); err != nil {
		t.Fatal(err)
	}
	for name, presented := range map[string]string{
		"spent":        challenge,
		"never issued": base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{0x77}, 32)),
		"short":        base64.RawURLEncoding.EncodeToString([]byte("short")),
	} {
		_, err := f.signIn(t, presented, adaCredential, f.ada.Credential.UserHandle)
		if err == nil || !strings.Contains(err.Error(), codeChallengeStale) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if failures := f.session.attempts.standing["ip:203.0.113.0/24"].Failures; failures != 0 {
		t.Errorf("a stale challenge was counted %d times", failures)
	}
}

// P-05: no options where no ceremony can finish, and none - nor a sign-in - where passkeys are off.
func TestPasskeysAreRefusedWhereTheyAreNoWayIn(t *testing.T) {
	f := newPasskeySignInFixture(t)
	_, err := f.options(t, "hubtask.example")
	refusedWith(t, err, codeNoRelyingParty)

	f.writer.Party.Installation = PasskeyInstallation{Multi: true}
	f.register(t)
	_, err = f.options(t, "acme.hubtask.example")
	refusedWith(t, err, codeNoRelyingParty)

	f = newPasskeySignInFixture(t)
	challenge := f.started(t)
	f.writer.Resolver = methodsResolver{methods: []string{domain.MethodDirect}}
	f.register(t)
	_, err = f.options(t, "acme.hubtask.example")
	refusedWith(t, err, codeNotOffered)
	_, err = f.signIn(t, challenge, adaCredential, f.ada.Credential.UserHandle)
	refusedWith(t, err, codeNotOffered)

	f = newPasskeySignInFixture(t)
	f.writer.Session.Multi = true
	f.register(t)
	_, err = f.registry.Invoke(t.Context(), StartPasskeySignInName, appshared.ActorContext{}, usecase.Input{
		"host": "nobody.hubtask.example",
	})
	refusedWith(t, err, "auth.tenant_unresolved")
}

// identity.md §13: a start password Hubtask drew is replaced at the next sign-in, by any door.
func TestAStartPasswordOwesTheChangeAfterAPasskey(t *testing.T) {
	f := newPasskeySignInFixture(t)
	held := f.store.holders[string(adaCredential)]
	held.StartPasswordAt = now.Add(-time.Hour)
	held.Account.Email, held.Account.SignInName = "", "lena"
	f.store.holders[string(adaCredential)] = held
	out, err := f.signInAsAda(t)
	if err != nil {
		t.Fatal(err)
	}
	methods, _ := out["methods"].([]any)
	if out["mfa_required"] != true || len(methods) != 1 || methods[0] != methodPasswordChange || out.String("email") != "lena" {
		t.Errorf("answered %v, want the change step naming lena", out)
	}
	if len(f.session.sessions.inserted) != 0 {
		t.Error("a session was opened before the change")
	}
}

// The ledger's lock at this door is the network's, checked before anything is read.
func TestALockedNetworkIsRefusedBeforeTheLookup(t *testing.T) {
	f := newPasskeySignInFixture(t)
	f.session.attempts.standing["ip:203.0.113.0/24"] = repository.AuthAttempt{
		Failures: 9, LockedUntil: now.Add(time.Hour),
	}
	_, err := f.signInAsAda(t)
	refusedWith(t, err, "auth.sign_in_locked")
	if len(f.verifier.assertions) != 0 {
		t.Error("a locked network's ceremony was verified")
	}
}
