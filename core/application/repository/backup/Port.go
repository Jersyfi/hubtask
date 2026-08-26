// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backup is the outbound port for the targets a tenant has configured (E-03).
package backup

import (
	"context"
	"time"

	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
)

// Targets stores and reads what a tenant has configured, and keeps the credential apart from
// everything else.
//
// Credentials is a method of its own rather than a field on a target, and that is the shape the
// whole "never returned by any read" requirement rests on: the rows that go to a response cannot
// contain a credential, because the statements behind them do not select one. A field that a
// mapper could accidentally copy is a field that eventually is.
type Targets interface {
	// Insert writes the target and its sealed credential. The credential arrives already sealed:
	// this port knows there is one and never what it says.
	Insert(ctx context.Context, target domain.Target, credential crypto.Sealed) error

	// List answers the tenant's targets, by name. Never a credential.
	List(ctx context.Context) ([]domain.Target, error)

	// Find answers one target, or ErrNotFound. Never a credential.
	Find(ctx context.Context, id shared.ID) (domain.Target, error)

	// Credential answers the sealed credential of a target, still sealed. The only caller is the
	// use case that opens a connection, and the only thing it does with the answer is hand it to
	// the encryptor.
	Credential(ctx context.Context, id shared.ID) (crypto.Sealed, error)

	// RecordTest writes down what the connection probe found: when, whether it worked, and the
	// message code when it did not. Never a driver message - one of those carries the host, the
	// user and sometimes the password (rule 10).
	RecordTest(ctx context.Context, id shared.ID, at time.Time, ok bool, code string) error

	// Coverage is what the installation's health surface asks: how many targets there are, and
	// how many of them store an archive unencrypted (backup-restore.md §10).
	Coverage(ctx context.Context) (Coverage, error)
}

// Coverage is the answer to "is this tenant backed up, and how badly".
type Coverage struct {
	Configured  int
	Unencrypted int
}

// Export is the tenant's rows as the archive writer needs them (E-05, backup-restore.md §3).
//
// It is keyed by table name rather than by the archive's entity names, and that is the seam: the
// database's vocabulary stops here, and the archive's begins on the other side. The deletion
// markers are written in the same vocabulary, so a tombstone against `work_item` and a page of
// `work_item` rows are asked for with one word.
//
// Everything is a callback rather than a slice, for the reason the archive's own Source is: the
// answer is as large as the tenant, and a method returning []Row would read a holding into memory
// before writing a byte of it (T-17). What the implementation does behind that is page on each
// entity's own key - never OFFSET - so that a page can neither repeat nor skip a row while the
// snapshot is open, and a resumed run continues where it stopped instead of counting again.
type Export interface {
	// Rows hands over one table's rows, oldest change first.
	//
	// since is exclusive and is the zero time for a whole read. A table that cannot date a change
	// is asked for whole and answers everything whatever is passed - the caller decides which
	// those are, because it is the archive that has to record the decision for a restore.
	Rows(ctx context.Context, table string, since time.Time, yield func(Row) error) error

	// Tombstones hands over one table's deletion markers after an instant, oldest first. Nothing
	// at all for a full archive, which has no earlier state to contradict.
	Tombstones(ctx context.Context, table string, since time.Time, yield func(Tombstone) error) error

	// MediaLocation answers where the bytes of one medium lie, by the checksum the archive
	// addresses it with, or ErrNotFound.
	MediaLocation(ctx context.Context, checksum string) (MediaLocation, error)
}

// Row is one row on its way into an archive.
type Row struct {
	// ID is the row's identity: the primary key, or its parts joined by "/" in the order the
	// schema declares them. It is also the cursor - the parts are split back out to ask for the
	// next page - which is why nothing that can contain a slash is ever part of one.
	ID string
	// ChangedAt is when the row last changed, and the zero time from a table that cannot say.
	ChangedAt time.Time
	// Data is the row, with `tenant_id` already removed: a restore into another tenant must not
	// carry the old one's identifier back in with it.
	Data map[string]any
}

// Tombstone is one deletion marker.
type Tombstone struct {
	ID        string
	DeletedAt time.Time
}

// MediaLocation is where one medium's bytes are, in the object store's terms.
type MediaLocation struct {
	StorageKey string
	Bytes      int64
}
