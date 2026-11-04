// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The people list's delivery state (user-experience.md §8.3): the queue's last invitation job per
// account, in the list's words, asked of MANAGE_MEMBERS at the workspace.

type invitationJobsDouble struct {
	jobs  []repository.InvitationJob
	asked []shared.ID
}

func (d *invitationJobsDouble) Latest(_ context.Context, ids []shared.ID) ([]repository.InvitationJob, error) {
	d.asked = ids
	return d.jobs, nil
}

func TestThePeopleListSaysHowEachInvitationMailWent(t *testing.T) {
	queued := time.Date(2026, 11, 4, 9, 0, 0, 0, time.UTC)
	finished := queued.Add(time.Minute)
	ids := []shared.ID{
		shared.MustParseID("0192f000-0000-7000-8000-00000000d001"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000d002"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000d003"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000d004"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000d005"),
		shared.MustParseID("0192f000-0000-7000-8000-00000000d006"),
	}
	jobs := &invitationJobsDouble{jobs: []repository.InvitationJob{
		{AccountID: ids[0], State: "PENDING", QueuedAt: queued},
		{AccountID: ids[1], State: "PENDING", Code: "mail.unavailable", QueuedAt: queued},
		{AccountID: ids[2], State: "RUNNING", QueuedAt: queued},
		{AccountID: ids[3], State: "SUCCEEDED", QueuedAt: queued, FinishedAt: finished},
		{AccountID: ids[4], State: "DEAD_LETTER", Code: "mail.recipient_rejected", Params: map[string]string{"status": "550"}, QueuedAt: queued, FinishedAt: finished},
		{AccountID: ids[5], State: "CANCELLED", QueuedAt: queued, FinishedAt: finished},
	}}
	auth := &authorizer{}
	h := ReadInvitationDeliveries{Jobs: jobs, Authorizer: auth, UnitOfWork: &unitOfWork{}}
	actor := adminActor()

	got, err := h.Execute(context.Background(), actor, ids)
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	want := []struct {
		state, code string
		at          time.Time
	}{
		{"QUEUED", "", queued}, {"QUEUED", "mail.unavailable", queued}, {"SENDING", "", queued},
		{"SENT", "", finished}, {"FAILED", "mail.recipient_rejected", finished}, {"CANCELLED", "", finished},
	}
	if len(got) != len(want) {
		t.Fatalf("answered %d deliveries: %+v", len(got), got)
	}
	for i, w := range want {
		if got[i].AccountID != ids[i] || got[i].State != w.state || got[i].Code != w.code || !got[i].At.Equal(w.at) {
			t.Errorf("delivery %d: %+v, want %+v", i, got[i], w)
		}
	}
	if got[4].Params["status"] != "550" {
		t.Errorf("the failure's parameters were lost: %v", got[4].Params)
	}
	if len(auth.requests) != 1 || auth.requests[0].Permission != "MANAGE_MEMBERS" || auth.requests[0].TokenScope != membersRead {
		t.Errorf("asked %+v", auth.requests)
	}

	// Refused before the queue is read; bounded like a name lookup.
	refused := &authorizer{refuse: shared.ErrForbidden}
	jobs.asked = nil
	if _, err := (ReadInvitationDeliveries{Jobs: jobs, Authorizer: refused, UnitOfWork: &unitOfWork{}}).Execute(context.Background(), actor, ids); !errors.Is(err, shared.ErrForbidden) || jobs.asked != nil {
		t.Errorf("a reader without the right got %v and the queue was asked %v", err, jobs.asked)
	}
	many := make([]shared.ID, MaxResolvedAccounts+1)
	if _, err := h.Execute(context.Background(), actor, many); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("201 accounts: %v", err)
	}
}
