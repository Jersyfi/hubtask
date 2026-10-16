// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/passkey"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
)

// UC-ID-22: a person's passkeys and security keys, through the registry as a client reaches them.

// passkeyStore keeps credentials in memory, per account, refusing an identifier held twice.
type passkeyStore struct {
	held   []domain.PasskeyCredential
	locked []shared.ID
}

var _ repository.PasskeyCredentials = (*passkeyStore)(nil)

func (s *passkeyStore) ListOf(_ context.Context, accountID shared.ID) ([]domain.PasskeyCredential, error) {
	var out []domain.PasskeyCredential
	for _, credential := range s.held {
		if credential.AccountID == accountID {
			out = append(out, credential)
		}
	}
	return out, nil
}

func (s *passkeyStore) CountOf(ctx context.Context, accountID shared.ID) (int, error) {
	held, _ := s.ListOf(ctx, accountID)
	return len(held), nil
}

func (s *passkeyStore) LockAccount(_ context.Context, accountID shared.ID) error {
	s.locked = append(s.locked, accountID)
	return nil
}

func (s *passkeyStore) Insert(_ context.Context, credential domain.PasskeyCredential) (bool, error) {
	for _, held := range s.held {
		if bytes.Equal(held.CredentialID, credential.CredentialID) {
			return false, nil
		}
	}
	s.held = append(s.held, credential)
	return true, nil
}

// challengeStore keeps registrations' challenges, spending them as the statement does.
type challengeStore struct {
	issued []issuedChallenge
}

type issuedChallenge struct {
	challenge domain.PasskeyChallenge
	token     string
	spent     bool
}

func (s *challengeStore) Issue(_ context.Context, challenge domain.PasskeyChallenge, presented domain.Token) error {
	for i := range s.issued {
		if s.issued[i].challenge.AccountID == challenge.AccountID {
			s.issued[i].spent = true
		}
	}
	s.issued = append(s.issued, issuedChallenge{challenge: challenge, token: presented.Secret()})
	return nil
}

func (s *challengeStore) Consume(
	_ context.Context, presented domain.Token, sessionID, accountID shared.ID, at time.Time,
) (domain.PasskeyChallenge, bool, error) {
	for i := range s.issued {
		held := &s.issued[i]
		if held.token == presented.Secret() && !held.spent && held.challenge.SessionID == sessionID &&
			held.challenge.AccountID == accountID && at.Before(held.challenge.ExpiresAt) {
			held.spent = true
			return held.challenge, true, nil
		}
	}
	return domain.PasskeyChallenge{}, false, nil
}

// verifierFake stands in for the adapter: the client data is the challenge itself, and the ceremony
// is accepted unless refused. What it was asked is kept, so a test sees the expectations the core
// handed over.
type verifierFake struct {
	refuse bool
	asked  []passkey.Registration
}

func (v *verifierFake) ChallengeOf(clientDataJSON []byte) ([]byte, error) {
	if bytes.HasPrefix(clientDataJSON, []byte("unreadable")) {
		return nil, shared.ErrValidation.WithDetail(passkey.CodeRegistrationInvalid)
	}
	return clientDataJSON, nil
}

func (v *verifierFake) VerifyRegistration(r passkey.Registration) (passkey.Credential, error) {
	v.asked = append(v.asked, r)
	if v.refuse {
		return passkey.Credential{}, shared.ErrValidation.WithDetail(passkey.CodeRegistrationInvalid)
	}
	return passkey.Credential{
		ID: r.RawID, PublicKey: []byte{0xa5, 0x01, 0x02}, Algorithm: -7, SignCount: 0,
		UserVerified: true, BackupEligible: true, BackedUp: true, Transports: r.Transports,
	}, nil
}

// steppingEntropy answers different bytes at every draw, as the real source does, so two starts
// issue two challenges.
type steppingEntropy struct{ draws byte }

func (e *steppingEntropy) Bytes(n int) ([]byte, error) {
	e.draws++
	return clock.FixedEntropy{Seed: e.draws * 16}.Bytes(n)
}

