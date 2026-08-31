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
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	sessionRowID = shared.ID("018f2a1b-0000-7000-8000-00000000aa01")
	refreshRowID = shared.ID("018f2a1b-0000-7000-8000-00000000aa02")
)

// idSequence answers distinct identifiers, because a sign-in mints a session and a token in one
// call and a fake answering one value would make the two rows indistinguishable.
type idSequence struct {
	queue []shared.ID
	index int
}

func (i *idSequence) NewID() shared.ID {
	if i.index >= len(i.queue) {
		return shared.ID("018f2a1b-0000-7000-8000-0000000000ff")
	}
	id := i.queue[i.index]
	i.index++
	return id
}

type sessionsStore struct {
	inserted []domain.Session
	sessions map[shared.ID]repository.SessionCredential
	listed   []domain.Session
	extended map[shared.ID]time.Time
	touched  []shared.ID
	revoked  []shared.ID
	// revokeChanged is what Revoke and RevokeAll report; a real repository reports false for a
	// row that is not the caller's or already stamped.
	revokeChanged bool
	revokedAll    int
}

func newSessionsStore() *sessionsStore {
	return &sessionsStore{
		sessions:      map[shared.ID]repository.SessionCredential{},
		extended:      map[shared.ID]time.Time{},
		revokeChanged: true,
	}
}

func (s *sessionsStore) Insert(_ context.Context, session domain.Session) error {
	s.inserted = append(s.inserted, session)
	return nil
}

func (s *sessionsStore) FindForAuth(_ context.Context, id shared.ID) (repository.SessionCredential, error) {
	credential, ok := s.sessions[id]
	if !ok {
		return repository.SessionCredential{}, shared.ErrNotFound.WithDetail("auth.session_not_found")
	}
	return credential, nil
}

func (s *sessionsStore) ForAccount(context.Context, shared.ID, time.Time) ([]domain.Session, error) {
	return s.listed, nil
}

func (s *sessionsStore) TouchLastSeen(_ context.Context, id shared.ID, _ time.Time) error {
	s.touched = append(s.touched, id)
	return nil
}

func (s *sessionsStore) Extend(_ context.Context, id shared.ID, expiresAt time.Time) error {
	s.extended[id] = expiresAt
	return nil
}

func (s *sessionsStore) Revoke(_ context.Context, id, _ shared.ID, _ time.Time) (bool, error) {
	if !s.revokeChanged {
		return false, nil
	}
	s.revoked = append(s.revoked, id)
	return true, nil
}

func (s *sessionsStore) RevokeAll(context.Context, shared.ID, time.Time) (int, error) {
	return s.revokedAll, nil
}

type refreshStore struct {
	inserted   []domain.RefreshToken
	presented  []domain.Token
	byToken    map[string]repository.RefreshCredential
	rotated    []shared.ID
	rotateFail bool
}

func newRefreshStore() *refreshStore {
	return &refreshStore{byToken: map[string]repository.RefreshCredential{}}
}

func (s *refreshStore) Insert(_ context.Context, token domain.RefreshToken, presented domain.Token) error {
	s.inserted = append(s.inserted, token)
	s.presented = append(s.presented, presented)
	return nil
}

func (s *refreshStore) FindByToken(_ context.Context, token domain.Token) (repository.RefreshCredential, error) {
	credential, ok := s.byToken[token.Secret()]
	if !ok {
		return repository.RefreshCredential{}, shared.ErrNotFound.WithDetail("auth.refresh_failed")
	}
	return credential, nil
}

func (s *refreshStore) Rotate(_ context.Context, id shared.ID, _ time.Time) (bool, error) {
	if s.rotateFail {
		return false, nil
	}
	s.rotated = append(s.rotated, id)
	return true, nil
}

type signInAccounts struct {
	byEmail map[string]repository.SignInAccount
}

