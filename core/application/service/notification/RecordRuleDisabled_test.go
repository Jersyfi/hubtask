// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	automation "github.com/Jersyfi/hubtask/core/domain/model/automation"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

func ruleRecorder(
	preferences *preferenceStore,
) (RecordRuleDisabled, *notificationStore, *jobQueue) {
	notifications, jobs := newNotifications(), &jobQueue{}
	return RecordRuleDisabled{
		Notifications: notifications, Preferences: preferences,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
		),
		Jobs:  jobs,
		Clock: clock.Fixed(now), IDs: &idSequence{}, Signals: &signalLog{},
	}, notifications, jobs
}

func disabledRule(author shared.ID) automation.Rule {
	return automation.Rule{
		ID:       shared.MustParseID("01936f2a-7c1e-7000-8000-0000000009a1"),
		TenantID: tenant, Name: "Escalate overdue approvals", CreatedBy: author,
	}
}

// A rule that stopped acting and told nobody is a rule whose owner discovers the silence weeks
// later, by which time the work has not been done and nobody knows since when.
func TestADisabledRuleTellsItsAuthor(t *testing.T) {
	service, notifications, jobs := ruleRecorder(newPreferences())

	if err := service.RuleDisabled(t.Context(), disabledRule(anna), now); err != nil {
		t.Fatalf("recording: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want one", len(written))
	}
	record := written[0]
	// The author, not the account the rule runs as: a service account has nobody behind it to read
	// a message, and the person who wrote the rule is the one who can fix it.
	if record.RecipientID != anna {
		t.Errorf("told %s, want the rule's author", record.RecipientID)
	}
	if record.Category != domain.CategoryIntegration {
		t.Errorf("category %q", record.Category)
	}
	// The rule stands in for the item: it is what the message is about.
	if record.ItemID != disabledRule(anna).ID {
		t.Errorf("the record is about %s", record.ItemID)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindNotificationDeliver {
		t.Fatalf("the delivery was not queued: %+v", jobs.requests)
	}
	// The record's own identifier, so a retried write cannot queue a second send of one message.
	if jobs.requests[0].DedupeKey != record.ID.String() {
		t.Errorf("the job's key is %q", jobs.requests[0].DedupeKey)
	}
}

// A rule written by something with no account behind it has nobody to tell, and inventing a
// recipient would be worse than the silence.
func TestARuleWithNoAuthorTellsNobody(t *testing.T) {
	service, notifications, jobs := ruleRecorder(newPreferences())

	if err := service.RuleDisabled(t.Context(), disabledRule(""), now); err != nil {
		t.Fatalf("recording: %v", err)
	}
	if len(notifications.written()) != 0 || len(jobs.requests) != 0 {
		t.Error("a rule with no author produced a notification")
	}
}

// The preference is honoured, exactly as it is for every other notification - the record still
// exists and says why nothing was sent.
func TestAnAuthorWhoSwitchedIntegrationMessagesOffIsNotSent(t *testing.T) {
	preferences := newPreferences()
	preferences.switchOff(anna, domain.CategoryIntegration)

	service, notifications, jobs := ruleRecorder(preferences)
	if err := service.RuleDisabled(t.Context(), disabledRule(anna), now); err != nil {
		t.Fatalf("recording: %v", err)
	}

	written := notifications.written()
	if len(written) != 1 {
		t.Fatalf("%d records written, want the suppressed one", len(written))
	}
	if written[0].State != domain.StateSuppressed {
		t.Errorf("state %q, want suppressed", written[0].State)
	}
	if len(jobs.requests) != 0 {
		t.Error("a suppressed message was queued for delivery")
	}
}
