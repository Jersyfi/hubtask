// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The notification record and the preferences against a real database (C-09): the deduplication
// the outbox's at-least-once delivery needs, the outcome a delivery writes back, the retention
// batch, and a cross-tenant negative for every method (gate SG-3).

func notificationRepo() postgres.NotificationRepository {
	return postgres.NewNotificationRepository()
}

func preferenceRepo() postgres.NotificationPreferenceRepository {
	return postgres.NewNotificationPreferenceRepository()
}

func pendingNotification(t *testing.T, tenant, recipient, actor, event shared.ID) domain.Notification {
	t.Helper()
	record, err := domain.New(domain.NewInput{
		ID: freshID(t), TenantID: tenant, RecipientID: recipient,
		Category: domain.CategoryComment, Channel: domain.ChannelEmail,
		EventID: event, ActorID: actor, At: created,
	})
	if err != nil {
		t.Fatalf("building the record: %v", err)
	}
	return record
}

func insertNotification(
	ctx context.Context, t *testing.T, tenant shared.ID, record domain.Notification,
) bool {
	t.Helper()
	var written bool
	if err := write(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		written, err = notificationRepo().Insert(ctx, record)
		return err
	}); err != nil {
		t.Fatalf("writing the notification: %v", err)
	}
	return written
}

func findNotification(
	ctx context.Context, t *testing.T, tenant, id shared.ID,
) (domain.Notification, error) {
	t.Helper()
	var stored domain.Notification
	err := read(ctx, t, tenant, func(ctx context.Context) error {
		var readErr error
		stored, readErr = notificationRepo().Find(ctx, id)
		return readErr
	})
	return stored, err
}

func TestANotificationIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)

	record := pendingNotification(t, tenantA, recipient, authorA, freshID(t))
	if !insertNotification(ctx, t, tenantA, record) {
		t.Fatal("the first write reported that somebody else had already made the record")
	}

	stored, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if stored.State != domain.StatePending || stored.Reason != "" {
		t.Errorf("stored %+v - a new record is pending and unexplained", stored)
	}
	if stored.RecipientID != recipient || stored.ActorID != authorA {
		t.Errorf("the references did not survive: %+v", stored)
	}
	if stored.EventID != record.EventID || stored.Attempts != 0 || stored.SentAt != nil {
		t.Errorf("stored %+v", stored)
	}
	if !stored.CreatedAt.Equal(created) {
		t.Errorf("created at %v, want %v", stored.CreatedAt, created)
	}
}

// The correction for at-least-once delivery, at the table rather than in a consumer: two
// dispatchers reacting to one event both write, and only one row exists (ADR-0007).
func TestTheSameEventNotifiesSomebodyOnlyOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)
	event := freshID(t)

	first := pendingNotification(t, tenantA, recipient, authorA, event)
	second := pendingNotification(t, tenantA, recipient, authorA, event)

	if !insertNotification(ctx, t, tenantA, first) {
		t.Fatal("the first write did not write")
	}
	if insertNotification(ctx, t, tenantA, second) {
		t.Error("the second delivery of the same event wrote a second record")
	}
	if _, err := findNotification(ctx, t, tenantA, second.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the second record exists after all: %v", err)
	}

	// Somebody else on the same event is a different notification, not a duplicate.
	other := seedAccount(ctx, t, tenantA)
	if !insertNotification(ctx, t, tenantA, pendingNotification(t, tenantA, other, authorA, event)) {
		t.Error("a second recipient of the same event was swallowed as a duplicate")
	}
}

// The invitation carries no event, so the partial index does not apply to it - and two invitations
// must not collapse into one just because both have a NULL where the index looks.
func TestRecordsWithoutAnEventAreNotDeduplicated(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)

	for range 2 {
		record, err := domain.New(domain.NewInput{
			ID: freshID(t), TenantID: tenantA, RecipientID: recipient,
			Category: domain.CategoryInvitation, Channel: domain.ChannelEmail, At: created,
		})
		if err != nil {
			t.Fatal(err)
		}
		if !insertNotification(ctx, t, tenantA, record) {
			t.Fatal("an invitation was swallowed as a duplicate of another invitation")
		}
	}
}

