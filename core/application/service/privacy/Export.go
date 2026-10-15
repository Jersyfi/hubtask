// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Jersyfi/hubtask/core/application/archive"
	backuprepo "github.com/Jersyfi/hubtask/core/application/repository/backup"
	repository "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	backupservice "github.com/Jersyfi/hubtask/core/application/service/backup"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/backupstorage"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The access and portability export (Art. 15 and 20).
//
// It is **not a new format**. `backup-restore.md` §9 settled that: an export is a Hubtask archive,
// "so an export is therefore simultaneously a restorable backup, without a second format coming
// into existence". So this writes the backup archive, through the backup's writer, with one thing
// changed - the source hands over the person's rows rather than the tenant's.

const (
	// ExportedAction is what the trail records when a copy of somebody's data is written. A
	// warning: this is the moment a person's data leaves the installation, and "who took a copy of
	// whose data, and where to" is a question with an answer.
	ExportedAction audit.Action = "dsr.exported"

	// CollectedAction is the entry an installation-wide case leaves **in every workspace it
	// touches**. audit.md §5: an instance administrator has no blanket insight into a tenant's
	// data without a documented occasion, and that occasion is itself audited - in the tenant
	// whose data was read, where its own administrator can see it.
	CollectedAction audit.Action = "dsr.collected"
)

// TargetStore opens a configured backup target. The same seam the audit export writes through: an
// export needs somewhere to put bytes and has no business with a target's credentials.
type TargetStore interface {
	OpenTarget(ctx context.Context, tenantID, targetID shared.ID) (backupstorage.Store, error)
}

// Accounts finds the person in one workspace by their address, inside that workspace's own
// transaction - the identity repository's question, asked once per workspace an installation-wide
// case visits.
type Accounts interface {
	FindByEmail(ctx context.Context, email string) (identity.Account, error)
}

// Exporter writes the archive an access or portability case produces.
type Exporter struct {
	Requests repository.Requests
	Subjects repository.Subjects
	// Accounts is how an installation-wide case knows the person in each workspace by their
	// account, so that their entries and comments - which name an account, not an address - are in
	// the archive too.
	Accounts Accounts
	Targets  TargetStore
	// Rows is the tenant's rows as the archive writer needs them - the same reader a backup uses,
	// because this is the same archive.
	Rows       backuprepo.Export
	Objects    storage.ObjectStore
	Cipher     crypto.StreamCipher
	Audit      audit.Sink
	Snapshot   persistence.Snapshot
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// SchemaVersion and ProductVersion go into the manifest, as they do for a backup: what an
	// archive has to record is the build that wrote it.
	SchemaVersion  string
	ProductVersion string
}

// Written is what one export produced.
type Written struct {
	// Archive is where it landed at the target.
	Archive string
	// Tenants is how many workspaces were collected from: one for an ordinary case, and as many as
	// the person is a member of for an installation-wide one.
	Tenants int
	Records int
}

// Export writes the archive for a case.
//
// An installation-wide case is a loop rather than a wider query: one workspace at a time, each read
// under that workspace's own tenant context, through the ordinary repositories. `SET LOCAL
// app.tenant_id` is never relaxed and no repository method takes a tenant argument - what crossed
// the boundary was a list of identifiers, and nothing else (data-protection.md §4). What it writes
// is still one archive (UC-PRV-06 check 3): a bundle whose members are the workspaces' archives.
func (e Exporter) Export(
	ctx context.Context, actor appshared.ActorContext, request domain.Request,
) (Written, error) {
	if request.TargetID.IsZero() {
		return Written{}, shared.ErrConflict.WithDetail(domain.CodeExportTargetRequired)
	}

	scopes, err := e.workspaces(ctx, actor, request)
	if err != nil {
		return Written{}, err
	}
	target, err := e.Targets.OpenTarget(ctx, actor.TenantID, request.TargetID)
	if err != nil {
		return Written{}, err
	}

	if request.Scope != domain.ScopeInstallation {
		own := scopes[0]
		prefix := ArchiveName(own.subjectID, own.email, e.Clock.Now())
		manifest, err := e.writeOne(ctx, actor, target, prefix, prefix, own, request)
		if err != nil {
			return Written{}, err
		}
		return Written{Archive: prefix, Tenants: 1, Records: manifest.Records}, nil
	}

	name := BundleName(request.ID, e.Clock.Now())
	outer := openBundle(ctx, target, name)
	written := Written{Archive: name, Tenants: len(scopes)}
	for _, workspace := range scopes {
		manifest, err := e.writeOne(ctx, actor, outer, workspace.tenantID.String(), name, workspace, request)
		if err != nil {
			outer.Abort(err)
			return Written{}, err
		}
		written.Records += manifest.Records
	}
	if err := outer.Close(); err != nil {
		return Written{}, err
	}
	return written, nil
}

