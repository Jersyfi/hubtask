// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ADR-0078 §6, UC-ID-04 check 9, UC-ID-05 check 6: a person is told by mail when a way into their
// account changes.

var wayInProvider = shared.ID("01936f2a-7c1e-7000-8000-0000000000a1")

func TestAChangeOfAWayInQueuesOneNoticeCarryingIdentifiersOnly(t *testing.T) {
	jobs := &jobQueue{}
	recorder := RecordWayInChanged{Jobs: jobs}
	at := time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)

	for _, change := range []identity.WayInChange{
		{Event: identity.WayInPasswordChanged, At: at},
		{Event: identity.WayInPasswordChanged, At: at.Add(time.Minute)},
		{Event: identity.WayInProviderConnected, At: at, ProviderID: wayInProvider, ProviderName: "Entra"},
	} {
		if err := recorder.WayInChanged(t.Context(), tenant, anna, change); err != nil {
			t.Fatalf("queueing: %v", err)
		}
	}

	if len(jobs.requests) != 3 {
		t.Fatalf("%d notices queued", len(jobs.requests))
	}
	keys := map[string]bool{}
	for _, request := range jobs.requests {
		if request.Kind != queue.KindWayInChangedEmail || request.TenantID != tenant {
			t.Errorf("queued %+v", request)
		}
		keys[request.DedupeKey] = true
		for name := range request.Payload {
			switch name {
			case "account_id", "event", "at", "provider_id", "provider_name":
			default:
				t.Errorf("a notice carries %q", name)
			}
		}
	}
	// Two changes a minute apart are two notices.
	if len(keys) != 3 {
		t.Errorf("two notices share a dedupe key: %v", keys)
	}
	connected := jobs.requests[2].Payload
	if connected["provider_id"] != wayInProvider.String() || connected["provider_name"] != "Entra" ||
		connected["account_id"] != anna.String() {
		t.Errorf("the connection's notice carries %v", connected)
	}
}

// providersHeld answers the providers that still exist.
type providersHeld map[shared.ID]bool

func (p providersHeld) Find(_ context.Context, id shared.ID) (identity.IdentityProvider, error) {
	if p[id] {
		return identity.IdentityProvider{ID: id}, nil
	}
	return identity.IdentityProvider{}, shared.ErrNotFound
}

func wayInSender(mailer *mailbox, providers providersHeld) SendWayInChanged {
	return SendWayInChanged{
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "de"),
			person(bert, "Bert", "", "en"),
			person(carla, "Carla", "carla@example.org", ""),
		),
		Workspaces: workspaceNamed{name: "Acme", locale: "en"},
		Providers:  providers,
		Mail:       mailer, Renderer: catalogue{}, UnitOfWork: &unitOfWork{},
		FallbackLocale: "en", BaseURL: "https://acme.example/",
	}
}

func TestEveryChangeIsToldInItsOwnWordsWithItsWayBack(t *testing.T) {
	at := time.Date(2026, 10, 15, 9, 30, 0, 0, time.UTC)
	for _, row := range []struct {
		event         identity.WayInEvent
		subject, body string
		link          string
	}{
		{identity.WayInPasswordChanged, subjectPasswordChanged, bodyPasswordChanged, "link=https://acme.example/"},
		{identity.WayInPasswordReset, subjectWayInReset, bodyWayInReset, "link=https://acme.example/"},
		{identity.WayInProviderConnected, subjectProviderConnected, bodyProviderConnected, "link=https://acme.example/profile/security"},
		{identity.WayInProviderDisconnected, subjectProviderDisconnected, bodyProviderDisconnected, "link=https://acme.example/"},
	} {
		mailer := &mailbox{}
		err := wayInSender(mailer, providersHeld{}).Execute(t.Context(), tenant, WayInNotice{
			AccountID: carla,
			Change:    identity.WayInChange{Event: row.event, At: at, ProviderID: wayInProvider, ProviderName: "Entra"},
		})
		if err != nil {
			t.Fatalf("%s: sending: %v", row.event, err)
		}
		if len(mailer.sent) != 1 || mailer.sent[0].To != "carla@example.org" {
			t.Fatalf("%s: sent %+v", row.event, mailer.sent)
		}
		sent := mailer.sent[0]
		if !strings.HasPrefix(sent.Subject, row.subject) {
			t.Errorf("%s: the subject was rendered as %q", row.event, sent.Subject)
		}
		for _, want := range []string{row.body, "workspace=Acme", "at=2026-10-15 09:30 UTC", row.link} {
			if !strings.Contains(sent.Body, want) {
				t.Errorf("%s: the body %q lacks %q", row.event, sent.Body, want)
			}
		}
	}
}

// The notice names no secret: its parameters are the workspace, the moment, the provider's name
// and a link to the installation - nothing that opens anything.
func TestTheNoticeNamesNoSecret(t *testing.T) {
	mailer := &mailbox{}
	if err := wayInSender(mailer, providersHeld{}).Execute(t.Context(), tenant, WayInNotice{
		AccountID: anna, Change: identity.WayInChange{Event: identity.WayInPasswordReset, At: now},
	}); err != nil {
		t.Fatalf("sending: %v", err)
	}
	body := mailer.sent[0].Body
	for _, named := range regexp.MustCompile(`(\w+)=`).FindAllStringSubmatch(body, -1) {
		switch name := named[1]; name {
		case "workspace", "at", "provider", "link":
		default:
			t.Errorf("the notice carries the parameter %q", name)
		}
	}
	if strings.Contains(body, "token") || strings.Contains(body, "#") {
		t.Errorf("the notice carries a link with a credential: %q", body)
	}
	// Anna chose German.
	if !strings.HasPrefix(mailer.sent[0].Subject, "[de] ") {
		t.Errorf("rendered as %q for a German reader", mailer.sent[0].Subject)
	}
}

func TestANoticeWithNobodyToTellIsFinishedBusiness(t *testing.T) {
	mailer := &mailbox{}
	sender := wayInSender(mailer, providersHeld{wayInProvider: true})
	for name, notice := range map[string]WayInNotice{
		"no address": {AccountID: bert, Change: identity.WayInChange{Event: identity.WayInPasswordChanged, At: now}},
		"account gone": {AccountID: shared.ID("01936f2a-7c1e-7000-8000-0000000000ff"),
			Change: identity.WayInChange{Event: identity.WayInPasswordChanged, At: now}},
		// The removal that queued it did not happen after all.
		"provider still there": {AccountID: anna, Change: identity.WayInChange{
			Event: identity.WayInProviderDisconnected, At: now, ProviderID: wayInProvider, ProviderName: "Entra",
		}},
	} {
		if err := sender.Execute(t.Context(), tenant, notice); err != nil {
			t.Errorf("%s: the job failed: %v", name, err)
		}
	}
	if len(mailer.sent) != 0 {
		t.Errorf("sent %+v", mailer.sent)
	}
}
