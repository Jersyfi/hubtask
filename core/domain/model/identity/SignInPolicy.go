// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"sort"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The sign-in rule of ADR-0068: the switches, the three levels, and the lock.
//
// The policy is an object rather than a number because two audiences are real at once. NIST
// SP 800-63B-4 advises against composition rules and periodic expiry; PCI DSS 4.0 §8.3.9 requires
// an expiry where a password is the only factor. An installation that could only be one of those
// would exclude the other, so every switch ships at the value NIST advises and can be turned up.
//
// Three levels decide it:
//
//	product minimum  ->  the instance's default and lock  ->  the workspace, where no lock lies
//
// The product's minimum is here, as constants: nobody goes below it, the operator included. The
// instance's level and the plan's live in the instance layer (ADR-0070); the workspace's lives in
// its settings document. A workspace only ever tightens, and Effective is where that is decided -
// once, so that the sign-in path, the setting screen and the check route cannot disagree.

// The product's own bounds. Not preferences: the values below which a switch stops protecting
// anybody, and above which it stops being usable.
const (
	// FloorMinLength is the shortest password any installation may demand. Eight is the floor
	// NIST SP 800-63B-4 §3.1.1.2 sets for a verifier-chosen minimum; twelve is the *default*,
	// and an installation that relaxes it stops at eight.
	FloorMinLength = 8
	// CeilingMinLength keeps a demanded minimum inside what MaxPasswordLength allows to be
	// typed at all. A rule nobody can satisfy locks the whole installation out of setting a
	// password, which is a defect rather than a policy.
	CeilingMinLength = 128
	// CeilingHistoryCount is why history is affordable: n comparisons per set, capped.
	CeilingHistoryCount = 10
	// CeilingSessionMaxDays is the horizon of the refresh token itself (RefreshTokenLifetime).
	// A session may not outlive the credential that renews it.
	CeilingSessionMaxDays = 30
	// CeilingMinAgeHours bounds how long somebody may be held away from their own password. A
	// person who suspects a password is compromised and cannot replace it is worse off than one
	// who can; a day is the most that reads as friction rather than as a lockout, and a reset
	// ignores the bound entirely (ADR-0068 §6).
	CeilingMinAgeHours = 24
	// FloorMaxAgeDays keeps an expiry from being an expiry every few hours.
	FloorMaxAgeDays = 1
)

// MfaRequirement is who the workspace demands a second factor of.
type MfaRequirement string

const (
	// MfaForNobody is the shipped value: enforcement is a decision, never a default.
	MfaForNobody MfaRequirement = "NONE"
	// MfaForAdmins is what `require_admin_totp` has meant since 0.6.0. The old boolean derives
	// from this one and writes back to it, so no stored row and no client has to move.
	MfaForAdmins MfaRequirement = "ADMINS"
	// MfaForEveryone reaches every person of the workspace - and no service account, which has
	// no authenticator and no person behind it to hold one.
	MfaForEveryone MfaRequirement = "EVERYONE"
)

// rank orders the three, so that "tighter" is one comparison rather than a table.
func (r MfaRequirement) rank() int {
	switch r {
	case MfaForEveryone:
		return 2
	case MfaForAdmins:
		return 1
	default:
		return 0
	}
}

// Valid reports whether this is one of the three the contract declares.
func (r MfaRequirement) Valid() bool {
	return r == MfaForNobody || r == MfaForAdmins || r == MfaForEveryone
}

// The ways into a workspace. `methods` narrows them: a workspace that signs in only through its
// provider switches the password field off for everybody, which is the reason the switch exists.
const (
	MethodPassword = "PASSWORD"
	MethodOidc     = "OIDC"
)

