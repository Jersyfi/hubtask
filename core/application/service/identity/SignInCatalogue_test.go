// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Every door of the password's life reaches its use case through the catalogue, which is the only
// way a client ever calls one: a descriptor nothing exercised is a route that compiles and answers
// nothing, and a projection nothing read is a field no client receives.
func TestThePasswordsLifeAnswersThroughTheRegistry(t *testing.T) {
	fixture := newStepFixture(now)
	fixture.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(16)}
	rules := GetSignInRules{
		Resolver:   fixture.passwords.writer.Resolver,
		Tenants:    tenantDirectory{single: tenant},
		Providers:  &providerStore{},
		UnitOfWork: &unitOfWork{}, Multi: true,
	}

	registry, err := usecase.NewRegistry(nil,
		rules.Descriptor(),
		ChangePassword{Writer: fixture.passwords.writer}.Descriptor(),
		CheckPassword{Writer: fixture.passwords.writer}.Descriptor(),
		ForgetPassword{
			Writer: fixture.passwords.writer, Notifier: &notifierFake{},
			Tenants: tenantDirectory{single: tenant}, Multi: true,
		}.Descriptor(),
		ResetPassword{Writer: fixture.passwords.writer}.Descriptor(),
		SetPasswordAndSignIn{Writer: fixture.passwords.writer}.Descriptor(),
		RegenerateRecoveryCodes{Writer: fixture.session.writer}.Descriptor(),
		ElevateSession{
			Writer: fixture.session.writer, Operators: &operatorRegister{empty: true},
			UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("the eight are not registrable: %v", err)
	}

	// The public read, with every member a card draws.
	answered, err := registry.Invoke(t.Context(), GetSignInRulesName, appshared.ActorContext{},
		usecase.Input{"host": "acme.hubtask.example", "tenant_slug": "acme"})
	if err != nil {
		t.Fatalf("reading the rules: %v", err)
	}
	for _, member := range []string{"workspace_host", "methods", "providers", "password", "legal"} {
		if _, held := answered[member]; !held {
			t.Errorf("the answer has no %q: %v", member, answered)
		}
	}
	password, _ := answered["password"].(usecase.Output)
	if password["min_length"] != 16 {
		t.Errorf("the rules answered %v", password)
	}
	if password["max_repeat"] != 0 {
		t.Errorf("a switch that is off answered %v, want zero", password["max_repeat"])
	}

	// The check, with the caller's own bearer as its proof.
	checked, err := registry.Invoke(t.Context(), CheckPasswordName, signedIn(),
		usecase.Input{"password": "Passwort2026!x"})
	if err != nil {
		t.Fatalf("checking: %v", err)
	}
	violations, _ := checked["violations"].([]any)
	if len(violations) != 1 {
		t.Fatalf("the check answered %v", checked)
	}
	if row, _ := violations[0].(usecase.Output); row["rule"] != string(domain.RuleCommon) {
		t.Errorf("the violation reads %v", violations[0])
	}

	// And without one it is an oracle, so it refuses.
	if _, err := registry.Invoke(t.Context(), CheckPasswordName, appshared.ActorContext{},
		usecase.Input{"password": "Passwort2026!x"}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("an unproved check answered %v", err)
	}

	// The change, behind the proof the step-up gives.
	if _, err := registry.Invoke(t.Context(), ChangePasswordName, signedIn(), usecase.Input{
		"password": "seven blue lanterns above", "step_up_token": "hbt_stp_x",
	}); err != nil {
		t.Fatalf("changing: %v", err)
	}

	// Forgetting says nothing, whatever it found.
	if _, err := registry.Invoke(t.Context(), ForgetPasswordName, appshared.ActorContext{},
		usecase.Input{"email": "bert@example.org", "tenant_slug": "acme"}); err != nil {
		t.Fatalf("forgetting: %v", err)
	}
}

// The violations projection carries the parameters each rule's sentence takes, because a sentence
// whose placeholder nobody fills is a sentence with a hole in it.
func TestTheViolationsProjectionCarriesItsParameters(t *testing.T) {
	out := violationsOutput([]domain.PasswordViolation{
		{Rule: domain.RuleCommon},
		domain.HistoryViolation(4),
	})

	rows, _ := out["violations"].([]any)
	if len(rows) != 2 {
		t.Fatalf("the projection answered %v", out)
	}
	first, _ := rows[0].(usecase.Output)
	if first["rule"] != "common" {
		t.Errorf("the first reads %v", first)
	}
	if _, held := first["params"]; held {
		t.Errorf("a rule with no parameters carries some: %v", first)
	}
	second, _ := rows[1].(usecase.Output)
	params, _ := second["params"].(usecase.Output)
	if params["count"] != "4" {
		t.Errorf("the history's parameters read %v", second)
	}
}

