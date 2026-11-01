// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"

	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type nearCensus struct {
	near  []quotaservice.Near
	asked int
}

func (c *nearCensus) Count(context.Context) (map[string]quotaservice.Band, []quotaservice.Near, error) {
	c.asked++
	return nil, c.near, nil
}

// A-18 counts workspaces without naming them; the operator's list names each one and the wall it
// stands at, so the alert has somewhere to go.
func TestTheWorkspacesNearAQuotaAreNamedWithTheirWall(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(operatorID))
	used := int64(95)
	census := &nearCensus{near: []quotaservice.Near{{
		TenantID: shared.ID("018f2a1b-0000-7000-8000-0000000000a1"), Slug: "acme",
		Quotas: []quotaservice.Standing{{Quota: quotaservice.Items, Limit: 100, Used: &used}},
	}}}

	out, err := ListWorkspacesNearQuota{Writer: writer, Census: census}.invoke(t.Context(), operator(), nil)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	rows, _ := out["data"].([]usecase.Output)
	if len(rows) != 1 || rows[0]["tenant_id"] != "018f2a1b-0000-7000-8000-0000000000a1" || rows[0]["slug"] != "acme" {
		t.Fatalf("rows %v, want acme by its identifier", rows)
	}
	quotas, _ := rows[0]["quotas"].([]usecase.Output)
	if len(quotas) != 1 || quotas[0]["quota"] != quotaservice.Items || quotas[0]["used"] != int64(95) ||
		quotas[0]["ratio"] != 0.95 {
		t.Fatalf("quotas %v, want items at 95 of 100", quotas)
	}
}

// The list reads every workspace's standing; a caller outside the register never reaches it.
func TestTheWorkspacesNearAQuotaDemandTheRegister(t *testing.T) {
	writer, _, _ := newInstanceWriter(newRegister(secondOperator))
	census := &nearCensus{}
	if _, err := (ListWorkspacesNearQuota{Writer: writer, Census: census}).
		Execute(t.Context(), operator()); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a reader outside the register answered %v", err)
	}
	if census.asked != 0 {
		t.Error("a refused caller reached the census")
	}
}
