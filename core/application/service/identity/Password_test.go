// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// passwordAccounts is the account surface of a password's life, in memory.
type passwordAccounts struct {
	rows    map[shared.ID]repository.PasswordAccount
	byEmail map[string]repository.PasswordAccount
	writes  []passwordWrite
	missing bool
}

type passwordWrite struct {
	accountID shared.ID
	hash      string
	at        time.Time
}

func (s *passwordAccounts) FindForPassword(
	_ context.Context, accountID shared.ID,
) (repository.PasswordAccount, error) {
	row, held := s.rows[accountID]
	if !held {
		return repository.PasswordAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return row, nil
}

func (s *passwordAccounts) FindByEmailForPassword(
	_ context.Context, email string,
) (repository.PasswordAccount, error) {
	row, held := s.byEmail[email]
	if !held {
		return repository.PasswordAccount{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return row, nil
}

func (s *passwordAccounts) SetPassword(
	_ context.Context, accountID shared.ID, hash string, at time.Time,
) (bool, error) {
	s.writes = append(s.writes, passwordWrite{accountID: accountID, hash: hash, at: at})
	return !s.missing, nil
}

// passwordHistories is the last few hashes, newest first.
type passwordHistories struct {
	hashes   []secret.Secret
	appended []string
	trimmed  []int
}

func (s *passwordHistories) Recent(
	_ context.Context, _ shared.ID, count int,
) ([]secret.Secret, error) {
	if count >= len(s.hashes) {
		return s.hashes, nil
	}
	return s.hashes[:count], nil
}

func (s *passwordHistories) Append(
	_ context.Context, _, _ shared.ID, hash string, _ time.Time,
) error {
	s.appended = append(s.appended, hash)
	return nil
}

func (s *passwordHistories) Trim(_ context.Context, _ shared.ID, keep int) error {
	s.trimmed = append(s.trimmed, keep)
	return nil
}

// stepUpFake is the proof, granted or withheld.
type stepUpFake struct {
	satisfied bool
	presented []string
}

func (s *stepUpFake) Available() bool { return true }

func (s *stepUpFake) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	s.presented = append(s.presented, token)
	return s.satisfied, nil
}

func (s *stepUpFake) Methods(context.Context, shared.ID, shared.ID) ([]stepupport.Method, error) {
	return []stepupport.Method{stepupport.MethodPassword}, nil
}

type passwordFixture struct {
	writer    PasswordWriter
	accounts  *passwordAccounts
	histories *passwordHistories
	sessions  *sessionsStore
	stepUp    *stepUpFake
	workspace *workspaceStore
	instance  *instanceSettings
	audit     *auditSink
	// providers is the workspace's ways in besides the password, which the fallback reads (ADR-0076
	// §4). Empty, so a test that switches the password off switches on a provider as well, or it is
	// testing a workspace with no way in - which signs in by password (E2, #1138).
	providers *providerStore
}

const heldPassword = "the one in force"

func newPasswordFixture(at time.Time) *passwordFixture {
	held := repository.PasswordAccount{
		Account: domain.Account{
			ID: account, TenantID: tenant, Kind: domain.AccountUser,
			Email: "bert@example.org", DisplayName: "Bert", Status: domain.AccountActive,
		},
		PasswordHash:  secret.New("hash:" + heldPassword),
		PasswordSetAt: at.Add(-30 * 24 * time.Hour),
	}
	f := &passwordFixture{
		accounts: &passwordAccounts{
			rows:    map[shared.ID]repository.PasswordAccount{account: held},
			byEmail: map[string]repository.PasswordAccount{"bert@example.org": held},
		},
		histories: &passwordHistories{},
		sessions:  newSessionsStore(),
		stepUp:    &stepUpFake{satisfied: true},
		workspace: &workspaceStore{row: domain.Workspace{
			Tenant: domain.Tenant{
				ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
			},
		}},
		instance: &instanceSettings{level: repository.InstanceLevel{
			Policy: domain.PolicyLayer{Locks: map[domain.PolicySwitch]bool{}},
			Legal:  domain.LegalLayer{Locks: map[domain.LegalLink]bool{}},
		}},
		audit:     &auditSink{},
		providers: newProviderStore(tenant),
	}
	f.writer = PasswordWriter{
		Session: SessionWriter{
			Passwords: &passwordsFake{}, Sessions: f.sessions, Audit: f.audit,
			Accounts: &signInAccounts{
				byEmail:    map[string]repository.SignInAccount{},
				redemption: map[string]repository.RedemptionAccount{},
			},
		},
		Resolver: SignInPolicyResolver{
			Workspaces: f.workspace, Instance: f.instance, UnitOfWork: &unitOfWork{},
		},
		Accounts: f.accounts, Histories: f.histories,
		StepUp:     f.stepUp,
		Text:       text.Composing{},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at),
		IDs: &idSequence{queue: []shared.ID{historyRowID}},
		WaysIn: WaysIn{
			Providers: f.providers, Workspaces: f.workspace, UnitOfWork: &unitOfWork{},
			Clock: clock.Fixed(at),
		},
	}
	return f
}

const (
	historyRowID   = shared.ID("018f2a1b-0000-7000-8000-0000000000h1")
	otherSessionID = shared.ID("018f2a1b-0000-7000-8000-00000000aa09")
)

func signedIn() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: account, TokenID: sessionRowID,
	}
}

