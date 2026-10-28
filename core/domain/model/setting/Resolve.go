// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import "slices"

// Resolve answers one setting at one place from the layers above it, top-down (configuration
// model §5): the nearest level that decides wins; a lock binds every level below and keeps the
// origin of the level that placed it first; a restriction narrows what the levels below may
// choose; a lower value under a lock, outside a restriction or - for MayTighten - looser than the
// value in force is ignored rather than refused. Refusing is the writer's job, where there is a
// field to name; resolving has to answer something for a row stored before the level above moved.
//
// The definition's first level is the top one and is held by nothing but the product's bounds:
// the operator decides the installation's value freely, every level below is held by it.
//
// The value is carried as stored: a kind's own bounds - a count's floor, a list's order - are
// applied by the package that owns the setting, after resolving, because some of them are not
// bounds a range can say (a password's maximum age is either off or at least a week).
func Resolve(definition Definition, layers ...Layer) Resolution {
	resolution := Resolution{Key: definition.Key, Effective: definition.Default, Source: LevelDefault}
	for _, layer := range layers {
		entry, held := layer.Entries[definition.Key]
		if !held || !definition.DecidedAt(layer.Level) {
			continue
		}
		if resolution.Locked() {
			continue
		}
		if entry.Value != nil && admits(definition, layer.Level, resolution, entry.Value) {
			resolution.Effective = entry.Value
			resolution.Source = layer.Level
		}
		switch entry.Hold {
		case HoldLock:
			if definition.Lockable {
				resolution.LockedBy = layer.Level
				resolution.LockedByPerson = entry.By
			}
		case HoldRestrict:
			if definition.Restrictable {
				resolution.Allowed = narrow(resolution.Allowed, entry.Allowed)
				resolution.Effective = within(resolution.Effective, resolution.Allowed)
			}
		case HoldSet:
		}
	}
	return resolution
}

// admits reports whether a level may set this value under what is in force.
func admits(definition Definition, level Level, inForce Resolution, value any) bool {
	if !allows(inForce.Allowed, value) {
		return false
	}
	if definition.heldToTighten(level) {
		return definition.Tightens(inForce.Effective, value)
	}
	return true
}

// allows reports whether a value lies inside what a restriction left.
func allows(allowed Allowed, value any) bool {
	switch typed := value.(type) {
	case int:
		return (allowed.Minimum == nil || typed >= *allowed.Minimum) &&
			(allowed.Maximum == nil || typed <= *allowed.Maximum)
	case string:
		return allowed.Options == nil || slices.Contains(allowed.Options, typed)
	case []string:
		if allowed.Options == nil {
			return true
		}
		for _, each := range typed {
			if !slices.Contains(allowed.Options, each) {
				return false
			}
		}
		return true
	}
	return true
}

// narrow intersects two restrictions: the lower may only narrow the upper.
func narrow(upper, lower Allowed) Allowed {
	out := upper
	if lower.Minimum != nil && (out.Minimum == nil || *lower.Minimum > *out.Minimum) {
		minimum := *lower.Minimum
		out.Minimum = &minimum
	}
	if lower.Maximum != nil && (out.Maximum == nil || *lower.Maximum < *out.Maximum) {
		maximum := *lower.Maximum
		out.Maximum = &maximum
	}
	if lower.Options != nil {
		if out.Options == nil {
			out.Options = slices.Clone(lower.Options)
		} else {
			out.Options = slices.DeleteFunc(slices.Clone(out.Options), func(option string) bool {
				return !slices.Contains(lower.Options, option)
			})
		}
	}
	return out
}

// within brings a value in force inside a restriction placed on it: a count is held at the
// range, a choice outside the subset becomes the subset's first, a list keeps what the subset
// holds.
func within(value any, allowed Allowed) any {
	switch typed := value.(type) {
	case int:
		if allowed.Minimum != nil && typed < *allowed.Minimum {
			return *allowed.Minimum
		}
		if allowed.Maximum != nil && typed > *allowed.Maximum {
			return *allowed.Maximum
		}
	case string:
		if len(allowed.Options) > 0 && !slices.Contains(allowed.Options, typed) {
			return allowed.Options[0]
		}
	case []string:
		if allowed.Options != nil {
			return slices.DeleteFunc(slices.Clone(typed), func(each string) bool {
				return !slices.Contains(allowed.Options, each)
			})
		}
	}
	return value
}
