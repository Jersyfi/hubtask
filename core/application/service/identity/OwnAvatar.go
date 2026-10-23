// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	SetOwnAvatarName    = "SetOwnAvatar"
	RemoveOwnAvatarName = "RemoveOwnAvatar"

	// AvatarSetAction and AvatarRemovedAction are the audit codes of a person showing a picture of
	// themselves and taking it away. The trail names the media object, never anything in it.
	AvatarSetAction     audit.Action = "account.avatar_set"
	AvatarRemovedAction audit.Action = "account.avatar_removed"
)

// SetOwnAvatar makes a picture the caller uploaded the avatar every member sees beside their name.
//
// Only one's own, for UpdateOwnAccount's reason, and only a picture one staged oneself: an object
// somebody else uploaded is answered as missing, as GetMedia answers it, so the call is no way to
// learn which identifiers exist. The object must be confirmed, staged as an avatar and judged an
// inline image (media.Object.Portrayable) - every member's browser draws it.
//
// The reference counts move in the same transaction, so DeleteMedia refuses a picture in use and
// the reconciliation takes the one it replaced.
type SetOwnAvatar struct {
	Accounts   repository.Accounts
	Avatars    repository.AccountAvatars
	Objects    mediarepo.Objects
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute sets the picture and returns the account.
func (h SetOwnAvatar) Execute(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (domain.Account, error) {
	if err := ownAccountActor(actor); err != nil {
		return domain.Account{}, err
	}
	if mediaID.IsZero() {
		return domain.Account{}, shared.ErrValidation.
			WithDetail("media.media_id_required").
			WithFields(shared.FieldError{Path: "/media_id", Code: "media.media_id_required"})
	}

	var updated domain.Account
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		account, err := findPerson(ctx, h.Accounts, actor)
		if err != nil {
			return err
		}
		if account.AvatarMediaID == mediaID {
			updated = account
			return nil
		}

		object, err := h.Objects.Find(ctx, mediaID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return pictureNotFound()
			}
			return err
		}
		if object.CreatedBy != actor.AccountID {
			return pictureNotFound()
		}
		if err := object.Portrayable(); err != nil {
			return err
		}

		now := h.Clock.Now()
		if err := h.Avatars.SetAvatar(ctx, account.ID, mediaID, now); err != nil {
			return err
		}
		if err := h.Objects.AdjustRefCount(ctx, mediaID, 1); err != nil {
			return err
		}
		if !account.AvatarMediaID.IsZero() {
			if err := h.Objects.AdjustRefCount(ctx, account.AvatarMediaID, -1); err != nil {
				return err
			}
		}

		before := account.AvatarMediaID
		account.AvatarMediaID = mediaID
		updated = account
		return recordAvatar(ctx, h.Audit, actor, AvatarSetAction, before, mediaID, now)
	})
	if err != nil {
		return domain.Account{}, err
	}
	return updated, nil
}

// Descriptor registers the use case in all three channels.
func (h SetOwnAvatar) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetOwnAvatarName,
		Summary: "Makes a picture the caller uploaded (usage AVATAR, confirmed) the avatar every member " +
			"sees beside their name, in place of the initials.",
		SideEffects: "Writes the account's picture, moves the media reference counts, and writes an " +
			"audit entry.",
		TokenScope: accountsWrite,
		Input: []usecase.Field{
			{
				Name: "media_id", Kind: usecase.KindID, Required: true,
				Description: "A confirmed media object the caller staged with usage AVATAR: a PNG, " +
					"JPEG, GIF or WebP image of at most 5 MB.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: AvatarSetAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetOwnAvatar) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	mediaID, err := in.ID("media_id")
	if err != nil {
		return nil, err
	}
	account, err := h.Execute(ctx, actor, mediaID)
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}

// RemoveOwnAvatar takes the caller's picture away: the initials are the avatar again. Idempotent -
// an account without a picture answers as it is and writes nothing.
type RemoveOwnAvatar struct {
	Accounts   repository.Accounts
	Avatars    repository.AccountAvatars
	Objects    mediarepo.Objects
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// Execute clears the picture and returns the account.
func (h RemoveOwnAvatar) Execute(
	ctx context.Context, actor appshared.ActorContext,
) (domain.Account, error) {
	if err := ownAccountActor(actor); err != nil {
		return domain.Account{}, err
	}

	var updated domain.Account
	err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		account, err := findPerson(ctx, h.Accounts, actor)
		if err != nil {
			return err
		}
		before := account.AvatarMediaID
		if before.IsZero() {
			updated = account
			return nil
		}

		now := h.Clock.Now()
		if err := h.Avatars.SetAvatar(ctx, account.ID, "", now); err != nil {
			return err
		}
		if err := h.Objects.AdjustRefCount(ctx, before, -1); err != nil {
			return err
		}
		account.AvatarMediaID = ""
		updated = account
		return recordAvatar(ctx, h.Audit, actor, AvatarRemovedAction, before, "", now)
	})
	if err != nil {
		return domain.Account{}, err
	}
	return updated, nil
}

// Descriptor registers the use case in all three channels.
func (h RemoveOwnAvatar) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:    RemoveOwnAvatarName,
		Summary: "Takes the caller's picture away, so the initials of the name are the avatar again.",
		SideEffects: "Clears the account's picture, lowers the media reference count, and writes an " +
			"audit entry. The picture itself is removed once nothing points at it.",
		TokenScope: accountsWrite,
		Audit: usecase.AuditDeclaration{
			Action: AvatarRemovedAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RemoveOwnAvatar) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	account, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	return accountOutput(account), nil
}

// ownAccountActor is the check both share with UpdateOwnAccount: the scope, and a person signed in.
func ownAccountActor(actor appshared.ActorContext) error {
	if err := actor.RequireScope(accountsWrite); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("accounts.self_required")
	}
	return nil
}

// findPerson reads the caller's account and refuses a service account: a machine has no face, and
// one acting as itself would choose what every member sees beside its runs.
func findPerson(
	ctx context.Context, accounts repository.Accounts, actor appshared.ActorContext,
) (domain.Account, error) {
	account, err := accounts.Find(ctx, actor.AccountID)
	if err != nil {
		return domain.Account{}, err
	}
	if account.Kind != domain.AccountUser {
		return domain.Account{}, shared.ErrForbidden.WithDetail("accounts.picture_needs_a_person")
	}
	return account, nil
}

// pictureNotFound is GetMedia's answer for an object the caller may not reach (T-04).
func pictureNotFound() error {
	return shared.ErrNotFound.WithDetail("media.not_found")
}

func recordAvatar(
	ctx context.Context, sink audit.Sink, actor appshared.ActorContext, action audit.Action,
	from, to shared.ID, now time.Time,
) error {
	// Identifiers only: the picture is the person's, and the trail never carries it (rule 10).
	return sink.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: accountTarget,
		TargetID:   actor.AccountID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(
			audit.Change{Field: "avatar_media_id", Classification: audit.Open, From: idText(from), To: idText(to)},
		),
	})
}

func idText(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}
