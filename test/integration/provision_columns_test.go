// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/text"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// UC-WRK-31 check 1 for the example collection, against the real stores and the resolver every
// reader of a structure uses, asked inside the provisioning transaction in the new tenant's scope:
// the installation's default structure opens the list, so the collection starts without columns,
// and its labels are seeded as before.
func TestAProvisionedExampleCollectionStartsWithoutColumnsUnderTheDefaultStructure(t *testing.T) {
	ctx := context.Background()
	_, signIn, uow := realWriter(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(clockadapter.System{}, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	containers := containerRepo()
	provisioning := adminservice.ProvisionTenant{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Accounts: postgres.NewAccountRepository(), Redemption: signIn,
		Grants:     postgres.NewMembershipGrantRepository(pageCursors()),
		Containers: containers, Buckets: postgres.NewBucketRepository(), Labels: postgres.NewLabelRepository(),
		Structure: work.StructureResolver{
			Containers: containers, Structures: postgres.NewStructureRepository(),
			Profiles: postgres.NewCapabilityProfileRepository(), Presets: postgres.NewPresetRepository(),
			Installation: postgres.NewInstallationPresetRepository(),
		},
		Events: postgres.NewOutbox(jobQueue(t)), Changes: postgres.NewChangeLog(),
		Audit: postgres.NewAuditSink(ids), Renderer: renderer,
		Domains: textadapter.Domains{}, Text: text.Composing{},
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
		Entropy: clockadapter.CryptoRandom{}, Tenancy: env.TenancyMulti,
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: ids.NewID(), AccountID: ids.NewID(),
		AccountName: "Root Operator", Scopes: []string{"admin:tenants"},
	}
	suffix := strings.ToLower(ids.NewID().String()[24:])
	provisioned, err := provisioning.Execute(ctx, operator, adminservice.ProvisionTenantCommand{
		Slug: "firstcolumns-" + suffix, DisplayName: "First columns", DefaultLocale: "en",
		DefaultTimeZone: "Europe/Berlin", OwnerEmail: "owner-" + suffix + "@firstcolumns.example",
	})
	if err != nil {
		t.Fatalf("provisioning: %v", err)
	}

	collection := provisioned.ExampleCollectionID.String()
	if columns := countIn(ctx, t, `SELECT count(*) FROM bucket WHERE collection_id = $1`, collection); columns != 0 {
		t.Errorf("the example collection has %d columns, want none", columns)
	}
	if labels := countIn(ctx, t, `SELECT count(*) FROM label WHERE collection_id = $1`, collection); labels != 4 {
		t.Errorf("the example collection has %d labels, want 4", labels)
	}
}
