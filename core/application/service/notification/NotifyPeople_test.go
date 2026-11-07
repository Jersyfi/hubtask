// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"slices"
	"testing"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

var (
	hub   = shared.ID("01936f2a-7c1e-7000-8000-0000000000a4")
	group = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
)

// noticeAuthorizer permits the caller unless refused, lets `readers` read the entry, and answers
// the roles each account holds where the entry is.
type noticeAuthorizer struct {
	refuse  bool
	readers []shared.ID
	roles   map[shared.ID][]string
	paths   [][]identity.Scope
}

func (a *noticeAuthorizer) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	if a.refuse {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return nil
}

func (a *noticeAuthorizer) CanSee(_ context.Context, _ appshared.ActorContext, account shared.ID, path []identity.Scope) (bool, error) {
	a.paths = append(a.paths, path)
	return slices.Contains(a.readers, account), nil
}

func (a *noticeAuthorizer) RolesBy(_ context.Context, _ appshared.ActorContext, account shared.ID, _ []identity.Scope) ([]string, error) {
	return a.roles[account], nil
}

type collectionStore struct{}

func (collectionStore) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub}, nil
}

type groupBook map[shared.ID][]shared.ID

func (g groupBook) Find(_ context.Context, id shared.ID) (identity.Group, error) {
	if _, found := g[id]; !found {
		return identity.Group{}, shared.ErrNotFound
	}
	return identity.Group{ID: id, TenantID: tenant}, nil
}

func (g groupBook) Members(_ context.Context, id shared.ID) ([]shared.ID, error) { return g[id], nil }

type holderBook map[string][]shared.ID

func (h holderBook) RoleHolders(_ context.Context, _ []identity.Scope, role string) ([]shared.ID, error) {
	return h[role], nil
}

type customRoleBook map[string]bool

func (c customRoleBook) Find(_ context.Context, key string) (identityrepo.CustomRoleRow, error) {
	if !c[key] {
		return identityrepo.CustomRoleRow{}, shared.ErrNotFound
	}
	return identityrepo.CustomRoleRow{}, nil
}

type auditLog struct{ entries []audit.Entry }

func (l *auditLog) Append(_ context.Context, entry audit.Entry) error {
	l.entries = append(l.entries, entry)
	return nil
}

type noticeFixture struct {
	desk          NoticeDesk
	notifications *notificationStore
	preferences   *preferenceStore
	jobs          *jobQueue
	authorizer    *noticeAuthorizer
	audit         *auditLog
	accounts      *accountStore
}

func noticeDesk(t *testing.T) noticeFixture {
	t.Helper()
	f := noticeFixture{
		notifications: newNotifications(),
		preferences:   newPreferences(),
		jobs:          &jobQueue{},
		authorizer:    &noticeAuthorizer{readers: []shared.ID{anna, bert, carla}},
		audit:         &auditLog{},
		accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "de"),
			person(bert, "Bert", "bert@example.org", "en"),
			person(carla, "Carla", "carla@example.org", "en"),
		),
	}
	f.desk = NoticeDesk{
		Notifications: f.notifications, Preferences: f.preferences, Accounts: f.accounts,
		Items: newItems(task("")), Containers: collectionStore{},
		Groups:      groupBook{group: {anna, bert, carla}},
		Holders:     holderBook{"ADMIN": {anna, bert}, "dispatcher": {carla}},
		CustomRoles: customRoleBook{"dispatcher": true},
		Restricted:  f.accounts, Authorizer: f.authorizer, Audit: f.audit, Jobs: f.jobs,
		UnitOfWork: &unitOfWork{}, Clock: clock.Fixed(now), IDs: &idSequence{},
	}
	return f
}

var noticeActor = appshared.ActorContext{
	TenantID: tenant, AccountID: shared.ID("01936f2a-7c1e-7000-8000-0000000000e1"),
	Kind: appshared.ActorServiceAccount, RunsForRule: shared.ID("01936f2a-7c1e-7000-8000-0000000000e2"),
}

