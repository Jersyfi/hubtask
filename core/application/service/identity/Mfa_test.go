// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// enrollmentsStore is MfaEnrollments in memory, with the adapter's atomic guards written out so
// the services are proved against the same rules the statements carry.
type enrollmentsStore struct {
	rows map[shared.ID]*repository.MfaEnrollment
}

func newEnrollments() *enrollmentsStore {
	return &enrollmentsStore{rows: map[shared.ID]*repository.MfaEnrollment{}}
}

func (s *enrollmentsStore) Upsert(
	_ context.Context, accountID shared.ID, sealed cryptoport.Sealed, _ time.Time,
) (bool, error) {
	if existing, ok := s.rows[accountID]; ok && !existing.ConfirmedAt.IsZero() {
		return false, nil
	}
	s.rows[accountID] = &repository.MfaEnrollment{AccountID: accountID, Secret: sealed}
	return true, nil
}

func (s *enrollmentsStore) Find(_ context.Context, accountID shared.ID) (repository.MfaEnrollment, error) {
	row, ok := s.rows[accountID]
	if !ok {
		return repository.MfaEnrollment{}, shared.ErrNotFound.WithDetail("auth.mfa_not_enrolled")
	}
	return *row, nil
}

func (s *enrollmentsStore) Confirm(
	_ context.Context, accountID shared.ID, step int64, at time.Time,
) (bool, error) {
	row, ok := s.rows[accountID]
	if !ok || !row.ConfirmedAt.IsZero() {
		return false, nil
	}
	row.ConfirmedAt, row.LastStep = at, step
	return true, nil
}

func (s *enrollmentsStore) RecordStep(
	_ context.Context, accountID shared.ID, step int64, _ time.Time,
) (bool, error) {
	row, ok := s.rows[accountID]
	if !ok || row.ConfirmedAt.IsZero() || row.LastStep >= step {
		return false, nil
	}
	row.LastStep = step
	return true, nil
}

func (s *enrollmentsStore) Disable(_ context.Context, accountID shared.ID) (bool, error) {
	if _, ok := s.rows[accountID]; !ok {
		return false, nil
	}
	delete(s.rows, accountID)
	return true, nil
}

type recoveryStore struct {
	codes map[string]bool // normalised code -> live
}

func newRecovery() *recoveryStore { return &recoveryStore{codes: map[string]bool{}} }

func (s *recoveryStore) Replace(
	_ context.Context, _ shared.ID, _ []shared.ID, presented []string, _ time.Time,
) error {
	s.codes = map[string]bool{}
	for _, code := range presented {
		s.codes[domain.NormalizeRecoveryCode(code)] = true
	}
	return nil
}

func (s *recoveryStore) Burn(_ context.Context, _ shared.ID, presented string, _ time.Time) (bool, error) {
	key := domain.NormalizeRecoveryCode(presented)
	if !s.codes[key] {
		return false, nil
	}
	s.codes[key] = false
	return true, nil
}

func (s *recoveryStore) Remaining(context.Context, shared.ID) (int, error) {
	left := 0
	for _, live := range s.codes {
		if live {
			left++
		}
	}
	return left, nil
}

type pendingStore struct {
	rows map[string]repository.PendingLookup // presented token -> lookup
}

func newPending() *pendingStore { return &pendingStore{rows: map[string]repository.PendingLookup{}} }

func (s *pendingStore) Insert(
	_ context.Context, credential domain.PendingCredential, presented domain.Token,
) error {
	s.rows[presented.Secret()] = repository.PendingLookup{
		TenantStatus: domain.TenantActive,
		Credential:   credential,
		Account: domain.Account{
			ID: credential.AccountID, TenantID: credential.TenantID,
			Kind: domain.AccountUser, DisplayName: "Bert", Status: domain.AccountActive,
		},
	}
	return nil
}

func (s *pendingStore) FindByToken(_ context.Context, token domain.Token) (repository.PendingLookup, error) {
	lookup, ok := s.rows[token.Secret()]
	if !ok {
		return repository.PendingLookup{}, shared.ErrNotFound.WithDetail("auth.mfa_challenge_failed")
	}
	return lookup, nil
}

func (s *pendingStore) Consume(_ context.Context, credentialID shared.ID, at time.Time) (bool, error) {
	for key, lookup := range s.rows {
		if lookup.Credential.ID == credentialID {
			if !lookup.Credential.ConsumedAt.IsZero() {
				return false, nil
			}
			lookup.Credential.ConsumedAt = at
			s.rows[key] = lookup
			return true, nil
		}
	}
	return false, nil
}

