// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// DeletionReason is why something was removed for good.
//
// The four the deletion journal allows. It is one vocabulary for the journal, the event and the
// metric rather than three that happen to agree today: an operator reading a cleanup log and a
// subscriber reacting to a purge are asking the same question, and two enumerations would
// eventually answer it with two different words.
type DeletionReason string

const (
	// DeletedByUser is somebody emptying their trash or purging one entry: an explicit act.
	DeletedByUser DeletionReason = "USER"
	// DeletedByRetention is the retention job doing what the tenant's policy says (ADR-0020).
	DeletedByRetention DeletionReason = "RETENTION"
	// DeletedByErasure is a data subject request being fulfilled (GDPR Art. 17).
	DeletedByErasure DeletionReason = "DSR_ERASURE"
	// DeletedByAdmin is an operator acting outside the ordinary paths.
	DeletedByAdmin DeletionReason = "ADMIN"
)

// Removal is one row that has gone for good, as the three records of it need to know it.
//
// One value rather than three parameter lists, because the three records are written together or
// not at all: the journal entry that stops a restore from bringing it back, the tombstone that stops
// a device from recreating it, and the change every consumer of the event stream hears about. A
// removal recorded in two of the three is the orphan the completeness rule forbids (ADR-0020 §6).
type Removal struct {
	// Entity is the table the row was in, in the words the journal and the tombstone use:
	// `work_item`, `container`. A string rather than an enumeration, because these two tables are
	// the ones this task removes from and the mechanism is meant to serve the rest of the catalogue
	// without a code change (data-retention.md §3).
	Entity   string
	EntityID shared.ID
	Reason   DeletionReason
}

// Tombstone is the marker a removal leaves behind (offline-sync.md §7).
//
// Without it the classic bug appears: a device that was offline for eight weeks still knows the
// entry, pushes a change for it, and the server - having no record that it ever existed - accepts
// the change and recreates it. The marker is what makes that push a `sync.gone` instead.
type Tombstone struct {
	Entity   string
	EntityID shared.ID
	// PurgeAfter is when the marker itself may go: the removal plus the maximum offline window. A
	// device whose cursor is older than that has to resynchronise from scratch anyway, so the
	// marker has done its work by then.
	PurgeAfter time.Time
}
