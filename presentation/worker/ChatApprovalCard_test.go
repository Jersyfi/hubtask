// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package worker

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

type teller struct{ told []string }

func (t *teller) TellApproval(_ context.Context, tenant, link, approval shared.ID, txn string) error {
	t.told = append(t.told, tenant.String()+"|"+link.String()+"|"+approval.String()+"|"+txn)
	return nil
}

// UC-SVC-09 check 6: the job posts the card it names; one naming no link is an internal error,
// not a card to nobody.
func TestAChatApprovalCardJobPostsTheCardItNames(t *testing.T) {
	tenant, link, approval := shared.ID("00000000-0000-7000-8000-000000000001"), shared.ID("00000000-0000-7000-8000-000000000002"), shared.ID("00000000-0000-7000-8000-000000000003")
	bots := &teller{}
	h := ChatApprovalCard{Bots: bots}
	if _, err := h.Run(context.Background(), queue.Job{TenantID: tenant, Payload: map[string]any{
		"link_id": link.String(), "approval_id": approval.String(), "txn": "k",
	}}); err != nil || len(bots.told) != 1 || bots.told[0] != tenant.String()+"|"+link.String()+"|"+approval.String()+"|k" {
		t.Fatalf("told = %v %v", bots.told, err)
	}
	if _, err := h.Run(context.Background(), queue.Job{TenantID: tenant, Payload: map[string]any{"approval_id": approval.String()}}); err == nil {
		t.Fatal("a job without a link ran")
	}
}
