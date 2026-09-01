// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	work "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	now          = time.Date(2026, 9, 1, 9, 0, 0, 0, time.UTC)
	operatorID   = shared.ID("018f2a1b-0000-7000-8000-0000000000a1")
	operatorHome = shared.ID("018f2a1b-0000-7000-8000-0000000000a0")
)

// operator is the control plane's actor: a PAT with the admin scope, minted in the operator's
// own workspace - deliberately not a member of the tenants it administers.
func operator() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: operatorHome, AccountID: operatorID,
		AccountName: "Root Operator", Scopes: []string{adminTenantsScope},
	}
}

// ---- fakes ----

type unitOfWork struct {
	scopes []persistence.Scope
}

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

type tenantsStore struct {
	inserted []adminrepo.TenantRecord
	record   adminrepo.TenantRecord
	findErr  error
	moved    []string
	moveOK   bool
	listed   []adminrepo.TenantRecord
}

func (s *tenantsStore) List(context.Context) ([]adminrepo.TenantRecord, error) {
	return s.listed, nil
}

func (s *tenantsStore) Insert(_ context.Context, record adminrepo.TenantRecord) error {
	s.inserted = append(s.inserted, record)
	return nil
}

func (s *tenantsStore) Find(context.Context) (adminrepo.TenantRecord, error) {
	if s.findErr != nil {
		return adminrepo.TenantRecord{}, s.findErr
	}
	return s.record, nil
}

func (s *tenantsStore) SetStatus(
	_ context.Context, from, to domain.TenantStatus, _ time.Time,
) (bool, error) {
	s.moved = append(s.moved, string(from)+"->"+string(to))
	return s.moveOK, nil
}

type journalStore struct{ entries []adminrepo.InstanceEvent }

func (s *journalStore) Record(_ context.Context, entry adminrepo.InstanceEvent) error {
	s.entries = append(s.entries, entry)
	return nil
}

type accountsStore struct{ inserted []domain.Account }

func (s *accountsStore) Insert(_ context.Context, account domain.Account) error {
	s.inserted = append(s.inserted, account)
	return nil
}

type redemptionStore struct {
	accountID shared.ID
	expiresAt time.Time
}

func (s *redemptionStore) SetRedemptionToken(
	_ context.Context, accountID shared.ID, _ domain.Token, expiresAt, _ time.Time,
) (bool, error) {
	s.accountID, s.expiresAt = accountID, expiresAt
	return true, nil
}

type grantsStore struct{ grants []domain.Grant }

func (s *grantsStore) Grant(_ context.Context, grant domain.Grant) error {
	s.grants = append(s.grants, grant)
	return nil
}

type containersStore struct{ inserted []work.Container }

func (s *containersStore) Insert(_ context.Context, container work.Container) error {
	s.inserted = append(s.inserted, container)
	return nil
}

type bucketsStore struct{ inserted []work.Bucket }

func (s *bucketsStore) Insert(_ context.Context, bucket work.Bucket) error {
	s.inserted = append(s.inserted, bucket)
	return nil
}

type labelsStore struct{ inserted []work.Label }

func (s *labelsStore) Insert(_ context.Context, label work.Label) error {
	s.inserted = append(s.inserted, label)
	return nil
}

type eventsStore struct{ appended []event.Envelope }

func (s *eventsStore) Append(_ context.Context, envelope event.Envelope) error {
	s.appended = append(s.appended, envelope)
	return nil
}

type changesStore struct{ recorded []changelog.Change }