func recipientsOf(records []domain.Notification) []shared.ID {
	var ids []shared.ID
	for _, record := range records {
		ids = append(ids, record.RecipientID)
	}
	slices.Sort(ids)
	return ids
}

// UC-AUT-09 check 1: a step names an account or a group; each member of a group is one recipient.
func TestANotificationStepTellsAnAccountOrEachMemberOfAGroup(t *testing.T) {
	f := noticeDesk(t)
	told, err := NotifyAccount{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, bert)
	if err != nil || told != 1 {
		t.Fatalf("account: told %d, %v", told, err)
	}

	f = noticeDesk(t)
	told, err = NotifyGroup{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, group)
	if err != nil || told != 3 {
		t.Fatalf("group: told %d, %v", told, err)
	}
	if got := recipientsOf(f.notifications.written()); !slices.Equal(got, []shared.ID{anna, bert, carla}) {
		t.Errorf("the records went to %v", got)
	}
	if len(f.jobs.requests) != 3 {
		t.Errorf("%d sends queued, want 3", len(f.jobs.requests))
	}
	for _, record := range f.notifications.written() {
		if record.Category != domain.CategorySLA || record.ItemID != itemID {
			t.Errorf("a record is not the entry's SLA notice: %+v", record)
		}
	}
	if len(f.audit.entries) != 1 || f.audit.entries[0].Action != RuleNoticeSentAction {
		t.Errorf("the step's audit entry: %+v", f.audit.entries)
	}
}

// UC-AUT-09 check 2: the recipient's preference applies - a member who switched SLA off gets a
// suppressed record and no send.
func TestARecipientWhoSwitchedTheCategoryOffIsNotSent(t *testing.T) {
	f := noticeDesk(t)
	f.preferences.switchOff(bert, domain.CategorySLA)
	if _, err := (NotifyGroup{Desk: f.desk}).Execute(t.Context(), noticeActor, itemID, group); err != nil {
		t.Fatal(err)
	}
	if len(f.jobs.requests) != 2 {
		t.Errorf("%d sends queued, want 2", len(f.jobs.requests))
	}
	for _, record := range f.notifications.written() {
		if record.RecipientID == bert && record.State != domain.StateSuppressed {
			t.Errorf("bert's record is %s, want suppressed", record.State)
		}
	}
}

// UC-AUT-09 check 3: a recipient who may not read the entry is not told; the path asked ends at
// the entry.
func TestARecipientWhoMayNotReadTheEntryIsNotTold(t *testing.T) {
	f := noticeDesk(t)
	f.authorizer.readers = []shared.ID{anna}
	told, err := NotifyGroup{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, group)
	if err != nil || told != 1 {
		t.Fatalf("told %d, %v", told, err)
	}
	if got := recipientsOf(f.notifications.written()); !slices.Equal(got, []shared.ID{anna}) {
		t.Errorf("the records went to %v", got)
	}
	last := f.authorizer.paths[len(f.authorizer.paths)-1]
	if last[len(last)-1] != identity.ItemScope(itemID) {
		t.Errorf("the read was asked about %v, not the entry", last)
	}
}

// UC-AUT-09 check 4 at the run: a recipient the workspace does not have is refused, and nothing is
// written.
func TestAnUnknownRecipientIsRefused(t *testing.T) {
	for name, run := range map[string]func(NoticeDesk) error{
		"account": func(d NoticeDesk) error {
			_, err := NotifyAccount{Desk: d}.Execute(t.Context(), noticeActor, itemID, shared.ID("01936f2a-7c1e-7000-8000-0000000000f9"))
			return err
		},
		"group": func(d NoticeDesk) error {
			_, err := NotifyGroup{Desk: d}.Execute(t.Context(), noticeActor, itemID, shared.ID("01936f2a-7c1e-7000-8000-0000000000f8"))
			return err
		},
		"role": func(d NoticeDesk) error {
			_, err := NotifyRole{Desk: d}.Execute(t.Context(), noticeActor, itemID, "nobody_has_this")
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := noticeDesk(t)
			err := run(f.desk)
			var refusal *shared.Error
			if !errors.As(err, &refusal) || refusal.DetailCode != RecipientUnknownCode {
				t.Fatalf("got %v, want %s", err, RecipientUnknownCode)
			}
			if len(f.notifications.written()) != 0 {
				t.Errorf("a refused step wrote %d records", len(f.notifications.written()))
			}
		})
	}
}

