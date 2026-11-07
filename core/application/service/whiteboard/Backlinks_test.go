// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"testing"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	openPage    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c101")
	privatePage = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c102")
	hiddenEntry = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c103")
	comment     = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c104")
	otherBoard  = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c105")
	secretBoard = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c106")
)

// texts points two pages, an entry's notes and a comment at a board: one page and the comment's
// entry in the private hub.
type texts struct{}

func (texts) Backlinks(context.Context, string, shared.ID, int) ([]knowledgerepo.Backlink, error) {
	return []knowledgerepo.Backlink{
		{SourceKind: "PAGE", SourceID: openPage, HubID: hub, Strength: "EMBED"},
		{SourceKind: "PAGE", SourceID: privatePage, HubID: private, Strength: "MENTION"},
		{SourceKind: "ENTRY", SourceID: entry, EntryID: entry, Strength: "MENTION"},
		{SourceKind: "COMMENT", SourceID: comment, EntryID: hiddenEntry, Strength: "MENTION"},
	}, nil
}

func (texts) Summaries(context.Context, []shared.ID) ([]knowledgerepo.Summary, error) {
	return []knowledgerepo.Summary{
		{ID: openPage, HubID: hub, Title: "Planning notes"},
		{ID: privatePage, HubID: private, Title: "Salaries"},
	}, nil
}

type entries struct{}

func (entries) Entries(context.Context, []shared.ID) ([]workrepo.EntrySummary, error) {
	return []workrepo.EntrySummary{
		{ID: entry, HubID: hub, Title: "Book the train"},
		{ID: hiddenEntry, HubID: private, Title: "Hire"},
	}, nil
}

// reader asks the test gate once per path.
type reader struct{ gate gate }

func (r reader) Permitted(_ context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		request.Path = path
		out[i] = r.gate.allows(actor, request)
	}
	return out, nil
}

// UC-CNV-07 check 1: a board lists the pages, entries' notes and other boards that embed or
// mention it, each with its title.
// UC-CNV-07 check 2: what sits in a hub the reader may not read is left out - a private page, a
// comment on a private entry, a board in a private hub - and the list says nothing of it.
func TestABoardListsWhatPointsAtIt(t *testing.T) {
	s, boards, _ := studio(gate{})
	s.Texts, s.Entries, s.Reader = texts{}, entries{}, reader{}
	created, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Retro"})
	if err != nil {
		t.Fatal(err)
	}
	boards.pointing = []repository.Pointing{
		{ID: otherBoard, HubID: hub, Title: "Roadmap", Strength: "EMBED"},
		{ID: secretBoard, HubID: private, Title: "Reorg", Strength: "CARD"},
		{ID: created.ID, HubID: hub, Title: "Retro", Strength: "CARD"},
	}
	handler := ListWhiteboardBacklinks{Studio: s}.Descriptor().Handler
	out, err := handler.Invoke(context.Background(), actor(bert), usecase.Input{"whiteboard_id": created.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	data := out["data"].([]usecase.Output)
	want := []struct{ kind, title, strength string }{
		{"PAGE", "Planning notes", "EMBED"}, {"ENTRY", "Book the train", "MENTION"}, {"WHITEBOARD", "Roadmap", "EMBED"},
	}
	if len(data) != len(want) {
		t.Fatalf("backlinks %+v", data)
	}
	for i, w := range want {
		if data[i]["source_kind"] != w.kind || data[i]["title"] != w.title || data[i]["strength"] != w.strength {
			t.Fatalf("backlink %d is %+v, want %+v", i, data[i], w)
		}
	}
	if data[0]["page_id"] != openPage.String() || data[1]["entry_id"] != entry.String() || data[2]["whiteboard_id"] != otherBoard.String() {
		t.Fatalf("identifiers %+v", data)
	}
	all, err := handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": created.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if n := len(all["data"].([]usecase.Output)); n != 6 {
		t.Fatalf("the private hub's owner sees %d backlinks, want 6", n)
	}
}

// UC-CNV-07 check 3: a reader who may not read the board's hub learns nothing, not even that it
// is pointed at.
func TestABoardsBacklinksAreClosedToWhoCannotReadIt(t *testing.T) {
	s, _, _ := studio(gate{})
	s.Texts, s.Entries, s.Reader = texts{}, entries{}, reader{}
	created, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: private, Title: "Reorg"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = ListWhiteboardBacklinks{Studio: s}.Execute(context.Background(), actor(bert), created.ID)
	if detail(err) != "containers.not_found" {
		t.Fatalf("got %v", err)
	}
}
