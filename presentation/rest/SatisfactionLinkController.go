// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"

	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// SatisfactionLinkDoor answers a resolution mail's satisfaction link (UC-SVC-07 check 6).
type SatisfactionLinkDoor interface {
	Execute(ctx context.Context, token integration.SatisfactionLinkToken, score int) error
}

// AnswerSatisfactionLink answers POST /public/satisfaction/{token}: a requester without an account
// rating their request, with no bearer and nobody signed in. Parsing happens here and nothing
// else; a malformed token answers what an unknown one does (T-21).
func (c *FeedDoor) AnswerSatisfactionLink(w http.ResponseWriter, r *http.Request, presented string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Satisfaction == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := integration.ParseSatisfactionLinkToken(presented)
	if err != nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("satisfaction.link_not_found"), requestID)
		return
	}
	var body openapi.SatisfactionLinkAnswer
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if err := c.Satisfaction.Execute(r.Context(), token, body.Score); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}