// tenantHostsFake answers the workspace's hosts.
type tenantHostsFake struct{ hosts []domain.TenantHost }

func (h tenantHostsFake) Insert(context.Context, domain.TenantHost) error { return nil }

func (h tenantHostsFake) List(context.Context) ([]domain.TenantHost, error) { return h.hosts, nil }

// noMethodStepUp is an account that holds nothing to prove itself afresh with.
type noMethodStepUp struct{ stepUpFake }

func (noMethodStepUp) Methods(context.Context, shared.ID, shared.ID) ([]stepupport.Method, error) {
	return nil, nil
}

type passkeyFixture struct {
	verifier   *verifierFake
	session    *sessionFixture
	store      *passkeyStore
	challenges *challengeStore
	stepUp     *stepUpFake
	writer     PasskeyWriter
	registry   *usecase.Registry
}

// newPasskeyFixture is Bert, a person in the workspace Acme of a multi-mode installation at
// hubtask.example, signed in on a live session, holding no passkey yet.
func newPasskeyFixture(t *testing.T) *passkeyFixture {
	t.Helper()
	session := mfaFixture(now)
	session.writer.Entropy = &steppingEntropy{}
	credential, _ := refreshCredential(now)
	session.sessions.sessions[sessionRowID] = repository.SessionCredential{
		TenantStatus: domain.TenantActive, Session: credential.Session, Account: credential.Account,
	}
	f := &passkeyFixture{
		session: session, store: &passkeyStore{}, challenges: &challengeStore{}, stepUp: &stepUpFake{satisfied: true},
		verifier: &verifierFake{},
	}
	f.writer = PasskeyWriter{
		Session: session.writer, Passkeys: f.store, Challenges: f.challenges, Verifier: f.verifier,
		Workspaces: &workspaceStore{row: domain.Workspace{
			Tenant: domain.Tenant{ID: tenant, Slug: "acme", DisplayName: "Acme"}, Version: 1,
		}},
		Installation: PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true},
		StepUp:       f.stepUp,
	}
	f.register(t)
	return f
}

func (f *passkeyFixture) register(t *testing.T) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil,
		ListOwnPasskeys{Writer: f.writer}.Descriptor(),
		StartPasskeyRegistration{Writer: f.writer}.Descriptor(),
		CompletePasskeyRegistration{Writer: f.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
}

func (f *passkeyFixture) holding(t *testing.T, count int) {
	t.Helper()
	for i := range count {
		f.store.held = append(f.store.held, domain.PasskeyCredential{
			ID:       shared.ID("01936f2a-7c1e-7000-8000-0000000000" + string(rune('a'+i/10)) + string(rune('0'+i%10))),
			TenantID: tenant, AccountID: account, CredentialID: []byte{byte(i + 1)},
			Name: "Key", CreatedAt: now.Add(time.Duration(i) * time.Minute),
		})
	}
}

func (f *passkeyFixture) list(t *testing.T, actor appshared.ActorContext) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), ListOwnPasskeysName, actor, usecase.Input{})
}