func (s *signInAccounts) FindForSignIn(_ context.Context, email string) (repository.SignInAccount, error) {
	found, ok := s.byEmail[strings.ToLower(email)]
	if !ok {
		return repository.SignInAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return found, nil
}

func (s *signInAccounts) SetRedemptionToken(
	context.Context, shared.ID, domain.Token, time.Time, time.Time,
) (bool, error) {
	return true, nil
}

func (s *signInAccounts) FindByRedemptionToken(context.Context, domain.Token) (repository.RedemptionAccount, error) {
	return repository.RedemptionAccount{}, shared.ErrNotFound.WithDetail("auth.redemption_failed")
}

func (s *signInAccounts) Redeem(context.Context, shared.ID, string, time.Time) (bool, error) {
	return true, nil
}

type attemptsStore struct {
	standing map[string]repository.AuthAttempt
	cleared  []string
}

func newAttemptsStore() *attemptsStore {
	return &attemptsStore{standing: map[string]repository.AuthAttempt{}}
}

func (s *attemptsStore) Find(_ context.Context, subject string) (repository.AuthAttempt, error) {
	return s.standing[subject], nil
}

func (s *attemptsStore) Record(_ context.Context, subject string, attempt repository.AuthAttempt) error {
	s.standing[subject] = attempt
	return nil
}

func (s *attemptsStore) Clear(_ context.Context, subject string) error {
	s.cleared = append(s.cleared, subject)
	delete(s.standing, subject)
	return nil
}

type tenantDirectory struct{ single shared.ID }

func (d tenantDirectory) Resolve(_ context.Context, slug string) (shared.ID, error) {
	if slug == "" || slug == "acme" {
		return d.single, nil
	}
	return "", shared.ErrNotFound.WithDetail("auth.tenant_unresolved")
}

// passwordsFake verifies by comparison and counts the decoy work, which is the assertion T-02's
// constant shape needs: the decoy must burn on exactly the paths a real check would.
type passwordsFake struct {
	valid   string
	decoyed int
}

func (p *passwordsFake) Hash(password secret.Secret) (string, error) {
	return "hash:" + password.Reveal(), nil
}

func (p *passwordsFake) Verify(stored string, password secret.Secret) (bool, error) {
	return stored == "hash:"+password.Reveal() || (p.valid != "" && password.Reveal() == p.valid), nil
}

func (p *passwordsFake) VerifyDecoy(secret.Secret) { p.decoyed++ }

type signerFake struct{}

func (signerFake) Issue(claims cryptoport.SessionClaims) string {
	return "hbt_sat_" + claims.SessionID.String()
}

func (signerFake) Validate(string, time.Time) (cryptoport.SessionClaims, error) {
	return cryptoport.SessionClaims{}, shared.ErrUnauthenticated.WithDetail("access.token_malformed")
}

type signalsFake struct{ reasons []string }

func (s *signalsFake) AuthFailure(_ context.Context, reason string) {
	s.reasons = append(s.reasons, reason)
}

type sessionFixture struct {
	writer   SessionWriter
	sessions *sessionsStore
	refresh  *refreshStore
	attempts *attemptsStore
	accounts *signInAccounts
	signals  *signalsFake
	audit    *auditSink
	work     *unitOfWork
}

func newSessionFixture(at time.Time) *sessionFixture {
	f := &sessionFixture{
		sessions: newSessionsStore(),
		refresh:  newRefreshStore(),
		attempts: newAttemptsStore(),
		accounts: &signInAccounts{byEmail: map[string]repository.SignInAccount{}},
		signals:  &signalsFake{},
		audit:    &auditSink{},
		work:     &unitOfWork{},
	}
	f.writer = SessionWriter{
		Accounts: f.accounts, Sessions: f.sessions, Refresh: f.refresh,
		Attempts: f.attempts, Tenants: tenantDirectory{single: tenant},
		Passwords: &passwordsFake{}, Signer: signerFake{},
		Audit: f.audit, Signals: f.signals,
		UnitOfWork: f.work, Clock: clock.Fixed(at),
		IDs:     &idSequence{queue: []shared.ID{sessionRowID, refreshRowID}},
		Entropy: clock.FixedEntropy{},
	}
	return f
}

func (f *sessionFixture) withAccount(email, password string) {
	f.accounts.byEmail[email] = repository.SignInAccount{
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			Email: email, DisplayName: "Bert", Status: domain.AccountActive,
		},
		PasswordHash:   secret.New("hash:" + password),
		TenantLocale:   "en",
		TenantTimeZone: "UTC",
	}
}

