// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The fixtures of the administration use cases. `tenant`, `account` and `now` belong to the
// authentication tests in this package and are reused rather than shadowed.
var (
	adminID   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	invitedID = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
)

// accountStore is the repository, in memory, keyed the way the table is.
type accountStore struct {
	byID       map[shared.ID]domain.Account
	byEmail    map[string]domain.Account
	inserted   []domain.Account
	preference []domain.Account
	insertErr  error
	// lapsed names invitations whose time ran out, which AcceptInvitation refuses as the
	// statement's `redemption_expires_at` comparison does.
	lapsed map[shared.ID]bool
}

// AcceptInvitation activates an invited account whose invitation has not run out.
func (s *accountStore) AcceptInvitation(_ context.Context, id shared.ID, _ time.Time) (bool, error) {
	account, found := s.byID[id]
	if !found || account.Status != domain.AccountInvited || s.lapsed[id] {
		return false, nil
	}
	account.Status = domain.AccountActive
	s.byID[id] = account
	s.byEmail[account.Email] = account
	return true, nil
}

func newAccounts(existing ...domain.Account) *accountStore {
	store := &accountStore{byID: map[shared.ID]domain.Account{}, byEmail: map[string]domain.Account{}}
	for _, account := range existing {
		store.byID[account.ID] = account
		if account.Email != "" {
			store.byEmail[account.Email] = account
		}
	}
	return store
}

func (s *accountStore) Find(_ context.Context, id shared.ID) (domain.Account, error) {
	account, found := s.byID[id]
	if !found {
		return domain.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

// Restricted answers from the store's own statuses. Nothing in this package processes
// automatically, so it is here to satisfy the port rather than to be asked.
func (s *accountStore) Restricted(
	_ context.Context, accountIDs []shared.ID,
) (map[shared.ID]bool, error) {
	restricted := map[shared.ID]bool{}
	for _, accountID := range accountIDs {
		if account, found := s.byID[accountID]; found && !account.Status.ProcessingAllowed() {
			restricted[accountID] = true
		}
	}
	return restricted, nil
}

func (s *accountStore) FindByEmail(_ context.Context, email string) (domain.Account, error) {
	account, found := s.byEmail[email]
	if !found {
		return domain.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

func (s *accountStore) Insert(_ context.Context, account domain.Account) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.byID[account.ID] = account
	s.byEmail[account.Email] = account
	s.inserted = append(s.inserted, account)
	return nil
}

// ListOfKind reads the store's own accounts. Insertion order, so that a listing test can say
// which came first without a clock.
func (s *accountStore) ListOfKind(_ context.Context, kind domain.AccountKind) ([]domain.Account, error) {
	var found []domain.Account
	for _, account := range s.byID {
		if account.Kind == kind {
			found = append(found, account)
		}
	}
	slices.SortFunc(found, func(a, b domain.Account) int { return strings.Compare(string(b.ID), string(a.ID)) })
	return found, nil
}

func (s *accountStore) UpdatePreferences(_ context.Context, account domain.Account, _ time.Time) error {
	s.byID[account.ID] = account
	s.preference = append(s.preference, account)
	return nil
}

var _ repository.Accounts = (*accountStore)(nil)

type authorizer struct {
	refuse error
	// refuseGrant answers only a request that hands out a role, as the authoriser's ceiling does.
	refuseGrant error
	// refuseRole answers access.role_above_own to a request at stake for that role only, as the
	// ceiling does for an administrator and OWNER.
	refuseRole domain.Role
	requests   []access.Request
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.requests = append(a.requests, request)
	if request.Grants != "" && a.refuseGrant != nil {
		return a.refuseGrant
	}
	if a.refuseRole != "" && request.Grants == a.refuseRole {
		return shared.ErrForbidden.WithDetail("access.role_above_own").
			WithParams(map[string]string{"role": string(a.refuseRole)})
	}
	return a.refuse
}

type auditSink struct{ entries []audit.Entry }

// Append refuses what the real sink refuses, so that an entry the trail would reject - one without
// a workspace, say - fails here as it fails on a real server.
func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	if err := entry.Validate(); err != nil {
		return err
	}
	s.entries = append(s.entries, entry)
	return nil
}

type notifier struct{ requests []queue.Request }

func (n *notifier) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	n.requests = append(n.requests, request)
	return shared.MustParseID("0192f000-0000-7000-8000-0000000000b1"), nil
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

func admin() appshared.ActorContext {
	return appshared.ActorContext{
		TenantID: tenant, AccountID: adminID, AccountName: "Anna",
		Kind: shared.ActorUser,
	}
}

func inviteHandler(accounts *accountStore, auth *authorizer, sink *auditSink, queued *notifier) InviteAccount {
	return InviteAccount{
		Accounts: accounts, Authorizer: auth, Notifier: queued, Audit: sink,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: invitedID},
	}
}

// The ordinary case, and everything an invitation owes: the account, the message, the entry.
func TestAnInvitationCreatesAnAccountThatCannotYetAct(t *testing.T) {
	accounts, auth, sink, queued := newAccounts(), &authorizer{}, &auditSink{}, &notifier{}

	invitation, err := inviteHandler(accounts, auth, sink, queued).
		Execute(t.Context(), admin(), InviteAccountCommand{Email: "Bert@Example.ORG"})
	if err != nil {
		t.Fatalf("inviting: %v", err)
	}
	account := invitation.Account

	if account.Status != domain.AccountInvited {
		t.Errorf("status %q, want INVITED", account.Status)
	}
	if account.Email != "bert@example.org" {
		t.Errorf("email %q, want it normalised", account.Email)
	}
	if len(accounts.inserted) != 1 {
		t.Fatalf("%d accounts written, want one", len(accounts.inserted))
	}
	if len(queued.requests) != 1 || queued.requests[0].Kind != queue.KindInvitationEmail {
		t.Errorf("queued %v, want an invitation message", queued.requests)
	}
	if len(sink.entries) != 1 || sink.entries[0].Action != AccountInvitedAction {
		t.Fatalf("audit entries %v, want one invitation", sink.entries)
	}
}

// The message carries identifiers and nothing else. A payload sits in a table nothing cleans, and
// an address in it is personal data outliving the account it belongs to (rule 10).
func TestTheQueuedMessageCarriesNoPersonalData(t *testing.T) {
	queued := &notifier{}

	if _, err := inviteHandler(newAccounts(), &authorizer{}, &auditSink{}, queued).
		Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"}); err != nil {
		t.Fatalf("inviting: %v", err)
	}

	payload := queued.requests[0].Payload
	if payload["account_id"] != invitedID.String() {
		t.Errorf("payload %v, want the account identifier", payload)
	}
	for field, value := range payload {
		if text, ok := value.(string); ok && text == "bert@example.org" {
			t.Errorf("the payload carries the address in %q", field)
		}
	}
}

