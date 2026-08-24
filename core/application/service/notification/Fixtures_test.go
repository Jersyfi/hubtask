// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"fmt"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// The fixtures shared by the consumer's and the delivery's tests: the repositories in memory, the
// accounts, and the entry a notification is about.

var (
	tenant     = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")
	collection = shared.ID("01936f2a-7c1e-7000-8000-0000000000a2")
	itemID     = shared.ID("01936f2a-7c1e-7000-8000-0000000000a3")
	anna       = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	bert       = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	carla      = shared.ID("01936f2a-7c1e-7000-8000-0000000000b3")
	eventID    = shared.ID("01936f2a-7c1e-7000-8000-0000000000c1")
	now        = time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
)

// notificationStore is the record repository, in memory, with the unique index the table carries.
type notificationStore struct {
	records map[shared.ID]domain.Notification
	order   []shared.ID
	// deduped is the partial unique index over (event, recipient, channel), keyed the same way.
	deduped   map[string]bool
	insertErr error
	saveErr   error
}

func newNotifications() *notificationStore {
	return &notificationStore{
		records: map[shared.ID]domain.Notification{}, deduped: map[string]bool{},
	}
}

func (s *notificationStore) Insert(_ context.Context, record domain.Notification) (bool, error) {
	if s.insertErr != nil {
		return false, s.insertErr
	}
	if !record.EventID.IsZero() {
		key := fmt.Sprintf("%s|%s|%s", record.EventID, record.RecipientID, record.Channel)
		if s.deduped[key] {
			return false, nil
		}
		s.deduped[key] = true
	}
	s.records[record.ID] = record
	s.order = append(s.order, record.ID)
	return true, nil
}

func (s *notificationStore) Find(_ context.Context, id shared.ID) (domain.Notification, error) {
	record, found := s.records[id]
	if !found {
		return domain.Notification{}, shared.ErrNotFound
	}
	return record, nil
}

func (s *notificationStore) Save(_ context.Context, record domain.Notification) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	if _, found := s.records[record.ID]; !found {
		return shared.ErrNotFound
	}
	s.records[record.ID] = record
	return nil
}

func (s *notificationStore) DeleteExpired(_ context.Context, cutoff time.Time, batch int) (int, error) {
	removed := 0
	for _, id := range append([]shared.ID(nil), s.order...) {
		if removed >= batch {
			break
		}
		if s.records[id].CreatedAt.Before(cutoff) {
			delete(s.records, id)
			removed++
		}
	}
	return removed, nil
}

func (s *notificationStore) CountExpired(_ context.Context, cutoff time.Time, ceiling int) (int, error) {
	due := 0
	for _, record := range s.records {
		if record.CreatedAt.Before(cutoff) {
			due++
		}
		if due >= ceiling {
			break
		}
	}
	return due, nil
}

// written returns the records in the order they were inserted.
func (s *notificationStore) written() []domain.Notification {
	out := make([]domain.Notification, 0, len(s.order))
	for _, id := range s.order {
		if record, found := s.records[id]; found {
			out = append(out, record)
		}
	}
	return out
}

var _ repository.Notifications = (*notificationStore)(nil)

// preferenceStore is what people have said, keyed the way the table's primary key is.
type preferenceStore struct{ rows map[string]domain.Preference }

func newPreferences() *preferenceStore {
	return &preferenceStore{rows: map[string]domain.Preference{}}
}

func preferenceKey(account shared.ID, category domain.Category, channel domain.Channel) string {
	return fmt.Sprintf("%s|%s|%s", account, category, channel)
}

func (s *preferenceStore) Find(
	_ context.Context, account shared.ID, category domain.Category, channel domain.Channel,
) (domain.Preference, error) {
	row, found := s.rows[preferenceKey(account, category, channel)]
	if !found {
		return domain.Preference{}, shared.ErrNotFound
	}
	return row, nil
}

