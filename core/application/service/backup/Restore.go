// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package backup

import (
	"context"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	repository "github.com/Jersyfi/hubtask/core/application/repository/backup"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

const (
	ListBackupsAtTargetName = "ListBackupsAtTarget"
)

// Restorer is what the restore-side use cases share.
//
// One struct for the same reason Writer and Runner are one each: three use cases over one act that
// disagreed about which cipher or which clock to use would be three chances for an archive to be
// listed under one set of assumptions and restored under another.
type Restorer struct {
	Targets    repository.Targets
	Encryptor  crypto.Encryptor
	Opener     backupstorage.Opener
	Cipher     crypto.StreamCipher
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// ListBackupsAtTarget answers what is at a target, read from the manifests there.
type ListBackupsAtTarget struct{ Restorer Restorer }

// Archive is one archive as the listing answers it.
//
// It is assembled from the target alone. No row in any database is consulted, which is the
// property §8.1 promises in as many words - "a restore works even when the database is lost and
// only the target credentials exist" - and the reason `backup_run` calls itself "a log and an
// accelerator, not a prerequisite for a restore".
type Archive struct {
	ArchiveID string
	// Path is the archive's directory at the target, and it is what a restore is asked for. A
	// path rather than a run identifier, because a run identifier only means something to a
	// database.
	Path            string
	CreatedAt       time.Time
	Mode            string
	ParentArchiveID string
	ScopeKind       string
	ScopeID         string
	SizeBytes       int64
	ItemCount       int64
	MediaCount      int64
	SchemaVersion   string
	ProductVersion  string
	Encrypted       bool
	EncryptionKeyID string
	// Complete is whether the run that wrote it finished. An archive without `checksums.txt` is
	// not damaged; it is a run still going or one that died, and whoever is choosing what to
	// restore from has to be able to tell those apart.
	Complete bool
}

// Execute lists the archives one target holds for this tenant.
//
// The one database read is the target's own row and its credential, which is what opening a
// connection needs and nothing more. Everything the answer is made of comes from manifests at the
// target - so the day this matters, when the database is a fresh empty one and all that is left is
// a bucket and a key, the listing still works.
func (h ListBackupsAtTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, targetID, scopeID shared.ID,
) ([]Archive, error) {
	if targetID.IsZero() {
		return nil, shared.ErrValidation.WithDetail("backup.target_id_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "backup.target_id_required"})
	}
	// BK-10 begins here, before anything is read: a caller asking for another tenant's archives is
	// refused rather than answered with an empty list. An empty list would be a wrong answer to a
	// question nobody may ask.
	if !scopeID.IsZero() && scopeID != actor.TenantID {
		return nil, shared.ErrValidation.WithDetail(domain.CodeRestoreArchiveScopeMismatch).
			WithFields(shared.FieldError{
				Path: "/tenant_id", Code: domain.CodeRestoreArchiveScopeMismatch,
			})
	}
	if err := h.Restorer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetChangedAction,
		TokenScope: backupRead,
		TargetType: targetType,
		TargetID:   targetID,
	}); err != nil {
		return nil, err
	}

	store, err := h.Restorer.open(ctx, actor.PersistenceScope(), targetID)
	if err != nil {
		return nil, err
	}

	// Outside a transaction. A listing is a call to somebody else's machine and can take as long
	// as they feel like taking (observability-reliability.md §8), and there is nothing in the
	// database this answer needs to be consistent with.
	described, err := archive.NewReader(store, h.Restorer.Cipher).List(ctx, "")
	if err != nil {
		return nil, err
	}

	// The tenant's own archives and nobody else's. The name is the filter, because a target can be
	// shared and the storage port's prefix is a place rather than a string: an archive's name is a
	// directory under the target's root. BK-10's listing half.
	mine := archive.Prefix(actor.TenantID)
	archives := make([]Archive, 0, len(described))
	for _, description := range described {
		if !strings.HasPrefix(description.Prefix, mine) {
			continue
		}
		archives = append(archives, archiveOf(description))
	}
	return archives, nil
}

// archiveOf is one description as the listing answers it. Manifest fields only, and no user
// content among them - the manifest is the one member of an archive that is never encrypted, and
// whoever holds the storage can read it (rule 10).
func archiveOf(description archive.Description) Archive {
	manifest := description.Manifest
	var items int64
	for _, count := range manifest.Counts {
		items += count
	}
	return Archive{
		ArchiveID:       manifest.ArchiveID,
		Path:            description.Prefix,
		CreatedAt:       manifest.SnapshotAt,
		Mode:            string(manifest.Mode),
		ParentArchiveID: manifest.ParentID,
		ScopeKind:       string(manifest.Scope.Kind),
		ScopeID:         manifest.Scope.ID,
		SizeBytes:       description.Bytes,
		ItemCount:       items,
		MediaCount:      manifest.MediaCount,
		SchemaVersion:   manifest.SchemaVersion,
		ProductVersion:  manifest.ProductVersion,
		Encrypted:       manifest.Encryption.IsEncrypted(),
		EncryptionKeyID: manifest.Encryption.KeyID,
		Complete:        description.Complete,
	}
}

// open reads one target and hands back something to talk to it with.
//
// The transaction is as short as it can be - a row and a sealed credential - and the connection is
// used outside it. A target is somebody else's machine, and a transaction waiting on one holds a
// pool connection the API shares (observability-reliability.md §8).
func (r Restorer) open(
	ctx context.Context, scope persistence.Scope, targetID shared.ID,
) (backupstorage.Store, error) {
	var store backupstorage.Store
	err := r.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		target, err := r.Targets.Find(ctx, targetID)
		if err != nil {
			return err
		}
		sealed, err := r.Targets.Credential(ctx, targetID)
		if err != nil {
			return err
		}
		credentials, err := unsealCredentials(ctx, r.Encryptor, targetID, sealed)
		if err != nil {
			return err
		}
		store, err = r.Opener.Open(ctx, backupstorage.Spec{
			Kind: target.Kind, Config: target.Config, Credentials: credentials,
		})
		return err
	})
	if err != nil {
		return nil, err
	}
	return store, nil
}

