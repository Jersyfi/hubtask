// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	forgeservice "github.com/Jersyfi/hubtask/core/application/service/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// CodeDeliverer takes a forge's delivery to a hook in.
type CodeDeliverer interface {
	Deliver(ctx context.Context, d forgeservice.Delivery) (forgeservice.Receipt, error)
}

func firstOf(values ...*string) string {
	for _, v := range values {
		if v != nil && *v != "" {
			return *v
		}
	}
	return ""
}

// DeliverCode answers POST /code-intake/{tenantId}/{hookId}. It reads the body whole - the request
// middleware bounds it - and passes the forge's headers on as they came: whether they prove the
// hook's secret is decided inwards.
func (c *RestController) DeliverCode(
	w http.ResponseWriter, r *http.Request, tenantID, hookID openapi_types.UUID, params openapi.DeliverCodeParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.CodeIntake == nil {
		c.pending.DeliverCode(w, r, tenantID, hookID, params)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	receipt, err := c.CodeIntake.Deliver(r.Context(), forgeservice.Delivery{
		TenantID: shared.ID(tenantID.String()), HookID: shared.ID(hookID.String()), Body: raw,
		Event:      firstOf(params.XGitHubEvent, params.XGitlabEvent, params.XForgejoEvent, params.XGiteaEvent),
		Signature:  firstOf(params.XHubSignature256, params.XForgejoSignature, params.XGiteaSignature),
		Token:      firstOf(params.XGitlabToken),
		DeliveryID: firstOf(params.XGitHubDelivery, params.XGitlabEventUUID, params.XForgejoDelivery, params.XGiteaDelivery),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusAccepted, openapi.CodeDeliveryReceipt{
		Received: receipt.Received, Linked: receipt.Linked, Unknown: receipt.Unknown,
	})
}
