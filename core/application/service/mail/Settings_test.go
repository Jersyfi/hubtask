// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"errors"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.ID("01936f2a-7c1e-7000-8000-000000000001")
	admin   = shared.ID("01936f2a-7c1e-7000-8000-000000000002")
	hub     = shared.ID("01936f2a-7c1e-7000-8000-000000000003")
	einkauf = shared.ID("01936f2a-7c1e-7000-8000-000000000004")
	entry   = shared.ID("01936f2a-7c1e-7000-8000-000000000005")
	fixed   = time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	person  = appshared.ActorContext{Kind: appshared.ActorUser, TenantID: tenant, AccountID: admin, AccountName: "Ada"}
)

type authorizer struct {
	asked  []access.Request
	refuse error
}

func (a *authorizer) Authorize(_ context.Context, _ appshared.ActorContext, request access.Request) error {
	a.asked = append(a.asked, request)
	return a.refuse
}

type unitOfWork struct{}

func (unitOfWork) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (unitOfWork) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, entry audit.Entry) error {
	t.entries = append(t.entries, entry)
	return nil
}

// store is the intake's store, in memory.
type store struct {
	settings domain.Settings
	receipts []domain.Receipt
	handles  map[shared.ID]string
	keys     map[shared.ID]domain.Key
	saveErr  error
}

func (s *store) Find(context.Context) (domain.Settings, error) { return s.settings, nil }
func (s *store) Save(_ context.Context, settings domain.Settings, _ shared.ID, _ time.Time) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.settings = settings
	return nil
}
func (s *store) WorkspaceNamed(context.Context, string) (shared.ID, error)  { return tenant, nil }
func (s *store) Seen(context.Context, string) (bool, error)                 { return false, nil }
func (s *store) Record(context.Context, domain.Receipt) (bool, error)       { return true, nil }
func (s *store) CountFrom(context.Context, string, time.Time) (int, error)  { return 0, nil }
func (s *store) Recent(context.Context, int) ([]domain.Receipt, error)      { return s.receipts, nil }
func (s *store) DeleteExpired(context.Context, time.Time, int) (int, error) { return 0, nil }
func (s *store) CountExpired(context.Context, time.Time, int) (int, error)  { return 0, nil }
func (s *store) CollectionByHandle(context.Context, string) (shared.ID, error) {
	return einkauf, nil
}
func (s *store) ItemByKey(context.Context, domain.Key) (shared.ID, error) { return entry, nil }
func (s *store) KeyOf(_ context.Context, id shared.ID) (domain.Key, error) {
	if key, ok := s.keys[id]; ok {
		return key, nil
	}
	return domain.Key{}, shared.ErrNotFound
}
func (s *store) SetHandle(_ context.Context, id shared.ID, handle string) error {
	s.handles[id] = handle
	return nil
}

// containers answers the collection and the hub.
type containers struct {
	workrepo.Containers
}

func (containers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	switch id {
	case einkauf:
		return work.Container{ID: einkauf, TenantID: tenant, Type: work.ContainerCollection, ParentID: hub, Name: "Einkauf", Handle: "einkauf"}, nil
	case hub:
		return work.Container{ID: hub, TenantID: tenant, Type: work.ContainerHub, Name: "Zuhause"}, nil
	}
	return work.Container{}, shared.ErrNotFound
}

func writer(auth *authorizer) (Writer, *store, *trail) {
	saved := &store{settings: domain.DefaultSettings(), handles: map[shared.ID]string{}, keys: map[shared.ID]domain.Key{}}
	sink := &trail{}
	return Writer{
		Settings: saved, Receipts: saved, Places: saved, Containers: containers{}, Authorizer: auth,
		Audit: sink, UnitOfWork: unitOfWork{}, Clock: clock.Fixed(fixed), Domain: "in.hubtask.app",
	}, saved, sink
}

