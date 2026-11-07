// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

const (
	SetDeskLogoName    = "SetDeskLogo"
	RemoveDeskLogoName = "RemoveDeskLogo"
	GetDeskLogoName    = "GetDeskLogo"

	// The trail names the media object, never anything in it.
	DeskLogoSetAction     audit.Action = "desk.logo_set"
	DeskLogoRemovedAction audit.Action = "desk.logo_removed"
	DeskLogoReadAction    audit.Action = "desk.logo_read"

	deskTarget = "desk"
)

// SetDeskLogo makes a picture the caller uploaded the logo a collection's portal draws (UC-SVC-05
// check 9), in place of its icon and colour. STRUCTURE at the collection, as its name and icon ask.
//
// Only a picture the caller staged themselves, for SetOwnAvatar's reason: somebody else's upload is
// answered as missing, so the call is no way to learn which identifiers exist. It must be a
// confirmed inline image (media.Object.Emblem) - every requester's browser draws it. The reference
// counts move in the same transaction, so the reconciliation takes the logo it replaced.
type SetDeskLogo struct {
	Desk    Desk
	Logos   repository.DeskLogos
	Objects mediarepo.Objects
}

// Execute sets the logo and answers it.
func (h SetDeskLogo) Execute(ctx context.Context, actor appshared.ActorContext, collectionID, mediaID shared.ID) (shared.ID, error) {
	if mediaID.IsZero() {
		return "", malformed("/media_id", "media.media_id_required")
	}
	if err := h.Desk.authorizeDesk(ctx, actor, collectionID, DeskLogoSetAction); err != nil {
		return "", err
	}
	err := h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		object, err := h.Objects.Find(ctx, mediaID)
		if errors.Is(err, shared.ErrNotFound) || (err == nil && object.CreatedBy != actor.AccountID) {
			return shared.ErrNotFound.WithDetail("media.not_found")
		}
		if err != nil {
			return err
		}
		if err := object.Emblem(); err != nil {
			return err
		}
		before, err := h.Logos.Find(ctx, collectionID)
		if err != nil || before == mediaID {
			return err
		}
		return swapLogo(ctx, h.Desk, h.Logos, h.Objects, actor, collectionID, before, mediaID, DeskLogoSetAction)
	})
	if err != nil {
		return "", err
	}
	return mediaID, nil
}

// swapLogo writes the new logo, moves both reference counts and records the change.
func swapLogo(
	ctx context.Context, desk Desk, logos repository.DeskLogos, objects mediarepo.Objects,
	actor appshared.ActorContext, collectionID, before, after shared.ID, action audit.Action,
) error {
	if err := logos.Set(ctx, collectionID, after); err != nil {
		return err
	}
	if !after.IsZero() {
		if err := objects.AdjustRefCount(ctx, after, 1); err != nil {
			return err
		}
	}
	if !before.IsZero() {
		if err := objects.AdjustRefCount(ctx, before, -1); err != nil {
			return err
		}
	}
	return desk.record(ctx, actor, action, deskTarget, collectionID, desk.Clock.Now(),
		audit.Change{Field: "logo_media_id", Classification: audit.Open, From: idText(before), To: idText(after)})
}

// Descriptor registers the use case in all three channels.
func (h SetDeskLogo) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetDeskLogoName, Permission: service.PermissionStructure,
		Summary: "Makes a picture the caller uploaded (usage DESK_LOGO, confirmed) the logo a collection's " +
			"portal shows, in place of its icon and colour.",
		SideEffects: "Writes the desk's logo, moves the media reference counts, and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "container_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."},
			{Name: "media_id", Kind: usecase.KindID, Required: true, Description: "A confirmed media object the caller staged with usage DESK_LOGO: a PNG, JPEG, GIF or WebP image of at most 5 MB."},
		},
		Audit: usecase.AuditDeclaration{Action: DeskLogoSetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("container_id")
			if err != nil {
				return nil, err
			}
			mediaID, err := in.ID("media_id")
			if err != nil {
				return nil, err
			}
			logo, err := h.Execute(ctx, actor, collectionID, mediaID)
			if err != nil {
				return nil, err
			}
			return deskLogoOutput(collectionID, logo), nil
		}),
	}, deskTarget)
}