func TestAPersonsPasskeysAreListedByNameAndDateAndNothingElse(t *testing.T) {
	f := newPasskeyFixture(t)
	used := now.Add(time.Hour)
	f.store.held = []domain.PasskeyCredential{
		{ID: "01936f2a-7c1e-7000-8000-000000000001", TenantID: tenant, AccountID: account,
			CredentialID: []byte{1, 2, 3}, PublicKey: []byte{9}, UserHandle: []byte{7},
			Name: "iPhone", CreatedAt: now, LastUsedAt: used},
		{ID: "01936f2a-7c1e-7000-8000-000000000002", TenantID: tenant, AccountID: account,
			CredentialID: []byte{4}, Name: "Key in the drawer", CreatedAt: now.Add(time.Minute)},
		{ID: "01936f2a-7c1e-7000-8000-000000000003", TenantID: tenant, AccountID: "someone-else",
			CredentialID: []byte{5}, Name: "Not Bert's", CreatedAt: now},
	}
	out, err := f.list(t, signedInActor())
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 2 || rows[0].String("name") != "iPhone" || rows[1].String("name") != "Key in the drawer" {
		t.Fatalf("listed %v", rows)
	}
	if at, _ := rows[0]["last_used_at"].(time.Time); !at.Equal(used) {
		t.Errorf("last used %v", rows[0]["last_used_at"])
	}
	if _, held := rows[1]["last_used_at"]; held {
		t.Error("a passkey never used has a last use")
	}
	for _, row := range rows {
		for key := range row {
			if key != "id" && key != "name" && key != "created_at" && key != "last_used_at" {
				t.Errorf("a listed passkey carries %q", key)
			}
		}
	}
	if out["can_add"] != true || out["limit"] != domain.MaxPasskeys || out.String("rp_id") != "acme.hubtask.example" {
		t.Errorf("can_add %v, limit %v, rp_id %q", out["can_add"], out["limit"], out.String("rp_id"))
	}
	if _, held := out["cannot_add_reason"]; held {
		t.Error("a reason was given where adding is possible")
	}
}

// P-05: the list says another cannot be added exactly where starting would refuse, and why.
func TestTheListSaysWhyAnotherCannotBeAdded(t *testing.T) {
	cases := map[string]struct {
		arrange func(f *passkeyFixture)
		reason  string
		rpID    string
	}{
		"ten held": {func(f *passkeyFixture) { f.holding(t, domain.MaxPasskeys) }, codeLimitReached, "acme.hubtask.example"},
		"no public address": {func(f *passkeyFixture) {
			f.writer.Installation = PasskeyInstallation{Multi: true}
		}, codeNoRelyingParty, ""},
		"no fresh proof to give": {func(f *passkeyFixture) {
			f.writer.StepUp = &noMethodStepUp{}
		}, codeStepUpNoMethod, "acme.hubtask.example"},
		"no step-up wired": {func(f *passkeyFixture) { f.writer.StepUp = nil }, codeStepUpNoMethod, "acme.hubtask.example"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPasskeyFixture(t)
			c.arrange(f)
			f.register(t)
			out, err := f.list(t, signedInActor())
			if err != nil {
				t.Fatal(err)
			}
			if out["can_add"] != false || out.String("cannot_add_reason") != c.reason || out.String("rp_id") != c.rpID {
				t.Errorf("can_add %v, reason %q, rp_id %q", out["can_add"], out.String("cannot_add_reason"), out.String("rp_id"))
			}
		})
	}
}

// The RP ID is the workspace's canonical host: in single mode the installation's, and where the
// workspace holds an active canonical row, that row's (identity.md §23.3).
func TestTheRelyingPartyIsTheWorkspacesCanonicalHost(t *testing.T) {
	f := newPasskeyFixture(t)
	f.writer.Installation = PasskeyInstallation{Host: "tasks.example.org", Scheme: "https", Port: "8443"}
	party, ok := relyingPartyOf(t, f)
	if !ok || party.ID != "tasks.example.org" || party.Origin != "https://tasks.example.org:8443" || party.Name != "Acme" {
		t.Errorf("single mode: %+v %v", party, ok)
	}

	f.writer.Installation = PasskeyInstallation{Host: "hubtask.example", Scheme: "https", Multi: true}
	f.writer.Hosts = tenantHostsFake{hosts: []domain.TenantHost{
		{Host: "old.acme.example", State: domain.HostBroken, Canonical: true},
		{Host: "acme.hubtask.example", State: domain.HostActive, Canonical: true},
	}}
	if party, ok := relyingPartyOf(t, f); !ok || party.ID != "acme.hubtask.example" {
		t.Errorf("an active canonical row: %+v %v", party, ok)
	}
	f.writer.Hosts = tenantHostsFake{}
	if party, ok := relyingPartyOf(t, f); !ok || party.ID != "acme.hubtask.example" {
		t.Errorf("no row: %+v %v", party, ok)
	}
}

