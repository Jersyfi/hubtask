// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package backup is where an operator says where backups go, and finds out whether they can
// (E-03, backup-restore.md §2).
//
// The authorisation shape here is the substance of the task rather than a detail. A backup target
// is a data egress channel by definition, so `backup-restore.md` §2 makes it a named, narrow
// exception to the SSRF rule: only an instance administrator may create one, tenant-owned targets
// are a switch that is off by default, and the connection probe runs through the same guard every
// outbound call does.
package backup

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

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
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	CreateBackupTargetName = "CreateBackupTarget"
	ListBackupTargetsName  = "ListBackupTargets"
	TestBackupTargetName   = "TestBackupTarget"

	// The two scopes. Reading which targets exist tells somebody where this installation's data
	// goes, which is worth a scope of its own; creating one opens a channel it can go down.
	backupRead   = "backup:read"
	backupManage = "backup:manage"

	targetType = "backup_target"

	// TargetChangedAction is the code backup-restore.md §2 names. A warning rather than an info:
	// nothing is destroyed, and a new place the tenant's data leaves for is exactly the entry an
	// auditor is looking for.
	TargetChangedAction audit.Action = "backup.target_changed"
	// TargetTestedAction is the probe. Info rather than a warning - nothing changes and nothing
	// is destroyed - and recorded all the same, because it writes bytes to a place outside this
	// system and "who was poking at the backup target on Tuesday" is a question with an answer.
	TargetTestedAction audit.Action = "backup.target_tested"
)

// credentialPurpose binds a sealed credential to the row it belongs to, so a ciphertext lifted out
// of one target and written into another no longer opens (E-02).
func credentialPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("backup_target.credential:" + id.String())
}

// Authorizer is the slice of the authorisation service these use cases need.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Writer is what the three use cases share: the shelf, the adapters, the key, and the clock.
//
// One struct rather than four fields repeated three times, for the reason the other contexts
// collect theirs: three use cases over one aggregate that disagreed about which encryptor to use
// would be three chances for a credential to be sealed under a key the others cannot open.
type Writer struct {
	Targets    repository.Targets
	Opener     backupstorage.Opener
	Encryptor  crypto.Encryptor
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Config     env.Config
}

// CreateBackupTarget writes down where backups go.
type CreateBackupTarget struct{ Writer Writer }

// ListBackupTargets reads back what has been written down - without a credential among it.
type ListBackupTargets struct{ Writer Writer }

// TestBackupTarget writes a probe object, reads it back and removes it.
type TestBackupTarget struct{ Writer Writer }

// CreateBackupTargetCommand is the input, typed.
type CreateBackupTargetCommand struct {
	Name                 string
	Kind                 domain.TargetKind
	Config               domain.TargetConfig
	Credentials          map[string]secret.Secret
	EncryptionMode       domain.EncryptionMode
	RegionNote           string
	InsecureAcknowledged bool
}

// Execute writes the target down, with its credential sealed.
func (h CreateBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateBackupTargetCommand,
) (domain.Target, error) {
	// The owner's right, which is the matrix's line for "the one thing an administrator cannot
	// do" (domain-model.md §3.2). In single-tenant operation the tenant's owner *is* the instance
	// administrator backup-restore.md §2 names, and in provider operation the switch below is
	// what stands between a tenant and an egress channel the operator did not choose.
	if err := h.Writer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionDeleteContainer,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetChangedAction,
		TokenScope: backupManage,
		TargetType: targetType,
		TargetID:   actor.TenantID,
	}); err != nil {
		return domain.Target{}, err
	}
	if err := h.Writer.tenantMayConfigureATarget(); err != nil {
		return domain.Target{}, err
	}

	id := h.Writer.IDs.NewID()
	now := h.Writer.Clock.Now()

	target, err := domain.NewTarget(domain.NewTargetInput{
		ID: id, TenantID: actor.TenantID, Name: cmd.Name, Kind: cmd.Kind, Config: cmd.Config,
		EncryptionMode: cmd.EncryptionMode, RegionNote: cmd.RegionNote,
		InsecureAcknowledged: cmd.InsecureAcknowledged, CreatedBy: actor.AccountID, Now: now,
	})
	if err != nil {
		return domain.Target{}, err
	}
	if err := h.Writer.supported(target.Kind); err != nil {
		return domain.Target{}, err
	}

	// Opened before it is stored, so a target that cannot be turned into a connection is a field
	// error now rather than a failed backup at three in the morning. Nothing is written to the
	// target here - opening is configuration, and the probe is a separate act somebody asks for.
	if _, err := h.Writer.Opener.Open(ctx, backupstorage.Spec{
		Kind: target.Kind, Config: target.Config, Credentials: cmd.Credentials,
	}); err != nil {
		return domain.Target{}, err
	}

	sealed, err := h.Writer.seal(ctx, id, cmd.Credentials)
	if err != nil {
		return domain.Target{}, err
	}

	err = h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Writer.Targets.Insert(ctx, target, sealed); err != nil {
			return err
		}
		return h.Writer.recordChange(ctx, actor, target, now)
	})
	if err != nil {
		return domain.Target{}, err
	}
	return target, nil
}