// workspace is one tenant to collect from, and how the person is known in it.
type workspace struct {
	tenantID  shared.ID
	subjectID shared.ID
	email     string
}

// workspaces answers which tenants this case reaches.
//
// For an ordinary case, the caller's own and nothing else. For an installation-wide one, every
// workspace the address is a member of - the one cross-tenant question in the system, asked of a
// function that answers tenant identifiers and nothing else.
func (e Exporter) workspaces(
	ctx context.Context, actor appshared.ActorContext, request domain.Request,
) ([]workspace, error) {
	own := workspace{
		tenantID: actor.TenantID, subjectID: request.SubjectAccountID, email: request.SubjectEmail,
	}
	if request.Scope != domain.ScopeInstallation {
		return []workspace{own}, nil
	}
	// No scope is asked for here. The case reached this job only through its start, which asked
	// the person who started it for `admin:tenants` (as recording it did); the job acts as the
	// installation, and an installation's actor holding the operator's scope would hand it to every
	// job there is.
	if request.SubjectEmail == "" {
		// An installation-wide case needs an address: an account identifier belongs to one
		// workspace by construction, so it cannot name the person anywhere else.
		return nil, shared.ErrValidation.
			WithDetail(domain.CodeSubjectRequired).
			WithFields(shared.FieldError{Path: "/subject_email", Code: domain.CodeSubjectRequired})
	}

	var tenants []shared.ID
	err := e.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		tenants, err = e.Subjects.Tenants(ctx, request.SubjectEmail)
		return err
	})
	if err != nil {
		return nil, err
	}

	scopes := make([]workspace, 0, len(tenants))
	for _, tenantID := range tenants {
		subject, err := e.accountIn(ctx, tenantID, request.SubjectEmail)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, workspace{tenantID: tenantID, subjectID: subject, email: request.SubjectEmail})
	}
	if len(scopes) == 0 {
		// Nobody by that address anywhere. An empty answer rather than an error: "this
		// installation holds nothing of theirs" is what the person asked to find out.
		return []workspace{own}, nil
	}
	return scopes, nil
}

// accountIn answers the person's account in one workspace, read under that workspace's context.
//
// None is an answer rather than an error: the account may have gone between the cross-workspace
// question and this one, and the collection then goes on by the address, as it did before.
func (e Exporter) accountIn(ctx context.Context, tenantID shared.ID, email string) (shared.ID, error) {
	if e.Accounts == nil {
		return "", nil
	}
	var found shared.ID
	err := e.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		account, err := e.Accounts.FindByEmail(ctx, email)
		if errors.Is(err, shared.ErrNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		found = account.ID
		return nil
	})
	return found, err
}

// exported is what one workspace's archive came to.
type exported struct {
	prefix  string
	Records int
}

