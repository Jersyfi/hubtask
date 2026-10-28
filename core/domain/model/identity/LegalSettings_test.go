// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"reflect"
	"testing"
)

// The golden test of the legal links' move onto the settings catalogue (F-5): the three functions
// below are the resolution as it stood before, kept verbatim but for their names, and every
// generated combination of three layers resolves the same through both.

func effectiveAccountLinkBefore(instance, plan LegalLayer) string {
	if instance.Locks[LinkAccount] {
		return instance.Links.AccountURL
	}
	if plan.Links.AccountURL != "" {
		return plan.Links.AccountURL
	}
	return instance.Links.AccountURL
}

// EffectiveLegal resolves workspace -> plan -> instance -> nothing (identity.md §18).
//
// A locked link is the level's that locked it, whatever the levels below hold, for Effective's
// reason: resolving has to answer something for a row written before the lock landed, and refusing
// is the writer's job where there is a field to name. An installation lock binds the plan as it
// binds the workspace.
func effectiveLegalBefore(instance, plan, workspace LegalLayer) (LegalLinks, map[LegalLink]LockOrigin) {
	resolved := LegalLinks{}
	locks := map[LegalLink]LockOrigin{}

	for _, name := range LegalLinkNames() {
		switch {
		case instance.Locks[name]:
			locks[name] = LockInstance
			resolved = resolved.With(name, instance.Links.Of(name))
		case plan.Locks[name]:
			locks[name] = LockPlan
			resolved = resolved.With(name, plan.Links.Of(name))
		case workspace.Links.Of(name) != "":
			resolved = resolved.With(name, workspace.Links.Of(name))
		case plan.Links.Of(name) != "":
			resolved = resolved.With(name, plan.Links.Of(name))
		default:
			resolved = resolved.With(name, instance.Links.Of(name))
		}
	}

	// The version is the terms link's, from wherever the link came from, and under its lock.
	source := legalSourceOfBefore(LinkTerms, instance, plan, workspace)
	switch source {
	case SourceInstance:
		resolved.TermsVersion = instance.Links.TermsVersion
	case SourcePlan:
		resolved.TermsVersion = plan.Links.TermsVersion
	case SourceWorkspace:
		resolved.TermsVersion = workspace.Links.TermsVersion
	}
	if resolved.TermsURL == "" {
		// A lock that pinned "no link" pins no terms either.
		resolved.TermsVersion = ""
	} else {
		resolved.TermsSetBy = termsSetByOf(source)
	}
	if origin := locks[LinkTerms]; origin != LockNone {
		locks[LinkTermsVersion] = origin
	}
	return resolved, locks
}

// LegalSourceOf answers which level a link in force came from, EffectiveLegal's order (P-06). A
// lock is its level's decision even where it pinned "no link".
func legalSourceOfBefore(name LegalLink, instance, plan, workspace LegalLayer) PolicySource {
	if name == LinkTermsVersion {
		// The version has no source of its own: it is the terms link's.
		name = LinkTerms
	}
	switch {
	case instance.Locks[name]:
		return SourceInstance
	case plan.Locks[name]:
		return SourcePlan
	case workspace.Links.Of(name) != "":
		return SourceWorkspace
	case plan.Links.Of(name) != "":
		return SourcePlan
	case instance.Links.Of(name) != "":
		return SourceInstance
	}
	return SourceDefault
}

// sampleLegalLayer is one level's links; a workspace never holds the provider's account link.
func sampleLegalLayer(seq *lcg, names []LegalLink) LegalLayer {
	values := []string{"", "", "https://a.example/x", "https://b.example/y"}
	layer := LegalLayer{Locks: map[LegalLink]bool{}}
	for _, name := range names {
		layer.Links = layer.Links.With(name, values[seq.next(len(values))])
		if seq.next(4) == 0 {
			layer.Locks[name] = seq.next(3) != 0
		}
	}
	return layer
}

func TestLegalResolvesAsBeforeTheCatalogue(t *testing.T) {
	seq := lcg(20261029)
	for run := range 5000 {
		levelNames := LevelLegalSettingNames()
		instance, plan := sampleLegalLayer(&seq, levelNames), sampleLegalLayer(&seq, levelNames)
		workspace := sampleLegalLayer(&seq, LegalSettingNames())
		links, locks := EffectiveLegal(instance, plan, workspace)
		wantLinks, wantLocks := effectiveLegalBefore(instance, plan, workspace)
		if !reflect.DeepEqual(links, wantLinks) || !reflect.DeepEqual(locks, wantLocks) {
			t.Fatalf("run %d:\n got  %+v %v\n want %+v %v", run, links, locks, wantLinks, wantLocks)
		}
		for _, name := range LevelLegalSettingNames() {
			if got, want := LegalSourceOf(name, instance, plan, workspace), legalSourceOfBefore(name, instance, plan, workspace); got != want {
				t.Fatalf("run %d, %s: source %s, want %s", run, name, got, want)
			}
		}
		if got, want := EffectiveAccountLink(instance, plan), effectiveAccountLinkBefore(instance, plan); got != want {
			t.Fatalf("run %d: account link %q, want %q", run, got, want)
		}
	}
}
