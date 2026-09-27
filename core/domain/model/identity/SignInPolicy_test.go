// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func intOf(value int) *int                           { return &value }
func boolOf(value bool) *bool                        { return &value }
func methodsOf(values ...string) *[]string           { return &values }
func requirementOf(r MfaRequirement) *MfaRequirement { return &r }

// What an installation that has decided nothing enforces: every switch at the value NIST advises.
func TestTheShippedRuleIsTheAdvisedOne(t *testing.T) {
	policy := DefaultSignInPolicy()

	if policy.Password.MinLength != MinPasswordLength {
		t.Errorf("minimum length %d, want %d", policy.Password.MinLength, MinPasswordLength)
	}
	for name, on := range map[string]bool{
		"min_lowercase": policy.Password.MinLowercase > 0,
		"min_uppercase": policy.Password.MinUppercase > 0,
		"min_digits":    policy.Password.MinDigits > 0,
		"min_symbols":   policy.Password.MinSymbols > 0,
		"min_classes":   policy.Password.MinClasses > 0,
		"max_repeat":    policy.Password.MaxRepeat > 0,
		"max_age_days":  policy.Password.MaxAgeDays > 0,
		"history_count": policy.Password.HistoryCount > 0,
		"min_age_hours": policy.Password.MinAgeHours > 0,
		"breach_check":  policy.Password.BreachCheck,
	} {
		if on {
			t.Errorf("%s ships on, and NIST SP 800-63B-4 advises against it", name)
		}
	}
	if !policy.Password.CommonPasswords || !policy.Password.ContextWords {
		t.Error("the two offline lists ship off, and NIST asks for them")
	}
	if policy.MfaRequiredFor != MfaForNobody {
		t.Error("enforcement is a decision, never a default")
	}
	if policy.Sessions.MaxDays != CeilingSessionMaxDays {
		t.Errorf("session bound %d, want the refresh token's own horizon", policy.Sessions.MaxDays)
	}
}

// The instance's default reaches every workspace, and the workspace tightens on top of it.
func TestThreeLevelsResolveInOrder(t *testing.T) {
	instance := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(14), HistoryCount: intOf(2)}}
	workspace := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(16)}}

	resolved := Effective(instance, PolicyLayer{}, workspace)

	if resolved.Policy.Password.MinLength != 16 {
		t.Errorf("minimum length %d, want the workspace's 16", resolved.Policy.Password.MinLength)
	}
	if resolved.Policy.Password.HistoryCount != 2 {
		t.Errorf("history %d, want the instance's 2", resolved.Policy.Password.HistoryCount)
	}
	if resolved.IsLocked(SwitchMinLength) {
		t.Error("an unlocked switch reads as locked")
	}
}

// A locked switch is the level above's, whatever the row below it holds - and it says whose.
func TestALockedSwitchKeepsTheLevelAbovesValueAndItsOrigin(t *testing.T) {
	instance := PolicyLayer{
		Patch: PolicyPatch{MinLength: intOf(14)},
		Locks: map[PolicySwitch]bool{SwitchMinLength: true},
	}
	workspace := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(20)}}

	resolved := Effective(instance, PolicyLayer{}, workspace)

	if resolved.Policy.Password.MinLength != 14 {
		t.Errorf("minimum length %d, want the locked 14", resolved.Policy.Password.MinLength)
	}
	if resolved.LockOf(SwitchMinLength) != LockInstance {
		t.Errorf("lock origin %q, want INSTANCE", resolved.LockOf(SwitchMinLength))
	}
}

// A lock without a value pins whatever the level above decided, which is how an operator fixes
// the product's own default without restating it.
func TestALockWithoutAValuePinsTheDefault(t *testing.T) {
	instance := PolicyLayer{Locks: map[PolicySwitch]bool{SwitchCommonPasswords: true}}
	workspace := PolicyLayer{Patch: PolicyPatch{CommonPasswords: boolOf(false)}}

	resolved := Effective(instance, PolicyLayer{}, workspace)

	if !resolved.Policy.Password.CommonPasswords {
		t.Error("a pinned default was switched off from below")
	}
}

