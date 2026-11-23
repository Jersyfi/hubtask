// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetWorkspaceHeaderName     = "SetWorkspaceHeader"
	SetWorkspacePictureName    = "SetWorkspacePicture"
	RemoveWorkspacePictureName = "RemoveWorkspacePicture"

	// The trail names the media object and the choice, never a file name or anything in a file
	// (rule 10).
	WorkspaceHeaderChangedAction  audit.Action = "workspace.header_changed"
	WorkspacePictureSetAction     audit.Action = "workspace.picture_set"
	WorkspacePictureRemovedAction audit.Action = "workspace.picture_removed"
)

// WorkspaceBrander is what the three branding use cases share (UC-INS-22, ADR-0112): the
// workspace's writer for its permission, its rule and its trail, the branding's store, and the media
// objects whose reference counts move with a picture.
type WorkspaceBrander struct {
	Writer   WorkspaceWriter
	Branding repository.WorkspaceBranding
	Objects  mediarepo.Objects
}

// prepare asks the permission that changes the workspace's name - STRUCTURE at the workspace, as
// UpdateWorkspace asks it (rule 2) - and then whether the levels above let the workspace show its
// own name and logo; where they do not, every change is refused naming who switched it off.
func (b WorkspaceBrander) prepare(ctx context.Context, actor appshared.ActorContext, action audit.Action) error {
	w := b.Writer
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     action,
		TokenScope: workspaceManage,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
	}); err != nil {
		return err
	}
	allowed, origin, err := b.allowed(ctx, actor.TenantID)
	if err != nil {
		return err
	}
	if !allowed {
		return shared.ErrForbidden.
			WithDetail("workspace.branding_off").
			WithParams(map[string]string{"origin": origin})
	}
	return nil
}

// allowed answers *the workspace may show its own name and logo* in force and, where it is off, the
// level that switched it off: the lock's origin, or the level whose value holds. Unlike the read's
// projection, a rule that cannot be resolved is an error here rather than the product's default -
// a write must not pass because the rule was unreadable. Without an instance layer the product's
// default, on, holds.
func (b WorkspaceBrander) allowed(ctx context.Context, tenantID shared.ID) (bool, string, error) {
	if b.Writer.Resolver.Instance == nil {
		return domain.DefaultSignInPolicy().OwnBranding, "", nil
	}
	resolved, err := b.Writer.Resolver.Resolve(ctx, tenantID)
	if err != nil {
		return false, "", err
	}
	return resolved.Effective.Policy.OwnBranding, brandingOrigin(resolved.Effective), nil
}

// brandingOrigin names who switched the branding off: the lock where there is one, else the level
// whose value is in force.
func brandingOrigin(effective domain.EffectivePolicy) string {
	if lock := effective.LockOf(domain.SwitchOwnBranding); lock != domain.LockNone {
		return string(lock)
	}
	return string(effective.SourceOf(domain.SwitchOwnBranding))
}

// ownBrandingAllowed is the switch in force for the read's projection, the product's default where
// there is no instance layer to resolve against.
func ownBrandingAllowed(resolved *ResolvedPolicy) bool {
	if resolved == nil {
		return domain.DefaultSignInPolicy().OwnBranding
	}
	return resolved.Effective.Policy.OwnBranding
}

// brandingOutput is the contract's WorkspaceBranding.
func brandingOutput(branding domain.Branding, allowed bool) usecase.Output {
	header := branding.Header
	if header == "" {
		header = domain.HeaderProduct
	}
	return usecase.Output{
		"header":              string(header),
		"logo_light_media_id": idText(branding.LogoLight),
		"logo_dark_media_id":  idText(branding.LogoDark),
		"symbol_media_id":     idText(branding.Symbol),
		"allowed":             allowed,
	}
}

// record writes one trail entry naming the fields that moved - identifiers and the choice only - and
// tells the other devices that the workspace's look changed (ADR-0112).
func (b WorkspaceBrander) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, now time.Time, changes ...audit.Change,
) error {
	if err := b.Writer.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: workspaceTarget,
		TargetID:   actor.TenantID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	}); err != nil {
		return err
	}
	return b.Writer.announce(ctx, actor)
}

// headerChange is the trail's line for a moved choice, nil where it did not move.
func headerChange(before, after domain.Branding) []audit.Change {
	if before.Header == after.Header {
		return nil
	}
	return []audit.Change{{
		Field: "header_shows", Classification: audit.Open,
		From: string(before.Header), To: string(after.Header),
	}}
}

