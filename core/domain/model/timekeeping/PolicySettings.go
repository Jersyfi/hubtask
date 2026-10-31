// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package timekeeping

import "github.com/Jersyfi/hubtask/core/domain/model/setting"

// The time policy's two entries of the settings catalogue. The workspace's own say is its
// time_policy row - versioned, synchronised and archived as one record - read as the WORKSPACE
// layer; the installation and a plan decide above it in their own tables and may lock either
// (a works council's aggregated visibility fixed for every workspace of a plan).
const (
	LockedThroughKey setting.Key = "time.locked_through"
	VisibilityKey    setting.Key = "time.visibility"
)

// The writers: the operator above, whoever approves time at the workspace.
func policyWriters() map[setting.Level]string {
	return map[setting.Level]string{
		setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
		setting.LevelWorkspace: "APPROVE_TIME",
	}
}

func policyLevels() []setting.Level {
	return []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace}
}

// LockedThroughDefinition declares the last locked day; without one no day is locked.
func LockedThroughDefinition() setting.Definition {
	return setting.Definition{
		Key: LockedThroughKey, Kind: setting.KindDate, Levels: policyLevels(),
		Lockable: true, May: setting.MayReplace, Writers: policyWriters(),
	}
}

// VisibilityDefinition declares what a lead sees of other people's time, FULL unless decided.
func VisibilityDefinition() setting.Definition {
	return setting.Definition{
		Key: VisibilityKey, Kind: setting.KindChoice,
		Options: []string{string(VisibilityFull), string(VisibilityAggregated)},
		Levels:  policyLevels(), Lockable: true, May: setting.MayReplace, Writers: policyWriters(),
		Default: string(VisibilityFull),
	}
}

// PolicyDefinitions is both, in the order the time settings screen draws them.
func PolicyDefinitions() []setting.Definition {
	return []setting.Definition{LockedThroughDefinition(), VisibilityDefinition()}
}

// Layer is the workspace's row as the settings layer it is: nothing before anybody set it, the
// visibility once they did, the locked day where there is one. A row without a locked day leaves
// that to the levels above.
func (p Policy) Layer() setting.Layer {
	layer := setting.Layer{Level: setting.LevelWorkspace, Entries: map[setting.Key]setting.Entry{}}
	if p.Version == 0 {
		return layer
	}
	if p.Visibility.Valid() {
		layer.Entries[VisibilityKey] = setting.Entry{Value: string(p.Visibility), Hold: setting.HoldSet, By: p.UpdatedBy}
	}
	if p.LockedThrough != nil {
		layer.Entries[LockedThroughKey] = setting.Entry{Value: p.LockedThrough.String(), Hold: setting.HoldSet, By: p.UpdatedBy}
	}
	return layer
}

// ResolvedPolicy is the policy in force with where each part comes from.
type ResolvedPolicy struct {
	// Policy is in force: the row's version and author, the resolved locked day and visibility.
	Policy        Policy
	LockedThrough setting.Resolution
	Visibility    setting.Resolution
}

// ResolvePolicy resolves the stored row under the installation's and the plan's layers. A value
// above that is not a day or not a visibility is read as none - the default - rather than refused,
// as resolving has to answer something.
func ResolvePolicy(stored Policy, above ...setting.Layer) ResolvedPolicy {
	layers := append(append([]setting.Layer{}, above...), stored.Layer())
	r := ResolvedPolicy{
		Policy:        stored,
		LockedThrough: setting.Resolve(LockedThroughDefinition(), layers...),
		Visibility:    setting.Resolve(VisibilityDefinition(), layers...),
	}
	r.Policy.LockedThrough = nil
	if text, ok := r.LockedThrough.Effective.(string); ok {
		if d, ok := ParseDay(text); ok {
			r.Policy.LockedThrough = &d
		}
	}
	r.Policy.Visibility = VisibilityFull
	if text, ok := r.Visibility.Effective.(string); ok && Visibility(text).Valid() {
		r.Policy.Visibility = Visibility(text)
	}
	return r
}

// Decided reports whether anything but Hubtask's default is in force: a device holds the default
// without being told.
func (r ResolvedPolicy) Decided() bool {
	return r.Policy.Version != 0 || r.LockedThrough.Source != setting.LevelDefault ||
		r.Visibility.Source != setting.LevelDefault || r.LockedThrough.Locked() || r.Visibility.Locked()
}

// Admits refuses a workspace policy that changes what a level above locked; restating the
// locked value is no change.
func (r ResolvedPolicy) Admits(p Policy) error {
	if r.LockedThrough.Locked() && dayText(p.LockedThrough) != dayText(r.Policy.LockedThrough) {
		return setting.ErrLocked(LockedThroughKey, r.LockedThrough.LockedBy)
	}
	if r.Visibility.Locked() && p.Visibility != r.Policy.Visibility {
		return setting.ErrLocked(VisibilityKey, r.Visibility.LockedBy)
	}
	return nil
}

func dayText(d *Day) string {
	if d == nil {
		return ""
	}
	return d.String()
}
