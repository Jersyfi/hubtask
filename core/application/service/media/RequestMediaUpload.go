// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package media is the upload life of a file: staged, carried to storage by somebody other than
// this server, confirmed, and eventually reclaimed (C-06, arc42 §8.4).
//
// The three steps exist because the server does not carry the bytes. A client asks where to put
// them, puts them there, and says it is done; only then does the record become usable. What that
// buys is the whole point of §8.4 - an upload never occupies a request handler for its duration -
// and what it costs is that a record can exist without its bytes, which is why PENDING is a state
// and why the reconciliation job knows about it.
package media

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	RequestMediaUploadName = "RequestMediaUpload"

	// The token scopes of the media context. Its own pair rather than items:write, because an
	// agent that may attach files is not thereby allowed to rewrite entries, and the reverse
	// (ADR-0005: scopes are a second, independent bound).
	mediaWrite = "media:write"
	mediaRead  = "media:read"

	mediaTarget = "media"

	// MediaStagedAction and MediaConfirmedAction are the audit codes. Stable: an auditor filters
	// on them and a SIEM rule matches on them (audit.md §2).
	MediaStagedAction    audit.Action = "media.staged"
	MediaConfirmedAction audit.Action = "media.confirmed"

	// UploadWindow is how long an upload target stays usable.
	//
	// Short, because the URL is the credential: whoever holds it may write those bytes, and the
	// window is the only thing that ends that. Long enough that a large file over a slow line
	// finishes - the limit is 64 MiB by default, and fifteen minutes covers it at dial-up speeds.
	UploadWindow = 15 * time.Minute
)

// RequestMediaUpload stages an object and answers with where its bytes go.
//
// The record is written before the bytes exist, which is what makes the three-step flow possible:
// the identifier the client uploads under is one this server minted, and a client can therefore
// never name where its bytes land (T-11 - a storage key is never user text).
//
// What it does not do is ask about a container. A staged object is inert - it covers nothing and
// is attached to nothing, and until it is, there is no entry to ask a permission question about.
// The permission is asked where the object gains meaning: SetCover and AttachMedia authorise
// against the entry, and both refuse an object the actor did not upload. Asking here instead would
// mean asking at the tenant scope, which refuses everybody whose membership sits on a hub or a
// collection - most of a real installation - for an act that grants nothing.
type RequestMediaUpload struct {
	Objects    repository.Objects
	Transfers  storage.TransferIssuer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Config     env.Config
}

// UploadCommand is the input, typed.
type UploadCommand struct {
	FileName string
	// ClaimedType is what the sender says the bytes will be. It is kept on the record and held
	// against the sniff at confirmation; it never decides anything on its own (T-11).
	ClaimedType string
	// Size is the exact size the upload will have. Declared here and measured at confirmation.
	Size  int64
	Usage media.Usage
}

// StagedUpload is the answer: the record, and the one-use target its bytes travel to.
type StagedUpload struct {
	Object   media.Object
	Transfer storage.Transfer
}

// Execute stages the object.
func (h RequestMediaUpload) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd UploadCommand,
) (StagedUpload, error) {
	if err := actor.RequireScope(mediaWrite); err != nil {
		return StagedUpload{}, err
	}

	now := h.Clock.Now()
	object, err := media.NewPendingObject(media.NewObjectInput{
		ID:           h.IDs.NewID(),
		TenantID:     actor.TenantID,
		FileName:     cmd.FileName,
		ClaimedType:  cmd.ClaimedType,
		DeclaredSize: cmd.Size,
		SizeLimit:    h.Config.Request.MaxUploadBytes,
		Usage:        cmd.Usage,
		CreatedBy:    actor.AccountID,
		Now:          now,
	})
	if err != nil {
		return StagedUpload{}, err
	}

	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if err := h.Objects.Insert(ctx, object); err != nil {
			return err
		}
		return h.recordAudit(ctx, object, actor, now)
	})
	if err != nil {
		return StagedUpload{}, err
	}

	// After the commit, deliberately. Minting the target is signing rather than I/O, but it is the
	// answer to a record that now exists - and issuing it first would hand out a capability for an
	// object the transaction might still roll back.
	transfer, err := h.Transfers.IssueUpload(object, now.Add(UploadWindow))
	if err != nil {
		return StagedUpload{}, err
	}
	return StagedUpload{Object: object, Transfer: transfer}, nil
}

