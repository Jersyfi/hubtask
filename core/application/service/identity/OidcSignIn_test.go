// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	provider "github.com/Jersyfi/hubtask/core/port/identityprovider"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// flowStore keeps the minutes between leaving and coming back.
type flowStore struct {
	byState map[string]domain.OidcFlow
	spent   map[string]bool
}

func newFlows() *flowStore {
	return &flowStore{byState: map[string]domain.OidcFlow{}, spent: map[string]bool{}}
}

func (s *flowStore) Insert(_ context.Context, flow domain.OidcFlow, presented domain.Token) error {
	s.byState[presented.Secret()] = flow
	return nil
}

func (s *flowStore) Consume(
	_ context.Context, presented domain.Token, _ time.Time,
) (domain.OidcFlow, bool, error) {
	flow, found := s.byState[presented.Secret()]
	if !found || s.spent[presented.Secret()] {
		return domain.OidcFlow{}, false, nil
	}
	s.spent[presented.Secret()] = true
	return flow, true, nil
}

// externalStore is the `account.external_subject` seam.
type externalStore struct {
	bySubject map[string]domain.Account
	links     []string
	refuse    bool
	// accounts is where a linked subject's account is read back from, so that the second
	// arrival finds the person rather than a stub the double invented.
	accounts *accountStore
}

func newExternal(accounts *accountStore) *externalStore {
	return &externalStore{bySubject: map[string]domain.Account{}, accounts: accounts}
}

func (s *externalStore) FindBySubject(_ context.Context, subject string) (domain.Account, error) {
	account, found := s.bySubject[subject]
	if !found {
		return domain.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

func (s *externalStore) LinkSubject(
	_ context.Context, accountID shared.ID, subject string, _ time.Time,
) (bool, error) {
	if s.refuse {
		return false, nil
	}
	s.links = append(s.links, subject)
	account, found := s.accounts.byID[accountID]
	if !found {
		account = domain.Account{ID: accountID, TenantID: tenant, Status: domain.AccountActive}
	}
	s.bySubject[subject] = account
	return true, nil
}

// arriving is the identity the library would have verified.
type arrivingDouble struct {
	identity provider.Identity
	err      error
	url      string
}

func (a *arrivingDouble) Check(context.Context, string) error { return nil }

func (a *arrivingDouble) AuthorizationURL(
	context.Context, provider.Config, provider.Authorization,
) (string, error) {
	if a.url == "" {
		return "https://login.example.org/authorize?state=x", nil
	}
	return a.url, nil
}

func (a *arrivingDouble) Exchange(
	context.Context, provider.Config, provider.Exchange,
) (provider.Identity, error) {
	return a.identity, a.err
}

// countingEntropy answers different bytes every time, which the fixed double does not. Two
// sign-ins in one test are two flows, and a double that minted one state twice would make the
// second look like a replay of the first.
type countingEntropy struct{ drawn byte }

func (e *countingEntropy) Bytes(n int) ([]byte, error) {
	e.drawn++
	material := make([]byte, n)
	for i := range material {
		material[i] = e.drawn + byte(i)
	}
	return material, nil
}

type oidcFixture struct {
	writer   OidcWriter
	session  *sessionFixture
	store    *providerStore
	flows    *flowStore
	external *externalStore
	accounts *accountStore
	relying  *arrivingDouble
}

func newOidcFixture(t *testing.T, at time.Time, existing ...domain.Account) *oidcFixture {
	t.Helper()
	session := mfaFixture(at)
	accounts := newAccounts(existing...)
	f := &oidcFixture{
		session: session, store: &providerStore{}, flows: newFlows(),
		external: newExternal(accounts), accounts: accounts,
		relying: &arrivingDouble{identity: provider.Identity{
			Subject: "provider-subject-1", Email: "ada@example.org",
			EmailVerified: true, DisplayName: "Ada",
		}},
	}
	session.writer.Entropy = &countingEntropy{}
	f.writer = OidcWriter{
		Session: session.writer, Providers: f.store, Flows: f.flows,
		External: f.external, Accounts: f.accounts, Relying: f.relying,
		RedirectURL: "https://hubtask.example/auth/callback",
	}

	// A configured, enabled provider that links inside example.org.
	configured, err := domain.NewIdentityProvider(domain.NewIdentityProviderInput{
		TenantID: tenant, Issuer: "https://login.example.org", ClientID: "hubtask",
		AllowedEmailDomains: []string{"example.org"}, Enabled: true, Now: at,
	})
	if err != nil {
		t.Fatalf("configuring: %v", err)
	}
	// Sealed through the fixture's own encryptor, so that opening it in the flow is the real
	// round trip rather than a value the double would refuse as inauthentic.
	sealed, err := session.writer.Encryptor.Seal(
		t.Context(), secret.New("s3cr3t"), clientSecretPurpose(tenant))
	if err != nil {
		t.Fatalf("sealing: %v", err)
	}
	if _, err := f.store.Upsert(t.Context(), configured, sealed, at); err != nil {
		t.Fatalf("storing the configuration: %v", err)
	}
	return f
}

// start runs the first half and answers the state it minted.
func start(t *testing.T, f *oidcFixture) secret.Secret {
	t.Helper()
	authorization, err := StartOidcSignIn{Writer: f.writer}.
		Execute(t.Context(), StartOidcSignInCommand{})
	if err != nil {
		t.Fatalf("starting: %v", err)
	}
	return authorization.State
}

// The acceptance criterion, in one test: a round trip ends in a session, and the account it
// belongs to was made on the way because nobody here had heard of that subject before.
func TestAFirstArrivalIsProvisionedAndEndsInASession(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)

	state := start(t, f)
	pair, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: state, UserAgent: "Firefox", RemoteAddr: "203.0.113.7",
	})
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if pair.AccessToken.IsEmpty() || pair.RefreshToken.IsEmpty() {
		t.Error("the pair is not a pair")
	}
	if pair.Session.ID.IsZero() {
		t.Error("no session was opened")
	}
	if len(f.external.links) != 1 || f.external.links[0] != "provider-subject-1" {
		t.Errorf("the subject was linked as %v", f.external.links)
	}

	provisioned := f.accounts.byEmail["ada@example.org"]
	if provisioned.Status != domain.AccountActive {
		t.Errorf("the provisioned account is %q, want ACTIVE", provisioned.Status)
	}
	actions := auditActions(f.session.audit.entries)
	if !containsAction(actions, OidcProvisionedAction) {
		t.Errorf("the trail holds %v, want a provisioning entry", actions)
	}
}

