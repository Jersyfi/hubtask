// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/audit"
	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	backupdomain "github.com/Jersyfi/hubtask/core/domain/model/backup"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	port "github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// External anchoring (A-2, P-13, audit.md §3): the chain proves tampering inside the database, and
// only a copy of its end kept outside says anything against somebody who can rewrite the whole of
// it. Once a day a job reads the chain's end and writes it to a backup target the workspace named,
// and `:verify` reads the last copy back and compares.

const (
	ConfigureAuditAnchoringName = "ConfigureAuditAnchoring"
	// AnchoringConfiguredAction is the entry the configuration writes, with the target before
	// and after: where the chain's end may leave the installation from now on.
	AnchoringConfiguredAction port.Action = "audit.anchoring_configured"
	// anchoringTarget is what the entry is about: the workspace's own anchoring, not a target.
	anchoringTarget = "audit_anchoring"
	// anchorPrefix is the key an anchor is written under at the target: one object per tenant and
	// day, named so that anybody finding one knows what it is.
	anchorPrefix = "hubtask-anchor-"
	// anchorFormatVersion is what the file says about its own shape.
	anchorFormatVersion = 1
	// anchorMaxSize bounds a read-back: an anchor is a few hundred bytes, and an object of a
	// megabyte under its name is not one.
	anchorMaxSize = 64 << 10

	// The reasons a read-back answers nothing, by code.
	CodeAnchorUnreadable      = "audit.anchor_unreadable"
	CodeAnchorReceiptMismatch = "audit.anchor_receipt_mismatch"
)

// AnchorDocument is the file at the target: the chain's end, when it was taken, and the build that
// wrote it. JSON, because it is read by people and by other tools as much as by this system.
type AnchorDocument struct {
	FormatVersion  int       `json:"format_version"`
	TenantID       string    `json:"tenant_id"`
	LastSeq        int64     `json:"last_seq"`
	ChainHash      string    `json:"chain_hash"`
	AnchoredAt     time.Time `json:"anchored_at"`
	ProductVersion string    `json:"product_version"`
}

// AnchorKey is where a tenant's anchor of one day lives at the target.
func AnchorKey(tenantID shared.ID, day time.Time) string {
	return anchorPrefix + tenantID.String() + "-" + day.UTC().Format("20060102") + ".json"
}

// TargetFinder is the one question the configuration asks about a target: is it one of this
// workspace's. The port bounds the answer to the tenant.
type TargetFinder interface {
	Find(ctx context.Context, id shared.ID) (backupdomain.Target, error)
}

// Anchoring is what the configuration and the job share.
type Anchoring struct {
	Workspaces identityrepo.Workspaces
	Targets    TargetFinder
	Trail      repository.Trail
	Anchors    repository.Anchors
	// Stores opens the target the workspace named, for the job's write and the check's read.
	Stores     TargetStore
	Jobs       Enqueuer
	Authorizer Authorizer
	Audit      port.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// ProductVersion goes into the file, for the manifest's reason.
	ProductVersion string
}

// Configuration is what stands after a configuration.
type Configuration struct {
	TargetID     shared.ID
	ConfiguredAt time.Time
	ConfiguredBy shared.ID
}

// ConfigureAuditAnchoring names the target, or switches anchoring off.
type ConfigureAuditAnchoring struct{ Anchoring Anchoring }

// Execute writes the setting, records it, and seeds the daily job.
func (h ConfigureAuditAnchoring) Execute(
	ctx context.Context, actor appshared.ActorContext, targetID shared.ID,
) (Configuration, error) {
	a := h.Anchoring
	if err := a.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []identity.Scope{identity.TenantScope()},
		Action:     AnchoringConfiguredAction,
		TokenScope: workspaceManage,
		TargetType: anchoringTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return Configuration{}, err
	}

	now := a.Clock.Now()
	err := a.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if !targetID.IsZero() {
			// One of the workspace's own targets, read through the port that bounds it to the
			// tenant: a target another workspace configured is not found here.
			if _, err := a.Targets.Find(ctx, targetID); err != nil {
				return err
			}
		}
		workspace, err := a.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		before := workspace.Settings.AuditAnchorTargetID
		if before == targetID {
			return nil
		}
		workspace.Settings.AuditAnchorTargetID = targetID
		written, err := a.Workspaces.Update(ctx, workspace, workspace.Version, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrConflict.WithDetail("workspace.version_conflict")
		}
		if !targetID.IsZero() {
			if err := a.wake(ctx, actor.TenantID, now); err != nil {
				return err
			}
		}
		return a.Audit.Append(ctx, port.Entry{
			TenantID: actor.TenantID, OccurredAt: now,
			Action: AnchoringConfiguredAction, Outcome: port.OutcomeSuccess,
			Severity:  port.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: anchoringTarget, TargetID: actor.TenantID,
			Context: port.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: port.Changes(port.Change{
				Field: "target_id", Classification: port.Open,
				From: before.String(), To: targetID.String(),
			}),
		})
	})
	if err != nil {
		return Configuration{}, err
	}
	return Configuration{TargetID: targetID, ConfiguredAt: now, ConfiguredBy: actor.AccountID}, nil
}

