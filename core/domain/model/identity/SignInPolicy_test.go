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

	ordered := Effective(PolicyLayer{Patch: PolicyPatch{Methods: methodsOf(MethodOidc, MethodDirect)}},
		PolicyLayer{}, PolicyLayer{})
	if ordered.Policy.Methods[0] != MethodDirect {
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
		Methods:        []string{MethodDirect, MethodOidc},
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
		{SwitchMethods, PolicyPatch{Methods: methodsOf(MethodDirect, MethodOidc, "PASSKEY")}},
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

	_, _, err := resolved.Tightened(DefaultSignInPolicy(), PolicyPatch{MinLength: intOf(20)})

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

// A form sends every row it shows, including the rows it is not allowed to move. Sending a locked
// switch back at the value it already holds asks for nothing, so it is refused by nothing - and
// without that, moving any one switch on a screen where some other switch is locked answers a
// refusal naming a switch nobody touched, and writes neither.
func TestALockedSwitchSentBackUnchangedIsNotARefusal(t *testing.T) {
	resolved := EffectivePolicy{
		Policy: DefaultSignInPolicy(),
		Locks:  map[PolicySwitch]LockOrigin{SwitchCommonPasswords: LockInstance},
	}

	result, moved, err := resolved.Tightened(DefaultSignInPolicy(), PolicyPatch{
		CommonPasswords: boolOf(resolved.Policy.Password.CommonPasswords),
		BreachCheck:     boolOf(!resolved.Policy.Password.BreachCheck),
	})
	if err != nil {
		t.Fatalf("a locked switch at its own value was refused: %v", err)
	}
	if len(moved) != 1 || moved[0].Field != string(SwitchBreachCheck) {
		t.Fatalf("changes %v, want only the one that moved", moved)
	}
	if result.Password.BreachCheck == DefaultSignInPolicy().Password.BreachCheck {
		t.Error("the switch that was allowed to move did not move")
	}

	// Moving it is still refused, which is the whole of the lock.
	if _, _, err := resolved.Tightened(DefaultSignInPolicy(), PolicyPatch{
		CommonPasswords: boolOf(!resolved.Policy.Password.CommonPasswords),
	}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("moving a locked switch answered %v", err)
	}
}

// A workspace may relax its *own* tightening, down to the level above and no further. Without this
// a workspace's setting was a one-way ratchet: somebody who raised the minimum length to twenty
// could not put it back to nineteen, because nineteen is below what they themselves had just typed -
// so a mistyped switch was permanent and the only way back was the control plane.
func TestAWorkspaceMayRelaxItsOwnTighteningDownToTheLevelAbove(t *testing.T) {
	instance := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(12), SessionMaxDays: intOf(30)}}
	// The workspace has already gone past it, in both directions the switches run.
	workspace := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(20), SessionMaxDays: intOf(7)}}
	resolved := Effective(instance, PolicyLayer{}, workspace)
	above := Effective(instance, PolicyLayer{}, PolicyLayer{}).Policy

	result, moved, err := resolved.Tightened(above, PolicyPatch{
		MinLength:      intOf(16),
		SessionMaxDays: intOf(14),
	})
	if err != nil {
		t.Fatalf("relaxing its own value towards the installation's was refused: %v", err)
	}
	if result.Password.MinLength != 16 || result.Sessions.MaxDays != 14 {
		t.Errorf("the rule reads %d/%d, want 16/14", result.Password.MinLength, result.Sessions.MaxDays)
	}
	if len(moved) != 2 {
		t.Errorf("changes %v, want both", moved)
	}

	// And not one step past it. The installation's floor is the thing that holds.
	for _, patch := range []PolicyPatch{{MinLength: intOf(11)}, {SessionMaxDays: intOf(31)}} {
		_, _, err := resolved.Tightened(above, patch)
		var refusal *shared.Error
		if !errors.As(err, &refusal) || refusal.DetailCode != "auth.policy_loosens" {
			t.Errorf("going below the installation answered %v", err)
		}
	}
}

