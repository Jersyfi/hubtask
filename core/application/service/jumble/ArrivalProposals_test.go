// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// UC-JUM-07 check 10: the seed reads the switch and the replay flag, and enqueues - nothing else.

type switchedWorkspace struct{ prepare bool }

func (s switchedWorkspace) Find(context.Context) (identity.Workspace, error) {
	workspace := identity.Workspace{Tenant: identity.Tenant{ID: tenant}}
	workspace.Settings.AiArrivalProposals = s.prepare
	return workspace, nil
}

func arrivalOf(t *testing.T, actor event.Actor) event.Envelope {
	t.Helper()
	entry, err := domain.NewEntry(domain.NewEntryInput{
		ID: entryID, TenantID: tenant, Channel: domain.ChannelAPI, RawSubject: "Order 42", Now: now,
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	envelope, err := event.NewJumbleEntryReceived(shared.ID("01936f2a-7c1e-7000-8000-0000000010e1"), entry, actor, now, event.Cause{})
	if err != nil {
		t.Fatalf("building the event: %v", err)
	}
	return envelope
}

func seed(prepare bool) (ArrivalProposals, *jobQueue, *auditSink) {
	jobs, trail := &jobQueue{}, &auditSink{}
	return ArrivalProposals{
		Workspaces: switchedWorkspace{prepare: prepare}, Jobs: jobs, Audit: trail, Clock: clock.Fixed(now),
	}, jobs, trail
}

func TestAnArrivalIsAskedAboutOnlyWhereTheWorkspaceSwitchedItOn(t *testing.T) {
	off, offJobs, offTrail := seed(false)
	if err := off.Deliver(context.Background(), arrivalOf(t, event.Actor{Kind: shared.ActorSystem})); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(offJobs.queued) != 0 || len(offTrail.entries) != 0 {
		t.Errorf("switched off, the seed queued %d and recorded %d", len(offJobs.queued), len(offTrail.entries))
	}

	on, jobs, trail := seed(true)
	if err := on.Deliver(context.Background(), arrivalOf(t, event.Actor{Kind: shared.ActorSystem})); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(jobs.queued) != 1 {
		t.Fatalf("%d jobs queued", len(jobs.queued))
	}
	job := jobs.queued[0]
	if job.Kind != queue.KindAiSuggestArrival || job.TenantID != tenant || job.DedupeKey != entryID.String() ||
		job.Payload["target_id"] != entryID.String() || job.MaxAttempts != queue.AiSuggestArrivalAttempts {
		t.Errorf("queued %+v", job)
	}
	// A mail or a webhook: nobody submitted it, so nobody is named.
	if _, named := job.Payload["submitted_by"]; named {
		t.Errorf("a system arrival named a submitter: %v", job.Payload)
	}
	if len(trail.entries) != 1 || trail.entries[0].Action != EntrySuggestionAskedAction ||
		trail.entries[0].ActorKind != shared.ActorSystem || len(trail.entries[0].Changes) != 0 {
		t.Errorf("the trail holds %+v", trail.entries)
	}
}

// Who made the arrival travels to the run, which judges whose content stays out against it: the
// account that submitted it, and the principal an agent or a rule acted for.
func TestWhoMadeAnArrivalTravelsToTheRun(t *testing.T) {
	agent := shared.ID("01936f2a-7c1e-7000-8000-0000000010a1")
	principal := shared.ID("01936f2a-7c1e-7000-8000-0000000010a2")
	on, jobs, _ := seed(true)
	if err := on.Deliver(context.Background(), arrivalOf(t, event.Actor{
		Kind: shared.ActorAIAgent, ID: agent, OnBehalfOf: principal,
	})); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	payload := jobs.queued[0].Payload
	if payload["submitted_by"] != agent.String() || payload["on_behalf_of"] != principal.String() {
		t.Errorf("the job carries %v", payload)
	}
}

// A restore's replay was asked about, or not, when it first arrived.
func TestAReplayedArrivalIsNotAskedAboutAgain(t *testing.T) {
	on, jobs, _ := seed(true)
	replayed := arrivalOf(t, event.Actor{Kind: shared.ActorSystem})
	replayed.Replay = true
	if err := on.Deliver(context.Background(), replayed); err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(jobs.queued) != 0 {
		t.Errorf("a replay queued %d jobs", len(jobs.queued))
	}
	if _, takes := any(on).(interface{ TakesReplays() }); takes {
		t.Error("the seed takes replays")
	}
}