func TestASignInOpensASessionAndAnswersThePair(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")

	pair, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "Bert@Example.ORG", Password: secret.New("correct horse battery"),
		UserAgent: "hubctl/1.0", RemoteAddr: "203.0.113.7:51234",
	})
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}

	if len(fixture.sessions.inserted) != 1 {
		t.Fatalf("%d sessions written, want one", len(fixture.sessions.inserted))
	}
	session := fixture.sessions.inserted[0]
	if session.IPClass != "203.0.113.0/24" || session.UserAgent != "hubctl/1.0" {
		t.Errorf("client hint %q %q, want the coarsened pair", session.UserAgent, session.IPClass)
	}
	if !session.ExpiresAt.Equal(now.Add(domain.RefreshTokenLifetime)) {
		t.Errorf("session horizon %v, want the refresh lifetime", session.ExpiresAt)
	}
	if len(fixture.refresh.inserted) != 1 {
		t.Fatalf("%d refresh tokens written, want one", len(fixture.refresh.inserted))
	}
	if fixture.refresh.inserted[0].SessionID != session.ID {
		t.Errorf("the refresh token does not hang off the session")
	}
	if pair.AccessToken.IsEmpty() || pair.RefreshToken.IsEmpty() {
		t.Error("a token of the pair is empty")
	}
	if !strings.HasPrefix(pair.RefreshToken.Reveal(), domain.RefreshTokenPrefix) {
		t.Errorf("refresh token %q lacks its prefix", pair.RefreshToken.Reveal())
	}
	if !pair.AccessExpiresAt.Equal(now.Add(domain.AccessTokenLifetime)) {
		t.Errorf("access expiry %v, want fifteen minutes", pair.AccessExpiresAt)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != SignedInAction {
		t.Errorf("audit %v, want the sign-in entry", fixture.audit.entries)
	}
	if len(fixture.attempts.cleared) == 0 {
		t.Error("a successful sign-in left the ledger standing")
	}
}

// T-02: the two refusals are one answer, byte for byte, and both cost the ledger a failure.
func TestWrongPasswordAndNoAccountAreOneAnswer(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	passwords := fixture.writer.Passwords.(*passwordsFake)

	_, wrongPassword := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("a wrong password!"),
		RemoteAddr: "203.0.113.7:1",
	})
	_, noAccount := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "nobody@example.org", Password: secret.New("a wrong password!"),
		RemoteAddr: "203.0.113.7:1",
	})

	if wrongPassword == nil || noAccount == nil {
		t.Fatal("a refusal is missing")
	}
	if wrongPassword.Error() != noAccount.Error() {
		t.Errorf("%q and %q differ - account existence is disclosed", wrongPassword, noAccount)
	}
	if passwords.decoyed != 1 {
		t.Errorf("the decoy burned %d times, want once (for the missing account)", passwords.decoyed)
	}
	if got := fixture.attempts.standing["account:bert@example.org"].Failures; got != 1 {
		t.Errorf("the account subject stands at %d, want 1", got)
	}
	if got := fixture.attempts.standing["ip:203.0.113.0/24"].Failures; got != 2 {
		t.Errorf("the network subject stands at %d, want 2", got)
	}
	if len(fixture.signals.reasons) != 2 || fixture.signals.reasons[0] != FailureWrongCredential {
		t.Errorf("signals %v, want two wrong_credential", fixture.signals.reasons)
	}
	if len(fixture.sessions.inserted) != 0 {
		t.Error("a refused sign-in opened a session")
	}
}

// The lockout engages by the clock: past the free attempts the subject waits, and after the
// window it may try again (T-02's progressive delay, proved with a clock).
func TestTheLockoutEngagesAndReleasesByTheClock(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")
	handler := SignIn{Writer: fixture.writer}

	for range 4 {
		_, err := handler.Execute(t.Context(), SignInCommand{
			Email: "bert@example.org", Password: secret.New("a wrong password!"),
		})
		if !errors.Is(err, shared.ErrUnauthenticated) {
			t.Fatalf("a wrong password answered %v", err)
		}
	}

	// Four failures put one second on the clock. The right password is refused inside the window -
	// and the refusal names the wait, not the credential.
	_, locked := handler.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	})
	if !errors.Is(locked, shared.ErrRateLimited) {
		t.Fatalf("inside the window the sign-in answered %v, want the lock", locked)
	}
	if !strings.Contains(locked.Error(), "auth.sign_in_locked") {
		t.Errorf("the lock answered %q", locked)
	}

	// The same call a second later succeeds: the delay is a delay, not a ban.
	later := newSessionFixture(now.Add(2 * time.Second))
	later.withAccount("bert@example.org", "correct horse battery")
	later.attempts.standing = fixture.attempts.standing
	if _, err := (SignIn{Writer: later.writer}).Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
	}); err != nil {
		t.Fatalf("after the window the sign-in answered %v", err)
	}
}

