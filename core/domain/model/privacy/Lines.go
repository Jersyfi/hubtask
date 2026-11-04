// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

// What an erasure does, kind by kind, in the words its confirmation says (user-experience.md
// §5.9): one line per kind the eraser handles, in a fixed order, so the person who starts it
// reads what goes, what stays without the person's name and what a hold keeps before it happens.

// LineKind is one kind of data the eraser handles.
type LineKind string

const (
	LineAccount            LineKind = "ACCOUNT"
	LineCredentials        LineKind = "CREDENTIALS"
	LinePersonalRules      LineKind = "PERSONAL_RULES"
	LineNotifications      LineKind = "NOTIFICATIONS"
	LineAiDrafts           LineKind = "AI_DRAFTS"
	LineKudos              LineKind = "KUDOS"
	LineArrangements       LineKind = "ARRANGEMENTS"
	LineMedia              LineKind = "MEDIA"
	LineTime               LineKind = "TIME"
	LineAssignments        LineKind = "ASSIGNMENTS"
	LineIntake             LineKind = "INTAKE"
	LineComments           LineKind = "COMMENTS"
	LinePageComments       LineKind = "PAGE_COMMENTS"
	LineWhiteboardComments LineKind = "WHITEBOARD_COMMENTS"
	LineGoalCheckIns       LineKind = "GOAL_CHECK_INS"
	LinePortfolioUpdates   LineKind = "PORTFOLIO_UPDATES"
	LinePageDrafts         LineKind = "PAGE_DRAFTS"
	LineEntries            LineKind = "ENTRIES"
)

// LineFate is what the erasure does to a kind.
type LineFate string

const (
	FateDeleted LineFate = "DELETED"
	// FateAnonymised stays without the person's name: as the former user's, or for the sums.
	FateAnonymised LineFate = "ANONYMISED"
	// FateKept stays as it is, because a hold reaches it.
	FateKept LineFate = "KEPT"
	// FateReleased is the person taken off it.
	FateReleased LineFate = "RELEASED"
)

// Line is one kind's fate, and how many where the preview reads the rows; nil where only the
// erasure itself counts them.
type Line struct {
	Kind  LineKind
	Fate  LineFate
	Count *int
}

// writingLines are the writings outside entries in the order their lines stand.
var writingLines = []struct {
	row  RowKind
	line LineKind
}{
	{RowPageComment, LinePageComments},
	{RowBoardComment, LineWhiteboardComments},
	{RowCheckIn, LineGoalCheckIns},
	{RowPortfolioUpdate, LinePortfolioUpdates},
	{RowPageDraft, LinePageDrafts},
}

// rowTally is how the rows of one kind end: by the mode's own fate, or kept by a hold.
type rowTally struct{ fate, kept int }

func lines(plan Plan, in PlanInput, tally map[RowKind]*rowTally) []Line {
	n := func(v int) *int { return &v }
	counted := func(kind LineKind, row RowKind, fate LineFate) []Line {
		t := tally[row]
		if t == nil {
			t = &rowTally{}
		}
		out := []Line{{Kind: kind, Fate: fate, Count: n(t.fate)}}
		if t.kept > 0 {
			out = append(out, Line{Kind: kind, Fate: FateKept, Count: n(t.kept)})
		}
		return out
	}
	// What a person wrote goes in a full deletion and stays as the former user's otherwise.
	written := FateAnonymised
	if in.Mode == ModeFullDelete {
		written = FateDeleted
	}

	out := []Line{{Kind: LineAccount, Fate: LineFate(plan.Account)}}
	// No hold reaches these; they go whatever is kept (data-protection.md §4.1).
	for _, kind := range []LineKind{
		LineCredentials, LinePersonalRules, LineNotifications, LineAiDrafts, LineKudos, LineArrangements, LineMedia,
	} {
		out = append(out, Line{Kind: kind, Fate: FateDeleted})
	}
	// Time stays for the sums in both modes; only its words go.
	out = append(out, Line{Kind: LineTime, Fate: FateAnonymised})
	out = append(out, counted(LineAssignments, RowAssignment, FateReleased)...)
	switch {
	case plan.KeepIntake:
		out = append(out, Line{Kind: LineIntake, Fate: FateKept, Count: n(in.Intake)})
	default:
		out = append(out, Line{Kind: LineIntake, Fate: written})
	}
	out = append(out, counted(LineComments, RowComment, written)...)
	for _, w := range writingLines {
		out = append(out, counted(w.line, w.row, written)...)
	}
	return append(out, counted(LineEntries, RowEntry, FateAnonymised)...)
}
