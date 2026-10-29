// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c001")
	anna    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c002")
	bert    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c003")
	hub     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c004")
	private = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c005")
	entry   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c006")
	start   = time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
)

type passthrough struct{}

func (passthrough) Within(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

func (passthrough) WithinReadOnly(ctx context.Context, _ persistence.Scope, fn func(context.Context) error) error {
	return fn(ctx)
}

type fixedClock struct{}

func (fixedClock) Now() time.Time { return start }

type counter struct{ n int }

func (c *counter) NewID() shared.ID {
	c.n++
	return shared.MustParseID(fmt.Sprintf("01936f2a-7c1e-7000-8000-%012x", c.n))
}

type trail struct{ entries []audit.Entry }

func (t *trail) Append(_ context.Context, entry audit.Entry) error {
	t.entries = append(t.entries, entry)
	return nil
}

// gate lets everybody read and draw at the shared hub; at the private hub only Anna, and Bert
// reads the shared hub only when reader is set.
type gate struct{ reader bool }

func (g gate) allows(actor appshared.ActorContext, request access.Request) bool {
	for _, scope := range request.Path {
		if scope.ID == private && actor.AccountID != anna {
			return false
		}
	}
	return !(g.reader && actor.AccountID == bert && request.Permission == service.PermissionWriteItems)
}

func (g gate) Authorize(_ context.Context, actor appshared.ActorContext, request access.Request) error {
	if !g.allows(actor, request) {
		return shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return nil
}

func (g gate) Permits(_ context.Context, actor appshared.ActorContext, request access.Request) (bool, error) {
	return g.allows(actor, request), nil
}

type hubs struct{ workrepo.Containers }

func (hubs) Find(_ context.Context, id shared.ID) (work.Container, error) {
	if id != hub && id != private {
		return work.Container{}, shared.ErrNotFound.WithDetail("containers.not_found")
	}
	return work.Container{ID: id, Type: work.ContainerHub}, nil
}

type store struct {
	boards   map[shared.ID]domain.Board
	versions map[shared.ID][]domain.Version
	pointing []repository.Pointing
}

func (s *store) Insert(_ context.Context, board domain.Board) error {
	s.boards[board.ID] = board
	return nil
}

func (s *store) Find(_ context.Context, id shared.ID) (domain.Board, error) {
	board, ok := s.boards[id]
	if !ok {
		return domain.Board{}, shared.ErrNotFound.WithDetail("whiteboards.not_found")
	}
	return board, nil
}

func (s *store) FindForUpdate(ctx context.Context, id shared.ID) (domain.Board, error) {
	return s.Find(ctx, id)
}

func (s *store) Update(_ context.Context, board domain.Board, expected int) error {
	if s.boards[board.ID].Version != expected {
		return shared.ErrVersionConflict
	}
	board.Version = expected + 1
	s.boards[board.ID] = board
	return nil
}

func (s *store) List(_ context.Context, hubID shared.ID, trashed bool, _ int) ([]repository.Summary, error) {
	var out []repository.Summary
	for _, board := range s.boards {
		if board.HubID == hubID && board.IsTrashed() == trashed {
			out = append(out, repository.Summary{Board: board, Elements: len(board.Scene.Elements)})
		}
	}
	return out, nil
}

func (s *store) Count(ctx context.Context, hubID shared.ID) (int, error) {
	list, err := s.List(ctx, hubID, false, 0)
	return len(list), err
}

func (s *store) Referencing(context.Context, string, shared.ID, int) ([]repository.Pointing, error) {
	return s.pointing, nil
}

func (s *store) Summaries(context.Context, []shared.ID) ([]repository.Summary, error) {
	return nil, nil
}

func (s *store) LastVersion(_ context.Context, id shared.ID) (domain.Version, bool, error) {
	list := s.versions[id]
	if len(list) == 0 {
		return domain.Version{}, false, nil
	}
	return list[len(list)-1], true, nil
}

func (s *store) InsertVersion(_ context.Context, version domain.Version) error {
	s.versions[version.BoardID] = append(s.versions[version.BoardID], version)
	return nil
}

func (s *store) FoldVersion(_ context.Context, version domain.Version) error {
	list := s.versions[version.BoardID]
	list[len(list)-1] = version
	return nil
}

func (s *store) NameVersion(_ context.Context, id shared.ID, seq int, name string) error {
	for i := range s.versions[id] {
		if s.versions[id][i].Seq == seq {
			s.versions[id][i].Name = name
			return nil
		}
	}
	return shared.ErrNotFound.WithDetail("whiteboards.version_not_found")
}

func (s *store) Versions(_ context.Context, id shared.ID, _, _ int) ([]repository.VersionRow, error) {
	var out []repository.VersionRow
	list := s.versions[id]
	for i := len(list) - 1; i >= 0; i-- {
		v := list[i]
		out = append(out, repository.VersionRow{Seq: v.Seq, Title: v.Title, AuthorID: v.AuthorID, Name: v.Name, CreatedAt: v.CreatedAt, Elements: len(v.Scene.Elements)})
	}
	return out, nil
}

func (s *store) Version(_ context.Context, id shared.ID, seq int) (domain.Version, error) {
	for _, v := range s.versions[id] {
		if v.Seq == seq {
			return v, nil
		}
	}
	return domain.Version{}, shared.ErrNotFound.WithDetail("whiteboards.version_not_found")
}

func studio(g gate) (Studio, *store, *trail) {
	boards, sink := &store{boards: map[shared.ID]domain.Board{}, versions: map[shared.ID][]domain.Version{}}, &trail{}
	return Studio{
		Boards: boards, Containers: hubs{}, Authorizer: g, Audit: sink, UnitOfWork: passthrough{},
		Clock: fixedClock{}, IDs: &counter{},
	}, boards, sink
}

func actor(id shared.ID) appshared.ActorContext {
	return appshared.ActorContext{TenantID: tenant, AccountID: id, Kind: appshared.ActorUser, Scopes: []string{boardsRead, boardsWrite}}
}

func detail(err error) string {
	var e *shared.Error
	if errors.As(err, &e) {
		return e.DetailCode
	}
	return ""
}

// The scene as a door sends it: decoded JSON, numbers as float64.
func sent() []any {
	return []any{
		map[string]any{"id": "s1", "kind": "STICKY", "x": 10.0, "y": 20.0, "width": 160.0, "height": 160.0,
			"text": "Book the train", "color_token": "amber"},
		map[string]any{"id": "b1", "kind": "SHAPE", "shape": "DIAMOND", "x": 300.0, "width": 120.0, "height": 120.0},
		map[string]any{"id": "l1", "kind": "CONNECTOR", "from": "s1", "to": "b1", "route": "ELBOW"},
		map[string]any{"id": "f1", "kind": "FRAME", "x": 0.0, "y": 0.0, "width": 1000.0, "height": 600.0, "text": "Ideas"},
		map[string]any{"id": "c1", "kind": "CARD", "x": 2000.0, "width": 240.0, "height": 120.0,
			"target": map[string]any{"kind": "ENTRY", "id": entry.String()}},
	}
}

// UC-CNV-01 check 1: a member makes a board in a hub and draws sticky notes, shapes, a connector
// attached to both ends and a frame on it; reading it back answers them as drawn, in order.
func TestABoardIsDrawnAndReadBack(t *testing.T) {
	s, _, sink := studio(gate{})
	create := CreateWhiteboard{Studio: s}.Descriptor().Handler
	out, err := create.Invoke(context.Background(), actor(anna), usecase.Input{"hub_id": hub.String(), "title": "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(string)
	update := UpdateWhiteboard{Studio: s}.Descriptor().Handler
	if _, err := update.Invoke(context.Background(), actor(bert), usecase.Input{"whiteboard_id": id, "elements": sent()}); err != nil {
		t.Fatal(err)
	}
	read, err := ReadWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": id})
	if err != nil {
		t.Fatal(err)
	}
	elements := read["elements"].([]usecase.Output)
	if len(elements) != 5 || elements[0]["color_token"] != "amber" || elements[1]["shape"] != "DIAMOND" ||
		elements[2]["from"] != "s1" || elements[2]["route"] != "ELBOW" || elements[2]["end"] != "ARROW" ||
		elements[4]["target"].(usecase.Output)["id"] != entry.String() {
		t.Fatalf("read back %+v", elements)
	}
	if read["writable"] != true || read["version"] != 2 {
		t.Fatalf("writable %v, version %v", read["writable"], read["version"])
	}
	if len(sink.entries) != 2 || sink.entries[1].Action != BoardChangedAction {
		t.Fatalf("trail %+v", sink.entries)
	}
	if title, ok := sink.entries[0].Changes[domain.FieldTitle].(map[string]any); !ok || title["to"] != nil {
		t.Fatalf("the title reached the trail as open text (rule 10): %+v", sink.entries[0].Changes)
	}
}

// UC-CNV-01 check 3: a reader of the hub who may not write it sees the board and may not draw on
// it; a member of another hub does not find a private hub's board.
func TestABoardKeepsItsHubsMemberships(t *testing.T) {
	s, _, _ := studio(gate{reader: true})
	board, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Plan"})
	if err != nil {
		t.Fatal(err)
	}
	_, writable, err := ReadWhiteboard{Studio: s}.Execute(context.Background(), actor(bert), board.ID)
	if err != nil || writable {
		t.Fatalf("a reader: writable %v, err %v", writable, err)
	}
	title := "Mine"
	if _, err := (UpdateWhiteboard{Studio: s}).Execute(context.Background(), actor(bert), UpdateWhiteboardCommand{ID: board.ID, Change: domain.Change{Title: &title}}); err == nil {
		t.Fatal("a reader drew on the board")
	}
	secret, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: private, Title: "Secret"})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := (ReadWhiteboard{Studio: s}).Execute(context.Background(), actor(bert), secret.ID); err == nil {
		t.Fatal("a private hub's board was read by another person")
	}
	if _, err := (Locator{Studio: s}).Locate(context.Background(), actor(anna), secret.ID); err != nil {
		t.Fatalf("the live document is not located: %v", err)
	}
}

// A scene that is not well-formed is refused whole, by its code, and nothing is written.
func TestAMalformedSceneIsRefused(t *testing.T) {
	s, boards, _ := studio(gate{})
	board, _ := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Plan"})
	for _, tc := range []struct {
		name     string
		elements any
		want     string
	}{
		{"not a list", "x", "whiteboards.elements_malformed"},
		{"a position that is no number", []any{map[string]any{"id": "a", "kind": "TEXT", "x": "left"}}, "whiteboards.elements_malformed"},
		{"a colour value", []any{map[string]any{"id": "a", "kind": "STICKY", "color_token": "#ffcc00"}}, "whiteboards.element_colour_unknown"},
		{"a card of nothing", []any{map[string]any{"id": "a", "kind": "CARD", "target": map[string]any{"kind": "ENTRY", "id": "x"}}}, "whiteboards.card_target_malformed"},
	} {
		_, err := UpdateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna),
			usecase.Input{"whiteboard_id": board.ID.String(), "elements": tc.elements})
		if detail(err) != tc.want {
			t.Errorf("%s: %v, want %s", tc.name, err, tc.want)
		}
	}
	if boards.boards[board.ID].Version != 1 {
		t.Fatal("a refused scene was written")
	}
}

