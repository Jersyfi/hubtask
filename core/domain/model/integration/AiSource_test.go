// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func sources(value domain.AiSources) *domain.AiSources { return &value }

func minimum(value domain.AiMinJurisdiction) *domain.AiMinJurisdiction { return &value }

func locked(names ...domain.AiSwitch) map[domain.AiSwitch]bool {
	locks := map[domain.AiSwitch]bool{}
	for _, name := range names {
		locks[name] = true
	}
	return locks
}

// "Stricter" for ai.sources is a subset: EITHER contains OFFERED and OWN, every value contains NONE.
func TestASourcesValueIsWithinExactlyTheValuesThatContainIt(t *testing.T) {
	within := map[domain.AiSources][]domain.AiSources{
		domain.AiSourcesNone:    {domain.AiSourcesNone, domain.AiSourcesOffered, domain.AiSourcesOwn, domain.AiSourcesEither},
		domain.AiSourcesOffered: {domain.AiSourcesOffered, domain.AiSourcesEither},
		domain.AiSourcesOwn:     {domain.AiSourcesOwn, domain.AiSourcesEither},
		domain.AiSourcesEither:  {domain.AiSourcesEither},
	}
	for _, narrow := range domain.AiSourcesValues() {
		for _, wide := range domain.AiSourcesValues() {
			want := false
			for _, container := range within[narrow] {
				want = want || container == wide
			}
			if got := narrow.Within(wide); got != want {
				t.Errorf("%s within %s = %v, want %v", narrow, wide, got, want)
			}
		}
	}
}

// Further right is stricter, and a third country meets only ANY.
func TestTheMinimumAdmitsExactlyTheDeclarationsAtOrAboveIt(t *testing.T) {
	admitted := map[domain.AiMinJurisdiction][]domain.AiJurisdiction{
		domain.AiMinAny:        {domain.AiThirdCountry, domain.AiAdequacy, domain.AiEEA, domain.AiSelfHosted},
		domain.AiMinAdequacy:   {domain.AiAdequacy, domain.AiEEA, domain.AiSelfHosted},
		domain.AiMinEEA:        {domain.AiEEA, domain.AiSelfHosted},
		domain.AiMinSelfHosted: {domain.AiSelfHosted},
	}
	for _, floor := range domain.AiMinJurisdictions() {
		for _, declared := range domain.AiJurisdictions() {
			want := false
			for _, ok := range admitted[floor] {
				want = want || ok == declared
			}
			if got := floor.Admits(declared); got != want {
				t.Errorf("%s admits %s = %v, want %v", floor, declared, got, want)
			}
		}
	}
}

// Every instance value × lock × workspace narrowing, with something offered (UC-AI-06 check 2).
func TestTheRuleResolvesInstanceThenWorkspace(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		instance  domain.AiLayer
		workspace domain.AiLayer
		offered   bool
		want      domain.AiSources
		origin    identity.PolicySource
		lock      identity.LockOrigin
	}{
		{"nothing decided is today's behaviour", domain.AiLayer{}, domain.AiLayer{}, true,
			domain.AiSourcesOwn, identity.SourceDefault, identity.LockNone},
		{"the instance opens both", domain.AiLayer{Sources: sources(domain.AiSourcesEither)}, domain.AiLayer{}, true,
			domain.AiSourcesEither, identity.SourceInstance, identity.LockNone},
		{"a workspace narrows an open EITHER to its own", domain.AiLayer{Sources: sources(domain.AiSourcesEither)},
			domain.AiLayer{Sources: sources(domain.AiSourcesOwn)}, true,
			domain.AiSourcesOwn, identity.SourceWorkspace, identity.LockNone},
		{"a workspace narrows an open EITHER to none", domain.AiLayer{Sources: sources(domain.AiSourcesEither)},
			domain.AiLayer{Sources: sources(domain.AiSourcesNone)}, true,
			domain.AiSourcesNone, identity.SourceWorkspace, identity.LockNone},
		{"a lock keeps a narrowing from applying", domain.AiLayer{Sources: sources(domain.AiSourcesEither), Locks: locked(domain.AiSwitchSources)},
			domain.AiLayer{Sources: sources(domain.AiSourcesOwn)}, true,
			domain.AiSourcesEither, identity.SourceInstance, identity.LockInstance},
		{"a lock never holds a workspace in", domain.AiLayer{Sources: sources(domain.AiSourcesOffered), Locks: locked(domain.AiSwitchSources)},
			domain.AiLayer{Sources: sources(domain.AiSourcesNone)}, true,
			domain.AiSourcesNone, identity.SourceWorkspace, identity.LockInstance},
		{"a workspace value wider than the instance is ignored", domain.AiLayer{Sources: sources(domain.AiSourcesOffered)},
			domain.AiLayer{Sources: sources(domain.AiSourcesEither)}, true,
			domain.AiSourcesOffered, identity.SourceInstance, identity.LockNone},
		{"with nothing offered EITHER resolves to OWN", domain.AiLayer{Sources: sources(domain.AiSourcesEither)}, domain.AiLayer{}, false,
			domain.AiSourcesOwn, identity.SourceInstance, identity.LockNone},
		{"with nothing offered OFFERED resolves to NONE", domain.AiLayer{Sources: sources(domain.AiSourcesOffered)}, domain.AiLayer{}, false,
			domain.AiSourcesNone, identity.SourceInstance, identity.LockNone},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule := domain.EffectiveAi(testCase.instance, domain.AiLayer{}, testCase.workspace, testCase.offered)
			if rule.Sources != testCase.want {
				t.Errorf("sources %s, want %s", rule.Sources, testCase.want)
			}
			if got := rule.OriginOf(domain.AiSwitchSources); got != testCase.origin {
				t.Errorf("origin %s, want %s", got, testCase.origin)
			}
			if got := rule.LockOf(domain.AiSwitchSources); got != testCase.lock {
				t.Errorf("lock %q, want %q", got, testCase.lock)
			}
		})
	}
}