func relyingPartyOf(t *testing.T, f *passkeyFixture) (relyingParty, bool) {
	t.Helper()
	party, ok, err := f.writer.relyingParty(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return party, ok
}

func TestOnlyAPersonHoldsPasskeys(t *testing.T) {
	f := newPasskeyFixture(t)
	for _, kind := range []appshared.ActorKind{
		appshared.ActorServiceAccount, appshared.ActorAutomation, appshared.ActorAIAgent, appshared.ActorSystem,
	} {
		actor := signedInActor()
		actor.Kind = kind
		_, err := f.list(t, actor)
		refusedWith(t, err, codePersonOnly)
	}
	// A service account's own row, whatever kind its credential claims.
	f.session.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountServiceAccount, DisplayName: "Robot", Status: domain.AccountActive,
	})
	f.writer.Session = f.session.writer
	f.register(t)
	_, err := f.list(t, signedInActor())
	refusedWith(t, err, codePersonOnly)

	_, err = f.list(t, appshared.ActorContext{})
	refusedWith(t, err, "access.credential_required")
}

func (f *passkeyFixture) start(t *testing.T, actor appshared.ActorContext, proof string) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), StartPasskeyRegistrationName, actor, usecase.Input{"step_up_token": proof})
}

// UC-ID-22 check 2, the first half: a fresh proof, then the options the device is asked with.
func TestStartingAsksAFreshProofThenAnswersTheOptions(t *testing.T) {
	f := newPasskeyFixture(t)
	f.holding(t, 1)
	f.store.held[0].UserHandle = bytes.Repeat([]byte{0x33}, domain.PasskeyUserHandleBytes)
	f.store.held[0].Transports = []string{"usb"}

	out, err := f.start(t, signedInActor(), "hbt_sup_fresh")
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	if len(f.stepUp.presented) != 1 || f.stepUp.presented[0] != "hbt_sup_fresh" {
		t.Errorf("the step-up saw %v", f.stepUp.presented)
	}
	options, _ := out["public_key"].(usecase.Output)
	rp, _ := options["rp"].(usecase.Output)
	user, _ := options["user"].(usecase.Output)
	if rp.String("id") != "acme.hubtask.example" || rp.String("name") != "Acme" {
		t.Errorf("relying party %v", rp)
	}
	// The account's handle, not a new one, and never the address.
	if user.String("id") != base64.RawURLEncoding.EncodeToString(f.store.held[0].UserHandle) ||
		user.String("name") != "bert@example.org" || user.String("displayName") != "Bert" {
		t.Errorf("user %v", user)
	}
	challenge, err := base64.RawURLEncoding.DecodeString(options.String("challenge"))
	if err != nil || len(challenge) != domain.TokenSecretBytes {
		t.Errorf("challenge %q", options.String("challenge"))
	}
	params, _ := options["pubKeyCredParams"].([]any)
	if len(params) != 3 || params[0].(usecase.Output)["alg"] != -7 || params[1].(usecase.Output)["alg"] != -8 ||
		params[2].(usecase.Output)["alg"] != -257 {
		t.Errorf("algorithms %v", params)
	}
	exclude, _ := options["excludeCredentials"].([]any)
	if len(exclude) != 1 || exclude[0].(usecase.Output).String("id") != base64.RawURLEncoding.EncodeToString([]byte{1}) {
		t.Errorf("exclude %v", exclude)
	}
	selection, _ := options["authenticatorSelection"].(usecase.Output)
	if selection.String("residentKey") != "preferred" || selection.String("userVerification") != "preferred" ||
		options.String("attestation") != "none" || options["timeout"] != 300000 {
		t.Errorf("selection %v, attestation %v, timeout %v", selection, options["attestation"], options["timeout"])
	}

	// The challenge waits bound to this session and this account, five minutes, under its own prefix,
	// and the account was locked for the count.
	if len(f.challenges.issued) != 1 {
		t.Fatalf("%d challenges issued", len(f.challenges.issued))
	}
	issued := f.challenges.issued[0]
	if issued.challenge.SessionID != sessionRowID || issued.challenge.AccountID != account ||
		!issued.challenge.ExpiresAt.Equal(now.Add(5*time.Minute)) ||
		!strings.HasPrefix(issued.token, domain.PasskeyChallengeTokenPrefix) ||
		!strings.HasSuffix(issued.token, options.String("challenge")) {
		t.Errorf("issued %+v as %q", issued.challenge, issued.token)
	}
	if len(f.store.locked) != 1 {
		t.Errorf("the account was locked %d times", len(f.store.locked))
	}
	if entries := f.session.audit.entries; len(entries) != 1 || entries[0].Action != PasskeyRegistrationStartedAction {
		t.Errorf("audited %+v", entries)
	}
}

