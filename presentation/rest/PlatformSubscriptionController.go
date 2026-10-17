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

const (
	updatePlatformSubscriptionUseCase       = "UpdatePlatformSubscription"
	deletePlatformSubscriptionUseCase       = "DeletePlatformSubscription"
	rotatePlatformSubscriptionSecretUseCase = "RotatePlatformSubscriptionSecret"
)

// UpdatePlatformSubscription answers PATCH /admin/subscriptions/{subscriptionId}: a merge patch,
// If-Match as the version, the step-up in its header.
func (c *RestController) UpdatePlatformSubscription(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
	params openapi.UpdatePlatformSubscriptionParams,
) {
	var body openapi.PlatformSubscriptionUpdate
	if err := decodeFrom(r.Body, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{
		"subscription_id": subscriptionID.String(), "step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	}
	if body.TargetUrl != nil {
		in["target_url"] = *body.TargetUrl
	}
	if body.State != nil {
		in["state"] = string(*body.State)
	}
	if params.IfMatch != nil {
		if version, ok := versionFromIfMatch((*string)(params.IfMatch)); ok {
			in["expected_version"] = version
		}
	}
	out, ok := c.read(w, r, updatePlatformSubscriptionUseCase, in)
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, out)
}

// DeletePlatformSubscription answers DELETE /admin/subscriptions/{subscriptionId}.
func (c *RestController) DeletePlatformSubscription(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
) {
	if _, ok := c.read(w, r, deletePlatformSubscriptionUseCase, usecase.Input{"subscription_id": subscriptionID.String()}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RotatePlatformSubscriptionSecret answers POST /admin/subscriptions/{subscriptionId}:rotate-secret.
func (c *RestController) RotatePlatformSubscriptionSecret(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
	params openapi.RotatePlatformSubscriptionSecretParams,
) {
	out, ok := c.read(w, r, rotatePlatformSubscriptionSecretUseCase, usecase.Input{
		"subscription_id": subscriptionID.String(), "step_up_token": stepUpHeaderField(params.XHubtaskStepUp),
	})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}

const (
	listPlatformDeliveriesUseCase        = "ListPlatformDeliveries"
	redeliverPlatformSubscriptionUseCase = "RedeliverPlatformSubscription"
)

// ListPlatformDeliveries answers GET /admin/subscriptions/{subscriptionId}/deliveries.
func (c *RestController) ListPlatformDeliveries(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
	params openapi.ListPlatformDeliveriesParams,
) {
	in := usecase.Input{"subscription_id": subscriptionID.String()}
	if params.Status != nil {
		in["status"] = string(*params.Status)
	}
	if params.Cursor != nil {
		in["cursor"] = *params.Cursor
	}
	if params.Size != nil {
		in["limit"] = *params.Size
	}
	out, ok := c.read(w, r, listPlatformDeliveriesUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, out)
}

// RedeliverPlatformSubscription answers POST /admin/subscriptions/{subscriptionId}:redeliver.
func (c *RestController) RedeliverPlatformSubscription(
	w http.ResponseWriter, r *http.Request, subscriptionID openapi_types.UUID,
) {
	out, ok := c.read(w, r, redeliverPlatformSubscriptionUseCase, usecase.Input{"subscription_id": subscriptionID.String()})
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusAccepted, out)
}
