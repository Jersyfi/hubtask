// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// The mail intake against a real database (ADR-0094): the store gives every collection a handle and
// every entry its number, the names route a message, the settings name one workspace on the
// receiving domain, and a message is taken once - every method bounded by row level security
// (gate SG-3).

func handleOf(ctx context.Context, t *testing.T, tenant, collection shared.ID) string {
	t.Helper()
	var found work.Container
	if err := read(ctx, t, tenant, func(ctx context.Context) error {
		var err error
		found, err = containerRepo().Find(ctx, collection)
		return err
	}); err != nil {
		t.Fatalf("reading the collection: %v", err)
	}
	return found.Handle
}

func TestEveryCollectionGetsAHandleAndEveryEntryANumber(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	hub := freshID(t)
	first, second := freshID(t), freshID(t)
	name := "Einkäufe " + freshName(t)[:6]
	var stored work.Container
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if _, err := containerRepo().Insert(ctx, containerIn(tenantA, authorA, hub, freshName(t), "a0")); err != nil {
			return err
		}
		var err error
		if stored, err = containerRepo().Insert(ctx, collectionIn(tenantA, authorA, first, hub, name, "a0")); err != nil {
			return err
		}
		// The same name under another spelling in the same hub is refused by the name index, so the
		// second collection shares only the handle's start.
		_, err = containerRepo().Insert(ctx, collectionIn(tenantA, authorA, second, hub, name+" Zwei", "a1"))
		return err
	}); err != nil {
		t.Fatalf("writing the collections: %v", err)
	}
	if stored.Handle == "" || stored.Handle != handleOf(ctx, t, tenantA, first) {
		t.Fatalf("the insert answered handle %q, the store holds %q", stored.Handle, handleOf(ctx, t, tenantA, first))
	}
	if !domain.ValidHandle(stored.Handle) || stored.Handle[:8] != "einkaufe" {
		t.Errorf("the handle %q is not the name folded", stored.Handle)
	}
	if handleOf(ctx, t, tenantA, second) == stored.Handle {
		t.Error("two collections share a handle")
	}

	// Numbers count per collection, from one.
	var numbers []int64
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		for index := range 3 {
			item, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, first, freshID(t), freshName(t), "a"+string(rune('0'+index))))
			if err != nil {
				return err
			}
			numbers = append(numbers, item.Number)
		}
		other, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, second, freshID(t), freshName(t), "a0"))
		numbers = append(numbers, other.Number)
		return err
	}); err != nil {
		t.Fatalf("writing the entries: %v", err)
	}
	if numbers[0] != 1 || numbers[1] != 2 || numbers[2] != 3 || numbers[3] != 1 {
		t.Errorf("numbers %v, want 1 2 3 and 1 in the other collection", numbers)
	}
}

func TestANameRoutesToItsPlaceAndAHandleChangesOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	_, collection := hubWithCollection(ctx, t, tenantA, authorA)
	handle := handleOf(ctx, t, tenantA, collection)
	entry := freshID(t)
	var number int64
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		item, err := itemRepo().Insert(ctx, taskIn(tenantA, authorA, collection, entry, freshName(t), "a0"))
		number = item.Number
		return err
	}); err != nil {
		t.Fatal(err)
	}
	store := postgres.NewMailIntakeRepository()
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		if found, err := store.CollectionByHandle(ctx, handle); err != nil || found != collection {
			t.Errorf("the handle named %s, %v", found, err)
		}
		if found, err := store.ItemByKey(ctx, domain.Key{Handle: handle, Number: number}); err != nil || found != entry {
			t.Errorf("the key named %s, %v", found, err)
		}
		if key, err := store.KeyOf(ctx, entry); err != nil || key.Handle != handle || key.Number != number {
			t.Errorf("the entry's key %+v, %v", key, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Another workspace does not see the name.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, err := store.CollectionByHandle(ctx, handle); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B resolved A's handle: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// A new handle; one another collection holds is a conflict.
	_, other := hubWithCollection(ctx, t, tenantA, authorA)
	renamed := "neu" + lettersOf(freshID(t))
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.SetHandle(ctx, collection, renamed)
	}); err != nil {
		t.Fatalf("setting the handle: %v", err)
	}
	err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.SetHandle(ctx, other, renamed)
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a taken handle answered %v", err)
	}
}

