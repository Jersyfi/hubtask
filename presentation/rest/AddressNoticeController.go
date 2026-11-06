// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The notice of a change of the workspace's address a member without an email address reads at the
// next sign-in (multi-tenancy.md §3.1).

const (
	listAddressNoticesUseCase        = "ListAddressNotices"
	acknowledgeAddressNoticesUseCase = "AcknowledgeAddressNotices"
)

// ListAddressNotices answers GET /accounts/me/address-notices.
func (c *RestController) ListAddressNotices(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), listAddressNoticesUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	answer := make([]openapi.AddressChangeNotice, 0, len(rows))
	for _, row := range rows {
		answer = append(answer, openapi.AddressChangeNotice{
			From: row.String("from"), To: row.String("to"), ChangedAt: timeValue(row["changed_at"]),
		})
	}
	writeJSON(w, r, http.StatusOK, answer)
}

// AcknowledgeAddressNotices answers POST /accounts/me/address-notices:acknowledge.
func (c *RestController) AcknowledgeAddressNotices(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	if _, err := c.UseCases.Invoke(r.Context(), acknowledgeAddressNoticesUseCase, actorOf(r), usecase.Input{}); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