// wake seeds or pulls forward this tenant's anchoring job: one per tenant, rescheduling itself,
// seeded by the write that made something owed - the shape every per-tenant job has, because
// nothing in this system may enumerate tenants.
func (a Anchoring) wake(ctx context.Context, tenantID shared.ID, at time.Time) error {
	_, err := a.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindAuditAnchor, TenantID: tenantID, DedupeKey: tenantID.String(), RunAt: at.UTC(),
	})
	return err
}

// Outcome is what one round of the job did, and when the next is owed. The zero time is a
// workspace that names no target: the job finishes, and the next configuration re-seeds it.
type Outcome struct {
	Anchored bool
	LastSeq  int64
	NextDue  time.Time
}

// Run does one round for a tenant: reads the chain's end, writes today's anchor where there is
// something new to anchor, records the row, and answers when to come back.
//
// One file a day, and only where the chain moved: an anchor of a sequence number already anchored
// would be a second row the primary key refuses, and a day on which nothing happened has nothing
// to seal that yesterday's file does not already seal.
func (a Anchoring) Run(ctx context.Context, tenantID shared.ID) (Outcome, error) {
	now := a.Clock.Now()
	scope := persistence.Scope{TenantID: tenantID}

	var (
		targetID shared.ID
		end      repository.ChainEnd
		last     repository.Anchor
	)
	err := a.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		workspace, err := a.Workspaces.Find(ctx)
		if err != nil {
			return err
		}
		targetID = workspace.Settings.AuditAnchorTargetID
		if targetID.IsZero() {
			return nil
		}
		if end, err = a.Trail.ChainEnd(ctx); err != nil {
			return err
		}
		last, err = a.Trail.LatestAnchor(ctx)
		return err
	})
	if err != nil {
		return Outcome{}, err
	}
	if targetID.IsZero() {
		return Outcome{}, nil
	}
	next := nextAnchorDue(now)
	if end.LastSeq == 0 || end.LastSeq == last.LastSeq || sameDay(last.AnchoredAt, now) {
		return Outcome{LastSeq: last.LastSeq, NextDue: next}, nil
	}

	// The write, outside any transaction: the target is somebody else's machine.
	document, err := json.Marshal(AnchorDocument{
		FormatVersion: anchorFormatVersion, TenantID: tenantID.String(),
		LastSeq: end.LastSeq, ChainHash: hex.EncodeToString(end.Hash),
		AnchoredAt: now.UTC(), ProductVersion: a.ProductVersion,
	})
	if err != nil {
		return Outcome{}, shared.Internalf("audit: encoding an anchor: %w", err)
	}
	store, err := a.Stores.OpenTarget(ctx, tenantID, targetID)
	if err != nil {
		return Outcome{}, err
	}
	if _, err := store.Put(ctx, AnchorKey(tenantID, now), bytes.NewReader(document)); err != nil {
		return Outcome{}, err
	}
	receipt := digestOf(document)

	err = a.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return a.Anchors.Record(ctx, repository.Anchor{
			AnchoredAt: now, LastSeq: end.LastSeq, ChainHash: end.Hash,
			Destination: targetID.String(), Receipt: receipt,
		})
	})
	if err != nil && !errors.Is(err, shared.ErrConflict) {
		return Outcome{}, err
	}
	return Outcome{Anchored: true, LastSeq: end.LastSeq, NextDue: next}, nil
}

// nextAnchorDue is the next day's moment: shortly after midnight UTC, so that a day's anchor is
// the chain as the day ended for everybody rather than as it ended in one zone.
func nextAnchorDue(now time.Time) time.Time {
	day := now.UTC().Truncate(24 * time.Hour)
	return day.Add(24*time.Hour + 5*time.Minute)
}

func sameDay(a, b time.Time) bool {
	if a.IsZero() || b.IsZero() {
		return false
	}
	return a.UTC().Truncate(24*time.Hour) == b.UTC().Truncate(24*time.Hour)
}

