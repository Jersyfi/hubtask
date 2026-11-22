// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	mailservice "github.com/Jersyfi/hubtask/core/application/service/mail"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// Connecting a mailbox and checking it against a real database: the mailbox is stored and its
// read is queued once, in the same transaction - the queue refuses a write outside one.

type permitIntake struct{}

func (permitIntake) Authorize(context.Context, appshared.ActorContext, access.Request) error {
	return nil
}

type roomForMailboxes struct{}

func (roomForMailboxes) Mailboxes(context.Context, string) error { return nil }

type signingIn struct{}

func (signingIn) Open(context.Context, mailboxport.Connection) (mailboxport.Session, error) {
	return signedIn{}, nil
}

type signedIn struct{}

func (signedIn) Fetch(context.Context, int) ([]mailboxport.Message, error) { return nil, nil }
func (signedIn) Done(context.Context, string) error                        { return nil }
func (signedIn) Close() error                                              { return nil }

type plainSealer struct{}

func (plainSealer) Seal(_ context.Context, plaintext secret.Secret, _ crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: "k1", Ciphertext: []byte(plaintext.Reveal())}, nil
}

func (plainSealer) Open(_ context.Context, sealed crypto.Sealed, _ crypto.Purpose) (secret.Secret, error) {
	return secret.New(string(sealed.Ciphertext)), nil
}

func mailboxPolls(ctx context.Context, t *testing.T, tenant, box shared.ID) int {
	t.Helper()
	var count int
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT count(*) FROM job WHERE tenant_id = $1 AND kind = 'mail.mailbox_poll' AND dedupe_key = $2`,
		tenant.String(), box.String()).Scan(&count); err != nil {
		t.Fatalf("counting the polls: %v", err)
	}
	return count
}

func TestConnectingAndCheckingAMailboxQueuesOneRead(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	boxes := mailservice.Boxes{
		Writer: mailservice.Writer{
			Authorizer: permitIntake{}, UnitOfWork: postgres.NewUnitOfWork(appPool(ctx, t)),
			Clock: clockadapter.System{},
		},
		Mailboxes: postgres.NewMailboxRepository(), Reader: signingIn{}, Encryptor: plainSealer{},
		Jobs: jobQueue(t), IDs: ids, Quota: roomForMailboxes{},
	}
	person := appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenantB, AccountID: authorB}

	out, err := mailservice.ConnectMailbox{Boxes: boxes}.Descriptor().Handler.Invoke(ctx, person, usecase.Input{
		"preset": "gmail", "address": lettersOf(freshID(t)) + "@gmail.com", "secret": "app-pw",
	})
	if err != nil {
		t.Fatalf("connecting: %v", err)
	}
	box := shared.MustParseID(out["id"].(string))
	t.Cleanup(func() {
		_, _ = mailservice.RemoveMailbox{Boxes: boxes}.Descriptor().Handler.Invoke(ctx, person,
			usecase.Input{"mailbox_id": box.String()})
	})
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		_, _, err := boxes.Mailboxes.Find(ctx, box)
		return err
	}); err != nil {
		t.Fatalf("the connected mailbox is not stored: %v", err)
	}
	if n := mailboxPolls(ctx, t, tenantB, box); n != 1 {
		t.Fatalf("connecting queued %d reads, want 1", n)
	}

	if _, err := (mailservice.CheckMailbox{Boxes: boxes}).Descriptor().Handler.Invoke(ctx, person,
		usecase.Input{"mailbox_id": box.String()}); err != nil {
		t.Fatalf("checking: %v", err)
	}
	if n := mailboxPolls(ctx, t, tenantB, box); n != 1 {
		t.Fatalf("after a check %d reads are queued, want 1", n)
	}
}