type policyFake struct{ required bool }

func (p policyFake) RequireAdminTotp(context.Context) (bool, error) { return p.required, nil }

type membershipsFake struct{ roles []domain.Role }

func (m membershipsFake) Along(context.Context, shared.ID, []domain.Scope) ([]domain.Membership, error) {
	held := make([]domain.Membership, 0, len(m.roles))
	for _, role := range m.roles {
		held = append(held, domain.Membership{Role: role, Scope: domain.TenantScope()})
	}
	return held, nil
}

func (m membershipsFake) SharedItemsIn(context.Context, shared.ID, shared.ID) ([]shared.ID, error) {
	return nil, nil
}

func (m membershipsFake) Administrators(context.Context, []domain.Scope) ([]shared.ID, error) {
	return nil, nil
}

// encryptorFake seals by remembering the plaintext under the purpose - which also proves the
// purpose binding: opening under another purpose fails as the real envelope would.
type encryptorFake struct {
	sealedBy map[string]string
}

func newEncryptor() *encryptorFake { return &encryptorFake{sealedBy: map[string]string{}} }

func (e *encryptorFake) Seal(
	_ context.Context, plaintext secret.Secret, purpose cryptoport.Purpose,
) (cryptoport.Sealed, error) {
	e.sealedBy[string(purpose)] = plaintext.Reveal()
	return cryptoport.Sealed{KeyID: "k1", Ciphertext: []byte(string(purpose))}, nil
}

func (e *encryptorFake) Open(
	_ context.Context, sealed cryptoport.Sealed, purpose cryptoport.Purpose,
) (secret.Secret, error) {
	if string(sealed.Ciphertext) != string(purpose) {
		return secret.Secret{}, cryptoport.NotAuthentic()
	}
	return secret.New(e.sealedBy[string(purpose)]), nil
}

func (e *encryptorFake) ActiveKeyID() string { return "k1" }

func (e *encryptorFake) KeyIDs() []string { return []string{"k1"} }

func (e *encryptorFake) Rewrap(_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose) (cryptoport.Sealed, error) {
	return cryptoport.Sealed{KeyID: "k1", Ciphertext: sealed.Ciphertext}, nil
}

// mfaFixture is the session fixture with the second factor wired.
func mfaFixture(at time.Time) *sessionFixture {
	fixture := newSessionFixture(at)
	fixture.writer.Enrollments = newEnrollments()
	fixture.writer.Recovery = newRecovery()
	fixture.writer.Pending = newPending()
	fixture.writer.Encryptor = newEncryptor()
	fixture.writer.Policy = policyFake{}
	fixture.writer.Memberships = membershipsFake{}
	fixture.writer.People = newAccounts(domain.Account{
		ID: account, TenantID: tenant, Kind: domain.AccountUser,
		Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
	})
	return fixture
}

func signedInActor() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: shared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Bert", TokenID: sessionRowID,
	}
}

// enrolled arms an enrolment through the real flow and answers the authenticator's secret.
func enrolled(t *testing.T, fixture *sessionFixture) []byte {
	t.Helper()
	minted, err := EnrollTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), EnrollTotpCommand{})
	if err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	material := fixture.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]

	code := domain.TotpCode([]byte(material), domain.TotpStep(now))
	if _, err := (ConfirmTotp{Writer: fixture.writer}).Execute(
		t.Context(), signedInActor(), ConfirmTotpCommand{Code: code},
	); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	_ = minted
	return []byte(material)
}

func TestEnrolmentArmsOnlyOnConfirmation(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")

	minted, err := EnrollTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), EnrollTotpCommand{})
	if err != nil {
		t.Fatalf("enrolling: %v", err)
	}
	if minted.Secret.IsEmpty() || minted.URI.IsEmpty() || len(minted.RecoveryCodes) != domain.RecoveryCodeCount {
		t.Fatalf("the single showing is incomplete: %d codes", len(minted.RecoveryCodes))
	}
	if !strings.Contains(minted.URI.Reveal(), "otpauth://totp/") ||
		!strings.Contains(minted.URI.Reveal(), "bert@example.org") {
		t.Errorf("uri %q", minted.URI.Reveal())
	}

	// Unconfirmed protects nobody: sign-in stays one-step.
	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("an unconfirmed enrolment changed sign-in: %+v %v", result, err)
	}

	// A wrong code does not arm.
	_, err = ConfirmTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		ConfirmTotpCommand{Code: "000000"})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("a wrong code answered %v", err)
	}

	// The right code arms, and sign-in becomes two-step.
	material := fixture.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	code := domain.TotpCode([]byte(material), domain.TotpStep(now))
	if _, err := (ConfirmTotp{Writer: fixture.writer}).Execute(
		t.Context(), signedInActor(), ConfirmTotpCommand{Code: code},
	); err != nil {
		t.Fatalf("confirming: %v", err)
	}
	result, err = SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if err != nil || result.Challenge == nil {
		t.Fatalf("an armed enrolment did not challenge: %+v %v", result, err)
	}
	if len(result.Challenge.Methods) != 2 || result.Challenge.Methods[0] != methodTotp {
		t.Errorf("methods %v, want TOTP and RECOVERY", result.Challenge.Methods)
	}
}