// NOTIFY_ROLE (fourth answers 6): the holders of a built-in or a custom role along the path, only
// where the role counts there.
func TestARoleStepTellsItsHoldersWhereTheRoleCounts(t *testing.T) {
	f := noticeDesk(t)
	// Bert's ADMIN is held on the workspace and does not count in this (private) hub.
	f.authorizer.roles = map[shared.ID][]string{anna: {"ADMIN"}, bert: {}, carla: {"dispatcher"}}
	told, err := NotifyRole{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, "ADMIN")
	if err != nil || told != 1 {
		t.Fatalf("built-in: told %d, %v", told, err)
	}
	if got := recipientsOf(f.notifications.written()); !slices.Equal(got, []shared.ID{anna}) {
		t.Errorf("the records went to %v", got)
	}

	f2 := noticeDesk(t)
	f2.authorizer.roles = f.authorizer.roles
	told, err = NotifyRole{Desk: f2.desk}.Execute(t.Context(), noticeActor, itemID, "dispatcher")
	if err != nil || told != 1 {
		t.Fatalf("custom: told %d, %v", told, err)
	}
}

// A person whose processing is restricted is left out of a rule's message (data-protection.md §4).
func TestARestrictedPersonIsNotToldByARule(t *testing.T) {
	f := noticeDesk(t)
	restricted := f.accounts.byID[carla]
	restricted.Status = identity.AccountRestricted
	f.accounts.byID[carla] = restricted
	told, err := NotifyGroup{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, group)
	if err != nil || told != 2 {
		t.Fatalf("told %d, %v", told, err)
	}
}

// The caller's own permission is asked first; a refusal writes nothing.
func TestARefusedCallerTellsNobody(t *testing.T) {
	f := noticeDesk(t)
	f.authorizer.refuse = true
	if _, err := (NotifyAccount{Desk: f.desk}).Execute(t.Context(), noticeActor, itemID, anna); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("got %v, want forbidden", err)
	}
	if len(f.notifications.written()) != 0 || len(f.jobs.requests) != 0 {
		t.Error("a refused caller's step wrote something")
	}
}

// A step that would tell more than the bound is refused whole, before anything is written.
func TestAStepBeyondTheBoundIsRefusedWhole(t *testing.T) {
	f := noticeDesk(t)
	var many []shared.ID
	for i := range MaxNoticeRecipients + 1 {
		id := (&idSequence{next: 1000 + i}).NewID()
		many = append(many, id)
		f.accounts.byID[id] = person(id, "x", "x@example.org", "en")
	}
	f.authorizer.readers = many
	f.desk.Groups = groupBook{group: many}
	_, err := NotifyGroup{Desk: f.desk}.Execute(t.Context(), noticeActor, itemID, group)
	var refusal *shared.Error
	if !errors.As(err, &refusal) || refusal.DetailCode != TooManyRecipientsCode {
		t.Fatalf("got %v, want %s", err, TooManyRecipientsCode)
	}
	if len(f.notifications.written()) != 0 {
		t.Error("a refused step wrote records")
	}
}

// The three are actions under the names the owner chose (fourth answers 6, UC-AUT-09).
func TestTheNotificationStepsAreTheirActions(t *testing.T) {
	for want, kind := range map[string]string{
		"NOTIFY_ACCOUNT": NotifyAccount{}.Descriptor().AutomationAction(),
		"NOTIFY_GROUP":   NotifyGroup{}.Descriptor().AutomationAction(),
		"NOTIFY_ROLE":    NotifyRole{}.Descriptor().AutomationAction(),
	} {
		if kind != want {
			t.Errorf("action %q, want %q", kind, want)
		}
	}
}
