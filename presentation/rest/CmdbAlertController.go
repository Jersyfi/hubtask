// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"io"
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// CmdbAlertDeliverer takes a delivery to an alert intake in.
type CmdbAlertDeliverer interface {
	Deliver(ctx context.Context, d cmdbservice.AlertDelivery) (cmdbservice.AlertReceipt, error)
}

// DeliverCiAlerts answers POST /cmdb/alert-intake/{tenantId}/{connectorId}. It reads the body
// whole - the request middleware bounds it - and passes the presented credential on as it came:
// whether it is the intake's is decided inwards.
func (c *RestController) DeliverCiAlerts(
	w http.ResponseWriter, r *http.Request, tenantID, connectorID openapi_types.UUID, params openapi.DeliverCiAlertsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.CmdbAlerts == nil {
		c.pending.DeliverCiAlerts(w, r, tenantID, connectorID, params)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	d := cmdbservice.AlertDelivery{
		TenantID: shared.ID(tenantID.String()), ConnectorID: shared.ID(connectorID.String()), Body: raw,
	}
	if bearer, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer "); ok {
		d.Bearer = strings.TrimSpace(bearer)
	}
	if params.XHubtaskSignature != nil {
		d.Signature = *params.XHubtaskSignature
	}
	if params.XHubtaskTimestamp != nil {
		d.Timestamp = *params.XHubtaskTimestamp
	}
	receipt, err := c.CmdbAlerts.Deliver(r.Context(), d)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusAccepted, openapi.CiAlertReceipt{
		Received: receipt.Received, Opened: receipt.Opened, Resolved: receipt.Resolved,
		Unchanged: receipt.Unchanged, EntriesFailed: receipt.EntriesFailed,
	})
}