// UC-CNV-03 checks 1 and 2: a board, or one frame of it, is exported as the native document and
// read back into a new board with the same objects.
func TestABoardIsExportedAndReadBack(t *testing.T) {
	s, _, sink := studio(gate{})
	scene, err := s.sceneOf(usecase.Input{"elements": sent()})
	if err != nil {
		t.Fatal(err)
	}
	board, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Retro", Scene: scene})
	if err != nil {
		t.Fatal(err)
	}
	export := ExportWhiteboard{Studio: s}.Descriptor().Handler
	whole, err := export.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": board.ID.String()})
	if err != nil || whole["format"] != ExportFormat || whole["version"] != ExportVersion {
		t.Fatalf("export %+v, %v", whole, err)
	}
	if sink.entries[len(sink.entries)-1].Action != BoardExportedAction {
		t.Fatal("the export is not in the trail")
	}
	framed, err := export.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": board.ID.String(), "frame": "f1"})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(framed["elements"].([]usecase.Output)); n != 4 {
		t.Fatalf("the frame exported %d objects, want the frame, the sticky note, the shape and their connector", n)
	}
	if _, err := export.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": board.ID.String(), "frame": "s1"}); detail(err) != "whiteboards.frame_not_found" {
		t.Fatalf("a sticky note exported as a frame: %v", err)
	}
	// Read back: the exported elements, as JSON carries them, make a new board of the same objects.
	elements := []any{}
	for _, element := range whole["elements"].([]usecase.Output) {
		copied := map[string]any{}
		for key, value := range element {
			if target, ok := value.(usecase.Output); ok {
				value = map[string]any(target)
			}
			copied[key] = value
		}
		elements = append(elements, copied)
	}
	again, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna),
		usecase.Input{"hub_id": hub.String(), "title": whole["title"], "elements": elements})
	if err != nil {
		t.Fatal(err)
	}
	copyBoard, _ := s.Boards.Find(context.Background(), shared.ID(again["id"].(string)))
	if !copyBoard.Scene.Equal(board.Scene) {
		t.Fatalf("read back %+v, want %+v", copyBoard.Scene, board.Scene)
	}
	if refs := copyBoard.References(); len(refs) != 1 || refs[0].Reference.Kind != reference.KindEntry {
		t.Fatalf("the copy's card points at %+v", refs)
	}
}

