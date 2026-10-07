// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// stepFixture is a sign-in that knows the rule: the session machinery of H-02 with ADR-0068's
// resolver behind it, so that the change step can be walked end to end.
type stepFixture struct {
	session   *sessionFixture
	passwords *passwordFixture
	pending   *pendingStore
	enroll    *enrollmentsStore
}

func newStepFixture(at time.Time) *stepFixture {
	session := newSessionFixture(at)
	passwords := newPasswordFixture(at)
	pending := newPending()
	enroll := newEnrollments()

	session.writer.Pending = pending
	session.writer.Enrollments = enroll
	session.writer.Recovery = newRecovery()
	session.writer.Memberships = membershipsFake{}

	passwords.writer.Session = session.writer
	passwords.writer.Pending = pending
	session.writer.Rule = passwords.writer

	return &stepFixture{session: session, passwords: passwords, pending: pending, enroll: enroll}
}

// signsIn puts an account behind the password and returns the sign-in's answer.
func (f *stepFixture) signsIn(t *testing.T, password string) SignInResult {
	t.Helper()
	f.session.withAccount("bert@example.org", password)
	result, err := SignIn{Writer: f.session.writer}.Execute(t.Context(), SignInCommand{
		Email: "bert@example.org", Password: secret.New(password),
	})
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	return result
}

// A tightened rule routes the next sign-in into the change step, and the challenge carries the rule
// the new password will be judged against.
func TestATightenedRuleRoutesTheNextSignInIntoTheChangeStep(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}

	result := fixture.signsIn(t, "correct horse battery")

	if result.Challenge == nil {
		t.Fatal("a password below the tightened minimum signed straight in")
	}
	if len(result.Challenge.Methods) != 1 || result.Challenge.Methods[0] != methodPasswordChange {
		t.Fatalf("the challenge owes %v", result.Challenge.Methods)
	}
	if result.Challenge.PasswordRules == nil {
		t.Fatal("the challenge carries no rules, so the field would have no list")
	}
	if result.Challenge.PasswordRules.MinLength != 30 {
		t.Errorf("the rules say %d characters", result.Challenge.PasswordRules.MinLength)
	}
}

// And a conforming password goes straight through, which is what makes a policy change invisible to
// everybody who already satisfies it.
func TestAConformingPasswordIsNotStopped(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(16)}

	result := fixture.signsIn(t, "a long enough passphrase")

	if result.Challenge != nil {
		t.Fatalf("a conforming password was stopped: %v", result.Challenge.Methods)
	}
	if result.Pair == nil {
		t.Fatal("no session answered")
	}
}

// A password older than the rotation moment meets the step; one set after it does not.
func TestTheRotationRoutesOnlyThePasswordsOlderThanIt(t *testing.T) {
	older := newStepFixture(now)
	older.passwords.instance.level.Policy.Patch = domain.PolicyPatch{}
	older.passwords.workspace.row.Settings.SignIn = rotationAt(now.Add(-time.Hour))
	if older.signsIn(t, "a long enough passphrase").Challenge == nil {
		t.Error("a password set before the rotation signed straight in")
	}

	newer := newStepFixture(now)
	newer.passwords.workspace.row.Settings.SignIn = rotationAt(now.Add(-40 * 24 * time.Hour))
	if challenge := newer.signsIn(t, "a long enough passphrase").Challenge; challenge != nil {
		t.Errorf("a password set after the rotation met %v", challenge.Methods)
	}
}

func rotationAt(moment time.Time) domain.PolicyPatch {
	return domain.PolicyPatch{RotationFrom: &moment}
}

// The session a sign-in opens carries the workspace's bounds and how it was opened.
func TestTheSessionCarriesTheWorkspacesBoundsAndItsMethod(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		SessionMaxDays: intOf(7), SessionIdleMinutes: intOf(30),
	}

	result := fixture.signsIn(t, "a long enough passphrase")

	if result.Pair == nil {
		t.Fatal("no session answered")
	}
	opened := result.Pair.Session
	if opened.SignedInWith != domain.SignedInWithPassword {
		t.Errorf("the session records %q", opened.SignedInWith)
	}
	if opened.IdleMinutes != 30 {
		t.Errorf("the idle bound is %d", opened.IdleMinutes)
	}
	if want := now.Add(7 * 24 * time.Hour); !opened.HardExpiresAt.Equal(want) {
		t.Errorf("the hard expiry is %v, want %v", opened.HardExpiresAt, want)
	}
}