func TestEnrollingWhileArmedIsRefused(t *testing.T) {
	fixture := mfaFixture(now)
	enrolled(t, fixture)

	_, err := EnrollTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(), EnrollTotpCommand{})
	if !errors.Is(err, shared.ErrConflict) || !strings.Contains(err.Error(), "auth.mfa_already_armed") {
		t.Fatalf("enrolling over an armed factor answered %v", err)
	}
}

func TestTheTwoStepSignInCompletesWithACode(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	material := enrolled(t, fixture)

	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
		UserAgent: "hubctl/1.0", RemoteAddr: "203.0.113.7:9",
	})
	if err != nil || result.Challenge == nil {
		t.Fatalf("no challenge: %+v %v", result, err)
	}

	// The confirming step is spent; the next window's code completes.
	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.writer
	completer.Clock = clock.Fixed(later)
	pair, remaining, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token,
		Code:         domain.TotpCode(material, domain.TotpStep(later)),
	})
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if remaining != -1 {
		t.Errorf("remaining %d, want the no-recovery marker", remaining)
	}
	if pair.RefreshToken.IsEmpty() {
		t.Fatal("no pair answered")
	}
	// The session records the sign-in's own device, not the completion call's.
	if len(fixture.sessions.inserted) != 1 || fixture.sessions.inserted[0].UserAgent != "hubctl/1.0" {
		t.Errorf("sessions %+v, want the sign-in's client hint", fixture.sessions.inserted)
	}

	// The pending credential died on use.
	if _, _, err := (CompleteSignIn{Writer: completer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: result.Challenge.Token,
		Code:         domain.TotpCode(material, domain.TotpStep(later)+1),
	}); err == nil {
		t.Fatal("a consumed pending credential completed a second sign-in")
	}
}

// The same code never verifies twice (H-02): the replay floor advances with the acceptance.
func TestTheSameCodeNeverCompletesTwice(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	material := enrolled(t, fixture)

	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.writer
	completer.Clock = clock.Fixed(later)
	code := domain.TotpCode(material, domain.TotpStep(later))

	first := signInChallenge(t, fixture)
	if _, _, err := (CompleteSignIn{Writer: completer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: first, Code: code,
	}); err != nil {
		t.Fatalf("the first completion failed: %v", err)
	}

	second := signInChallenge(t, fixture)
	_, _, err := CompleteSignIn{Writer: completer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: second, Code: code,
	})
	if err == nil || !strings.Contains(err.Error(), "auth.mfa_code_invalid") {
		t.Fatalf("the same code completed twice: %v", err)
	}
}

func TestARecoveryCodeWorksOnceAndCountsDown(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	enrolled(t, fixture)
	codes := recoveryCodesOf(fixture)

	challenge := signInChallenge(t, fixture)
	pair, remaining, err := CompleteSignIn{Writer: fixture.writer}.Execute(t.Context(), CompleteSignInCommand{
		PendingToken: challenge,
		RecoveryCode: secret.New(strings.ToLower(codes[0])),
	})
	if err != nil || pair.RefreshToken.IsEmpty() {
		t.Fatalf("a recovery completion failed: %v", err)
	}
	if remaining != domain.RecoveryCodeCount-1 {
		t.Errorf("remaining %d, want %d", remaining, domain.RecoveryCodeCount-1)
	}

	burned := false
	for _, entry := range fixture.audit.entries {
		if entry.Action == RecoveryCodeUsedAction && entry.Severity == audit.SeverityWarning {
			burned = true
		}
	}
	if !burned {
		t.Error("the consumption was not audited")
	}

	// Once means once.
	again := signInChallenge(t, fixture)
	if _, _, err := (CompleteSignIn{Writer: fixture.writer}).Execute(t.Context(), CompleteSignInCommand{
		PendingToken: again, RecoveryCode: secret.New(codes[0]),
	}); err == nil {
		t.Fatal("a burned recovery code completed a sign-in")
	}
}