func TestThePlanCannotUndoTheInstancesLock(t *testing.T) {
	rule := domain.EffectiveAi(
		domain.AiLayer{Sources: sources(domain.AiSourcesOffered), MinJurisdiction: minimum(domain.AiMinEEA),
			Locks: locked(domain.AiSwitchSources, domain.AiSwitchMinJurisdiction)},
		domain.AiLayer{Sources: sources(domain.AiSourcesEither), MinJurisdiction: minimum(domain.AiMinAny)},
		domain.AiLayer{}, true)
	if rule.Sources != domain.AiSourcesOffered || rule.MinJurisdiction != domain.AiMinEEA {
		t.Errorf("the plan moved a locked value: %+v", rule)
	}
}

// multi-tenancy.md §4.2: a plan narrows what the installation leaves open and never widens it; a
// plan's value written before the installation narrowed is ignored, and its lock still holds the
// value in force.
func TestAPlanOnlyNarrowsTheInstallationsOpenRule(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		instance    domain.AiLayer
		plan        domain.AiLayer
		wantSources domain.AiSources
		wantMinimum domain.AiMinJurisdiction
		wantOrigin  identity.PolicySource
		wantLock    identity.LockOrigin
		wantMinLock identity.LockOrigin
	}{
		{"a plan narrows", domain.AiLayer{Sources: sources(domain.AiSourcesEither)},
			domain.AiLayer{Sources: sources(domain.AiSourcesOffered)},
			domain.AiSourcesOffered, domain.AiMinAny, identity.SourcePlan, identity.LockNone, identity.LockNone},
		{"a plan cannot widen", domain.AiLayer{Sources: sources(domain.AiSourcesOffered)},
			domain.AiLayer{Sources: sources(domain.AiSourcesEither)},
			domain.AiSourcesOffered, domain.AiMinAny, identity.SourceInstance, identity.LockNone, identity.LockNone},
		{"a wide plan value ignored, its lock held", domain.AiLayer{Sources: sources(domain.AiSourcesOffered)},
			domain.AiLayer{Sources: sources(domain.AiSourcesEither), Locks: locked(domain.AiSwitchSources)},
			domain.AiSourcesOffered, domain.AiMinAny, identity.SourceInstance, identity.LockPlan, identity.LockNone},
		{"a plan's looser minimum ignored", domain.AiLayer{MinJurisdiction: minimum(domain.AiMinEEA)},
			domain.AiLayer{MinJurisdiction: minimum(domain.AiMinAny)},
			domain.AiSourcesOwn, domain.AiMinEEA, identity.SourceDefault, identity.LockNone, identity.LockNone},
		{"a plan's stricter minimum, locked", domain.AiLayer{MinJurisdiction: minimum(domain.AiMinAdequacy)},
			domain.AiLayer{MinJurisdiction: minimum(domain.AiMinSelfHosted), Locks: locked(domain.AiSwitchMinJurisdiction)},
			domain.AiSourcesOwn, domain.AiMinSelfHosted, identity.SourceDefault, identity.LockNone, identity.LockPlan},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			rule := domain.EffectiveAi(testCase.instance, testCase.plan, domain.AiLayer{}, true)
			if rule.Sources != testCase.wantSources || rule.MinJurisdiction != testCase.wantMinimum {
				t.Errorf("sources %s, minimum %s; want %s, %s", rule.Sources, rule.MinJurisdiction,
					testCase.wantSources, testCase.wantMinimum)
			}
			if got := rule.OriginOf(domain.AiSwitchSources); got != testCase.wantOrigin {
				t.Errorf("sources set by %s, want %s", got, testCase.wantOrigin)
			}
			if rule.LockOf(domain.AiSwitchSources) != testCase.wantLock ||
				rule.LockOf(domain.AiSwitchMinJurisdiction) != testCase.wantMinLock {
				t.Errorf("locks %v", rule.Locks)
			}
		})
	}
}

