// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/mail"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// A notice about a failing mailbox against a real database: the row carries the mailbox as its
// subject through migration 0338's foreign key, another workspace cannot read it, and removing the
// mailbox removes the notice with it.
func TestAFailingMailboxNoticeNamesItsMailboxAndGoesWithIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	recipient := seedAccount(ctx, t, tenantA)
	boxes := postgres.NewMailboxRepository()
	box, err := mail.NewMailbox(mail.NewMailboxInput{
		ID: freshID(t), TenantID: tenantA, Preset: "gmail", Address: lettersOf(freshID(t)) + "@gmail.com",
		Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return boxes.Insert(ctx, box, crypto.Sealed{KeyID: "k1", Ciphertext: []byte{1}}, authorA)
	}); err != nil {
		t.Fatalf("storing the mailbox: %v", err)
	}

	record, err := domain.New(domain.NewInput{
		ID: freshID(t), TenantID: tenantA, RecipientID: recipient,
		Category: domain.CategoryIntegration, Channel: domain.ChannelEmail,
		MailboxID: box.ID, At: created,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !insertNotification(ctx, t, tenantA, record) {
		t.Fatal("the first write reported an existing record")
	}
	stored, err := findNotification(ctx, t, tenantA, record.ID)
	if err != nil || stored.MailboxID != box.ID {
		t.Fatalf("read back mailbox %v, %v; want %v", stored.MailboxID, err, box.ID)
	}
	if _, err := findNotification(ctx, t, tenantB, record.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B read A's notice: %v", err)
	}

	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return boxes.Delete(ctx, box.ID)
	}); err != nil {
		t.Fatalf("removing the mailbox: %v", err)
	}
	if _, err := findNotification(ctx, t, tenantA, record.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("the notice outlived its mailbox: %v", err)
	}
}