// The step-up is the proof of the old password. Without it the bearer alone is a stolen tab.
func TestAChangeWithoutAStepUpIsRefused(t *testing.T) {
	fixture := newPasswordFixture(now)

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns")})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a change without a proof answered %v", err)
	}
	if len(fixture.accounts.writes) != 0 {
		t.Error("the password was written anyway")
	}
}

// A proof the verifier refuses is no proof.
func TestAChangeWithARefusedStepUpIsRefused(t *testing.T) {
	fixture := newPasswordFixture(now)
	fixture.stepUp.satisfied = false

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a refused proof answered %v", err)
	}
}

// The change lands, the previous hash goes into the history, and every other session ends while
// this one does not.
func TestAChangeEndsTheOtherSessionsAndKeepsThisOne(t *testing.T) {
	fixture := newPasswordFixture(now)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{HistoryCount: intOf(3)}
	fixture.sessions.listed = []domain.Session{
		{ID: sessionRowID, TenantID: tenant, AccountID: account},
		{ID: otherSessionID, TenantID: tenant, AccountID: account},
	}

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"})
	if err != nil {
		t.Fatalf("the change was refused: %v", err)
	}

	if len(fixture.accounts.writes) != 1 {
		t.Fatalf("%d writes, want one", len(fixture.accounts.writes))
	}
	written := fixture.accounts.writes[0]
	if written.hash != "hash:seven blue lanterns" {
		t.Errorf("the hash written was %q", written.hash)
	}
	if !written.at.Equal(now) {
		t.Errorf("the moment written was %v, want %v", written.at, now)
	}
	if len(fixture.histories.appended) != 1 || fixture.histories.appended[0] != "hash:"+heldPassword {
		t.Errorf("the history holds %v, want the password that just stopped being current",
			fixture.histories.appended)
	}
	if len(fixture.histories.trimmed) != 1 || fixture.histories.trimmed[0] != 3 {
		t.Errorf("the history was trimmed to %v, want the policy's three", fixture.histories.trimmed)
	}
	// Every other session ends in one statement, and the one the caller holds is the one spared.
	if len(fixture.sessions.keptByOthers) != 1 || fixture.sessions.keptByOthers[0] != sessionRowID {
		t.Errorf("the others ended sparing %v, want the caller's session", fixture.sessions.keptByOthers)
	}
	if len(fixture.audit.entries) != 1 ||
		fixture.audit.entries[0].Action != PasswordChangedAction {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

// The trail says a password changed and never anything about its shape.
func TestTheTrailSaysNothingAboutThePassword(t *testing.T) {
	fixture := newPasswordFixture(now)

	if err := (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{
			Password: secret.New("a memorable phrase"), StepUpToken: "hbt_stp_x",
		}); err != nil {
		t.Fatalf("the change was refused: %v", err)
	}

	changes := fixture.audit.entries[0].Changes
	if len(changes) != 1 {
		t.Fatalf("the trail records %d fields, want only the moment", len(changes))
	}
	recorded, held := changes["password_set_at"]
	if !held {
		t.Fatalf("the trail records %v, want the moment", changes)
	}
	if rendered := fmt.Sprintf("%v", recorded); strings.Contains(rendered, "memorable") {
		t.Fatalf("the trail carries the password: %v", recorded)
	}
}

// The one in force is its own rule: "you already have this one" is a different sentence from
// "you used it a while ago".
func TestThePasswordInForceIsRefusedAsItsOwnRule(t *testing.T) {
	fixture := newPasswordFixture(now)

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New(heldPassword), StepUpToken: "hbt_stp_x"})

	rules := refusedRules(t, err)
	if len(rules) != 1 || rules[0] != string(domain.RuleNotCurrent) {
		t.Fatalf("the refusal named %v, want not_current alone", rules)
	}
}

