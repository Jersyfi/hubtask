// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MaxAffectedPerEntry bounds the CIs one entry names as affected: an incident or a change touches a
// handful, and the impact over them is walked on every read.
const MaxAffectedPerEntry = 50

// Affected is a configuration item an entry - an incident, a change, a problem - names as the one
// it is about (research itsm-cmdb-ai I2). The CI and the entry may sit in different hubs; each is
// judged at its own.
type Affected struct {
	ID       shared.ID
	TenantID shared.ID
	ItemID   shared.ID
	CIID     shared.ID
	AddedBy  shared.ID
	AddedAt  time.Time
}

// Window is the time an entry plans to touch its CIs: its start and due. An entry without both has
// no window and collides with nothing.
type Window struct {
	Start *time.Time
	End   *time.Time
}

// Open says whether the window has both ends, in order.
func (w Window) Open() bool {
	return w.Start != nil && w.End != nil && !w.End.Before(*w.Start)
}

// Overlaps says whether two windows share a moment; an end is inclusive, as a due day is.
func (w Window) Overlaps(o Window) bool {
	if !w.Open() || !o.Open() {
		return false
	}
	return !w.End.Before(*o.Start) && !o.End.Before(*w.Start)
}

// Neighbour is another open entry naming a CI as affected.
type Neighbour struct {
	ItemID shared.ID
	CIID   shared.ID
	Window Window
}

// Collision is another entry planned on the same CI, or on one the entry's CIs' failure reaches,
// in an overlapping window (research I2, change collision).
type Collision struct {
	ItemID shared.ID
	CIID   shared.ID
	// Direct is true when both entries name the CI itself; false when the other's CI is one the
	// entry's CIs reach.
	Direct bool
}

// Collisions answers the neighbours planned in a window that overlaps the entry's, on a CI it
// names or reaches; each other entry once, by its first CI in the order given, a direct one first.
func Collisions(self shared.ID, window Window, own map[shared.ID]bool, reached map[shared.ID]bool, neighbours []Neighbour) []Collision {
	if !window.Open() {
		return nil
	}
	seen := map[shared.ID]int{}
	var out []Collision
	for _, n := range neighbours {
		if n.ItemID == self || !window.Overlaps(n.Window) {
			continue
		}
		direct := own[n.CIID]
		if !direct && !reached[n.CIID] {
			continue
		}
		if at, ok := seen[n.ItemID]; ok {
			if direct && !out[at].Direct {
				out[at] = Collision{ItemID: n.ItemID, CIID: n.CIID, Direct: true}
			}
			continue
		}
		seen[n.ItemID] = len(out)
		out = append(out, Collision{ItemID: n.ItemID, CIID: n.CIID, Direct: direct})
	}
	return out
}
