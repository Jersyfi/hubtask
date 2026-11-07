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

type addressQueue struct {
	requests []queue.Request
	keys     map[string]bool
}

func (q *addressQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	if q.keys == nil {
		q.keys = map[string]bool{}
	}
	if !q.keys[request.DedupeKey] {
		q.keys[request.DedupeKey] = true
		q.requests = append(q.requests, request)
	}
	return "", nil
}

type noticeStore struct {
	recorded map[shared.ID]identity.AddressChangeNotice
}

func (n *noticeStore) Record(_ context.Context, account shared.ID, from, to string, at time.Time) error {
	n.recorded[account] = identity.AddressChangeNotice{From: from, To: to, ChangedAt: at}
	return nil
}
func (n *noticeStore) Of(context.Context, shared.ID) ([]identity.AddressChangeNotice, error) {
	return nil, nil
}
func (n *noticeStore) Acknowledge(context.Context, shared.ID) error { return nil }

// UC-INS-19 check 8: every member with an address is mailed once - a retried write queues nothing
// twice - and a member without one is told at the next sign-in.
func TestAChangeOfAddressIsToldOnceToEveryMember(t *testing.T) {
	jobs, notices := &addressQueue{}, &noticeStore{recorded: map[shared.ID]identity.AddressChangeNotice{}}
	record := RecordAddressChange{Jobs: jobs, Notices: notices}
	at := time.Date(2026, 10, 28, 9, 0, 0, 0, time.UTC)
	mailed := []shared.ID{"018f2a1b-0000-7000-8000-0000000000d1", "018f2a1b-0000-7000-8000-0000000000d2"}
	unmailed := []shared.ID{"018f2a1b-0000-7000-8000-0000000000d3"}
	for range 2 {
		if err := record.AddressChanged(t.Context(), "018f2a1b-0000-7000-8000-0000000000b1",
			"meier.tasks.example.net", "familie-meier.tasks.example.net", at, mailed, unmailed); err != nil {
			t.Fatal(err)
		}
	}
	if len(jobs.requests) != 2 {
		t.Fatalf("%d mails queued, want one per member with an address", len(jobs.requests))
	}
	for _, request := range jobs.requests {
		if request.Kind != queue.KindAddressChangedEmail || request.Payload["from"] != "meier.tasks.example.net" ||
			request.Payload["to"] != "familie-meier.tasks.example.net" {
			t.Errorf("job %+v", request)
		}
	}
	if got := notices.recorded[unmailed[0]]; got.From != "meier.tasks.example.net" || len(notices.recorded) != 1 {
		t.Errorf("notices %+v", notices.recorded)
	}
}

// UC-INS-19 checks 8 and 9: the mail names the old and the new address in the member's language,
// and its link the workspace's main row; a member who is gone, or has no address, is sent nothing.
func TestTheAddressChangeMailNamesBothAddressesAndTheMainRow(t *testing.T) {
	const gone = shared.ID("01936f2a-7c1e-7000-8000-0000000000c9")
	silent := person("01936f2a-7c1e-7000-8000-0000000000ca", "Ola", "", "en")
	mailer := &mailbox{}
	send := SendAddressChange{
		Accounts: newAccounts(person(anna, "Anna", "anna@example.org", "de"), silent),
		Mail:     mailer, Renderer: catalogue{}, FallbackLocale: "en", UnitOfWork: &unitOfWork{},
		Addresses: movedAddress(),
	}
	notice := AddressNotice{AccountID: anna, From: "anna.hub.example.org", To: "tasks.anna.example"}
	if err := send.Execute(t.Context(), tenant, notice); err != nil || len(mailer.sent) != 1 {
		t.Fatalf("sending: %v, %d sent", err, len(mailer.sent))
	}
	sent := mailer.sent[0]
	if sent.To != "anna@example.org" || !strings.HasPrefix(sent.Subject, "[de] "+subjectAddressChanged) {
		t.Errorf("sent %q to %q, want the German subject to anna", sent.Subject, sent.To)
	}
	if !strings.Contains(sent.Body, "from=anna.hub.example.org") || !strings.Contains(sent.Body, "to=tasks.anna.example") {
		t.Errorf("the body names neither address: %q", sent.Body)
	}
	if !strings.Contains(sent.Body, "link="+mainBase) {
		t.Errorf("the link is not the main row: %q", sent.Body)
	}
	for _, who := range []shared.ID{gone, silent.ID} {
		if err := send.Execute(t.Context(), tenant, AddressNotice{AccountID: who, From: notice.From, To: notice.To}); err != nil {
			t.Errorf("%s: %v", who, err)
		}
	}
	if len(mailer.sent) != 1 {
		t.Errorf("%d mails, want none to a member who is gone or has no address", len(mailer.sent)-1)
	}
}