func TestATenantHeaderMayConfirmButNeverOverrule(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.withAccount("bert@example.org", "correct horse battery")

	if _, err := (SignIn{Writer: fixture.writer}).Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
		TenantHeader: tenant.String(),
	}); err != nil {
		t.Fatalf("a confirming header refused: %v", err)
	}

	_, err := SignIn{Writer: fixture.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New("correct horse battery"),
		TenantHeader: string(account),
	})
	if !errors.Is(err, shared.ErrForbidden) || !strings.Contains(err.Error(), "access.tenant_mismatch") {
		t.Fatalf("a contradicting header answered %v, want tenant_mismatch", err)
	}
}

func refreshCredential(at time.Time) (repository.RefreshCredential, domain.Token) {
	presented, _ := domain.NewRefreshToken(tenant, []byte(strings.Repeat("a", domain.TokenSecretBytes)))
	return repository.RefreshCredential{
		Token: domain.RefreshToken{
			ID: refreshRowID, TenantID: tenant, SessionID: sessionRowID,
			CreatedAt: at.Add(-time.Hour), ExpiresAt: at.Add(29 * 24 * time.Hour),
		},
		Session: domain.Session{
			ID: sessionRowID, TenantID: tenant, AccountID: account,
			CreatedAt: at.Add(-time.Hour), ExpiresAt: at.Add(29 * 24 * time.Hour),
		},
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			DisplayName: "Bert", Status: domain.AccountActive,
		},
	}, presented
}

func TestARefreshRotatesAndSlidesTheHorizon(t *testing.T) {
	fixture := newSessionFixture(now)
	credential, presented := refreshCredential(now)
	fixture.refresh.byToken[presented.Secret()] = credential

	pair, err := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: secret.New(presented.Secret()),
	})
	if err != nil {
		t.Fatalf("refreshing: %v", err)
	}

	if len(fixture.refresh.rotated) != 1 || fixture.refresh.rotated[0] != credential.Token.ID {
		t.Errorf("rotated %v, want the presented token retired", fixture.refresh.rotated)
	}
	if len(fixture.refresh.inserted) != 1 {
		t.Fatalf("%d new tokens, want one", len(fixture.refresh.inserted))
	}
	if pair.RefreshToken.Reveal() == presented.Secret() {
		t.Error("the answer is the token that was presented - nothing rotated")
	}
	if horizon, ok := fixture.sessions.extended[sessionRowID]; !ok ||
		!horizon.Equal(now.Add(domain.RefreshTokenLifetime)) {
		t.Errorf("session horizon %v, want it slid to the new token's", horizon)
	}
}

// T-01's core: a rotated token presented again kills the family, raises the reuse reason, and
// answers exactly what an unknown token answers.
func TestReplayAfterRotationKillsTheFamily(t *testing.T) {
	fixture := newSessionFixture(now)
	credential, presented := refreshCredential(now)
	credential.Token.RotatedAt = now.Add(-time.Minute)
	fixture.refresh.byToken[presented.Secret()] = credential

	_, reused := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: secret.New(presented.Secret()),
	})
	unknown, _ := domain.NewRefreshToken(tenant, []byte(strings.Repeat("b", domain.TokenSecretBytes)))
	_, notFound := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: secret.New(unknown.Secret()),
	})

	if reused == nil || notFound == nil {
		t.Fatal("a refusal is missing")
	}
	if reused.Error() != notFound.Error() {
		t.Errorf("%q and %q differ - reuse is disclosed to the prober", reused, notFound)
	}
	if len(fixture.sessions.revoked) != 1 || fixture.sessions.revoked[0] != sessionRowID {
		t.Errorf("revoked %v, want the whole session", fixture.sessions.revoked)
	}
	found := false
	for _, reason := range fixture.signals.reasons {
		if reason == FailureRefreshReused {
			found = true
		}
	}
	if !found {
		t.Errorf("signals %v, want %s - A-15 has nothing to watch otherwise",
			fixture.signals.reasons, FailureRefreshReused)
	}
	warned := false
	for _, entry := range fixture.audit.entries {
		if entry.Action == RefreshReuseAction && entry.Severity == audit.SeverityWarning {
			warned = true
		}
	}
	if !warned {
		t.Errorf("audit %v, want the reuse warning", fixture.audit.entries)
	}
}

