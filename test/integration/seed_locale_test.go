// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"slices"
	"strings"
	"testing"

	adminservice "github.com/Jersyfi/hubtask/core/application/service/admin"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	textadapter "github.com/Jersyfi/hubtask/infrastructure/text"
)

// user-experience.md §4 against the real stores: a workspace provisioned in German whose owner
// redeems the invitation in English finds its default hub, example collection, columns and labels
// in English - with the events and change entries a rename owes - except what somebody already
// renamed; and another workspace's seed is untouched (gate SG-3).
func TestTheOwnersRedemptionNamesTheSeedInTheOwnersLanguage(t *testing.T) {
	ctx := context.Background()
	writer, signIn, uow := realWriter(ctx, t)
	admin := adminPool(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	hybrid, err := clockadapter.NewHybridClock(clockadapter.System{}, "server-integration")
	if err != nil {
		t.Fatalf("building the clock: %v", err)
	}
	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	containers, buckets, labels := containerRepo(), postgres.NewBucketRepository(), postgres.NewLabelRepository()
	events, changes := postgres.NewOutbox(jobQueue(t)), postgres.NewChangeLog()
	provisioning := adminservice.ProvisionTenant{
		Tenants: postgres.NewAdminTenantRepository(), Journal: postgres.NewInstanceJournal(pageCursors()),
		Accounts: postgres.NewAccountRepository(), Redemption: signIn,
		Grants:     postgres.NewMembershipGrantRepository(pageCursors()),
		Containers: containers, Buckets: buckets, Labels: labels,
		Events: events, Changes: changes, Audit: postgres.NewAuditSink(ids), Renderer: renderer,
		Domains: textadapter.Domains{}, Text: text.Composing{},
		UnitOfWork: uow, Clock: clockadapter.System{}, IDs: ids, HLC: hybrid,
		Entropy: clockadapter.CryptoRandom{}, Tenancy: env.TenancyMulti,
	}
	operator := appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: ids.NewID(), AccountID: ids.NewID(),
		AccountName: "Root Operator", Scopes: []string{"admin:tenants"},
	}
	provision := func(name string) adminservice.ProvisionedTenant {
		t.Helper()
		suffix := strings.ToLower(ids.NewID().String()[24:])
		provisioned, err := provisioning.Execute(ctx, operator, adminservice.ProvisionTenantCommand{
			Slug: "seedlocale-" + name + "-" + suffix, DisplayName: "Seed " + name, DefaultLocale: "de",
			DefaultTimeZone: "Europe/Berlin", OwnerEmail: "owner-" + suffix + "@seedlocale.example",
		})
		if err != nil {
			t.Fatalf("provisioning %s: %v", name, err)
		}
		return provisioned
	}
	mine, theirs := provision("a"), provision("b")

	// A label somebody renamed before the owner arrived keeps its name.
	if _, err := admin.Exec(ctx, `UPDATE label SET name = 'Ideen', version = 2
		WHERE collection_id = $1 AND name = 'Idee'`, mine.ExampleCollectionID.String()); err != nil {
		t.Fatalf("renaming a label: %v", err)
	}

	redeem := identityservice.RedeemInvitation{Writer: writer, Seed: adminservice.LocaliseSeed{
		Tenants: postgres.NewAdminTenantRepository(), Containers: containers, Buckets: buckets, Labels: labels,
		Events: events, Changes: changes, Renderer: renderer, Text: text.Composing{}, IDs: ids, HLC: hybrid,
	}}
	if _, err := redeem.Execute(ctx, identityservice.RedeemInvitationCommand{
		Token: mine.OwnerRedemption, Password: secret.New("seven blue lanterns above the harbour"),
		Locale: "en",
	}); err != nil {
		t.Fatalf("redeeming: %v", err)
	}

	names := func(query string, id shared.ID) []string {
		t.Helper()
		rows, err := admin.Query(ctx, query, id.String())
		if err != nil {
			t.Fatalf("reading names: %v", err)
		}
		defer rows.Close()
		var found []string
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				t.Fatalf("scanning: %v", err)
			}
			found = append(found, name)
		}
		slices.Sort(found)
		return found
	}
	const (
		containerNames = `SELECT name FROM container WHERE id = $1`
		bucketNames    = `SELECT name FROM bucket WHERE collection_id = $1`
		labelNames     = `SELECT name FROM label WHERE collection_id = $1`
	)
	for _, tc := range []struct {
		what  string
		query string
		id    shared.ID
		want  []string
	}{
		{"my hub", containerNames, mine.DefaultHubID, []string{"General"}},
		{"my collection", containerNames, mine.ExampleCollectionID, []string{"Getting started"}},
		{"my columns", bucketNames, mine.ExampleCollectionID, []string{"Done", "In progress", "To do"}},
		{"my labels", labelNames, mine.ExampleCollectionID, []string{"Ideen", "Important", "Quick win", "Waiting"}},
		{"their hub", containerNames, theirs.DefaultHubID, []string{"Allgemein"}},
		{"their collection", containerNames, theirs.ExampleCollectionID, []string{"Erste Schritte"}},
		{"their columns", bucketNames, theirs.ExampleCollectionID, []string{"Erledigt", "In Arbeit", "Zu erledigen"}},
	} {
		if got := names(tc.query, tc.id); !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v, want %v", tc.what, got, tc.want)
		}
	}

	// A rename owes the event outwards and the change entry for offline clients, per field.
	if renamed := countIn(ctx, t, `SELECT count(*) FROM change_log
		WHERE entity_id = $1 AND payload ->> 'name' = 'General'`,
		mine.DefaultHubID.String()); renamed != 1 {
		t.Errorf("the hub's rename has %d change entries, want 1", renamed)
	}
	if announced := countIn(ctx, t, `SELECT count(*) FROM outbox_event
		WHERE tenant_id = $1 AND event_type LIKE '%container.renamed%'`, mine.Tenant.ID.String()); announced != 2 {
		t.Errorf("%d container renames were announced, want the hub's and the collection's", announced)
	}
	if version := countIn(ctx, t, `SELECT version FROM container WHERE id = $1`, mine.DefaultHubID.String()); version != 2 {
		t.Errorf("the hub is at version %d, want 2", version)
	}
}
