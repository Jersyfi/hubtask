// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"reflect"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
)

// The golden test of the AI rule's move onto the settings catalogue (F-5): effectiveAiBefore is
// EffectiveAi as it stood before, kept verbatim but for its name, and every generated combination
// of three layers resolves the same through both.

// EffectiveAi resolves product default → instance → plan → workspace into the rule in force.
//
// The instance may set any value; the plan only narrows what the instance leaves in force - both
// are the levels a lock comes from. The workspace may only narrow, and only where no lock lies,
// except that narrowing to NONE is always its own (ADR-0072 §2: a lock holds a workspace out of a
// source, never in one). A workspace value
// that is no narrowing of what is above it now is ignored rather than honoured - written before
// the level above moved, the way sign-in's Effective treats a stale row.
func effectiveAiBefore(instance, plan, workspace AiLayer, offered bool) AiRule {
	rule := AiRule{
		Sources:         AiSourcesOwn,
		MinJurisdiction: AiMinAny,
		PersonOptOut:    AiOptOutOffered,
		Locks:           map[AiSwitch]identity.LockOrigin{},
		Origins:         map[AiSwitch]identity.PolicySource{},
	}

	for _, level := range []struct {
		layer  AiLayer
		origin identity.LockOrigin
		source identity.PolicySource
	}{{instance, identity.LockInstance, identity.SourceInstance}, {plan, identity.LockPlan, identity.SourcePlan}} {
		// The plan narrows what the installation left in force and never widens it; a value written
		// before the installation narrowed is ignored, as sign-in's Effective treats one (§4.2).
		below := level.origin != identity.LockInstance
		if level.layer.Sources != nil && level.layer.Sources.Valid() && rule.Locks[AiSwitchSources] == identity.LockNone &&
			(!below || level.layer.Sources.Within(rule.Sources)) {
			rule.Sources = *level.layer.Sources
			rule.Origins[AiSwitchSources] = level.source
		}
		if level.layer.MinJurisdiction != nil && level.layer.MinJurisdiction.Valid() &&
			rule.Locks[AiSwitchMinJurisdiction] == identity.LockNone &&
			(!below || slices.Index(aiMinOrder, *level.layer.MinJurisdiction) >= slices.Index(aiMinOrder, rule.MinJurisdiction)) {
			rule.MinJurisdiction = *level.layer.MinJurisdiction
			rule.Origins[AiSwitchMinJurisdiction] = level.source
		}
		// Only OFFERED is honoured above the workspace: a level that cannot name a legal basis
		// cannot make AI part of everybody's work (milestone PH, decision 4).
		if level.layer.PersonOptOut != nil && *level.layer.PersonOptOut == AiOptOutOffered &&
			rule.Locks[AiSwitchPersonOptOut] == identity.LockNone {
			rule.Origins[AiSwitchPersonOptOut] = level.source
		}
		for _, name := range AiSwitches() {
			if level.layer.Locks[name] && rule.Locks[name] == identity.LockNone {
				rule.Locks[name] = level.origin
			}
		}
	}

	rule.Above = rule.Sources
	if narrowed := workspace.Sources; narrowed != nil && narrowed.Valid() && narrowed.Within(rule.Sources) &&
		(rule.Locks[AiSwitchSources] == identity.LockNone || *narrowed == AiSourcesNone) {
		rule.Sources = *narrowed
		rule.Origins[AiSwitchSources] = identity.SourceWorkspace
	}

	// The workspace's NOT_OFFERED stands only with a basis and where nothing is locked; one stored
	// before a lock was placed is ignored, as a stale narrowing is.
	if position := workspace.PersonOptOut; position != nil && rule.Locks[AiSwitchPersonOptOut] == identity.LockNone {
		switch {
		case *position == AiOptOutNotOffered && workspace.Basis.Valid():
			rule.PersonOptOut, rule.Basis = AiOptOutNotOffered, workspace.Basis
			rule.Origins[AiSwitchPersonOptOut] = identity.SourceWorkspace
		case *position == AiOptOutOffered:
			rule.Origins[AiSwitchPersonOptOut] = identity.SourceWorkspace
		}
	}

	if !offered {
		rule.Sources = aiSourcesOf(rule.Sources.AllowsOwn(), false)
	}
	return rule
}

type aiSeq uint64

func (l *aiSeq) next(n int) int {
	*l = *l*6364136223846793005 + 1442695040888963407
	return int((*l >> 33) % aiSeq(n))
}

func sampleAiLayer(seq *aiSeq) AiLayer {
	layer := AiLayer{Locks: map[AiSwitch]bool{}}
	sources := []AiSources{AiSourcesEither, AiSourcesOffered, AiSourcesOwn, AiSourcesNone, "BOGUS"}
	floors := append(slices.Clone(aiMinOrder), "BOGUS")
	positions := []AiPersonOptOut{AiOptOutOffered, AiOptOutNotOffered}
	if seq.next(3) != 0 {
		value := sources[seq.next(len(sources))]
		layer.Sources = &value
	}
	if seq.next(3) != 0 {
		value := floors[seq.next(len(floors))]
		layer.MinJurisdiction = &value
	}
	if seq.next(3) != 0 {
		value := positions[seq.next(len(positions))]
		layer.PersonOptOut = &value
	}
	if seq.next(2) == 0 {
		layer.Basis = []AiOptOutBasis{"", AiBasisWorksAgreement, AiOptOutBasis("BOGUS")}[seq.next(3)]
	}
	for _, name := range AiSwitches() {
		if seq.next(4) == 0 {
			layer.Locks[name] = seq.next(3) != 0
		}
	}
	return layer
}

func TestEffectiveAiResolvesAsBeforeTheCatalogue(t *testing.T) {
	seq := aiSeq(20261030)
	for run := range 5000 {
		instance, plan, workspace := sampleAiLayer(&seq), sampleAiLayer(&seq), sampleAiLayer(&seq)
		offered := seq.next(2) == 0
		want := effectiveAiBefore(instance, plan, workspace, offered)
		got := EffectiveAi(instance, plan, workspace, offered)
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d:\n got  %+v\n want %+v", run, got, want)
		}
	}
}