func TestAFirstPasskeyGetsANewHandleAndAManagedAccountItsSignInName(t *testing.T) {
	f := newPasskeyFixture(t)
	f.session.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser, SignInName: "lena",
		DisplayName: "Lena", Status: domain.AccountActive,
	})
	f.writer.Session = f.session.writer
	f.register(t)
	out, err := f.start(t, signedInActor(), "hbt_sup_fresh")
	if err != nil {
		t.Fatal(err)
	}
	user, _ := out["public_key"].(usecase.Output)["user"].(usecase.Output)
	handle, err := base64.RawURLEncoding.DecodeString(user.String("id"))
	if err != nil || len(handle) != domain.PasskeyUserHandleBytes || user.String("name") != "lena" {
		t.Errorf("user %v", user)
	}
	if !bytes.Equal(f.challenges.issued[0].challenge.UserHandle, handle) {
		t.Error("the challenge carries another handle than the options")
	}
}

func TestStartingIsRefusedWhereItCannotEndInAPasskey(t *testing.T) {
	cases := map[string]struct {
		arrange   func(f *passkeyFixture)
		actor     func() appshared.ActorContext
		code      string
		proofUsed bool
	}{
		"no proof": {func(f *passkeyFixture) { f.stepUp.satisfied = false }, signedInActor, stepupport.CodeRequired, true},
		"ten held, before the proof": {func(f *passkeyFixture) { f.holding(t, domain.MaxPasskeys) }, signedInActor,
			codeLimitReached, false},
		"no public address, before the proof": {func(f *passkeyFixture) { f.writer.Installation = PasskeyInstallation{} },
			signedInActor, codeNoRelyingParty, false},
		"a personal token, not a session": {func(*passkeyFixture) {}, func() appshared.ActorContext {
			actor := signedInActor()
			actor.TokenID = "01936f2a-7c1e-7000-8000-00000000dead"
			return actor
		}, "auth.step_up_session_required", false},
		"an automation": {func(*passkeyFixture) {}, func() appshared.ActorContext {
			actor := signedInActor()
			actor.Kind = appshared.ActorAutomation
			return actor
		}, codePersonOnly, false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			f := newPasskeyFixture(t)
			c.arrange(f)
			f.register(t)
			_, err := f.start(t, c.actor(), "hbt_sup_fresh")
			refusedWith(t, err, c.code)
			if used := len(f.stepUp.presented) > 0; used != c.proofUsed {
				t.Errorf("the proof was presented: %v", used)
			}
			if len(f.challenges.issued) != 0 {
				t.Error("a challenge was issued")
			}
		})
	}
}

// The newest start wins: an older challenge of the account cannot finish.
func TestASecondStartSpendsTheFirst(t *testing.T) {
	f := newPasskeyFixture(t)
	for range 2 {
		if _, err := f.start(t, signedInActor(), "hbt_sup_fresh"); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.challenges.issued) != 2 || !f.challenges.issued[0].spent || f.challenges.issued[1].spent {
		t.Errorf("issued %+v", f.challenges.issued)
	}
	if len(f.stepUp.presented) != 2 {
		t.Errorf("a second start asked no new proof: %v", f.stepUp.presented)
	}
}

