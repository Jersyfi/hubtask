// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"reflect"
	"testing"
	"time"
)

// The golden test of the move onto the settings catalogue (F-5): effectiveBefore is Effective as
// it stood before, kept verbatim, and every generated combination of three layers must resolve
// to the same rule, the same locks and the same sources through both.
func effectiveBefore(instance, plan, workspace PolicyLayer) EffectivePolicy {
	resolved := DefaultSignInPolicy()
	locks := map[PolicySwitch]LockOrigin{}
	sources := map[PolicySwitch]PolicySource{}

	for _, level := range []struct {
		layer  PolicyLayer
		origin LockOrigin
		source PolicySource
	}{{instance, LockInstance, SourceInstance}, {plan, LockPlan, SourcePlan}, {workspace, LockNone, SourceWorkspace}} {
		// Every level below the installation is held by what is above it: the plan as the
		// workspace is (multi-tenancy.md §4.2).
		below := level.origin != LockInstance
		for _, name := range level.layer.Patch.Decided() {
			if locks[name] != LockNone && below {
				// A locked switch: the lower level's value is ignored rather than refused here.
				// Refusing is the writer's job, where there is a field to name; resolving has to
				// answer something for a row that was written before the lock landed.
				continue
			}
			if below && !tightens(name, resolved, level.layer.Patch) {
				// The row is *older* than the level above it, and the level above has since moved
				// past it. Ignoring it here is what makes "a lower level only ever tightens" true
				// over time rather than only at the moment of the write: an operator who raises the
				// minimum to sixteen has raised it for the plan and the workspace that stored
				// fourteen last year too.
				continue
			}
			resolved = applySwitch(resolved, name, level.layer.Patch)
			sources[name] = level.source
		}
		if level.origin == LockNone {
			continue
		}
		for name, locked := range level.layer.Locks {
			// A lock keeps the origin of the level that placed it first: a plan cannot claim a
			// switch the installation already fixed.
			if locked && locks[name] == LockNone {
				locks[name] = level.origin
			}
		}
	}

	return EffectivePolicy{Policy: bounded(resolved), Locks: locks, Sources: sources}
}

// lcg is a fixed sequence, so that a failure reproduces.
type lcg uint64

func (l *lcg) next(n int) int {
	*l = *l*6364136223846793005 + 1442695040888963407
	return int((*l >> 33) % lcg(n))
}

func samplePatch(seq *lcg) PolicyPatch {
	counts := []int{0, 1, 4, 8, 10, 12, 14, 16, 30, 200}
	number := func() *int {
		if seq.next(3) != 0 {
			return nil
		}
		value := counts[seq.next(len(counts))]
		return &value
	}
	flag := func() *bool {
		if seq.next(3) != 0 {
			return nil
		}
		value := seq.next(2) == 0
		return &value
	}
	patch := PolicyPatch{
		MinLength: number(), MinLowercase: number(), MinUppercase: number(), MinDigits: number(),
		MinSymbols: number(), MinClasses: number(), MaxRepeat: number(), CommonPasswords: flag(),
		ContextWords: flag(), BreachCheck: flag(), MaxAgeDays: number(), HistoryCount: number(),
		MinAgeHours: number(), SessionMaxDays: number(), SessionIdleMinutes: number(),
		ManagedAccounts: flag(), PrivateHubs: flag(),
	}
	if seq.next(3) == 0 {
		mfa := []MfaRequirement{MfaForNobody, MfaForAdmins, MfaForEveryone}[seq.next(3)]
		patch.MfaRequiredFor = &mfa
	}
	if seq.next(3) == 0 {
		methods := [][]string{{MethodDirect}, {MethodOidc}, {MethodDirect, MethodOidc}, {MethodDirect, MethodPasskey}}[seq.next(4)]
		patch.Methods = &methods
	}
	if seq.next(4) == 0 {
		moment := time.Date(2026, 1, 1+seq.next(20), 0, 0, 0, 0, time.UTC)
		patch.RotationFrom = &moment
	}
	return patch
}

func sampleLayer(seq *lcg) PolicyLayer {
	layer := PolicyLayer{Patch: samplePatch(seq), Locks: map[PolicySwitch]bool{}}
	for _, name := range PolicySwitches() {
		if seq.next(5) == 0 {
			layer.Locks[name] = seq.next(4) != 0
		}
	}
	return layer
}

func TestEffectiveResolvesAsBeforeTheCatalogue(t *testing.T) {
	seq := lcg(20261028)
	for run := range 5000 {
		instance, plan, workspace := sampleLayer(&seq), sampleLayer(&seq), sampleLayer(&seq)
		want := effectiveBefore(instance, plan, workspace)
		got := Effective(instance, plan, workspace)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d:\n got  %+v\n want %+v", run, got, want)
		}
	}
}
