// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
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
// statement, and the act is audited at warning severity without the address
// (UC-JUM-02 checks 3 and 6).
func TestTheIntakeTokenIsMintedOnceAndRotates(t *testing.T) {
	store := &intakeStore{}
	sink := &auditSink{}
	auth := &authorizer{}
	rotate := RotateJumbleIntake{
		Intake: store, Authorizer: auth, Audit: sink,
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
		t.Fatalf("audited %+v", sink.entries)
	}
	for i, entry := range sink.entries {
		if entry.Severity != audit.SeverityWarning {
			t.Errorf("entry %d at severity %s, want warning", i, entry.Severity)
		}
		trail := fmt.Sprintf("%+v", entry)
		for _, token := range []integration.InboundToken{first.Token, second.Token} {
			if strings.Contains(trail, token.Secret()) {
				t.Errorf("entry %d carries a part of the address: %s", i, trail)
			}
		}
	}
	if len(auth.requests) == 0 || auth.requests[0].Permission != service.PermissionAutomation {
		t.Errorf("asked %+v, want the automation permission", auth.requests)
	}
}

// Making an address is the automation permission's; anybody else is refused and nothing is
// minted (UC-JUM-02 check 1).
func TestOnlyTheAutomationPermissionMakesAnAddress(t *testing.T) {
	store := &intakeStore{}
	sink := &auditSink{}
	rotate := RotateJumbleIntake{
		Intake: store, Authorizer: &authorizer{refuse: shared.ErrForbidden}, Audit: sink,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), Entropy: &entropy{},
	}
	if _, err := rotate.Execute(context.Background(), actor()); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if store.hash != "" || len(sink.entries) != 0 {
		t.Errorf("a refused rotation left a token %q or a trail %+v", store.hash, sink.entries)
	}
}

// A delivery on the address lands as an undecided WEBHOOK entry with no author, its sender kept as
// provenance only, and fires the arrival event a captured entry fires - the one a rule on new
// jumble entries is triggered by (UC-JUM-04 checks 1, 2, 3 and 6).
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

	if entry.Channel != domain.ChannelWebhook || entry.TenantID != tenant || entry.Status != domain.StatusNew {
		t.Errorf("the entry reads %+v", entry)
	}
	if !entry.CreatedBy.IsZero() || entry.Sender != "bridge@example.org" {
		t.Errorf("the entry names %q as its author and %q as its sender", entry.CreatedBy, entry.Sender)
	}
	if len(events.envelopes) != 1 || events.envelopes[0].Type != event.JumbleEntryReceived {
		t.Fatalf("announced %+v", events.envelopes)
	}
	if events.envelopes[0].Actor.Kind != shared.ActorSystem || !events.envelopes[0].Actor.ID.IsZero() {
		t.Errorf("the event names an author: %+v", events.envelopes[0].Actor)
	}
}

// Every reason not to serve is the same refusal: a wrong token, a rotated one, and a tenant that
// never minted one all answer the same not-found and store nothing (UC-JUM-02 check 4, UC-JUM-04
// check 4; the malformed one is the door's, in presentation/intake).
func TestEveryIntakeRefusalIsTheSame(t *testing.T) {
	store := &intakeStore{}
	entries := newEntryStore()
	replaced, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	rotate := RotateJumbleIntake{
		Intake: store, Authorizer: &authorizer{}, UnitOfWork: unitOfWork{},
		Clock: clock.Fixed(now), Entropy: &entropy{},
	}
	if err := store.SetToken(context.Background(), replaced, now); err != nil {
		t.Fatal(err)
	}
	if _, err := rotate.Execute(context.Background(), actor()); err != nil {
		t.Fatalf("replacing: %v", err)
	}
	door := IntakeJumbleEntry{
		Intake: store, Entries: entries,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}

	stranger, err := integration.NewInboundToken(tenant, bytes.Repeat([]byte{9}, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}

	for name, delivery := range map[string]IntakeDelivery{
		"a token nobody minted": {Token: stranger, Body: "x"},
		"a replaced token":      {Token: replaced, Body: "x"},
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

// A delivery is held to the bounds a quick capture is: an empty one and an overlong subject are
// refused with their codes and store nothing (UC-JUM-04 check 5).
func TestADeliveryKeepsTheCapturesBounds(t *testing.T) {
	store := &intakeStore{}
	entries := newEntryStore()
	token, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetToken(context.Background(), token, now); err != nil {
		t.Fatal(err)
	}
	door := IntakeJumbleEntry{
		Intake: store, Entries: entries, Events: &published{},
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}

	for code, delivery := range map[string]IntakeDelivery{
		"jumble.entry_empty":      {Token: token},
		"jumble.subject_too_long": {Token: token, Subject: strings.Repeat("s", domain.MaxSubjectLength+1), Body: "x"},
		"jumble.body_too_large":   {Token: token, Body: strings.Repeat("b", domain.MaxBodyBytes+1)},
	} {
		_, err := door.Execute(context.Background(), delivery)
		if !errors.Is(err, shared.ErrValidation) {
			t.Errorf("%s: error %v, want a validation refusal", code, err)
			continue
		}
		found := false
		for _, field := range shared.AsError(err).Fields {
			found = found || field.Code == code
		}
		if !found {
			t.Errorf("refused with %+v, want %s", shared.AsError(err).Fields, code)
		}
	}
	if len(entries.rows) != 0 {
		t.Error("a refused delivery was stored")
	}
}