// writeOne writes the archive of one workspace.
//
// prefix is where its files land in store; archiveName is what the case and the trail know it
// by - the same for a case of one workspace, the bundle's for an installation-wide one.
func (e Exporter) writeOne(
	ctx context.Context, actor appshared.ActorContext, store backupstorage.Store,
	prefix, archiveName string, in workspace, request domain.Request,
) (exported, error) {
	scope := persistence.Scope{TenantID: in.tenantID}
	written := exported{prefix: archiveName}

	err := e.Snapshot.WithinSnapshot(ctx, scope, func(snapshotCtx context.Context, at time.Time) error {
		source := subjectSource{
			// Redacted for the tenant export's reason (tenant-export.md §9): this archive
			// leaves unencrypted, and a person's own password hash is still a credential of
			// this installation - handing it out invites offline guessing against it.
			inner:   backupservice.Redacted(backupservice.ExportSource{Export: e.Rows, SnapshotAt: at}),
			subject: in.subjectID,
			email:   in.email,
			counted: &written.Records,
		}
		media := backupservice.ExportMedia{Export: e.Rows, Objects: e.Objects}

		_, err := archive.NewWriter(store, e.Cipher, media).Write(snapshotCtx, archive.Request{
			ArchiveID: e.IDs.NewID(), Prefix: prefix,
			Scope:      archive.Scope{Kind: archive.ScopeTenant, ID: in.tenantID.String()},
			Mode:       archive.ModeFull,
			SnapshotAt: at,
			// Unencrypted, as `backup-restore.md` §9 allows: the archive is handed to the person
			// it is about, and a copy they cannot open is not a copy of their data. The target is
			// the operator's own and the transfer to the person is the operator's act.
			Encryption:     archive.Encryption{Mode: archive.EncryptionNone},
			SchemaVersion:  e.SchemaVersion,
			ProductVersion: e.ProductVersion,
			IncludeMedia:   true,
			// The person's own audit entries are part of what is held about them, and they are
			// personal data: an access request that left them out would be incomplete.
			IncludeAudit: true,
		}, source)
		return err
	})
	if err != nil {
		return exported{}, err
	}

	if err := e.recordCollection(ctx, actor, in, request, written); err != nil {
		return exported{}, err
	}
	return written, nil
}

// recordCollection writes the entry the collection owes, in the workspace it read.
//
// audit.md §5: an instance administrator has no blanket insight into a tenant's data without a
// documented occasion, and that occasion is itself audited. So the entry goes into the workspace
// whose data was read - where its own administrator can see it - and names the case rather than
// the person.
func (e Exporter) recordCollection(
	ctx context.Context, actor appshared.ActorContext, in workspace,
	request domain.Request, written exported,
) error {
	action := ExportedAction
	if in.tenantID != actor.TenantID {
		action = CollectedAction
	}

	scope := persistence.Scope{TenantID: in.tenantID}
	return e.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return e.Audit.Append(ctx, audit.Entry{
			TenantID: in.tenantID, OccurredAt: e.Clock.Now(),
			Action: action, Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: requestTarget, TargetID: request.ID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(
				audit.Change{Field: "archive", Classification: audit.Open, To: written.prefix},
				audit.Change{Field: "records", Classification: audit.Open, To: written.Records},
				audit.Change{Field: "scope", Classification: audit.Open, To: string(request.Scope)},
			),
			LegalBasis: LegalBasisOf(request.Kind),
		})
	})
}

// BundleName is the one object an installation-wide case writes. Named by the case, never by the
// person: the name is in every workspace's trail, and an address there would be personal data in
// an audit entry (rule 10).
func BundleName(caseID shared.ID, at time.Time) string {
	return fmt.Sprintf("hubtask-dsr-%s-%s.zip", caseID.String(), at.UTC().Format("20060102T150405Z"))
}

// ArchiveName is the directory one export lives in at a target.
//
// `hubtask-dsr-` rather than `hubtask-backup-`, and that is deliberate on both counts. It is a
// Hubtask archive and a restore can be pointed at it by name (`archive_id`); it is **not** a backup
// of the workspace, and the listing at a target keeps what starts with `hubtask-backup-<tenant>-`,
// so an export never appears among the backups an operator restores from by accident.
func ArchiveName(subjectID shared.ID, email string, at time.Time) string {
	who := subjectID.String()
	if who == "" {
		who = strings.NewReplacer("@", "-at-", ".", "-").Replace(email)
	}
	return fmt.Sprintf("hubtask-dsr-%s-%s", who, at.UTC().Format("20060102T150405Z"))
}
