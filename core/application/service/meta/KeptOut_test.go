// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package meta

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

var keptPerson = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

type keptReader struct{ kept integration.KeptOut }

func (k keptReader) Read(context.Context, shared.ID) (integration.KeptOut, error) { return k.kept, nil }

// UC-PRV-05 check 7, UC-PRV-04 check 5 and P-05: a person who keeps their own content out, or
// whose processing is restricted, is offered no AI action, so both features read false for them -
// and only for them.
func TestAKeptOutCallerIsOfferedNoAiAction(t *testing.T) {
	for _, reason := range []integration.KeptOutReason{integration.KeptOutObjected, integration.KeptOutRestricted} {
		t.Run(string(reason), func(t *testing.T) {
			handler, _ := aiManifest(true, aiprovider.ProviderCapabilities{
				Kind: "openai_compatible", Completion: true, Embedding: true,
				CompletionModel: "m", EmbeddingModel: "e", EmbeddingDimensions: 3,
			})
			handler.Kept = keptReader{integration.NewKeptOut([]integration.KeptPerson{
				{AccountID: keptPerson, Reason: reason},
			})}

			kept := member()
			kept.AccountID = keptPerson
			capabilities, err := handler.Execute(t.Context(), kept)
			if err != nil {
				t.Fatalf("execute failed: %v", err)
			}
			if capabilities.Features["ai_suggestions"] || capabilities.Features["semantic_search"] {
				t.Errorf("a kept-out person is offered AI: %v", capabilities.Features)
			}

			colleague := member()
			colleague.AccountID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
			capabilities, err = handler.Execute(t.Context(), colleague)
			if err != nil {
				t.Fatalf("execute failed: %v", err)
			}
			if !capabilities.Features["ai_suggestions"] || !capabilities.Features["semantic_search"] {
				t.Errorf("a colleague lost AI because somebody else is kept out: %v", capabilities.Features)
			}
		})
	}
}