// started runs the first half and answers the challenge the device would sign, base64url.
func (f *passkeyFixture) started(t *testing.T) string {
	t.Helper()
	out, err := f.start(t, signedInActor(), "hbt_sup_fresh")
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	return out["public_key"].(usecase.Output).String("challenge")
}

func (f *passkeyFixture) complete(
	t *testing.T, challenge, name string, rawID byte,
) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), CompletePasskeyRegistrationName, signedInActor(), usecase.Input{
		"name": name, "raw_id": base64.RawURLEncoding.EncodeToString([]byte{rawID, rawID}),
		"client_data_json": challenge, "attestation_object": "o2NmbXRkbm9uZQ",
		"transports": []any{"internal", "hybrid"},
	})
}

// UC-ID-22 checks 2 and 6: the device's answer and the name keep the passkey, and the first factor of
// an account without codes shows the ten.
func TestAFirstPasskeyIsKeptAndAnswersTheRecoveryCodes(t *testing.T) {
	f := newPasskeyFixture(t)
	challenge := f.started(t)
	out, err := f.complete(t, challenge, "  iPhone ", 0x61)
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	passkeyOut, _ := out["passkey"].(usecase.Output)
	if passkeyOut.String("name") != "iPhone" {
		t.Errorf("answered %v", passkeyOut)
	}
	codes, _ := out["recovery_codes"].([]any)
	if len(codes) != domain.RecoveryCodeCount {
		t.Fatalf("%d recovery codes answered, want ten", len(codes))
	}
	recovery := f.session.writer.Recovery.(*recoveryStore)
	if len(recovery.codes) != domain.RecoveryCodeCount {
		t.Errorf("%d codes stored", len(recovery.codes))
	}

	// What was kept, with the handle the start chose and the name trimmed.
	if len(f.store.held) != 1 {
		t.Fatalf("%d credentials held", len(f.store.held))
	}
	held := f.store.held[0]
	if held.Name != "iPhone" || !bytes.Equal(held.UserHandle, f.challenges.issued[0].challenge.UserHandle) ||
		!bytes.Equal(held.CredentialID, []byte{0x61, 0x61}) || held.AccountID != account ||
		len(held.Transports) != 2 || !held.CreatedAt.Equal(now) {
		t.Errorf("held %+v", held)
	}
	// The verifier was asked against this workspace and the challenge the start issued.
	asked := f.verifier.asked[0]
	if asked.RelyingParty.ID != "acme.hubtask.example" || asked.RelyingParty.Origin != "https://acme.hubtask.example" ||
		base64.RawURLEncoding.EncodeToString(asked.Challenge) != challenge {
		t.Errorf("verified against %+v", asked)
	}
	// The trail: the start and the addition, neither with the name (rule 10).
	entries := f.session.audit.entries
	if len(entries) != 2 || entries[1].Action != PasskeyAddedAction {
		t.Fatalf("audited %+v", entries)
	}
	for _, entry := range entries {
		if strings.Contains(fmt.Sprintf("%+v", entry), "iPhone") {
			t.Errorf("an audit entry carries the name: %+v", entry)
		}
	}

	// A second passkey keeps the codes the account already holds.
	out, err = f.complete(t, f.started(t), "Key in the drawer", 0x62)
	if err != nil {
		t.Fatal(err)
	}
	if _, held := out["recovery_codes"]; held || recovery.replacements != 1 {
		t.Errorf("the second passkey answered codes (%d replacements)", recovery.replacements)
	}
}