func TestTheOutcomeOfADeliveryIsWrittenBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)

	record := pendingNotification(t, tenantA, recipient, authorA, freshID(t))
	insertNotification(ctx, t, tenantA, record)

	sentAt := created.Add(time.Minute)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return notificationRepo().Save(ctx, record.Failed(false).Sent(sentAt))
	}); err != nil {
		t.Fatalf("recording the outcome: %v", err)
	}

	stored, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if stored.State != domain.StateSent || stored.SentAt == nil || !stored.SentAt.Equal(sentAt) {
		t.Errorf("stored %+v", stored)
	}
	if stored.Attempts != 1 {
		t.Errorf("attempts %d, want 1", stored.Attempts)
	}
	// The recipient is not in the UPDATE's SET list: a delivery may change what happened to a
	// notification and never who it is for.
	if stored.RecipientID != recipient {
		t.Errorf("the recipient moved to %q", stored.RecipientID)
	}
}

// A record the retention sweep reached while its delivery was in flight is gone, and the delivery
// is told so rather than believing it wrote something.
func TestWritingBackToARecordThatIsGoneIsNotFound(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	record := pendingNotification(t, tenantA, seedAccount(ctx, t, tenantA), authorA, freshID(t))
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return notificationRepo().Save(ctx, record.Sent(created))
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("writing into nothing reported %v, want ErrNotFound", err)
	}
}

func TestTheRetentionSweepTakesOneBatchOfTheOldest(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)

	// Three expired and one fresh. The cutoff sits between them.
	expired := make([]domain.Notification, 0, 3)
	for i := range 3 {
		record := pendingNotification(t, tenantA, recipient, authorA, freshID(t))
		record.CreatedAt = created.Add(-time.Duration(i+1) * time.Hour)
		insertNotification(ctx, t, tenantA, record)
		expired = append(expired, record)
	}
	fresh := pendingNotification(t, tenantA, recipient, authorA, freshID(t))
	insertNotification(ctx, t, tenantA, fresh)

	cutoff := created.Add(-30 * time.Minute)

	var due, removed int
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		var err error
		if due, err = notificationRepo().CountExpired(ctx, cutoff, 3); err != nil {
			return err
		}
		removed, err = notificationRepo().DeleteExpired(ctx, cutoff, 2)
		return err
	}); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if due != 3 {
		t.Errorf("%d records due, want 3", due)
	}
	if removed != 2 {
		t.Errorf("%d removed, want the batch of 2", removed)
	}
	// Oldest first: the two eldest went and the third is still there.
	if _, err := findNotification(ctx, t, tenantA, expired[2].ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the oldest record survived the batch: %v", err)
	}
	if _, err := findNotification(ctx, t, tenantA, expired[0].ID); err != nil {
		t.Errorf("the batch reached past its size: %v", err)
	}
	if _, err := findNotification(ctx, t, tenantA, fresh.ID); err != nil {
		t.Errorf("an unexpired record was swept: %v", err)
	}
}

func TestAPreferenceIsWrittenAndReadBack(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	account := seedAccount(ctx, t, tenantA)

	// Saying nothing is not a row, and the repository says so rather than inventing the default -
	// the default is the domain's (notification.DefaultPreference).
	err := read(ctx, t, tenantA, func(ctx context.Context) error {
		_, findErr := preferenceRepo().
			Find(ctx, account, domain.CategoryComment, domain.ChannelEmail)
		return findErr
	})
	if !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an account that has said nothing reported %v, want ErrNotFound", err)
	}

	off := domain.DefaultPreference(tenantA, account, domain.CategoryComment, domain.ChannelEmail)
	off.Enabled = false
	off.UpdatedAt = created
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return preferenceRepo().Save(ctx, off)
	}); err != nil {
		t.Fatalf("saving: %v", err)
	}

	var stored domain.Preference
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = preferenceRepo().
			Find(ctx, account, domain.CategoryComment, domain.ChannelEmail)
		return findErr
	}); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if stored.Enabled {
		t.Error("the switch did not stick")
	}
	if !stored.IncludeTitle {
		t.Error("switching the category off also withheld the title")
	}

	// Saying it twice is the same statement, not a second row.
	off.IncludeTitle = false
	off.UpdatedAt = created.Add(time.Hour)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return preferenceRepo().Save(ctx, off)
	}); err != nil {
		t.Fatalf("saving again: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		var findErr error
		stored, findErr = preferenceRepo().
			Find(ctx, account, domain.CategoryComment, domain.ChannelEmail)
		return findErr
	}); err != nil {
		t.Fatalf("reading it back: %v", err)
	}
	if stored.IncludeTitle {
		t.Error("the second write did not replace the first")
	}
}