// The plan's layer is a parameter with no writer yet (ADR-0070 §3). It resolves between the two,
// so that the milestone that fills it is not a migration through the sign-in path.
func TestThePlansLayerSitsBetweenTheTwo(t *testing.T) {
	instance := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(12)}}
	plan := PolicyLayer{
		Patch: PolicyPatch{MinLength: intOf(15)},
		Locks: map[PolicySwitch]bool{SwitchMinLength: true},
	}
	workspace := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(30)}}

	resolved := Effective(instance, plan, workspace)

	if resolved.Policy.Password.MinLength != 15 {
		t.Errorf("minimum length %d, want the plan's 15", resolved.Policy.Password.MinLength)
	}
	if resolved.LockOf(SwitchMinLength) != LockPlan {
		t.Errorf("lock origin %q, want PLAN", resolved.LockOf(SwitchMinLength))
	}
}

// The product's bounds hold against the operator too: below them a switch protects nobody, above
// them it is a rule nobody can satisfy.
func TestTheProductsBoundsHoldAgainstTheOperator(t *testing.T) {
	resolved := Effective(PolicyLayer{Patch: PolicyPatch{
		MinLength:      intOf(2),
		HistoryCount:   intOf(50),
		MinAgeHours:    intOf(1000),
		MaxAgeDays:     intOf(0),
		SessionMaxDays: intOf(365),
		MinClasses:     intOf(9),
	}}, PolicyLayer{}, PolicyLayer{})

	password, sessions := resolved.Policy.Password, resolved.Policy.Sessions
	for _, check := range []struct {
		name string
		got  int
		want int
	}{
		{"min_length", password.MinLength, FloorMinLength},
		{"history_count", password.HistoryCount, CeilingHistoryCount},
		{"min_age_hours", password.MinAgeHours, CeilingMinAgeHours},
		{"max_age_days", password.MaxAgeDays, 0},
		{"session_max_days", sessions.MaxDays, CeilingSessionMaxDays},
		{"min_classes", password.MinClasses, 4},
	} {
		if check.got != check.want {
			t.Errorf("%s resolved to %d, want %d", check.name, check.got, check.want)
		}
	}
}

// A workspace nobody can sign in to is a support call, not a policy.
func TestTheMethodsAreNeverEmptyAndKeepTheContractsOrder(t *testing.T) {
	resolved := Effective(PolicyLayer{Patch: PolicyPatch{Methods: methodsOf("SEMAPHORE")}},
		PolicyLayer{}, PolicyLayer{})
	if len(resolved.Policy.Methods) != 2 {
		t.Errorf("methods %v, want both back", resolved.Policy.Methods)
	}

	ordered := Effective(PolicyLayer{Patch: PolicyPatch{Methods: methodsOf(MethodOidc, MethodPassword)}},
		PolicyLayer{}, PolicyLayer{})
	if ordered.Policy.Methods[0] != MethodPassword {
		t.Errorf("methods %v, want the contract's order", ordered.Policy.Methods)
	}
}