// brandingDescriptor is what the three descriptors share.
func brandingDescriptor(d usecase.Descriptor) usecase.Descriptor {
	d.Permission = service.PermissionStructure
	d.TokenScope = workspaceManage
	d.Audit = usecase.AuditDeclaration{
		Action: d.Audit.Action, TargetType: workspaceTarget, Severity: audit.SeverityNotice, Required: true,
	}
	d.Activity = usecase.ActivityDeclaration{
		Exempt: "a workspace's own name and logo are not an entry, and the item history is keyed on an entry.",
	}
	return d
}

// SetWorkspaceHeader chooses what the header and the sign-in card show (UC-INS-22 check 3).
type SetWorkspaceHeader struct{ Brander WorkspaceBrander }

// Execute stores the choice and answers the branding.
func (h SetWorkspaceHeader) Execute(
	ctx context.Context, actor appshared.ActorContext, header domain.HeaderShows,
) (domain.Branding, error) {
	b := h.Brander
	if err := b.prepare(ctx, actor, WorkspaceHeaderChangedAction); err != nil {
		return domain.Branding{}, err
	}
	var answer domain.Branding
	err := b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := b.Branding.Lock(ctx)
		if err != nil {
			return err
		}
		after, err := before.WithHeader(header)
		if err != nil {
			return err
		}
		answer = after
		if after.Header == before.Header {
			return nil
		}
		now := b.Writer.Clock.Now()
		if err := b.Branding.Set(ctx, after, now); err != nil {
			return err
		}
		return b.record(ctx, actor, WorkspaceHeaderChangedAction, now, headerChange(before, after)...)
	})
	return answer, err
}

// Descriptor registers the use case in all three channels.
func (h SetWorkspaceHeader) Descriptor() usecase.Descriptor {
	return brandingDescriptor(usecase.Descriptor{
		Name: SetWorkspaceHeaderName,
		Summary: "Chooses what the workspace's header and sign-in card show: the product's name, the " +
			"workspace's name, or its own logo alone.",
		SideEffects: "Writes the workspace's choice and an audit entry.",
		Input: []usecase.Field{{
			Name: "header", Kind: usecase.KindString, Required: true,
			Enum: []string{string(domain.HeaderProduct), string(domain.HeaderWorkspaceName), string(domain.HeaderLogo)},
			Description: "PRODUCT (the default), WORKSPACE_NAME, or LOGO - which needs one of the " +
				"workspace's pictures.",
		}},
		Audit: usecase.AuditDeclaration{Action: WorkspaceHeaderChangedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			header, err := domain.ParseHeaderShows(in.String("header"))
			if err != nil {
				return nil, err
			}
			branding, err := h.Execute(ctx, actor, header)
			if err != nil {
				return nil, err
			}
			return brandingOutput(branding, true), nil
		}),
	})
}

// SetWorkspacePicture makes a picture the caller uploaded one of the workspace's three (UC-INS-22
// check 1).
//
// Only a picture the caller staged themselves, for SetOwnAvatar's reason: somebody else's upload is
// answered as missing. It must be a confirmed raster image staged for a workspace
// (media.Object.WorkspaceLogo) - every member's browser draws it. The reference counts move in the
// same transaction, so the reconciliation takes the picture it replaced.
type SetWorkspacePicture struct{ Brander WorkspaceBrander }

// Execute sets the picture and answers the branding.
func (h SetWorkspacePicture) Execute(
	ctx context.Context, actor appshared.ActorContext, picture domain.Picture, mediaID shared.ID,
) (domain.Branding, error) {
	if mediaID.IsZero() {
		return domain.Branding{}, shared.ErrValidation.
			WithDetail("media.media_id_required").
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.media_id_required"})
	}
	b := h.Brander
	if err := b.prepare(ctx, actor, WorkspacePictureSetAction); err != nil {
		return domain.Branding{}, err
	}
	var answer domain.Branding
	err := b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		object, err := b.Objects.Find(ctx, mediaID)
		if errors.Is(err, shared.ErrNotFound) || (err == nil && object.CreatedBy != actor.AccountID) {
			return pictureNotFound()
		}
		if err != nil {
			return err
		}
		if err := object.WorkspaceLogo(); err != nil {
			return err
		}
		before, err := b.Branding.Lock(ctx)
		if err != nil {
			return err
		}
		answer = before
		replaced := before.Of(picture)
		if replaced == mediaID {
			return nil
		}
		after := before.WithPicture(picture, mediaID)
		now := b.Writer.Clock.Now()
		if err := b.Branding.Set(ctx, after, now); err != nil {
			return err
		}
		if err := b.Objects.AdjustRefCount(ctx, mediaID, 1); err != nil {
			return err
		}
		if !replaced.IsZero() {
			if err := b.Objects.AdjustRefCount(ctx, replaced, -1); err != nil {
				return err
			}
		}
		answer = after
		return b.record(ctx, actor, WorkspacePictureSetAction, now, audit.Change{
			Field: picture.Field(), Classification: audit.Open, From: idText(replaced), To: idText(mediaID),
		})
	})
	return answer, err
}

