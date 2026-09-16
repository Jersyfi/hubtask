// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer is an import from another system (P-08, backup-restore.md §9): the use case
// that accepts one, the read that answers its report, and the job that runs it.
//
// One ingestion path, not two: the file is converted into the records a backup archive holds and
// applied through the restore's applier in MERGE mode with skip, under identities derived from
// the source's own. Nothing here writes a row of its own.
package importer

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/importer"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ImportEntriesName = "ImportEntries"
	GetImportName     = "GetImport"

	// ImportedAction is the audit code: somebody brought another system's data in.
	ImportedAction audit.Action = "import.requested"

	importTarget    = "import"
	containersWrite = "containers:write"
	containersRead  = "containers:read"
)

// Authorizer is the slice of the authorisation service this use case needs.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// The slices of the ports the import needs - an interface each, so that the use case and the
// runner can be tested without a database and the presentation layer keeps pointing inwards.
type (
	// ObjectFinder answers the uploaded file's record, and marks it for deletion when the job
	// ends.
	ObjectFinder interface {
		Find(ctx context.Context, id shared.ID) (media.Object, error)
		MarkDeleted(ctx context.Context, id shared.ID, at time.Time) (bool, error)
	}
	// ContainerFinder answers the hub.
	ContainerFinder interface {
		Find(ctx context.Context, id shared.ID) (work.Container, error)
	}
	// Enqueuer accepts the job.
	Enqueuer interface {
		Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
	}
)

var (
	_ ObjectFinder    = mediarepo.Objects(nil)
	_ ContainerFinder = workrepo.Containers(nil)
	_ Enqueuer        = queue.Queue(nil)
)

// ImportEntries accepts an import: checks the hub, the file and the kind, writes the run and
// enqueues the job.
type ImportEntries struct {
	Runs       repository.Runs
	Objects    ObjectFinder
	Containers ContainerFinder
	Authorizer Authorizer
	Jobs       Enqueuer
	// Kinds is which kinds this build converts; a kind the contract declares and no converter
	// serves is refused by name here rather than in the job.
	Kinds      []domain.Kind
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// ImportCommand is what a caller asks.
type ImportCommand struct {
	MediaID shared.ID
	Kind    domain.Kind
	HubID   shared.ID
	Mapping map[string]string
}

// Accepted is what the use case answers: the job, and the run whose report the job will write.
type Accepted struct {
	JobID    shared.ID
	ImportID shared.ID
}

// Execute accepts the import.
func (h ImportEntries) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ImportCommand,
) (Accepted, error) {
	if cmd.HubID.IsZero() {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeHubRequired).
			WithFields(shared.FieldError{Path: "/hub_id", Code: domain.CodeHubRequired})
	}
	if !h.serves(cmd.Kind) {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeKindUnsupported).
			WithParams(map[string]string{"kind": string(cmd.Kind)}).
			WithFields(shared.FieldError{Path: "/kind", Code: domain.CodeKindUnsupported})
	}
	// Before the transaction: a refusal writes an audit entry, and one written inside the
	// transaction would be rolled back with the refusal (audit.md §7). STRUCTURE on the hub,
	// because an import creates collections there.
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: domainservice.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope(), identity.HubScope(cmd.HubID)},
		Action:     ImportedAction,
		TokenScope: containersWrite,
		TargetType: importTarget,
		TargetID:   cmd.HubID,
	}); err != nil {
		return Accepted{}, err
	}

	importID := h.IDs.NewID()
	var accepted Accepted
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		hub, err := h.Containers.Find(ctx, cmd.HubID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return shared.ErrNotFound.WithDetail("containers.not_found").
					WithParams(map[string]string{"container_id": cmd.HubID.String()})
			}
			return err
		}
		if hub.Type != work.ContainerHub {
			return shared.ErrValidation.WithDetail(domain.CodeNotAHub).
				WithFields(shared.FieldError{Path: "/hub_id", Code: domain.CodeNotAHub})
		}
		if err := h.checkObject(ctx, actor, cmd.MediaID); err != nil {
			return err
		}
		if err := h.Runs.Insert(ctx, domain.Run{
			ID: importID, TenantID: actor.TenantID, RequestedBy: actor.AccountID,
			Kind: cmd.Kind, MediaID: cmd.MediaID, HubID: cmd.HubID, Mapping: cmd.Mapping,
			Zone: actor.TimeZone, Language: actor.Locale,
			Status: domain.StatusPending, CreatedAt: h.Clock.Now(),
		}); err != nil {
			return err
		}
		jobID, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind:      queue.KindImport,
			TenantID:  actor.TenantID,
			Payload:   map[string]any{"import_id": importID.String()},
			DedupeKey: "import:" + importID.String(),
		})
		if err != nil {
			return err
		}
		accepted = Accepted{JobID: jobID, ImportID: importID}
		return nil
	})
	if err != nil {
		return Accepted{}, err
	}
	return accepted, nil
}

