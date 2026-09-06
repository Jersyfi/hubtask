// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sealing

import (
	"context"
	"errors"
	"testing"
	"time"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

var (
	operatorHome = shared.MustParseID("018f2a1b-0000-7000-8000-0000000000a0")
	operatorID   = shared.MustParseID("018f2a1b-0000-7000-8000-0000000000a1")
	tenantOne    = shared.MustParseID("018f2a1b-0000-7000-8000-0000000000b1")
	tenantTwo    = shared.MustParseID("018f2a1b-0000-7000-8000-0000000000b2")
	now          = time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
)

func operator() appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: operatorHome, AccountID: operatorID,
		AccountName: "Root Operator", Scopes: []string{adminTenantsScope},
	}
}

type ring struct{ keys []string }

func (r ring) Seal(context.Context, secret.Secret, cryptoport.Purpose) (cryptoport.Sealed, error) {
	return cryptoport.Sealed{}, nil
}

func (r ring) Open(context.Context, cryptoport.Sealed, cryptoport.Purpose) (secret.Secret, error) {
	return secret.Secret{}, nil
}

func (r ring) Rewrap(_ context.Context, sealed cryptoport.Sealed, _ cryptoport.Purpose) (cryptoport.Sealed, error) {
	return sealed, nil
}

func (r ring) ActiveKeyID() string {
	if len(r.keys) == 0 {
		return ""
	}
	return r.keys[0]
}

func (r ring) KeyIDs() []string { return r.keys }

type unitOfWork struct{ scopes []persistence.Scope }

func (u *unitOfWork) Within(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	u.scopes = append(u.scopes, scope)
	return fn(ctx)
}

func (u *unitOfWork) WithinReadOnly(ctx context.Context, scope persistence.Scope, fn func(context.Context) error) error {
	return u.Within(ctx, scope, fn)
}

type tenants struct{ listed []adminrepo.TenantRecord }

func (t tenants) List(context.Context) ([]adminrepo.TenantRecord, error) { return t.listed, nil }
func (tenants) Insert(context.Context, adminrepo.TenantRecord) error     { return nil }
func (tenants) Find(context.Context) (adminrepo.TenantRecord, error) {
	return adminrepo.TenantRecord{}, shared.ErrNotFound
}

func (tenants) SetStatus(context.Context, identity.TenantStatus, identity.TenantStatus, time.Time) (bool, error) {
	return false, nil
}

func (tenants) RequestDeletion(context.Context, time.Time, time.Time) (bool, error) {
	return false, nil
}

// census answers a different count per tenant, keyed by the scope the unit of work opened.
type census struct {
	work   *unitOfWork
	counts map[shared.ID]map[string]int64
}

func (c census) CountByKey(context.Context) (map[string]int64, error) {
	current := c.work.scopes[len(c.work.scopes)-1].TenantID
	return c.counts[current], nil
}

type jobs struct{ requests []queue.Request }

func (j *jobs) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	j.requests = append(j.requests, request)
	return shared.MustParseID("018f2a1b-0000-7000-8000-0000000000ff"), nil
}

type sink struct{ entries []audit.Entry }

func (s *sink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type resealer struct {
	store   string
	outcome Outcome
	err     error
	tenants []shared.ID
}

func (r *resealer) Store() string { return r.store }
func (r *resealer) Reseal(_ context.Context, tenantID shared.ID) (Outcome, error) {
	r.tenants = append(r.tenants, tenantID)
	return r.outcome, r.err
}

type signals struct{ seen map[string]int64 }

func (s *signals) SecretResealed(_ context.Context, store, outcome string, count int64) {
	if s.seen == nil {
		s.seen = map[string]int64{}
	}
	s.seen[store+"/"+outcome] += count
}

func TestTheStatusSumsTheCensusOverEveryWorkspaceAndListsTheRingFirst(t *testing.T) {
	work := &unitOfWork{}
	status := ReadEncryptionStatus{
		Tenants: tenants{listed: []adminrepo.TenantRecord{{ID: tenantOne}, {ID: tenantTwo}}},
		Census: census{work: work, counts: map[shared.ID]map[string]int64{
			tenantOne: {"k1": 3, "gone": 1},
			tenantTwo: {"k1": 2, "k2": 5},
		}},
		Encryptor: ring{keys: []string{"k2", "k1"}}, UnitOfWork: work,
	}

	answer, err := status.Execute(t.Context(), operator())
	if err != nil {
		t.Fatalf("reading the status: %v", err)
	}
	if answer.ActiveKeyID != "k2" {
		t.Errorf("active key %q", answer.ActiveKeyID)
	}
	want := []KeyUsage{
		{KeyID: "k2", Active: true, InRing: true, SealedValues: 5},
		{KeyID: "k1", InRing: true, SealedValues: 5},
		{KeyID: "gone", SealedValues: 1},
	}
	if len(answer.Keys) != len(want) {
		t.Fatalf("keys %+v", answer.Keys)
	}
	for i := range want {
		if answer.Keys[i] != want[i] {
			t.Errorf("key %d: %+v, want %+v", i, answer.Keys[i], want[i])
		}
	}
	// The enumerator ran installation-scoped, and every count in the tenant's own transaction.
	if len(work.scopes) != 3 || !work.scopes[0].Installation ||
		work.scopes[1].TenantID != tenantOne || work.scopes[2].TenantID != tenantTwo {
		t.Errorf("scopes %+v", work.scopes)
	}
}

func TestTheStatusIsTheControlPlanesAlone(t *testing.T) {
	status := ReadEncryptionStatus{
		Tenants: tenants{}, Encryptor: ring{keys: []string{"k1"}}, UnitOfWork: &unitOfWork{},
	}
	member := operator()
	member.Scopes = nil
	if _, err := status.Execute(t.Context(), member); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a member read the keyring: %v", err)
	}
}

