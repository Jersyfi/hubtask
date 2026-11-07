// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"sort"
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	team      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d001")
	product   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d002")
	foreign   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d003")
	elsewhere = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000d004")
)

// contents is the hub's tree: its pages by parent with their keys, and the store's boards.
type contents struct {
	store *store
	pages map[shared.ID]repository.Sibling // the page and its key
	under map[shared.ID]shared.ID          // page -> its parent
	hubOf map[shared.ID]shared.ID
}

func (c contents) ParentHub(_ context.Context, id shared.ID) (shared.ID, error) {
	if h, ok := c.hubOf[id]; ok {
		return h, nil
	}
	return "", shared.ErrNotFound
}

func (c contents) Siblings(_ context.Context, hubID, parent shared.ID) ([]repository.Sibling, error) {
	var out []repository.Sibling
	for id, page := range c.pages {
		if c.under[id] == parent && c.hubOf[id] == hubID {
			out = append(out, page)
		}
	}
	for _, board := range c.store.boards {
		if board.HubID == hubID && board.ParentPageID == parent && board.OrderKey != "" && !board.IsTrashed() {
			out = append(out, repository.Sibling{ID: board.ID, OrderKey: board.OrderKey, IsBoard: true})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].OrderKey < out[j].OrderKey })
	return out, nil
}

func (c contents) Under(_ context.Context, pages []shared.ID) ([]shared.ID, error) {
	var out []shared.ID
	for _, board := range c.store.boards {
		for _, page := range pages {
			if board.ParentPageID == page && !board.IsTrashed() {
				out = append(out, board.ID)
			}
		}
	}
	return out, nil
}

func placed(t *testing.T) (Studio, *store) {
	t.Helper()
	s, boards, _ := studio(gate{})
	s.Contents = contents{
		store: boards,
		pages: map[shared.ID]repository.Sibling{
			team:    {ID: team, OrderKey: "a0"},
			product: {ID: product, OrderKey: "a1"},
			foreign: {ID: foreign, OrderKey: "a0"},
		},
		under: map[shared.ID]shared.ID{},
		hubOf: map[shared.ID]shared.ID{team: hub, product: hub, foreign: elsewhere},
	}
	return s, boards
}

// UC-CNV-10 checks 1 and 2: a board is created under a page after a sibling, moved to the top
// after a sibling page, and the tree order lists it by parent and place.
func TestABoardIsCreatedUnderAPageAndMovedAmongThePages(t *testing.T) {
	s, _ := placed(t)
	ctx := context.Background()
	retro, err := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Retro", ParentPageID: team})
	if err != nil || retro.ParentPageID != team || retro.OrderKey == "" {
		t.Fatalf("created %+v, %v; want it under Team with a key", retro, err)
	}
	second, err := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Second", ParentPageID: team, AfterID: retro.ID})
	if err != nil || second.OrderKey <= retro.OrderKey {
		t.Fatalf("created after the retro %+v, %v", second, err)
	}
	moved, err := MoveWhiteboard{Studio: s}.Execute(ctx, actor(anna), MoveWhiteboardCommand{WhiteboardID: second.ID, AfterID: team})
	if err != nil || !moved.ParentPageID.IsZero() || moved.OrderKey <= "a0" || moved.OrderKey >= "a1" {
		t.Fatalf("moved %+v, %v; want it at the top between Team and Product", moved, err)
	}
	if moved.Version != second.Version+1 {
		t.Errorf("version %d, want one more", moved.Version)
	}
	if _, err := (MoveWhiteboard{Studio: s}).Execute(ctx, actor(anna), MoveWhiteboardCommand{WhiteboardID: moved.ID, ExpectedVersion: 1}); detail(err) != "whiteboards.version_conflict" {
		t.Errorf("a stale move: %v", err)
	}
	if _, err := (MoveWhiteboard{Studio: s}).Execute(ctx, actor(anna), MoveWhiteboardCommand{WhiteboardID: moved.ID, AfterID: retro.ID}); detail(err) != "whiteboards.after_not_sibling" {
		t.Errorf("after a board of another parent: %v", err)
	}
	tree, _, err := ListWhiteboards{Studio: s}.Execute(ctx, actor(anna), hub, false, "", 0, true)
	if err != nil || len(tree) != 2 || tree[0]["id"] != moved.ID.String() || tree[1]["parent_page_id"] != team.String() {
		t.Errorf("tree %v, %v; want the top-level board first, then the one under Team", tree, err)
	}
}

// UC-CNV-10 check 3: another hub's page and a board are no parents.
func TestABoardStandsOnlyUnderAPageOfItsHub(t *testing.T) {
	s, _ := placed(t)
	ctx := context.Background()
	board, err := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Map"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (MoveWhiteboard{Studio: s}).Execute(ctx, actor(anna), MoveWhiteboardCommand{WhiteboardID: board.ID, ParentPageID: foreign}); detail(err) != "whiteboards.parent_other_space" {
		t.Errorf("under another hub's page: %v", err)
	}
	other, _ := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Other"})
	if _, err := (MoveWhiteboard{Studio: s}).Execute(ctx, actor(anna), MoveWhiteboardCommand{WhiteboardID: board.ID, ParentPageID: other.ID}); detail(err) != "whiteboards.no_children" {
		t.Errorf("under a board: %v", err)
	}
	if _, err := (CreateWhiteboard{Studio: s}).Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Child", ParentPageID: other.ID}); detail(err) != "whiteboards.no_children" {
		t.Errorf("created under a board: %v", err)
	}
}

// UC-CNV-10 check 4: the boards under a trashed page are lifted to its parent at its place, in
// their order, and keep everything else.
func TestLiftingPutsTheBoardsWhereThePageStood(t *testing.T) {
	s, boards := placed(t)
	ctx := context.Background()
	first, _ := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "One", ParentPageID: team, Scene: nil})
	second, _ := CreateWhiteboard{Studio: s}.Execute(ctx, actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Two", ParentPageID: team, AfterID: first.ID})
	if err := s.LiftFrom(ctx, actor(anna), []shared.ID{team}, "", "a0", "a1"); err != nil {
		t.Fatalf("lifting: %v", err)
	}
	one, two := boards.boards[first.ID], boards.boards[second.ID]
	if !one.ParentPageID.IsZero() || !two.ParentPageID.IsZero() {
		t.Fatalf("parents %s, %s; want the top", one.ParentPageID, two.ParentPageID)
	}
	if one.OrderKey <= "a0" || two.OrderKey >= "a1" || one.Title != "One" {
		t.Errorf("keys %s, %s; want both between a0 and a1", one.OrderKey, two.OrderKey)
	}
}