// A value set to what it already holds is not a change: a client sending the whole form back
// would otherwise write an audit entry saying nothing happened, every time.
func TestOnlyWhatMovedIsAChange(t *testing.T) {
	resolved := Effective(PolicyLayer{}, PolicyLayer{}, PolicyLayer{})

	_, moved, err := resolved.Tightened(DefaultSignInPolicy(), PolicyPatch{
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
		MfaRequiredFor: requirementOf(MfaForEveryone), Methods: methodsOf(MethodDirect),
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
		if SwitchText(resolved.Policy, name) == "" && name != SwitchRotationFrom {
			t.Errorf("%s has no spelling for the audit trail", name)
		}
	}
	if SwitchText(resolved.Policy, SwitchRotationFrom) != moment.Format(time.RFC3339) {
		t.Errorf("the rotation reads %q", SwitchText(resolved.Policy, SwitchRotationFrom))
	}
}

// A workspace's stored row is not a licence to stay behind. An operator who raises the minimum has
// raised it for the workspace that stored a lower value last year too - which is what makes "a
// workspace only ever tightens" true over time and not only at the moment of the write.
func TestAStoredWorkspaceValueCannotOutliveATighterDefault(t *testing.T) {
	workspace := PolicyLayer{Patch: PolicyPatch{MinLength: intOf(14), HistoryCount: intOf(6)}}

	before := Effective(PolicyLayer{Patch: PolicyPatch{MinLength: intOf(12)}}, PolicyLayer{}, workspace)
	if before.Policy.Password.MinLength != 14 {
		t.Errorf("minimum length %d, want the workspace's 14", before.Policy.Password.MinLength)
	}

	after := Effective(PolicyLayer{Patch: PolicyPatch{MinLength: intOf(16)}}, PolicyLayer{}, workspace)
	if after.Policy.Password.MinLength != 16 {
		t.Errorf("minimum length %d, want the instance's raised 16", after.Policy.Password.MinLength)
	}
	if after.Policy.Password.HistoryCount != 6 {
		t.Errorf("history %d, want the workspace's tightening left standing", after.Policy.Password.HistoryCount)
	}
}

// A save of three switches is a save of three switches: what the caller did not send stays.
func TestAPatchMergesRatherThanReplaces(t *testing.T) {
	stored := PolicyPatch{MinLength: intOf(14), HistoryCount: intOf(3), BreachCheck: boolOf(true)}

	merged := stored.Merge(PolicyPatch{HistoryCount: intOf(5)})

	if merged.MinLength == nil || *merged.MinLength != 14 {
		t.Error("an untouched switch was lost")
	}
	if merged.HistoryCount == nil || *merged.HistoryCount != 5 {
		t.Error("the sent switch did not move")
	}
	if merged.BreachCheck == nil || !*merged.BreachCheck {
		t.Error("an untouched flag was lost")
	}
}

// Merge is one switch at a time, and every one of them. A switch that merged into the wrong field
// would be a setting somebody saved and another they lost.
func TestEverySwitchMergesIntoItsOwnField(t *testing.T) {
	moment := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC)
	whole := PolicyPatch{
		MinLength: intOf(20), MinLowercase: intOf(1), MinUppercase: intOf(2), MinDigits: intOf(3),
		MinSymbols: intOf(4), MinClasses: intOf(4), MaxRepeat: intOf(2),
		CommonPasswords: boolOf(true), ContextWords: boolOf(true), BreachCheck: boolOf(true),
		MaxAgeDays: intOf(90), HistoryCount: intOf(5), MinAgeHours: intOf(6),
		MfaRequiredFor: requirementOf(MfaForEveryone), Methods: methodsOf(MethodDirect),
		SessionMaxDays: intOf(7), SessionIdleMinutes: intOf(30), RotationFrom: &moment,
	}

	merged := PolicyPatch{}.Merge(whole)
	if len(merged.Decided()) != len(PolicySwitches()) {
		t.Fatalf("%d switches merged, want %d", len(merged.Decided()), len(PolicySwitches()))
	}

	resolved := Effective(PolicyLayer{Patch: merged}, PolicyLayer{}, PolicyLayer{})
	for name, want := range map[PolicySwitch]string{
		SwitchMinLength: "20", SwitchMinLowercase: "1", SwitchMinUppercase: "2",
		SwitchMinDigits: "3", SwitchMinSymbols: "4", SwitchMinClasses: "4",
		SwitchMaxRepeat: "2", SwitchCommonPasswords: "true", SwitchContextWords: "true",
		SwitchBreachCheck: "true", SwitchMaxAgeDays: "90", SwitchHistoryCount: "5",
		SwitchMinAgeHours: "6", SwitchMfaRequiredFor: "EVERYONE", SwitchMethods: "PASSWORD",
		SwitchSessionMaxDays: "7", SwitchSessionIdleMinutes: "30",
	} {
		if got := SwitchText(resolved.Policy, name); got != want {
			t.Errorf("%s merged to %q, want %q", name, got, want)
		}
	}
}