// A trashed board is not found and not listed; restored, it is back with its objects.
func TestABoardGoesToTheTrashAndComesBack(t *testing.T) {
	s, _, _ := studio(gate{})
	scene, _ := s.sceneOf(usecase.Input{"elements": sent()})
	board, _ := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Retro", Scene: scene})
	if _, err := (TrashWhiteboard{Studio: s}).Execute(context.Background(), actor(anna), board.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := (ReadWhiteboard{Studio: s}).Execute(context.Background(), actor(anna), board.ID); detail(err) != "whiteboards.not_found" {
		t.Fatalf("a trashed board was read: %v", err)
	}
	if list, _ := (ListWhiteboards{Studio: s}).Execute(context.Background(), actor(anna), hub, false); len(list) != 0 {
		t.Fatal("a trashed board is listed")
	}
	if trashed, _ := (ListWhiteboards{Studio: s}).Execute(context.Background(), actor(anna), hub, true); len(trashed) != 1 || trashed[0]["element_count"] != 5 {
		t.Fatalf("the trash: %+v", trashed)
	}
	restored, err := RestoreWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), board.ID)
	if err != nil || restored.IsTrashed() || len(restored.Scene.Elements) != 5 {
		t.Fatalf("restored %+v, %v", restored, err)
	}
}

