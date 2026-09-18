// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer is the outbound side of an import (P-08): the run's repository, and the
// converter port - a foreign file in, archive records out.
//
// One ingestion path, not two (backup-restore.md §9): a converter never writes a row. It reads
// what somebody exported elsewhere and answers the same records a backup archive holds, under
// identities derived from the source's own, and the restore's applier lands them. The converters
// live in infrastructure/importer, one per kind, because a foreign format is an adapter's business
// even when it is a file rather than a wire.
package importer

import (
	"context"
	"io"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backup "github.com/Jersyfi/hubtask/core/domain/model/backup"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Runs stores what an import asked and did. Every method runs inside a transaction whose tenant
// is set (ADR-0010); none takes a tenant.
type Runs interface {
	// Insert writes the accepted run, PENDING.
	Insert(ctx context.Context, run domain.Run) error
	// Find answers one run, or ErrNotFound.
	Find(ctx context.Context, id shared.ID) (domain.Run, error)
	// Claim moves the run to RUNNING and answers whether it could; a run already RUNNING claims
	// itself again, which is what makes a job that died resumable.
	Claim(ctx context.Context, id shared.ID, at time.Time) (bool, error)
	// RecordProgress writes how far the applier got and the report so far, in the transaction
	// of the batch that got it there (the applier's BK-7 rule).
	RecordProgress(ctx context.Context, id shared.ID, report backup.Report, progress map[string]int) error
	// Finish records how the run ended. Refused for a run that is no longer going.
	Finish(ctx context.Context, outcome domain.Outcome) error
}

// Source is the file and what the import knows about where it lands.
type Source struct {
	// Content is the file's bytes, bounded by the upload limit before it reached here.
	Content io.Reader
	// Hub is the hub the collections land under; the converter writes its containers beneath it.
	Hub shared.ID
	// Name is what the file was called when it arrived, without its extension, or empty where
	// nothing is known. A source that names nothing of its own - a CSV - names its collection
	// after it (issue 766): two files are then two collections, where a constant name met the
	// hub's unique name on the second import.
	Name string
	// Digest is the file's SHA-256, lower hex. A converter whose source carries no identity of
	// its own - a CSV - derives every identity from it and the hub, so that the same file
	// imported into the same hub twice produces the same identifiers and the second import
	// creates nothing; a source that names itself - a Trello board - derives from that name
	// instead, so that a fresh export of the same board lands on the rows the first one made.
	Digest string
	// Mapping is the CSV kind's column mapping; empty for every other kind.
	Mapping map[string]string
	// Now stamps the rows the source carries no time for.
	Now time.Time
	// Actor is who imports, recorded as the creator of what lands.
	Actor shared.ID
	// Zone is the actor's time zone, for a source that writes dates without one.
	Zone string
	// Language is the actor's locale, for the entries' content_language.
	Language string
}

// Result is what a converter answered: the records per entity, in the archive's entity order,
// and the rows it could not read.
type Result struct {
	// Records is keyed by the archive entity name (`containers`, `work_items`, …), each list in
	// an order a restore can apply - a parent before its children.
	Records map[string][]archive.Record
	Refused []domain.Refusal
	// Unmapped counts what the source carried and the product has no place for - a Trello
	// member's assignment - by name, so that the report can say what was lost.
	Unmapped map[string]int
}

// Converter turns one kind of file into records.
type Converter interface {
	Kind() domain.Kind
	// Convert reads the whole source. A file that is not the kind it claims to be is an error
	// (CodeFileNotKind); a row that cannot be read is a refusal, and the rest lands.
	Convert(ctx context.Context, source Source) (Result, error)
}
