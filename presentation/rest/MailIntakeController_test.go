// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The public mail route (G-11). What is decided out here is what is tested out here: the body
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
