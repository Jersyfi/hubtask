// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package ai

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	port "github.com/Jersyfi/hubtask/core/port/ai"
)

// A source the rule no longer allows refuses with its own reason, still as an unavailable
// dependency: every caller carries on without AI, and a person is told why.
func TestARefusedSourceSaysWhyAndIsStillUnavailable(t *testing.T) {
	_, err := Refused{Detail: "ai.source_not_allowed"}.Complete(context.Background(), port.CompletionRequest{})
	if !errors.Is(err, shared.ErrUnavailable) || shared.AsError(err).DetailCode != "ai.source_not_allowed" {
		t.Errorf("answered %v", err)
	}
	if _, err := (Refused{Detail: "ai.unavailable"}).Embed(context.Background(), nil); !errors.Is(err, port.ErrUnavailable) {
		t.Errorf("an unavailable source answered %v", err)
	}
	if (Refused{}).Capabilities().Enabled() {
		t.Error("a refused source claims a capability")
	}
}

// A caller that short-cuts on Capabilities - Suggest, before it queues anything - asks the provider
// for its refusal rather than naming one, so the source's own reason reaches the person.
func TestARefusedSourceSaysWhyBeforeAnyCall(t *testing.T) {
	err := port.RefusalOf(Refused{Detail: "ai.source_not_allowed"})
	if !errors.Is(err, shared.ErrUnavailable) || shared.AsError(err).DetailCode != "ai.source_not_allowed" {
		t.Errorf("the refusal before any call is %v", err)
	}
	if err := port.RefusalOf(Refused{}); !errors.Is(err, port.ErrUnavailable) ||
		shared.AsError(err).DetailCode != "ai.unavailable" {
		t.Errorf("a refusal without a reason is %v", err)
	}
}

type embedder struct{ Noop }

func (embedder) Capabilities() port.ProviderCapabilities {
	return port.ProviderCapabilities{Kind: "test", Embedding: true, EmbeddingModel: "embed", Completion: true}
}

func (embedder) Embed(context.Context, []string) (port.EmbeddingResult, error) {
	return port.EmbeddingResult{Model: "embed", Vectors: [][]float32{{1}}}, nil
}

// An offered model's vectors are stored under its own space, so an own model of the same name is
// never compared with them; the wire call is the adapter's.
func TestASpacedProviderNamesItsOwnSpace(t *testing.T) {
	spaced := Spaced{Provider: embedder{}, Space: "offered:x/embed"}
	if spaced.Capabilities().EmbeddingModel != "offered:x/embed" || !spaced.Capabilities().Completion {
		t.Errorf("capabilities %+v", spaced.Capabilities())
	}
	result, err := spaced.Embed(context.Background(), []string{"a"})
	if err != nil || result.Model != "offered:x/embed" || len(result.Vectors) != 1 {
		t.Errorf("embedded %+v (%v)", result, err)
	}
	if width, err := spaced.MeasureEmbedding(context.Background()); width != 0 || err != nil {
		t.Errorf("an adapter that cannot measure answered %d, %v", width, err)
	}
}
