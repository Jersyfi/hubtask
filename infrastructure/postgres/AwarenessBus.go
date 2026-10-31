// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package postgres

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres/sqlc"
)

// maxAwarenessPayload keeps a notification under PostgreSQL's 8000 bytes. An awareness message
// that does not fit - base64 of more than about 5.8 KB - stays in its process: a cursor and a name
// are a few hundred bytes, and a larger state is sent again by the editor soon enough.
const maxAwarenessPayload = 7900

// awarenessTimeout bounds one publication (rule 7); awareness is the latest cursor, and a late one
// is worth less than the connection's turn.
const awarenessTimeout = 2 * time.Second

// AwarenessBus relays a live document's awareness to the other processes (offline-sync.md §13,
// SY-E) on `hubtask_awareness`, through the transaction wrapper (rule 3). The listener's origin
// marks the sender, so a process does not hand its own message out twice.
type AwarenessBus struct {
	Work     persistence.UnitOfWork
	Listener *ChangeListener
	// Signals counts each publication's outcome; nil counts nothing.
	Signals AwarenessSignals
}

// AwarenessSignals is where a publication's outcome is counted (`sent`, `too_large`, `failed`).
// Awareness is best effort, so a lost one is never the connection's error; the counter is how an
// operator learns that co-editing across replicas degraded.
type AwarenessSignals interface {
	AwarenessRelayed(ctx context.Context, outcome string)
}

// Publish sends the message to the document's connections on the other processes; one too large
// for a notification is not sent.
func (b AwarenessBus) Publish(ctx context.Context, tenantID shared.ID, kind string, documentID shared.ID, message []byte) error {
	payload := b.Listener.origin + "/" + tenantID.String() + "/" + kind + "/" + documentID.String() + "|" +
		base64.StdEncoding.EncodeToString(message)
	if len(payload) > maxAwarenessPayload {
		b.count(ctx, "too_large")
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, awarenessTimeout)
	defer cancel()
	err := b.Work.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		tx, err := FromContext(ctx)
		if err != nil {
			return err
		}
		return sqlc.New(tx).NotifyAwareness(ctx, payload)
	})
	if err != nil {
		b.count(ctx, "failed")
		return err
	}
	b.count(ctx, "sent")
	return nil
}

func (b AwarenessBus) count(ctx context.Context, outcome string) {
	if b.Signals != nil {
		b.Signals.AwarenessRelayed(ctx, outcome)
	}
}
