// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package ai is the outbound half of ADR-0012: the port through which this product may ask a model
// something, and never a dependency of anything inwards of it.
//
// Two rules shape every type here, and both come from ai-first.md rather than from taste.
//
// The product is complete without a provider (QS-09). So the port has a working implementation
// that calls nothing - NoopAi - it is the default in every mode, and it *refuses* rather than
// answering empty: a summary that is the empty string and a summary a model declined to write are
// the same value, and an installation has to be able to tell the difference. Every refusal is
// ErrUnavailable with the detail code ai.unavailable, which is a 503 with a problem document.
//
// Content is data, never instruction (ai-first.md §1.3). A title, a note or a comment reaching a
// model is context that is quoted, not a sentence that is obeyed - which is why a request carries
// Messages with a Role rather than one assembled string. An adapter cannot merge a system
// instruction and a user's note into one blob without deleting the distinction the guardrail is
// made of.
//
// What is deliberately not here: streaming, tool calling, and images. None of 0.7.0's four features
// asks for any of them (ADR-0049), and a port that declares what nothing implements is a promise a
// client would find empty.
package ai

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Dependency is the name this provider has in /meta/health and in the metrics. Stable, short and
// without spaces, the way core/port/health asks - and the same string infrastructure/health has
// been naming in its tests since before there was a provider behind it.
const Dependency = "ai_provider"

// Feature is what a person loses when the provider is gone: suggestions, not "AI"
// (observability-reliability.md §7). The bare feature name the degradation report carries.
const Feature = "ai_suggestions"

// FeatureSemanticSearch is the second thing a provider outage costs: search stops finding entries
// by what they mean and finds them only by the words in them (J-10, ADR-0050). The same name
// /meta/capabilities answers under, so a client reading either learns about one feature.
//
// Named unconditionally, even in an installation whose database has no pgvector and therefore never
// had semantic search to lose. The alternative is a health probe that reads the database to decide
// what to call itself, which is a probe that fails during exactly the outage it exists to report -
// and an installation that cannot search by meaning already answers `semantic_search: false` from
// /meta/capabilities, which is where a client looks before it offers the feature at all. That
// manifest entry needs the store *and* a provider that embeds (issue 502); this one is a name.
const FeatureSemanticSearch = "semantic_search"

// ErrUnavailable is every refusal this port can produce, and there is deliberately only one.
//
// It is what NoopAi answers, what a tenant without ai_processing_allowed answers (J-02), what an
// exhausted budget answers (J-15) and what an open circuit answers (J-03). A caller's correct
// response to all four is the same - carry on without the suggestion - and four codes would invite
// four handlings of one situation. Which of them it was is a matter for the log and the metric,
// where it is a label rather than a contract.
//
// The category is Unavailable, so it reaches the wire as 503 with a problem document, which is the
// shape arc42 QS-09 fixes.
var ErrUnavailable = shared.ErrUnavailable.WithDetail("ai.unavailable")

// Role is who a message in a completion request is speaking as.
//
// The vocabulary is two words on purpose. System is the installation's own instruction, written by
// this project and versioned as a prompt file; User is everything else, including every byte that
// came out of the database. There is no third role an adapter could put content into and no way to
// promote content to an instruction, because that promotion is exactly the attack ai-first.md
// §1.3 forbids.
type Role string

const (
	// RoleSystem is the prompt. It comes from the prompt store and never from a request.
	RoleSystem Role = "system"
	// RoleUser is content: a title, a note, an email in the jumble. Quoted, never obeyed.
	RoleUser Role = "user"
)

// Message is one turn of a completion request.
type Message struct {
	Role    Role
	Content string
}

// CompletionRequest is one question, already rendered from a prompt.
//
// PromptID and PromptVersion travel with it rather than being derived from the messages, because
// they end up in a suggestion's provenance (J-05): "which prompt produced this" has to be
// answerable a year later, when the messages are long gone and the prompt has been rewritten
// twice. They are the store's own coordinates (ADR-0049 decision 3).
type CompletionRequest struct {
	PromptID      string
	PromptVersion string
	Messages      []Message
	// MaxOutputTokens bounds the answer. Zero leaves it to the provider's own default, which is
	// what a local model usually wants and a metered one usually does not.
	MaxOutputTokens int
}