// PasswordPolicy is the thirteen switches about the string itself.
//
// Zero is off for every count, which is what makes the zero value of the whole struct the
// loosest policy the product permits rather than an accidental one. `blocklist_file` is
// deliberately not here: the operator's file lives on the operator's disk, so it is an instance
// switch a workspace can neither point at nor read, and a password found in it is refused by the
// same `common` rule as one from the embedded list.
type PasswordPolicy struct {
	MinLength    int
	MinLowercase int
	MinUppercase int
	MinDigits    int
	MinSymbols   int
	// MinClasses counts how many of the four kinds appear. It cannot be expressed in the four
	// counts above - "one digit and one symbol" is not "two of four" - and both are what real
	// policies ask for, which is why both exist.
	MinClasses int
	// MaxRepeat is the longest run of one character allowed. 0 is off.
	MaxRepeat int
	// CommonPasswords checks the embedded list and the operator's file, offline.
	CommonPasswords bool
	// ContextWords refuses a password that carries the address, the person's name, the
	// workspace's name or its host.
	ContextWords bool
	// BreachCheck asks the corpus an installation configured. Off where none is.
	BreachCheck bool
	// MaxAgeDays expires a password. 0 is off, which is the shipped value and the advised one.
	MaxAgeDays int
	// HistoryCount is how many previous passwords are refused. 0 is off.
	HistoryCount int
	// MinAgeHours is how long a password must stand before it may be replaced - the switch that
	// stops a history being walked in a minute. 0 is off, and a reset ignores it.
	MinAgeHours int
}

// SessionPolicy is the two bounds a session answers to beside its own expiry.
type SessionPolicy struct {
	// MaxDays is the longest a session may live however often it is refreshed. 0 means the
	// refresh token's own horizon is the only bound.
	MaxDays int
	// IdleMinutes ends a session nobody has used. 0 is off.
	IdleMinutes int
}

// SignInPolicy is the whole rule as it stands for one workspace at one moment.
type SignInPolicy struct {
	Password PasswordPolicy
	// MfaRequiredFor is `require_admin_totp` grown up.
	MfaRequiredFor MfaRequirement
	// Methods are the ways in, in the order a screen draws them. Empty means the product's
	// default: the password and any provider that is configured.
	Methods  []string
	Sessions SessionPolicy
	// RotationFrom is the moment "require a new password from everyone" was pressed. A password
	// set before it meets PASSWORD_CHANGE at the next sign-in and a session opened before it is
	// refused on its next request - which is the whole of the enforcement, with no job and no
	// write into anybody's row (ADR-0068 §3).
	RotationFrom time.Time
}

// DefaultSignInPolicy is what an installation that has decided nothing enforces.
//
// Twelve characters, no composition rule, the two offline lists on, no expiry, no history, no
// second factor demanded, both methods, and the refresh token's own horizon. Every one of those
// is the value NIST advises; each switch above it carries the sentence that says what turning it
// on costs, including a line added to every password screen.
func DefaultSignInPolicy() SignInPolicy {
	return SignInPolicy{
		Password: PasswordPolicy{
			MinLength:       MinPasswordLength,
			CommonPasswords: true,
			ContextWords:    true,
		},
		MfaRequiredFor: MfaForNobody,
		Methods:        []string{MethodPassword, MethodOidc},
		Sessions:       SessionPolicy{MaxDays: CeilingSessionMaxDays},
	}
}

// PolicySwitch names one switch, in the spelling the contract and the audit trail use. A field
// error names it, a lock is held against it, and a patch addresses it - three readers of one
// name, which is why it is a type and not a literal in three places.
type PolicySwitch string

// The switches, in the three groups of ADR-0068 §1.
const (
	SwitchMinLength       PolicySwitch = "min_length"
	SwitchMinLowercase    PolicySwitch = "min_lowercase"
	SwitchMinUppercase    PolicySwitch = "min_uppercase"
	SwitchMinDigits       PolicySwitch = "min_digits"
	SwitchMinSymbols      PolicySwitch = "min_symbols"
	SwitchMinClasses      PolicySwitch = "min_classes"
	SwitchMaxRepeat       PolicySwitch = "max_repeat"
	SwitchCommonPasswords PolicySwitch = "common_passwords"
	SwitchContextWords    PolicySwitch = "context_words"
	SwitchBreachCheck     PolicySwitch = "breach_check"
	SwitchMaxAgeDays      PolicySwitch = "max_age_days"
	SwitchHistoryCount    PolicySwitch = "history_count"
	SwitchMinAgeHours     PolicySwitch = "min_age_hours"

	SwitchMfaRequiredFor PolicySwitch = "mfa_required_for"
	SwitchMethods        PolicySwitch = "methods"

	SwitchSessionMaxDays     PolicySwitch = "session_max_days"
	SwitchSessionIdleMinutes PolicySwitch = "session_idle_minutes"

	SwitchRotationFrom PolicySwitch = "rotation_from"
)

