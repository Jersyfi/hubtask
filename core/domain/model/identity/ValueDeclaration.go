// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

// What kind of value each key of the installation's level takes (UC-INS-09 checks 1 and 3).
//
// Declared here, beside the switch it is about, rather than guessed by whoever draws or reads it:
// a screen that guessed a key's kind from its current value had nothing to guess from while the
// key was undecided, and sent a number as text. The declaration is what the control plane answers
// for every key, decided or not, so a client keeps no list of its own and a key added later needs
// no new control.

// ValueKind is the shape of a value.
type ValueKind string

const (
	// KindCount is a whole number, never negative.
	KindCount ValueKind = "COUNT"
	// KindFlag is true or false.
	KindFlag ValueKind = "FLAG"
	// KindChoice is one value out of a closed set.
	KindChoice ValueKind = "CHOICE"
	// KindChoices is a list drawn from a closed set.
	KindChoices ValueKind = "CHOICES"
	// KindLink is an https address.
	KindLink ValueKind = "LINK"
	// KindLocale is a BCP 47 tag.
	KindLocale ValueKind = "LOCALE"
	// KindTimeZone is an IANA zone.
	KindTimeZone ValueKind = "TIME_ZONE"
)

// WorkspaceMay is what an open key leaves a workspace free to do, which is what the words beside a
// lock have to say: a password rule is made stricter, a limit is raised, a link is replaced.
type WorkspaceMay string

const (
	// MayTighten: a workspace may choose a stricter value.
	MayTighten WorkspaceMay = "TIGHTEN"
	// MayRaise: a workspace may set a higher limit of its own.
	MayRaise WorkspaceMay = "RAISE"
	// MayReplace: a workspace may set its own value instead.
	MayReplace WorkspaceMay = "REPLACE"
	// MayNeverLock is a default and nothing else: there is no lock to describe.
	MayNeverLock WorkspaceMay = ""
)

// ValueDeclaration is one key's kind, its closed set where it has one, a count's bounds, and what
// a workspace may do while the key is open.
type ValueDeclaration struct {
	Kind ValueKind
	// Options is the closed set of a choice, from the loosest value to the strictest.
	Options []string
	// Minimum and Maximum are a count's bounds as the product applies them; nil is unbounded.
	Minimum *int
	Maximum *int
	May     WorkspaceMay
}

func bounds(minimum int, maximum *int) (*int, *int) { return &minimum, maximum }

func ceiling(value int) *int { return &value }

func count(minimum int, maximum *int) ValueDeclaration {
	low, high := bounds(minimum, maximum)
	return ValueDeclaration{Kind: KindCount, Minimum: low, Maximum: high, May: MayTighten}
}

// DeclarationOfSwitch answers a sign-in switch's declaration, and false for one the installation
// does not hold at all (`rotation_from`, an event a workspace raises for its own people).
//
// The bounds are the ones `bounded` applies; a value past them is held at them on read, at every
// level, the operator's included.
//
//nolint:gocyclo,cyclop // one case per switch is the declaration
func DeclarationOfSwitch(name PolicySwitch) (ValueDeclaration, bool) {
	switch name {
	case SwitchMinLength:
		return count(FloorMinLength, ceiling(CeilingMinLength)), true
	case SwitchMinLowercase, SwitchMinUppercase, SwitchMinDigits, SwitchMinSymbols,
		SwitchMaxRepeat, SwitchSessionIdleMinutes, SwitchMaxAgeDays:
		return count(0, nil), true
	case SwitchMinClasses:
		return count(0, ceiling(4)), true
	case SwitchHistoryCount:
		return count(0, ceiling(CeilingHistoryCount)), true
	case SwitchMinAgeHours:
		return count(0, ceiling(CeilingMinAgeHours)), true
	case SwitchSessionMaxDays:
		return count(1, ceiling(CeilingSessionMaxDays)), true
	case SwitchCommonPasswords, SwitchContextWords, SwitchBreachCheck, SwitchManagedAccounts:
		return ValueDeclaration{Kind: KindFlag, May: MayTighten}, true
	case SwitchMfaRequiredFor:
		return ValueDeclaration{
			Kind: KindChoice, May: MayTighten,
			Options: []string{string(MfaForNobody), string(MfaForAdmins), string(MfaForEveryone)},
		}, true
	case SwitchMethods:
		return ValueDeclaration{
			Kind: KindChoices, May: MayTighten, Options: []string{MethodDirect, MethodOidc},
		}, true
	}
	return ValueDeclaration{}, false
}

// DeclarationOfLink is every legal link's: an address a workspace may replace with its own.
func DeclarationOfLink(LegalLink) ValueDeclaration {
	return ValueDeclaration{Kind: KindLink, May: MayReplace}
}

// The three localisation keys, in the spelling the contract uses.
const (
	LocalisationLocale    = "locale"
	LocalisationTimeZone  = "time_zone"
	LocalisationWeekStart = "week_start"
)

// LocalisationKeys is the three, in the order a screen draws them.
func LocalisationKeys() []string {
	return []string{LocalisationLocale, LocalisationTimeZone, LocalisationWeekStart}
}

// DeclarationOfLocalisation answers a localisation key's declaration: a default, never a lock.
func DeclarationOfLocalisation(name string) (ValueDeclaration, bool) {
	switch name {
	case LocalisationLocale:
		return ValueDeclaration{Kind: KindLocale, May: MayNeverLock}, true
	case LocalisationTimeZone:
		return ValueDeclaration{Kind: KindTimeZone, May: MayNeverLock}, true
	case LocalisationWeekStart:
		return ValueDeclaration{Kind: KindChoice, May: MayNeverLock, Options: WeekStartDays()}, true
	}
	return ValueDeclaration{}, false
}