// Descriptor registers the use case in all three channels.
func (h SetWorkspacePicture) Descriptor() usecase.Descriptor {
	return brandingDescriptor(usecase.Descriptor{
		Name: SetWorkspacePictureName,
		Summary: "Makes a picture the caller uploaded (usage WORKSPACE_LOGO, confirmed) the workspace's " +
			"logo for the light theme, its logo for the dark theme, or its square symbol.",
		SideEffects: "Writes the workspace's picture, moves the media reference counts, and writes an " +
			"audit entry naming the media object.",
		Input: []usecase.Field{
			pictureField(),
			{Name: "media_id", Kind: usecase.KindID, Required: true,
				Description: "A confirmed media object the caller staged with usage WORKSPACE_LOGO: a " +
					"PNG, JPEG, GIF or WebP image of at most 5 MB."},
		},
		Audit: usecase.AuditDeclaration{Action: WorkspacePictureSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			picture, err := domain.ParsePicture(in.String("picture"))
			if err != nil {
				return nil, err
			}
			mediaID, err := in.ID("media_id")
			if err != nil {
				return nil, err
			}
			branding, err := h.Execute(ctx, actor, picture, mediaID)
			if err != nil {
				return nil, err
			}
			return brandingOutput(branding, true), nil
		}),
	})
}

// RemoveWorkspacePicture takes one of the workspace's pictures away. Idempotent: a picture that is
// not there answers the branding as it stands and writes nothing. The last picture taken away under
// "the logo alone" returns the header to the product's name in the same write.
type RemoveWorkspacePicture struct{ Brander WorkspaceBrander }

// Execute clears the picture and answers the branding.
func (h RemoveWorkspacePicture) Execute(
	ctx context.Context, actor appshared.ActorContext, picture domain.Picture,
) (domain.Branding, error) {
	b := h.Brander
	if err := b.prepare(ctx, actor, WorkspacePictureRemovedAction); err != nil {
		return domain.Branding{}, err
	}
	var answer domain.Branding
	err := b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := b.Branding.Lock(ctx)
		if err != nil {
			return err
		}
		answer = before
		removed := before.Of(picture)
		if removed.IsZero() {
			return nil
		}
		after := before.WithPicture(picture, "")
		now := b.Writer.Clock.Now()
		if err := b.Branding.Set(ctx, after, now); err != nil {
			return err
		}
		if err := b.Objects.AdjustRefCount(ctx, removed, -1); err != nil {
			return err
		}
		answer = after
		changes := append([]audit.Change{{
			Field: picture.Field(), Classification: audit.Open, From: idText(removed), To: nil,
		}}, headerChange(before, after)...)
		return b.record(ctx, actor, WorkspacePictureRemovedAction, now, changes...)
	})
	return answer, err
}

// Descriptor registers the use case in all three channels.
func (h RemoveWorkspacePicture) Descriptor() usecase.Descriptor {
	return brandingDescriptor(usecase.Descriptor{
		Name:    RemoveWorkspacePictureName,
		Summary: "Takes one of the workspace's pictures away; the last one taken away under the logo alone returns the header to the product's name.",
		SideEffects: "Clears the workspace's picture, lowers the media reference count, and writes an " +
			"audit entry. The picture itself is removed once nothing points at it.",
		Input: []usecase.Field{pictureField()},
		Audit: usecase.AuditDeclaration{Action: WorkspacePictureRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			picture, err := domain.ParsePicture(in.String("picture"))
			if err != nil {
				return nil, err
			}
			branding, err := h.Execute(ctx, actor, picture)
			if err != nil {
				return nil, err
			}
			return brandingOutput(branding, true), nil
		}),
	})
}

func pictureField() usecase.Field {
	pictures := make([]string, 0, len(domain.Pictures()))
	for _, picture := range domain.Pictures() {
		pictures = append(pictures, string(picture))
	}
	return usecase.Field{
		Name: "picture", Kind: usecase.KindString, Required: true, Enum: pictures,
		Description: "LOGO_LIGHT, the logo on the light theme; LOGO_DARK, the one on the dark theme; " +
			"or SYMBOL, the square symbol.",
	}
}
