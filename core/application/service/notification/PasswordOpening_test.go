// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// ADR-0078 §3: a workspace's administrators are told when an operator opens its password,
// with the end, and when the opening ends, with how.

func TestEveryAdministratorIsQueuedANoticeOfTheOpening(t *testing.T) {
	jobs := &jobQueue{}
	memberships := &administratorStore{byPath: map[string][]shared.ID{"tenant": {anna, carla}}}
	recorder := RecordPasswordOpening{Memberships: memberships, Jobs: jobs}
	until := now.Add(24 * time.Hour)

	if err := recorder.PasswordOpened(t.Context(), tenant, until); err != nil {
		t.Fatalf("queueing the opening: %v", err)
	}
	if err := recorder.PasswordClosed(t.Context(), tenant, "EXPIRED"); err != nil {
		t.Fatalf("queueing the close: %v", err)
	}

	// The workspace as a whole: the tenant level, and nothing a caller could widen.
	if len(memberships.asked) != 2 || len(memberships.asked[0]) != 1 ||
		memberships.asked[0][0] != identity.TenantScope() {
		t.Errorf("the administrators were resolved along %v", memberships.asked)
	}
	if len(jobs.requests) != 4 {
		t.Fatalf("%d notices queued, want one per administrator per act", len(jobs.requests))
	}
	keys := map[string]bool{}
	for _, request := range jobs.requests {
		if request.Kind != queue.KindPasswordOpeningEmail || request.TenantID != tenant {
			t.Errorf("queued %+v", request)
		}
		keys[request.DedupeKey] = true
	}
	if len(keys) != 4 {
		t.Errorf("two notices share a dedupe key: %v", keys)
	}
	opened := jobs.requests[0].Payload
	if opened["account_id"] != anna.String() || opened["event"] != OpeningEventOpened ||
		opened["until"] != until.Format(time.RFC3339) {
		t.Errorf("the opening's notice carries %v", opened)
	}
	closed := jobs.requests[3].Payload
	if closed["account_id"] != carla.String() || closed["event"] != OpeningEventClosed || closed["ended"] != "EXPIRED" {
		t.Errorf("the close's notice carries %v", closed)
	}
	// Identifiers, an event and a moment - nothing a person wrote.
	for _, request := range jobs.requests {
		for name := range request.Payload {
			if name != "account_id" && name != "event" && name != "until" && name != "ended" {
				t.Errorf("a notice carries %q", name)
			}
		}
	}
}

func openingSender(mailer *mailbox) SendPasswordOpening {
	return SendPasswordOpening{
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "de"),
			person(bert, "Bert", "", "en"),
			person(carla, "Carla", "carla@example.org", ""),
		),
		Workspaces: workspaceNamed{name: "Acme", locale: "en"},
		Mail:       mailer, Renderer: catalogue{}, UnitOfWork: &unitOfWork{},
		FallbackLocale: "en", BaseURL: "https://acme.example/",
	}
}

// workspaceNamed answers the workspace's name and default language.
type workspaceNamed struct{ name, locale string }

func (w workspaceNamed) Find(context.Context) (identity.Workspace, error) {
	workspace := identity.Workspace{}
	workspace.DisplayName, workspace.DefaultLocale = w.name, w.locale
	return workspace, nil
}

func TestTheNoticeSaysUntilWhenInTheRecipientsLanguage(t *testing.T) {
	mailer := &mailbox{}
	until := time.Date(2026, 10, 7, 14, 30, 0, 0, time.UTC)
	err := openingSender(mailer).Execute(t.Context(), tenant, OpeningNotice{
		AccountID: anna, Event: OpeningEventOpened, Until: until,
	})
	if err != nil {
		t.Fatalf("sending: %v", err)
	}
	if len(mailer.sent) != 1 || mailer.sent[0].To != "anna@example.org" {
		t.Fatalf("sent %+v", mailer.sent)
	}
	sent := mailer.sent[0]
	if !strings.HasPrefix(sent.Subject, "[de] "+subjectPasswordOpened) {
		t.Errorf("the subject was rendered as %q", sent.Subject)
	}
	for _, want := range []string{bodyPasswordOpened, "until=2026-10-07 14:30 UTC", "workspace=Acme",
		"link=https://acme.example/administration/sign-in"} {
		if !strings.Contains(sent.Body, want) {
			t.Errorf("the body %q lacks %q", sent.Body, want)
		}
	}
}

func TestTheCloseSaysHowItEnded(t *testing.T) {
	for ended, body := range map[string]string{"OPERATOR": bodyPasswordClosedEarly, "EXPIRED": bodyPasswordClosed} {
		mailer := &mailbox{}
		err := openingSender(mailer).Execute(t.Context(), tenant, OpeningNotice{
			AccountID: carla, Event: OpeningEventClosed, Ended: ended,
		})
		if err != nil {
			t.Fatalf("%s: sending: %v", ended, err)
		}
		if len(mailer.sent) != 1 || !strings.Contains(mailer.sent[0].Body, body) ||
			!strings.Contains(mailer.sent[0].Subject, subjectPasswordClosed) {
			t.Errorf("%s: sent %+v", ended, mailer.sent)
		}
		// Carla chose no language: the workspace's.
		if strings.HasPrefix(mailer.sent[0].Subject, "[de]") {
			t.Errorf("%s: rendered in German for a recipient of an English workspace", ended)
		}
	}
}

func TestANoticeForSomebodyUnreachableIsFinishedBusiness(t *testing.T) {
	mailer := &mailbox{}
	sender := openingSender(mailer)
	for _, account := range []shared.ID{bert, shared.ID("01936f2a-7c1e-7000-8000-0000000000ff")} {
		if err := sender.Execute(t.Context(), tenant, OpeningNotice{
			AccountID: account, Event: OpeningEventOpened, Until: now,
		}); err != nil {
			t.Errorf("an unreachable recipient failed the job: %v", err)
		}
	}
	if len(mailer.sent) != 0 {
		t.Errorf("sent %+v to nobody", mailer.sent)
	}
}