// The history refuses the last n, and the n+1-th is accepted.
func TestTheHistoryRefusesTheLastFewAndNoMore(t *testing.T) {
	fixture := newPasswordFixture(now)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{HistoryCount: intOf(2)}
	fixture.histories.hashes = []secret.Secret{
		secret.New("hash:the one before this"),
		secret.New("hash:and the one before that"),
		secret.New("hash:the third one back"),
	}

	refused := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{
			Password: secret.New("and the one before that"), StepUpToken: "hbt_stp_x",
		})
	rules := refusedRules(t, refused)
	if len(rules) != 1 || rules[0] != string(domain.RuleHistory) {
		t.Fatalf("the second one back answered %v", rules)
	}

	accepted := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{
			Password: secret.New("the third one back"), StepUpToken: "hbt_stp_x",
		})
	if accepted != nil {
		t.Fatalf("the third one back, past a depth of two, answered %v", accepted)
	}
}

// The minimum age is about the clock rather than about the password, so it is refused on its own
// and a reset ignores it entirely.
func TestTheMinimumAgeRefusesAChangeAndNotAReset(t *testing.T) {
	fixture := newPasswordFixture(now)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{MinAgeHours: intOf(6)}
	fresh := fixture.accounts.rows[account]
	fresh.PasswordSetAt = now.Add(-time.Hour)
	fixture.accounts.rows[account] = fresh

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a change an hour after the last one answered %v", err)
	}

	rules, resolveErr := fixture.writer.ResolveFor(t.Context(), tenant)
	if resolveErr != nil {
		t.Fatalf("resolving: %v", resolveErr)
	}
	if err := fixture.writer.MinimumAge(PasswordCandidate{
		Account: fresh, Rules: rules, IsReset: true,
	}); err != nil {
		t.Errorf("a reset was held to the minimum age: %v", err)
	}
}

// The check demands the same proof the setting demands: without one it is an oracle telling
// anybody whether a word is on a blocklist or in somebody's history.
func TestTheCheckWithoutAProofIsRefused(t *testing.T) {
	fixture := newPasswordFixture(now)

	_, err := CheckPassword{Writer: fixture.writer}.Execute(t.Context(),
		appshared.ActorContext{}, CheckPasswordCommand{Password: secret.New("password123")})

	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("an unproved check answered %v", err)
	}
}

// And with one it answers identifiers, never sentences.
func TestTheCheckAnswersRuleIdentifiers(t *testing.T) {
	fixture := newPasswordFixture(now)

	violations, err := CheckPassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		CheckPasswordCommand{Password: secret.New("Passw0rd!2026")})
	if err != nil {
		t.Fatalf("the check was refused: %v", err)
	}

	if len(violations) != 1 || violations[0].Rule != domain.RuleCommon {
		t.Fatalf("the check answered %+v, want the common list alone", violations)
	}
}

// The check consumes nothing: a question asked while somebody is still typing must not end the
// sign-in it is asked during.
func TestTheCheckSpendsNoCredential(t *testing.T) {
	fixture := newPasswordFixture(now)

	if _, err := (CheckPassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		CheckPasswordCommand{Password: secret.New("seven blue lanterns")}); err != nil {
		t.Fatalf("the check was refused: %v", err)
	}

	if len(fixture.accounts.writes) != 0 || len(fixture.sessions.keptByOthers) != 0 {
		t.Error("the check wrote something")
	}
}

// An account that signs in through a provider has no password to change, and giving it one here
// would be a second way in that nobody asked for.
func TestAnAccountWithNoPasswordCannotChangeOne(t *testing.T) {
	fixture := newPasswordFixture(now)
	held := fixture.accounts.rows[account]
	held.PasswordHash = secret.Secret{}
	fixture.accounts.rows[account] = held

	err := ChangePassword{Writer: fixture.writer}.Execute(t.Context(), signedIn(),
		ChangePasswordCommand{Password: secret.New("seven blue lanterns"), StepUpToken: "hbt_stp_x"})

	if !errors.Is(err, shared.ErrConflict) {
		t.Fatalf("an account with no password answered %v", err)
	}
}