// The pending credential can reach no other route, proved by trying (H-02's acceptance).
func TestThePendingCredentialCanDoNothingElse(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	enrolled(t, fixture)
	challenge := signInChallenge(t, fixture)

	// Not a bearer credential: it has the wrong shape for the middleware by construction, and
	// the refresh and redemption parsers refuse it as they refuse anything foreign.
	if _, err := domain.ParseRefreshToken(challenge.Reveal()); err == nil {
		t.Error("a pending token parsed as a refresh token")
	}
	if _, err := domain.ParseRedemptionToken(challenge.Reveal()); err == nil {
		t.Error("a pending token parsed as a redemption token")
	}
	// A TOTP credential cannot open the enrolment routes: a stolen password must not replace
	// the very factor that stops it.
	_, err := EnrollTotp{Writer: fixture.writer}.Execute(t.Context(),
		appshared.Anonymous("en", "UTC"), EnrollTotpCommand{PendingToken: challenge})
	if err == nil {
		t.Fatal("a TOTP pending credential enrolled")
	}
}

// The tenant switch routes an unenrolled ADMIN into enrolment and leaves a MEMBER untouched.
func TestEnforcementRoutesAdminsIntoEnrolment(t *testing.T) {
	admin := mfaFixture(now)
	admin.withAccount("bert@example.org", "correct horse battery")
	admin.writer.Policy = policyFake{required: true}
	admin.writer.Memberships = membershipsFake{roles: []domain.Role{domain.RoleAdmin}}

	result, err := SignIn{Writer: admin.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if err != nil || result.Challenge == nil {
		t.Fatalf("no challenge for the unenrolled admin: %+v %v", result, err)
	}
	if len(result.Challenge.Methods) != 1 || result.Challenge.Methods[0] != methodEnroll {
		t.Fatalf("methods %v, want ENROLL alone", result.Challenge.Methods)
	}

	// The whole enforcement flow: enrol with the pending credential, confirm, receive the pair.
	minted, err := EnrollTotp{Writer: admin.writer}.Execute(t.Context(),
		appshared.Anonymous("en", "UTC"), EnrollTotpCommand{PendingToken: result.Challenge.Token})
	if err != nil {
		t.Fatalf("enrolling through the pending credential: %v", err)
	}
	_ = minted
	material := admin.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	confirmed, err := ConfirmTotp{Writer: admin.writer}.Execute(t.Context(),
		appshared.Anonymous("en", "UTC"), ConfirmTotpCommand{
			PendingToken: result.Challenge.Token,
			Code:         domain.TotpCode([]byte(material), domain.TotpStep(now)),
		})
	if err != nil {
		t.Fatalf("confirming through the pending credential: %v", err)
	}
	if confirmed.Pair == nil {
		t.Fatal("the enforcement confirmation opened no session")
	}

	member := mfaFixture(now)
	member.withAccount("carol@example.org", "another right password")
	member.writer.Policy = policyFake{required: true}
	member.writer.Memberships = membershipsFake{roles: []domain.Role{domain.RoleMember}}
	result, err = SignIn{Writer: member.writer}.Execute(t.Context(), SignInCommand{
		Email: "carol@example.org", Password: secret.New("another right password"),
	})
	if err != nil || result.Pair == nil {
		t.Fatalf("a MEMBER was challenged under the admin switch: %+v %v", result, err)
	}
}

func TestDisableDemandsTheFreshPasswordAndTheSwitchWins(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	enrolled(t, fixture)

	// The wrong password is the generic refusal, and nothing is disabled.
	err := DisableTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New("a wrong password!!")})
	if err == nil || !strings.Contains(err.Error(), "auth.sign_in_failed") {
		t.Fatalf("a wrong password answered %v", err)
	}

	// Under the switch, an admin cannot disable at all.
	fixture.writer.Policy = policyFake{required: true}
	fixture.writer.Memberships = membershipsFake{roles: []domain.Role{domain.RoleOwner}}
	err = DisableTotp{Writer: fixture.writer}.Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New("correct horse battery")})
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "auth.mfa_required_by_tenant") {
		t.Fatalf("the switch answered %v", err)
	}

	// Off the switch, the fresh password disables, and it is audited.
	fixture.writer.Policy = policyFake{}
	if err := (DisableTotp{Writer: fixture.writer}).Execute(t.Context(), signedInActor(),
		DisableTotpCommand{Password: secret.New("correct horse battery")}); err != nil {
		t.Fatalf("disabling: %v", err)
	}
	disabled := false
	for _, entry := range fixture.audit.entries {
		if entry.Action == MfaDisabledAction {
			disabled = true
		}
	}
	if !disabled {
		t.Error("the disable was not audited")
	}
	if _, err := fixture.writer.Enrollments.Find(t.Context(), account); !errors.Is(err, shared.ErrNotFound) {
		t.Error("the enrolment survived the disable")
	}
}

