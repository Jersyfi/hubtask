// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package ai holds the outbound AI adapters (ADR-0012, ADR-0049). This file is the one that calls
// nothing, and it is the default in every mode.
package ai

import (
	"context"

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
