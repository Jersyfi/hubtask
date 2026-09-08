// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/clock"
	httpport "github.com/Jersyfi/hubtask/core/port/httpclient"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// OpenAiCompatibleKind is this adapter's name in the metric label and in the health report.
const OpenAiCompatibleKind = "openai_compatible"

// targetClass is what the guarded client's duration histogram labels this call. A class, never a
// host: the endpoint is a tenant's configuration, and a label per endpoint would grow a series per
// customer (observability-reliability.md §3.2, rule 10).
const targetClass = "ai"

// OpenAiCompatible speaks the wire format OpenAI, Azure, Mistral, vLLM and LiteLLM share
// (ai-first.md §2, ADR-0049).
//
// Two endpoints wide, which is the whole reason it brings no dependency: `POST
// /v1/chat/completions` and `POST /v1/embeddings`, JSON in and JSON out. What an SDK would add on
// top of the guarded client this project already mandates is a typed struct and a retry policy
// that would have to be switched off (ADR-0049 decision 1).
//
// It holds a configuration rather than being one. A provider is per tenant, so an adapter that was
// a singleton would either serve one workspace or hold a map - and the second is a cache with an
// invalidation problem attached to a credential. The resolver builds one of these per call
// instead; it is a struct of strings and a shared HTTP port, so building it costs nothing.
type OpenAiCompatible struct {
	// Client is the one way out of this process (rule 6). Shared, and the only thing here that is.
	Client httpport.Port
	// Breaker guards this endpoint. Never nil in production; a nil one means "no breaker", which
	// is what a test that is not about the breaker wants.
	Breaker Breaker
	// Clock stamps the result, so that provenance does not depend on the caller remembering to.
	Clock clock.Clock
	// Meter records what the call cost. Never nil - NewOpenAiCompatible substitutes a no-op.
	Meter Meter

	BaseURL         string
	APIKey          secret.Secret
	CompletionModel string
	EmbeddingModel  string
}

// Breaker is the slice of the circuit breaker this adapter needs. A local interface rather than an
// import of the resilience adapter, because adapters do not know each other
// (project-structure.md §2).
type Breaker interface {
	Do(ctx context.Context, call func(context.Context) error) error
}

// Meter is what the adapter reports. Counts, durations and model names - never a token, a note or
// anything a person wrote (rule 10). A local interface for the reason Breaker is one.
type Meter interface {
	// AiCall records one finished call: the provider kind, the operation, and the result as the
	// domain's own category vocabulary in lower case.
	AiCall(ctx context.Context, kind, operation, result string)
	// AiTokens records what a call consumed. Zero where a provider reports nothing, which some do.
	AiTokens(ctx context.Context, kind, operation string, input, output int)
}

var _ port.Provider = OpenAiCompatible{}

// Capabilities answers from configuration rather than by calling anybody: it is read on every
// health scrape and on every capability manifest.
func (p OpenAiCompatible) Capabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{
		Kind:            OpenAiCompatibleKind,
		Completion:      p.CompletionModel != "",
		Embedding:       p.EmbeddingModel != "",
		CompletionModel: p.CompletionModel,
		EmbeddingModel:  p.EmbeddingModel,
	}
}

// The wire shapes. Only the fields this adapter sends and reads: a struct that mirrored the whole
// API would be a maintenance burden for fields nothing uses.

type chatRequest struct {
	Model     string        `json:"model"`
	Messages  []chatMessage `json:"messages"`
	MaxTokens int           `json:"max_completion_tokens,omitempty"`
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message chatMessage `json:"message"`
	} `json:"choices"`
	Usage usage `json:"usage"`
}

type embeddingRequest struct {
	Model string   `json:"model"`
	Input []string `json:"input"`
}

type embeddingResponse struct {
	Model string `json:"model"`
	Data  []struct {
		Index     int       `json:"index"`
		Embedding []float32 `json:"embedding"`
	} `json:"data"`
	Usage usage `json:"usage"`
}

type usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// Complete asks the model one already-rendered question.
func (p OpenAiCompatible) Complete(
	ctx context.Context, request port.CompletionRequest,
) (port.CompletionResult, error) {
	if p.CompletionModel == "" {
		return port.CompletionResult{}, port.ErrUnavailable
	}

	messages := make([]chatMessage, 0, len(request.Messages))
	for _, message := range request.Messages {
		// The roles travel as they arrived. An adapter that flattened them into one string would
		// undo the distinction Prompt.Ask exists to create (ai-first.md §1.3).
		messages = append(messages, chatMessage{Role: string(message.Role), Content: message.Content})
	}

	var answer chatResponse
	err := p.call(ctx, "complete", "/chat/completions", chatRequest{
		Model: p.CompletionModel, Messages: messages, MaxTokens: request.MaxOutputTokens,
	}, &answer)
	if err != nil {
		return port.CompletionResult{}, err
	}
	if len(answer.Choices) == 0 {
		return port.CompletionResult{}, port.ErrUnavailable.
			WithCause(errors.New("the provider answered no choices"))
	}
	p.Meter.AiTokens(ctx, OpenAiCompatibleKind, "complete",
		answer.Usage.PromptTokens, answer.Usage.CompletionTokens)

	return port.CompletionResult{
		Text: answer.Choices[0].Message.Content,
		// The model the provider says answered, not the one configured. They differ whenever a
		// provider resolves an alias, and a suggestion has to record what actually answered it.
		Model:         firstNonEmpty(answer.Model, p.CompletionModel),
		PromptID:      request.PromptID,
		PromptVersion: request.PromptVersion,
		ProducedAt:    p.Clock.Now().UTC(),
		Usage: port.Usage{
			InputTokens: answer.Usage.PromptTokens, OutputTokens: answer.Usage.CompletionTokens,
		},
	}, nil
}