// `EVERYONE` reaches a person and not a service account: an authenticator nobody holds cannot be
// demanded of an actor with nobody behind it.
func TestEveryoneReachesAPersonAndNotAServiceAccount(t *testing.T) {
	person := newStepFixture(now)
	person.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone),
	}
	result := person.signsIn(t, "a long enough passphrase")
	if result.Challenge == nil || result.Challenge.Methods[0] != methodEnroll {
		t.Fatalf("an ordinary member was not routed into enrolment: %+v", result.Challenge)
	}

	service := newStepFixture(now)
	service.passwords.instance.level.Policy.Patch = domain.PolicyPatch{
		MfaRequiredFor: requirementOf(domain.MfaForEveryone),
	}
	service.session.withAccount("bot@example.org", "a long enough passphrase")
	held := service.session.accounts.byEmail["bot@example.org"]
	held.Account.Kind = domain.AccountServiceAccount
	service.session.accounts.byEmail["bot@example.org"] = held

	answer, err := SignIn{Writer: service.session.writer}.Execute(t.Context(), SignInCommand{
		Email: "bot@example.org", Password: secret.New("a long enough passphrase"),
	})
	if err != nil {
		t.Fatalf("signing in: %v", err)
	}
	if answer.Challenge != nil {
		t.Errorf("a service account was asked for an authenticator: %v", answer.Challenge.Methods)
	}
}

func requirementOf(r domain.MfaRequirement) *domain.MfaRequirement { return &r }

func boolOf(value bool) *bool { return &value }

// The change step's credential can do one thing, and doing it is the sign-in.
func TestTheChangeStepSetsThePasswordAndFinishesTheSignIn(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
	challenge := fixture.signsIn(t, "correct horse battery").Challenge
	if challenge == nil {
		t.Fatal("no challenge to complete")
	}

	pair, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: challenge.Token,
			Password:     secret.New("seven blue lanterns above the quiet harbour"),
		})
	if err != nil {
		t.Fatalf("the change step was refused: %v", err)
	}
	if pair.Session.ID.IsZero() {
		t.Error("no session was opened")
	}
	if len(fixture.passwords.accounts.writes) != 1 {
		t.Errorf("%d passwords written", len(fixture.passwords.accounts.writes))
	}

	// And the credential is spent: presenting it again completes nothing.
	if _, err := (SetPasswordAndSignIn{Writer: fixture.passwords.writer}).Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: challenge.Token,
			Password:     secret.New("eight green lanterns above the quiet harbour"),
		}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("the credential worked twice: %v", err)
	}
}

// A password the rule still refuses leaves the credential unspent, so the person can try again.
func TestTheChangeStepRefusesAPasswordTheRuleRefuses(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
	challenge := fixture.signsIn(t, "correct horse battery").Challenge

	_, err := SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: challenge.Token, Password: secret.New("still too short"),
		})
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a short password answered %v", err)
	}

	if _, err := (SetPasswordAndSignIn{Writer: fixture.passwords.writer}).Execute(t.Context(),
		SetPasswordAndSignInCommand{
			PendingToken: challenge.Token,
			Password:     secret.New("seven blue lanterns above the quiet harbour"),
		}); err != nil {
		t.Fatalf("the credential was spent by the refusal: %v", err)
	}
}

// The workspace tightens through the patch, behind the step-up, and a locked switch is refused
// against its own field.
func TestTheWorkspacePatchTightensBehindAStepUp(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)

	_, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn: WorkspacePolicyChange{Policy: domain.PolicyPatch{MinLength: intOf(16)}},
		})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a policy change without a proof answered %v", err)
	}

	changed, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn:      WorkspacePolicyChange{Policy: domain.PolicyPatch{MinLength: intOf(16)}},
			StepUpToken: "hbt_stp_x",
		})
	if err != nil {
		t.Fatalf("the tightening was refused: %v", err)
	}
	if changed.Settings.SignIn.MinLength == nil || *changed.Settings.SignIn.MinLength != 16 {
		t.Errorf("the stored patch holds %v", changed.Settings.SignIn.MinLength)
	}
}

