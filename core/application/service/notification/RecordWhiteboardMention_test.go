// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"strings"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

var mentionBoard = shared.ID("01936f2a-7c1e-7000-8000-0000000000f1")

// UC-CNV-09 check 2 at the recording: each person a comment names gets one COMMENT record naming
// the board and the writer, its delivery queued under its own kind; the writer is not told.
func TestThePeopleACommentNamesAreRecorded(t *testing.T) {
	notifications, jobs := newNotifications(), &jobQueue{}
	recorder := RecordWhiteboardMention{
		Notifications: notifications,
		Accounts: newAccounts(
			person(anna, "Anna", "anna@example.org", "en"),
			person(bert, "Bert", "bert@example.org", "de"),
		),
		Preferences: newPreferences(), Jobs: jobs, IDs: &idSequence{},
	}
	if err := recorder.Mentioned(t.Context(), tenant, mentionBoard, anna, []shared.ID{anna, bert}, now); err != nil {
		t.Fatalf("recording: %v", err)
	}
	written := notifications.written()
	if len(written) != 1 || written[0].RecipientID != bert || written[0].WhiteboardID != mentionBoard ||
		written[0].ActorID != anna || written[0].Category != domain.CategoryComment || written[0].State != domain.StatePending {
		t.Fatalf("records %+v", written)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindWhiteboardMentionEmail {
		t.Fatalf("queued %+v", jobs.requests)
	}
}

type boardStore struct{ board whiteboard.Board }

func (s boardStore) Find(_ context.Context, id shared.ID) (whiteboard.Board, error) {
	if id != s.board.ID {
		return whiteboard.Board{}, shared.ErrNotFound
	}
	return s.board, nil
}

// At the delivery: the mail names the board and links to it; a board that is gone leaves the
// withheld sentence.
func TestTheMentionMailNamesTheBoard(t *testing.T) {
	for _, tc := range []struct {
		name   string
		boards boardStore
		body   string
	}{
		{"the board", boardStore{board: whiteboard.Board{ID: mentionBoard, Title: "Retro"}}, bodyWhiteboardMention},
		{"the board gone", boardStore{}, bodyWhiteboardMention + withheldSuffix},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := delivery(t, domain.CategoryComment, true)
			record, err := domain.New(domain.NewInput{
				ID: shared.ID("01936f2a-7c1e-7000-8000-0000000000f8"), TenantID: tenant, RecipientID: bert,
				Category: domain.CategoryComment, Channel: domain.ChannelEmail, WhiteboardID: mentionBoard, At: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := fixture.notifications.Insert(t.Context(), record); err != nil {
				t.Fatal(err)
			}
			fixture.delivery.Boards = tc.boards
			if err := fixture.delivery.Execute(t.Context(), tenant, record.ID, false); err != nil {
				t.Fatalf("delivering: %v", err)
			}
			if len(fixture.mailbox.sent) != 1 {
				t.Fatalf("%d messages sent, want one", len(fixture.mailbox.sent))
			}
			body := fixture.mailbox.sent[0].Body
			if !strings.HasPrefix(body, tc.body+" ") || !strings.Contains(body, "/whiteboards/"+mentionBoard.String()) {
				t.Errorf("the body %q, want %s linking to the board", body, tc.body)
			}
			if tc.body == bodyWhiteboardMention && !strings.Contains(body, "title=Retro") {
				t.Errorf("the body %q does not name the board", body)
			}
		})
	}
}