// Execute lists the targets.
func (h ListBackupTargets) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Target, error) {
	// The administrator's line rather than the owner's: knowing where the data goes is part of
	// running the workspace, and somebody who may not create a target may still need to see that
	// one exists and that its last probe failed.
	if err := h.Writer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetChangedAction,
		TokenScope: backupRead,
		TargetType: targetType,
		TargetID:   actor.TenantID,
	}); err != nil {
		return nil, err
	}

	var targets []domain.Target
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		targets, err = h.Writer.Targets.List(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return targets, nil
}

// Probe is what the connection test found.
type Probe struct {
	OK      bool
	Latency time.Duration
	// Writable is whether the write half worked. A target that can be read and not written is a
	// permissions mistake somebody can act on, and it is the common one.
	Writable bool
	// FreeBytes is how much room is left, and nil where the protocol cannot say. A bucket cannot
	// say at all, which is why the contract's field is nullable.
	FreeBytes *int64
	// ErrorCode is why not, as a message code. Never the driver's message: an FTP or SSH library
	// quotes the host, the user and sometimes the password (rule 10, T-18).
	ErrorCode string
}

// Execute writes a probe object, reads it back, and removes it.
//
// Outside a transaction, deliberately and by rule: a target is somebody else's machine, and a
// database transaction waiting on one holds a connection for as long as they feel like taking
// (observability-reliability.md §8). What is read and what is written down are two short
// transactions with the probe in between.
func (h TestBackupTarget) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (Probe, error) {
	if id.IsZero() {
		return Probe{}, shared.ErrValidation.
			WithDetail("backup.target_id_required").
			WithFields(shared.FieldError{Path: "/target_id", Code: "backup.target_id_required"})
	}
	if err := h.Writer.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     TargetTestedAction,
		TokenScope: backupManage,
		TargetType: targetType,
		TargetID:   id,
	}); err != nil {
		return Probe{}, err
	}

	var target domain.Target
	var sealed crypto.Sealed
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if target, err = h.Writer.Targets.Find(ctx, id); err != nil {
			return err
		}
		sealed, err = h.Writer.Targets.Credential(ctx, id)
		return err
	})
	if err != nil {
		return Probe{}, err
	}

	credentials, err := h.Writer.unseal(ctx, id, sealed)
	if err != nil {
		return Probe{}, err
	}

	started := h.Writer.Clock.Now()
	probe := h.Writer.probe(ctx, target, credentials)
	probe.Latency = h.Writer.Clock.Now().Sub(started)

	err = h.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Writer.Targets.RecordTest(
			ctx, id, h.Writer.Clock.Now(), probe.OK, probe.ErrorCode); err != nil {
			return err
		}
		return h.Writer.recordProbe(ctx, actor, target, probe)
	})
	if err != nil {
		return Probe{}, err
	}
	return probe, nil
}

// probe is the write-read-delete the contract describes. It answers a result rather than an error:
// "the target is unreachable" is what the caller asked to find out, not a failure of the call.
func (w Writer) probe(
	ctx context.Context, target domain.Target, credentials map[string]secret.Secret,
) Probe {
	store, err := w.Opener.Open(ctx, backupstorage.Spec{
		Kind: target.Kind, Config: target.Config, Credentials: credentials,
	})
	if err != nil {
		return Probe{ErrorCode: codeOf(err)}
	}

	// A key under the target's own namespace, named so that anybody finding one left behind knows
	// what it was: a probe that was interrupted between the write and the delete.
	key := "hubtask-connection-probe/" + w.IDs.NewID().String()
	content := []byte("hubtask connection probe")

	if _, err := store.Put(ctx, key, bytesReader(content)); err != nil {
		return Probe{ErrorCode: codeOf(err)}
	}
	probe := Probe{Writable: true}

	stream, err := store.Get(ctx, key)
	if err != nil {
		probe.ErrorCode = codeOf(err)
	} else {
		read, readErr := readAll(stream)
		_ = stream.Close()
		switch {
		case readErr != nil:
			probe.ErrorCode = codeOf(readErr)
		case string(read) != string(content):
			// The one failure a status code cannot describe: a target that accepts a write and
			// answers something else is worse than one that refuses, because a backup written to
			// it looks like a backup.
			probe.ErrorCode = "backup.target_returned_other_bytes"
		default:
			probe.OK = true
		}
	}

	// Removed whatever happened. A probe that left its object behind would litter the target with
	// one per test, and an operator counting archives would count them.
	if err := store.Delete(ctx, key); err != nil && probe.ErrorCode == "" {
		probe.OK, probe.ErrorCode = false, codeOf(err)
	}

	if reporter, answers := store.(backupstorage.SpaceReporter); answers {
		if free, err := reporter.FreeBytes(ctx); err == nil {
			probe.FreeBytes = &free
		}
	}
	return probe
}