func TestTheSettingsNameOneWorkspaceOnTheDomain(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewMailIntakeRepository()
	name := "familie-" + lettersOf(freshID(t))
	settings := domain.Settings{
		Enabled: true, Name: name,
		Senders:      domain.Senders{Level: domain.LevelAllowed, Allowed: []string{"@lieferant.de"}, Blocked: []string{"x@spam.de"}},
		SenderRoutes: []domain.SenderRoute{{Match: "@lieferant.de", CollectionID: freshID(t)}},
		OpenedBy:     authorA,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Save(ctx, settings, authorA, time.Now())
	}); err != nil {
		t.Fatalf("saving: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, err := store.Find(ctx)
		if err != nil || found.Name != name || found.Senders.Level != domain.LevelAllowed ||
			len(found.SenderRoutes) != 1 || found.OpenedBy != authorA || found.Senders.Blocked[0] != "x@spam.de" {
			t.Errorf("found %+v, %v", found, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	named := func() (shared.ID, error) {
		var found shared.ID
		err := postgres.NewUnitOfWork(appPool(ctx, t)).WithinReadOnly(ctx, persistence.InstallationScope(),
			func(ctx context.Context) error {
				var err error
				found, err = store.WorkspaceNamed(ctx, name)
				return err
			})
		return found, err
	}
	if found, err := named(); err != nil || found != tenantA {
		t.Errorf("the name named %s, %v", found, err)
	}
	// Another workspace may not take the name.
	err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.Save(ctx, domain.Settings{Enabled: true, Name: name, Senders: domain.Senders{Level: domain.LevelMembers}}, authorB, time.Now())
	})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a taken name answered %v", err)
	}
	// Switched off, the name names nobody.
	settings.Enabled = false
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Save(ctx, settings, authorA, time.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := named(); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("an intake that is off answered %v", err)
	}
}