// The direction of every switch, in one table - because it is not obvious from the type: more
// characters is stricter, fewer days until expiry is stricter, and turning a list on is stricter.
func TestTighteningHasADirectionPerSwitch(t *testing.T) {
	inForce := SignInPolicy{
		Password: PasswordPolicy{
			MinLength: 12, MinLowercase: 1, MinUppercase: 1, MinDigits: 1, MinSymbols: 1,
			MinClasses: 2, MaxRepeat: 3, CommonPasswords: true, ContextWords: true,
			BreachCheck: false, MaxAgeDays: 90, HistoryCount: 3, MinAgeHours: 2,
		},
		MfaRequiredFor: MfaForAdmins,
		Methods:        []string{MethodPassword, MethodOidc},
		Sessions:       SessionPolicy{MaxDays: 14, IdleMinutes: 60},
	}

	tighter := []struct {
		name  PolicySwitch
		patch PolicyPatch
	}{
		{SwitchMinLength, PolicyPatch{MinLength: intOf(16)}},
		{SwitchMinLowercase, PolicyPatch{MinLowercase: intOf(2)}},
		{SwitchMinClasses, PolicyPatch{MinClasses: intOf(4)}},
		{SwitchMaxRepeat, PolicyPatch{MaxRepeat: intOf(2)}},
		{SwitchBreachCheck, PolicyPatch{BreachCheck: boolOf(true)}},
		{SwitchMaxAgeDays, PolicyPatch{MaxAgeDays: intOf(30)}},
		{SwitchHistoryCount, PolicyPatch{HistoryCount: intOf(10)}},
		{SwitchMinAgeHours, PolicyPatch{MinAgeHours: intOf(6)}},
		{SwitchMfaRequiredFor, PolicyPatch{MfaRequiredFor: requirementOf(MfaForEveryone)}},
		{SwitchMethods, PolicyPatch{Methods: methodsOf(MethodOidc)}},
		{SwitchSessionMaxDays, PolicyPatch{SessionMaxDays: intOf(7)}},
		{SwitchSessionIdleMinutes, PolicyPatch{SessionIdleMinutes: intOf(30)}},
	}
	for _, entry := range tighter {
		if err := Tighten(entry.name, inForce, entry.patch); err != nil {
			t.Errorf("%s refused a tightening: %v", entry.name, err)
		}
	}

	looser := []struct {
		name  PolicySwitch
		patch PolicyPatch
	}{
		{SwitchMinLength, PolicyPatch{MinLength: intOf(8)}},
		{SwitchMinDigits, PolicyPatch{MinDigits: intOf(0)}},
		{SwitchMaxRepeat, PolicyPatch{MaxRepeat: intOf(0)}},
		{SwitchMaxRepeat, PolicyPatch{MaxRepeat: intOf(4)}},
		{SwitchCommonPasswords, PolicyPatch{CommonPasswords: boolOf(false)}},
		{SwitchMaxAgeDays, PolicyPatch{MaxAgeDays: intOf(0)}},
		{SwitchMaxAgeDays, PolicyPatch{MaxAgeDays: intOf(180)}},
		{SwitchHistoryCount, PolicyPatch{HistoryCount: intOf(1)}},
		{SwitchMfaRequiredFor, PolicyPatch{MfaRequiredFor: requirementOf(MfaForNobody)}},
		{SwitchMethods, PolicyPatch{Methods: methodsOf(MethodPassword, MethodOidc, "PASSKEY")}},
		{SwitchSessionMaxDays, PolicyPatch{SessionMaxDays: intOf(30)}},
		{SwitchSessionIdleMinutes, PolicyPatch{SessionIdleMinutes: intOf(0)}},
	}
	for _, entry := range looser {
		err := Tighten(entry.name, inForce, entry.patch)
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s accepted a loosening: %v", entry.name, err)
		}
	}
}

// A switch off is the loosest value there is: turning it on is a tightening whatever the number.
func TestASwitchThatIsOffAcceptsAnyValueOnTopOfIt(t *testing.T) {
	off := SignInPolicy{Password: PasswordPolicy{MinLength: 12}}
	for _, patch := range []PolicyPatch{
		{MaxAgeDays: intOf(365)},
		{MaxRepeat: intOf(9)},
		{SessionIdleMinutes: intOf(10000)},
	} {
		for _, name := range patch.Decided() {
			if err := Tighten(name, off, patch); err != nil {
				t.Errorf("%s refused to be switched on: %v", name, err)
			}
		}
	}
}

