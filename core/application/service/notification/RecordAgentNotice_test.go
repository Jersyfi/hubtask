// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var notedAgent = shared.ID("01936f2a-7c1e-7000-8000-0000000000e1")

// UC-AGT-02 check 3 at the recording: the owner of an agent that acted with them told gets one
// INTEGRATION record naming the agent, its delivery queued under its own kind.
func TestAnAgentsOwnerIsToldItActed(t *testing.T) {
	notifications, jobs := newNotifications(), &jobQueue{}
	recorder := RecordAgentNotice{
		Notifications: notifications,
		Accounts:      newAccounts(person(anna, "Anna", "anna@example.org", "en")),
		Preferences:   newPreferences(), Jobs: jobs, IDs: &idSequence{},
	}
	if err := recorder.AgentActed(t.Context(), tenant, notedAgent, anna, now); err != nil {
		t.Fatalf("recording: %v", err)
	}
	written := notifications.written()
	if len(written) != 1 || written[0].AgentID != notedAgent || written[0].RecipientID != anna ||
		written[0].Category != domain.CategoryIntegration || written[0].State != domain.StatePending {
		t.Fatalf("records %+v", written)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindAgentNoticeEmail {
		t.Fatalf("queued %+v", jobs.requests)
	}
	if err := recorder.AgentActed(t.Context(), tenant, notedAgent, "", now); err != nil || len(notifications.written()) != 1 {
		t.Fatalf("an agent without an owner told somebody: %v", err)
	}
}
