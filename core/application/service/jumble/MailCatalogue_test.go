// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The address use cases as the catalogue serves them, the place reader against the work stores,
// and the small rules a delivery's title follows.

func TestTheAddressUseCasesAnswerThroughTheCatalogue(t *testing.T) {
	writer := addressWriter(newAddressBook(), &authorizer{}, inboundDomain)
	in := usecase.Input{"target_kind": "COLLECTION", "target_id": mailCollection.String()}

	shown, err := ShowMailAddress{Writer: writer}.Descriptor().Handler.Invoke(context.Background(), actor(), in)
	if err != nil || shown["available"] != true || shown["target_id"] != mailCollection.String() {
		t.Fatalf("shown %+v, %v", shown, err)
	}
	opened, err := SetMailSenderPolicy{Writer: writer}.Descriptor().Handler.Invoke(context.Background(), actor(),
		usecase.Input{"target_kind": "COLLECTION", "target_id": mailCollection.String(), "sender_policy": "ANYONE"})
	if err != nil || opened["sender_policy"] != "ANYONE" || opened["opened_by"] != account.String() {
		t.Fatalf("opened %+v, %v", opened, err)
	}
	rotated, err := RotateMailAddress{Writer: writer}.Descriptor().Handler.Invoke(context.Background(), actor(), in)
	if err != nil || rotated["address"] == shown["address"] {
		t.Fatalf("rotated %+v, %v", rotated, err)
	}

	for _, bad := range []usecase.Input{
		{"target_kind": "SOMEWHERE"},
		{"target_kind": "ITEM"},
		{"target_kind": "ITEM", "target_id": "not-an-id"},
	} {
		if _, err := (ShowMailAddress{Writer: writer}).Descriptor().Handler.Invoke(context.Background(), actor(), bad); !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%v answered %v", bad, err)
		}
	}
	none := addressWriter(newAddressBook(), &authorizer{}, "")
	if out, _ := (ShowMailAddress{Writer: none}).Descriptor().Handler.Invoke(context.Background(), actor(),
		usecase.Input{"target_kind": "JUMBLE"}); out["available"] != false || len(out) != 1 {
		t.Errorf("without a domain answered %+v", out)
	}
}

func TestCommentingAnEntryAnswersThroughTheCatalogue(t *testing.T) {
	h := newHarness()
	entry, _ := domain.NewEntry(domain.NewEntryInput{
		ID: entryID, TenantID: tenant, Channel: domain.ChannelEmail, RawSubject: "Only a subject", Now: now,
	})
	_ = h.store.Insert(context.Background(), entry)
	calls := &catalogue{}
	out, err := CommentJumbleEntry{Writer: h.writer, Catalogue: calls}.Descriptor().Handler.Invoke(
		context.Background(), actor(), usecase.Input{"entry_id": entryID.String(), "item_id": mailItem.String()})
	if err != nil || out["status"] != "PROCESSED" || calls.calls[0].in["body"] != "Only a subject" {
		t.Fatalf("answered %+v, %v, calls %+v", out, err, calls.calls)
	}
	// Meant for nothing and naming nothing: there is no entry to comment on.
	other := newHarness()
	plain, _ := domain.NewEntry(domain.NewEntryInput{ID: entryID, TenantID: tenant, Channel: domain.ChannelAPI, RawSubject: "x", Now: now})
	_ = other.store.Insert(context.Background(), plain)
	if _, err := (CommentJumbleEntry{Writer: other.writer, Catalogue: &catalogue{}}).Execute(context.Background(), actor(), entryID, ""); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("no destination answered %v", err)
	}
}

type workItems struct {
	workrepo.Items
	rows map[shared.ID]work.WorkItem
}

func (w workItems) Find(_ context.Context, id shared.ID) (work.WorkItem, error) {
	if row, ok := w.rows[id]; ok {
		return row, nil
	}
	return work.WorkItem{}, shared.ErrNotFound
}

type workContainers struct {
	workrepo.Containers
	rows map[shared.ID]work.Container
}

func (w workContainers) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if row, ok := w.rows[id]; ok {
		return row, nil
	}
	return work.Container{}, shared.ErrNotFound
}

func TestAPlaceIsReadWithWhetherItTakesAnything(t *testing.T) {
	hub := shared.ID("01936f2a-7c1e-7000-8000-000000009010")
	archived := time.Unix(0, 0)
	places := MailPlaces{
		Containers: workContainers{rows: map[shared.ID]work.Container{
			hub:            {ID: hub, Type: work.ContainerHub, Name: "Agency"},
			mailCollection: {ID: mailCollection, Type: work.ContainerCollection, ParentID: hub, Name: "Office"},
		}},
		Items: workItems{rows: map[shared.ID]work.WorkItem{
			mailItem: {ID: mailItem, CollectionID: mailCollection, Title: "Homepage", ArchivedAt: &archived},
		}},
	}
	ctx := context.Background()

	jumble, err := places.Find(ctx, domain.MailTarget{Kind: domain.TargetJumble})
	if err != nil || len(jumble.Path) != 1 {
		t.Errorf("the jumble: %+v, %v", jumble, err)
	}
	collection, err := places.Find(ctx, domain.MailTarget{Kind: domain.TargetCollection, ID: mailCollection})
	if err != nil || collection.Name != "Office" || collection.Closed {
		t.Errorf("the collection: %+v, %v", collection, err)
	}
	item, err := places.Find(ctx, domain.MailTarget{Kind: domain.TargetItem, ID: mailItem})
	if err != nil || item.Name != "Homepage" || !item.Closed || item.CollectionID != mailCollection {
		t.Errorf("the archived entry: %+v, %v", item, err)
	}
	for _, target := range []domain.MailTarget{
		{Kind: domain.TargetCollection, ID: hub}, // a hub takes no mail
		{Kind: domain.TargetCollection, ID: "01936f2a-7c1e-7000-8000-000000009099"},
		{Kind: domain.TargetItem, ID: "01936f2a-7c1e-7000-8000-000000009099"},
		{Kind: "ELSEWHERE"},
	} {
		if _, err := places.Find(ctx, target); !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%+v answered %v", target, err)
		}
	}
	if err := placeGone(shared.ErrUnavailable); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("an outage became %v", err)
	}
}

func TestAMailIsAlwaysGivenATitle(t *testing.T) {
	cases := map[string]AddressedMail{
		"Subject":         {Subject: "  Subject "},
		"First line":      {Body: "\n  First line\nsecond"},
		"From: a@b.test":  {Sender: "a@b.test"},
		"From: A <a@b.c>": {Sender: "a@b.c", SenderName: "A"},
	}
	for want, mail := range cases {
		if got := mailTitle(mail); got != want {
			t.Errorf("mailTitle(%+v) = %q, want %q", mail, got, want)
		}
	}
}