// CompletionResult is the answer with its provenance attached.
//
// Model and ProducedAt are on the result rather than looked up by the caller because the
// configuration can change between the call and the write: a suggestion has to record the model
// that actually answered it, not the one configured when somebody comes to store it.
type CompletionResult struct {
	Text          string
	Model         string
	PromptID      string
	PromptVersion string
	ProducedAt    time.Time
	Usage         Usage
}

// EmbeddingResult is a batch of vectors and the model that produced them.
//
// The model is not decoration. Vectors are only comparable within one model's space, so an
// installation whose embedding model changes has an index holding two incompatible geometries and
// a hybrid search that ranks the mixture by nothing. Recording the model per batch is what lets
// the search notice and re-embed instead of quietly answering wrongly (ADR-0049 decision 4).
type EmbeddingResult struct {
	// Vectors are in the order of the texts that were sent, one per text.
	Vectors [][]float32
	Model   string
	// Dimensions is the length every vector in this batch has. Carried explicitly so a caller can
	// reject a batch that does not match its index without measuring a slice it may not store.
	Dimensions int
	ProducedAt time.Time
	Usage      Usage
}

// Usage is what the call cost, in the only unit every provider reports. It feeds the metric and
// the tenant's budget (J-15) and nothing else - it is not a price, and this project does not
// have one.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// ProviderCapabilities is what this installation's provider can actually do.
//
// It exists because the answer is not a property of the adapter but of the model behind it: an
// Ollama endpoint serving a chat model that cannot embed is an ordinary, supported configuration,
// and the search has to degrade to lexical rather than fail. A value beats an error here - the
// capability manifest can publish it, and a client can render one control fewer instead of showing
// one that will always refuse.
type ProviderCapabilities struct {
	// Kind names the adapter: noop, openai_compatible, ollama. It is a metric label, so the set
	// stays closed and small (observability-reliability.md §3.2).
	Kind string
	// Completion and Embedding are what the configured models support. Both false is NoopAi, and
	// also a provider configured with no model at all.
	Completion bool
	Embedding  bool
	// CompletionModel and EmbeddingModel are the configured names, for the health report and for
	// provenance. Empty where the corresponding capability is false.
	CompletionModel string
	EmbeddingModel  string
	// EmbeddingDimensions is the vector length this provider produces, or zero when it does not
	// embed. The search's index is built for one length, so a change is a re-index rather than a
	// setting (J-09).
	EmbeddingDimensions int
}

// Enabled reports whether this provider can do anything at all. It is what the health probe reads
// to tell "disabled" from "down": an installation that configured nothing is not broken.
func (c ProviderCapabilities) Enabled() bool { return c.Completion || c.Embedding }

// Provider is the port (ADR-0012, ai-first.md §2).
//
// Three methods, and the contract around them is as much of the port as the signatures:
//
//   - Every call honours the context's deadline. There is no timeout parameter, because a call
//     without a deadline is a defect the caller commits rather than a mode the port offers
//     (rule 7).
//   - Every failure is ErrUnavailable, wrapped. An unreachable endpoint, a refused key, a rate
//     limit at the provider and an open circuit are one answer to a caller: carry on without it.
//     A raw transport error must never surface - it can carry a URL, and a URL can carry a key
//     (security.md §9).
//   - No implementation logs, meters or traces the content it was given. Rule 10 is not a
//     guideline here: the request is the one place in this system where user content is handed to
//     something outside it, so the telemetry around it is counts, durations and model names.
type Provider interface {
	// Complete answers one rendered prompt.
	Complete(ctx context.Context, request CompletionRequest) (CompletionResult, error)
	// Embed turns texts into vectors, in order. An empty slice is not an error: it is a batch with
	// nothing in it, and answering an error would make every caller check first.
	Embed(ctx context.Context, texts []string) (EmbeddingResult, error)
	// Capabilities is what this provider can do. It is read on every health scrape and on every
	// capability manifest, so it answers from configuration rather than by calling anybody.
	Capabilities() ProviderCapabilities
}
