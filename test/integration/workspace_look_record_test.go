// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	settingservice "github.com/Jersyfi/hubtask/core/application/service/setting"
	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// workspaceMember seeds an account with a role at the workspace itself.
func workspaceMember(ctx context.Context, t *testing.T, tenant shared.ID) shared.ID {
	t.Helper()
	member := seedAccount(ctx, t, tenant)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO membership (id, tenant_id, account_id, scope_type, role)
		 VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
		freshID(t).String(), tenant.String(), member.String()); err != nil {
		t.Fatalf("granting the workspace role: %v", err)
	}
	return member
}

// UC-INS-22 check 7 against a real database: a change of the workspace's look is the workspace's
// `setting` record in its own change log - the switches as they stand, under no container - which a
// member of the workspace receives through the stream and the pull, and a member of another
// workspace never does (ChangeLog.Record and After hold to the transaction's workspace).
func TestTheWorkspacesLookReachesItsMembersAsItsSettingRecord(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	member := streamActor(tenantA, workspaceMember(ctx, t, tenantA))
	stranger := streamActor(tenantB, workspaceMember(ctx, t, tenantB))
	stream := streamFor(ctx, t)
	memberFrom, err := stream.Resume(ctx, member, "")
	if err != nil {
		t.Fatalf("resuming as the member: %v", err)
	}
	strangerFrom, err := stream.Resume(ctx, stranger, "")
	if err != nil {
		t.Fatalf("resuming as the other workspace's member: %v", err)
	}

	fixed := portclock.Fixed(created)
	hybrid, err := clockadapter.NewHybridClock(fixed, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	modules := settingservice.Modules{
		Settings: postgres.SettingRepository{}, Containers: containerRepo(),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: fixed, IDs: clockadapter.NewUUIDv7(fixed),
		Changes: postgres.NewChangeLog(), HLC: hybrid,
	}
	changer := appshared.ActorContext{TenantID: tenantA, AccountID: authorA, Kind: shared.ActorUser}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return modules.AnnounceWorkspace(ctx, changer)
	}); err != nil {
		t.Fatalf("announcing: %v", err)
	}

	isTheRecord := func(record syncservice.Record) bool {
		return record.Entity == settingservice.Entity && record.EntityID == tenantA && record.ContainerID.IsZero()
	}
	batch, err := stream.Next(ctx, member, memberFrom)
	if err != nil {
		t.Fatalf("reading as the member: %v", err)
	}
	var received *syncservice.Record
	for i := range batch.Records {
		if isTheRecord(batch.Records[i]) {
			received = &batch.Records[i]
		}
	}
	if received == nil {
		t.Fatal("the workspace's member was not sent the workspace's record")
	}
	if _, carries := received.Payload["modules"]; !carries || len(received.Payload) != 2 || received.Payload["hub_id"] != nil {
		t.Errorf("the record carries %v, want the workspace's switches alone", received.Payload)
	}

	pull := pullFor(ctx, t)
	page, err := pull.Pull(ctx, member, syncservice.PullRequest{
		DeviceID: freshID(t), Cursor: pull.Encode(memberFrom), Limit: 100,
	})
	if err != nil {
		t.Fatalf("pulling: %v", err)
	}
	pulled := false
	for _, record := range page.Records {
		pulled = pulled || (record.Entity == settingservice.Entity && record.EntityID == tenantA)
	}
	if !pulled {
		t.Error("the workspace's record is not on the member's pull")
	}

	elsewhere, err := stream.Next(ctx, stranger, strangerFrom)
	if err != nil {
		t.Fatalf("reading as the other workspace's member: %v", err)
	}
	for _, record := range elsewhere.Records {
		if record.EntityID == tenantA {
			t.Errorf("another workspace's member was sent %s of this one", record.Entity)
		}
	}
}
