// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var (
	remindedRound = shared.ID("01936f2a-7c1e-7000-8000-0000000000d1")
	roundHub      = shared.ID("01936f2a-7c1e-7000-8000-0000000000d2")
)

// UC-AST-10 check 7 at the recording: each owner gets one ATTESTATION record naming the round and the
// writer, queued under its own kind; the writer is never told of their own reminder, and an owner
// who switched rounds off is recorded as suppressed with nothing queued.
func TestARoundsReminderIsRecordedForItsOwners(t *testing.T) {
	notifications, jobs, preferences := newNotifications(), &jobQueue{}, newPreferences()
	carl := shared.ID("01936f2a-7c1e-7000-8000-0000000000c9")
	preferences.switchOff(carl, domain.CategoryAttestation)
	recorder := RecordAttestation{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "de"),
			person(carl, "Carl", "carl@example.org", "en"),
		),
		Preferences: preferences, Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
	}
	if err := recorder.AttestationReminder(t.Context(), tenant, anna, remindedRound, []shared.ID{anna, bert, carl}); err != nil {
		t.Fatalf("recording: %v", err)
	}
	pending, suppressed := 0, 0
	for _, record := range notifications.written() {
		if record.Category != domain.CategoryAttestation || record.CampaignID != remindedRound || record.ActorID != anna || record.RecipientID == anna {
			t.Fatalf("record %+v", record)
		}
		switch record.State {
		case domain.StatePending:
			pending++
		case domain.StateSuppressed:
			suppressed++
		}
	}
	if pending != 1 || suppressed != 1 {
		t.Fatalf("%d pending, %d suppressed; want 1 and 1", pending, suppressed)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindAttestationEmail {
		t.Fatalf("queued %+v", jobs.requests)
	}
}

type rounds struct{ waiting int }

func (r rounds) RoundWaiting(context.Context, shared.ID, shared.ID) (string, shared.ID, int, error) {
	return "Q4 round", roundHub, r.waiting, nil
}

// At the delivery: the mail names the writer, the round and how many of the recipient's objects
// wait, and links to the hub's rounds.
func TestTheRoundsReminderNamesTheRoundAndTheCount(t *testing.T) {
	fixture := delivery(t, domain.CategoryAttestation, true)
	record, err := domain.New(domain.NewInput{
		ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000d8"), TenantID: tenant, RecipientID: bert,
		Category: domain.CategoryAttestation, Channel: domain.ChannelEmail, CampaignID: remindedRound, ActorID: anna, At: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
		t.Fatal(err)
	}
	fixture.delivery.Rounds = rounds{waiting: 3}
	if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(fixture.mailbox.sent) != 1 {
		t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
	}
	body := fixture.mailbox.sent[0].Body
	if !strings.HasPrefix(body, bodyAttestationReminder+" ") || !strings.Contains(body, "/hubs/"+roundHub.String()+"/inventory/campaigns") ||
		!strings.Contains(body, "title=Q4 round") || !strings.Contains(body, "count=3") {
		t.Errorf("the body %q", body)
	}
}
