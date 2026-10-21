// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The reads a person meets before asking AI anything: which entries look like the title being
// typed (UC-AI-04 check 7).
const findDuplicatesByTitleUseCase = "FindDuplicatesByTitle"

// FindDuplicatesByTitle answers POST /items:duplicates.
//
// A read sent as a POST for the reason `/search` is one: no idempotency key is read, and the answer
// is marked not to be stored by anything between the server and the person typing, because it is
// made of their words and their entries' titles.
func (c *RestController) FindDuplicatesByTitle(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.DuplicatesByTitleQuery
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), findDuplicatesByTitleUseCase, actorOf(r),
		usecase.Input{"title": body.Title})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	items, _ := out["items"].([]usecase.Output)
	if items == nil {
		items = []usecase.Output{}
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, r, http.StatusOK, map[string]any{"items": items})
}

const readAiBudgetStateUseCase = "ReadAiBudgetState"

// ReadAiBudgetState answers GET /ai-budget-state: how today's AI budget stands, for whoever is
// about to ask (UC-AI-04 check 9).
func (c *RestController) ReadAiBudgetState(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	out, err := c.UseCases.Invoke(r.Context(), readAiBudgetStateUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, map[string]any{"state": out["state"], "renews_at": out["renews_at"]})
}