// The second arrival finds the subject and makes nothing.
func TestASecondArrivalFindsTheSameAccount(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)

	first, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "one", State: start(t, f),
	})
	if err != nil {
		t.Fatalf("first arrival: %v", err)
	}
	before := len(f.accounts.byID)

	second, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "two", State: start(t, f),
	})
	if err != nil {
		t.Fatalf("second arrival: %v", err)
	}
	if len(f.accounts.byID) != before {
		t.Errorf("the second arrival made an account: %d, want %d", len(f.accounts.byID), before)
	}
	if first.Session.AccountID != second.Session.AccountID {
		t.Error("the same subject signed in as two different people")
	}
}

// Linking hands somebody an account that already exists, so it happens only on a verified
// address inside the configured domains - and it is recorded, because it is the event a review
// looks for.
func TestAVerifiedAddressInsideTheDomainsLinksAndIsRecorded(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	existing := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountActive,
	}
	f := newOidcFixture(t, at, existing)

	pair, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "the-code", State: start(t, f),
	})
	if err != nil {
		t.Fatalf("completing: %v", err)
	}
	if pair.Session.AccountID != existing.ID {
		t.Errorf("the session belongs to %s, want the existing account", pair.Session.AccountID)
	}
	if !containsAction(auditActions(f.session.audit.entries), OidcLinkedAction) {
		t.Error("taking over an existing account was not recorded")
	}
}

// The two ways linking must not happen: an address the provider did not verify, and one outside
// the configured domains. Both provision instead - a new account is the safe answer.
func TestLinkingNeedsAVerifiedAddressInsideTheDomains(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	existing := domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "ada@example.org", DisplayName: "Ada",
		Status: domain.AccountActive,
	}

	cases := []struct {
		name     string
		identity provider.Identity
	}{
		{name: "unverified", identity: provider.Identity{
			Subject: "s-2", Email: "ada@example.org", EmailVerified: false, DisplayName: "Ada"}},
		{name: "outside the domains", identity: provider.Identity{
			Subject: "s-3", Email: "ada@elsewhere.org", EmailVerified: true, DisplayName: "Ada"}},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newOidcFixture(t, at, existing)
			f.relying.identity = c.identity

			pair, err := CompleteOidcSignIn{Writer: f.writer}.
				Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: start(t, f)})
			if err != nil {
				t.Fatalf("completing: %v", err)
			}
			if pair.Session.AccountID == existing.ID {
				t.Error("an unverified or out-of-domain address took over an existing account")
			}
			if containsAction(auditActions(f.session.audit.entries), OidcLinkedAction) {
				t.Error("a link was recorded where none should have happened")
			}
		})
	}
}