func TestAMessageIsReceivedOnceAndForgottenAfterItsPeriod(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewMailIntakeRepository()
	messageID := "<" + freshName(t) + "@example.org>"
	at := time.Now().UTC().Truncate(time.Microsecond)
	receipt := domain.Receipt{
		ID: freshID(t), MessageID: messageID, ReceivedAt: at.Add(-40 * 24 * time.Hour), Sender: "a@b.de",
		Subject: "Hallo", Outcome: domain.OutcomeJumble, Reason: domain.ReasonNoRoute, EntryID: freshID(t), Door: "inbound",
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if recorded, err := store.Record(ctx, receipt); err != nil || !recorded {
			t.Errorf("the first receipt: %v, %v", recorded, err)
		}
		again := receipt
		again.ID = freshID(t)
		if recorded, err := store.Record(ctx, again); err != nil || recorded {
			t.Errorf("the second receipt: %v, %v", recorded, err)
		}
		if seen, err := store.Seen(ctx, messageID); err != nil || !seen {
			t.Errorf("seen %v, %v", seen, err)
		}
		if sent, err := store.CountFrom(ctx, "a@b.de", at.Add(-50*24*time.Hour)); err != nil || sent < 1 {
			t.Errorf("counted %d, %v", sent, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	// Tenant B neither sees it nor counts it.
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if seen, _ := store.Seen(ctx, messageID); seen {
			t.Error("tenant B saw A's message")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		due, err := store.CountExpired(ctx, at.Add(-30*24*time.Hour), 100)
		if err != nil || due < 1 {
			t.Errorf("due %d, %v", due, err)
		}
		removed, err := store.DeleteExpired(ctx, at.Add(-30*24*time.Hour), 100)
		if err != nil || removed < 1 {
			t.Errorf("removed %d, %v", removed, err)
		}
		if seen, _ := store.Seen(ctx, messageID); seen {
			t.Error("a receipt past its period is still there")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

// lettersOf is an identifier as lowercase letters, for a handle or a name that must hold nothing else.
func lettersOf(id shared.ID) string {
	out := make([]byte, 0, 12)
	for _, r := range id.String() {
		if len(out) == 12 {
			break
		}
		switch {
		case r >= 'a' && r <= 'z':
			out = append(out, byte(r))
		case r >= '0' && r <= '9':
			out = append(out, byte('a'+(r-'0')))
		}
	}
	return string(out)
}

func TestAMailboxIsStoredWithItsSecretSealedAndOnlyItsWorkspaceSeesIt(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	store := postgres.NewMailboxRepository()
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: freshID(t), TenantID: tenantA, Preset: "gmail", Address: lettersOf(freshID(t)) + "@gmail.com",
		Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	sealed := crypto.Sealed{KeyID: "k1", Ciphertext: []byte{1, 2, 3}}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Insert(ctx, box, sealed, authorA)
	}); err != nil {
		t.Fatalf("storing: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Insert(ctx, box, sealed, authorA)
	}); !errors.Is(err, shared.ErrConflict) {
		t.Errorf("the same address twice answered %v", err)
	}
	failing := box.Checked(time.Now(), "mail.mailbox_refused")
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.RecordCheck(ctx, failing)
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, secret, err := store.Find(ctx, box.ID)
		if err != nil || found.Host != "imap.gmail.com" || found.Port != 993 || found.Failure != "mail.mailbox_refused" ||
			found.FailingSince == nil || secret.KeyID != "k1" || len(secret.Ciphertext) != 3 {
			t.Errorf("found %+v / %+v, %v", found, secret, err)
		}
		listed, err := store.List(ctx)
		if err != nil || len(listed) == 0 {
			t.Errorf("listed %d, %v", len(listed), err)
		}
		count, err := store.Count(ctx)
		if err != nil || count < 1 {
			t.Errorf("counted %d, %v", count, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := read(ctx, t, tenantB, func(ctx context.Context) error {
		if _, _, err := store.Find(ctx, box.ID); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("tenant B found A's mailbox: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		return store.Delete(ctx, box.ID)
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B removed A's mailbox: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Delete(ctx, box.ID)
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAnOAuthMailboxKeepsItsAppAndAWaitingConnectionIsTakenOnce(t *testing.T) {
	ctx := context.Background()
	seedContainerTenants(ctx, t)
	now := time.Now().UTC().Truncate(time.Microsecond)
	store := postgres.NewMailboxRepository()
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: freshID(t), TenantID: tenantA, Protocol: domain.ProtocolIMAP, Host: "outlook.office365.com",
		Address: lettersOf(freshID(t)) + "@contoso.com", Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	preset, _ := domain.OAuthPresetNamed("microsoft")
	box.Auth, box.OAuth = domain.AuthOAuth, &domain.OAuthClient{
		Provider: "microsoft", AuthorizeURL: preset.AuthorizeURL, TokenURL: preset.TokenURL, Scopes: preset.Scopes,
		ClientID: "inst-id", Installation: true,
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Insert(ctx, box, crypto.Sealed{KeyID: "k1", Ciphertext: []byte{1}}, authorA)
	}); err != nil {
		t.Fatalf("storing: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return store.Reseal(ctx, box.ID, crypto.Sealed{KeyID: "k2", Ciphertext: []byte{2, 2}})
	}); err != nil {
		t.Fatalf("resealing: %v", err)
	}
	if err := read(ctx, t, tenantA, func(ctx context.Context) error {
		found, sealed, err := store.Find(ctx, box.ID)
		if err != nil || found.Auth != domain.AuthOAuth || found.OAuth == nil || found.OAuth.ClientID != "inst-id" ||
			!found.OAuth.Installation || len(found.OAuth.Scopes) != len(preset.Scopes) || sealed.KeyID != "k2" {
			t.Errorf("found %+v %+v / %+v, %v", found, found.OAuth, sealed, err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	waiting := postgres.NewMailboxAuthorizationRepository()
	pending := domain.PendingAuthorization{
		ID: freshID(t), TenantID: tenantA, AccountID: authorA, Client: *box.OAuth, Host: "outlook.office365.com",
		Port: 993, LoginHint: "team@contoso.com", ExpiresAt: now.Add(domain.PendingTTL),
	}
	hash := []byte(lettersOf(freshID(t)))
	ownSecret := crypto.Sealed{KeyID: "k3", Ciphertext: []byte{3}}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		return waiting.Insert(ctx, pending, hash, crypto.Sealed{KeyID: "kv", Ciphertext: []byte{9}}, &ownSecret)
	}); err != nil {
		t.Fatalf("storing a waiting connection: %v", err)
	}
	if err := write(ctx, t, tenantB, func(ctx context.Context) error {
		_, _, _, err := waiting.Take(ctx, hash)
		return err
	}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("tenant B took A's waiting connection: %v", err)
	}
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		taken, verifier, secret, err := waiting.Take(ctx, hash)
		if err != nil || taken.AccountID != authorA || taken.LoginHint != "team@contoso.com" || taken.Client.ClientID != "inst-id" ||
			verifier.KeyID != "kv" || secret == nil || secret.KeyID != "k3" || !taken.ExpiresAt.Equal(pending.ExpiresAt) {
			t.Errorf("took %+v / %+v / %+v, %v", taken, verifier, secret, err)
		}
		_, _, _, err = waiting.Take(ctx, hash)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("a waiting connection was taken twice: %v", err)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	expired := pending
	expired.ID, expired.ExpiresAt = freshID(t), now.Add(-time.Minute)
	if err := write(ctx, t, tenantA, func(ctx context.Context) error {
		if err := waiting.Insert(ctx, expired, []byte(lettersOf(freshID(t))), crypto.Sealed{KeyID: "kv", Ciphertext: []byte{9}}, nil); err != nil {
			return err
		}
		gone, err := waiting.DeleteExpired(ctx, now)
		if gone < 1 {
			t.Errorf("forgot %d expired", gone)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	_ = write(ctx, t, tenantA, func(ctx context.Context) error { return store.Delete(ctx, box.ID) })
}
