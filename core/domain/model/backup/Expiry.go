// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Archive is one archive as the retention plan sees it: when it was taken, what it is, and what it
// needs in order to be restorable.
//
// Deliberately not the manifest. The plan decides from four facts, and giving it the whole manifest
// would let a future rule decide from a count or a size - which is a different kind of retention
// and one §6 does not have.
type Archive struct {
	ID shared.ID
	// TakenAt is the instant the archive represents, which is what the generations are counted in.
	TakenAt time.Time
	Mode    Mode
	// ParentID is the archive this one continues, empty on a full one.
	ParentID shared.ID
}

// Expiry is what a plan decided: what stays, what may go, and why nothing at all went.
type Expiry struct {
	Keep []Archive
	// Expire is what may be deleted, oldest first - which is also the order to delete in, so that
	// a run interrupted half way has removed the least useful ones.
	Expire []Archive
	// FloorHeld is whether min_keep was what stopped further deletion. It is reported rather than
	// inferred, because "the plan wanted to delete more and was not allowed to" is the sentence an
	// operator needs when a target fills up.
	FloorHeld bool
	// ChainHeld names the archives kept only because something newer needs them. An operator
	// looking at a target that will not shrink is looking at these.
	ChainHeld []Archive
}

// Apply decides which archives a plan keeps.
//
// The generations are counted in the schedule's own zone, because a day is a day where the
// operator is: a plan asked to keep fourteen dailies across a zone change should keep fourteen of
// their days, not fourteen of UTC's.
//
// Two rules sit above the generations and both exist to make retention safe rather than tidy:
//
//   - **An archive another kept archive needs is kept.** An incremental restores only through its
//     chain back to a full archive, so deleting a parent does not free one archive - it destroys
//     every archive after it, silently, and the loss is discovered at the restore. §6 does not
//     spell this out because it is not a retention rule; it is what makes the retention rules
//     survivable.
//   - **`min_keep` is a floor, not a generation.** Whatever the counts work out to, that many
//     archives stay. It is the answer to the failure mode that makes retention frightening - a
//     plan that is misread, or a clock that jumps, deleting the last copy of everything.
func (r Retention) Apply(archives []Archive, zone *time.Location) Expiry {
	if zone == nil {
		zone = time.UTC
	}
	// Newest first: every generation is "the most recent N of something", and counting from the
	// newest end is what makes that one pass.
	ordered := slices.Clone(archives)
	slices.SortFunc(ordered, func(a, b Archive) int {
		if !a.TakenAt.Equal(b.TakenAt) {
			return b.TakenAt.Compare(a.TakenAt)
		}
		// A tie is broken by identity so that the same input decides the same way twice. Two
		// archives at one instant is a clock that went backwards, and an arbitrary answer to it
		// is worse than a fixed one.
		return strings.Compare(string(b.ID), string(a.ID))
	})

	kept := map[shared.ID]bool{}
	for index := range min(r.KeepLast, len(ordered)) {
		kept[ordered[index].ID] = true
	}
	keepNewestPerBucket(ordered, zone, r.KeepDaily, dayOf, kept)
	keepNewestPerBucket(ordered, zone, r.KeepWeekly, weekOf, kept)
	keepNewestPerBucket(ordered, zone, r.KeepMonthly, monthOf, kept)
	keepNewestPerBucket(ordered, zone, r.KeepYearly, yearOf, kept)

	// The floor, before the chain: an archive kept by the floor may itself need a parent, and
	// applying the chain rule afterwards is what catches that.
	floorHeld := false
	for index := 0; index < len(ordered) && len(kept) < r.MinKeep; index++ {
		if !kept[ordered[index].ID] {
			kept[ordered[index].ID] = true
			floorHeld = true
		}
	}

	chainHeld := keepAncestors(ordered, kept)

	expiry := Expiry{FloorHeld: floorHeld}
	for _, archive := range ordered {
		if kept[archive.ID] {
			expiry.Keep = append(expiry.Keep, archive)
			continue
		}
		expiry.Expire = append(expiry.Expire, archive)
	}
	// Oldest first, which is also the order to delete in: a deletion pass interrupted half way
	// has removed the least useful archives rather than a random selection of them.
	slices.Reverse(expiry.Expire)
	for _, id := range chainHeld {
		if index := slices.IndexFunc(ordered, func(a Archive) bool { return a.ID == id }); index >= 0 {
			expiry.ChainHeld = append(expiry.ChainHeld, ordered[index])
		}
	}
	return expiry
}

// keepNewestPerBucket keeps the newest archive of each of the most recent count buckets.
//
// "The newest of each period" rather than "the oldest": a daily backup is kept because it is the
// state at the end of that day, which is what somebody restoring "yesterday" means. Walking a
// newest-first list and taking the first archive of every new bucket is exactly that.
func keepNewestPerBucket(
	ordered []Archive, zone *time.Location, count int, bucket func(time.Time) string,
	kept map[shared.ID]bool,
) {
	if count <= 0 {
		return
	}
	seen := map[string]bool{}
	for _, archive := range ordered {
		key := bucket(archive.TakenAt.In(zone))
		if seen[key] {
			continue
		}
		if len(seen) >= count {
			return
		}
		seen[key] = true
		kept[archive.ID] = true
	}
}

// keepAncestors keeps everything a kept archive needs, and answers what was kept only for that.
//
// Walked repeatedly rather than recursively: a chain is short, the input is small, and a loop that
// cannot recurse cannot blow a stack on an archive whose parent points at itself - which is a shape
// a target somebody has been editing can produce.
func keepAncestors(ordered []Archive, kept map[shared.ID]bool) []shared.ID {
	parents := map[shared.ID]shared.ID{}
	for _, archive := range ordered {
		if !archive.ParentID.IsZero() {
			parents[archive.ID] = archive.ParentID
		}
	}

	var held []shared.ID
	for range len(ordered) {
		added := false
		for id := range kept {
			parent, hasParent := parents[id]
			if !hasParent || kept[parent] {
				continue
			}
			kept[parent] = true
			held = append(held, parent)
			added = true
		}
		if !added {
			break
		}
	}
	slices.Sort(held)
	return held
}

func dayOf(at time.Time) string   { return at.Format("2006-01-02") }
func monthOf(at time.Time) string { return at.Format("2006-01") }
func yearOf(at time.Time) string  { return at.Format("2006") }

// weekOf is the ISO week, which is the one definition of "week" that does not move between
// locales: a week starts on Monday and belongs to the year its Thursday is in.
func weekOf(at time.Time) string {
	year, week := at.ISOWeek()
	return fmt.Sprintf("%04d-W%02d", year, week)
}