func (s *preferenceStore) Save(_ context.Context, preference domain.Preference) error {
	s.rows[preferenceKey(preference.AccountID, preference.Category, preference.Channel)] = preference
	return nil
}

// switchOff is the acceptance criterion's setup: somebody who has said they do not want this.
func (s *preferenceStore) switchOff(account shared.ID, category domain.Category) {
	preference := domain.DefaultPreference(tenant, account, category, domain.ChannelEmail)
	preference.Enabled = false
	_ = s.Save(context.Background(), preference)
}

var _ repository.Preferences = (*preferenceStore)(nil)

// accountStore is the identity slice this package reads: who somebody is and where to reach them.
type accountStore struct {
	byID map[shared.ID]identity.Account
}

func newAccounts(accounts ...identity.Account) *accountStore {
	store := &accountStore{byID: map[shared.ID]identity.Account{}}
	for _, account := range accounts {
		store.byID[account.ID] = account
	}
	return store
}

func person(id shared.ID, name, email, locale string) identity.Account {
	return identity.Account{
		ID: id, TenantID: tenant, DisplayName: name, Email: email, Locale: locale,
		Status: identity.AccountActive,
	}
}

func (s *accountStore) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	account, found := s.byID[id]
	if !found {
		return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

func (s *accountStore) FindByEmail(context.Context, string) (identity.Account, error) {
	return identity.Account{}, shared.ErrNotFound
}
func (s *accountStore) Insert(context.Context, identity.Account) error { return nil }
func (s *accountStore) UpdatePreferences(context.Context, identity.Account, time.Time) error {
	return nil
}

// itemStore is the entry slice: only Find is ever called, and the rest exists to satisfy the port.
type itemStore struct {
	item  work.WorkItem
	found bool
}

func newItems(item work.WorkItem) *itemStore { return &itemStore{item: item, found: true} }

func (s *itemStore) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	if !s.found || id != s.item.ID {
		return work.WorkItem{}, shared.ErrNotFound
	}
	return s.item, nil
}

// memberStore is one entry's member list.
type memberStore struct{ members []shared.ID }

func (s *memberStore) List(context.Context, shared.ID) ([]shared.ID, error) {
	return s.members, nil
}

var (
	_ Entries = (*itemStore)(nil)
	_ Members = (*memberStore)(nil)
)

// jobQueue records what was asked for.
type jobQueue struct {
	requests []queue.Request
	err      error
}

func (q *jobQueue) Enqueue(_ context.Context, request queue.Request) error {
	if q.err != nil {
		return q.err
	}
	q.requests = append(q.requests, request)
	return nil
}

// idSequence hands out a fresh identifier per call, so a test with three recipients gets three
// records rather than one written three times.
type idSequence struct{ next int }

func (s *idSequence) NewID() shared.ID {
	s.next++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8e00-%012x", s.next))
}

// task is the entry the notifications are about.
func task(assignee shared.ID) work.WorkItem {
	return work.WorkItem{
		ID: itemID, TenantID: tenant, CollectionID: collection, Type: work.ItemTask,
		Title: "Review the quote", AssigneeID: assignee, CreatedAt: now, UpdatedAt: now,
		Version: 1,
	}
}

// signalLog records what the metrics adapter would have been told.
type signalLog struct {
	recorded []string
	sent     []string
	failed   []string
}

func (s *signalLog) NotificationRecorded(_ context.Context, category, channel, state string) {
	s.recorded = append(s.recorded, fmt.Sprintf("%s/%s/%s", category, channel, state))
}

func (s *signalLog) NotificationSent(_ context.Context, category, channel string, _ float64) {
	s.sent = append(s.sent, fmt.Sprintf("%s/%s", category, channel))
}

func (s *signalLog) NotificationFailed(_ context.Context, category, channel, reason string) {
	s.failed = append(s.failed, fmt.Sprintf("%s/%s/%s", category, channel, reason))
}

var _ Signals = (*signalLog)(nil)
