// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ConnectorScheduler runs one connector's scheduled pull and answers when it is owed next.
type ConnectorScheduler interface {
	RunScheduled(ctx context.Context, tenantID, connectorID shared.ID) (time.Time, error)
}

// CmdbConnectorPull is one connector's scheduled pull (research itsm-cmdb-ai C2): one job per
// connector, deduplicated by its identifier, rescheduling itself to the moment the connector is
// owed next and finishing when it is switched off or unscheduled - the next definition seeds it.
//
// Detached: a pull calls the source between its pages, and no transaction stays open across a
// call; each page commits in its own.
type CmdbConnectorPull struct {
	Connectors ConnectorScheduler
	// Now is the clock the wait is measured on; nil is the system's.
	Now func() time.Time
}

var (
	_ queue.Handler  = CmdbConnectorPull{}
	_ queue.Detached = CmdbConnectorPull{}
)

// OwnsItsTransactions marks the handler detached.
func (CmdbConnectorPull) OwnsItsTransactions() {}

// Run pulls the connector the job names, when it is due.
func (h CmdbConnectorPull) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	raw, _ := job.Payload["connector_id"].(string)
	id, err := shared.ParseID(raw)
	if job.TenantID.IsZero() || err != nil {
		return queue.Result{}, shared.ErrInternal.WithDetail("cmdb.connector_pull_without_connector")
	}
	next, err := h.Connectors.RunScheduled(ctx, job.TenantID, id)
	if err != nil {
		return queue.Result{}, err
	}
	if next.IsZero() {
		return queue.Result{}, nil
	}
	now := time.Now
	if h.Now != nil {
		now = h.Now
	}
	return queue.Result{Repeat: true, RepeatAfter: waitUntil(now(), next, 7*24*time.Hour)}, nil
}
