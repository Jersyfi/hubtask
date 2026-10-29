// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard_test

import (
	"errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
)

const (
	entryID = shared.ID("0190a000-0000-7000-8000-000000000001")
	boardID = shared.ID("0190a000-0000-7000-8000-000000000002")
)

func sticky(id string) whiteboard.Element {
	return whiteboard.Element{ID: id, Kind: whiteboard.KindSticky, X: 10, Y: 10, Width: 160, Height: 160, Text: "Idea"}
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

func TestCheckSceneJudgesEveryObject(t *testing.T) {
	card := whiteboard.Element{ID: "c", Kind: whiteboard.KindCard, Width: 240, Height: 120,
		Target: reference.Reference{Kind: reference.KindEntry, ID: entryID}}
	cases := []struct {
		name     string
		elements []whiteboard.Element
		want     string
	}{
		{"a sticky note, a shape, a connector between them, a frame and a card", []whiteboard.Element{
			sticky("a"), {ID: "b", Kind: whiteboard.KindShape, Shape: whiteboard.ShapeDiamond, Width: 80, Height: 80},
			{ID: "l", Kind: whiteboard.KindConnector, From: "a", To: "b", Route: whiteboard.RouteElbow},
			{ID: "f", Kind: whiteboard.KindFrame, Width: 1000, Height: 800, Text: "Start"}, card,
		}, ""},
		{"a free connector runs backwards", []whiteboard.Element{{ID: "l", Kind: whiteboard.KindConnector, Width: -40, Height: -10}}, ""},
		{"an identifier not shaped as an anchor", []whiteboard.Element{sticky("a b")}, "whiteboards.element_id_malformed"},
		{"two objects of one identifier", []whiteboard.Element{sticky("a"), sticky("a")}, "whiteboards.element_id_duplicate"},
		{"an unknown kind", []whiteboard.Element{{ID: "a", Kind: "PEN"}}, "whiteboards.element_kind_unknown"},
		{"an unknown outline", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindShape, Shape: "STAR"}}, "whiteboards.element_shape_unknown"},
		{"a position beyond the board", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindText, X: 2e6}}, "whiteboards.element_out_of_bounds"},
		{"not a number", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindText, Y: math.NaN()}}, "whiteboards.element_out_of_bounds"},
		{"a sticky note of negative width", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindSticky, Width: -1}}, "whiteboards.element_size_invalid"},
		{"a colour that is no label token", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindSticky, Colour: "#ff0000"}}, "whiteboards.element_colour_unknown"},
		{"too many words", []whiteboard.Element{{ID: "a", Kind: whiteboard.KindSticky, Text: strings.Repeat("x", 2001)}}, "whiteboards.element_text_too_long"},
		{"a connector to nothing on the board", []whiteboard.Element{sticky("a"), {ID: "l", Kind: whiteboard.KindConnector, From: "a", To: "z"}}, "whiteboards.connector_end_unknown"},
		{"a connector to a connector", []whiteboard.Element{
			sticky("a"), {ID: "l", Kind: whiteboard.KindConnector, From: "a"}, {ID: "m", Kind: whiteboard.KindConnector, From: "l"},
		}, "whiteboards.connector_end_unknown"},
		{"an unknown route", []whiteboard.Element{{ID: "l", Kind: whiteboard.KindConnector, Route: "ZIGZAG"}}, "whiteboards.connector_route_unknown"},
		{"a card of a page", []whiteboard.Element{{ID: "c", Kind: whiteboard.KindCard, Target: reference.Reference{Kind: reference.KindPage, ID: entryID}}}, "whiteboards.card_target_unsupported"},
		{"a card of no entry", []whiteboard.Element{{ID: "c", Kind: whiteboard.KindCard, Target: reference.Reference{Kind: reference.KindEntry}}}, "whiteboards.card_target_malformed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := whiteboard.CheckScene(whiteboard.Scene{Elements: tc.elements}, nil)
			if got := detail(err); got != tc.want {
				t.Fatalf("detail = %q, want %q (err %v)", got, tc.want, err)
			}
		})
	}
}

func TestCheckSceneBoundsTheBoard(t *testing.T) {
	elements := make([]whiteboard.Element, whiteboard.MaxElements+1)
	for i := range elements {
		elements[i] = whiteboard.Element{ID: "e" + strings.Repeat("x", i%3) + string(rune('a'+i%26)) + itoa(i), Kind: whiteboard.KindText}
	}
	if _, err := whiteboard.CheckScene(whiteboard.Scene{Elements: elements}, nil); detail(err) != "whiteboards.too_many_elements" {
		t.Fatalf("err = %v, want too_many_elements", err)
	}
	if _, err := whiteboard.CheckScene(whiteboard.Scene{Elements: elements[:whiteboard.MaxElements]}, nil); err != nil {
		t.Fatalf("a full board is refused: %v", err)
	}
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for ; i > 0; i /= 10 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
	}
	return string(digits)
}

