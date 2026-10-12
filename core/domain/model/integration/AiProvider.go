// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AiProvider is the workspace's configuration for the outbound AI port (ADR-0012).
//
// It lives beside the webhook subscription rather than in a package of its own, and the reason is
// ai-first.md's own first paragraph: "AI in the domain model" is what this project decided against.
// What is modelled here is an *integration* - an endpoint this installation may call, with a sealed
// credential and a declared jurisdiction - which is the same kind of object as a webhook target and
// carries none of the meaning of what is asked of it.
//
// The sealed key is deliberately not a field. A configuration a service can hold in a struct is one
// a service can log, and the one caller that needs the plaintext asks the repository for it by name
// (security.md §8, the discipline the identity provider's client secret follows).
type AiProvider struct {
	// ID names the row. A workspace's own provider is still found by its workspace; the id is what
	// a workspace's choice of an offered model points at (ADR-0072 §1).
	ID shared.ID
	// TenantID is the workspace whose provider this is, and zero for a model the installation
	// offers to every workspace.
	TenantID        shared.ID
	Kind            AiProviderKind
	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
	// HasAPIKey says whether an envelope is stored, which is as much as any reader is told.
	HasAPIKey    bool
	Jurisdiction AiJurisdiction
	// ProcessingAllowed is ai-first.md §2's switch, checked before every call. Configuring a
	// provider and consenting to send this workspace's content to it are two decisions, and this
	// is the second one.
	ProcessingAllowed bool
	// DisplayName and Processor are what a workspace is shown of an offered model: its name and who
	// processes what is sent to it, as the operator declares them. Empty on a workspace's own row.
	DisplayName string
	Processor   string
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// Installation reports whether the installation offers this model rather than one workspace
// configuring it.
func (p AiProvider) Installation() bool { return p.TenantID.IsZero() }

// AiProviderKind is which adapter answers (ADR-0049).
type AiProviderKind string

const (
	// AiNoop calls nothing. What an installation has until somebody chooses otherwise, and a
	// value a workspace can choose deliberately to switch AI off without losing the rest of its
	// configuration.
	AiNoop AiProviderKind = "NOOP"
	// AiOpenAiCompatible covers OpenAI, Azure, Mistral, vLLM and LiteLLM, which agree on a wire
	// format - which is why the kind names the format and not a vendor.
	AiOpenAiCompatible AiProviderKind = "OPENAI_COMPATIBLE"
	// AiOllama is a local model.
	AiOllama AiProviderKind = "OLLAMA"
)

var aiProviderKinds = []AiProviderKind{AiNoop, AiOpenAiCompatible, AiOllama}

// AiProviderKinds is the closed set, in the contract's order.
func AiProviderKinds() []AiProviderKind { return slices.Clone(aiProviderKinds) }

// Valid reports whether the kind is one of the defined ones.
func (k AiProviderKind) Valid() bool { return slices.Contains(aiProviderKinds, k) }

// AiJurisdiction is where the provider processes what is sent to it, as the operator declares it.
//
// A declaration rather than something this software can verify, and that is the point: ADR-0018
// decision 7 asks for a transfer to be *documented* rather than prevented, because whether a
// particular transfer is lawful is an assessment for the controller and not for a task manager.
type AiJurisdiction string

const (
	// AiSelfHosted is a model this installation runs itself. No transfer to anybody, which is the
	// path ADR-0018 decision 7 calls recommended.
	AiSelfHosted AiJurisdiction = "SELF_HOSTED"
	// AiEEA is a provider inside the European Economic Area.
	AiEEA AiJurisdiction = "EEA"
	// AiAdequacy is a third country an adequacy decision covers (Art. 45).
	AiAdequacy AiJurisdiction = "ADEQUACY"
	// AiThirdCountry is everything else, and the only value that needs the operator's confirmation.
	AiThirdCountry AiJurisdiction = "THIRD_COUNTRY"
)

var aiJurisdictions = []AiJurisdiction{AiSelfHosted, AiEEA, AiAdequacy, AiThirdCountry}

// AiJurisdictions is the closed set, in the contract's order.
func AiJurisdictions() []AiJurisdiction { return slices.Clone(aiJurisdictions) }

// Valid reports whether the jurisdiction is one of the defined ones.
func (j AiJurisdiction) Valid() bool { return slices.Contains(aiJurisdictions, j) }

// NeedsThirdCountryConfirmation reports whether choosing this jurisdiction is a transfer the
// installation's operator has to have confirmed (ADR-0018 decision 7, data-protection.md §6).
//
// One method rather than a comparison spelled out at each call site, so that a fifth value added
// later is classified here and not in three places that have drifted apart.
func (j AiJurisdiction) NeedsThirdCountryConfirmation() bool { return j == AiThirdCountry }

// maxModelName matches the column and the contract.
const maxModelName = 200

// NewAiProviderInput is what configuring one needs. The key is not here: it is sealed by the
// application layer and travels to the repository as an envelope, never through the domain.
type NewAiProviderInput struct {
	TenantID          shared.ID
	Kind              AiProviderKind
	BaseURL           string
	CompletionModel   string
	EmbeddingModel    string
	Jurisdiction      AiJurisdiction
	ProcessingAllowed bool
	Now               time.Time
}

// NewAiProvider validates a configuration and normalises what has a normal form.
//
// Four rules, each of which exists because the alternative is a configuration that looks saved and
// does not work:
//
//   - A kind that calls something needs an address, and NOOP must not carry one - an endpoint
//     nothing will call is a value that will be wrong by the time somebody switches the kind back.
//   - At least one model, unless the kind is NOOP. A provider configured with neither completes
//     nothing and embeds nothing, which is NOOP with extra steps and a key stored for no reason.
//   - Consent needs something to consent to. `processing_allowed` on a NOOP is a switch with
//     nothing behind it, and leaving it set would make the next configuration send content the
//     moment it is saved.
//   - The jurisdiction of a self-hosted model is SELF_HOSTED and nothing else, because a local
//     model transfers to nobody and declaring otherwise would put a transfer in the record that
//     never happens.
func NewAiProvider(in NewAiProviderInput) (AiProvider, error) {
	if in.TenantID.IsZero() || in.Now.IsZero() {
		return AiProvider{}, shared.ErrInternal.WithDetail("ai.provider_incomplete")
	}
	if !in.Kind.Valid() {
		return AiProvider{}, fieldError("/kind", "ai.kind_unknown")
	}
	if !in.Jurisdiction.Valid() {
		return AiProvider{}, fieldError("/jurisdiction", "ai.jurisdiction_unknown")
	}

	completion, err := modelName("/completion_model", in.CompletionModel)
	if err != nil {
		return AiProvider{}, err
	}
	embedding, err := modelName("/embedding_model", in.EmbeddingModel)
	if err != nil {
		return AiProvider{}, err
	}

	baseURL := strings.TrimSpace(in.BaseURL)
	if in.Kind == AiNoop {
		if baseURL != "" || completion != "" || embedding != "" {
			return AiProvider{}, fieldError("/kind", "ai.noop_carries_configuration")
		}
		if in.ProcessingAllowed {
			return AiProvider{}, fieldError("/processing_allowed", "ai.noop_cannot_process")
		}
	} else {
		if baseURL, err = aiBaseURL(baseURL); err != nil {
			return AiProvider{}, err
		}
		if completion == "" && embedding == "" {
			return AiProvider{}, fieldError("/completion_model", "ai.model_required")
		}
	}

	if in.Kind == AiOllama && in.Jurisdiction != AiSelfHosted {
		return AiProvider{}, fieldError("/jurisdiction", "ai.local_model_is_self_hosted")
	}

	return AiProvider{
		TenantID: in.TenantID, Kind: in.Kind, BaseURL: baseURL,
		CompletionModel: completion, EmbeddingModel: embedding,
		Jurisdiction: in.Jurisdiction, ProcessingAllowed: in.ProcessingAllowed,
		CreatedAt: in.Now.UTC(), Version: 1,
	}, nil
}

// aiBaseURL checks the shape of the endpoint. Whether it may be dialled is the guard's question,
// which is TargetURL's reasoning applied to the same kind of address - so the rules are the same
// three: parseable with a host, http or https, and no credentials in the URL.
func aiBaseURL(raw string) (string, error) {
	switch {
	case raw == "":
		return "", fieldError("/base_url", "ai.base_url_required")
	case len(raw) > maxTargetURL:
		return "", fieldError("/base_url", "ai.base_url_too_long")
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", fieldError("/base_url", "ai.base_url_malformed")
	}
	if parsed.Scheme != "https" && parsed.Scheme != "http" {
		return "", fieldError("/base_url", "ai.base_url_scheme_unsupported")
	}
	if parsed.User != nil {
		// Credentials in the URL would be a second key, stored in clear in a column and travelling
		// in every log line that records the endpoint. The sealed key is the one credential.
		return "", fieldError("/base_url", "ai.base_url_carries_credentials")
	}
	return raw, nil
}

// modelName trims and bounds a model name. Empty is allowed and means "this provider does not do
// that", which is a real configuration: a local chat model that cannot embed is ordinary.
func modelName(path, raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if len(name) > maxModelName {
		return "", fieldError(path, "ai.model_name_too_long")
	}
	return name, nil
}

// maxOfferedText bounds an offered model's display name and processor, matching the columns.
const maxOfferedText = 200

// NewOfferedAiProviderInput is what offering a model to every workspace needs: a workspace row's
// configuration, without a workspace and without consent, plus what each workspace is shown.
type NewOfferedAiProviderInput struct {
	ID              shared.ID
	DisplayName     string
	Processor       string
	Kind            AiProviderKind
	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
	Jurisdiction    AiJurisdiction
	Now             time.Time
}

// NewOfferedAiProvider validates a model the installation offers (ADR-0072 §1).
//
// The configuration is checked by the same rules as a workspace's own, through NewAiProvider, so
// that an offered model cannot be one a workspace would have been refused. Three things differ:
// a workspace is shown a name and a processor, so both are required; offering NOOP offers nothing;
// and consent is never set here, because switching an offered model on is each workspace's own
// act (ADR-0072 §3) - the row carries `processing_allowed = false` for good.
func NewOfferedAiProvider(in NewOfferedAiProviderInput) (AiProvider, error) {
	if in.ID.IsZero() {
		return AiProvider{}, shared.ErrInternal.WithDetail("ai.provider_incomplete")
	}
	name := strings.TrimSpace(in.DisplayName)
	switch {
	case name == "":
		return AiProvider{}, fieldError("/display_name", "ai.display_name_required")
	case len(name) > maxOfferedText:
		return AiProvider{}, fieldError("/display_name", "ai.display_name_too_long")
	}
	processor := strings.TrimSpace(in.Processor)
	switch {
	case processor == "":
		return AiProvider{}, fieldError("/processor", "ai.processor_required")
	case len(processor) > maxOfferedText:
		return AiProvider{}, fieldError("/processor", "ai.processor_too_long")
	}
	if in.Kind == AiNoop {
		return AiProvider{}, fieldError("/kind", "ai.offered_needs_a_model")
	}

	// The workspace-level rules, run with a placeholder workspace: NewAiProvider asks for one, and
	// the rules it applies do not depend on which.
	checked, err := NewAiProvider(NewAiProviderInput{
		TenantID: in.ID, Kind: in.Kind, BaseURL: in.BaseURL,
		CompletionModel: in.CompletionModel, EmbeddingModel: in.EmbeddingModel,
		Jurisdiction: in.Jurisdiction, Now: in.Now,
	})
	if err != nil {
		return AiProvider{}, err
	}
	checked.ID = in.ID
	checked.TenantID = ""
	checked.DisplayName = name
	checked.Processor = processor
	return checked, nil
}
