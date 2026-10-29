// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package realtime is the live document (F-8): a document edited by several people at once, held
// as an append-only log of CRDT updates, a snapshot that compacts it, and the awareness the
// editors share while connected (offline-sync.md §13).
//
// The server never reads an update. The format is the editors' CRDT update, and
// merging is the clients': two updates applied in any order give the same document. What the
// server decides is who may read and write which document, the order it hands updates out in,
// how large one may be, and when the history behind a snapshot may go.
package realtime

import (
	"regexp"
	"strconv"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind names what a document is (a page, a whiteboard). Each kind is registered by the step that
// builds it, with the place its documents are judged at.
type Kind string

var kindPattern = regexp.MustCompile(`^[a-z][a-z_]{1,31}$`)

// Valid reports whether the kind is spelt as the store accepts it.
func (k Kind) Valid() bool { return kindPattern.MatchString(string(k)) }

// Ref names one document.
type Ref struct {
	Kind Kind
	ID   shared.ID
}

// NewRef checks what the database constrains, before the database has to.
func NewRef(kind string, id shared.ID) (Ref, error) {
	ref := Ref{Kind: Kind(kind), ID: id}
	if !ref.Kind.Valid() {
		return Ref{}, shared.ErrValidation.WithDetail("realtime.kind_invalid").
			WithFields(shared.FieldError{Path: "/kind", Code: "realtime.kind_invalid"})
	}
	if id.IsZero() {
		return Ref{}, shared.ErrValidation.WithDetail("realtime.document_required").
			WithFields(shared.FieldError{Path: "/document_id", Code: "realtime.document_required"})
	}
	return ref, nil
}

// The bounds. An update is one person's keystrokes since the last one sent, a paste at most; a
// snapshot is the whole document. Both are bounded so that one connection cannot fill the store,
// and awareness - a cursor, a selection, a name - far below either.
const (
	MaxUpdateBytes    = 1 << 20
	MaxSnapshotBytes  = 16 << 20
	MaxAwarenessBytes = 16 << 10
	// CompactAfter is how many updates past the snapshot make the server ask a writer for a new
	// one: the cost of opening a document is the snapshot and the updates after it.
	CompactAfter = 500
	// MaxOpenUpdates bounds what one opening reads; a log longer than that past its snapshot is
	// answered in pages and compaction is asked for.
	MaxOpenUpdates = 2000
)

// Update is one CRDT update as the log keeps it: its place in the document's order, the bytes as
// the editor sent them, who sent them and when. Author is empty once the account is gone.
type Update struct {
	Seq      int64
	Data     []byte
	AuthorID shared.ID
	At       time.Time
}

// CheckUpdate bounds an update before it is stored.
func CheckUpdate(data []byte) error {
	switch {
	case len(data) == 0:
		return shared.ErrValidation.WithDetail("realtime.update_empty")
	case len(data) > MaxUpdateBytes:
		return shared.ErrValidation.WithDetail("realtime.update_too_large").
			WithParams(map[string]string{"max": strconv.Itoa(MaxUpdateBytes)})
	}
	return nil
}

// Snapshot is the document merged up to an update: what a writer encoded from its own state, so
// that opening the document reads one value and the updates after it instead of the whole log.
type Snapshot struct {
	UpToSeq int64
	State   []byte
	At      time.Time
}

// CheckSnapshot bounds a snapshot against the document's head: it can only cover what was there.
func CheckSnapshot(snapshot Snapshot, head, current int64) error {
	switch {
	case len(snapshot.State) == 0:
		return shared.ErrValidation.WithDetail("realtime.snapshot_empty")
	case len(snapshot.State) > MaxSnapshotBytes:
		return shared.ErrValidation.WithDetail("realtime.snapshot_too_large").
			WithParams(map[string]string{"max": strconv.Itoa(MaxSnapshotBytes)})
	case snapshot.UpToSeq < 1 || snapshot.UpToSeq > head:
		return shared.ErrValidation.WithDetail("realtime.snapshot_beyond_head").
			WithParams(map[string]string{"head": strconv.FormatInt(head, 10)})
	case snapshot.UpToSeq <= current:
		// An older snapshot than the one held covers less; a second one at the same update says
		// nothing new. Either is a writer that raced another, and the first one stands.
		return shared.ErrVersionConflict.WithDetail("realtime.snapshot_superseded")
	}
	return nil
}

// Head is where a document's log stands: the last update, and the one its snapshot covers.
type Head struct {
	Seq         int64
	SnapshotSeq int64
}

// CompactionDue reports whether the log has grown far enough past its snapshot that a writer
// should send a new one.
func (h Head) CompactionDue() bool { return h.Seq-h.SnapshotSeq >= CompactAfter }

// CheckAwareness bounds one awareness message, which is relayed and never stored.
func CheckAwareness(data []byte) error {
	if len(data) == 0 || len(data) > MaxAwarenessBytes {
		return shared.ErrValidation.WithDetail("realtime.awareness_invalid").
			WithParams(map[string]string{"max": strconv.Itoa(MaxAwarenessBytes)})
	}
	return nil
}