func TestCheckSceneKeepsOnlyWhatAKindDraws(t *testing.T) {
	scene, err := whiteboard.CheckScene(whiteboard.Scene{Elements: []whiteboard.Element{
		{ID: "a", Kind: whiteboard.KindSticky, Shape: whiteboard.ShapeDiamond, Route: whiteboard.RouteCurved, From: "b",
			Target: reference.Reference{Kind: reference.KindEntry, ID: entryID}},
		{ID: "b", Kind: whiteboard.KindShape},
		{ID: "l", Kind: whiteboard.KindConnector, From: "a", To: "b"},
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	a, b, l := scene.Elements[0], scene.Elements[1], scene.Elements[2]
	if a.Shape != "" || a.Route != "" || a.From != "" || a.Target != (reference.Reference{}) {
		t.Fatalf("a sticky note kept another kind's members: %+v", a)
	}
	if b.Shape != whiteboard.ShapeRectangle {
		t.Fatalf("a shape without an outline is %q, want a rectangle", b.Shape)
	}
	if l.Route != whiteboard.RouteStraight || l.Start != whiteboard.HeadNone || l.End != whiteboard.HeadArrow {
		t.Fatalf("a connector's defaults: %+v", l)
	}
}

func TestReferencesNameCardsAndLinksOnce(t *testing.T) {
	other := shared.ID("0190a000-0000-7000-8000-000000000003")
	scene := whiteboard.Scene{Elements: []whiteboard.Element{
		{ID: "s", Kind: whiteboard.KindSticky, Text: "see [it](hubtask:entry/" + entryID.String() + ") and [page](hubtask:page/" + other.String() + ")"},
		{ID: "c", Kind: whiteboard.KindCard, Target: reference.Reference{Kind: reference.KindEntry, ID: entryID}},
		{ID: "t", Kind: whiteboard.KindText, Text: "[me](hubtask:whiteboard/" + boardID.String() + ")"},
	}}
	found := scene.References(boardID)
	if len(found) != 2 {
		t.Fatalf("found %d references, want the entry and the page: %+v", len(found), found)
	}
	if found[0].Reference.ID != entryID || found[0].Strength != reference.Card {
		t.Fatalf("the entry is drawn as %+v, want its card", found[0])
	}
	if found[1].Reference.Kind != reference.KindPage || found[1].Strength != reference.Mention {
		t.Fatalf("the page is %+v, want a mention", found[1])
	}
	if cards := scene.Cards(); len(cards) != 1 || cards[0] != entryID {
		t.Fatalf("cards = %v", cards)
	}
	trashed := time.Now()
	if refs := (whiteboard.Board{ID: boardID, Scene: scene, DeletedAt: &trashed}).References(); refs != nil {
		t.Fatalf("a trashed board points at %v", refs)
	}
}

func TestWithinAFrame(t *testing.T) {
	scene := whiteboard.Scene{Elements: []whiteboard.Element{
		{ID: "f", Kind: whiteboard.KindFrame, X: 0, Y: 0, Width: 500, Height: 500},
		{ID: "in", Kind: whiteboard.KindSticky, X: 10, Y: 10, Width: 100, Height: 100},
		{ID: "out", Kind: whiteboard.KindSticky, X: 600, Y: 10, Width: 100, Height: 100},
		{ID: "edge", Kind: whiteboard.KindSticky, X: 450, Y: 10, Width: 100, Height: 100},
		{ID: "l1", Kind: whiteboard.KindConnector, From: "in", To: "f"},
		{ID: "l2", Kind: whiteboard.KindConnector, From: "in", To: "out"},
	}}
	frame, ok := scene.Frame("f")
	if !ok {
		t.Fatal("the frame is not found")
	}
	if _, ok := scene.Frame("in"); ok {
		t.Fatal("a sticky note is found as a frame")
	}
	var ids []string
	for _, element := range scene.Within(frame).Elements {
		ids = append(ids, element.ID)
	}
	if strings.Join(ids, ",") != "f,in,l1" {
		t.Fatalf("within the frame: %v", ids)
	}
}

func TestBoardChanged(t *testing.T) {
	now := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	board, err := whiteboard.NewBoard(whiteboard.NewBoardInput{
		ID: boardID, TenantID: entryID, HubID: entryID, Title: "  Retro  ", CreatedBy: entryID, Now: now,
	})
	if err != nil || board.Title != "Retro" {
		t.Fatalf("board %+v, err %v", board, err)
	}
	same := "Retro"
	if _, fields, _ := board.Changed(whiteboard.Change{Title: &same, Scene: &whiteboard.Scene{}}, entryID, now, nil); len(fields) != 0 {
		t.Fatalf("an unchanged board changed %v", fields)
	}
	scene := whiteboard.Scene{Elements: []whiteboard.Element{sticky("a")}}
	next, fields, err := board.Changed(whiteboard.Change{Scene: &scene}, entryID, now.Add(time.Minute), nil)
	if err != nil || len(fields) != 1 || fields[0] != whiteboard.FieldScene || !next.UpdatedAt.After(now) {
		t.Fatalf("fields %v, err %v", fields, err)
	}
	empty := " "
	if _, _, err := board.Changed(whiteboard.Change{Title: &empty}, entryID, now, nil); detail(err) != "whiteboards.title_empty" {
		t.Fatalf("an empty title: %v", err)
	}
}