// The reset token is minted at delivery, so the plaintext exists exactly once - in the message on
// its way out - and an account that signs in through a provider gets a link to nothing.
func TestTheResetTokenIsMintedAtDelivery(t *testing.T) {
	fixture := newResetFixture(now)

	link, err := MintResetToken{Writer: fixture.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if link.Token.IsEmpty() || !link.HasPassword {
		t.Fatalf("the link answered %+v", link)
	}
	if link.Address != "bert@example.org" {
		t.Errorf("the link is addressed to %q", link.Address)
	}
	if _, err := domain.ParsePendingToken(link.Token.Reveal()); err != nil {
		t.Errorf("the token is not one: %v", err)
	}

	// An account that signs in some other way gets the other mail and no token at all.
	held := fixture.accounts.rows[account]
	held.PasswordHash = secret.Secret{}
	fixture.accounts.rows[account] = held
	provider, err := MintResetToken{Writer: fixture.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil {
		t.Fatalf("minting for a provider account: %v", err)
	}
	if provider.HasPassword || !provider.Token.IsEmpty() {
		t.Errorf("a provider account was given a password link: %+v", provider)
	}

	// And an account that is gone is finished business rather than a failure.
	delete(fixture.accounts.rows, account)
	gone, err := MintResetToken{Writer: fixture.writer}.MintResetToken(t.Context(), tenant, account)
	if err != nil || gone.Address != "" {
		t.Errorf("a missing account answered %+v (%v)", gone, err)
	}
}

// The three-level projection is what a screen with eighteen rows draws from: every switch says
// what is in force, what the level above set, and where a lock came from.
func TestTheWorkspacePolicyProjectionSaysAllThreeThings(t *testing.T) {
	fixture := newWorkspacePolicyFixture(now)
	fixture.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(14)}
	fixture.instance.level.Policy.Locks = map[domain.PolicySwitch]bool{domain.SwitchMinLength: true}
	fixture.instance.level.Legal = domain.LegalLayer{
		Links: domain.LegalLinks{ImprintURL: "https://host.example/imprint"},
		Locks: map[domain.LegalLink]bool{domain.LinkImprint: true},
	}
	fixture.store.row.Settings.SignIn = domain.PolicyPatch{HistoryCount: intOf(4)}

	resolved, err := fixture.writer.Resolver.Resolve(t.Context(), tenant)
	if err != nil {
		t.Fatalf("resolving: %v", err)
	}
	out := signInPolicyOutput(resolved)

	password, _ := out["password"].(usecase.Output)
	locked, _ := password["min_length"].(usecase.Output)
	if locked["value"] != 14 || locked["installation"] != 14 || locked["lock"] != "INSTANCE" {
		t.Errorf("the locked switch reads %v", locked)
	}
	open, _ := password["history_count"].(usecase.Output)
	if open["value"] != 4 || open["installation"] != 0 || open["lock"] != nil {
		t.Errorf("the workspace's own switch reads %v", open)
	}

	session, _ := out["session"].(usecase.Output)
	if _, held := session["max_days"]; !held {
		t.Errorf("the session bounds read %v", session)
	}
	legal, _ := out["legal"].(usecase.Output)
	imprint, _ := legal["imprint_url"].(usecase.Output)
	if imprint["value"] != "https://host.example/imprint" || imprint["lock"] != "INSTANCE" {
		t.Errorf("the locked link reads %v", imprint)
	}
	if out["rotation_from"] != nil {
		t.Errorf("a rotation nobody asked for reads %v", out["rotation_from"])
	}
	methods, _ := out["methods"].(usecase.Output)
	if values, _ := methods["value"].([]any); len(values) != 2 {
		t.Errorf("the methods read %v", methods)
	}
}

// The flat patch a client sends is read switch by switch, and a value of the wrong kind is refused
// against its own field rather than silently dropped.
func TestTheFlatPatchIsReadAndChecked(t *testing.T) {
	change, err := policyPatchFrom(map[string]any{
		"min_length": 18, "min_digits": float64(1), "history_count": int64(4),
		"common_passwords": true, "mfa_required_for": "EVERYONE",
		"methods": []any{"PASSWORD"}, "session_idle_minutes": 30,
		"imprint_url": "https://acme.example/imprint", "rotation_from": "now",
		// A key this build does not know is ignored, so a newer client loses the switch it has
		// and not the save.
		"what_i_had_for_lunch": 1,
	})
	if err != nil {
		t.Fatalf("the patch was refused: %v", err)
	}
	if change.Policy.MinLength == nil || *change.Policy.MinLength != 18 {
		t.Errorf("min_length read as %v", change.Policy.MinLength)
	}
	if change.Policy.MinDigits == nil || *change.Policy.MinDigits != 1 {
		t.Errorf("a JSON number read as %v", change.Policy.MinDigits)
	}
	if change.Policy.HistoryCount == nil || *change.Policy.HistoryCount != 4 {
		t.Errorf("a 64-bit number read as %v", change.Policy.HistoryCount)
	}
	if !change.RotateNow {
		t.Error("the rotation was not read")
	}
	if change.Legal[domain.LinkImprint] != "https://acme.example/imprint" {
		t.Errorf("the link read as %q", change.Legal[domain.LinkImprint])
	}
	if change.IsEmpty() {
		t.Error("a patch with nine switches reads as empty")
	}
	if !(WorkspacePolicyChange{Legal: map[domain.LegalLink]string{}}).IsEmpty() {
		t.Error("a patch with nothing in it does not read as empty")
	}

	for _, wrong := range []map[string]any{
		{"min_length": "eighteen"},
		{"common_passwords": 1},
		{"mfa_required_for": "SOMETIMES"},
		{"mfa_required_for": 1},
		{"methods": "PASSWORD"},
		{"methods": []any{1}},
		{"imprint_url": 1},
		{"rotation_from": "2026-09-27T00:00:00Z"},
	} {
		if _, err := policyPatchFrom(wrong); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%v answered %v", wrong, err)
		}
	}
}