// Gate SG-3: every method of a new repository refuses to reach across the tenant boundary. Not one
// test per method by hand, because the point is the list being complete - a method added later and
// left out of a hand-written list is exactly the one that would leak.
func TestNoNotificationMethodReachesIntoAnotherTenant(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)

	recipient := seedAccount(ctx, t, tenantA)
	record := pendingNotification(t, tenantA, recipient, authorA, freshID(t))
	insertNotification(ctx, t, tenantA, record)

	preference := domain.DefaultPreference(
		tenantA, recipient, domain.CategoryComment, domain.ChannelEmail)
	preference.UpdatedAt = created
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return preferenceRepo().Save(ctx, preference)
	}); err != nil {
		t.Fatalf("seeding the preference: %v", err)
	}

	for _, tc := range []struct {
		method string
		call   func(context.Context) error
	}{
		{"Find", func(ctx context.Context) error {
			_, err := notificationRepo().Find(ctx, record.ID)
			return err
		}},
		{"Save", func(ctx context.Context) error {
			return notificationRepo().Save(ctx, record.Sent(created))
		}},
		{"Preferences.Find", func(ctx context.Context) error {
			_, err := preferenceRepo().
				Find(ctx, recipient, domain.CategoryComment, domain.ChannelEmail)
			return err
		}},
	} {
		t.Run(tc.method, func(t *testing.T) {
			// Tenant B's transaction, tenant A's identifiers. Nothing may come back.
			err := write(ctx, t, tenantB, tc.call)
			if err == nil {
				t.Errorf("%s answered across the tenant boundary", tc.method)
			}
			if !errors.Is(err, shared.ErrNotFound) && !errors.Is(err, shared.ErrUnavailable) {
				t.Errorf("%s failed with %v, want a refusal", tc.method, err)
			}
		})
	}

	// The sweep is not refused, it simply finds nothing: row level security narrows what it
	// deletes rather than making the statement fail, which is the stronger property - a sweep that
	// errored across the boundary would still have had to be trusted not to match.
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		due, err := notificationRepo().CountExpired(ctx, created.Add(time.Hour), 100)
		if err != nil {
			return err
		}
		if due != 0 {
			t.Errorf("CountExpired saw %d of another tenant's records", due)
		}
		removed, err := notificationRepo().DeleteExpired(ctx, created.Add(time.Hour), 100)
		if err != nil {
			return err
		}
		if removed != 0 {
			t.Errorf("DeleteExpired removed %d of another tenant's records", removed)
		}
		return nil
	}); err != nil {
		t.Fatalf("sweeping in the other tenant: %v", err)
	}
	if _, err := findNotification(ctx, t, tenantA, record.ID); err != nil {
		t.Errorf("the other tenant's sweep reached this record: %v", err)
	}

	// The two writes are their own case: they do not read across the boundary, they would write
	// into it - and what stops them is the tenant-scoped foreign key, because tenant B holds no
	// account with that identifier (ADR-0024).
	stray := pendingNotification(t, tenantB, recipient, authorA, freshID(t))
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, insertErr := notificationRepo().Insert(ctx, stray)
		return insertErr
	}); err == nil {
		t.Error("Insert wrote a record for another tenant's account")
	}

	strayPreference := domain.DefaultPreference(
		tenantB, recipient, domain.CategoryComment, domain.ChannelEmail)
	strayPreference.UpdatedAt = created
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return preferenceRepo().Save(ctx, strayPreference)
	}); err == nil {
		t.Error("Preferences.Save wrote a row for another tenant's account")
	}
}
