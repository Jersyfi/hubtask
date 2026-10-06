// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import "strings"

// The two levels of instance value that are not switches (SI-17, the concept's §5.3 and §6.4).
//
// Both are "the installation decides a default and a workspace may differ", which is the model §5.3
// draws for every instance-wide setting. What separates them is whether a lock is even meaningful,
// and that difference is in the shape rather than in a rule somebody has to remember.

// LocalisationDefaults is the language, time zone and week start a workspace inherits.
//
// **There is no lock, and there is deliberately no field for one.** The concept's §5.7: "Die
// Sprache und das Aussehen eines Arbeitsbereichs. Eine Instanz gibt einen Standard, nie ein
// Schloss: ein Unternehmen, das nicht auf Deutsch arbeiten darf, weil der Betreiber es so
// eingestellt hat, ist ein Produktfehler." A type with nowhere to put a lock cannot grow one by
// accident.
//
// An empty string is "the installation decides nothing here", and the workspace falls through to
// the product's own default — not to an empty language, which is not a language.
type LocalisationDefaults struct {
	Locale   string
	TimeZone string
	// WeekStart is the ISO weekday a calendar opens on, 1 being Monday. Zero is undecided.
	WeekStart int
}

// IsZero reports whether the installation decided none of the three.
func (d LocalisationDefaults) IsZero() bool {
	return strings.TrimSpace(d.Locale) == "" && strings.TrimSpace(d.TimeZone) == "" && d.WeekStart == 0
}

// MaxWeekStart is Sunday in the ISO numbering the rest of this product uses.
const MaxWeekStart = 7

// QuotaDefaults is the ceiling the installation sets for each quota, by the quota's own name.
//
// A map rather than a struct with seven fields, and that is the one place this package bends: the
// vocabulary belongs to `core/application/service/quota`, which the domain may not import, and a
// second copy of seven names here would be a second copy to keep in step. What the domain does hold
// is the rule — a ceiling is a whole number that is not negative — and the lock beside each.
//
// Unlike localisation, a lock **is** meaningful: an operator running a platform may set a ceiling
// and forbid a workspace raising it, which is exactly what §6.4's "Tarif, Ausnahme je Bereich" will
// mean once there are plans.
type QuotaDefaults struct {
	Limits map[string]int64
	Locks  map[string]bool
}

// Of answers one ceiling and whether the installation set it.
func (d QuotaDefaults) Of(name string) (int64, bool) {
	limit, held := d.Limits[name]
	return limit, held
}

// Locked reports whether a workspace may set its own.
func (d QuotaDefaults) Locked(name string) bool { return d.Locks[name] }

// IsZero reports whether the installation set no ceiling at all.
func (d QuotaDefaults) IsZero() bool { return len(d.Limits) == 0 }