func TestTheSettingsAreAnAdministratorsAndSayTheAddress(t *testing.T) {
	auth := &authorizer{}
	w, saved, sink := writer(auth)
	out, err := SaveMailSettings{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{
		"enabled": true, "name": "Familie-Meier", "default_collection_id": einkauf.String(),
		"senders":       map[string]any{"level": "MEMBERS_AND_ALLOWED", "allowed": []any{"lieferant.de"}, "blocked": []any{"@spam.test"}},
		"sender_routes": []any{map[string]any{"match": "@lieferant.de", "collection_id": einkauf.String()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !saved.settings.Enabled || saved.settings.Name != "familie-meier" || saved.settings.OpenedBy != admin ||
		saved.settings.Senders.Allowed[0] != "@lieferant.de" || len(saved.settings.SenderRoutes) != 1 {
		t.Errorf("saved %+v", saved.settings)
	}
	if out["address"] != "familie-meier@in.hubtask.app" || out["domain"] != "in.hubtask.app" ||
		out["default_collection_id"] != einkauf.String() {
		t.Errorf("answered %+v", out)
	}
	if auth.asked[0].Permission != service.PermissionAutomation || len(sink.entries) != 1 {
		t.Errorf("asked %+v, recorded %d", auth.asked, len(sink.entries))
	}
	shown, err := ShowMailSettings{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{})
	if err != nil || shown["name"] != "familie-meier" {
		t.Errorf("shown %+v, %v", shown, err)
	}
}

func TestBadSettingsAndAForeignPlaceAreRefused(t *testing.T) {
	w, _, _ := writer(&authorizer{})
	for name, in := range map[string]usecase.Input{
		"a bad level":     {"senders": map[string]any{"level": "SOME"}},
		"a bad name":      {"name": "-x-"},
		"an unknown home": {"default_collection_id": entry.String()},
		"a route nowhere": {"sender_routes": []any{map[string]any{"match": "@a.de", "collection_id": "nope"}}},
	} {
		if _, err := (SaveMailSettings{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person, in); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
	refused := &authorizer{refuse: shared.ErrForbidden}
	w, _, _ = writer(refused)
	for _, descriptor := range []usecase.Descriptor{
		ShowMailSettings{Writer: w}.Descriptor(), SaveMailSettings{Writer: w}.Descriptor(), ListMailReceipts{Writer: w}.Descriptor(),
	} {
		if _, err := descriptor.Handler.Invoke(context.Background(), person, usecase.Input{}); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s without the right answered %v", descriptor.Name, err)
		}
	}
}

func TestTheLogIsReadNewestFirstWithItsReasons(t *testing.T) {
	w, saved, _ := writer(&authorizer{})
	saved.receipts = []domain.Receipt{
		{ID: "r2", ReceivedAt: fixed, Sender: "x@y.de", Subject: "Hi", Outcome: domain.OutcomeJumble, Reason: domain.ReasonSenderNotAllowed, EntryID: "e1", Door: "inbound"},
		{ID: "r1", ReceivedAt: fixed.Add(-time.Hour), Sender: "a@b.de", Subject: "#einkauf", Outcome: domain.OutcomeEntry, ItemID: entry, Door: "familie@gmail.com"},
	}
	out, err := ListMailReceipts{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	rows := out["receipts"].([]any)
	first, second := rows[0].(map[string]any), rows[1].(map[string]any)
	if first["reason"] != "SENDER_NOT_ALLOWED" || first["entry_id"] != "e1" || second["item_id"] != entry.String() {
		t.Errorf("rows %+v", rows)
	}
	if _, has := second["reason"]; has {
		t.Error("a filed message carries a reason")
	}
}

func TestAHandleIsChangedByWhoShapesTheCollection(t *testing.T) {
	auth := &authorizer{}
	w, saved, sink := writer(auth)
	out, err := SetCollectionHandle{Writer: w}.Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"collection_id": einkauf.String(), "handle": " Markt "})
	if err != nil || out["handle"] != "markt" || saved.handles[einkauf] != "markt" {
		t.Fatalf("answered %+v, %v, stored %v", out, err, saved.handles)
	}
	if auth.asked[0].Permission != service.PermissionStructure || sink.entries[0].Changes["handle"] == nil {
		t.Errorf("asked %+v, recorded %+v", auth.asked, sink.entries)
	}
	for name, in := range map[string]usecase.Input{
		"a hyphen":      {"collection_id": einkauf.String(), "handle": "web-1"},
		"no collection": {"handle": "markt"},
	} {
		if _, err := (SetCollectionHandle{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person, in); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s answered %v", name, err)
		}
	}
	if _, err := (SetCollectionHandle{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"collection_id": hub.String(), "handle": "zuhause"}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a hub answered %v", err)
	}
	// The same handle again changes nothing and records nothing.
	before := len(sink.entries)
	if _, err := (SetCollectionHandle{Writer: w}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"collection_id": einkauf.String(), "handle": "einkauf"}); err != nil || len(sink.entries) != before {
		t.Errorf("the same handle: %v, %d entries", err, len(sink.entries)-before)
	}
}

func TestARepliesAddressCarriesTheTicketKey(t *testing.T) {
	w, saved, _ := writer(&authorizer{})
	replies := Replies{Writer: w}
	if address, _ := replies.ReplyAddress(context.Background(), tenant, entry); address != "" {
		t.Errorf("an intake that is off answered %q", address)
	}
	saved.settings = domain.Settings{Enabled: true, Name: "familie"}
	saved.keys[entry] = domain.Key{Handle: "einkauf", Number: 42}
	if address, err := replies.ReplyAddress(context.Background(), tenant, entry); err != nil || address != "familie+einkauf-42@in.hubtask.app" {
		t.Errorf("answered %q, %v", address, err)
	}
	if address, err := replies.ReplyAddress(context.Background(), tenant, einkauf); err != nil || address != "" {
		t.Errorf("an entry without a key answered %q, %v", address, err)
	}
	w.Domain = ""
	if address, _ := (Replies{Writer: w}).ReplyAddress(context.Background(), tenant, entry); address != "" {
		t.Errorf("without a receiving domain answered %q", address)
	}
}