// A state is one round trip. Presented twice, the second is refused - and refused the way an
// unknown one is, because which it was is not for the presenter to learn.
func TestAStateIsSpentOnceAndTheReplayIsIndistinguishable(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	state := start(t, f)

	if _, err := (CompleteOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: state}); err != nil {
		t.Fatalf("the first use: %v", err)
	}

	_, replay := CompleteOidcSignIn{Writer: f.writer}.
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: state})
	unknown := func() error {
		token, _ := domain.NewOidcFlowState(tenant, make([]byte, domain.TokenSecretBytes))
		_, err := CompleteOidcSignIn{Writer: f.writer}.
			Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: secret.New(token.Secret())})
		return err
	}()

	if replay == nil {
		t.Fatal("a spent state was accepted a second time")
	}
	if shared.AsError(replay).Code != shared.AsError(unknown).Code {
		t.Errorf("a replay answers %q and an unknown state %q - they must be the same refusal",
			shared.AsError(replay).Code, shared.AsError(unknown).Code)
	}
}

// The acceptance criterion says it in as many words: a suspended or disabled account is refused at the token
// exchange, not only at its first sign-in.
func TestADisabledAccountIsRefusedAtTheExchange(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	f := newOidcFixture(t, at)
	f.external.bySubject["provider-subject-1"] = domain.Account{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e2"), TenantID: tenant,
		Kind: domain.AccountUser, Email: "bert@example.org", DisplayName: "Bert",
		Status: domain.AccountDisabled,
	}

	if _, err := (CompleteOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: start(t, f)}); err == nil {
		t.Fatal("a disabled account signed in through the provider")
	}
}

// A workspace with no provider, and one that switched it off, both refuse - and neither leaves a
// flow behind for somebody to present later.
func TestNoProviderAndASwitchedOffOneBothRefuse(t *testing.T) {
	at := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)

	none := newOidcFixture(t, at)
	none.store.configured = nil
	if _, err := (StartOidcSignIn{Writer: none.writer}).
		Execute(t.Context(), StartOidcSignInCommand{}); err == nil {
		t.Error("a workspace with no provider started a sign-in")
	}

	off := newOidcFixture(t, at)
	off.store.configured.Enabled = false
	if _, err := (StartOidcSignIn{Writer: off.writer}).
		Execute(t.Context(), StartOidcSignInCommand{}); err == nil {
		t.Error("a switched-off provider started a sign-in")
	}
	if len(off.flows.byState) != 0 {
		t.Error("a refused start left a flow behind")
	}
}

// A provider that cannot be reached leaves no flow row: the person is told it is the provider,
// and nothing is written for a sign-in that never began.
func TestAnUnreachableProviderLeavesNoFlow(t *testing.T) {
	f := newOidcFixture(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	f.relying.url = ""
	f.writer.Relying = &refusingRelying{}

	if _, err := (StartOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), StartOidcSignInCommand{}); err == nil {
		t.Fatal("an unreachable provider started a sign-in")
	}
	if len(f.flows.byState) != 0 {
		t.Errorf("a refused start wrote %d flows", len(f.flows.byState))
	}
}

type refusingRelying struct{}

func (refusingRelying) Check(context.Context, string) error { return nil }

func (refusingRelying) AuthorizationURL(
	context.Context, provider.Config, provider.Authorization,
) (string, error) {
	return "", shared.ErrUnavailable.WithDetail("auth.provider_unreachable")
}

func (refusingRelying) Exchange(
	context.Context, provider.Config, provider.Exchange,
) (provider.Identity, error) {
	return provider.Identity{}, shared.ErrUnavailable.WithDetail("auth.provider_unreachable")
}

// A token the adapter refused is a refusal here too, and the flow is spent all the same: the
// state was presented, so it is over whatever the provider said afterwards.
func TestATokenTheAdapterRefusedEndsTheFlow(t *testing.T) {
	f := newOidcFixture(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))
	f.relying.err = shared.ErrValidation.WithDetail("auth.identity_token_invalid")
	state := start(t, f)

	if _, err := (CompleteOidcSignIn{Writer: f.writer}).
		Execute(t.Context(), CompleteOidcSignInCommand{Code: "x", State: state}); err == nil {
		t.Fatal("an unverifiable token opened a session")
	}
	if !f.flows.spent[state.Reveal()] {
		t.Error("the state survived a failed exchange")
	}
}

// The header may confirm the state's workspace and never overrule it (decision 3).
func TestTheTenantHeaderMayNotOverruleTheState(t *testing.T) {
	f := newOidcFixture(t, time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC))

	_, err := CompleteOidcSignIn{Writer: f.writer}.Execute(t.Context(), CompleteOidcSignInCommand{
		Code: "x", State: start(t, f),
		TenantHeader: "01936f2a-7c1e-7000-8000-0000000000ff",
	})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a contradicting header answered %v, want a refusal", err)
	}
}

func auditActions(entries []audit.Entry) []audit.Action {
	actions := make([]audit.Action, 0, len(entries))
	for _, entry := range entries {
		actions = append(actions, entry.Action)
	}
	return actions
}

func containsAction(actions []audit.Action, wanted audit.Action) bool {
	for _, action := range actions {
		if action == wanted {
			return true
		}
	}
	return false
}
