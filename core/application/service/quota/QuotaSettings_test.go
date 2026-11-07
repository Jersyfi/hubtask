// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"reflect"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The golden test of the quotas' move onto the settings catalogue (F-5): the four functions below
// are the resolution as it stood before, kept verbatim but for their names, and every generated
// combination of levels resolves the same through both.

// planApplies reports whether the plan's value for a quota is read: never under the installation's
// lock, which binds the plan as it binds every workspace.
func (l Levels) planAppliesBefore(name string) bool {
	return !l.InstanceLocks[name] && valueOf(l.Plan.Overrides, name) != nil
}

// SourceOf answers which level set the value in force.
func (l Levels) sourceOfBefore(name string) string {
	switch {
	case valueOf(l.Exception, name) != nil:
		if name == AiOwnTokensPerDay {
			return SourceWorkspace
		}
		return SourceException
	case name == AiOwnTokensPerDay:
		return SourceDefault
	case l.planAppliesBefore(name):
		return SourcePlan
	case valueOf(l.Instance, name) != nil:
		return SourceInstance
	default:
		return SourceDefault
	}
}

// LockOf answers which level locked a quota, or "" - the installation's lock first, which binds the
// plan's. The operator's exception still decides under either: a lock says what a workspace may not
// ask for, not what the operator may not grant.
func (l Levels) lockOfBefore(name string) string {
	switch {
	case name == AiOwnTokensPerDay:
		return ""
	case l.InstanceLocks[name]:
		return LockInstance
	case l.Plan.Locks[name]:
		return LockPlan
	default:
		return ""
	}
}

// Resolve answers the limits in force: the mode's table, the installation's defaults, the plan's
// where the installation does not lock them, and the operator's exception as the last word. The own
// model's budget is the workspace's alone: no product default, installation or plan reaches it.
func resolveBefore(levels Levels, mode env.TenancyMode) Limits {
	limits := Defaults(mode)
	for _, name := range OperatorNames() {
		if value := valueOf(levels.Instance, name); value != nil {
			limits.set(name, *value)
		}
		if levels.planAppliesBefore(name) {
			limits.set(name, *valueOf(levels.Plan.Overrides, name))
		}
		if value := valueOf(levels.Exception, name); value != nil {
			limits.set(name, *value)
		}
	}
	if value := levels.Exception.AiOwnTokensPerDay; value != nil {
		limits.AiOwnTokensPerDay = *value
	}
	return limits
}

type quotaSeq uint64

func (q *quotaSeq) next(n int) int {
	*q = *q*6364136223846793005 + 1442695040888963407
	return int((*q >> 33) % quotaSeq(n))
}

func sampleOverrides(seq *quotaSeq) repository.Overrides {
	values := []int64{0, 1, 50, 1000}
	value := func() *int64 {
		if seq.next(2) == 0 {
			return nil
		}
		held := values[seq.next(len(values))]
		return &held
	}
	return repository.Overrides{
		APIRequestsPerMinute: value(), Items: value(), MediaBytes: value(), AutomationRunsPerHour: value(),
		WebhookTargets: value(), ExportJobs: value(), Accounts: value(), AiTokensPerDay: value(),
		AiOwnTokensPerDay: value(), AiArrivalProposalsPerDay: value(),
	}
}

func sampleLocks(seq *quotaSeq) map[string]bool {
	locks := map[string]bool{}
	for _, name := range Names() {
		if seq.next(3) == 0 {
			locks[name] = seq.next(3) != 0
		}
	}
	return locks
}

func TestQuotasResolveAsBeforeTheCatalogue(t *testing.T) {
	seq := quotaSeq(20261031)
	for run := range 5000 {
		levels := Levels{
			Instance: sampleOverrides(&seq), InstanceLocks: sampleLocks(&seq),
			Plan:      repository.PlanLimits{Overrides: sampleOverrides(&seq), Locks: sampleLocks(&seq)},
			Exception: sampleOverrides(&seq),
		}
		for _, mode := range []env.TenancyMode{env.TenancySingle, env.TenancyMulti} {
			if got, want := Resolve(levels, mode), resolveBefore(levels, mode); !reflect.DeepEqual(got, want) {
				t.Fatalf("run %d:\n got  %+v\n want %+v", run, got, want)
			}
		}
		for _, name := range Names() {
			if got, want := levels.SourceOf(name), levels.sourceOfBefore(name); got != want {
				t.Fatalf("run %d, %s: source %s, want %s", run, name, got, want)
			}
			if got, want := levels.LockOf(name), levels.lockOfBefore(name); got != want {
				t.Fatalf("run %d, %s: lock %s, want %s", run, name, got, want)
			}
		}
	}
}