// The count of recovery codes rides beside the account, where somebody can act on it.
func TestTheOwnAccountCarriesTheRecoveryCount(t *testing.T) {
	fixture := newRecoveryFixture(t)
	handler := GetOwnAccount{
		Accounts:   fixture.writer.People,
		UnitOfWork: &unitOfWork{},
		Recovery:   fixture.writer.Recovery,
	}
	actor := recoveryActor()
	actor.Scopes = []string{accountsRead}

	own, err := handler.ExecuteWithRecovery(t.Context(), actor)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if own.Account.ID != account {
		t.Errorf("the account reads %v", own.Account.ID)
	}
	if own.RecoveryCodesRemaining != 0 {
		t.Errorf("the count reads %d, want zero answered as zero", own.RecoveryCodesRemaining)
	}

	// Without the store there is nothing to count, and the member is absent rather than zero.
	handler.Recovery = nil
	none, err := handler.ExecuteWithRecovery(t.Context(), actor)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if none.RecoveryCodesRemaining != -1 {
		t.Errorf("an installation with no second factor answered %d", none.RecoveryCodesRemaining)
	}
}

// The four that answer with a session or with a secret, through the catalogue: the reset, the
// change step, the codes and the elevation each have a projection no other test reads.
func TestTheRemainingDoorsAnswerThroughTheRegistry(t *testing.T) {
	reset := newResetFixture(now)
	token := reset.mintedFor(t, now, domain.PendingReset)

	registry, err := usecase.NewRegistry(nil,
		ResetPassword{Writer: reset.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	pair, err := registry.Invoke(t.Context(), ResetPasswordName, appshared.ActorContext{},
		usecase.Input{"token": token, "password": "seven blue lanterns above"})
	if err != nil {
		t.Fatalf("resetting: %v", err)
	}
	if pair.String("token_type") != "Bearer" || pair.String("access_token") == "" {
		t.Errorf("the reset answered %v", pair)
	}

	// The change step, which answers the session the sign-in was going to open.
	step := newStepFixture(now)
	step.passwords.instance.level.Policy.Patch = domain.PolicyPatch{MinLength: intOf(30)}
	challenge := step.signsIn(t, "correct horse battery").Challenge
	if challenge == nil {
		t.Fatal("no challenge to complete")
	}
	steps, err := usecase.NewRegistry(nil,
		SetPasswordAndSignIn{Writer: step.passwords.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	if _, err := steps.Invoke(t.Context(), SetPasswordAndSignInName, appshared.ActorContext{},
		usecase.Input{
			"pending_token": challenge.Token.Reveal(),
			"password":      "seven blue lanterns above the quiet harbour",
		}); err != nil {
		t.Fatalf("completing the step: %v", err)
	}

	// The codes, shown once.
	codes := newRecoveryFixture(t)
	fresh, err := usecase.NewRegistry(nil,
		RegenerateRecoveryCodes{Writer: codes.writer}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	answered, err := fresh.Invoke(t.Context(), RegenerateRecoveryCodesName, recoveryActor(),
		usecase.Input{"step_up_token": codes.proof})
	if err != nil {
		t.Fatalf("regenerating: %v", err)
	}
	if shown, _ := answered["recovery_codes"].([]any); len(shown) != domain.RecoveryCodeCount {
		t.Errorf("the answer carries %v", answered)
	}

	// And the elevation, which says how long is left.
	raised := newRecoveryFixture(t)
	elevations, err := usecase.NewRegistry(nil, ElevateSession{
		Writer: raised.writer, Operators: &operatorRegister{empty: true},
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	elevated, err := elevations.Invoke(t.Context(), ElevateSessionName, recoveryActor(),
		usecase.Input{"step_up_token": raised.proof})
	if err != nil {
		t.Fatalf("elevating: %v", err)
	}
	if elevated.Int("remaining_seconds") != int(domain.ElevationLifetime.Seconds()) {
		t.Errorf("the elevation answered %v", elevated)
	}
}