// The clock proves the refusals: an expired token, an expired session, a revoked session.
func TestARefreshRefusesWhatTheClockOrARevocationEnded(t *testing.T) {
	cases := map[string]func(*repository.RefreshCredential){
		"an expired token":   func(c *repository.RefreshCredential) { c.Token.ExpiresAt = now.Add(-time.Minute) },
		"an expired session": func(c *repository.RefreshCredential) { c.Session.ExpiresAt = now.Add(-time.Minute) },
		"a revoked session":  func(c *repository.RefreshCredential) { c.Session.RevokedAt = now.Add(-time.Minute) },
	}
	for name, wound := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newSessionFixture(now)
			credential, presented := refreshCredential(now)
			wound(&credential)
			fixture.refresh.byToken[presented.Secret()] = credential

			_, err := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
				RefreshToken: secret.New(presented.Secret()),
			})
			if !errors.Is(err, shared.ErrUnauthenticated) {
				t.Fatalf("answered %v, want a refusal", err)
			}
			if len(fixture.refresh.inserted) != 0 {
				t.Error("a refused exchange minted a token")
			}
		})
	}
}

// A concurrent exchange of one token is two holders as far as anybody can tell, and gets the
// reuse answer.
func TestAConcurrentRotationCountsAsReuse(t *testing.T) {
	fixture := newSessionFixture(now)
	credential, presented := refreshCredential(now)
	fixture.refresh.byToken[presented.Secret()] = credential
	fixture.refresh.rotateFail = true

	_, err := RefreshSession{Writer: fixture.writer}.Execute(t.Context(), RefreshSessionCommand{
		RefreshToken: secret.New(presented.Secret()),
	})
	if err == nil {
		t.Fatal("a lost race minted a pair")
	}
	if len(fixture.sessions.revoked) != 1 {
		t.Errorf("revoked %v, want the session", fixture.sessions.revoked)
	}
}

// validatingSigner answers fixed claims for one token, standing in for the real signature check.
type validatingSigner struct {
	token  string
	claims cryptoport.SessionClaims
}

func (s validatingSigner) Issue(cryptoport.SessionClaims) string { return s.token }

func (s validatingSigner) Validate(presented string, now time.Time) (cryptoport.SessionClaims, error) {
	if presented != s.token {
		return cryptoport.SessionClaims{}, shared.ErrUnauthenticated.WithDetail("access.token_malformed")
	}
	if !now.Before(s.claims.ExpiresAt) {
		return cryptoport.SessionClaims{}, shared.ErrUnauthenticated.WithDetail("access.token_expired")
	}
	return s.claims, nil
}

func sessionAuthenticator(sessions *sessionsStore, signer validatingSigner, at time.Time) AuthenticateToken {
	return AuthenticateToken{
		Tokens:        &tokens{},
		UnitOfWork:    &unitOfWork{},
		Clock:         clock.Fixed(at),
		Sessions:      sessions,
		Signer:        signer,
		SessionScopes: []string{"items:read", "items:write"},
	}
}

// The session path of authentication: a valid token becomes an actor whose credential is the
// session, carrying every declared scope.
func TestASessionTokenAuthenticates(t *testing.T) {
	sessions := newSessionsStore()
	credential, _ := refreshCredential(now)
	sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: credential.Session, Account: credential.Account,
		TenantLocale: "en", TenantTimeZone: "UTC",
	}
	signer := validatingSigner{token: "hbt_sat_x", claims: cryptoport.SessionClaims{
		TenantID: tenant, SessionID: sessionRowID, AccountID: account,
		ExpiresAt: now.Add(10 * time.Minute),
	}}

	actor, err := sessionAuthenticator(sessions, signer, now).Execute(t.Context(),
		AuthenticateTokenCommand{Credential: "hbt_sat_x"})
	if err != nil {
		t.Fatalf("authenticating: %v", err)
	}
	if actor.TokenID != sessionRowID {
		t.Errorf("TokenID %v, want the session - the listing marks the current row by it", actor.TokenID)
	}
	if actor.AccountID != account || actor.TenantID != tenant {
		t.Errorf("actor %+v, want the session's account and tenant", actor)
	}
	if len(actor.Scopes) == 0 {
		t.Error("a session actor carries no scopes")
	}
}

