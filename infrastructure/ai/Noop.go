// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package ai holds the outbound AI adapters (ADR-0012, ADR-0049). This file is the one that calls
// nothing, and it is the default in every mode.
package ai

import (
	"context"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
)

// NoopKind is this adapter's name in the metric label and in the health report.
const NoopKind = "noop"

// Noop is the provider an installation has until somebody configures one (ai-first.md §2).
//
// It refuses. That is the whole design and it is not laziness: a Complete that answered "" and a
// Complete whose model had nothing to say are the same value, and QS-09 is a claim about what an
// installation *tells* you when AI is off - "all core features stay available; AI endpoints respond
// 503". A silent empty answer would satisfy the first half of that sentence and quietly break the
// second, and the failure would surface as a suggestion nobody could explain rather than as a
// configuration nobody made.
//
// It is stateless and has no configuration, so it is a value rather than a constructor's result.
type Noop struct{}

var _ port.Provider = Noop{}

func (Noop) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{}, port.ErrUnavailable
}

func (Noop) Embed(context.Context, []string) (port.EmbeddingResult, error) {
	return port.EmbeddingResult{}, port.ErrUnavailable
}

// Capabilities reports a provider that can do nothing. Enabled() is therefore false, which is what
// the health probe reads to say `disabled` instead of `down`: an installation that configured no
// AI is not one whose AI is broken.
func (Noop) Capabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{Kind: NoopKind}
}

// Refused is Noop with a reason: every call answers ErrUnavailable with the detail the rule gave -
// `ai.source_not_allowed` where the installation no longer allows the source a workspace chose -
// so that "AI is off here" and "this AI is no longer allowed here" are told apart.
type Refused struct{ Detail string }

var _ port.Provider = Refused{}

func (r Refused) err() error {
	if r.Detail == "" || r.Detail == "ai.unavailable" {
		return port.ErrUnavailable
	}
	return shared.ErrUnavailable.WithDetail(r.Detail)
}

func (r Refused) Complete(context.Context, port.CompletionRequest) (port.CompletionResult, error) {
	return port.CompletionResult{}, r.err()
}

func (r Refused) Embed(context.Context, []string) (port.EmbeddingResult, error) {
	return port.EmbeddingResult{}, r.err()
}

func (Refused) Capabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{Kind: NoopKind}
}

// Spaced is an adapter whose vectors are stored under another name than its model's: an offered
// model's, qualified so that a workspace's own model of the same name is a different space
// (ADR-0072 §6). The wire call is the adapter's own; only the name the store keys on changes.
type Spaced struct {
	Provider port.Provider
	Space    string
}

var _ port.Provider = Spaced{}

func (s Spaced) Complete(ctx context.Context, request port.CompletionRequest) (port.CompletionResult, error) {
	return s.Provider.Complete(ctx, request)
}

func (s Spaced) Embed(ctx context.Context, texts []string) (port.EmbeddingResult, error) {
	result, err := s.Provider.Embed(ctx, texts)
	if err == nil && result.Model != "" {
		result.Model = s.Space
	}
	return result, err
}

func (s Spaced) Capabilities() port.ProviderCapabilities {
	capabilities := s.Provider.Capabilities()
	if capabilities.Embedding {
		capabilities.EmbeddingModel = s.Space
	}
	return capabilities
}

// MeasureEmbedding asks the adapter underneath, where it can say.
func (s Spaced) MeasureEmbedding(ctx context.Context) (int, error) {
	if measured, can := s.Provider.(port.Measured); can {
		return measured.MeasureEmbedding(ctx)
	}
	return 0, nil
}
