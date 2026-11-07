// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// PublicRoadmapReader is the slice of the public roadmap read this controller needs.
type PublicRoadmapReader interface {
	Execute(ctx context.Context, token integration.RoadmapLinkToken, zone string) (usecase.Output, error)
}

// GetPublicRoadmap answers GET /public/roadmaps/{token}: a portfolio's roadmap opened by its public
// link, with no bearer credential. Parsing happens here and nothing else; every question is asked
// inwards, and every refusal looks the same from out here (T-21).
func (c *FeedDoor) GetPublicRoadmap(w http.ResponseWriter, r *http.Request, presented string, params openapi.GetPublicRoadmapParams) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Roadmaps == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := integration.ParseRoadmapLinkToken(presented)
	if err != nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("portfolios.link_not_found"), requestID)
		return
	}
	zone := ""
	if params.Zone != nil {
		zone = *params.Zone
	}
	road, err := c.Roadmaps.Execute(r.Context(), token, zone)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	writeJSON(w, r, http.StatusOK, road)
}
