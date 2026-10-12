// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// UC-ID-20 check 7, its first half: a managed account asks for an address, a link goes to it, and
// nothing on the account changes yet.

type addressRequest struct {
	credential domain.PendingCredential
	address    string
	token      string
}

// addressStore holds the requests in memory. Confirm writes the address onto the account store, as
// the statement does, and answers a conflict for an address `taken` names.
type addressStore struct {
	accounts *accountStore
	requests []*addressRequest
	taken    map[string]bool
}

func (s *addressStore) LockAccount(_ context.Context, id shared.ID) (domain.Account, error) {
	account, found := s.accounts.byID[id]
	if !found {
		return domain.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

func (s *addressStore) LatestRequest(_ context.Context, id shared.ID) (time.Time, error) {
	var latest time.Time
	for _, request := range s.requests {
		if request.credential.AccountID == id && request.credential.CreatedAt.After(latest) {
			latest = request.credential.CreatedAt
		}
	}
	return latest, nil
}

func (s *addressStore) Supersede(_ context.Context, id shared.ID, at time.Time) error {
	for _, request := range s.requests {
		if request.credential.AccountID == id && request.credential.ConsumedAt.IsZero() {
			request.credential.ConsumedAt = at
		}
	}
	return nil
}

func (s *addressStore) Insert(_ context.Context, credential domain.PendingCredential, address string) error {
	s.requests = append(s.requests, &addressRequest{credential: credential, address: address})
	return nil
}

func (s *addressStore) Mint(
	_ context.Context, id shared.ID, presented domain.Token, now time.Time,
) (repository.AddressMail, error) {
	for _, request := range s.requests {
		if request.credential.ID == id && request.credential.Verify(now) == nil {
			request.token = presented.Secret()
			account := s.accounts.byID[request.credential.AccountID]
			return repository.AddressMail{Address: request.address, DisplayName: account.DisplayName}, nil
		}
	}
	return repository.AddressMail{}, shared.ErrNotFound.WithDetail("auth.address_link_invalid")
}

func (s *addressStore) FindByToken(_ context.Context, token domain.Token) (repository.AddressLookup, error) {
	for _, request := range s.requests {
		if request.token != "" && request.token == token.Secret() {
			account := s.accounts.byID[request.credential.AccountID]
			return repository.AddressLookup{
				Credential: request.credential, Address: request.address,
				AccountStatus: account.Status, TenantStatus: domain.TenantActive,
			}, nil
		}
	}
	return repository.AddressLookup{}, shared.ErrNotFound.WithDetail("auth.address_link_invalid")
}

func (s *addressStore) Consume(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	for _, request := range s.requests {
		if request.credential.ID == id && request.credential.ConsumedAt.IsZero() {
			request.credential.ConsumedAt = at
			return true, nil
		}
	}
	return false, nil
}

func (s *addressStore) Confirm(_ context.Context, id shared.ID, address string, _ time.Time) (bool, error) {
	if s.taken[address] {
		return false, shared.ErrConflict.WithDetail("accounts.address_taken")
	}
	account, found := s.accounts.byID[id]
	if !found || !account.OnlyThePasswordReaches() {
		return false, nil
	}
	account.Email = address
	s.accounts.byID[id] = account
	return true, nil
}

var _ repository.AddressRequests = (*addressStore)(nil)

type addressFixture struct {
	registry   *usecase.Registry
	store      *addressStore
	accounts   *accountStore
	authorizer *authorizer
	jobs       *notifier
	audit      *auditSink
	clock      *movingClock
}

// movingClock is a clock a test can move.
type movingClock struct{ at time.Time }

func (c *movingClock) Now() time.Time { return c.at }

var _ clock.Clock = (*movingClock)(nil)

func newAddressFixture(t *testing.T) *addressFixture {
	t.Helper()
	f := &addressFixture{authorizer: &authorizer{}, jobs: &notifier{}, audit: &auditSink{}, clock: &movingClock{at: now}}
	renewal := newRenewalFixture(t)
	f.accounts = renewal.accounts
	f.store = &addressStore{accounts: f.accounts, taken: map[string]bool{}}
	registry, err := usecase.NewRegistry(nil, RequestAddress{
		Requests: f.store, Authorizer: f.authorizer, Notifier: f.jobs, Audit: f.audit,
		UnitOfWork: &unitOfWork{}, Clock: f.clock, IDs: ids{next: "01936f2a-7c1e-7000-8000-0000000000a7"},
	}.Descriptor())
	if err != nil {
		t.Fatalf("registering: %v", err)
	}
	f.registry = registry
	return f
}

// lena is the managed person, signed in as herself.
func lena() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: lenaID, AccountName: "Lena", Kind: shared.ActorUser,
		Scopes: []string{accountsWrite},
	}
}

func (f *addressFixture) request(
	t *testing.T, actor appshared.ActorContext, id shared.ID, email string,
) (usecase.Output, error) {
	t.Helper()
	return f.registry.Invoke(t.Context(), RequestAddressName, actor,
		usecase.Input{"account_id": id.String(), "email": email})
}

