// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/google/uuid"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Connecting a provider from a signed-in session (ADR-0078 §1): the two halves, as the provider
// step-up's are, on the caller's own account.

func connectedIdentityResponse(out usecase.Output) openapi.ConnectedIdentity {
	providerID, _ := uuid.Parse(out.String("provider_id"))
	return openapi.ConnectedIdentity{
		ProviderId:   providerID,
		ProviderName: out.String("provider_name"),
		LinkedAt:     timeValue(out["linked_at"]),
	}
}