// tenantMayConfigureATarget is the switch backup-restore.md §2 names.
//
// It has no meaning in single-tenant operation: there the tenant's owner is the person running the
// installation, and there is nobody the switch could be protecting. In provider operation it is
// the difference between an egress channel the operator chose and one a customer did.
func (w Writer) tenantMayConfigureATarget() error {
	if w.Config.Tenancy != env.TenancyMulti || w.Config.Backup.TenantTargets {
		return nil
	}
	return shared.ErrForbidden.WithDetail("backup.tenant_targets_disabled")
}

// supported refuses a kind this build has no adapter for, in the words a client can act on.
func (w Writer) supported(kind domain.TargetKind) error {
	if slices.Contains(w.Opener.Kinds(), kind) {
		return nil
	}
	return shared.ErrValidation.
		WithDetail(backupstorage.CodeKindUnsupported).
		WithParams(map[string]string{"kind": kind.String()}).
		WithFields(shared.FieldError{
			Path:   "/kind",
			Code:   backupstorage.CodeKindUnsupported,
			Params: map[string]string{"kind": kind.String()},
		})
}

// seal turns the credential map into one sealed value, bound to the target it belongs to.
func (w Writer) seal(
	ctx context.Context, id shared.ID, credentials map[string]secret.Secret,
) (crypto.Sealed, error) {
	if len(credentials) == 0 {
		// A target that needs no credential - a local directory, a public WebDAV - stores none,
		// and stores no key identifier either. An empty ciphertext is honest about that where a
		// sealed empty map would claim there is something to open.
		return crypto.Sealed{}, nil
	}

	plain := make(map[string]string, len(credentials))
	for name, value := range credentials {
		plain[name] = value.Reveal()
	}
	encoded, err := json.Marshal(plain)
	if err != nil {
		return crypto.Sealed{}, shared.ErrInternal.WithDetail("backup.credentials_unserialisable")
	}

	return w.Encryptor.Seal(ctx, secret.New(string(encoded)), credentialPurpose(id))
}

// unseal is the only place a stored credential becomes readable, and the only caller is the probe.
func (w Writer) unseal(
	ctx context.Context, id shared.ID, sealed crypto.Sealed,
) (map[string]secret.Secret, error) {
	if sealed.IsZero() {
		return nil, nil
	}

	opened, err := w.Encryptor.Open(ctx, sealed, credentialPurpose(id))
	if err != nil {
		return nil, err
	}

	var plain map[string]string
	if err := json.Unmarshal([]byte(opened.Reveal()), &plain); err != nil {
		return nil, shared.ErrInternal.WithDetail("backup.credentials_unreadable")
	}

	credentials := make(map[string]secret.Secret, len(plain))
	for name, value := range plain {
		credentials[name] = secret.New(value)
	}
	return credentials, nil
}

