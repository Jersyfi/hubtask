// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/base64"
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

// CmdbInventoryDeliverer takes a post to an inventory intake in.
type CmdbInventoryDeliverer interface {
	Deliver(ctx context.Context, d cmdbservice.AlertDelivery) (cmdbservice.InventoryReceipt, error)
}

// DeliverCiAlerts answers POST /cmdb/alert-intake/{tenantId}/{connectorId}. It reads the body
// whole - the request middleware bounds it - and passes the presented credential on as it came:
// whether it is the intake's is decided inwards.
func (c *IntakeDoor) DeliverCiAlerts(
	w http.ResponseWriter, r *http.Request, tenantID, connectorID openapi_types.UUID, params openapi.DeliverCiAlertsParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.CmdbAlerts == nil {
		pending{}.DeliverCiAlerts(w, r, tenantID, connectorID, params)
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
	d.Bearer = presentedSecret(r)
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

// presentedSecret is an intake's secret as the request carries it: a bearer token, or the
// password of basic authentication for a program that knows no other way. Whether it is the
// intake's is decided inwards.
func presentedSecret(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if bearer, ok := strings.CutPrefix(header, "Bearer "); ok {
		return strings.TrimSpace(bearer)
	}
	if encoded, ok := strings.CutPrefix(header, "Basic "); ok {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encoded))
		if err != nil {
			return ""
		}
		if _, password, ok := strings.Cut(string(raw), ":"); ok {
			return password
		}
	}
	return ""
}

// DeliverCiInventory answers POST /cmdb/inventory-intake/{tenantId}/{connectorId}, the server
// address a GLPI Agent is given. A post with no credential at all is challenged for basic
// authentication whatever the address names - the agent sends its password only when asked, and
// a challenge that depended on the intake would tell a prober which exist.
func (c *IntakeDoor) DeliverCiInventory(
	w http.ResponseWriter, r *http.Request, tenantID, connectorID openapi_types.UUID, params openapi.DeliverCiInventoryParams,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.CmdbInventory == nil {
		pending{}.DeliverCiInventory(w, r, tenantID, connectorID, params)
		return
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		WriteProblem(w, shared.ErrMalformedRequest.WithDetail("request.body_unreadable").WithCause(err), requestID)
		return
	}
	d := cmdbservice.AlertDelivery{
		TenantID: shared.ID(tenantID.String()), ConnectorID: shared.ID(connectorID.String()), Body: raw,
		Bearer: presentedSecret(r),
	}
	if params.XHubtaskSignature != nil {
		d.Signature = *params.XHubtaskSignature
	}
	if params.XHubtaskTimestamp != nil {
		d.Timestamp = *params.XHubtaskTimestamp
	}
	if d.Bearer == "" && d.Signature == "" {
		writeBasicChallenge(w, shared.ErrUnauthenticated.WithDetail("cmdb.intake_credential_missing"), requestID)
		return
	}
	receipt, err := c.CmdbInventory.Deliver(r.Context(), d)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	out := openapi.CiInventoryReceipt{Status: "ok"}
	if receipt.Contact {
		expiration := inventoryContactHours
		out.Expiration = &expiration
		out.Tasks = &map[string]any{"inventory": map[string]any{"server": "glpi", "version": "1.0"}}
	} else {
		k := receipt.Run.Counts
		out.Counts = &openapi.CiRunCounts{
			Items: k.Items, Created: k.Created, Updated: k.Updated, Unchanged: k.Unchanged, Held: k.Held,
			Duplicates: k.Duplicates, Refused: k.Refused,
		}
		out.RunId = optionalUUID(receipt.Run.ID.String())
	}
	writeJSON(w, r, http.StatusOK, out)
}

// inventoryContactHours is how long an agent waits before its next contact: GLPI's own default,
// which an agent's own schedule overrides.
const inventoryContactHours = 24