// The trail records that an invitation happened, and hashes where it went.
func TestTheTrailRecordsTheInvitationWithoutTheAddress(t *testing.T) {
	sink := &auditSink{}

	if _, err := inviteHandler(newAccounts(), &authorizer{}, sink, &notifier{}).
		Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"}); err != nil {
		t.Fatalf("inviting: %v", err)
	}

	entry := sink.entries[0]
	if entry.Severity != audit.SeverityNotice {
		t.Errorf("severity %q, want NOTICE - somebody gained a way in", entry.Severity)
	}
	email, recorded := entry.Changes["email"].(map[string]any)
	if !recorded {
		t.Fatalf("the entry does not record the address at all: %v", entry.Changes)
	}
	if email["to"] == "bert@example.org" {
		t.Error("the trail carries the address in clear")
	}
	if email["changed"] != true {
		t.Errorf("the address is recorded as %v, want it masked", email)
	}
}

// Inviting somebody who is already here is the common mistake, and it gets a clear answer rather
// than a unique constraint surfacing from the depths - and nothing goes out: no second account, no
// mail, no link (UC-ID-14 check 6). An invitation still waiting is named as one, so the inviter's
// next step is a new link for it rather than a second account.
func TestInvitingSomebodyWhoIsAlreadyHereIsAConflict(t *testing.T) {
	cases := []struct {
		name   string
		status domain.AccountStatus
		links  bool
		want   string
	}{
		{"a person here, with mail", domain.AccountActive, false, "accounts.email_taken"},
		{"a person here, without mail", domain.AccountActive, true, "accounts.email_taken"},
		{"a disabled person, with mail", domain.AccountDisabled, false, "accounts.email_taken"},
		{"an invitation waiting, with mail", domain.AccountInvited, false, "accounts.already_invited"},
		{"an invitation waiting, without mail", domain.AccountInvited, true, "accounts.already_invited"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			existing, err := domain.Invite(shared.ID("01936f2a-7c1e-7000-8000-0000000000a9"),
				tenant, "bert@example.org", "Bert", nil, nil)
			if err != nil {
				t.Fatalf("preparing: %v", err)
			}
			existing.Status = c.status
			accounts, queued, minter := newAccounts(existing), &notifier{}, &linkMinter{}
			handler := inviteHandler(accounts, &authorizer{}, &auditSink{}, queued)
			if c.links {
				handler.Links = InvitationLinks{Minter: minter, BaseURL: "https://home.example"}
			}

			_, err = handler.Execute(t.Context(), admin(), InviteAccountCommand{Email: "BERT@example.org"})

			if got := detailOf(err); got != c.want {
				t.Fatalf("answered %q (%v), want %s", got, err, c.want)
			}
			if !errors.Is(err, shared.ErrConflict) {
				t.Errorf("category %v, want a conflict", err)
			}
			if shared.AsError(err).Params["account_id"] != existing.ID.String() {
				t.Errorf("params %v, want the account that holds the address", shared.AsError(err).Params)
			}
			if len(accounts.inserted) != 0 || len(queued.requests) != 0 || len(minter.minted) != 0 {
				t.Error("something went out for an address that is already here")
			}
		})
	}
}

// The permission is checked before anything is written, and a refusal writes nothing at all.
func TestAnInvitationNeedsThePermissionToManageMembers(t *testing.T) {
	accounts, sink, queued := newAccounts(), &auditSink{}, &notifier{}
	auth := &authorizer{refuse: shared.ErrForbidden.WithDetail("access.not_permitted")}

	_, err := inviteHandler(accounts, auth, sink, queued).
		Execute(t.Context(), admin(), InviteAccountCommand{Email: "bert@example.org"})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want the refusal", err)
	}
	if len(accounts.inserted) != 0 || len(queued.requests) != 0 {
		t.Error("a refused invitation wrote something")
	}
	if len(auth.requests) != 1 || auth.requests[0].Permission != "MANAGE_MEMBERS" {
		t.Errorf("asked for %v, want the member management permission", auth.requests)
	}
}

// A malformed address never reaches the database.
func TestAMalformedAddressIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	accounts := newAccounts()

	_, err := inviteHandler(accounts, &authorizer{}, &auditSink{}, &notifier{}).
		Execute(t.Context(), admin(), InviteAccountCommand{Email: "not an address"})

	if err == nil || shared.AsError(err).DetailCode != "accounts.email_malformed" {
		t.Fatalf("error %v, want the address refused", err)
	}
	if len(accounts.inserted) != 0 {
		t.Error("the account was written despite the refusal")
	}
}