// UC-PRV-05 check 11: a plan locks *each person may keep their content out* and cannot set AI for
// everybody - only the workspace chooses that, with its basis.
func TestAPlanLocksThePersonsChoiceAndCannotTakeIt(t *testing.T) {
	notOffered := domain.AiOptOutNotOffered
	offered := domain.AiOptOutOffered
	rule := domain.EffectiveAi(domain.AiLayer{}, domain.AiLayer{PersonOptOut: &notOffered}, domain.AiLayer{}, true)
	if rule.PersonOptOut != domain.AiOptOutOffered {
		t.Errorf("a plan's NOT_OFFERED took effect: %s", rule.PersonOptOut)
	}
	basis := domain.AiOptOutBasis("CONTRACT")
	rule = domain.EffectiveAi(domain.AiLayer{},
		domain.AiLayer{PersonOptOut: &offered, Locks: locked(domain.AiSwitchPersonOptOut)},
		domain.AiLayer{PersonOptOut: &notOffered, Basis: basis}, true)
	if rule.PersonOptOut != domain.AiOptOutOffered || rule.LockOf(domain.AiSwitchPersonOptOut) != identity.LockPlan ||
		rule.OriginOf(domain.AiSwitchPersonOptOut) != identity.SourcePlan {
		t.Errorf("under the plan's lock the rule reads %s, lock %q, origin %s", rule.PersonOptOut,
			rule.LockOf(domain.AiSwitchPersonOptOut), rule.OriginOf(domain.AiSwitchPersonOptOut))
	}
}

// UC-AI-06 check 4: the minimum a plan set is named as the plan's in the own model's refusal.
func TestAnOwnModelBelowThePlansMinimumIsRefusedNamingThePlan(t *testing.T) {
	rule := domain.EffectiveAi(domain.AiLayer{}, domain.AiLayer{MinJurisdiction: minimum(domain.AiMinEEA)}, domain.AiLayer{}, false)
	err := shared.AsError(rule.AdmitsOwn(domain.AiAdequacy))
	if err == nil || err.Params["origin"] != "PLAN" {
		t.Fatalf("got %v", err)
	}
}

func TestANarrowingIsRefusedWhereLockedOrLooser(t *testing.T) {
	open := domain.EffectiveAi(domain.AiLayer{Sources: sources(domain.AiSourcesOffered)}, domain.AiLayer{}, domain.AiLayer{}, true)
	closed := domain.EffectiveAi(domain.AiLayer{Sources: sources(domain.AiSourcesEither), Locks: locked(domain.AiSwitchSources)},
		domain.AiLayer{}, domain.AiLayer{}, true)

	for _, testCase := range []struct {
		name     string
		rule     domain.AiRule
		proposed domain.AiSources
		code     string
	}{
		{"wider than the instance", open, domain.AiSourcesEither, "auth.policy_loosens"},
		{"another source than the instance", open, domain.AiSourcesOwn, "auth.policy_loosens"},
		{"narrower but locked", closed, domain.AiSourcesOwn, "auth.policy_locked"},
		{"an unknown value", open, "SOME", "auth.policy_value_invalid"},
		{"none under a lock", closed, domain.AiSourcesNone, ""},
		{"the same as above", open, domain.AiSourcesOffered, ""},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.rule.Narrowed(testCase.proposed)
			if got := shared.AsError(err); (got == nil) != (testCase.code == "") || (got != nil && got.DetailCode != testCase.code) {
				t.Errorf("got %v, want %q", err, testCase.code)
			}
		})
	}
}

func TestNoneIsAlwaysPermittedAndTheOthersOnlyByTheRule(t *testing.T) {
	for _, value := range domain.AiSourcesValues() {
		rule := domain.EffectiveAi(domain.AiLayer{Sources: sources(value)}, domain.AiLayer{}, domain.AiLayer{}, true)
		if !rule.Permits(domain.AiSourceNone) {
			t.Errorf("%s does not permit NONE", value)
		}
		if rule.Permits(domain.AiSourceOwn) != value.AllowsOwn() || rule.Permits(domain.AiSourceOffered) != value.AllowsOffered() {
			t.Errorf("%s permits the wrong sources", value)
		}
	}
}