// checkObject is the file: staged as an import, confirmed, and the caller's own. Every other
// answer is the same code, so that a guessed identifier learns nothing (T-04's shape).
func (h ImportEntries) checkObject(ctx context.Context, actor appshared.ActorContext, mediaID shared.ID) error {
	object, err := h.Objects.Find(ctx, mediaID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return shared.ErrNotFound.WithDetail("media.not_found").
				WithParams(map[string]string{"media_id": mediaID.String()})
		}
		return err
	}
	if object.CreatedBy != actor.AccountID {
		return shared.ErrNotFound.WithDetail(domain.CodeMediaNotOwned).
			WithParams(map[string]string{"media_id": mediaID.String()})
	}
	if object.Usage != media.UsageImport {
		return shared.ErrValidation.WithDetail(domain.CodeMediaNotImport).
			WithFields(shared.FieldError{Path: "/media_id", Code: domain.CodeMediaNotImport})
	}
	if object.Status != media.StatusReady || object.DeletedAt != nil {
		return shared.ErrConflict.WithDetail(domain.CodeMediaNotReady).
			WithParams(map[string]string{"media_id": mediaID.String()})
	}
	return nil
}

func (h ImportEntries) serves(kind domain.Kind) bool {
	for _, each := range h.Kinds {
		if each == kind {
			return true
		}
	}
	return false
}

// Descriptor is the catalogue entry.
func (h ImportEntries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ImportEntriesName,
		Summary: "Imports entries from another system - a CSV, a Trello board, a Google Tasks " +
			"export, a Microsoft To Do dump - as collections under a hub, through the same path a " +
			"restore uses: converted into archive records under derived identities and applied " +
			"in MERGE mode with skip, so that the same file twice creates nothing the second " +
			"time. The file arrives first through the media upload staged as an import.",
		SideEffects: "Writes the import run and enqueues its job; the job writes the collections, " +
			"buckets, labels, entries and links, advances the workspace's synchronisation epoch, " +
			"and deletes the uploaded file. Records the request in the audit trail.",
		TokenScope:  containersWrite,
		Destructive: false,
		Input: []usecase.Field{
			{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "The uploaded file, staged with usage IMPORT and confirmed."},
			{
				Name: "kind", Kind: usecase.KindString, Required: true,
				Enum:        []string{string(domain.KindCSV), string(domain.KindTrello), string(domain.KindGoogleTasks), string(domain.KindMicrosoftTodo)},
				Description: "The system the file came from.",
			},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the imported collections land under."},
			{
				Name: "mapping", Kind: usecase.KindObject,
				Description: "For CSV: which column carries which field, by header name, where the header does not already say.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: ImportedAction, TargetType: importTarget, Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ImportEntries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	mediaID, err := in.ID("media_id")
	if err != nil {
		return nil, err
	}
	hubID, err := in.ID("hub_id")
	if err != nil {
		return nil, err
	}
	kind, err := domain.ParseKind(in.String("kind"))
	if err != nil {
		return nil, err
	}
	mapping := map[string]string{}
	if raw, ok := in["mapping"].(map[string]any); ok {
		for key, value := range raw {
			if text, ok := value.(string); ok {
				mapping[key] = text
			}
		}
	}
	accepted, err := h.Execute(ctx, actor, ImportCommand{MediaID: mediaID, Kind: kind, HubID: hubID, Mapping: mapping})
	if err != nil {
		return nil, err
	}
	return usecase.Output{
		"job_id":     accepted.JobID.String(),
		"import_id":  accepted.ImportID.String(),
		"result_url": "/imports/" + accepted.ImportID.String(),
		"request_id": correlation.RequestIDFrom(ctx),
	}, nil
}
