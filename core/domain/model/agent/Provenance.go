// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"slices"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Mark is the machine-readable provenance of a value that came from AI and a person accepted
// (ai-first.md §2.1, AI-2): which object and which of its fields, the suggestion, the model and
// prompt that produced it, the agent where one proposed it, and who accepted it when. It outlives
// the suggestion record and travels with the archive; it carries the names of the fields, never
// their values.
type Mark struct {
	ID            shared.ID
	TargetType    string
	TargetID      shared.ID
	Fields        []string
	SuggestionID  shared.ID
	AgentID       shared.ID
	Model         string
	PromptID      string
	PromptVersion string
	AcceptedBy    shared.ID
	AcceptedAt    time.Time
}

// NewMark checks a mark: an object, the person who accepted it and the model; the fields sorted.
func NewMark(m Mark) (Mark, error) {
	if m.TargetType == "" || m.TargetID.IsZero() || m.AcceptedBy.IsZero() || m.Model == "" {
		return Mark{}, shared.ErrInternal.WithDetail("provenance.incomplete")
	}
	fields := slices.Clone(m.Fields)
	slices.Sort(fields)
	m.Fields = slices.Compact(fields)
	return m, nil
}
