// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"
	"errors"
	"fmt"

	port "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
)

// OllamaKind is this adapter's name in the metric label and in the health report.
const OllamaKind = "ollama"

// Ollama speaks to a model this installation runs itself (ai-first.md §3, ADR-0012).
//
// For a self-hosted product this is not the second-best provider but the intended one: §3's row
// says "local models become the norm in self-hosting" and answers it with "the Ollama adapter from
// day one". It is also the only configuration with nothing to assess under ADR-0018 decision 7 -
// a local model transfers to nobody, which is why the domain fixes its jurisdiction at
// SELF_HOSTED.
//
// A separate adapter rather than the OpenAI-compatible one pointed at Ollama's compatibility
// endpoints, which ADR-0049 option 4 rejected: that surface is a translation layer whose gaps are
// the local models', and `Capabilities()` is precisely how an installation learns that its model
// cannot embed. A compatibility shim answers that question with a runtime error instead.
//
// Two endpoints, like the other one, and the same transport underneath: `POST /api/chat` and
// `POST /api/embed`, with `stream` off because this port answers whole results.
type Ollama struct {
	Client  httpport.Port
	Breaker Breaker
	Clock   clock.Clock
	Meter   Meter

	BaseURL         string
	CompletionModel string
	EmbeddingModel  string
}

var _ port.Provider = Ollama{}

// Capabilities answers from configuration, like every provider's.
//
// A local endpoint serving a chat model that cannot embed is an ordinary, supported configuration
// rather than a misconfiguration, and this is where an installation is told so - the search then
// degrades to lexical with a reason instead of failing (J-10).
func (p Ollama) Capabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{
		Kind:            OllamaKind,
		Completion:      p.CompletionModel != "",
		Embedding:       p.EmbeddingModel != "",
		CompletionModel: p.CompletionModel,
		EmbeddingModel:  p.EmbeddingModel,
	}
}

func (p Ollama) transport() transport {
	meter := p.Meter
	if meter == nil {
		meter = noMeter{}
	}
	// No key: a local endpoint is reached over the installation's own network and Ollama has no
	// credential of its own. An operator who has put one behind a proxy configures the
	// OpenAI-compatible adapter, which is where a key belongs.
	return transport{client: p.Client, breaker: p.Breaker, meter: meter, kind: OllamaKind}
}

// The wire shapes. Only the fields this adapter sends and reads.

type ollamaChatRequest struct {
	Model    string        `json:"model"`
	Messages []chatMessage `json:"messages"`
	// Stream off: this port answers whole results, and a streaming answer would have to be
	// assembled here into the same thing.
	Stream bool `json:"stream"`
	// Options carries what OpenAI puts at the top level. Omitted when nothing is set, so a
	// provider that does not read it is never sent an empty object.
	Options *ollamaOptions `json:"options,omitempty"`
}

type ollamaOptions struct {
	NumPredict int `json:"num_predict,omitempty"`
}

type ollamaChatResponse struct {
	Model   string      `json:"model"`
	Message chatMessage `json:"message"`
	// Ollama's own names for what OpenAI calls prompt and completion tokens.
	PromptEvalCount int `json:"prompt_eval_count"`
	EvalCount       int `json:"eval_count"`
}

type ollamaEmbedRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type ollamaEmbedResponse struct {
	Model string `json:"model"`
	// A list per input, in the order the inputs were sent - Ollama does not index them, so the
	// order *is* the alignment and a short answer is unalignable.
	Embeddings      [][]float32 `json:"embeddings"`
	PromptEvalCount int         `json:"prompt_eval_count"`
}

// Complete asks the local model one already-rendered question.
func (p Ollama) Complete(
	ctx context.Context, request port.CompletionRequest,
) (port.CompletionResult, error) {
	if p.CompletionModel == "" {
		return port.CompletionResult{}, port.ErrUnavailable
	}

	messages := make([]chatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		// The roles travel as they arrived, for the reason the other adapter's do: flattening
		// them would undo the distinction Prompt.Ask exists to create (ai-first.md §1.3).
		messages = append(messages, chatMessage{Role: string(message.Role), Content: message.Content})
	}

	body := ollamaChatRequest{Model: p.CompletionModel, Messages: messages}
	if request.MaxOutputTokens > 0 {
		body.Options = &ollamaOptions{NumPredict: request.MaxOutputTokens}
	}

	var answer ollamaChatResponse
	if err := p.transport().call(
		ctx, "complete", endpoint(p.BaseURL, "/api/chat"), body, &answer,
	); err != nil {
		return port.CompletionResult{}, err
	}
	if answer.Message.Content == "" {
		return port.CompletionResult{}, port.ErrUnavailable.
			WithCause(errors.New("the provider answered no message"))
	}
	p.transport().meter.AiTokens(ctx, OllamaKind, "complete", answer.PromptEvalCount, answer.EvalCount)

	return port.CompletionResult{
		Text:          answer.Message.Content,
		Model:         firstNonEmpty(answer.Model, p.CompletionModel),
		PromptID:      request.PromptID,
		PromptVersion: request.PromptVersion,
		ProducedAt:    p.Clock.Now().UTC(),
		Usage: port.Usage{
			InputTokens: answer.PromptEvalCount, OutputTokens: answer.EvalCount,
		},
	}, nil
}

// Embed turns texts into vectors, in order.
func (p Ollama) Embed(ctx context.Context, texts []string) (port.EmbeddingResult, error) {
	if p.EmbeddingModel == "" {
		return port.EmbeddingResult{}, port.ErrUnavailable
	}
	if len(texts) == 0 {
		return port.EmbeddingResult{Model: p.EmbeddingModel, ProducedAt: p.Clock.Now().UTC()}, nil
	}

	var answer ollamaEmbedResponse
	if err := p.transport().call(ctx, "embed", endpoint(p.BaseURL, "/api/embed"),
		ollamaEmbedRequest{Model: p.EmbeddingModel, Input: texts}, &answer); err != nil {
		return port.EmbeddingResult{}, err
	}
	if len(answer.Embeddings) != len(texts) {
		// The order is the alignment here - there is no index to fall back on - so a short answer
		// is not a partial success but one nobody can match to its text.
		return port.EmbeddingResult{}, port.ErrUnavailable.WithCause(fmt.Errorf(
			"the provider answered %d vectors for %d texts", len(answer.Embeddings), len(texts)))
	}

	dimensions := 0
	for _, vector := range answer.Embeddings {
		if dimensions == 0 {
			dimensions = len(vector)
		} else if len(vector) != dimensions {
			// One batch, one geometry: a mixture cannot go into one index (J-09).
			return port.EmbeddingResult{}, port.ErrUnavailable.
				WithCause(errors.New("the provider answered vectors of differing lengths"))
		}
	}
	p.transport().meter.AiTokens(ctx, OllamaKind, "embed", answer.PromptEvalCount, 0)

	return port.EmbeddingResult{
		Vectors:    answer.Embeddings,
		Model:      firstNonEmpty(answer.Model, p.EmbeddingModel),
		Dimensions: dimensions,
		ProducedAt: p.Clock.Now().UTC(),
		Usage:      port.Usage{InputTokens: answer.PromptEvalCount},
	}, nil
}
