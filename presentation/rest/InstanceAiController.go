// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The AI models the installation offers (ADR-0072 §1), over REST. The operator's view: everything
// but the key, which no call answers.

func offeredAiModelResponse(out usecase.Output) openapi.OfferedAiModel {
	answer := openapi.OfferedAiModel{
		DisplayName:  out.String("display_name"),
		Processor:    out.String("processor"),
		Kind:         openapi.AiProviderKind(out.String("kind")),
		Jurisdiction: openapi.AiJurisdiction(out.String("jurisdiction")),
	}
	answer.Id = uuidValue(out.String("id"))
	if baseURL := out.String("base_url"); baseURL != "" {
		answer.BaseUrl = &baseURL
	}
	if model := out.String("completion_model"); model != "" {
		answer.CompletionModel = &model
	}
	if model := out.String("embedding_model"); model != "" {
		answer.EmbeddingModel = &model
	}
	if held, present := out["has_api_key"].(bool); present {
		answer.HasApiKey = held
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
