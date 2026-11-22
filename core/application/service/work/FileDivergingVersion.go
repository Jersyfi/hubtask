// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const FileDivergingVersionName = "FileDivergingVersion"

// The two sides a filed version comes from.
const (
	// VersionMine is the caller's own text, given up for the one in place.
	VersionMine = "MINE"
	// VersionReplaced is the text in place that the caller is about to replace with their own.
	VersionReplaced = "REPLACED"
)

// FileDivergingVersion keeps a version of an entry's title or notes that is about to be lost
// because somebody else changed the field while the caller was writing (offline-sync.md §5, the
// online case). It is the push's FileDisplaced reached from outside: the same write as any
// comment - permission, event, change log entry, audit entry - with a heading of its own, since
// no merge took place and no device is named.
type FileDivergingVersion struct {
	Comments AddComment
}

// FileDivergingVersionCommand is the input, typed.
type FileDivergingVersionCommand struct {
	ItemID shared.ID
	// Field is the entry's field the text belongs to: notes or title.
	Field string
	// Side is VersionMine or VersionReplaced.
	Side string
	Body string
}

// Execute files the version and returns the comment that holds it.
func (h FileDivergingVersion) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd FileDivergingVersionCommand,
) (domain.Comment, error) {
	if cmd.Field != domain.FieldNotes && cmd.Field != domain.FieldTitle {
		return domain.Comment{}, shared.ErrValidation.
			WithDetail("comments.version_field_unknown").
			WithFields(shared.FieldError{Path: "/field", Code: "comments.version_field_unknown"})
	}
	code, inPlace := "", ""
	switch cmd.Side {
	case VersionMine:
		// The caller's own text: nothing on the server holds it to compare against.
		code = domain.SetAsideVersionCode
	case VersionReplaced:
		code, inPlace = domain.ReplacedVersionCode, cmd.Field
	default:
		return domain.Comment{}, shared.ErrValidation.
			WithDetail("comments.version_side_unknown").
			WithFields(shared.FieldError{Path: "/side", Code: "comments.version_side_unknown"})
	}
	return h.Comments.Execute(ctx, actor, AddCommentCommand{
		ItemID: cmd.ItemID, Body: cmd.Body, SystemCode: code, InPlace: inPlace,
		SystemParams: map[string]string{
			"field": cmd.Field,
			"at":    h.Comments.Writer.Clock.Now().UTC().Format(time.RFC3339),
		},
	})
}

// Descriptor is the catalogue entry.
func (h FileDivergingVersion) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       FileDivergingVersionName,
		Permission: service.PermissionWriteItems,
		Summary: "Keeps a version of an entry's title or notes that somebody else changed " +
			"meanwhile, as a system comment on the entry in the caller's name: MINE is the " +
			"caller's own text, given up for the one in place, which nothing on the server can " +
			"check; REPLACED is the text in place the caller is about to replace, refused unless " +
			"it is the entry's value of the field now. The same permission as any comment; the " +
			"entry itself is not changed.",
		SideEffects: "Writes the comment, announces " + string(event.CommentCreated) +
			", records the change for offline clients, writes an audit entry and a step of the " +
			"entry's history.",
		TokenScope: commentsWrite,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry whose text is kept.",
			},
			{
				Name: "field", Kind: usecase.KindString, Required: true,
				Enum:        []string{domain.FieldNotes, domain.FieldTitle},
				Description: "Which of the entry's texts the version is of.",
			},
			{
				Name: "side", Kind: usecase.KindString, Required: true,
				Enum: []string{VersionMine, VersionReplaced},
				Description: "MINE for the caller's own text, given up; REPLACED for the text in " +
					"place the caller is about to replace, which must be the entry's value now.",
			},
			{
				Name: "body", Kind: usecase.KindString, Required: true,
				Description: "The text, bounded as the field it is a version of: a title at most " +
					"500 characters, notes by no bound of their own beyond the request's size.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: CommentCreatedAction, TargetType: commentTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "the step it writes is AddComment's item.commented, through the same write.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h FileDivergingVersion) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	itemID, err := in.ID("item_id")
	if err != nil {
		return nil, err
	}
	comment, err := h.Execute(ctx, actor, FileDivergingVersionCommand{
		ItemID: itemID, Field: in.String("field"), Side: in.String("side"), Body: in.String("body"),
	})
	if err != nil {
		return nil, err
	}
	return commentOutput(comment), nil
}
