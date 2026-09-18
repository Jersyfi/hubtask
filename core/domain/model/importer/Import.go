// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer is an import from another system (P-08, backup-restore.md §9): a file
// somebody exported elsewhere, converted into the records a backup archive holds and applied
// through the restore. The run is the import's own record; the report it carries is the
// restore's, because the applying is the restore's.
package importer

import (
	"time"

	backup "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Kind is the system the file came from.
type Kind string

const (
	KindCSV           Kind = "CSV"
	KindTrello        Kind = "TRELLO"
	KindGoogleTasks   Kind = "GOOGLE_TASKS"
	KindMicrosoftTodo Kind = "MICROSOFT_TODO"
)

// ParseKind reads a submitted kind. The contract's enum is the whole list; which of them this
// build converts is the converters' business and refused by name there.
func ParseKind(value string) (Kind, error) {
	kind := Kind(value)
	switch kind {
	case KindCSV, KindTrello, KindGoogleTasks, KindMicrosoftTodo:
		return kind, nil
	}
	return "", shared.ErrValidation.
		WithDetail(CodeKindUnknown).
		WithParams(map[string]string{"value": value}).
		WithFields(shared.FieldError{Path: "/kind", Code: CodeKindUnknown})
}

// Status is where the run stands.
type Status string

const (
	StatusPending   Status = "PENDING"
	StatusRunning   Status = "RUNNING"
	StatusSucceeded Status = "SUCCEEDED"
	StatusFailed    Status = "FAILED"
)

// Refusal is one row of the source the converter could not read, while the rest landed.
type Refusal struct {
	Row  int
	Code string
}

// Run is one import.
type Run struct {
	ID          shared.ID
	TenantID    shared.ID
	RequestedBy shared.ID
	Kind        Kind
	MediaID     shared.ID
	HubID       shared.ID
	// Mapping is the CSV kind's: which column carries which field, by header name.
	Mapping map[string]string
	// Zone and Language are the requester's at the request: what a date without a zone and an
	// entry without a language are read in.
	Zone     string
	Language string
	Status   Status
	// Report is what the applier did, in the restore's shape, once the job has run.
	Report backup.Report
	// Refused is what the converter could not read, by row.
	Refused []Refusal
	// Progress is how far the applier got, per entity, for a resumed attempt.
	Progress   map[string]int
	ErrorCode  string
	CreatedAt  time.Time
	StartedAt  *time.Time
	FinishedAt *time.Time
}

// Outcome is how a run ended.
type Outcome struct {
	ID         shared.ID
	Status     Status
	Report     backup.Report
	Refused    []Refusal
	ErrorCode  string
	FinishedAt time.Time
}

// The message codes.
const (
	CodeKindUnknown      = "imports.kind_unknown"
	CodeKindUnsupported  = "imports.kind_unsupported"
	CodeNotFound         = "imports.not_found"
	CodeMediaNotImport   = "imports.media_not_import"
	CodeMediaNotReady    = "imports.media_not_ready"
	CodeMediaNotOwned    = "imports.media_not_owned"
	CodeHubRequired      = "imports.hub_required"
	CodeNotAHub          = "imports.not_a_hub"
	CodeFileNotKind      = "imports.file_not_kind"
	CodeFileEmpty        = "imports.file_empty"
	CodeRunNotRunning    = "imports.run_not_running"
	CodeRowUnreadable    = "imports.row_unreadable"
	CodeRowDateInvalid   = "imports.row_date_invalid"
	CodeRowZoneUnknown   = "imports.row_zone_unknown"
	CodeRowTitleMissing  = "imports.row_title_missing"
	CodeRowParentUnknown = "imports.row_parent_unknown"
	CodeMappingUnknown   = "imports.mapping_unknown"
	CodeEncodingInvalid  = "imports.encoding_invalid"
	// CodeCollectionExists is a collection the file would create colliding, by name, with one the
	// hub already holds (issue 766). A refusal recorded on the run rather than a retried database
	// error: the next attempt would meet the same name.
	CodeCollectionExists = "imports.collection_exists"
)
