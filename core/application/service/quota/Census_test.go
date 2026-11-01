// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package quota

import (
	"context"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

type tenantList []adminrepo.TenantRecord

func (t tenantList) List(context.Context) ([]adminrepo.TenantRecord, error) { return t, nil }

// itemsPerTenant answers the items of whichever workspace the transaction was opened for.
type itemsPerTenant struct {
	usageFake
	uow   *unitOfWork
	items map[shared.ID]int64
}

func (u *itemsPerTenant) Items(context.Context) (int64, error) {
	return u.items[u.uow.scopes[len(u.uow.scopes)-1].TenantID], nil
}

// A-18's input: one workspace at 95% of its items while twenty others create far below it is
// counted as approaching, whatever the last create said; a suspended one is not counted.
func TestTheCensusCountsEveryWorkspaceNearItsCeiling(t *testing.T) {
	near := shared.ID("018f2a1b-0000-7000-8000-0000000000a1")
	walled := shared.ID("018f2a1b-0000-7000-8000-0000000000a2")
	suspended := shared.ID("018f2a1b-0000-7000-8000-0000000000a3")
	tenants := tenantList{
		{ID: near, Status: identity.TenantActive},
		{ID: walled, Status: identity.TenantActive},
		{ID: suspended, Status: identity.TenantSuspended},
	}
	items := map[shared.ID]int64{near: 95, walled: 100, suspended: 100}
	for i := range 20 {
		id := shared.ID("018f2a1b-0000-7000-8000-0000000001" + string(rune('a'+i)) + "0")
		tenants = append(tenants, adminrepo.TenantRecord{ID: id, Status: identity.TenantActive})
		items[id] = 10
	}
	limit := int64(100)
	uow := &unitOfWork{}
	census := Census{
		Tenants: tenants, Store: &storeFake{zone: "UTC", overrides: repository.Overrides{Items: &limit}},
		Usage: &itemsPerTenant{uow: uow, items: items}, UnitOfWork: uow,
		Clock: clock.Fixed(time.Date(2026, 11, 1, 9, 0, 0, 0, time.UTC)),
	}
	bands, listed, err := census.Count(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := bands[Items]; got.Approaching != 2 || got.AtLimit != 1 {
		t.Fatalf("items band %+v, want 2 approaching and 1 at the limit", got)
	}
	if len(listed) != 2 || listed[0] != near || listed[1] != walled {
		t.Fatalf("listed %v, want the two workspaces near the ceiling", listed)
	}
}
