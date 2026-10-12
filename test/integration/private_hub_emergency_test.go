// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	notificationrepo "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/notification"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/stepup"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// freshProof stands in for the step-up, which is not what these tests are about: one token
// proves, any other is refused.
type freshProof struct{}

func (freshProof) Available() bool { return true }
func (freshProof) Methods(context.Context, shared.ID, shared.ID) ([]stepup.Method, error) {
	return []stepup.Method{stepup.MethodPassword}, nil
}
func (freshProof) Satisfied(_ context.Context, _ shared.ID, token string) (bool, error) {
	return token == "hbt_sup_fresh", nil
}

// failingNotices is the notification table refusing a write: the opening must open nothing.
type failingNotices struct{ notificationrepo.Notifications }

func (failingNotices) Insert(context.Context, domain.Notification) (bool, error) {
	return false, shared.ErrUnavailable.WithDetail("postgres.query_failed")
}

// openingRegistry is OpenPrivateHub the way main.go builds it, at the fixed moment `created`.
func openingRegistry(ctx context.Context, t *testing.T, notifications notificationrepo.Notifications) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	ids := clockadapter.NewUUIDv7(portclock.Fixed(created))
	registry, err := usecase.NewRegistry(nil, work.OpenPrivateHub{
		Accesses: postgres.NewEmergencyAccessRepository(), Hubs: postgres.NewHubLockRepository(),
		Notices: notification.RecordEmergencyAccess{
			Notifications: notifications, Accounts: postgres.NewAccountRepository(),
			Preferences: postgres.NewNotificationPreferenceRepository(),
			Jobs:        jobQueue(t), Clock: portclock.Fixed(created), IDs: ids,
		},
		Authorizer: realAuthoriser(ctx, t), StepUp: freshProof{},
		Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: portclock.Fixed(created), IDs: ids,
	}.Descriptor())
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// listingRegistry is the three readers of the emergency access, at the moment `at`.
func listingRegistry(ctx context.Context, t *testing.T, at time.Time) *usecase.Registry {
	t.Helper()
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	authoriser := realAuthoriser(ctx, t)
	authoriser.Clock = portclock.Fixed(at)
	registry, err := usecase.NewRegistry(nil,
		work.ListEmergencyNotices{
			Accesses: postgres.NewEmergencyAccessRepository(), UnitOfWork: unitOfWork, Clock: portclock.Fixed(at),
		}.Descriptor(),
		work.ListPrivateHubAccesses{
			Accesses: postgres.NewEmergencyAccessRepository(), Authorizer: authoriser, UnitOfWork: unitOfWork,
		}.Descriptor(),
		work.ListPrivateHubs{
			Hubs: postgres.NewPrivateHubRepository(), Policies: postgres.NewLifecycleRepository(),
			Authorizer: authoriser, UnitOfWork: unitOfWork,
			Accesses: postgres.NewEmergencyAccessRepository(), Owners: authoriser, Clock: portclock.Fixed(at),
		}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building the catalogue: %v", err)
	}
	return registry
}

// present is the owner as a request through the API carries them.
func present(tenant, account shared.ID) appshared.ActorContext {
	actor := administrator(tenant, account)
	actor.Present = true
	actor.Scopes = []string{"containers:write", "containers:read", "items:read", "members:read"}
	return actor
}

// readsTheHub answers whether the authoriser, at the moment, lets the actor read the private hub's
// entry - the question every reader of the hub asks.
func readsTheHub(ctx context.Context, t *testing.T, w privateWorld, actor appshared.ActorContext, at time.Time) bool {
	t.Helper()
	authoriser := realAuthoriser(ctx, t)
	authoriser.Clock = portclock.Fixed(at)
	err := authoriser.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Action: "item.read",
		Path: []identity.Scope{identity.TenantScope(), identity.HubScope(w.hub), identity.CollectionScope(w.collection)},
		On:   access.ItemSubject{Does: service.ItemRead, ID: w.item},
	})
	switch {
	case err == nil:
		return true
	case errors.Is(err, shared.ErrNotFound):
		return false
	default:
		t.Fatalf("reading the hub: %v", err)
		return false
	}
}