// A switch nobody declared is neither carried nor applied: a name the domain does not know is a
// name nothing enforces, and quietly storing one would be a setting that looks set and does nothing.
func TestAnUndeclaredSwitchIsCarriedByNothing(t *testing.T) {
	patch := PolicyPatch{MinLength: intOf(20)}

	if patch.carries("what_i_had_for_lunch") {
		t.Error("a name nobody declared reads as decided")
	}
	if SwitchText(DefaultSignInPolicy(), "what_i_had_for_lunch") != "" {
		t.Error("a name nobody declared has a spelling")
	}
	unchanged := applySwitch(DefaultSignInPolicy(), "what_i_had_for_lunch", patch)
	if unchanged.Password != DefaultSignInPolicy().Password ||
		unchanged.MfaRequiredFor != DefaultSignInPolicy().MfaRequiredFor {
		t.Error("a name nobody declared changed the policy")
	}
	// And Tighten has nothing to check: the patch does not carry it. That is safe rather than lax,
	// because `Tightened` only ever walks `Decided()`, which never names it either - so a switch
	// nobody declared reaches no comparison and no write.
	if err := Tighten("what_i_had_for_lunch", DefaultSignInPolicy(), patch); err != nil {
		t.Errorf("a name the patch does not carry was judged: %v", err)
	}
	if _, moved, err := (EffectivePolicy{Policy: DefaultSignInPolicy()}).Tightened(DefaultSignInPolicy(),
		PolicyPatch{},
	); err != nil || len(moved) != 0 {
		t.Errorf("an empty patch moved %v (%v)", moved, err)
	}
}

// The old boolean, written, as the rule it has always meant (UC-ID-12 check 2): on is
// "administrators or stricter", off is nobody, and a flag sent as it stands moves nothing.
func TestTheOldFlagTranslatesIntoTheRule(t *testing.T) {
	cases := []struct {
		flag    bool
		inForce MfaRequirement
		want    MfaRequirement
		moves   bool
	}{
		{true, MfaForNobody, MfaForAdmins, true},
		{true, MfaForAdmins, MfaForAdmins, false},
		{true, MfaForEveryone, MfaForEveryone, false},
		{false, MfaForNobody, MfaForNobody, false},
		{false, MfaForAdmins, MfaForNobody, true},
		{false, MfaForEveryone, MfaForNobody, true},
	}
	for _, c := range cases {
		got, moves := RequirementForAdminFlag(c.flag, c.inForce)
		if got != c.want || moves != c.moves {
			t.Errorf("flag %v over %s: (%s, %v), want (%s, %v)", c.flag, c.inForce, got, moves, c.want, c.moves)
		}
		if reads := got.CoversAdmins(); reads != c.flag {
			t.Errorf("flag %v over %s reads back as %v", c.flag, c.inForce, reads)
		}
	}
}