// UC-CNV-01 check 8: a freehand stroke, a web link and a group are read back as drawn.
func TestAStrokeALinkAndAGroupAreReadBack(t *testing.T) {
	s, _, _ := studio(gate{})
	out, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{
		"hub_id": hub.String(), "title": "Sketch", "elements": []any{
			map[string]any{"id": "p1", "kind": "STROKE", "pen": "HIGHLIGHTER", "width": 40.0, "height": 20.0,
				"points": []any{0.0, 0.0, 20.0, 10.0, 40.0, 20.0}, "group": "g1"},
			map[string]any{"id": "w1", "kind": "LINK", "url": "https://example.org/plan", "width": 240.0, "height": 80.0, "group": "g1"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": out["id"]})
	if err != nil {
		t.Fatal(err)
	}
	elements := read["elements"].([]usecase.Output)
	points, _ := elements[0]["points"].([]float64)
	if elements[0]["pen"] != "HIGHLIGHTER" || len(points) != 6 || points[5] != 20 || elements[0]["group"] != "g1" ||
		elements[1]["url"] != "https://example.org/plan" || elements[1]["group"] != "g1" {
		t.Fatalf("read back %+v", elements)
	}
	_, err = UpdateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{
		"whiteboard_id": out["id"], "elements": []any{map[string]any{"id": "p1", "kind": "STROKE", "points": []any{1.0}}},
	})
	if detail(err) != "whiteboards.elements_malformed" {
		t.Fatalf("an odd list of numbers: %v", err)
	}
}

type movingClock struct{ at *time.Time }

func (c movingClock) Now() time.Time { return *c.at }

// UC-CNV-06 checks 1 and 2: the saves of one drawing session are one version, a rest starts the
// next; a version is read back with its objects and named, and a reader may not name one.
func TestABoardKeepsItsHistory(t *testing.T) {
	s, boards, _ := studio(gate{})
	at := start
	s.Clock = movingClock{at: &at}
	out, err := CreateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"hub_id": hub.String(), "title": "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	id := out["id"].(string)
	update := UpdateWhiteboard{Studio: s}.Descriptor().Handler
	save := func(who shared.ID, elements []any) {
		t.Helper()
		if _, err := update.Invoke(context.Background(), actor(who), usecase.Input{"whiteboard_id": id, "elements": elements}); err != nil {
			t.Fatal(err)
		}
	}
	save(anna, sent()[:1])
	at = at.Add(2 * time.Minute)
	save(bert, sent()[:2])
	at = at.Add(domain.SessionGap)
	save(anna, sent())
	list, err := ListWhiteboardVersions{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": id})
	if err != nil {
		t.Fatal(err)
	}
	rows := list["data"].([]usecase.Output)
	if len(rows) != 2 || rows[0]["seq"] != 2 || rows[0]["element_count"] != 5 || rows[1]["element_count"] != 2 || rows[1]["author_id"] != bert.String() {
		t.Fatalf("history %+v", rows)
	}
	read, err := ReadWhiteboardVersion{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": id, "seq": 1})
	if err != nil || len(read["elements"].([]usecase.Output)) != 2 {
		t.Fatalf("version 1: %+v, %v", read, err)
	}
	named, err := NameWhiteboardVersion{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": id, "seq": 2, "name": " Before the workshop "})
	if err != nil || named["name"] != "Before the workshop" {
		t.Fatalf("named %+v, %v", named, err)
	}
	at = at.Add(time.Minute)
	save(anna, sent()[:3])
	if got := len(boards.versions[shared.MustParseID(id)]); got != 3 {
		t.Fatalf("a save after a named version folded into it: %d versions", got)
	}
	readOnly := s
	readOnly.Authorizer = gate{reader: true}
	if _, err := (NameWhiteboardVersion{Studio: readOnly}).Descriptor().Handler.Invoke(context.Background(), actor(bert), usecase.Input{"whiteboard_id": id, "seq": 1, "name": "x"}); err == nil {
		t.Fatal("a reader named a version")
	}
}