func (s *changesStore) Record(_ context.Context, change changelog.Change) error {
	s.recorded = append(s.recorded, change)
	return nil
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

// renderer records what it was asked so the test can prove the seeded names came through the
// workspace's own locale.
type renderer struct{ asked []string }

func (r *renderer) Render(locale, code string, _ map[string]string) string {
	r.asked = append(r.asked, locale+":"+code)
	return "rendered " + code
}

type hlcSource struct{ count uint32 }

func (h *hlcSource) Next() shared.HLC {
	h.count++
	return shared.HLC{Physical: now, Counter: h.count, Device: "server"}
}

type provisionFixture struct {
	handler    ProvisionTenant
	tenants    *tenantsStore
	journal    *journalStore
	accounts   *accountsStore
	redemption *redemptionStore
	grants     *grantsStore
	containers *containersStore
	buckets    *bucketsStore
	labels     *labelsStore
	events     *eventsStore
	changes    *changesStore
	audit      *auditSink
	renderer   *renderer
	work       *unitOfWork
}

func newProvisionFixture() *provisionFixture {
	f := &provisionFixture{
		tenants: &tenantsStore{}, journal: &journalStore{}, accounts: &accountsStore{},
		redemption: &redemptionStore{}, grants: &grantsStore{}, containers: &containersStore{},
		buckets: &bucketsStore{}, labels: &labelsStore{}, events: &eventsStore{},
		changes: &changesStore{}, audit: &auditSink{}, renderer: &renderer{}, work: &unitOfWork{},
	}
	f.handler = ProvisionTenant{
		Tenants: f.tenants, Journal: f.journal, Accounts: f.accounts,
		Redemption: f.redemption, Grants: f.grants, Containers: f.containers,
		Buckets: f.buckets, Labels: f.labels, Events: f.events, Changes: f.changes,
		Audit: f.audit, Renderer: f.renderer, UnitOfWork: f.work,
		Clock: clock.Fixed(now), IDs: &sequentialIDs{}, HLC: &hlcSource{},
		Entropy: clock.FixedEntropy{}, Tenancy: env.TenancyMulti,
	}
	return f
}

// sequentialIDs answers distinct, valid identifiers, because provisioning mints a dozen rows in
// one call and a fake answering one value would make them indistinguishable.
type sequentialIDs struct{ count int }

func (i *sequentialIDs) NewID() shared.ID {
	i.count++
	suffix := []byte{'0', '0'}
	suffix[0] = "0123456789ab"[i.count/10%12]
	suffix[1] = "0123456789ab"[i.count%10]
	return shared.ID("018f2a1b-9999-7000-8000-0000000000" + string(suffix))
}

func provisionCommand() ProvisionTenantCommand {
	return ProvisionTenantCommand{
		Slug: "acme", DisplayName: "Acme GmbH", DefaultLocale: "de",
		DefaultTimeZone: "Europe/Berlin", OwnerEmail: "eva@acme.example",
	}
}

func TestProvisioningCreatesTheWholeSection5Table(t *testing.T) {
	f := newProvisionFixture()

	provisioned, err := f.handler.Execute(t.Context(), operator(), provisionCommand())
	if err != nil {
		t.Fatalf("provisioning: %v", err)
	}

	tenant := provisioned.Tenant
	if tenant.Slug != "acme" || tenant.Status != domain.TenantActive || tenant.DefaultLocale != "de" {
		t.Errorf("tenant %+v", tenant)
	}
	if len(f.tenants.inserted) != 1 || f.tenants.inserted[0].ID != tenant.ID {
		t.Fatalf("tenant rows %+v", f.tenants.inserted)
	}

	// The transaction runs in the NEW tenant's own scope - the first write is already bounded.
	if len(f.work.scopes) != 1 || f.work.scopes[0].TenantID != tenant.ID {
		t.Errorf("scope %+v, want the new tenant's", f.work.scopes)
	}

	// The owner: invited, way in stored, shown once, OWNER at the tenant scope.
	if len(f.accounts.inserted) != 1 || f.accounts.inserted[0].Status != domain.AccountInvited {
		t.Fatalf("owner %+v", f.accounts.inserted)
	}
	owner := f.accounts.inserted[0]
	if f.redemption.accountID != owner.ID || !f.redemption.expiresAt.After(now) {
		t.Errorf("redemption stored for %v until %v", f.redemption.accountID, f.redemption.expiresAt)
	}
	if provisioned.OwnerRedemption.Reveal() == "" {
		t.Error("no redemption token answered")
	}
	if len(f.grants.grants) != 1 || f.grants.grants[0].Role != domain.RoleOwner ||
		f.grants.grants[0].Scope != domain.TenantScope() || f.grants.grants[0].AccountID != owner.ID {
		t.Errorf("grant %+v", f.grants.grants)
	}

	// The structure: a hub, a collection inside it, three buckets (the last done), four labels.
	if len(f.containers.inserted) != 2 {
		t.Fatalf("%d containers", len(f.containers.inserted))
	}
	hub, collection := f.containers.inserted[0], f.containers.inserted[1]
	if hub.Type != work.ContainerHub || collection.Type != work.ContainerCollection ||
		collection.ParentID != hub.ID {
		t.Errorf("structure %+v / %+v", hub, collection)
	}
	if provisioned.DefaultHubID != hub.ID || provisioned.ExampleCollectionID != collection.ID {
		t.Errorf("answered ids %v %v", provisioned.DefaultHubID, provisioned.ExampleCollectionID)
	}
	if hub.CreatedBy != owner.ID {
		t.Errorf("the hub's creator is %v, want the owner", hub.CreatedBy)
	}
	if len(f.buckets.inserted) != 3 || !f.buckets.inserted[2].IsDoneBucket ||
		f.buckets.inserted[0].IsDoneBucket || f.buckets.inserted[1].IsDoneBucket {
		t.Errorf("buckets %+v", f.buckets.inserted)
	}
	if f.buckets.inserted[0].OrderKey >= f.buckets.inserted[1].OrderKey ||
		f.buckets.inserted[1].OrderKey >= f.buckets.inserted[2].OrderKey {
		t.Errorf("bucket order %q %q %q", f.buckets.inserted[0].OrderKey,
			f.buckets.inserted[1].OrderKey, f.buckets.inserted[2].OrderKey)
	}
	if len(f.labels.inserted) != 4 {
		t.Errorf("%d labels", len(f.labels.inserted))
	}

	// Every seeded row announced itself: an event and a change entry each, so a synchronising
	// client's first pull sees the same workspace the API answers.
	if len(f.events.appended) != 9 || len(f.changes.recorded) != 9 {
		t.Errorf("%d events, %d changes, want 9 each", len(f.events.appended), len(f.changes.recorded))
	}

	// The evidence: the new tenant's own trail, and the instance journal.
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != TenantProvisionedAction ||
		f.audit.entries[0].TenantID != tenant.ID {
		t.Errorf("audit %+v", f.audit.entries)
	}
	if len(f.journal.entries) != 1 || f.journal.entries[0].Action != journalProvisioned ||
		f.journal.entries[0].TenantSlug != "acme" || f.journal.entries[0].ActorLabel != "Root Operator" {
		t.Errorf("journal %+v", f.journal.entries)
	}
}

func TestTheSeededNamesSpeakTheWorkspacesLocale(t *testing.T) {
	f := newProvisionFixture()

	if _, err := f.handler.Execute(t.Context(), operator(), provisionCommand()); err != nil {
		t.Fatalf("provisioning: %v", err)
	}

	if len(f.renderer.asked) != 9 {
		t.Fatalf("%d names rendered, want 9: %v", len(f.renderer.asked), f.renderer.asked)
	}
	for _, ask := range f.renderer.asked {
		if ask[:3] != "de:" {
			t.Errorf("a name was rendered as %q, want the workspace's locale", ask)
		}
	}
	if f.containers.inserted[0].Name != "rendered seed.hub.name" {
		t.Errorf("hub name %q", f.containers.inserted[0].Name)
	}
}

func TestProvisioningOnlyExistsInMultiMode(t *testing.T) {
	f := newProvisionFixture()
	f.handler.Tenancy = env.TenancySingle

	_, err := f.handler.Execute(t.Context(), operator(), provisionCommand())

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.multi_mode_required" {
		t.Errorf("answer %v, want admin.multi_mode_required", err)
	}
	if len(f.tenants.inserted) != 0 {
		t.Error("a tenant landed in single mode")
	}
}