// recordChange writes the entry backup-restore.md §2 asks for. The configuration is recorded and
// the credential is not: what an auditor needs is where the data may now go, and a bucket name is
// that answer while a secret key is not.
func (w Writer) recordChange(
	ctx context.Context, actor appshared.ActorContext, target domain.Target, now time.Time,
) error {
	changes := []audit.Change{
		{Field: "kind", Classification: audit.Open, To: target.Kind.String()},
		{Field: "encryption_mode", Classification: audit.Open, To: target.EncryptionMode.String()},
	}
	if warnings := target.Warnings(); len(warnings) > 0 {
		// Who accepted an insecure target is the entry somebody looks for afterwards, and it is
		// the reason the acknowledgement is a person and a moment rather than a flag.
		changes = append(changes, audit.Change{
			Field: "insecure_acknowledged", Classification: audit.Open,
			To: strings.Join(warnings, " "),
		})
	}
	for _, setting := range slices.Sorted(maps.Keys(target.Config)) {
		changes = append(changes, audit.Change{
			Field: "config." + setting, Classification: audit.Open,
			To: target.Config.Get(setting),
		})
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now,
		Action: TargetChangedAction, Outcome: audit.OutcomeSuccess,
		Severity:  audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: targetType, TargetID: target.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// recordProbe writes down that somebody checked, and what came back.
func (w Writer) recordProbe(
	ctx context.Context, actor appshared.ActorContext, target domain.Target, probe Probe,
) error {
	outcome := audit.OutcomeSuccess
	if !probe.OK {
		outcome = audit.OutcomeFailed
	}

	return w.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: w.Clock.Now(),
		Action: TargetTestedAction, Outcome: outcome, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: targetType, TargetID: target.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// codeOf is what a target's failure is written down as: a message code, never a message.
func codeOf(err error) string {
	if code := shared.AsError(err).DetailCode; code != "" {
		return code
	}
	if errors.Is(err, shared.ErrNotFound) {
		return backupstorage.CodeObjectNotFound
	}
	return backupstorage.CodeTargetFailed
}

// bytesReader and readAll keep the two io calls this file makes in one place. The application
// layer streams to a port rather than importing a transport, and these are the smallest possible
// bridge between a byte slice and a stream.
func bytesReader(content []byte) io.Reader { return bytes.NewReader(content) }

func readAll(from io.Reader) ([]byte, error) {
	// Bounded: a probe wrote twenty-four bytes, and a target answering more than that is either
	// broken or hostile. Reading whatever it sends would be the first place a target could make
	// this process allocate (T-17).
	return io.ReadAll(io.LimitReader(from, 4096))
}

// Descriptor registers the creation in all three channels.
func (h CreateBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateBackupTargetName,
		Summary: "Writes down where backups go: the kind of target, where it points, and what it " +
			"authenticates with. Credentials are stored encrypted and are never returned by any " +
			"read. Storing an archive unencrypted, or reaching a target over a protocol that " +
			"carries bytes in the clear, both need insecure_acknowledged - and both stay visible " +
			"as a warning on the target afterwards. Needs the owner's right, because a backup " +
			"target is a channel the tenant's data leaves by.",
		SideEffects: "Writes the target, seals its credentials, and writes an audit entry. " +
			"Nothing is written to the target itself - use the connection test for that.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "name", Kind: usecase.KindString, Required: true,
				Description: "What an operator picks this target out by. Unique per tenant, " +
					"regardless of case.",
			},
			{
				Name: "kind", Kind: usecase.KindString, Required: true,
				Description: "The protocol. This build speaks LOCAL, S3, SFTP and WEBDAV; the " +
					"other names in the contract are refused until an adapter exists for them.",
			},
			{
				Name: "config", Kind: usecase.KindObject, Required: true,
				Description: "Where the target points: bucket, endpoint, region, host, path, url. " +
					"Names and numbers, never a credential.",
			},
			{
				Name: "credentials", Kind: usecase.KindObject,
				Description: "What the target authenticates with: access_key and secret_key, " +
					"password, private_key. Stored encrypted and never answered.",
			},
			{
				Name: "encryption_mode", Kind: usecase.KindString,
				Enum:        []string{"AES256_GCM", "NONE"},
				Description: "Whether the archive is encrypted before it leaves. AES256_GCM unless said otherwise.",
			},
			{
				Name: "region_note", Kind: usecase.KindString,
				Description: "Where the bytes physically are. Prose for an auditor; nothing enforces it.",
			},
			{
				Name: "insecure_acknowledged", Kind: usecase.KindBool,
				Description: "Required for an unencrypted target or a plaintext protocol. Who " +
					"gave it and when are recorded.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TargetChangedAction, TargetType: targetType,
			Severity: audit.SeverityWarning, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor registers the listing.
func (h ListBackupTargets) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListBackupTargetsName,
		Summary: "Lists the targets configured for this tenant, by name. Never a credential: what " +
			"comes back is where a target points, how it is encrypted, what it warns about, and " +
			"how its last connection test went.",
		SideEffects: "None. Reads only.",
		TokenScope:  backupRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: TargetChangedAction, TargetType: targetType,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor registers the connection test.
func (h TestBackupTarget) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: TestBackupTargetName,
		Summary: "Writes a small probe object to the target, reads it back, and removes it. " +
			"Answers whether it worked, how long it took, whether the target is writable, how " +
			"much room is left where the protocol can say, and a message code when it did not " +
			"work. A target that is unreachable is an answer rather than an error.",
		SideEffects: "Writes and removes one probe object at the target, records the result on " +
			"the target, and writes an audit entry.",
		TokenScope: backupManage,
		Input: []usecase.Field{
			{
				Name: "target_id", Kind: usecase.KindID, Required: true,
				Description: "The target to check.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TargetTestedAction, TargetType: targetType,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	target, err := h.Execute(ctx, actor, CreateBackupTargetCommand{
		Name:                 in.String("name"),
		Kind:                 domain.TargetKind(in.String("kind")),
		Config:               settingsOf(in, "config"),
		Credentials:          credentialsOf(in, "credentials"),
		EncryptionMode:       domain.EncryptionMode(in.String("encryption_mode")),
		RegionNote:           in.String("region_note"),
		InsecureAcknowledged: in.Bool("insecure_acknowledged"),
	})
	if err != nil {
		return nil, err
	}
	return targetOutput(target), nil
}

func (h ListBackupTargets) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	targets, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(targets))
	for _, target := range targets {
		rows = append(rows, targetOutput(target))
	}
	return usecase.Output{"data": rows}, nil
}

func (h TestBackupTarget) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("target_id")
	if err != nil {
		return nil, err
	}
	probe, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}

	out := usecase.Output{
		"ok":         probe.OK,
		"latency_ms": probe.Latency.Milliseconds(),
		"writable":   probe.Writable,
	}
	if probe.FreeBytes != nil {
		out["free_bytes"] = *probe.FreeBytes
	}
	if probe.ErrorCode != "" {
		out["error_code"] = probe.ErrorCode
	}
	return out, nil
}

