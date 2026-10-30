// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The AI provider's configuration use cases.
const ()

// aiProviderResponse maps the use case's answer. The API key is not among the fields, because it
// is not among the use case's either - there is no call that answers it.
func aiProviderResponse(out usecase.Output) openapi.AiProvider {
	answer := openapi.AiProvider{
		Kind:         openapi.AiProviderKind(out.String("kind")),
		Jurisdiction: openapi.AiJurisdiction(out.String("jurisdiction")),
	}
	if baseURL := out.String("base_url"); baseURL != "" {
		answer.BaseUrl = &baseURL
	}
	if model := out.String("completion_model"); model != "" {
		answer.CompletionModel = &model
	}
	if model := out.String("embedding_model"); model != "" {
		answer.EmbeddingModel = &model
	}
	// Whether a key is stored. Not the key: there is no call that answers that one.
	if held, present := out["has_api_key"].(bool); present {
		answer.HasApiKey = held
	}
	if allowed, held := out["processing_allowed"].(bool); held {
		answer.ProcessingAllowed = allowed
	}
	if created, held := out["created_at"].(time.Time); held {
		answer.CreatedAt = created
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	if version, held := out["version"].(int); held {
		answer.Version = version
	}
	return answer
}