func TestProvisioningDemandsTheAdminScope(t *testing.T) {
	f := newProvisionFixture()
	actor := operator()
	actor.Scopes = []string{"items:write"}

	_, err := f.handler.Execute(t.Context(), actor, provisionCommand())

	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("answer %v, want forbidden", err)
	}
}

func TestProvisioningRefusesABadSlugBeforeAnyWrite(t *testing.T) {
	f := newProvisionFixture()
	cmd := provisionCommand()
	cmd.Slug = "Bad Slug!"

	_, err := f.handler.Execute(t.Context(), operator(), cmd)

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != "admin.slug_invalid" {
		t.Errorf("answer %v, want admin.slug_invalid", err)
	}
	if len(f.work.scopes) != 0 {
		t.Error("a transaction opened for an invalid request")
	}
}

func TestTheOwnersNameDefaultsToTheAddress(t *testing.T) {
	f := newProvisionFixture()
	cmd := provisionCommand()
	cmd.OwnerDisplayName = ""

	if _, err := f.handler.Execute(t.Context(), operator(), cmd); err != nil {
		t.Fatalf("provisioning: %v", err)
	}
	if f.accounts.inserted[0].DisplayName != "eva@acme.example" {
		t.Errorf("owner name %q", f.accounts.inserted[0].DisplayName)
	}
}
