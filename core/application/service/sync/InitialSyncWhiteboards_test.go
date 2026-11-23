// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"slices"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

func (s *snapshotStore) Whiteboards(_ context.Context, after shared.ID, batch int) ([]whiteboard.Board, error) {
	return pageAfter(s.boards, func(b whiteboard.Board) string { return b.ID.String() }, after.String(), batch), nil
}

func (s *snapshotStore) WhiteboardComments(_ context.Context, after shared.ID, batch int) ([]repository.BoardComment, error) {
	return pageAfter(s.boardNotes, func(c repository.BoardComment) string { return c.Comment.ID.String() }, after.String(), batch), nil
}

// UC-CNV-09 check 6, the device's copy: the walk carries a board's comments after the boards,
// each filed under its board's hub and as ListWhiteboardComments answers it - a comment taken
// back without its words -, and a comment on a board in a hub the reader may not read is withheld.
func TestTheWalkCarriesTheCommentsOfTheBoardsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	board, secret := idAt('9', 1), idAt('9', 2)
	said, taken, hidden := idAt('8', 1), idAt('8', 2), idAt('8', 3)
	gone := now.Add(-time.Minute)
	store.boards = []whiteboard.Board{
		{ID: board, TenantID: tenant, HubID: hub, Title: "Retro", Version: 1},
		{ID: secret, TenantID: tenant, HubID: otherHub, Title: "Secret", Version: 1},
	}
	store.boardNotes = []repository.BoardComment{
		{Comment: whiteboard.Comment{ID: said, BoardID: board, AuthorID: account, Body: "Move this", Element: "s1", Version: 1}, HubID: hub},
		{Comment: whiteboard.Comment{ID: taken, BoardID: board, AuthorID: account, Body: "left behind", DeletedAt: &gone, Version: 2}, HubID: hub},
		{Comment: whiteboard.Comment{ID: hidden, BoardID: secret, AuthorID: account, Body: "Not yours", Version: 1}, HubID: otherHub},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	var comments []shared.ID
	lastBoard := -1
	for i, record := range records {
		if record.Entity == "whiteboard" {
			lastBoard = i
		}
		if record.Entity != "whiteboard_comment" {
			continue
		}
		if lastBoard < 0 {
			t.Errorf("a comment was walked before any board")
		}
		comments = append(comments, record.EntityID)
		if record.ContainerID != hub || record.Payload["whiteboard_id"] != board.String() {
			t.Errorf("a comment record %+v, want it filed under its board's hub", record)
		}
		switch record.EntityID {
		case said:
			if record.Payload["body"] != "Move this" || record.Payload["deleted"] != false {
				t.Errorf("the comment's record: %+v", record.Payload)
			}
		case taken:
			if record.Payload["body"] != "" || record.Payload["deleted"] != true {
				t.Errorf("a comment taken back carries words: %+v", record.Payload)
			}
		}
	}
	if !slices.Equal(comments, []shared.ID{said, taken}) {
		t.Errorf("comments delivered %v, want %v - never %s, whose hub is out of reach", comments, []shared.ID{said, taken}, hidden)
	}
}

// UC-CNV-09 check 6, the open client: a comment's record is filed under its board's hub, and READ
// there decides who the stream sends it to, as for any record filed under a hub. That the comment
// is a kind of the walk is not asked here; the walk above proves it.
func TestTheStreamSendsACommentRecordByReadOnItsBoardsHub(t *testing.T) {
	mine, theirs := entry(1, hub), entry(2, otherHub)
	mine.Entity, theirs.Entity = entityWhiteboardComment, entityWhiteboardComment
	pull, f := pulling(t, mine, theirs)
	f.auth.allowed[otherHub] = false

	batch, err := pull.Stream.Next(t.Context(), actor(), Position{IssuedAt: now})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	if len(batch.Records) != 1 || batch.Records[0].ContainerID != hub {
		t.Errorf("the stream sent %+v, want the comment of the hub the reader reads alone", batch.Records)
	}
}

// A board is read offline before it was ever opened (UC-CNV-01 check 9): the walk carries the
// boards with their scenes, each filed under its hub, and a board in a hub the reader may not read
// is withheld.
func TestTheWalkCarriesTheBoardsOfTheHubsTheReaderReads(t *testing.T) {
	pull, f, store := walking(t)
	shown, hidden := idAt('9', 1), idAt('9', 2)
	note := whiteboard.Element{ID: "s1", Kind: whiteboard.KindSticky, Width: 160, Height: 160, Text: "Keep it"}
	store.boards = []whiteboard.Board{
		{ID: shown, TenantID: tenant, HubID: hub, Title: "Retro", Scene: whiteboard.Scene{Elements: []whiteboard.Element{note}}, Version: 3},
		{ID: hidden, TenantID: tenant, HubID: otherHub, Title: "Secret", Version: 1},
	}
	f.auth.allowed[otherHub] = false

	records, _ := wholeWalk(t, pull, 100)

	var boards []shared.ID
	for _, record := range records {
		if record.Entity != "whiteboard" {
			continue
		}
		boards = append(boards, record.EntityID)
		elements, _ := record.Payload["elements"].([]map[string]any)
		if record.ContainerID != hub || record.Payload["hub_id"] != hub.String() || record.Payload["title"] != "Retro" ||
			record.Payload["element_count"] != 1 || (elements == nil && record.Payload["elements"] == nil) {
			t.Errorf("a board record %+v, want it filed under its hub with its scene", record)
		}
	}
	if !slices.Equal(boards, []shared.ID{shown}) {
		t.Errorf("boards delivered %v, want only %s - never %s, whose hub is out of reach", boards, shown, hidden)
	}
}
