// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

func (s *store) InsertComment(_ context.Context, comment domain.Comment) error {
	s.comments = append(s.comments, comment)
	return nil
}

func (s *store) FindComment(_ context.Context, id shared.ID) (domain.Comment, error) {
	for _, comment := range s.comments {
		if comment.ID == id {
			return comment, nil
		}
	}
	return domain.Comment{}, shared.ErrNotFound.WithDetail("whiteboards.comment_not_found")
}

func (s *store) Comments(_ context.Context, board shared.ID, _ int) ([]domain.Comment, error) {
	var out []domain.Comment
	for _, comment := range s.comments {
		if comment.BoardID == board {
			out = append(out, comment)
		}
	}
	return out, nil
}

func (s *store) UpdateComment(_ context.Context, comment domain.Comment, expected int) error {
	for i := range s.comments {
		if s.comments[i].ID == comment.ID {
			if s.comments[i].Version != expected {
				return shared.ErrVersionConflict
			}
			comment.Version = expected + 1
			s.comments[i] = comment
			return nil
		}
	}
	return shared.ErrNotFound
}

// readers answers that everybody but the outsider reads the board's hub.
type readers struct{ outsider shared.ID }

func (r readers) CanSee(_ context.Context, _ appshared.ActorContext, account shared.ID, _ []identity.Scope) (bool, error) {
	return account != r.outsider, nil
}

type told struct{ people [][]shared.ID }

func (t *told) Mentioned(_ context.Context, _, _, _ shared.ID, people []shared.ID, _ time.Time) error {
	t.people = append(t.people, people)
	return nil
}

var carl = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c007")

func named(id shared.ID) string { return "[@P](hubtask:person/" + id.String() + ")" }

// UC-CNV-09 checks 1 and 2: a thread is pinned to an object of the board, a reader lists it with its
// reply, and the people it names who read the board are told - not the writer, not an outsider; a
// rewrite tells only the person it adds.
func TestACommentIsPinnedAndTellsWhoItNames(t *testing.T) {
	s, _, sink := studio(gate{})
	mentions := &told{}
	s.People, s.Mentions = readers{outsider: carl}, mentions
	ctx := context.Background()
	out, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"hub_id": hub.String(), "title": "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	board := out["id"].(string)
	add := AddWhiteboardComment{Studio: s}.Descriptor().Handler
	thread, err := add.Invoke(ctx, actor(anna), usecase.Input{
		"whiteboard_id": board, "body": "Look " + named(bert) + named(anna) + named(carl),
		"element_id": "s1", "x": 120.0, "y": 40.0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if thread["element_id"] != "s1" || thread["x"] != 120.0 || thread["parent_id"] != nil {
		t.Fatalf("the thread %+v", thread)
	}
	if len(mentions.people) != 1 || len(mentions.people[0]) != 1 || mentions.people[0][0] != bert {
		t.Fatalf("told %v, want Bert alone", mentions.people)
	}
	if _, err := add.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": board, "body": "Agreed", "parent_id": thread["id"]}); err != nil {
		t.Fatal(err)
	}
	list, err := ListWhiteboardComments{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": board})
	if err != nil {
		t.Fatal(err)
	}
	if data := list["data"].([]usecase.Output); len(data) != 2 || data[1]["parent_id"] != thread["id"] {
		t.Fatalf("listed %+v", data)
	}
	change := ChangeWhiteboardComment{Studio: s}.Descriptor().Handler
	if _, err := change.Invoke(ctx, actor(anna), usecase.Input{"comment_id": thread["id"], "body": "Look " + named(bert) + named(carl)}); err != nil {
		t.Fatal(err)
	}
	if len(mentions.people) != 1 {
		t.Fatalf("a rewrite naming nobody new told %v", mentions.people)
	}
	other := shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c008")
	if _, err := change.Invoke(ctx, actor(anna), usecase.Input{"comment_id": thread["id"], "body": "Look " + named(bert) + named(other)}); err != nil {
		t.Fatal(err)
	}
	if len(mentions.people) != 2 || mentions.people[1][0] != other {
		t.Fatalf("told %v, want the person added", mentions.people)
	}
	for _, entry := range sink.entries {
		if body, ok := entry.Changes["body"].(map[string]any); ok && body["to"] != nil {
			t.Fatalf("a comment's words in the trail: %+v", body)
		}
	}
}

// UC-CNV-09 check 3: whoever may draw resolves a thread; only its author rewrites or takes it back;
// a reader of the hub who may not draw writes none, and nobody outside it reads them.
func TestACommentIsResolvedByAWriterAndChangedByItsAuthor(t *testing.T) {
	s, _, _ := studio(gate{reader: true})
	ctx := context.Background()
	out, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"hub_id": hub.String(), "title": "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	board := out["id"].(string)
	add := AddWhiteboardComment{Studio: s}.Descriptor().Handler
	thread, err := add.Invoke(ctx, actor(anna), usecase.Input{"whiteboard_id": board, "body": "Here", "x": 5.0, "y": 5.0})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := add.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": board, "body": "Me too"}); err == nil {
		t.Fatal("a reader who may not draw commented")
	}
	if _, err := add.Invoke(ctx, actor(anna), usecase.Input{"whiteboard_id": board, "body": "x", "x": "far"}); detail(err) != "whiteboards.comment_pin_malformed" {
		t.Fatalf("a malformed place: %v", err)
	}
	change := ChangeWhiteboardComment{Studio: s}.Descriptor().Handler
	if _, err := change.Invoke(ctx, actor(bert), usecase.Input{"comment_id": thread["id"], "resolved": true}); err == nil {
		t.Fatal("a reader resolved the thread")
	}
	resolved, err := change.Invoke(ctx, actor(anna), usecase.Input{"comment_id": thread["id"], "resolved": true})
	if err != nil || resolved["resolved"] != true || resolved["version"] != 2 {
		t.Fatalf("resolved %+v, %v", resolved, err)
	}
	s2, _, _ := studio(gate{})
	s2.Boards = s.Boards
	if _, err := (ChangeWhiteboardComment{Studio: s2}).Descriptor().Handler.Invoke(ctx, actor(bert), usecase.Input{"comment_id": thread["id"], "body": "Mine now"}); detail(err) != "whiteboards.comment_not_own" {
		t.Fatalf("another person rewrote the comment: %v", err)
	}
	if _, err := (DeleteWhiteboardComment{Studio: s}).Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"comment_id": thread["id"]}); err != nil {
		t.Fatal(err)
	}
	list, err := ListWhiteboardComments{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": board})
	if err != nil {
		t.Fatal(err)
	}
	if data := list["data"].([]usecase.Output); len(data) != 1 || data[0]["deleted"] != true || data[0]["body"] != "" {
		t.Fatalf("a comment taken back reads %+v", data)
	}
	privateBoard, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(ctx, actor(anna), usecase.Input{"hub_id": private.String(), "title": "Mine"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (ListWhiteboardComments{Studio: s}).Descriptor().Handler.Invoke(ctx, actor(bert), usecase.Input{"whiteboard_id": privateBoard["id"]}); err == nil {
		t.Fatal("somebody outside the hub read its board's comments")
	}
}
