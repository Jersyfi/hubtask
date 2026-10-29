// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// InventoryDocument is one post to an inventory intake as its adapter read it: an agent asking
// what to do, or the machine it describes as a payload.
type InventoryDocument struct {
	// Contact is an agent's first request of a session, which carries no inventory.
	Contact bool
	Payload domain.Payload
}

// InventoryParser translates an agent's post into a document; the format is the adapter's.
type InventoryParser func(kind domain.ConnectorKind, body []byte) (InventoryDocument, error)

// InventoryReceipt is what a post did: nothing for a contact, otherwise its run.
type InventoryReceipt struct {
	Contact bool
	Run     domain.Run
}

// DeliverInventory authenticates a post to an inventory intake by the intake's sealed secret, as
// an alert delivery is, and reconciles the machine it describes as the connector's source - the
// one reconciliation every source writes through (research itsm-cmdb-ai §4.6, GLPI's agent). The
// run is the connector's last, as a pull's is.
func (s Connectors) DeliverInventory(ctx context.Context, d AlertDelivery, parse InventoryParser) (InventoryReceipt, error) {
	c, actor, err := s.intake(ctx, d, true)
	if err != nil {
		return InventoryReceipt{}, err
	}
	doc, err := parse(c.Kind, d.Body)
	if err != nil {
		return InventoryReceipt{}, err
	}
	if doc.Contact {
		return InventoryReceipt{Contact: true}, nil
	}
	if err := s.Inventory.authorizeHub(ctx, actor, c.HubID, true, ReconciledAction, connectorTarget, c.ID); err != nil {
		// The person it runs as may no longer write the hub: refused as any other intake reason,
		// and kept on the connector for the administrator.
		return InventoryReceipt{}, s.refusedRun(ctx, d.TenantID, c)
	}
	run, err := s.reconcilePage(ctx, actor, c, doc.Payload, false)
	if err != nil {
		return InventoryReceipt{}, err
	}
	outcome := domain.ConnectorOK
	if run.Counts.Items == 0 {
		outcome = domain.ConnectorEmpty
	}
	next := c.Ran(s.Inventory.Clock.Now(), run.ID, outcome, "", c.Cursor, nil)
	err = s.Inventory.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := s.Store.RecordRun(ctx, next); err != nil {
			return err
		}
		return s.Inventory.record(ctx, actor, ConnectorRanAction, connectorTarget, c.ID,
			audit.Change{Field: "outcome", Classification: audit.Open, To: string(outcome)},
			audit.Change{Field: "items", Classification: audit.Open, To: run.Counts.Items})
	})
	if err != nil {
		return InventoryReceipt{}, err
	}
	return InventoryReceipt{Run: run}, nil
}

func (s Connectors) refusedRun(ctx context.Context, tenantID shared.ID, c domain.Connector) error {
	next := c.Ran(s.Inventory.Clock.Now(), "", domain.ConnectorFailed, "cmdb.connector_run_as_refused", c.Cursor, nil)
	if err := s.Inventory.UnitOfWork.Within(ctx, appshared.ActorContext{TenantID: tenantID}.PersistenceScope(), func(ctx context.Context) error {
		return s.Store.RecordRun(ctx, next)
	}); err != nil {
		return err
	}
	return intakeNotFound()
}
