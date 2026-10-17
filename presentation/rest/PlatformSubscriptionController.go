// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	listPlatformSubscriptionsUseCase  = "ListPlatformSubscriptions"
	createPlatformSubscriptionUseCase = "CreatePlatformSubscription"
	readPlatformSubscriptionUseCase   = "ReadPlatformSubscription"
)

// ListPlatformSubscriptions answers GET /admin/subscriptions.
func (c *RestController) ListPlatformSubscriptions(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listPlatformSubscriptionsUseCase, usecase.Input{})
	if !ok {
		return
	}
	rows, _ := out["data"].([]usecase.Output)
	if rows == nil {
		rows = []usecase.Output{}
	}
	writeJSON(w, r, http.StatusOK, rows)
}

// CreatePlatformSubscription answers POST /admin/subscriptions, the step-up in its header.
func (c *RestController) CreatePlatformSubscription(
	w http.ResponseWriter, r *http.Request, params openapi.CreatePlatformSubscriptionParams,
) {
	var body openapi.PlatformSubscriptionCreate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, createPlatformSubscriptionUseCase, usecase.Input{
		"target_url": body.TargetUrl, "step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusCreated, out)
}

// ReadPlatformSubscription answers GET /admin/subscriptions/{subscriptionId}.
func (c *RestController) ReadPlatformSubscription(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
) {
	out, ok := c.read(w, r, readPlatformSubscriptionUseCase, usecase.Input{"subscription_id": subscriptionID.String()})
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, out)
}
