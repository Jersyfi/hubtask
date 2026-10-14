// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"strings"
	"sync"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/mail"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// capturedMail is a mail server that keeps what it is handed.
type capturedMail struct {
	mu   sync.Mutex
	sent []mail.Message
}

func (c *capturedMail) Send(_ context.Context, message mail.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, message)
	return nil
}

// grantingRegistry is GrantMembership the way main.go builds it, telling the person given a role.
func grantingRegistry(ctx context.Context, t *testing.T) *usecase.Registry {
	t.Helper()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	registry, err := usecase.NewRegistry(nil, identityservice.GrantMembership{
		Grants: postgres.NewMembershipGrantRepository(pageCursors()), Accounts: postgres.NewAccountRepository(),
		Groups: postgres.NewGroupRepository(pageCursors()), Hubs: postgres.NewHubLockRepository(),
		Authorizer: realAuthoriser(ctx, t), Audit: postgres.NewAuditSink(ids),
		UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)), Clock: clockadapter.System{}, IDs: ids,
		Notices: notification.RecordRoleGranted{
			Notifications: postgres.NewNotificationRepository(), Accounts: postgres.NewAccountRepository(),
			Preferences: postgres.NewNotificationPreferenceRepository(),
			Jobs:        jobQueue(t), Clock: clockadapter.System{}, IDs: ids,
		},
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// UC-NOT-01 check 9 end to end: a role given through the registry writes the person's record and
// its job in the grant's transaction, and the real delivery sends one mail in the person's language
// naming who gave it, the role and the hub, with the hub's link. Given again, nothing new is
// written; a role revoked before its mail goes takes its record along.
func TestARoleGivenToAPersonIsMailedOnce(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	bert := seedPerson(ctx, t, w.tenant, "Bert Beispiel", freshName(t)+"@test.invalid", "de")
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET display_name = 'Paul Parent' WHERE id = $1`,
		w.administrator.String()); err != nil {
		t.Fatal(err)
	}
	actor := administrator(w.tenant, w.administrator)
	actor.Scopes = []string{"members:write"}
	registry := grantingRegistry(ctx, t)
	grant := usecase.Input{
		"account_id": bert.String(), "scope_type": "HUB", "scope_id": w.sharedHub.String(), "role": "CONTRIBUTOR",
	}

	out, err := registry.Invoke(ctx, identityservice.GrantMembershipName, actor, grant)
	if err != nil {
		t.Fatalf("granting: %v", err)
	}
	records := func(membershipID string) int {
		return countIn(ctx, t, `SELECT count(*) FROM notification WHERE tenant_id = $1 AND recipient_id = $2
			AND category = 'MEMBERSHIP' AND state = 'PENDING' AND coalesce(membership_id::text, '') LIKE $3`,
			w.tenant.String(), bert.String(), membershipID)
	}
	if n := records(out.String("id")); n != 1 {
		t.Fatalf("%d records naming the grant, want one", n)
	}
	if jobs := countIn(ctx, t, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'notification.role_granted'
		AND max_attempts = 20`, w.tenant.String()); jobs != 1 {
		t.Errorf("%d delivery jobs of the role's own kind, want one", jobs)
	}

	// The same role again: the person already holds it, and is not told twice.
	if _, err := registry.Invoke(ctx, identityservice.GrantMembershipName, actor, grant); err != nil {
		t.Fatalf("granting again: %v", err)
	}
	if n := records("%"); n != 1 {
		t.Errorf("%d records after the repeat, want still one", n)
	}

	var recordID string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT id::text FROM notification WHERE membership_id = $1`,
		out.String("id")).Scan(&recordID); err != nil {
		t.Fatalf("reading the record: %v", err)
	}
	var hubName string
	if err := adminPool(ctx, t).QueryRow(ctx, `SELECT name FROM container WHERE id = $1`,
		w.sharedHub.String()).Scan(&hubName); err != nil {
		t.Fatal(err)
	}

	renderer, err := i18n.NewRenderer()
	if err != nil {
		t.Fatal(err)
	}
	box := &capturedMail{}
	delivery := notification.DeliverNotification{
		Notifications: postgres.NewNotificationRepository(), Preferences: postgres.NewNotificationPreferenceRepository(),
		Accounts: postgres.NewAccountRepository(), Items: itemRepo(),
		Grants: postgres.NewMembershipGrantRepository(pageCursors()), Places: containerRepo(),
		Mail: box, Renderer: renderer, UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
		Clock: clockadapter.System{}, BaseURL: "https://hub.test.invalid",
	}
	if err := delivery.Execute(ctx, w.tenant, shared.MustParseID(recordID), false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(box.sent) != 1 {
		t.Fatalf("%d mails, want one", len(box.sent))
	}
	sent := box.sent[0]
	for _, want := range []string{"Paul Parent", "Beitragende Person", "den Hub", hubName,
		"https://hub.test.invalid/hubs/" + w.sharedHub.String()} {
		if !strings.Contains(sent.Body, want) {
			t.Errorf("the mail lacks %q:\n%s", want, sent.Body)
		}
	}
	if !strings.Contains(sent.Subject, hubName) || strings.Contains(sent.Subject+sent.Body, "{") {
		t.Errorf("subject %q, body %q: want the hub named and no placeholder left", sent.Subject, sent.Body)
	}
	stored, err := findNotification(ctx, t, w.tenant, shared.MustParseID(recordID))
	if err != nil || stored.State != domain.StateSent {
		t.Errorf("the record after the send: %+v, %v", stored, err)
	}

	// A role on the collection, revoked before its mail went: the record goes with the grant.
	second, err := registry.Invoke(ctx, identityservice.GrantMembershipName, actor, usecase.Input{
		"account_id": bert.String(), "scope_type": "COLLECTION", "scope_id": w.sharedColl.String(), "role": "VIEWER",
	})
	if err != nil {
		t.Fatalf("granting on the collection: %v", err)
	}
	if n := records(second.String("id")); n != 1 {
		t.Fatalf("%d records for the collection's role, want one", n)
	}
	if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM membership WHERE id = $1`, second.String("id")); err != nil {
		t.Fatalf("revoking: %v", err)
	}
	if n := records(second.String("id")); n != 0 {
		t.Errorf("%d records about a revoked role, want none", n)
	}
}