// PolicySwitches is every switch, in the order a screen draws them. The settings screen, the
// patch and the tests read this rather than each keeping a list.
func PolicySwitches() []PolicySwitch {
	return []PolicySwitch{
		SwitchMinLength, SwitchMinLowercase, SwitchMinUppercase, SwitchMinDigits,
		SwitchMinSymbols, SwitchMinClasses, SwitchMaxRepeat, SwitchCommonPasswords,
		SwitchContextWords, SwitchBreachCheck, SwitchMaxAgeDays, SwitchHistoryCount,
		SwitchMinAgeHours,
		SwitchMfaRequiredFor, SwitchMethods,
		SwitchSessionMaxDays, SwitchSessionIdleMinutes,
		SwitchRotationFrom,
	}
}

// LockOrigin is where a lock came from. A screen says which, because "ask your administrator"
// and "ask your provider" are different sentences and a reader told the wrong one writes the
// wrong mail (ADR-0070 §3).
type LockOrigin string

const (
	// LockNone is an open switch: the workspace may tighten it.
	LockNone LockOrigin = ""
	// LockInstance is the operator's.
	LockInstance LockOrigin = "INSTANCE"
	// LockPlan is the plan's. Nothing writes it yet - plans are their own milestone - and the
	// origin exists from the first row so that the plan layer is not a migration through the
	// sign-in path later.
	LockPlan LockOrigin = "PLAN"
)

// PolicyPatch is one level's say: a nil pointer is a switch that level did not decide.
//
// Typed pointers rather than a map of values, because a switch's kind is part of the rule - a
// string where a number belongs is a refusal at the edge of the system, not a surprise three
// layers in.
type PolicyPatch struct {
	MinLength       *int
	MinLowercase    *int
	MinUppercase    *int
	MinDigits       *int
	MinSymbols      *int
	MinClasses      *int
	MaxRepeat       *int
	CommonPasswords *bool
	ContextWords    *bool
	BreachCheck     *bool
	MaxAgeDays      *int
	HistoryCount    *int
	MinAgeHours     *int

	MfaRequiredFor *MfaRequirement
	Methods        *[]string

	SessionMaxDays     *int
	SessionIdleMinutes *int

	RotationFrom *time.Time
}

// IsEmpty reports whether this level decided nothing at all.
func (p PolicyPatch) IsEmpty() bool { return len(p.Decided()) == 0 }

// Decided answers the switches this patch carries, in PolicySwitches' order.
func (p PolicyPatch) Decided() []PolicySwitch {
	decided := make([]PolicySwitch, 0, len(PolicySwitches()))
	for _, name := range PolicySwitches() {
		if p.carries(name) {
			decided = append(decided, name)
		}
	}
	return decided
}

func (p PolicyPatch) carries(name PolicySwitch) bool {
	switch name {
	case SwitchMinLength:
		return p.MinLength != nil
	case SwitchMinLowercase:
		return p.MinLowercase != nil
	case SwitchMinUppercase:
		return p.MinUppercase != nil
	case SwitchMinDigits:
		return p.MinDigits != nil
	case SwitchMinSymbols:
		return p.MinSymbols != nil
	case SwitchMinClasses:
		return p.MinClasses != nil
	case SwitchMaxRepeat:
		return p.MaxRepeat != nil
	case SwitchCommonPasswords:
		return p.CommonPasswords != nil
	case SwitchContextWords:
		return p.ContextWords != nil
	case SwitchBreachCheck:
		return p.BreachCheck != nil
	case SwitchMaxAgeDays:
		return p.MaxAgeDays != nil
	case SwitchHistoryCount:
		return p.HistoryCount != nil
	case SwitchMinAgeHours:
		return p.MinAgeHours != nil
	case SwitchMfaRequiredFor:
		return p.MfaRequiredFor != nil
	case SwitchMethods:
		return p.Methods != nil
	case SwitchSessionMaxDays:
		return p.SessionMaxDays != nil
	case SwitchSessionIdleMinutes:
		return p.SessionIdleMinutes != nil
	case SwitchRotationFrom:
		return p.RotationFrom != nil
	}
	return false
}