// signInChallenge runs the first step and answers the pending token.
func signInChallenge(t *testing.T, fixture *sessionFixture) secret.Secret {
	t.Helper()
	result, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
		UserAgent: "hubctl/1.0", RemoteAddr: "203.0.113.7:9",
	})
	if err != nil || result.Challenge == nil {
		t.Fatalf("no challenge: %+v %v", result, err)
	}
	return result.Challenge.Token
}

// recoveryCodesOf reads the live codes back out of the fake, in minted form.
func recoveryCodesOf(fixture *sessionFixture) []string {
	store := fixture.writer.Recovery.(*recoveryStore)
	codes := make([]string, 0, len(store.codes))
	for code, live := range store.codes {
		if live {
			codes = append(codes, code)
		}
	}
	return codes
}

// The channel round trip: every H-01/H-02 use case invoked through its descriptor, the way REST,
// MCP and automation all reach it - which is also what proves the projections each channel reads.
func TestTheAuthUseCasesRoundTripThroughTheRegistry(t *testing.T) {
	fixture := mfaFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")

	registry, err := usecase.NewRegistry(nil,
		SignIn{Writer: fixture.writer}.Descriptor(),
		RefreshSession{Writer: fixture.writer}.Descriptor(),
		CompleteSignIn{Writer: fixture.writer}.Descriptor(),
		EnrollTotp{Writer: fixture.writer}.Descriptor(),
		ConfirmTotp{Writer: fixture.writer}.Descriptor(),
		DisableTotp{Writer: fixture.writer}.Descriptor(),
		RedeemInvitation{Writer: fixture.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}

	anonymous := appshared.Anonymous("en", "UTC")

	// Enrol and confirm, signed in.
	out, err := registry.Invoke(t.Context(), EnrollTotpName, signedInActor(), usecase.Input{})
	if err != nil {
		t.Fatalf("enrolling through the registry: %v", err)
	}
	if out.String("secret") == "" || out.String("otpauth_uri") == "" {
		t.Fatalf("the single showing is missing from the output: %v", out)
	}
	material := fixture.writer.Encryptor.(*encryptorFake).sealedBy[string(mfaSecretPurpose(account))]
	if _, err := registry.Invoke(t.Context(), ConfirmTotpName, signedInActor(), usecase.Input{
		"code": domain.TotpCode([]byte(material), domain.TotpStep(now)),
	}); err != nil {
		t.Fatalf("confirming through the registry: %v", err)
	}

	// The two-step sign-in, both halves through the registry.
	out, err = registry.Invoke(t.Context(), SignInName, anonymous, usecase.Input{
		"email": "bert@example.org", "password": "correct horse battery",
	})
	if err != nil {
		t.Fatalf("signing in through the registry: %v", err)
	}
	if required, _ := out["mfa_required"].(bool); !required {
		t.Fatalf("no challenge in the output: %v", out)
	}
	later := now.Add(domain.TotpStepSeconds * time.Second)
	completer := fixture.writer
	completer.Clock = clock.Fixed(later)
	completed, err := usecase.HandlerFunc(CompleteSignIn{Writer: completer}.invoke).Invoke(
		t.Context(), anonymous, usecase.Input{
			"pending_token": out["pending_token"],
			"code":          domain.TotpCode([]byte(material), domain.TotpStep(later)),
		})
	if err != nil {
		t.Fatalf("completing through the handler: %v", err)
	}
	if completed.String("access_token") == "" || completed.String("refresh_token") == "" {
		t.Fatalf("the pair is missing from the output: %v", completed)
	}

	// Disable through the registry, with the fresh password.
	if _, err := registry.Invoke(t.Context(), DisableTotpName, signedInActor(), usecase.Input{
		"password": "correct horse battery",
	}); err != nil {
		t.Fatalf("disabling through the registry: %v", err)
	}
}