// Embed turns texts into vectors, in order.
func (p OpenAiCompatible) Embed(
	ctx context.Context, texts []string,
) (port.EmbeddingResult, error) {
	if p.EmbeddingModel == "" {
		return port.EmbeddingResult{}, port.ErrUnavailable
	}
	// A batch with nothing in it is not an error: answering one would make every caller check
	// first, and a provider asked for no embeddings would refuse anyway.
	if len(texts) == 0 {
		return port.EmbeddingResult{Model: p.EmbeddingModel, ProducedAt: p.Clock.Now().UTC()}, nil
	}

	var answer embeddingResponse
	err := p.call(ctx, "embed", "/embeddings", embeddingRequest{
		Model: p.EmbeddingModel, Input: texts,
	}, &answer)
	if err != nil {
		return port.EmbeddingResult{}, err
	}
	if len(answer.Data) != len(texts) {
		// Vectors are matched to texts by position, so a short answer is not a partial success -
		// it is an answer nobody can align, and storing it would attach one entry's meaning to
		// another's row.
		return port.EmbeddingResult{}, port.ErrUnavailable.WithCause(fmt.Errorf(
			"the provider answered %d vectors for %d texts", len(answer.Data), len(texts)))
	}

	vectors := make([][]float32, len(texts))
	dimensions := 0
	for _, entry := range answer.Data {
		if entry.Index < 0 || entry.Index >= len(vectors) {
			return port.EmbeddingResult{}, port.ErrUnavailable.
				WithCause(fmt.Errorf("the provider indexed a vector at %d", entry.Index))
		}
		if dimensions == 0 {
			dimensions = len(entry.Embedding)
		} else if len(entry.Embedding) != dimensions {
			// One batch, one geometry. A batch of mixed widths cannot go into one index, and
			// noticing it here is cheaper than noticing it as a search that ranks by nothing.
			return port.EmbeddingResult{}, port.ErrUnavailable.
				WithCause(errors.New("the provider answered vectors of differing lengths"))
		}
		vectors[entry.Index] = entry.Embedding
	}
	p.Meter.AiTokens(ctx, OpenAiCompatibleKind, "embed", answer.Usage.PromptTokens, 0)

	return port.EmbeddingResult{
		Vectors:    vectors,
		Model:      firstNonEmpty(answer.Model, p.EmbeddingModel),
		Dimensions: dimensions,
		ProducedAt: p.Clock.Now().UTC(),
		Usage:      port.Usage{InputTokens: answer.Usage.PromptTokens},
	}, nil
}

// call is the one path out, so that the breaker, the metric and the error shape are decided once.
//
// Every failure becomes ErrUnavailable with the transport error as its cause. That is the port's
// contract and it is a security rule as much as an ergonomic one: a raw transport error can carry
// the URL it failed against, and a URL can carry a key (security.md §9, T-18). The cause is kept
// for the log, where the redacting logger handles it, and never reaches a caller's answer.
func (p OpenAiCompatible) call(
	ctx context.Context, operation, path string, body, into any,
) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		p.Meter.AiCall(ctx, OpenAiCompatibleKind, operation, "internal")
		return shared.ErrInternal.WithCause(fmt.Errorf("encoding the %s request: %w", operation, err))
	}

	header := map[string][]string{"Content-Type": {"application/json"}}
	if !p.APIKey.IsEmpty() {
		// The credential is a header and never the URL, which is why the domain refuses an
		// endpoint carrying user info: two places for one secret is one place too many.
		header["Authorization"] = []string{"Bearer " + p.APIKey.Reveal()}
	}

	run := func(ctx context.Context) error {
		response, err := p.Client.Do(ctx, httpport.Request{
			Method: http.MethodPost, URL: strings.TrimSuffix(p.BaseURL, "/") + path,
			Header: header, Body: encoded, TargetClass: targetClass,
		})
		if err != nil {
			return port.ErrUnavailable.WithCause(fmt.Errorf("calling the provider: %w", err))
		}
		if response.Status < 200 || response.Status > 299 {
			// The body is deliberately not read into the error. A provider's error document is
			// somebody else's text and may quote the request, which is a person's note.
			return port.ErrUnavailable.
				WithCause(fmt.Errorf("the provider answered %d", response.Status))
		}
		if err := json.Unmarshal(response.Body, into); err != nil {
			return port.ErrUnavailable.
				WithCause(fmt.Errorf("decoding the %s answer: %w", operation, err))
		}
		return nil
	}

	if p.Breaker != nil {
		err = p.Breaker.Do(ctx, run)
	} else {
		err = run(ctx)
	}

	p.Meter.AiCall(ctx, OpenAiCompatibleKind, operation, resultOf(err))
	if err == nil {
		return nil
	}
	// A breaker that refused, a timeout and a refused key are one answer to a caller: carry on
	// without it. Which of them it was is the cause, in the log, where it is a label rather than
	// a contract (core/port/ai).
	if errors.Is(err, shared.ErrUnavailable) {
		return err
	}
	return port.ErrUnavailable.WithCause(err)
}

// resultOf is the metric's `result` label: `ok`, or the domain error's category in lower case,
// which is the vocabulary observability-reliability.md §5 fixes for every use case metric.
func resultOf(err error) string {
	if err == nil {
		return "ok"
	}
	return strings.ToLower(string(shared.AsError(err).Category))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// noMeter is what an adapter built without one reports to. Not nil-checking at four call sites:
// a metric that is optional is a metric somebody forgets to guard.
type noMeter struct{}

func (noMeter) AiCall(context.Context, string, string, string)     {}
func (noMeter) AiTokens(context.Context, string, string, int, int) {}