func TestACompletionIsRefusedWhereItIsNotTheSessionsOwnFreshAttempt(t *testing.T) {
	t.Run("never started", func(t *testing.T) {
		f := newPasskeyFixture(t)
		_, err := f.complete(t, base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{9}, 32)), "Key", 1)
		refusedWith(t, err, codeChallengeStale)
	})
	t.Run("spent once already", func(t *testing.T) {
		f := newPasskeyFixture(t)
		challenge := f.started(t)
		if _, err := f.complete(t, challenge, "Key", 1); err != nil {
			t.Fatal(err)
		}
		_, err := f.complete(t, challenge, "Key", 2)
		refusedWith(t, err, codeChallengeStale)
	})
	t.Run("superseded by a newer start", func(t *testing.T) {
		f := newPasskeyFixture(t)
		older := f.started(t)
		f.started(t)
		_, err := f.complete(t, older, "Key", 1)
		refusedWith(t, err, codeChallengeStale)
	})
	t.Run("another session's", func(t *testing.T) {
		f := newPasskeyFixture(t)
		challenge := f.started(t)
		f.challenges.issued[0].challenge.SessionID = "01936f2a-7c1e-7000-8000-0000000000ff"
		_, err := f.complete(t, challenge, "Key", 1)
		refusedWith(t, err, codeChallengeStale)
	})
	t.Run("a challenge of another length", func(t *testing.T) {
		f := newPasskeyFixture(t)
		_, err := f.complete(t, base64.RawURLEncoding.EncodeToString([]byte("short")), "Key", 1)
		refusedWith(t, err, codeChallengeStale)
	})
}

func TestARefusedCeremonySpendsTheChallengeAndKeepsNothing(t *testing.T) {
	f := newPasskeyFixture(t)
	f.verifier.refuse = true
	_, err := f.complete(t, f.started(t), "Key", 1)
	refusedWith(t, err, passkey.CodeRegistrationInvalid)
	if !f.challenges.issued[0].spent || len(f.store.held) != 0 {
		t.Errorf("spent %v, held %d", f.challenges.issued[0].spent, len(f.store.held))
	}

	// Unreadable client data names no challenge, so nothing is spent.
	f = newPasskeyFixture(t)
	f.started(t)
	_, err = f.complete(t, base64.RawURLEncoding.EncodeToString([]byte("unreadable")), "Key", 1)
	refusedWith(t, err, passkey.CodeRegistrationInvalid)
	if f.challenges.issued[0].spent {
		t.Error("unreadable client data spent the challenge")
	}
	_, err = f.registry.Invoke(t.Context(), CompletePasskeyRegistrationName, signedInActor(), usecase.Input{
		"name": "Key", "raw_id": "not base64url!", "client_data_json": "AQ", "attestation_object": "AQ",
	})
	refusedWith(t, err, passkey.CodeRegistrationInvalid)
}

// A name the server refuses leaves the challenge for a second try with a better one.
func TestAnInvalidNameSpendsNothing(t *testing.T) {
	f := newPasskeyFixture(t)
	challenge := f.started(t)
	_, err := f.complete(t, challenge, strings.Repeat("x", domain.MaxPasskeyNameLength+1), 1)
	refusedWith(t, err, "auth.passkey_name_invalid")
	if f.challenges.issued[0].spent {
		t.Fatal("an invalid name spent the challenge")
	}
	if _, err := f.complete(t, challenge, "Laptop", 1); err != nil {
		t.Errorf("the second try: %v", err)
	}
}

func TestACredentialHeldOrOneTooManyIsRefused(t *testing.T) {
	f := newPasskeyFixture(t)
	if _, err := f.complete(t, f.started(t), "Key", 7); err != nil {
		t.Fatal(err)
	}
	_, err := f.complete(t, f.started(t), "Same device", 7)
	refusedWith(t, err, codeAlreadyRegistered)

	// UC-ID-22 check 3: the eleventh, reached while this ceremony ran.
	f = newPasskeyFixture(t)
	f.holding(t, domain.MaxPasskeys-1)
	challenge := f.started(t)
	f.holding(t, 1)
	f.store.held[len(f.store.held)-1].CredentialID = []byte{0xee}
	_, err = f.complete(t, challenge, "The eleventh", 0x70)
	refusedWith(t, err, codeLimitReached)
	if len(f.store.held) != domain.MaxPasskeys {
		t.Errorf("%d held", len(f.store.held))
	}
}