// The refusal names the field, so a screen with eighteen rows marks the one that was wrong.
func TestALockedSwitchIsRefusedAgainstItsField(t *testing.T) {
	resolved := EffectivePolicy{
		Policy: DefaultSignInPolicy(),
		Locks:  map[PolicySwitch]LockOrigin{SwitchMinLength: LockInstance},
	}

	_, _, err := resolved.Tightened(PolicyPatch{MinLength: intOf(20)})

	var refusal *shared.Error
	if !errors.As(err, &refusal) || len(refusal.Fields) != 1 {
		t.Fatalf("the refusal was %v", err)
	}
	field := refusal.Fields[0]
	if field.Path != "/sign_in_policy/min_length" || field.Code != "auth.policy_locked" {
		t.Errorf("field %q code %q", field.Path, field.Code)
	}
	if field.Params["origin"] != string(LockInstance) {
		t.Errorf("origin %q, want the one a reader is told to ask", field.Params["origin"])
	}
}

// A value set to what it already holds is not a change: a client sending the whole form back
// would otherwise write an audit entry saying nothing happened, every time.
func TestOnlyWhatMovedIsAChange(t *testing.T) {
	resolved := Effective(PolicyLayer{}, PolicyLayer{}, PolicyLayer{})

	_, moved, err := resolved.Tightened(PolicyPatch{
		MinLength:    intOf(MinPasswordLength),
		HistoryCount: intOf(5),
	})
	if err != nil {
		t.Fatalf("the patch was refused: %v", err)
	}
	if len(moved) != 1 || moved[0].Field != string(SwitchHistoryCount) {
		t.Fatalf("changes %v, want only the history", moved)
	}
	if moved[0].From != "0" || moved[0].To != "5" {
		t.Errorf("the change reads %q -> %q", moved[0].From, moved[0].To)
	}
}

// The rotation is a moment, and it only ever moves forward: rewinding it would un-require a
// rotation already asked for, and the sessions it ended are already gone.
func TestTheRotationOnlyMovesForward(t *testing.T) {
	asked := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	inForce := SignInPolicy{RotationFrom: asked}

	later := asked.Add(time.Hour)
	if err := Tighten(SwitchRotationFrom, inForce, PolicyPatch{RotationFrom: &later}); err != nil {
		t.Errorf("a later rotation was refused: %v", err)
	}
	earlier := asked.Add(-time.Hour)
	if err := Tighten(SwitchRotationFrom, inForce, PolicyPatch{RotationFrom: &earlier}); err == nil {
		t.Error("the rotation was rewound")
	}
}

// Every switch the contract declares is addressable, and the patch and the audit spelling agree
// on all of them. A switch nobody can name is a switch nobody can set.
func TestEverySwitchIsAddressable(t *testing.T) {
	moment := time.Date(2026, 9, 26, 0, 0, 0, 0, time.UTC)
	whole := PolicyPatch{
		MinLength: intOf(20), MinLowercase: intOf(1), MinUppercase: intOf(1), MinDigits: intOf(1),
		MinSymbols: intOf(1), MinClasses: intOf(4), MaxRepeat: intOf(2),
		CommonPasswords: boolOf(true), ContextWords: boolOf(true), BreachCheck: boolOf(true),
		MaxAgeDays: intOf(90), HistoryCount: intOf(5), MinAgeHours: intOf(1),
		MfaRequiredFor: requirementOf(MfaForEveryone), Methods: methodsOf(MethodPassword),
		SessionMaxDays: intOf(7), SessionIdleMinutes: intOf(30), RotationFrom: &moment,
	}

	decided := whole.Decided()
	if len(decided) != len(PolicySwitches()) {
		t.Fatalf("%d switches are addressable, want %d", len(decided), len(PolicySwitches()))
	}
	if empty := (PolicyPatch{}); !empty.IsEmpty() {
		t.Error("an empty patch does not read as empty")
	}

	resolved := Effective(PolicyLayer{Patch: whole}, PolicyLayer{}, PolicyLayer{})
	for _, name := range PolicySwitches() {
		if switchText(resolved.Policy, name) == "" && name != SwitchRotationFrom {
			t.Errorf("%s has no spelling for the audit trail", name)
		}
	}
	if switchText(resolved.Policy, SwitchRotationFrom) != moment.Format(time.RFC3339) {
		t.Errorf("the rotation reads %q", switchText(resolved.Policy, SwitchRotationFrom))
	}
}