// recordAudit writes the evidence.
//
// The file name is not in it. It is user content, and rule 10 keeps user content out of the trail;
// what an auditor needs is who staged how many bytes of what kind, and that is all of it here.
func (h RequestMediaUpload) recordAudit(
	ctx context.Context, object media.Object, actor appshared.ActorContext, now time.Time,
) error {
	return h.Audit.Append(ctx, audit.Entry{
		TenantID:   object.TenantID,
		OccurredAt: now,
		Action:     MediaStagedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: mediaTarget,
		TargetID:   object.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "usage", Classification: audit.Open, To: string(object.Usage)},
			audit.Change{Field: "status", Classification: audit.Open, To: string(object.Status)},
			audit.Change{
				Field: "declared_size", Classification: audit.Open,
				To: sizeString(object.ByteSize),
			},
		),
	})
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h RequestMediaUpload) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RequestMediaUploadName,
		Summary: "Stages an upload and answers with where the bytes go: a presigned object-storage " +
			"URL, or this server's token-protected content route on a local-storage installation. " +
			"The object stays PENDING until it is confirmed; a staging nobody ever confirms is " +
			"reclaimed by the reconciliation job.",
		SideEffects: "Writes the media record and an audit entry, and mints a one-object, " +
			"expiring upload target.",
		TokenScope: mediaWrite,
		Input: []usecase.Field{
			{
				Name: "usage", Kind: usecase.KindString, Required: true,
				Description: "What the object is for: COVER or ATTACHMENT. It is held against the " +
					"use at attachment time, so an object staged as a cover cannot become an " +
					"attachment behind the client's back.",
			},
			{
				Name: "size", Kind: usecase.KindInt, Required: true,
				Description: "The exact size in bytes. Bounded by the installation's upload limit, " +
					"and measured again at confirmation - a declaration that turns out to be wrong " +
					"is refused there.",
			},
			{
				Name: "file_name", Kind: usecase.KindString,
				Description: "The name the file arrived under, kept for the download. Never a path: " +
					"a name carrying a separator is refused.",
			},
			{
				Name: "content_type", Kind: usecase.KindString,
				Description: "The claim about what the bytes are. Reconciled against the bytes at " +
					"confirmation and never trusted on its own.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MediaStagedAction, TargetType: mediaTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A media object is not a work item and has no history of its own; what a person " +
				"reads is the entry it ends up covering or attached to, and that entry's history " +
				"records the attachment rather than the upload.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

// invoke is the adapter between the catalogue's untyped input and the typed command, for all three
// channels at once.
func (h RequestMediaUpload) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	usage, err := media.ParseUsage(in.String("usage"))
	if err != nil {
		return nil, err
	}

	staged, err := h.Execute(ctx, actor, UploadCommand{
		FileName:    in.String("file_name"),
		ClaimedType: in.String("content_type"),
		Size:        int64(in.Int("size")),
		Usage:       usage,
	})
	if err != nil {
		return nil, err
	}
	return stagedOutput(staged), nil
}

// mediaOutput is the projection both media operations answer with.
func mediaOutput(object media.Object) usecase.Output {
	return usecase.Output{
		"id":           object.ID.String(),
		"file_name":    textOrNil(object.FileName),
		"content_type": object.ContentType,
		"size":         object.ByteSize,
		"checksum":     textOrNil(object.Checksum),
		"usage":        string(object.Usage),
		"status":       string(object.Status),
		"ref_count":    object.RefCount,
		"created_by":   object.CreatedBy.String(),
		"created_at":   object.CreatedAt,
	}
}

// stagedOutput adds the target the staging answered with. A separate transfer key rather than a
// flattened URL, because the contract carries the method and the expiry beside it and a client
// that only got a string would have to guess both.
func stagedOutput(staged StagedUpload) usecase.Output {
	out := mediaOutput(staged.Object)
	out["upload"] = transferOutput(staged.Transfer)
	return out
}

func transferOutput(transfer storage.Transfer) map[string]any {
	return map[string]any{
		"url":        transfer.URL,
		"method":     transfer.Method,
		"expires_at": transfer.ExpiresAt,
	}
}

// textOrNil is how an optional text reaches a projection: the value, or an explicit null. Leaving
// the key out would say "this server does not know about file names", which is a different
// statement from "this object has none".
func textOrNil(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func sizeString(bytes int64) string { return strconv.FormatInt(bytes, 10) }
