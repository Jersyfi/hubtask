// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// TrashKind says which of the two aggregates a trash entry describes.
//
// The trash is the one view in this system that mixes them. Everywhere else a hub and a task are
// answered by different endpoints because a caller knows which it asked for; here a person asks
// "what did I delete" and the answer is whatever they deleted, in the order they deleted it.
type TrashKind string

const (
	TrashContainerKind TrashKind = "CONTAINER"
	TrashItemKind      TrashKind = "ITEM"
)

// TrashEntry is one deletion, as the trash lists it.
//
// One entry per deletion and not per deleted row. A hub with two hundred entries under it went into
// the trash as one act and comes back as one act (I-C2), so a list of its subtree would be two
// hundred lines describing one decision - and a person looking for what they deleted would have to
// find it among the things that merely came along.
//
// It is a projection rather than an aggregate: it carries what the trash view shows and what
// restoring one needs, and nothing else. Reading the entry back as a Container or a WorkItem is a
// second read, which is what Restore does - the entry says what is there, and the aggregate is what
// the rules are applied to.
type TrashEntry struct {
	Kind TrashKind
	ID   shared.ID
	// BatchID names every row this deletion took, the entry's own included. It is what a restore
	// is keyed on, so it travels to the client: restoring is an act on the deletion, not on the row.
	BatchID shared.ID
	// DeletedAt is when it went in, and the anchor the retention period runs from
	// (data-retention.md §3).
	DeletedAt time.Time
	// Title is the container's name or the item's title. User content, so it goes to the client and
	// never into a log, a metric or an audit entry (rule 10).
	Title string
	// Subtype is the kind within the kind: HUB or COLLECTION for a container, TASK, WORK_PACKAGE or
	// ACTIVITY for an item. A client draws a different icon for each, and a string rather than
	// either enumeration keeps this projection from having to be two types.
	Subtype string
	// HubID is the level the permission question is asked at. A membership held at a hub applies
	// downwards, so an entry that named only its collection could not be shown to somebody whose
	// right sits on the hub above it - and the trash is the one view that spans hubs, so there is no
	// path parameter to read it from. Empty for a hub, which is its own level.
	HubID shared.ID
	// CollectionID and ParentID say where the entry would go back to, which is what lets a client
	// show "in Shopping" beside a deleted entry rather than only its title. Empty where they do not
	// apply - a hub sits in nothing, and an item is under a collection rather than a container.
	CollectionID shared.ID
	ParentID     shared.ID
	// Version is the row's, so that a restore can be sent with the version it was read at.
	Version int
}
