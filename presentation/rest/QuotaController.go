// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The quota surface's use case names (H-08).
const (
	readQuotasUseCase         = "ReadQuotas"
	updateTenantQuotasUseCase = "UpdateTenantQuotas"
)

// ReadQuotas answers GET /quotas. Written out longhand for ListServiceAccounts' reason.
func (c *RestController) ReadQuotas(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), readQuotasUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, quotaStandingsResponse(out))
}
