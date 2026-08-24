// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The handler is detached, and that is a declaration rather than a configuration: the runner reads
// the interface to decide whether to open a transaction around it (queue.Detached).
func TestTheDeliveryHandlerOwnsItsTransactions(t *testing.T) {
	var handler queue.Handler = NotificationDelivery{}
	if _, detached := handler.(queue.Detached); !detached {
		t.Error("the delivery would run inside the runner's transaction, holding a database " +
			"connection open across an SMTP conversation")
	}
}

func TestAJobWithoutATenantIsRefused(t *testing.T) {
	_, err := NotificationDelivery{}.Run(t.Context(), queue.Job{
		Kind: queue.KindNotificationDeliver,
		Payload: map[string]any{
			"notification_id": "01936f2a-7c1e-7000-8000-0000000000e1",
		},
	})
	if err == nil {
		t.Fatal("a job naming no workspace was accepted")
	}
	if got := shared.AsError(err).DetailCode; got != "notifications.job_without_tenant" {
		t.Errorf("detail %q", got)
	}
}

// A payload is data that outlived the process that wrote it, so what is not an identifier is a
// defect rather than something to work around - and it is parsed rather than trusted.
func TestAMalformedPayloadIsRefusedBeforeAnyWork(t *testing.T) {
	tenant := shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")

	for _, tc := range []struct {
		name    string
		payload map[string]any
	}{
		{"no identifier at all", map[string]any{}},
		{"a number where an identifier belongs", map[string]any{"notification_id": 42}},
		{"something that is not a UUID", map[string]any{"notification_id": "../../etc/passwd"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NotificationDelivery{}.Run(t.Context(), queue.Job{
				Kind: queue.KindNotificationDeliver, TenantID: tenant, Payload: tc.payload,
			})
			if err == nil {
				t.Fatal("accepted")
			}
			if got := shared.AsError(err).DetailCode; got != "notifications.payload_malformed" {
				t.Errorf("detail %q", got)
			}
		})
	}
}