func TestAManagedPersonAsksForTheirOwnAddress(t *testing.T) {
	f := newAddressFixture(t)

	out, err := f.request(t, lena(), lenaID, " Lena@Example.ORG ")
	if err != nil {
		t.Fatalf("asking: %v", err)
	}
	if expires, _ := out["expires_at"].(time.Time); !expires.Equal(now.Add(24 * time.Hour)) {
		t.Errorf("the link runs until %v, want a day", out["expires_at"])
	}
	if len(f.authorizer.requests) != 0 {
		t.Error("a person's own address asked the workspace's permission")
	}
	if len(f.store.requests) != 1 || f.store.requests[0].address != "lena@example.org" ||
		f.store.requests[0].credential.Purpose != domain.PendingAddress {
		t.Fatalf("the requests read %+v", f.store.requests)
	}
	if f.accounts.byID[lenaID].Email != "" {
		t.Error("the account took the address before anybody confirmed it")
	}

	if len(f.jobs.requests) != 1 {
		t.Fatalf("%d jobs queued, want the mail", len(f.jobs.requests))
	}
	job := f.jobs.requests[0]
	if job.Kind != queue.KindAddressConfirmationEmail ||
		job.Payload["request_id"] != f.store.requests[0].credential.ID.String() ||
		strings.Contains(fmt.Sprint(job.Payload), "example.org") {
		t.Errorf("the job reads %+v", job)
	}

	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != AddressRequestedAction {
		t.Fatalf("the trail holds %v", f.audit.entries)
	}
	if written := fmt.Sprint(f.audit.entries[0].Changes); strings.Contains(written, "example.org") {
		t.Errorf("the trail keeps the address readable: %s", written)
	}
}

func TestAnAdministratorAsksForAManagedPersonsAddress(t *testing.T) {
	f := newAddressFixture(t)
	if _, err := f.request(t, admin(), lenaID, "lena@example.org"); err != nil {
		t.Fatalf("asking: %v", err)
	}
	if len(f.authorizer.requests) != 1 || f.authorizer.requests[0].Action != AddressRequestedAction ||
		f.authorizer.requests[0].TargetID != lenaID {
		t.Errorf("the questions were %+v", f.authorizer.requests)
	}
}

func TestAnAddressRequestIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	cases := []struct {
		name    string
		arrange func(f *addressFixture) (appshared.ActorContext, shared.ID, string)
		want    string
	}{
		{"somebody else's, without the permission", func(f *addressFixture) (appshared.ActorContext, shared.ID, string) {
			f.authorizer.refuse = shared.ErrForbidden.WithDetail("access.denied")
			return admin(), lenaID, "lena@example.org"
		}, "access.denied"},
		{"an account with an address", func(*addressFixture) (appshared.ActorContext, shared.ID, string) {
			return admin(), bertID, "bert@elsewhere.example"
		}, "accounts.not_managed"},
		{"a service account", func(*addressFixture) (appshared.ActorContext, shared.ID, string) {
			return admin(), robotID, "robot@example.org"
		}, "accounts.not_managed"},
		{"nobody here", func(*addressFixture) (appshared.ActorContext, shared.ID, string) {
			return admin(), "01936f2a-7c1e-7000-8000-0000000000ff", "x@example.org"
		}, "accounts.not_found"},
		{"not an address", func(*addressFixture) (appshared.ActorContext, shared.ID, string) {
			return lena(), lenaID, "lena at home"
		}, "accounts.email_malformed"},
		{"a token without the account scope", func(*addressFixture) (appshared.ActorContext, shared.ID, string) {
			actor := lena()
			actor.Scopes = []string{"items:read"}
			return actor, lenaID, "lena@example.org"
		}, "access.insufficient_scope"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newAddressFixture(t)
			actor, id, email := c.arrange(f)
			_, err := f.request(t, actor, id, email)
			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if len(f.store.requests)+len(f.jobs.requests)+len(f.audit.entries) != 0 {
				t.Error("something was written")
			}
		})
	}
}

// Ten minutes between two links; the newer one replaces the earlier, which then confirms nothing.
func TestAnotherLinkWaitsAndThenReplacesTheFirst(t *testing.T) {
	f := newAddressFixture(t)
	if _, err := f.request(t, lena(), lenaID, "lena@example.org"); err != nil {
		t.Fatalf("asking: %v", err)
	}

	f.clock.at = now.Add(10*time.Minute - time.Second)
	_, err := f.request(t, lena(), lenaID, "lena@elsewhere.example")
	refusal := shared.AsError(err)
	if refusal == nil || refusal.DetailCode != "accounts.address_requested_recently" ||
		refusal.Params["retry_after_seconds"] != "1" || refusal.Category != shared.CategoryRateLimited {
		t.Fatalf("a second link at once answered %v", err)
	}

	f.clock.at = now.Add(10 * time.Minute)
	if _, err := f.request(t, lena(), lenaID, "lena@elsewhere.example"); err != nil {
		t.Fatalf("asking again: %v", err)
	}
	if len(f.store.requests) != 2 || f.store.requests[0].credential.ConsumedAt.IsZero() ||
		!f.store.requests[1].credential.ConsumedAt.IsZero() {
		t.Errorf("the first link is still live: %+v", f.store.requests)
	}
}

// Whether an address belongs to somebody here is not said at the request.
func TestAnAddressSomebodyHoldsIsAcceptedAlike(t *testing.T) {
	f := newAddressFixture(t)
	f.store.taken["bert@example.org"] = true
	if _, err := f.request(t, lena(), lenaID, "bert@example.org"); err != nil {
		t.Errorf("an address somebody holds answered %v", err)
	}
}
