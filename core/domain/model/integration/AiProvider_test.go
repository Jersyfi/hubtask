// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration_test

import (
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

func aiTenant() shared.ID {
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
}

func aiInput() domain.NewAiProviderInput {
	return domain.NewAiProviderInput{
		TenantID:        aiTenant(),
		Kind:            domain.AiOpenAiCompatible,
		BaseURL:         "https://api.example.org/v1",
		CompletionModel: "a-model",
		Jurisdiction:    domain.AiEEA,
		Now:             time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC),
	}
}

func TestAValidProviderIsAccepted(t *testing.T) {
	configured, err := domain.NewAiProvider(aiInput())
	if err != nil {
		t.Fatalf("a valid provider was refused: %v", err)
	}
	if configured.Version != 1 || configured.CreatedAt.IsZero() {
		t.Errorf("the provider came back unstamped: %+v", configured)
	}
	if configured.ProcessingAllowed {
		t.Error("consent was granted by a configuration that did not ask for it")
	}
}

// Every refusal is one of these, and each exists because the alternative is a configuration that
// looks saved and does not work.
func TestAProviderThatCouldNotWorkIsRefused(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		amend func(*domain.NewAiProviderInput)
		code  string
	}{
		{"an unknown kind", func(in *domain.NewAiProviderInput) {
			in.Kind = "GUESS"
		}, "ai.kind_unknown"},
		{"an unknown jurisdiction", func(in *domain.NewAiProviderInput) {
			in.Jurisdiction = "SOMEWHERE"
		}, "ai.jurisdiction_unknown"},
		{"a calling kind with no endpoint", func(in *domain.NewAiProviderInput) {
			in.BaseURL = ""
		}, "ai.base_url_required"},
		{"an endpoint that is not an address", func(in *domain.NewAiProviderInput) {
			in.BaseURL = "not an address"
		}, "ai.base_url_malformed"},
		{"an endpoint on a scheme nothing dials", func(in *domain.NewAiProviderInput) {
			in.BaseURL = "ftp://models.example.org"
		}, "ai.base_url_scheme_unsupported"},
		{"credentials smuggled into the endpoint", func(in *domain.NewAiProviderInput) {
			in.BaseURL = "https://key:secret@api.example.org/v1"
		}, "ai.base_url_carries_credentials"},
		{"a provider with no model at all", func(in *domain.NewAiProviderInput) {
			in.CompletionModel = ""
			in.EmbeddingModel = ""
		}, "ai.model_required"},
		{"a model name longer than the column", func(in *domain.NewAiProviderInput) {
			in.CompletionModel = string(make([]byte, 201))
		}, "ai.model_name_too_long"},
		{"a NOOP carrying an endpoint it will never call", func(in *domain.NewAiProviderInput) {
			in.Kind = domain.AiNoop
			in.Jurisdiction = domain.AiSelfHosted
		}, "ai.noop_carries_configuration"},
		{"consent to a provider that calls nothing", func(in *domain.NewAiProviderInput) {
			in.Kind = domain.AiNoop
			in.Jurisdiction = domain.AiSelfHosted
			in.BaseURL, in.CompletionModel, in.EmbeddingModel = "", "", ""
			in.ProcessingAllowed = true
		}, "ai.noop_cannot_process"},
		{"a local model declared to be somewhere else", func(in *domain.NewAiProviderInput) {
			in.Kind = domain.AiOllama
			in.Jurisdiction = domain.AiThirdCountry
		}, "ai.local_model_is_self_hosted"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			in := aiInput()
			testCase.amend(&in)

			_, err := domain.NewAiProvider(in)
			if err == nil {
				t.Fatal("the configuration was accepted")
			}
			if got := shared.AsError(err).DetailCode; got != testCase.code {
				t.Errorf("detail code %q, want %q", got, testCase.code)
			}
		})
	}
}

func TestANoopProviderIsAValidConfiguration(t *testing.T) {
	in := aiInput()
	in.Kind = domain.AiNoop
	in.Jurisdiction = domain.AiSelfHosted
	in.BaseURL, in.CompletionModel = "", ""

	configured, err := domain.NewAiProvider(in)
	if err != nil {
		t.Fatalf("switching AI off was refused: %v", err)
	}
	if configured.Kind != domain.AiNoop || configured.ProcessingAllowed {
		t.Errorf("a switched-off provider came back as %+v", configured)
	}
}

// Only one of the four jurisdictions is a transfer somebody has to have confirmed, and the
// classification lives on the value so that a fifth one is classified once rather than in three
// places that drift apart.
func TestOnlyAThirdCountryNeedsTheOperatorsConfirmation(t *testing.T) {
	needs := map[domain.AiJurisdiction]bool{
		domain.AiSelfHosted:   false,
		domain.AiEEA:          false,
		domain.AiAdequacy:     false,
		domain.AiThirdCountry: true,
	}

	for _, jurisdiction := range domain.AiJurisdictions() {
		want, known := needs[jurisdiction]
		if !known {
			t.Fatalf("%q is a jurisdiction this test has never classified", jurisdiction)
		}
		if got := jurisdiction.NeedsThirdCountryConfirmation(); got != want {
			t.Errorf("%q needs confirmation = %v, want %v", jurisdiction, got, want)
		}
	}
}

func TestTheClosedSetsAreClosed(t *testing.T) {
	if domain.AiProviderKind("SOMETHING").Valid() {
		t.Error("an invented kind reports itself valid")
	}
	if domain.AiJurisdiction("SOMEWHERE").Valid() {
		t.Error("an invented jurisdiction reports itself valid")
	}
	if len(domain.AiProviderKinds()) != 3 || len(domain.AiJurisdictions()) != 4 {
		t.Error("a value was added to a closed set without this test being told")
	}
}
