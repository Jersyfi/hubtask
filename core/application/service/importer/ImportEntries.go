// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package importer is an import from another system (backup-restore.md §9): the use case
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
	// ContainerFinder answers the hub, and the collection an import lands in.
	ContainerFinder interface {
		Find(ctx context.Context, id shared.ID) (work.Container, error)
	}
	// BucketReader answers the target collection's live columns and the last one's rank.
	BucketReader interface {
		List(ctx context.Context, collectionID shared.ID) ([]work.Bucket, error)
		LastOrderKey(ctx context.Context, collectionID shared.ID) (string, error)
	}
	// LabelReader answers the target collection's live labels.
	LabelReader interface {
		List(ctx context.Context, collectionID shared.ID) ([]work.Label, error)
	}
	// ItemOrders answers the rank of the target collection's last top-level entry.
	ItemOrders interface {
		LastOrderKey(ctx context.Context, collectionID, parentID shared.ID) (string, error)
	}
	// Enqueuer accepts the job.
	Enqueuer interface {
		Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
	}
)

var (
	_ ObjectFinder    = mediarepo.Objects(nil)
	_ ContainerFinder = workrepo.Containers(nil)
	_ BucketReader    = workrepo.Buckets(nil)
	_ LabelReader     = workrepo.Labels(nil)
	_ ItemOrders      = workrepo.Items(nil)
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
	// CollectionID, where set, is a collection of the hub the entries land in, instead of new
	// collections under the hub.
	CollectionID shared.ID
	Mapping      map[string]string
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
	if cmd.Kind.LandsPages() && !cmd.CollectionID.IsZero() {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeKindTakesNoCollection).
			WithFields(shared.FieldError{Path: "/collection_id", Code: domain.CodeKindTakesNoCollection})
	}
	if !h.serves(cmd.Kind) {
		return Accepted{}, shared.ErrValidation.WithDetail(domain.CodeKindUnsupported).
			WithParams(map[string]string{"kind": string(cmd.Kind)}).
			WithFields(shared.FieldError{Path: "/kind", Code: domain.CodeKindUnsupported})
	}
	// Before the transaction: a refusal writes an audit entry, and one written inside the
	// transaction would be rolled back with the refusal (audit.md §7). STRUCTURE on the hub,
	// because an import creates collections there; into a collection, STRUCTURE on that one,
	// because the import may add its columns and labels. The collection is read first, so that the
	// path asked about is its own and not the one the caller named.
	path, target := []identity.Scope{identity.TenantScope(), identity.HubScope(cmd.HubID)}, cmd.HubID
	if !cmd.CollectionID.IsZero() {
		var collection work.Container
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := h.collection(ctx, cmd)
			collection = found
			return err
		})
		if err != nil {
			return Accepted{}, err
		}
		path, target = domainservice.ContainerScopes(collection), collection.ID
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: domainservice.PermissionStructure,
		Path:       path,
		Action:     ImportedAction,
		TokenScope: containersWrite,
		TargetType: importTarget,
		TargetID:   target,
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
		if !cmd.CollectionID.IsZero() {
			if _, err := h.collection(ctx, cmd); err != nil {
				return err
			}
		}
		if err := h.checkObject(ctx, actor, cmd.MediaID); err != nil {
			return err
		}
		if err := h.Runs.Insert(ctx, domain.Run{
			ID: importID, TenantID: actor.TenantID, RequestedBy: actor.AccountID,
			Kind: cmd.Kind, MediaID: cmd.MediaID, HubID: cmd.HubID, CollectionID: cmd.CollectionID,
			Mapping: cmd.Mapping, Zone: actor.TimeZone, Language: actor.Locale,
			Status: domain.StatusPending, CreatedAt: h.Clock.Now(),
		}); err != nil {
			return err
		}
		// Into a collection, a kind of its own: a process of the release before, which would land
		// the file as new collections, does not claim it (rule 12).
		kind := queue.KindImport
		if !cmd.CollectionID.IsZero() {
			kind = queue.KindImportIntoCollection
		}
		jobID, err := h.Jobs.Enqueue(ctx, queue.Request{
			Kind:      kind,
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

// collection answers the collection the import lands in: a live collection of the named hub, or
// one refusal whatever else it is, so that a guessed identifier learns nothing about another hub.
func (h ImportEntries) collection(ctx context.Context, cmd ImportCommand) (work.Container, error) {
	collection, err := h.Containers.Find(ctx, cmd.CollectionID)
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return work.Container{}, err
	}
	if err != nil || collection.Type != work.ContainerCollection || collection.ParentID != cmd.HubID || collection.DeletedAt != nil {
		return work.Container{}, shared.ErrValidation.WithDetail(domain.CodeNotInHub).
			WithFields(shared.FieldError{Path: "/collection_id", Code: domain.CodeNotInHub})
	}
	return collection, nil
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
		Name:       ImportEntriesName,
		Permission: domainservice.PermissionStructure,
		Summary: "Imports entries from another system - a CSV, a Trello board, a Google Tasks " +
			"export, a Microsoft To Do dump, an issue tracker's projects with their types, statuses, " +
			"fields, history and attachments - as collections under a hub, or as entries in one of its " +
			"collections, or a wiki space's pages into the hub, through the same path a " +
			"restore uses: converted into archive records under derived identities and applied " +
			"in MERGE mode with skip, so that the same file twice creates nothing the second " +
			"time. The file arrives first through the media upload staged as an import.",
		SideEffects: "Writes the import run and enqueues its job; the job writes the collections, " +
			"buckets, labels, entry types, workflows, fields, entries, their history, comments, " +
			"attachments and links, or the pages, advances the workspace's synchronisation epoch, " +
			"and deletes the uploaded file. Records the request in the audit trail.",
		TokenScope:  containersWrite,
		Destructive: false,
		Input: []usecase.Field{
			{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "The uploaded file, staged with usage IMPORT and confirmed."},
			{
				Name: "kind", Kind: usecase.KindString, Required: true,
				Enum: []string{
					string(domain.KindCSV), string(domain.KindTrello), string(domain.KindGoogleTasks), string(domain.KindMicrosoftTodo),
					string(domain.KindIssueTracker), string(domain.KindWiki), string(domain.KindMarkdown),
				},
				Description: "The system the file came from.",
			},
			{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub the imported collections land under."},
			{Name: "collection_id", Kind: usecase.KindID, Description: "A collection of that hub to land the entries in, instead of new collections under the hub."},
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
	collectionID, err := in.ID("collection_id")
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
	accepted, err := h.Execute(ctx, actor, ImportCommand{MediaID: mediaID, Kind: kind, HubID: hubID, CollectionID: collectionID, Mapping: mapping})
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
