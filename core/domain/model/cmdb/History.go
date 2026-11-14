// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Verb is what one step of an object's history records; a client renders `activity.ci_<verb>`.
type Verb string

// The verbs of an object's history (UC-AST-01). Stored, so never renamed.
const (
	VerbCreated      Verb = "ci.created"
	VerbChanged      Verb = "ci.changed"
	VerbStageChanged Verb = "ci.stage_changed"
	VerbRelated      Verb = "ci.related"
	VerbUnrelated    Verb = "ci.unrelated"
	VerbConfirmed    Verb = "ci.confirmed"
	VerbAttested     Verb = "ci.attested"
	VerbMerged       Verb = "ci.merged"
	VerbAssetMoved   Verb = "ci.asset_moved"
	VerbFileAttached Verb = "ci.file_attached"
	VerbFileRemoved  Verb = "ci.file_removed"
)

var verbs = []Verb{
	VerbCreated, VerbChanged, VerbStageChanged, VerbRelated, VerbUnrelated, VerbConfirmed, VerbAttested,
	VerbMerged, VerbAssetMoved, VerbFileAttached, VerbFileRemoved,
}

// Code is the message code a client renders for the verb.
func (v Verb) Code() string { return "activity." + strings.Replace(string(v), ".", "_", 1) }

// Moved is what the history keeps of one field: its values where they are the object's own
// vocabulary or an identifier, and only that it changed where the value is an attribute.
type Moved struct {
	From, To string
	Changed  bool
}

// Step is one thing done to an object, who did it and when.
type Step struct {
	ID        shared.ID
	CIID      shared.ID
	ActorKind shared.ActorKind
	ActorID   shared.ID
	Verb      Verb
	Changes   map[string]Moved
	At        time.Time
}

// NewStep checks a step before it is written: a known verb, an object, an actor kind of the
// vocabulary and a moment.
func NewStep(id, ci shared.ID, kind shared.ActorKind, actor shared.ID, verb Verb, changes map[string]Moved, at time.Time) (Step, error) {
	if id.IsZero() || ci.IsZero() || at.IsZero() || !slices.Contains(verbs, verb) ||
		!slices.Contains(shared.ActorKinds(), kind) || kind == shared.ActorAnonymous {
		return Step{}, shared.ErrInternal.WithDetail("cmdb.history_step_invalid")
	}
	if changes == nil {
		changes = map[string]Moved{}
	}
	return Step{ID: id, CIID: ci, ActorKind: kind, ActorID: actor, Verb: verb, Changes: changes, At: at}, nil
}

// StageStep is the step of a stage or status moved; none where neither moved.
func StageStep(before, after CI) (map[string]Moved, bool) {
	changes := map[string]Moved{}
	if before.Stage != after.Stage {
		changes["stage"] = Moved{From: string(before.Stage), To: string(after.Stage)}
	}
	if before.Status != after.Status {
		changes["status"] = Moved{From: string(before.Status), To: string(after.Status)}
	}
	return changes, len(changes) > 0
}

// ComplianceStep is the step of an owner, a location, a protection need, a review day or a
// quantity changed: people and places by identifier, the rest by what they became.
func ComplianceStep(before, after CI) (map[string]Moved, bool) {
	changes := map[string]Moved{}
	ids := func(field string, from, to shared.ID) {
		if from != to {
			changes[field] = Moved{From: idText(from), To: idText(to)}
		}
	}
	b, a := before.Compliance, after.Compliance
	ids("owner_account_id", b.OwnerAccount, a.OwnerAccount)
	ids("owner_group_id", b.OwnerGroup, a.OwnerGroup)
	ids("location_id", b.Location, a.Location)
	if b.Protection.Protection != a.Protection.Protection || !slices.Equal(b.Protection.Fixed, a.Protection.Fixed) ||
		b.Protection.Reason != a.Protection.Reason {
		changes["protection"] = Moved{Changed: true}
	}
	if dayText(b.ReviewDue) != dayText(a.ReviewDue) {
		changes["review_due"] = Moved{From: dayText(b.ReviewDue), To: dayText(a.ReviewDue)}
	}
	if b.Quantity != a.Quantity {
		changes["quantity"] = Moved{Changed: true}
	}
	return changes, len(changes) > 0
}

func idText(id shared.ID) string {
	if id.IsZero() {
		return ""
	}
	return id.String()
}

func dayText(day *time.Time) string {
	if day == nil {
		return ""
	}
	return day.Format(time.DateOnly)
}
