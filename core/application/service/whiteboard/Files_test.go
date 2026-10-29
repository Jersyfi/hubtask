// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package whiteboard

import (
	"context"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	annasPicture = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c201")
	privatePic   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c202")
	pending      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c203")
)

// uploads holds Anna's picture, one on a board of the private hub, and an upload not confirmed.
type uploads struct{}

func (uploads) Find(_ context.Context, id shared.ID) (media.Object, error) {
	switch id {
	case annasPicture:
		return media.Object{ID: id, Status: media.StatusReady, CreatedBy: anna}, nil
	case privatePic:
		return media.Object{ID: id, Status: media.StatusReady, CreatedBy: anna}, nil
	case pending:
		return media.Object{ID: id, Status: media.StatusPending, CreatedBy: bert}, nil
	}
	return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
}

func (uploads) ReferencingBoards(_ context.Context, id shared.ID) ([]shared.ID, error) {
	switch id {
	case annasPicture:
		return []shared.ID{hub}, nil
	case privatePic:
		return []shared.ID{private}, nil
	}
	return nil, nil
}

// UC-CNV-08 check 1: a writer places their own upload on a board, and a picture object draws it.
// UC-CNV-08 check 2: a picture another board they may read holds is placed too (copied between
// boards); one only a board out of reach holds, an unconfirmed upload or none at all is not
// found, and a reader who may not write is refused.
func TestAPictureIsPlacedOnABoard(t *testing.T) {
	s, boards, sink := studio(gate{reader: true})
	s.Media, s.Reader = uploads{}, reader{gate: gate{reader: true}}
	board, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Moodboard"})
	if err != nil {
		t.Fatal(err)
	}
	place := AttachWhiteboardMedia{Studio: s}.Descriptor().Handler
	if _, err := place.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": board.ID.String(), "media_id": annasPicture.String()}); err != nil {
		t.Fatal(err)
	}
	if got := boards.media[board.ID]; len(got) != 1 || got[0] != annasPicture {
		t.Fatalf("held %v", got)
	}
	if last := sink.entries[len(sink.entries)-1]; last.Action != BoardMediaPlacedAction {
		t.Fatalf("recorded %v", last.Action)
	}
	_, err = UpdateWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{
		"whiteboard_id": board.ID.String(),
		"elements": []any{map[string]any{"id": "i1", "kind": "IMAGE", "width": 320.0, "height": 200.0, "media_id": annasPicture.String()}},
	})
	if err != nil {
		t.Fatal(err)
	}
	read, err := ReadWhiteboard{Studio: s}.Descriptor().Handler.Invoke(context.Background(), actor(anna), usecase.Input{"whiteboard_id": board.ID.String()})
	if err != nil || read["elements"].([]usecase.Output)[0]["media_id"] != annasPicture.String() {
		t.Fatalf("read %v, %v", read, err)
	}

	other, err := CreateWhiteboard{Studio: s}.Execute(context.Background(), actor(anna), CreateWhiteboardCommand{HubID: hub, Title: "Copy"})
	if err != nil {
		t.Fatal(err)
	}
	if err := (AttachWhiteboardMedia{Studio: s}).Execute(context.Background(), actor(anna), other.ID, annasPicture); err != nil {
		t.Fatalf("the uploader copied: %v", err)
	}
	for _, c := range []struct {
		name   string
		object shared.ID
		want   string
	}{
		{"a picture only a private board holds", privatePic, "whiteboards.media_not_found"},
		{"an upload not confirmed", pending, "whiteboards.media_not_found"},
		{"no upload at all", shared.MustParseID("01936f2a-7c1e-7000-8000-00000000c299"), "whiteboards.media_not_found"},
	} {
		s2 := s
		s2.Authorizer = gate{}
		if err := (AttachWhiteboardMedia{Studio: s2}).Execute(context.Background(), actor(bert), other.ID, c.object); detail(err) != c.want {
			t.Errorf("%s: got %v", c.name, err)
		}
	}
	s3 := s
	s3.Authorizer = gate{}
	if err := (AttachWhiteboardMedia{Studio: s3}).Execute(context.Background(), actor(bert), other.ID, annasPicture); err != nil {
		t.Errorf("a picture a readable board holds: %v", err)
	}
	if err := (AttachWhiteboardMedia{Studio: s}).Execute(context.Background(), actor(bert), other.ID, annasPicture); detail(err) != "containers.not_found" {
		t.Errorf("a reader who may not write placed a file: %v", err)
	}
}
