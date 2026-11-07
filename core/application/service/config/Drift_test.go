// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
)

// A change made outside a plan - on a screen, through REST or `hubctl` - since the scope's latest
// version is recorded as its own version before the next apply: the apply's version holds only its
// own changes and its applier, and the state the screens left is a version to compare and roll
// back to (configuration.md §5).
func TestAChangeMadeOutsideAPlanIsItsOwnVersionBeforeTheNextApply(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident(), "plan_hash": plan["plan_hash"]})

	// On a screen: a type made directly.
	f.world.types["problem"] = map[string]any{"key": "problem", "level": "TASK", "name": "Problem", "system": false}

	change := map[string]any{"types": map[string]any{"change": map[string]any{"level": "TASK", "name": "Change"}}}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": change})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": change, "plan_hash": plan["plan_hash"]})

	if len(f.store.revisions) != 4 {
		t.Fatalf("versions %+v, want the state before, the apply, the direct change, the apply", f.store.revisions)
	}
	direct, applied := f.store.revisions[2], f.store.revisions[3]
	if direct.ActorKind != domain.ActorDirect || !direct.ActorID.IsZero() || direct.Counts.Added != 1 || direct.Version != 3 {
		t.Errorf("the direct change's version %+v", direct)
	}
	if _, held := direct.Document["types"]["change"]; held {
		t.Error("the direct change's version holds the apply's type")
	}
	if applied.Version != 4 || applied.Counts.Added != 1 || applied.ActorID != person.AccountID {
		t.Errorf("the apply's version %+v", applied)
	}

	// Nothing changed since: no version of its own.
	again := map[string]any{"types": map[string]any{"release": map[string]any{"level": "TASK", "name": "Release"}}}
	plan = invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": again})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": again, "plan_hash": plan["plan_hash"]})
	if len(f.store.revisions) != 5 {
		t.Errorf("%d versions, want one more for the apply alone", len(f.store.revisions))
	}
}

// A scope's first apply keeps the state before it as version one, with no counts, so even the
// first apply can be rolled back.
func TestTheFirstApplyKeepsTheStateBeforeItToRollBackTo(t *testing.T) {
	f := newFixture()
	plan := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident()})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"document": incident(), "plan_hash": plan["plan_hash"]})

	if len(f.store.revisions) != 2 {
		t.Fatalf("versions %+v, want the state before and the apply", f.store.revisions)
	}
	before := f.store.revisions[0]
	if before.Version != 1 || before.ActorKind != domain.ActorDirect || before.Counts != (domain.Counts{}) {
		t.Errorf("the state before %+v", before)
	}
	if _, held := before.Document["types"]["incident"]; held {
		t.Error("the state before holds the apply's type")
	}

	back := invoke(t, PlanConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1})
	invoke(t, ApplyConfig{f.desk}.Descriptor(), person, usecase.Input{"to_version": 1, "plan_hash": back["plan_hash"], "acknowledge": true})
	if f.world.types["incident"] != nil {
		t.Error("the first apply was not rolled back")
	}
}