// Merge answers this patch with another's decisions written over it. What the other did not decide
// is left standing, which is what makes a save of three switches a save of three switches.
func (p PolicyPatch) Merge(other PolicyPatch) PolicyPatch {
	merged := p
	for _, name := range other.Decided() {
		switch name {
		case SwitchMinLength:
			merged.MinLength = other.MinLength
		case SwitchMinLowercase:
			merged.MinLowercase = other.MinLowercase
		case SwitchMinUppercase:
			merged.MinUppercase = other.MinUppercase
		case SwitchMinDigits:
			merged.MinDigits = other.MinDigits
		case SwitchMinSymbols:
			merged.MinSymbols = other.MinSymbols
		case SwitchMinClasses:
			merged.MinClasses = other.MinClasses
		case SwitchMaxRepeat:
			merged.MaxRepeat = other.MaxRepeat
		case SwitchCommonPasswords:
			merged.CommonPasswords = other.CommonPasswords
		case SwitchContextWords:
			merged.ContextWords = other.ContextWords
		case SwitchBreachCheck:
			merged.BreachCheck = other.BreachCheck
		case SwitchMaxAgeDays:
			merged.MaxAgeDays = other.MaxAgeDays
		case SwitchHistoryCount:
			merged.HistoryCount = other.HistoryCount
		case SwitchMinAgeHours:
			merged.MinAgeHours = other.MinAgeHours
		case SwitchMfaRequiredFor:
			merged.MfaRequiredFor = other.MfaRequiredFor
		case SwitchMethods:
			merged.Methods = other.Methods
		case SwitchSessionMaxDays:
			merged.SessionMaxDays = other.SessionMaxDays
		case SwitchSessionIdleMinutes:
			merged.SessionIdleMinutes = other.SessionIdleMinutes
		case SwitchRotationFrom:
			merged.RotationFrom = other.RotationFrom
		}
	}
	return merged
}

// PolicyLayer is a level of the resolution: what it decided, and what it forbids the level below
// to touch. Locks are only ever read from the instance's and the plan's layer - a workspace has
// nobody below it to lock.
type PolicyLayer struct {
	Patch PolicyPatch
	// Locks is the switches this level fixed. A switch locked without a value in the same patch
	// locks whatever the level above it decided, which is how an operator pins the product's
	// default without restating it.
	Locks map[PolicySwitch]bool
}

// EffectivePolicy is the resolved rule plus who fixed what. The locks travel with the value
// because every reader of one needs the other: the screen to draw the reason, the writer to
// refuse the change, and the audit entry to say which level the refusal came from.
type EffectivePolicy struct {
	Policy SignInPolicy
	Locks  map[PolicySwitch]LockOrigin
}

// LockOf answers where a switch is locked, or LockNone.
func (e EffectivePolicy) LockOf(name PolicySwitch) LockOrigin { return e.Locks[name] }

// IsLocked is LockOf as the question a writer asks.
func (e EffectivePolicy) IsLocked(name PolicySwitch) bool { return e.Locks[name] != LockNone }

// Effective resolves the three levels above the product's default into one rule.
//
// The plan's layer is a parameter with no writer yet, deliberately (ADR-0070 §3): a resolver that
// grew the parameter later would grow it through the sign-in path, the setting screen and the
// check route at once.
//
// A level may only tighten what the level above decided, and a locked switch is not the level
// below's to touch at all. Both are enforced here rather than at the edge, because the edge is
// three routes and this is one place.
func Effective(instance, plan, workspace PolicyLayer) EffectivePolicy {
	resolved := DefaultSignInPolicy()
	locks := map[PolicySwitch]LockOrigin{}

	for _, level := range []struct {
		layer  PolicyLayer
		origin LockOrigin
	}{{instance, LockInstance}, {plan, LockPlan}, {workspace, LockNone}} {
		for _, name := range level.layer.Patch.Decided() {
			if locks[name] != LockNone && level.origin == LockNone {
				// A locked switch: the workspace's value is ignored rather than refused here.
				// Refusing is the writer's job, where there is a field to name; resolving has to
				// answer something for a row that was written before the lock landed.
				continue
			}
			if level.origin == LockNone && !tightens(name, resolved, level.layer.Patch) {
				// The workspace's row is *older* than the level above it, and the level above has
				// since moved past it. Ignoring it here is what makes "a workspace only ever
				// tightens" true over time rather than only at the moment of the write: an
				// operator who raises the minimum to sixteen has raised it for the workspace that
				// stored fourteen last year too.
				continue
			}
			resolved = applySwitch(resolved, name, level.layer.Patch)
		}
		if level.origin == LockNone {
			continue
		}
		for name, locked := range level.layer.Locks {
			if locked {
				locks[name] = level.origin
			}
		}
	}

	return EffectivePolicy{Policy: bounded(resolved), Locks: locks}
}