// What is stored is what was counted: NFKC before both, so a password typed on two keyboards is
// one password.
func TestTheStoredHashIsOfTheNormalisedPassword(t *testing.T) {
	fixture := newPasswordFixture(now)

	// A full-width A folds to an ASCII one; the fixture's normaliser folds exactly this.
	if err := (ChangePassword{Writer: fixture.writer}).Execute(t.Context(), signedIn(),
		ChangePasswordCommand{
			Password: secret.New("ＺEBRA marmalade"), StepUpToken: "hbt_stp_x",
		}); err != nil {
		t.Fatalf("the change was refused: %v", err)
	}

	if got := fixture.accounts.writes[0].hash; got != "hash:ZEBRA marmalade" {
		t.Errorf("the hash is of %q, want the normalised form", got)
	}
}

// refusedRules reads the rules a refusal names.
func refusedRules(t *testing.T, err error) []string {
	t.Helper()
	var refusal *shared.Error
	if !errors.As(err, &refusal) {
		t.Fatalf("not a domain refusal: %v", err)
	}
	rules := make([]string, 0, len(refusal.Fields))
	for _, field := range refusal.Fields {
		rules = append(rules, field.Code[len("auth.password_rule."):])
	}
	return rules
}

// notifierFake records what was queued.
type notifierFake struct{ queued []queue.Request }

func (n *notifierFake) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	n.queued = append(n.queued, request)
	return shared.ID("018f2a1b-0000-7000-8000-00000000bb01"), nil
}

// The same answer for an address that holds an account and for one that does not. Asserted on the
// *answer*, because that is the only thing a caller sees - and on what was queued, because that is
// the only thing that differs.
func TestForgettingAnsweersTheSameForEveryAddress(t *testing.T) {
	fixture := newPasswordFixture(now)
	notifier := &notifierFake{}
	handler := ForgetPassword{
		Writer: fixture.writer, Notifier: notifier,
		Tenants: tenantDirectory{single: tenant}, Multi: true,
	}

	held := handler.Execute(t.Context(), ForgetPasswordCommand{
		Email: "bert@example.org", TenantSlug: "acme",
	})
	unheld := handler.Execute(t.Context(), ForgetPasswordCommand{
		Email: "nobody@example.org", TenantSlug: "acme",
	})

	if held != nil || unheld != nil {
		t.Fatalf("the two answers were %v and %v, want both nil", held, unheld)
	}
	if len(notifier.queued) != 1 {
		t.Fatalf("%d mails queued, want one", len(notifier.queued))
	}
	if notifier.queued[0].Payload["account_id"] != account.String() {
		t.Errorf("the job names %v", notifier.queued[0].Payload)
	}
	// Identifiers only: an address in the payload would be one in the queue table and in every
	// log line about the job.
	if _, leaked := notifier.queued[0].Payload["email"]; leaked {
		t.Error("the job payload carries an address")
	}
}

// The request is in the trail exactly where there is a trail to put it in.
func TestOnlyARealAccountsResetRequestIsRecorded(t *testing.T) {
	fixture := newPasswordFixture(now)
	handler := ForgetPassword{
		Writer: fixture.writer, Notifier: &notifierFake{},
		Tenants: tenantDirectory{single: tenant}, Multi: true,
	}

	if err := handler.Execute(t.Context(), ForgetPasswordCommand{
		Email: "nobody@example.org", TenantSlug: "acme",
	}); err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(fixture.audit.entries) != 0 {
		t.Fatalf("an address nobody holds was written into the trail: %v", fixture.audit.entries)
	}

	if err := handler.Execute(t.Context(), ForgetPasswordCommand{
		Email: "bert@example.org", TenantSlug: "acme",
	}); err != nil {
		t.Fatalf("refused: %v", err)
	}
	if len(fixture.audit.entries) != 1 ||
		fixture.audit.entries[0].Action != PasswordResetRequestedAction {
		t.Errorf("the trail holds %v", fixture.audit.entries)
	}
}

// A disabled account gets no link, and says nothing about being disabled.
func TestADisabledAccountGetsNoLinkAndTheSameAnswer(t *testing.T) {
	fixture := newPasswordFixture(now)
	held := fixture.accounts.byEmail["bert@example.org"]
	held.Account.Status = domain.AccountDisabled
	fixture.accounts.byEmail["bert@example.org"] = held
	notifier := &notifierFake{}

	err := ForgetPassword{
		Writer: fixture.writer, Notifier: notifier,
		Tenants: tenantDirectory{single: tenant}, Multi: true,
	}.Execute(t.Context(), ForgetPasswordCommand{Email: "bert@example.org", TenantSlug: "acme"})

	if err != nil {
		t.Fatalf("a disabled account answered %v, want the same silence", err)
	}
	if len(notifier.queued) != 0 {
		t.Error("a link was queued for a disabled account")
	}
}