// A switch the installation locked is not the workspace's to touch, and the refusal names it.
func TestALockedSwitchIsRefusedByTheService(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{
		domain.SwitchCommonPasswords: true,
	}

	_, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn: WorkspacePolicyChange{
				Policy: domain.PolicyPatch{CommonPasswords: boolOf(false)},
			},
			StepUpToken: "hbt_stp_x",
		})

	var refusal *shared.Error
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 {
		t.Fatalf("the refusal was %v", err)
	}
	if refusal.Fields[0].Path != "/sign_in_policy/common_passwords" {
		t.Errorf("the refusal names %q", refusal.Fields[0].Path)
	}
}

// The rotation is one write. Nothing is enqueued and nothing is walked, which is what makes it cost
// the same for ten accounts and for ten thousand.
func TestTheRotationIsOneWriteAndNoJob(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)

	changed, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn:      WorkspacePolicyChange{RotateNow: true},
			StepUpToken: "hbt_stp_x",
		})
	if err != nil {
		t.Fatalf("the rotation was refused: %v", err)
	}
	if changed.Settings.SignIn.RotationFrom == nil ||
		!changed.Settings.SignIn.RotationFrom.Equal(now.UTC()) {
		t.Fatalf("the moment written is %v", changed.Settings.SignIn.RotationFrom)
	}
	if len(fixture.store.updates) != 1 {
		t.Errorf("%d writes, want one", len(fixture.store.updates))
	}
}

// `mfa_required_for` writes the old boolean back, so no stored row and no client has to move.
func TestTheRequirementWritesTheOldBooleanBack(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)

	changed, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn: WorkspacePolicyChange{
				Policy: domain.PolicyPatch{MfaRequiredFor: requirementOf(domain.MfaForEveryone)},
			},
			StepUpToken: "hbt_stp_x",
		})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !changed.Settings.RequireAdminTotp {
		t.Error("the old boolean was left off while the new switch demands a factor of everybody")
	}
}

// A legal link the instance locked is refused; an open one is stored and checked.
func TestTheLegalLinksAreCheckedAndLockable(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)

	if _, err := (UpdateWorkspace{Writer: fixture.writer}).Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn: WorkspacePolicyChange{
				Legal: map[domain.LegalLink]string{domain.LinkImprint: "javascript:alert(1)"},
			},
			StepUpToken: "hbt_stp_x",
		}); !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("a script link answered %v", err)
	}

	changed, err := UpdateWorkspace{Writer: fixture.writer}.Execute(t.Context(), workspaceActor(),
		UpdateWorkspaceCommand{
			SignIn: WorkspacePolicyChange{
				Legal: map[domain.LegalLink]string{domain.LinkImprint: "https://acme.example/imprint"},
			},
			StepUpToken: "hbt_stp_x",
		})
	if err != nil {
		t.Fatalf("a real link was refused: %v", err)
	}
	if changed.Settings.Legal.ImprintURL != "https://acme.example/imprint" {
		t.Errorf("the stored link is %q", changed.Settings.Legal.ImprintURL)
	}
}

// workspacePolicyFixture is the workspace writer with the resolver and the step-up wired.
type workspacePolicyFixture struct {
	writer   WorkspaceWriter
	store    *workspaceStore
	instance *instanceSettings
	audit    *auditSink
	stepUp   *stepUpFake
}

func newWorkspacePolicyFixture(at time.Time) *workspacePolicyFixture {
	store := &workspaceStore{row: domain.Workspace{
		Tenant: domain.Tenant{
			ID: tenant, Slug: "acme", DisplayName: "Acme", Status: domain.TenantActive,
			DefaultLocale: "en", DefaultTimeZone: "UTC",
		},
		Version: 4,
	}}
	instance := &instanceSettings{level: repository.InstanceLevel{
		Policy: domain.PolicyLayer{Locks: map[domain.PolicySwitch]bool{}},
		Legal:  domain.LegalLayer{Locks: map[domain.LegalLink]bool{}},
	}}
	f := &workspacePolicyFixture{
		store: store, instance: instance, audit: &auditSink{},
		stepUp: &stepUpFake{satisfied: true},
	}
	f.writer = WorkspaceWriter{
		Workspaces: store, Authorizer: &authorizer{}, Audit: f.audit,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(at), Text: text.Composing{},
		Resolver: SignInPolicyResolver{
			Workspaces: store, Instance: instance, UnitOfWork: &unitOfWork{},
		},
		StepUp: f.stepUp,
	}
	return f
}
