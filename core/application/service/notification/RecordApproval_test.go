// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var askedApproval = shared.ID("01936f2a-7c1e-7000-8000-0000000000a7")

func waitingApproval() servicedesk.Approval {
	return servicedesk.Approval{
		ID: askedApproval, TenantID: tenant, ItemID: itemID, RequestedBy: anna,
		Levels: []servicedesk.OpenLevel{{Counted: []shared.ID{bert}}}, State: servicedesk.ApprovalPending,
	}
}

// UC-SVC-09 check 4 at the recording: the people a level counts get an ASSIGNMENT record naming the
// approval, a reminder a REMINDER one, each queued under the approval's kind; the person who asked
// is not told of their own request.
func TestAnApprovalsPeopleAreRecorded(t *testing.T) {
	notifications, jobs := newNotifications(), &jobQueue{}
	recorder := RecordApproval{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "de"),
		),
		Preferences: newPreferences(), Jobs: jobs, Clock: clock.Fixed(now), IDs: &idSequence{},
	}
	if err := recorder.ApprovalAsked(t.Context(), waitingApproval(), []shared.ID{anna, bert}, anna); err != nil {
		t.Fatal(err)
	}
	if err := recorder.ApprovalReminded(t.Context(), waitingApproval(), []shared.ID{bert}); err != nil {
		t.Fatal(err)
	}
	written := notifications.written()
	if len(written) != 2 || written[0].RecipientID != bert || written[0].ApprovalID != askedApproval ||
		written[0].Category != domain.CategoryAssignment || written[1].Category != domain.CategoryReminder {
		t.Fatalf("records %+v", written)
	}
	if len(jobs.requests) != 2 || jobs.requests[0].Kind != queue.KindApprovalEmail {
		t.Fatalf("queued %+v", jobs.requests)
	}
}

type approvalStore struct{ approval servicedesk.Approval }

func (s approvalStore) Find(_ context.Context, id shared.ID) (servicedesk.Approval, error) {
	if id != s.approval.ID {
		return servicedesk.Approval{}, shared.ErrNotFound
	}
	return s.approval, nil
}

// At the delivery: asked and reminded link to the approvals list, which opens the app and signs
// nobody in (P-02); the person who asked, no longer counted, is told the level is overdue and
// linked to the entry; each names the entry's title.
func TestTheApprovalMailSaysWhatWaits(t *testing.T) {
	for _, tc := range []struct {
		name      string
		category  domain.Category
		recipient shared.ID
		body      string
		link      string
	}{
		{"asked", domain.CategoryAssignment, bert, bodyApprovalAsked, "/approvals"},
		{"reminded", domain.CategoryReminder, bert, bodyApprovalReminder, "/approvals"},
		{"overdue", domain.CategoryReminder, anna, bodyApprovalOverdue, "/items/" + itemID.String()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := delivery(t, tc.category, true)
			record, err := domain.New(domain.NewInput{
				ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000a8"), TenantID: tenant, RecipientID: tc.recipient,
				Category: tc.category, Channel: domain.ChannelEmail, ApprovalID: askedApproval, At: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			fixture.delivery.Approvals = approvalStore{approval: waitingApproval()}
			if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.Contains(body, tc.body+" ") || !strings.Contains(body, tc.link) || !strings.Contains(body, "title=Review the quote") {
				t.Errorf("the body %q, want %s naming the entry and linking to %s", body, tc.body, tc.link)
			}
		})
	}
}