// applySwitch writes one switch of a patch onto the policy. Every switch is here once, which is
// what makes the product bounds below the only other place a value is touched.
func applySwitch(policy SignInPolicy, name PolicySwitch, patch PolicyPatch) SignInPolicy {
	switch name {
	case SwitchMinLength:
		policy.Password.MinLength = *patch.MinLength
	case SwitchMinLowercase:
		policy.Password.MinLowercase = *patch.MinLowercase
	case SwitchMinUppercase:
		policy.Password.MinUppercase = *patch.MinUppercase
	case SwitchMinDigits:
		policy.Password.MinDigits = *patch.MinDigits
	case SwitchMinSymbols:
		policy.Password.MinSymbols = *patch.MinSymbols
	case SwitchMinClasses:
		policy.Password.MinClasses = *patch.MinClasses
	case SwitchMaxRepeat:
		policy.Password.MaxRepeat = *patch.MaxRepeat
	case SwitchCommonPasswords:
		policy.Password.CommonPasswords = *patch.CommonPasswords
	case SwitchContextWords:
		policy.Password.ContextWords = *patch.ContextWords
	case SwitchBreachCheck:
		policy.Password.BreachCheck = *patch.BreachCheck
	case SwitchMaxAgeDays:
		policy.Password.MaxAgeDays = *patch.MaxAgeDays
	case SwitchHistoryCount:
		policy.Password.HistoryCount = *patch.HistoryCount
	case SwitchMinAgeHours:
		policy.Password.MinAgeHours = *patch.MinAgeHours
	case SwitchMfaRequiredFor:
		policy.MfaRequiredFor = *patch.MfaRequiredFor
	case SwitchMethods:
		policy.Methods = append([]string(nil), *patch.Methods...)
	case SwitchSessionMaxDays:
		policy.Sessions.MaxDays = *patch.SessionMaxDays
	case SwitchSessionIdleMinutes:
		policy.Sessions.IdleMinutes = *patch.SessionIdleMinutes
	case SwitchRotationFrom:
		policy.RotationFrom = patch.RotationFrom.UTC()
	}
	return policy
}

// bounded brings a resolved policy inside the product's own bounds. The operator is included:
// a value below the floor is not a relaxation the product offers, and a value above a ceiling is
// a rule nobody could satisfy.
func bounded(policy SignInPolicy) SignInPolicy {
	policy.Password.MinLength = clamp(policy.Password.MinLength, FloorMinLength, CeilingMinLength)
	policy.Password.MinLowercase = atLeast(policy.Password.MinLowercase, 0)
	policy.Password.MinUppercase = atLeast(policy.Password.MinUppercase, 0)
	policy.Password.MinDigits = atLeast(policy.Password.MinDigits, 0)
	policy.Password.MinSymbols = atLeast(policy.Password.MinSymbols, 0)
	policy.Password.MinClasses = clamp(policy.Password.MinClasses, 0, 4)
	policy.Password.MaxRepeat = atLeast(policy.Password.MaxRepeat, 0)
	policy.Password.HistoryCount = clamp(policy.Password.HistoryCount, 0, CeilingHistoryCount)
	policy.Password.MinAgeHours = clamp(policy.Password.MinAgeHours, 0, CeilingMinAgeHours)
	if policy.Password.MaxAgeDays > 0 {
		policy.Password.MaxAgeDays = atLeast(policy.Password.MaxAgeDays, FloorMaxAgeDays)
	} else {
		policy.Password.MaxAgeDays = 0
	}
	if !policy.MfaRequiredFor.Valid() {
		policy.MfaRequiredFor = MfaForNobody
	}
	policy.Methods = boundedMethods(policy.Methods)
	policy.Sessions.MaxDays = clamp(policy.Sessions.MaxDays, 1, CeilingSessionMaxDays)
	policy.Sessions.IdleMinutes = atLeast(policy.Sessions.IdleMinutes, 0)
	return policy
}

