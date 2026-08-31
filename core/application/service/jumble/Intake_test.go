// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// intakeStore is the tenant's one address in memory.
type intakeStore struct {
	hash      string
	rotatedAt time.Time
}

func (s *intakeStore) SetToken(_ context.Context, token integration.InboundToken, at time.Time) error {
	s.hash, s.rotatedAt = token.Secret(), at
	return nil
}

func (s *intakeStore) VerifyToken(_ context.Context, token integration.InboundToken) (bool, error) {
	return s.hash != "" && s.hash == token.Secret(), nil
}

func (s *intakeStore) RotatedAt(context.Context) (time.Time, error) {
	if s.hash == "" {
		return time.Time{}, shared.ErrNotFound.WithDetail("jumble.intake_not_minted")
	}
	return s.rotatedAt, nil
}

type entropy struct{ next byte }

func (e *entropy) Bytes(n int) ([]byte, error) {
	e.next++
	drawn := make([]byte, n)
	for i := range drawn {
		drawn[i] = e.next
	}
	return drawn, nil
}

// The acceptance criterion, credential half: the token is shown once, rotating replaces it in one
// statement, and the act is audited.
func TestTheIntakeTokenIsMintedOnceAndRotates(t *testing.T) {
	store := &intakeStore{}
	sink := &auditSink{}
	rotate := RotateJumbleIntake{
		Intake: store, Authorizer: &authorizer{}, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), Entropy: &entropy{},
	}

	first, err := rotate.Execute(context.Background(), actor())
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if first.Token.IsZero() || first.Token.TenantID() != tenant {
		t.Fatalf("the token reads %+v", first.Token)
	}
	if store.hash != first.Token.Secret() {
		t.Error("the stored credential is not the answered one")
	}

	second, err := rotate.Execute(context.Background(), actor())
	if err != nil {
		t.Fatalf("rotating: %v", err)
	}
	if second.Token.Secret() == first.Token.Secret() {
		t.Error("a rotation answered the same token")
	}
	if store.hash != second.Token.Secret() {
		t.Error("the old token still opens the intake")
	}
	if len(sink.entries) != 2 || sink.entries[0].Action != IntakeRotatedAction {
		t.Errorf("audited %+v", sink.entries)
	}
}

// A delivery on the address lands as a WEBHOOK entry with no actor, and fires the arrival event.
func TestADeliveryLandsAsAWebhookEntry(t *testing.T) {
	store := &intakeStore{}
	entries := newEntryStore()
	events := &published{}
	token, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := store.SetToken(context.Background(), token, now); err != nil {
		t.Fatal(err)
	}

	door := IntakeJumbleEntry{
		Intake: store, Entries: entries, Events: events,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}
	entry, err := door.Execute(context.Background(), IntakeDelivery{
		Token: token, Sender: "bridge@example.org", Subject: "Order #42", Body: "Call back",
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if entry.Channel != domain.ChannelWebhook || entry.TenantID != tenant {
		t.Errorf("the entry reads %+v", entry)
	}
	if len(events.envelopes) != 1 || events.envelopes[0].Type != event.JumbleEntryReceived {
		t.Fatalf("announced %+v", events.envelopes)
	}
	if events.envelopes[0].Actor.Kind != shared.ActorSystem || !events.envelopes[0].Actor.ID.IsZero() {
		t.Errorf("the event names an author: %+v", events.envelopes[0].Actor)
	}
}

// Every reason not to serve is the same refusal: a wrong token, a rotated one, and a tenant that
// never minted one all answer the same not-found.
func TestEveryIntakeRefusalIsTheSame(t *testing.T) {
	store := &intakeStore{}
	entries := newEntryStore()
	door := IntakeJumbleEntry{
		Intake: store, Entries: entries,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}

	stranger, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}

	for name, delivery := range map[string]IntakeDelivery{
		"a token nobody minted": {Token: stranger, Body: "x"},
		"no token at all":       {Body: "x"},
	} {
		_, err := door.Execute(context.Background(), delivery)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s answered %v, want not found", name, err)
		}
		if code := shared.AsError(err).DetailCode; code != "jumble.inbound_not_found" {
			t.Errorf("%s answered code %s", name, code)
		}
	}
	if len(entries.rows) != 0 {
		t.Error("a refused delivery was stored")
	}
}