// Descriptor registers the listing.
func (h ListBackupsAtTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListBackupsAtTargetName,
		Summary: "Lists the archives one backup target holds for this tenant, read from the " +
			"manifests at the target rather than from the database. That is what makes it the " +
			"reading that survives a total loss: with the target's credentials and nothing else, " +
			"this still answers. Each entry says when it was taken, what it covers, how large it " +
			"is, whether it is full or incremental, which archive it continues, which key it is " +
			"encrypted under, and whether the run that wrote it finished.",
		SideEffects: "None. Reads at the target and writes nothing anywhere.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "Which target to read.",
			},
			{
				Name: "tenant_id", Kind: usecase.KindID,
				Description: "Whose archives. Only your own, and asking for another's is " +
					"refused rather than answered empty.",
			},
			{
				Name: "refresh", Kind: usecase.KindBool,
				Description: "Read the target again rather than answering from a cache. This " +
					"build has no cache, so it reads again either way.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TargetChangedAction, TargetType: targetType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListBackupsAtTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	targetID, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	var scopeID shared.ID
	if named := in.OptionalString("tenant_id"); named != nil && *named != "" {
		scopeID, err = shared.ParseID(*named)
		if err != nil {
			return nil, err
		}
	}

	archives, err := h.Execute(ctx, actor, targetID, scopeID)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(archives))
	for _, found := range archives {
		rows = append(rows, archiveOutput(found))
	}
	return usecase.Output{"data": rows}, nil
}

// archiveOutput is one archive as the three channels answer it.
func archiveOutput(found Archive) usecase.Output {
	out := usecase.Output{
		"archive_id":      found.ArchiveID,
		"path":            found.Path,
		"created_at":      found.CreatedAt,
		"mode":            found.Mode,
		"size_bytes":      found.SizeBytes,
		"item_count":      found.ItemCount,
		"media_count":     found.MediaCount,
		"schema_version":  found.SchemaVersion,
		"product_version": found.ProductVersion,
		"encrypted":       found.Encrypted,
		"complete":        found.Complete,
		"scope": map[string]any{
			"kind": found.ScopeKind,
			"id":   found.ScopeID,
		},
	}
	for name, value := range map[string]string{
		"parent_archive_id": found.ParentArchiveID,
		"encryption_key_id": found.EncryptionKeyID,
	} {
		if value != "" {
			out[name] = value
		}
	}
	return out
}
