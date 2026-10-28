// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package setting is the one settings catalogue (domain-model.md §2, "The settings catalogue", principle P-17): what a
// setting is - its key, its kind, its bounds, the levels that may decide it and how - and the one
// resolver that answers, for every setting at every place, `{effective, source, locked_by,
// allowed}`.
//
// Every setting of every level is declared here once, in the same shape, beside the switch it is
// about: the installation's areas, a plan's, a workspace's, a hub's and a collection's structure,
// a person's own, and the module switches. A screen, the configuration file, `hubctl`, REST and MCP
// all read the same declaration, so none of them keeps a list of its own.
package setting

import (
	"slices"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Key is a setting's stable name: the area and the switch, dotted - `sign_in.min_length`,
// `structure.views`, `module.planning`.
type Key string

// Level is one place a setting can be decided at, top-down. The spellings are the contract's
// source words (identity.PolicySource), so a resolution's source is answered as it always was.
type Level string

const (
	// LevelDefault is Hubtask's own value, shipped with the release.
	LevelDefault Level = "DEFAULT"
	// LevelInstance is the operator's installation.
	LevelInstance Level = "INSTANCE"
	// LevelPlan is the plan a workspace is on (multi-tenancy.md §4.2).
	LevelPlan Level = "PLAN"
	// LevelWorkspace is the workspace's administrators'.
	LevelWorkspace Level = "WORKSPACE"
	// LevelHub is whoever shapes the hub.
	LevelHub Level = "HUB"
	// LevelCollection is whoever shapes the collection.
	LevelCollection Level = "COLLECTION"
	// LevelPerson is the person's own.
	LevelPerson Level = "PERSON"
)

var levels = [...]Level{
	LevelDefault, LevelInstance, LevelPlan, LevelWorkspace, LevelHub, LevelCollection, LevelPerson,
}

// Levels is every level, top-down.
func Levels() []Level { return levels[:] }

// Valid reports whether the level is one of the seven.
func (l Level) Valid() bool { return slices.Contains(levels[:], l) }

// Below reports whether this level sits below the other one.
func (l Level) Below(other Level) bool {
	return slices.Index(levels[:], l) > slices.Index(levels[:], other)
}

// Kind is the shape of a setting's value. The first eight are the installation's (identity.ValueKind
// spells them the same); STRUCTURE is a value whose shape its own package owns - a collection's
// levels, its views - and which this package only carries.
type Kind string

const (
	KindCount     Kind = "COUNT"
	KindFlag      Kind = "FLAG"
	KindChoice    Kind = "CHOICE"
	KindChoices   Kind = "CHOICES"
	KindLink      Kind = "LINK"
	KindText      Kind = "TEXT"
	KindLocale    Kind = "LOCALE"
	KindTimeZone  Kind = "TIME_ZONE"
	KindStructure Kind = "STRUCTURE"
)

// Hold is how a level holds a value it decides (configuration model §6): *set* is a default the
// levels below may change, *lock* fixes it for every level below, *restrict* leaves the levels
// below a choice only inside a range or a subset.
type Hold string

const (
	HoldSet      Hold = "SET"
	HoldLock     Hold = "LOCK"
	HoldRestrict Hold = "RESTRICT"
)

// Valid reports whether the hold is one of the three.
func (h Hold) Valid() bool { return h == HoldSet || h == HoldLock || h == HoldRestrict }

// May is what an open setting leaves the level below free to do: what the words beside a lock
// say. The spellings are identity.WorkspaceMay's.
type May string

const (
	// MayTighten: a lower level may only choose a stricter value (Definition.Tightens decides).
	MayTighten May = "TIGHTEN"
	// MayRaise: a lower level may set a higher limit of its own.
	MayRaise May = "RAISE"
	// MayReplace: a lower level may set any value of its own.
	MayReplace May = "REPLACE"
	// MayPlanReplace: open, only a plan may set its own.
	MayPlanReplace May = "PLAN_REPLACE"
	// MayNeverLock is a default and nothing else.
	MayNeverLock May = ""
)

// Allowed is what a level leaves the levels below: a count's range, or a subset of a choice's
// options. The zero value leaves everything the declaration allows.
type Allowed struct {
	Minimum *int
	Maximum *int
	// Options is the subset left, nil for every option.
	Options []string
}

// IsOpen reports whether nothing is narrowed.
func (a Allowed) IsOpen() bool { return a.Minimum == nil && a.Maximum == nil && a.Options == nil }

// Definition is one entry of the catalogue.
type Definition struct {
	Key  Key
	Kind Kind
	// Options is a choice's closed set, from the loosest to the strictest.
	Options []string
	// Minimum and Maximum are a count's bounds as the product applies them; nil is unbounded.
	Minimum *int
	Maximum *int
	// Levels is where the setting may be decided, top-down; the product's default is implied.
	Levels []Level
	// Lockable says whether a level may lock it; Restrictable whether it may restrict it.
	Lockable     bool
	Restrictable bool
	May          May
	// Writers names, per level, the permission (or the operator's scope) a writer needs there.
	Writers map[Level]string
	// Preview names the use case that answers, before a save, what a change reaches; empty where
	// a change reaches nothing beyond the value itself.
	Preview string
	// Default is Hubtask's own value; nil where the value's own package resolves the default.
	Default any
	// Tightens decides, for MayTighten, whether a proposed value is at least as strict as the one
	// in force. Nil accepts every value.
	Tightens func(inForce, proposed any) bool
	// TightenFrom is the highest level held to Tightens; empty holds every level below the first.
	// A module is switched freely by the workspace and only narrowed by a hub.
	TightenFrom Level
}

// heldToTighten reports whether a level's value must tighten the one in force.
func (d Definition) heldToTighten(level Level) bool {
	if d.May != MayTighten || d.Tightens == nil || len(d.Levels) == 0 || level == d.Levels[0] {
		return false
	}
	return d.TightenFrom == "" || level == d.TightenFrom || level.Below(d.TightenFrom)
}

// DecidedAt reports whether a level may decide the setting.
func (d Definition) DecidedAt(level Level) bool { return slices.Contains(d.Levels, level) }

// Validate refuses a declaration that could not be resolved: an unknown kind or level, levels out
// of order, a lock declared where only defaults are, a count with its bounds crossed.
func (d Definition) Validate() error {
	invalid := func(reason string) error {
		return shared.ErrInternal.WithDetail("setting.definition_invalid").
			WithParams(map[string]string{"setting": string(d.Key), "reason": reason})
	}
	switch {
	case d.Key == "":
		return invalid("key")
	case !slices.Contains(kinds[:], d.Kind):
		return invalid("kind")
	case len(d.Levels) == 0:
		return invalid("levels")
	case d.Minimum != nil && d.Maximum != nil && *d.Minimum > *d.Maximum:
		return invalid("bounds")
	case d.May == MayNeverLock && (d.Lockable || d.Restrictable):
		return invalid("lock")
	}
	for index, level := range d.Levels {
		if !level.Valid() || level == LevelDefault {
			return invalid("level")
		}
		if index > 0 && !level.Below(d.Levels[index-1]) {
			return invalid("order")
		}
	}
	return nil
}

var kinds = [...]Kind{
	KindCount, KindFlag, KindChoice, KindChoices, KindLink, KindText, KindLocale, KindTimeZone,
	KindStructure,
}

// Entry is what one level stores about one setting. A nil Value with HoldLock locks whatever the
// levels above decided, which is how a level pins a value without restating it.
type Entry struct {
	Value   any
	Hold    Hold
	Allowed Allowed
	// By is who set the entry, where the store knows it.
	By shared.ID
}

// Layer is one level's say: the entries it stores, by key.
type Layer struct {
	Level   Level
	Entries map[Key]Entry
}

// Resolution is the one answer every reader of a setting gets.
type Resolution struct {
	Key       Key
	Effective any
	// Source is the level whose value is in force; LevelDefault where no level decided it.
	Source Level
	// LockedBy is the level that locked it, empty where it is open; LockedByPerson who, where
	// known.
	LockedBy       Level
	LockedByPerson shared.ID
	// Allowed is what is left to the next level below.
	Allowed Allowed
}

// Locked reports whether a level above fixed the value.
func (r Resolution) Locked() bool { return r.LockedBy != "" }