// The task's acceptance: the access notifies before it answers any read. Before the opening the
// owner reads nothing; an opening whose members cannot be told opens nothing; a successful one has
// every member's record, the mail's job and the trail's entry committed before the first read is
// answered - the managed member without a mailbox included, with a record the app shows them.
func TestTheEmergencyAccessNotifiesBeforeItAnswersAnyRead(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	admin := adminPool(ctx, t)
	maria := freshID(t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO account (id, tenant_id, display_name, email) VALUES ($1, $2, 'Maria Parent', $3)`,
			[]any{maria.String(), w.tenant.String(), "maria-" + maria.String() + "@example.org"}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, scope_id, role)
		  VALUES ($1, $2, $3, 'COLLECTION', $4, 'MEMBER')`,
			[]any{freshID(t).String(), w.tenant.String(), maria.String(), w.collection.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	owner := present(w.tenant, w.administrator)
	input := usecase.Input{"hub_id": w.hub.String(), "reason": "Maria is in hospital", "step_up_token": "hbt_sup_fresh"}

	if readsTheHub(ctx, t, w, owner, created) {
		t.Fatal("the owner read the private hub before opening it")
	}

	// The members cannot be told: nothing opens.
	_, err := openingRegistry(ctx, t, failingNotices{postgres.NewNotificationRepository()}).
		Invoke(ctx, work.OpenPrivateHubName, owner, input)
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("an opening nobody could be told of answered %v", err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM private_hub_access WHERE hub_id = $1`, w.hub.String()); n != 0 {
		t.Fatalf("%d accesses after the failed opening", n)
	}
	if readsTheHub(ctx, t, w, owner, created) {
		t.Fatal("the owner read the hub although its members were never told")
	}

	out, err := openingRegistry(ctx, t, postgres.NewNotificationRepository()).
		Invoke(ctx, work.OpenPrivateHubName, owner, input)
	if err != nil {
		t.Fatalf("opening: %v", err)
	}
	accessID := out.String("id")

	// Committed with the access, before any read: a record per member, a job for the one with an
	// address, the managed member's suppressed for no address, the trail's entry.
	records := func(state, reason string) int {
		return countIn(ctx, t, `SELECT count(*) FROM notification WHERE private_hub_access_id = $1
			AND category = 'EMERGENCY_ACCESS' AND state = $2 AND coalesce(reason, '') = $3`,
			accessID, state, reason)
	}
	if records("PENDING", "") != 1 || records("SUPPRESSED", domain.ReasonNoAddress) != 1 {
		t.Errorf("records: %d pending, %d suppressed for no address; want one each",
			records("PENDING", ""), records("SUPPRESSED", domain.ReasonNoAddress))
	}
	if jobs := countIn(ctx, t, `SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'notification.emergency_access'
		AND max_attempts = 20`, w.tenant.String()); jobs != 1 {
		t.Errorf("%d delivery jobs of the access's own kind, want one", jobs)
	}
	if entries := countIn(ctx, t, `SELECT count(*) FROM audit_log WHERE tenant_id = $1
		AND action = 'container.emergency_access_opened' AND target_id = $2
		AND NOT (changes::text LIKE '%hospital%')`, w.tenant.String(), w.hub.String()); entries != 1 {
		t.Errorf("%d trail entries with the reason masked, want one", entries)
	}

	// Only now is a read answered - for the owner, present, during the hour.
	if !readsTheHub(ctx, t, w, owner, created.Add(30*time.Minute)) {
		t.Error("the owner cannot read the hub they opened")
	}
	if readsTheHub(ctx, t, w, administrator(w.tenant, w.administrator), created.Add(30*time.Minute)) {
		t.Error("the access counted for something that is not the person present")
	}
	if readsTheHub(ctx, t, w, owner, created.Add(time.Hour)) {
		t.Error("the access outlived its hour")
	}

	// The member without a mailbox - a managed account, a sign-in name and no address - is told in
	// the app from the record written in the opening's transaction, and so is everybody else.
	lena := w.member
	if _, err := admin.Exec(ctx, `UPDATE account SET sign_in_name = $2 WHERE id = $1`,
		lena.String(), "lena"+lena.String()[24:]); err != nil {
		t.Fatalf("making the child's account a managed one: %v", err)
	}
	lists := listingRegistry(ctx, t, created.Add(10*time.Minute))
	notices, err := lists.Invoke(ctx, work.ListEmergencyNoticesName, present(w.tenant, lena), usecase.Input{})
	if err != nil {
		t.Fatalf("the child's notices: %v", err)
	}
	if rows, _ := notices["data"].([]usecase.Output); len(rows) != 1 ||
		rows[0]["reason"] != "Maria is in hospital" || rows[0]["opened_by"] != w.administrator.String() {
		t.Errorf("the managed member is shown %v, want the access with its reason", notices["data"])
	}
	accesses, err := lists.Invoke(ctx, work.ListPrivateHubAccessesName, present(w.tenant, lena),
		usecase.Input{"hub_id": w.hub.String()})
	if rows, _ := accesses["data"].([]usecase.Output); err != nil || len(rows) != 1 {
		t.Errorf("the hub's member reads its accesses: %v, %v", accesses, err)
	}
	if _, err := listingRegistry(ctx, t, created.Add(time.Hour)).Invoke(ctx, work.ListPrivateHubAccessesName,
		owner, usecase.Input{"hub_id": w.hub.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the owner after the hour reads the hub's accesses: %v, want not found", err)
	}
	hubs, err := lists.Invoke(ctx, work.ListPrivateHubsName, owner, usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range hubs["data"].([]usecase.Output) {
		if row["id"] != w.hub.String() {
			continue
		}
		emergency, _ := row["emergency_access"].(usecase.Output)
		if emergency["offered"] != false || emergency["open_until"] != created.Add(time.Hour) {
			t.Errorf("the owner's row while the access runs: %v", emergency)
		}
	}

	// Not renewable while it runs.
	_, err = openingRegistry(ctx, t, postgres.NewNotificationRepository()).
		Invoke(ctx, work.OpenPrivateHubName, owner, input)
	if !errors.Is(err, shared.ErrConflict) || shared.AsError(err).DetailCode != "containers.emergency_running" {
		t.Errorf("a second access answered %v", err)
	}
}

// The archive carries the members' record of who opened their hub, and a restore writes it back
// whole; an access whose hub the restore did not bring back is skipped rather than refused.
func TestTheArchiveCarriesTheEmergencyAccesses(t *testing.T) {
	ctx := context.Background()
	hub := privateHub(ctx, t, tenantA, authorA)
	opening := workdomain.EmergencyAccess{
		ID: freshID(t), TenantID: tenantA, HubID: hub, OpenedBy: authorA,
		Reason: "the insurance papers", OpenedAt: created, EndsAt: created.Add(time.Hour),
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return postgres.NewEmergencyAccessRepository().Insert(ctx, opening)
	}); err != nil {
		t.Fatalf("writing the access: %v", err)
	}

	row := exportedRowOf(ctx, t, tenantA, "private_hub_access", opening.ID)
	if row["reason"] != "the insurance papers" || row["hub_id"] != hub.String() {
		t.Fatalf("the export carries %v", row)
	}
	remove := func() {
		if _, err := adminPool(ctx, t).Exec(ctx, `DELETE FROM private_hub_access WHERE id = $1`, opening.ID.String()); err != nil {
			t.Fatalf("removing the access: %v", err)
		}
	}
	remove()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "private_hub_access", row, false)
		return err
	}); err != nil {
		t.Fatalf("restoring the access: %v", err)
	}
	restored := exportedRowOf(ctx, t, tenantA, "private_hub_access", opening.ID)
	for column, before := range row {
		if after := restored[column]; !sameJSON(before, after) {
			t.Errorf("%s came back as %#v, want %#v", column, after, before)
		}
	}

	remove()
	orphan := make(map[string]any, len(row))
	for key, value := range row {
		orphan[key] = value
	}
	orphan["hub_id"] = freshID(t).String()
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		_, err := importRepo().Write(ctx, "private_hub_access", orphan, false)
		return err
	}); err != nil {
		t.Fatalf("an access without its hub was refused: %v", err)
	}
	if n := countIn(ctx, t, `SELECT count(*) FROM private_hub_access WHERE id = $1`, opening.ID.String()); n != 0 {
		t.Errorf("an access without its hub was written")
	}
}

// Somebody other than the owner is refused before anything is looked up; another workspace sees
// neither the access nor the hub.
func TestOnlyTheOwnerOpensAndNoOtherWorkspaceSeesIt(t *testing.T) {
	ctx := context.Background()
	w := seedPrivateWorld(ctx, t)
	registry := openingRegistry(ctx, t, postgres.NewNotificationRepository())
	input := usecase.Input{"hub_id": w.hub.String(), "reason": "curiosity", "step_up_token": "hbt_sup_fresh"}

	if _, err := registry.Invoke(ctx, work.OpenPrivateHubName, present(w.tenant, w.member), input); !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("a member who is not the owner: %v, want forbidden", err)
	}
	if _, err := registry.Invoke(ctx, work.OpenPrivateHubName, present(tenantB, authorB), input); err == nil {
		t.Error("another workspace's owner opened the hub")
	}

	if _, err := registry.Invoke(ctx, work.OpenPrivateHubName, present(w.tenant, w.administrator),
		usecase.Input{"hub_id": w.hub.String(), "reason": "the insurance papers", "step_up_token": "hbt_sup_fresh"}); err != nil {
		t.Fatalf("opening: %v", err)
	}
	var seen []shared.ID
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		accesses, err := postgres.NewEmergencyAccessRepository().List(ctx, w.hub, 10)
		for _, each := range accesses {
			seen = append(seen, each.ID)
		}
		if err != nil {
			return err
		}
		opened, err := postgres.NewMembershipRepository().HubsOpenedFor(ctx, w.administrator, []shared.ID{w.hub}, created)
		seen = append(seen, opened...)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(seen) != 0 {
		t.Errorf("another workspace sees %v", seen)
	}
}