func digestOf(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// AnchorCheck is what reading the last anchor back found (P-13): the moment and the sequence
// number it sealed, whether the external copy holds the chain end this database computes there,
// and why not where it could not be compared.
type AnchorCheck struct {
	Configured bool
	AnchoredAt time.Time
	LastSeq    int64
	// Agrees is nil where there was nothing to compare: no anchor, or a copy that could not be
	// read.
	Agrees    *bool
	ErrorCode string
}

// readBack reads the last anchor's copy from the target and compares it with the chain end the
// verification computed at that sequence - the recomputed hash where the walk covered it, and the
// stored hash otherwise.
//
// A disagreement is a chain rewritten below the anchor, or a copy somebody replaced: either way
// the external copy and the database no longer tell the same story, which is exactly the question
// an anchor exists to answer.
func (a Anchoring) readBack(
	ctx context.Context, tenantID shared.ID, anchor repository.Anchor, computed []byte,
) AnchorCheck {
	check := AnchorCheck{Configured: true, AnchoredAt: anchor.AnchoredAt, LastSeq: anchor.LastSeq}
	destination, err := shared.ParseID(anchor.Destination)
	if err != nil {
		check.ErrorCode = CodeAnchorUnreadable
		return check
	}
	store, err := a.Stores.OpenTarget(ctx, tenantID, destination)
	if err != nil {
		check.ErrorCode = CodeAnchorUnreadable
		return check
	}
	stream, err := store.Get(ctx, AnchorKey(tenantID, anchor.AnchoredAt))
	if err != nil {
		check.ErrorCode = CodeAnchorUnreadable
		return check
	}
	content, err := io.ReadAll(io.LimitReader(stream, anchorMaxSize+1))
	_ = stream.Close()
	if err != nil || len(content) > anchorMaxSize {
		check.ErrorCode = CodeAnchorUnreadable
		return check
	}
	if digestOf(content) != anchor.Receipt {
		check.ErrorCode = CodeAnchorReceiptMismatch
		return check
	}
	var document AnchorDocument
	if err := json.Unmarshal(content, &document); err != nil || document.LastSeq != anchor.LastSeq {
		check.ErrorCode = CodeAnchorReceiptMismatch
		return check
	}
	held, err := hex.DecodeString(document.ChainHash)
	if err != nil {
		check.ErrorCode = CodeAnchorReceiptMismatch
		return check
	}
	agrees := bytes.Equal(held, computed)
	check.Agrees = &agrees
	return check
}

// Descriptor registers the configuration in all three channels.
func (h ConfigureAuditAnchoring) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ConfigureAuditAnchoringName,
		Summary: "Names the workspace's backup target the audit chain's end is anchored to once " +
			"a day - the last sequence number and its hash, written as a small file the " +
			"verification reads back and compares - or switches anchoring off. The target is one " +
			"of the workspace's own; what is recommended for a tenant's target applies to it, and " +
			"an anchor is small enough that the lock can be long. Needs STRUCTURE at the workspace.",
		SideEffects: "Writes the setting, writes an audit entry with the target before and after, " +
			"and seeds the daily anchoring job.",
		TokenScope: workspaceManage,
		Input: []usecase.Field{
			{Name: "target_id", Kind: usecase.KindID,
				Description: "The backup target the daily anchor is written to. Omitted or null " +
					"switches anchoring off; the anchors already written stay."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AnchoringConfiguredAction, TargetType: anchoringTarget,
			Severity: port.SeverityNotice, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ConfigureAuditAnchoring) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	var targetID shared.ID
	if in.Present("target_id") && in.String("target_id") != "" {
		parsed, err := in.ID("target_id")
		if err != nil {
			return nil, err
		}
		targetID = parsed
	}
	configured, err := h.Execute(ctx, actor, targetID)
	if err != nil {
		return nil, err
	}
	return ConfigurationOutput(configured), nil
}

// ConfigurationOutput is the answer as every channel renders it.
func ConfigurationOutput(configured Configuration) usecase.Output {
	out := usecase.Output{
		"target_id":     nil,
		"configured_at": configured.ConfiguredAt.UTC(),
		"configured_by": nil,
	}
	if !configured.TargetID.IsZero() {
		out["target_id"] = configured.TargetID.String()
	}
	if !configured.ConfiguredBy.IsZero() {
		out["configured_by"] = configured.ConfiguredBy.String()
	}
	return out
}

// workspaceManage is the scope a configuration of the workspace asks for, the same one the
// workspace's own settings ask for.
const workspaceManage = "workspace:manage"