func TestAnOwnModelBelowTheMinimumIsRefusedNamingItAndWhoSetIt(t *testing.T) {
	rule := domain.EffectiveAi(domain.AiLayer{MinJurisdiction: minimum(domain.AiMinEEA)}, domain.AiLayer{}, domain.AiLayer{}, false)

	err := shared.AsError(rule.AdmitsOwn(domain.AiAdequacy))
	if err == nil || err.DetailCode != "ai.jurisdiction_below_minimum" {
		t.Fatalf("got %v", err)
	}
	if err.Params["minimum"] != "EEA" || err.Params["origin"] != "INSTANCE" {
		t.Errorf("params %v", err.Params)
	}
	if rule.AdmitsOwn(domain.AiSelfHosted) != nil {
		t.Error("a self-hosted model was refused under EEA")
	}
}

func TestAChoiceIsWhole(t *testing.T) {
	now := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	offered := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

	for _, testCase := range []struct {
		name    string
		source  domain.AiSourceKind
		offered shared.ID
		code    string
	}{
		{"offered with its model", domain.AiSourceOffered, offered, ""},
		{"own", domain.AiSourceOwn, "", ""},
		{"none", domain.AiSourceNone, "", ""},
		{"offered without a model", domain.AiSourceOffered, "", "ai.offered_model_unknown"},
		{"own naming an offered model", domain.AiSourceOwn, offered, "ai.offered_model_without_offered_source"},
		{"an unknown source", "SOMEONE", "", "ai.source_unknown"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := domain.NewAiChoice(aiTenant(), testCase.source, testCase.offered, now)
			if got := shared.AsError(err); (got == nil) != (testCase.code == "") || (got != nil && got.DetailCode != testCase.code) {
				t.Errorf("got %v, want %q", err, testCase.code)
			}
		})
	}
}

func offeredInput() domain.NewOfferedAiProviderInput {
	return domain.NewOfferedAiProviderInput{
		ID:              shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"),
		DisplayName:     " Large (EU) ",
		Processor:       "A processor",
		Kind:            domain.AiOpenAiCompatible,
		BaseURL:         "https://api.example.org/v1",
		CompletionModel: "a-model",
		Jurisdiction:    domain.AiEEA,
		Now:             time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC),
	}
}

func TestAnOfferedModelIsTheInstallationsAndNeverConsented(t *testing.T) {
	offered, err := domain.NewOfferedAiProvider(offeredInput())
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
	if !offered.Installation() || offered.ProcessingAllowed || offered.DisplayName != "Large (EU)" {
		t.Errorf("came back as %+v", offered)
	}
}

func TestAnOfferedModelNeedsWhatAWorkspaceIsShown(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		amend func(*domain.NewOfferedAiProviderInput)
		code  string
	}{
		{"no name", func(in *domain.NewOfferedAiProviderInput) { in.DisplayName = " " }, "ai.display_name_required"},
		{"a long name", func(in *domain.NewOfferedAiProviderInput) { in.DisplayName = strings.Repeat("a", 201) }, "ai.display_name_too_long"},
		{"no processor", func(in *domain.NewOfferedAiProviderInput) { in.Processor = "" }, "ai.processor_required"},
		{"a long processor", func(in *domain.NewOfferedAiProviderInput) { in.Processor = strings.Repeat("a", 201) }, "ai.processor_too_long"},
		{"offering nothing", func(in *domain.NewOfferedAiProviderInput) { in.Kind = domain.AiNoop }, "ai.offered_needs_a_model"},
		{"the workspace rules still apply", func(in *domain.NewOfferedAiProviderInput) { in.BaseURL = "" }, "ai.base_url_required"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := offeredInput()
			testCase.amend(&in)
			_, err := domain.NewOfferedAiProvider(in)
			if got := shared.AsError(err); got == nil || got.DetailCode != testCase.code {
				t.Errorf("got %v, want %q", err, testCase.code)
			}
		})
	}
}

func TestAnOfferedModelsVectorsLiveInTheirOwnSpace(t *testing.T) {
	id := shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	if got := domain.OfferedEmbeddingSpace(id, "embed"); got == "embed" || !strings.HasSuffix(got, "/embed") {
		t.Errorf("space %q", got)
	}
	if domain.OfferedEmbeddingSpace(id, "") != "" {
		t.Error("a model that does not embed named a space")
	}
}