// targetOutput is a target as the three channels answer it. There is no credential in here and
// nowhere for one to go, which is the point rather than an omission.
func targetOutput(target domain.Target) usecase.Output {
	config := map[string]any{}
	for name, value := range target.Config {
		config[name] = value
	}

	out := usecase.Output{
		"id":              target.ID.String(),
		"name":            target.Name,
		"kind":            target.Kind.String(),
		"scope":           scopeOf(target),
		"config":          config,
		"encryption_mode": target.EncryptionMode.String(),
		"enabled":         target.Enabled,
		"warnings":        target.Warnings(),
	}
	if target.EncryptionKeyID != "" {
		out["encryption_key_id"] = target.EncryptionKeyID
	}
	if target.RegionNote != "" {
		out["region_note"] = target.RegionNote
	}
	if !target.LastTestAt.IsZero() {
		out["last_test_at"] = target.LastTestAt
	}
	if target.LastTestOK != nil {
		out["last_test_ok"] = *target.LastTestOK
	}
	return out
}

func scopeOf(target domain.Target) string {
	if target.IsInstanceWide() {
		return "INSTANCE"
	}
	return "TENANT"
}

// settingsOf narrows the contract's `additionalProperties: true` to the scalars a target's
// configuration is made of. A nested document is a shape nothing reads and a place for something
// to hide; a number and a flag are spelled out, because a client that sends `"port": 22` means the
// same thing as one that sends `"22"`.
func settingsOf(in usecase.Input, field string) domain.TargetConfig {
	settings := domain.TargetConfig{}
	for name, value := range objectOf(in, field) {
		if text, ok := scalarText(value); ok {
			settings[name] = text
		}
	}
	return settings
}

// credentialsOf is the same narrowing for the secret half, and every value is wrapped on the way
// in - so a credential is in a masking type from the first line of the application layer onwards
// (T-18).
func credentialsOf(in usecase.Input, field string) map[string]secret.Secret {
	credentials := map[string]secret.Secret{}
	for name, value := range objectOf(in, field) {
		if text, ok := scalarText(value); ok && text != "" {
			credentials[name] = secret.New(text)
		}
	}
	if len(credentials) == 0 {
		return nil
	}
	return credentials
}

func objectOf(in usecase.Input, field string) map[string]any {
	object, _ := in[field].(map[string]any)
	return object
}

// scalarText turns the three JSON scalars into the string a configuration holds. Anything else -
// an object, an array, a null - is dropped rather than stringified, so a target's configuration
// cannot end up holding "map[a:1]".
func scalarText(value any) (string, bool) {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed), true
	case bool:
		return strconv.FormatBool(typed), true
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64), true
	case int:
		return strconv.Itoa(typed), true
	case int64:
		return strconv.FormatInt(typed, 10), true
	case json.Number:
		return typed.String(), true
	default:
		return "", false
	}
}