// Revoking a session refuses its access token on the very next request - the acceptance's
// "immediately", proved against the row rather than the token's own fifteen minutes.
func TestARevokedSessionRefusesItsPairImmediately(t *testing.T) {
	sessions := newSessionsStore()
	credential, _ := refreshCredential(now)
	credential.Session.RevokedAt = now.Add(-time.Second)
	sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: credential.Session, Account: credential.Account,
	}
	signer := validatingSigner{token: "hbt_sat_x", claims: cryptoport.SessionClaims{
		TenantID: tenant, SessionID: sessionRowID, AccountID: account,
		ExpiresAt: now.Add(10 * time.Minute),
	}}

	_, err := sessionAuthenticator(sessions, signer, now).Execute(t.Context(),
		AuthenticateTokenCommand{Credential: "hbt_sat_x"})
	if err == nil || !strings.Contains(err.Error(), "auth.session_revoked") {
		t.Fatalf("a revoked session's token answered %v", err)
	}
}

func sessionActor(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: shared.ActorUser, TenantID: tenant, AccountID: account,
		AccountName: "Bert", TokenID: sessionRowID, Scopes: scopes,
	}
}

func TestListSessionsMarksTheCurrentOne(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.sessions.listed = []domain.Session{
		{ID: sessionRowID, AccountID: account, CreatedAt: now},
		{ID: refreshRowID, AccountID: account, CreatedAt: now.Add(-time.Hour)},
	}

	sessions, currentID, err := ListSessions{Writer: fixture.writer}.
		Execute(t.Context(), sessionActor(accountsRead))
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("%d sessions, want two", len(sessions))
	}
	if currentID != sessionRowID {
		t.Errorf("current %v, want the actor's own session", currentID)
	}

	out := sessionOutput(sessions[0], currentID)
	if current, _ := out["current"].(bool); !current {
		t.Error("the answering session is not marked current")
	}
	out = sessionOutput(sessions[1], currentID)
	if current, _ := out["current"].(bool); current {
		t.Error("another session is marked current")
	}
}

func TestRevokeSessionIsIdempotentForOnesOwnAndNotFoundForOthers(t *testing.T) {
	fixture := newSessionFixture(now)
	handler := RevokeSession{Writer: fixture.writer}

	// The live one revokes and is audited.
	credential, _ := refreshCredential(now)
	fixture.sessions.sessions[sessionRowID] = repository.SessionCredential{
		Session: credential.Session, Account: credential.Account,
	}
	if err := handler.Execute(t.Context(), sessionActor(accountRead), sessionRowID); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != SessionRevokedAction {
		t.Errorf("audit %v, want the revocation entry", fixture.audit.entries)
	}

	// Already over: idempotent success, and no second entry.
	fixture.sessions.revokeChanged = false
	if err := handler.Execute(t.Context(), sessionActor(accountRead), sessionRowID); err != nil {
		t.Fatalf("revoking twice answered %v, want success", err)
	}
	if len(fixture.audit.entries) != 1 {
		t.Error("a repeat wrote a second entry")
	}

	// Somebody else's, or unknown: one indistinguishable not-found.
	other := newSessionFixture(now)
	other.sessions.revokeChanged = false
	err := RevokeSession{Writer: other.writer}.Execute(t.Context(), sessionActor(accountRead), refreshRowID)
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("somebody else's session answered %v, want not found", err)
	}
}

func TestRevokeAllSessionsAuditsTheCount(t *testing.T) {
	fixture := newSessionFixture(now)
	fixture.sessions.revokedAll = 3

	if err := (RevokeAllSessions{Writer: fixture.writer}).
		Execute(t.Context(), sessionActor(accountRead)); err != nil {
		t.Fatalf("revoking all: %v", err)
	}
	if len(fixture.audit.entries) != 1 || fixture.audit.entries[0].Action != SessionsRevokedAction {
		t.Fatalf("audit %v, want one entry", fixture.audit.entries)
	}

	// Nothing live, nothing written: an entry saying something ended would be a false one.
	quiet := newSessionFixture(now)
	if err := (RevokeAllSessions{Writer: quiet.writer}).
		Execute(t.Context(), sessionActor(accountRead)); err != nil {
		t.Fatalf("an empty revocation answered %v", err)
	}
	if len(quiet.audit.entries) != 0 {
		t.Error("an empty revocation wrote an entry")
	}
}