func TestTheRequestQueuesOneRoundPerWorkspaceAndRecordsTheAsk(t *testing.T) {
	work := &unitOfWork{}
	queued := &jobs{}
	trail := &sink{}
	request := ResealSecrets{
		Tenants: tenants{listed: []adminrepo.TenantRecord{{ID: tenantOne}, {ID: tenantTwo}}},
		Jobs:    queued, Encryptor: ring{keys: []string{"k2", "k1"}},
		Audit: trail, UnitOfWork: work, Clock: clock.Fixed(now),
	}

	accepted, err := request.Execute(t.Context(), operator())
	if err != nil {
		t.Fatalf("requesting: %v", err)
	}
	if accepted.ActiveKeyID != "k2" || accepted.QueuedTenants != 2 {
		t.Errorf("accepted %+v", accepted)
	}
	if len(queued.requests) != 2 {
		t.Fatalf("jobs %+v", queued.requests)
	}
	for i, tenant := range []shared.ID{tenantOne, tenantTwo} {
		job := queued.requests[i]
		if job.Kind != queue.KindSecretReseal || job.TenantID != tenant ||
			job.DedupeKey != "reseal:"+tenant.String() || job.Payload["active_key_id"] != "k2" {
			t.Errorf("job %d: %+v", i, job)
		}
		// Each job is queued in the workspace's own transaction, opened by the operator.
		if scope := work.scopes[1+i]; scope.TenantID != tenant || scope.ActorID != operatorID {
			t.Errorf("scope %d: %+v", i, scope)
		}
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != ResealRequestedAction ||
		trail.entries[0].TenantID != operatorHome || trail.entries[0].Severity != audit.SeverityNotice {
		t.Errorf("trail %+v", trail.entries)
	}
}

func TestAnInstallationWithoutKeysHasNothingToReseal(t *testing.T) {
	request := ResealSecrets{
		Tenants: tenants{listed: []adminrepo.TenantRecord{{ID: tenantOne}}}, Jobs: &jobs{},
		Encryptor: ring{}, Audit: &sink{}, UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now),
	}
	_, err := request.Execute(t.Context(), operator())
	if !errors.Is(err, shared.ErrUnavailable) ||
		shared.AsError(err).DetailCode != cryptoport.CodeNoEncryptionKey {
		t.Fatalf("an installation without keys answered %v", err)
	}
}

func TestARoundRunsEveryResealerReportsAndLeavesEvidenceOnlyWhenSomethingMoved(t *testing.T) {
	first := &resealer{store: "account_mfa", outcome: Outcome{Rewrapped: 2}}
	second := &resealer{store: "backup_target", outcome: Outcome{Skipped: 1}}
	trail := &sink{}
	seen := &signals{}
	round := RunReseal{
		Resealers: []Resealer{first, second}, Encryptor: ring{keys: []string{"k2", "k1"}},
		Audit: trail, Clock: clock.Fixed(now), Signals: seen,
	}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantOne}

	outcome, err := round.Execute(t.Context(), system)
	if err != nil {
		t.Fatalf("running the round: %v", err)
	}
	if outcome != (Outcome{Rewrapped: 2, Skipped: 1}) {
		t.Errorf("outcome %+v", outcome)
	}
	if first.tenants[0] != tenantOne || second.tenants[0] != tenantOne {
		t.Error("a resealer was not told the tenant")
	}
	if seen.seen["account_mfa/rewrapped"] != 2 || seen.seen["backup_target/skipped"] != 1 {
		t.Errorf("signals %+v", seen.seen)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != ResealedAction ||
		trail.entries[0].TenantID != tenantOne || trail.entries[0].ActorKind != shared.ActorSystem {
		t.Errorf("trail %+v", trail.entries)
	}

	// The normal second run: everything is already under the current key, and nothing is recorded.
	first.outcome, second.outcome = Outcome{}, Outcome{}
	if _, err := round.Execute(t.Context(), system); err != nil {
		t.Fatal(err)
	}
	if len(trail.entries) != 1 {
		t.Errorf("a round that moved nothing left an entry: %+v", trail.entries)
	}
}

func TestARoundFailsWholeWhenAStoreCannotBeReached(t *testing.T) {
	broken := &resealer{store: "webhook_subscription", err: shared.ErrUnavailable.WithDetail("postgres.query_failed")}
	round := RunReseal{
		Resealers: []Resealer{broken}, Encryptor: ring{keys: []string{"k1"}},
		Audit: &sink{}, Clock: clock.Fixed(now),
	}
	_, err := round.Execute(t.Context(), appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantOne})
	if !errors.Is(err, shared.ErrUnavailable) {
		t.Fatalf("a broken store answered %v", err)
	}
}

func TestOnlyAMissingKeyIsSkippedOver(t *testing.T) {
	if !Unopenable(shared.ErrUnavailable.WithDetail(cryptoport.CodeUnknownKey)) {
		t.Error("a missing key is not skippable")
	}
	if Unopenable(shared.ErrUnavailable.WithDetail("postgres.query_failed")) {
		t.Error("an unreachable store was skipped over")
	}
	if Unopenable(cryptoport.NotAuthentic()) {
		t.Error("a tampered value was skipped over")
	}
}
