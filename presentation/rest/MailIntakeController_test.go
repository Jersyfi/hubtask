// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	mailmodel "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The public mail route. What is decided out here is what is tested out here: the body
// reaches the intake as the bytes that arrived, an installation that does not serve the route says
// nothing about why, and the entry's identifier is the answer.

type mailDeliverer struct {
	entry     jumbledomain.Entry
	err       error
	presented string
	raw       []byte
	invoked   bool
}

func (d *mailDeliverer) Deliver(
	_ context.Context, presented string, raw []byte,
) (jumbledomain.Entry, error) {
	d.presented, d.raw, d.invoked = presented, raw, true
	return d.entry, d.err
}

func deliverMail(
	t *testing.T, door MailDeliverer, token, body string,
) *httptest.ResponseRecorder {
	t.Helper()

	controller := NewRestController()
	controller.MailIntake = door

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
		APIBasePath+"/jumble/mail/"+token, strings.NewReader(body))
	request.Header.Set("Content-Type", "message/rfc822")

	recorder := httptest.NewRecorder()
	controller.Routes().ServeHTTP(recorder, request)
	return recorder
}

// The message reaches the intake as the bytes that arrived - headers, boundaries and all. Anything
// this layer did to them would be a second parser nobody wrote down.
func TestTheMailRouteHandsOverTheMessageAsItArrived(t *testing.T) {
	door := &mailDeliverer{entry: jumbledomain.Entry{
		ID: shared.ID("0192f000-0000-7000-8000-0000000000e1"),
	}}
	raw := "From: a@example.org\r\nSubject: Order #42\r\n\r\nCall back\r\n"

	response := deliverMail(t, door, "tenant.secret", raw)

	if response.Code != http.StatusCreated {
		t.Fatalf("the route answered %d: %s", response.Code, response.Body)
	}
	if !door.invoked {
		t.Fatal("the intake was never asked")
	}
	if string(door.raw) != raw {
		t.Errorf("the intake was handed %q", door.raw)
	}
	if door.presented != "tenant.secret" {
		t.Errorf("the token reached the intake as %q", door.presented)
	}

	var answered struct {
		EntryID string `json:"entry_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the documented shape: %v", err)
	}
	if answered.EntryID != "0192f000-0000-7000-8000-0000000000e1" {
		t.Errorf("the answer names %q", answered.EntryID)
	}
}

// An installation that does not serve the route answers the pending not-found, exactly as the
// webhook door does: what the internet learns from it is nothing at all.
func TestAnUnwiredMailRouteSaysNothingAboutWhy(t *testing.T) {
	response := deliverMail(t, nil, "tenant.secret", "From: a@example.org\r\n\r\nhello\r\n")

	if response.Code != http.StatusNotFound {
		t.Errorf("an unwired route answered %d", response.Code)
	}
}

// A refusal from below reaches the caller as the problem document it is, with its own code: "raise
// the bound" and "look at the entry" are different answers and they have to look different.
func TestAMailRefusedBelowKeepsItsCode(t *testing.T) {
	door := &mailDeliverer{
		err: shared.ErrValidation.WithDetail("mail.attachment_too_large"),
	}

	response := deliverMail(t, door, "tenant.secret", "From: a@example.org\r\n\r\nhello\r\n")

	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("the route answered %d: %s", response.Code, response.Body)
	}
	if !strings.Contains(response.Body.String(), "mail.attachment_too_large") {
		t.Errorf("the problem is %s", response.Body)
	}
}

// The receiving domain's route (ADR-0092): a wrong key answers the not-found an unwired route
// does, the right one hands the bytes and the named recipient over and answers 202 with what the
// message became.
type addressedDoor struct {
	recipient string
	raw       []byte
}

func (d *addressedDoor) Authorized(header string) bool { return header == "Bearer k" }

func (d *addressedDoor) Deliver(
	_ context.Context, recipient string, raw []byte,
) (jumbleservice.MailDeliveryResult, error) {
	d.recipient, d.raw = recipient, raw
	return jumbleservice.MailDeliveryResult{
		Outcome: mailmodel.OutcomeEntry, ItemID: shared.ID("0192f000-0000-7000-8000-0000000000e2"),
	}, nil
}

func TestTheInboundRouteTakesOnlyTheKey(t *testing.T) {
	door := &addressedDoor{}
	controller := NewRestController()
	controller.AddressedMail = door
	send := func(key string) *httptest.ResponseRecorder {
		request := httptest.NewRequestWithContext(t.Context(), http.MethodPost,
			APIBasePath+"/mail/inbound?recipient=a.bbbbbbbbbbbbbbbb%40in.example.test",
			strings.NewReader("From: a@example.org\r\n\r\nhi\r\n"))
		request.Header.Set("Content-Type", "message/rfc822")
		request.Header.Set("Authorization", key)
		recorder := httptest.NewRecorder()
		controller.Routes().ServeHTTP(recorder, request)
		return recorder
	}

	if response := send("Bearer wrong"); response.Code != http.StatusNotFound || door.raw != nil {
		t.Fatalf("a wrong key answered %d and reached the door: %t", response.Code, door.raw != nil)
	}
	response := send("Bearer k")
	if response.Code != http.StatusAccepted {
		t.Fatalf("the key answered %d: %s", response.Code, response.Body)
	}
	if door.recipient != "a.bbbbbbbbbbbbbbbb@in.example.test" || !strings.Contains(string(door.raw), "hi") {
		t.Errorf("handed %q and %q", door.recipient, door.raw)
	}
	var answered struct {
		Outcome string `json:"outcome"`
		ItemID  string `json:"item_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil ||
		answered.Outcome != "ENTRY" || answered.ItemID != "0192f000-0000-7000-8000-0000000000e2" {
		t.Errorf("answered %s (%v)", response.Body, err)
	}
}
