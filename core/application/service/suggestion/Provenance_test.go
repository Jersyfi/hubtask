// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"testing"

	agentdomain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type marks struct{ written []agentdomain.Mark }

func (m *marks) InsertMark(_ context.Context, mark agentdomain.Mark) error {
	m.written = append(m.written, mark)
	return nil
}

type markIDs struct{}

func (markIDs) NewID() shared.ID { return shared.MustParseID("0192f000-0000-7000-8000-0000000000aa") }

// UC-AGT-05 check 1: accepting marks the entry with the fields the model proposed, the model and
// prompt and who accepted - never a value; what the person overrode is theirs.
func TestAcceptingMarksTheValuesThatCameFromAI(t *testing.T) {
	cases, world := newWorld()
	written := &marks{}
	cases.Marks, cases.IDs = written, markIDs{}
	stored := proposal()
	stored.Payload = map[string]any{"title": "Buy oat milk", "notes": "two litres"}
	world.store.proposals[proposalID] = stored

	if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), person(), proposalID,
		map[string]any{"notes": "one litre"}); err != nil {
		t.Fatalf("accepting: %v", err)
	}
	if len(written.written) != 1 {
		t.Fatalf("marks = %+v", written.written)
	}
	m := written.written[0]
	if m.TargetID != targetID || len(m.Fields) != 1 || m.Fields[0] != "title" || m.AcceptedBy != person().AccountID ||
		m.SuggestionID != proposalID || m.Model != stored.Model || m.PromptVersion != stored.PromptVersion {
		t.Fatalf("mark = %+v", m)
	}
}

// UC-AGT-05 check 1: a value an agent proposed and a person accepted is marked with that agent;
// one a person asked for names none.
func TestAcceptingNamesTheAgentThatAsked(t *testing.T) {
	triage := shared.MustParseID("0192f000-0000-7000-8000-0000000000ab")
	for _, tc := range []struct {
		name  string
		asked shared.ID
	}{{"an agent asked", triage}, {"a person asked", ""}} {
		t.Run(tc.name, func(t *testing.T) {
			cases, world := newWorld()
			written := &marks{}
			cases.Marks, cases.IDs = written, markIDs{}
			stored := proposal()
			stored.Payload = map[string]any{"title": "Printer down"}
			stored.AgentID = tc.asked
			world.store.proposals[proposalID] = stored

			if _, err := (AcceptSuggestion{Cases: cases}).Execute(context.Background(), person(), proposalID, nil); err != nil {
				t.Fatalf("accepting: %v", err)
			}
			if len(written.written) != 1 || written.written[0].AgentID != tc.asked {
				t.Fatalf("marks = %+v, want agent %q", written.written, tc.asked)
			}
		})
	}
}
