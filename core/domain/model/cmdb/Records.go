// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Counts are a run's figures by outcome.
type Counts struct {
	Items            int
	Created          int
	Updated          int
	Unchanged        int
	Held             int
	Duplicates       int
	Refused          int
	RelationsAdded   int
	RelationsSeen    int
	RelationsRefused int
}

// Count adds a decision to the figures.
func (c *Counts) Count(d Decision) {
	c.Items++
	switch d.Outcome {
	case OutcomeCreate:
		c.Created++
	case OutcomeUpdate:
		c.Updated++
	case OutcomeUnchanged:
		c.Unchanged++
	case OutcomeHold:
		c.Held++
	case OutcomeDuplicate:
		c.Duplicates++
	case OutcomeRefused:
		c.Refused++
	}
}

// Run is one reconciliation of a source's payload in a hub, with its explanation (R8). A dry run
// is answered and never stored.
type Run struct {
	ID        shared.ID
	HubID     shared.ID
	Source    string
	ActorID   shared.ID
	StartedAt time.Time
	Counts    Counts
	Decisions []Decision
}

// Hold is an item kept for review instead of becoming a CI (R6), with the reason's code.
type Hold struct {
	ID         shared.ID
	HubID      shared.ID
	Run        shared.ID
	Source     string
	Item       Item
	Reason     string
	CreatedAt  time.Time
	ResolvedAt *time.Time
	ResolvedBy shared.ID
}

// Duplicate is a set of CIs one item matched, open until somebody merges them or says they are
// distinct (R7).
type Duplicate struct {
	ID         shared.ID
	HubID      shared.ID
	CIs        []shared.ID
	Rule       string
	CreatedAt  time.Time
	ResolvedAt *time.Time
	ResolvedBy shared.ID
	Survivor   shared.ID
}

// The reasons an item is held.
const (
	HoldInsufficient   = "cmdb.hold_insufficient_identifiers"
	HoldMayNotCreate   = "cmdb.hold_source_may_not_create"
	HoldClassForbidden = "cmdb.hold_class_not_written_by_source"
)