// boundedMethods keeps the list to the ways in that exist, in the contract's order, without
// duplicates - and never empty, because a workspace nobody can sign in to is a support call and
// not a policy.
func boundedMethods(methods []string) []string {
	kept := make([]string, 0, 2)
	for _, method := range []string{MethodPassword, MethodOidc} {
		for _, offered := range methods {
			if offered == method {
				kept = append(kept, method)
				break
			}
		}
	}
	if len(kept) == 0 {
		return []string{MethodPassword, MethodOidc}
	}
	return kept
}

// Tighten decides whether a proposed value for one switch is a tightening of the one in force.
//
// Direction is per switch and is not obvious from the type: more characters is stricter, fewer
// days until expiry is stricter, and turning a list on is stricter. The one table is here so that
// the settings screen, the writer and the tests read the same answer.
func Tighten(name PolicySwitch, inForce SignInPolicy, proposed PolicyPatch) error {
	if !proposed.carries(name) {
		return nil
	}
	if tightens(name, inForce, proposed) {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("auth.policy_loosens").
		WithParams(map[string]string{"switch": string(name)}).
		WithFields(shared.FieldError{
			Path: "/sign_in_policy/" + string(name),
			Code: "auth.policy_loosens",
		})
}

//nolint:gocyclo,cyclop // One switch per case is the table; splitting it hides the direction.
func tightens(name PolicySwitch, inForce SignInPolicy, proposed PolicyPatch) bool {
	password, sessions := inForce.Password, inForce.Sessions
	switch name {
	case SwitchMinLength:
		return *proposed.MinLength >= password.MinLength
	case SwitchMinLowercase:
		return *proposed.MinLowercase >= password.MinLowercase
	case SwitchMinUppercase:
		return *proposed.MinUppercase >= password.MinUppercase
	case SwitchMinDigits:
		return *proposed.MinDigits >= password.MinDigits
	case SwitchMinSymbols:
		return *proposed.MinSymbols >= password.MinSymbols
	case SwitchMinClasses:
		return *proposed.MinClasses >= password.MinClasses
	case SwitchMaxRepeat:
		// Off is the loosest; a shorter run allowed is stricter than a longer one.
		return offOrLower(*proposed.MaxRepeat, password.MaxRepeat)
	case SwitchCommonPasswords:
		return *proposed.CommonPasswords || !password.CommonPasswords
	case SwitchContextWords:
		return *proposed.ContextWords || !password.ContextWords
	case SwitchBreachCheck:
		return *proposed.BreachCheck || !password.BreachCheck
	case SwitchMaxAgeDays:
		return offOrLower(*proposed.MaxAgeDays, password.MaxAgeDays)
	case SwitchHistoryCount:
		return *proposed.HistoryCount >= password.HistoryCount
	case SwitchMinAgeHours:
		return *proposed.MinAgeHours >= password.MinAgeHours
	case SwitchMfaRequiredFor:
		return proposed.MfaRequiredFor.rank() >= inForce.MfaRequiredFor.rank()
	case SwitchMethods:
		// Fewer ways in is stricter: a subset of what is in force, never a method it removed.
		return isSubset(*proposed.Methods, inForce.Methods)
	case SwitchSessionMaxDays:
		return *proposed.SessionMaxDays <= sessions.MaxDays && *proposed.SessionMaxDays >= 1
	case SwitchSessionIdleMinutes:
		return offOrLower(*proposed.SessionIdleMinutes, sessions.IdleMinutes)
	case SwitchRotationFrom:
		// A moment only ever moves forward: rewinding it would let a workspace un-require a
		// rotation it already asked for, and the sessions it ended are already gone.
		return !proposed.RotationFrom.Before(inForce.RotationFrom)
	}
	return false
}

// offOrLower is the direction of every switch where zero means off: off is the loosest value,
// and among the values that are on, the smaller one is stricter.
func offOrLower(proposed, inForce int) bool {
	if inForce == 0 {
		return true
	}
	return proposed > 0 && proposed <= inForce
}

func isSubset(proposed, inForce []string) bool {
	if len(inForce) == 0 {
		return true
	}
	for _, method := range proposed {
		found := false
		for _, offered := range inForce {
			if offered == method {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return len(proposed) > 0
}

// Tightened applies a workspace's patch onto the rule in force, refusing a switch the level
// above locked and a switch the patch loosens - by field, so that a screen with eighteen rows
// marks the one that was wrong.
//
// It answers the fields that moved for the audit entry, in a stable order, WorkspaceChange's
// discipline: a value set to what it already is is not a change.
func (e EffectivePolicy) Tightened(patch PolicyPatch) (SignInPolicy, []FieldChange, error) {
	moved := map[string]FieldChange{}
	result := e.Policy

	for _, name := range patch.Decided() {
		if origin := e.LockOf(name); origin != LockNone {
			return SignInPolicy{}, nil, shared.ErrValidation.
				WithDetail("auth.policy_locked").
				WithParams(map[string]string{"switch": string(name), "origin": string(origin)}).
				WithFields(shared.FieldError{
					Path:   "/sign_in_policy/" + string(name),
					Code:   "auth.policy_locked",
					Params: map[string]string{"origin": string(origin)},
				})
		}
		if err := Tighten(name, e.Policy, patch); err != nil {
			return SignInPolicy{}, nil, err
		}
		before := SwitchText(result, name)
		result = applySwitch(result, name, patch)
		if after := SwitchText(result, name); after != before {
			moved[string(name)] = FieldChange{Field: string(name), From: before, To: after}
		}
	}

	return bounded(result), sortedChanges(moved), nil
}

// SwitchText is a switch's value as a trail spells it: a field name and a value, never a sentence
// (ADR-0011). Exported because the instance layer's journal names what moved and has to compare two
// levels to find out, and a second spelling of a switch's value would be a second thing to get wrong.
func SwitchText(policy SignInPolicy, name PolicySwitch) string {
	switch name {
	case SwitchMinLength:
		return itoa(policy.Password.MinLength)
	case SwitchMinLowercase:
		return itoa(policy.Password.MinLowercase)
	case SwitchMinUppercase:
		return itoa(policy.Password.MinUppercase)
	case SwitchMinDigits:
		return itoa(policy.Password.MinDigits)
	case SwitchMinSymbols:
		return itoa(policy.Password.MinSymbols)
	case SwitchMinClasses:
		return itoa(policy.Password.MinClasses)
	case SwitchMaxRepeat:
		return itoa(policy.Password.MaxRepeat)
	case SwitchCommonPasswords:
		return boolText(policy.Password.CommonPasswords)
	case SwitchContextWords:
		return boolText(policy.Password.ContextWords)
	case SwitchBreachCheck:
		return boolText(policy.Password.BreachCheck)
	case SwitchMaxAgeDays:
		return itoa(policy.Password.MaxAgeDays)
	case SwitchHistoryCount:
		return itoa(policy.Password.HistoryCount)
	case SwitchMinAgeHours:
		return itoa(policy.Password.MinAgeHours)
	case SwitchMfaRequiredFor:
		return string(policy.MfaRequiredFor)
	case SwitchMethods:
		methods := append([]string(nil), policy.Methods...)
		sort.Strings(methods)
		return joinComma(methods)
	case SwitchSessionMaxDays:
		return itoa(policy.Sessions.MaxDays)
	case SwitchSessionIdleMinutes:
		return itoa(policy.Sessions.IdleMinutes)
	case SwitchRotationFrom:
		if policy.RotationFrom.IsZero() {
			return ""
		}
		return policy.RotationFrom.UTC().Format(time.RFC3339)
	}
	return ""
}

func joinComma(values []string) string {
	joined := ""
	for index, value := range values {
		if index > 0 {
			joined += ","
		}
		joined += value
	}
	return joined
}

func clamp(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func atLeast(value, low int) int {
	if value < low {
		return low
	}
	return value
}