// RemoveDeskLogo takes the desk's logo away: the portal draws the collection's icon and colour
// again. Idempotent - a desk without a logo answers as it is and writes nothing.
type RemoveDeskLogo struct {
	Desk    Desk
	Logos   repository.DeskLogos
	Objects mediarepo.Objects
}

// Execute clears the logo.
func (h RemoveDeskLogo) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) error {
	if err := h.Desk.authorizeDesk(ctx, actor, collectionID, DeskLogoRemovedAction); err != nil {
		return err
	}
	return h.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		before, err := h.Logos.Find(ctx, collectionID)
		if err != nil || before.IsZero() {
			return err
		}
		return swapLogo(ctx, h.Desk, h.Logos, h.Objects, actor, collectionID, before, "", DeskLogoRemovedAction)
	})
}

// Descriptor registers the use case in all three channels.
func (h RemoveDeskLogo) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: RemoveDeskLogoName, Permission: service.PermissionStructure,
		Summary: "Takes a desk's logo away, so its portal draws the collection's icon and colour again.",
		SideEffects: "Clears the desk's logo, lowers the media reference count, and writes an audit entry. " +
			"The picture itself is removed once nothing points at it.",
		TokenScope: deskWrite,
		Input:      []usecase.Field{{Name: "container_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."}},
		Audit:      usecase.AuditDeclaration{Action: DeskLogoRemovedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("container_id")
			if err != nil {
				return nil, err
			}
			if err := h.Execute(ctx, actor, collectionID); err != nil {
				return nil, err
			}
			return deskLogoOutput(collectionID, ""), nil
		}),
	}, deskTarget)
}

// GetDeskLogo answers a desk's logo to the collection's readers - its team, who hold no REQUEST
// there and so are refused the requester catalogue that carries it to requesters.
type GetDeskLogo struct {
	Desk  Desk
	Logos repository.DeskLogos
}

// Execute answers the logo, zero where the desk draws its icon and colour.
func (h GetDeskLogo) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) (shared.ID, error) {
	if err := h.Desk.authorizeDeskAs(ctx, actor, collectionID, service.PermissionRead, DeskLogoReadAction, deskRead); err != nil {
		return "", err
	}
	var logo shared.ID
	err := h.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Logos.Find(ctx, collectionID)
		logo = found
		return err
	})
	return logo, err
}

// Descriptor registers the use case in all three channels.
func (h GetDeskLogo) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: GetDeskLogoName, Permission: service.PermissionRead,
		Summary:     "Answers the logo a collection's portal shows, or none where it draws the collection's icon and colour.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "container_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."}},
		Audit: usecase.AuditDeclaration{Action: DeskLogoReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("container_id")
			if err != nil {
				return nil, err
			}
			logo, err := h.Execute(ctx, actor, collectionID)
			if err != nil {
				return nil, err
			}
			return deskLogoOutput(collectionID, logo), nil
		}),
	}, deskTarget)
}

// authorizeDesk reads the collection and asks STRUCTURE there; a hub carries no desk.
func (d Desk) authorizeDesk(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, action audit.Action) error {
	return d.authorizeDeskAs(ctx, actor, collectionID, service.PermissionStructure, action, deskWrite)
}

// authorizeDeskAs reads the collection and asks the permission there; a hub carries no desk.
func (d Desk) authorizeDeskAs(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, permission service.Permission, action audit.Action, scope string,
) error {
	if collectionID.IsZero() {
		return malformed("/container_id", "requests.collection_required")
	}
	var collection workdomain.Container
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := d.Containers.Find(ctx, collectionID)
		collection = found
		return err
	})
	if err != nil {
		return err
	}
	if err := d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: service.ContainerScopes(collection),
		Action: action, TokenScope: scope, TargetType: deskTarget, TargetID: collectionID,
	}); err != nil {
		return err
	}
	if collection.Type != workdomain.ContainerCollection {
		return malformed("/container_id", "servicedesk.logo_needs_a_collection")
	}
	return nil
}

// deskLogoOutput is the contract's DeskLogo.
func deskLogoOutput(collectionID, logo shared.ID) usecase.Output {
	return usecase.Output{"collection_id": collectionID.String(), "logo_media_id": idText(logo)}
}

func idText(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}
