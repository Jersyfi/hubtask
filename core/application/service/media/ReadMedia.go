// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

const (
	GetMediaName = "GetMedia"

	// MediaReadAction is the audit code of an attempted read, declared for the reason
	// ItemReadAction is: an ordinary read writes no entry, a refused one does (audit.md §4).
	MediaReadAction audit.Action = "media.read"

	// DownloadWindow is how long a download target stays usable.
	//
	// Shorter than the upload window, because the two are not the same risk. An upload target
	// admits bytes to an object nothing points at yet; a download target hands out somebody's
	// file, and a client that needs it again asks again - which costs one request and is the
	// whole reason the window can be this short.
	DownloadWindow = 5 * time.Minute
)

// Reader is the slice of the authorisation service this package needs: which of these paths may
// the actor read.
//
// The many-paths form rather than Authorize, because a media object can serve several entries and
// the question is whether *any* of them is readable. Asked one Authorize at a time, an object
// attached to five entries would write four DENIED entries for somebody who was not in the end
// denied anything, which is a trail an auditor cannot read (audit.md §4, access.Permitted).
type Reader interface {
	Permitted(
		ctx context.Context, actor appshared.ActorContext, request access.Request,
		paths [][]identity.Scope,
	) ([]bool, error)
}

// GetMedia reads one media object's record.
//
// Who may read it is not a question about the object - an object is a file, and a file has no
// place in the hierarchy the role matrix resolves along. It is a question about what the object
// serves: whoever may read an entry it covers or is attached to may read the record behind the
// picture they are already looking at. Its uploader may too, which is what makes the three-step
// flow finishable - between the staging and the first attachment the object serves nothing, and
// there would otherwise be a window in which not even the person who staged it can read it back.
type GetMedia struct {
	Objects    repository.Objects
	Containers workrepo.Containers
	Transfers  storage.TransferIssuer
	Reader     Reader
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// MediaQuery is the input, typed.
type MediaQuery struct{ MediaID shared.ID }

// ViewedMedia is the answer: the record, and where its bytes come from when there are any.
type ViewedMedia struct {
	Object media.Object
	// Download is the target, zero for an object that is not READY. A PENDING object has no
	// bytes worth handing out a capability for: nothing has judged them, and T-11 forbids serving
	// what no sniff has decided the delivery of.
	Download storage.Transfer
}

// Execute reads the object.
func (h GetMedia) Execute(
	ctx context.Context, actor appshared.ActorContext, query MediaQuery,
) (ViewedMedia, error) {
	if err := actor.RequireScope(mediaRead); err != nil {
		return ViewedMedia{}, err
	}
	if query.MediaID.IsZero() {
		return ViewedMedia{}, mediaIDRequired()
	}

	object, paths, err := h.read(ctx, actor, query.MediaID)
	if err != nil {
		return ViewedMedia{}, err
	}
	if err := h.ensureReadable(ctx, actor, object, paths); err != nil {
		return ViewedMedia{}, err
	}
	if object.Status != media.StatusReady {
		return ViewedMedia{Object: object}, nil
	}

	// After the permission question, deliberately: minting is signing, and a capability minted for
	// somebody who turns out not to be allowed to read the object is one that was handed out.
	download, err := h.Transfers.IssueDownload(object, h.Clock.Now().Add(DownloadWindow))
	if err != nil {
		return ViewedMedia{}, err
	}
	return ViewedMedia{Object: object, Download: download}, nil
}

// read reads the object together with the entries it serves and the collections those sit in - one
// read-only transaction, because the permission question needs all three and none of them decides
// a write (multi-tenancy.md §7).
func (h GetMedia) read(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (media.Object, [][]identity.Scope, error) {
	var (
		object media.Object
		paths  [][]identity.Scope
	)

	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := h.Objects.Find(ctx, mediaID)
		if err != nil {
			return err
		}
		object = found

		items, err := h.Objects.ReferencingItems(ctx, mediaID)
		if err != nil {
			return err
		}
		paths, err = h.pathsOf(ctx, items)
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return media.Object{}, nil, mediaNotFound(mediaID)
		}
		return media.Object{}, nil, err
	}
	if object.DeletedAt != nil {
		// Marked for the reconciliation to remove. It serves nothing and is on its way out; saying
		// so is the same answer an identifier that never existed gets (T-04).
		return media.Object{}, nil, mediaNotFound(mediaID)
	}
	return object, paths, nil
}

// pathsOf resolves the collections the referencing entries sit in, as the paths a membership is
// resolved along.
//
// One read per distinct collection rather than per entry: five attachments of one file inside one
// collection are one question, and the reference list is bounded by the port either way. Duplicates
// are kept rather than folded - Permitted answers per path, and a shorter list would not make a
// different decision, only a less obvious one.
func (h GetMedia) pathsOf(
	ctx context.Context, items []repository.ItemRef,
) ([][]identity.Scope, error) {
	seen := make(map[shared.ID][]identity.Scope, len(items))
	paths := make([][]identity.Scope, 0, len(items))

	for _, ref := range items {
		path, known := seen[ref.CollectionID]
		if !known {
			collection, err := h.Containers.Find(ctx, ref.CollectionID)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					// The entry's collection is gone while the entry is not. A tenant-scoped
					// foreign key makes this unreachable (ADR-0024), so it is a defect rather
					// than a reference that quietly stops counting.
					return nil, shared.ErrInternal.
						WithDetail("items.collection_missing").WithCause(err)
				}
				return nil, err
			}
			path = containerPath(collection)
			seen[ref.CollectionID] = path
		}
		paths = append(paths, path)
	}
	return paths, nil
}

// ensureReadable answers the one permission question: the uploader, or somebody who may read an
// entry the object serves.
//
// An object nobody may reach is reported missing rather than forbidden. Its identifier is one the
// caller either guessed or was handed by something they may not read, and both deserve the same
// answer as an identifier that names nothing (T-04, multi-tenancy.md §2).
func (h GetMedia) ensureReadable(
	ctx context.Context, actor appshared.ActorContext, object media.Object,
	paths [][]identity.Scope,
) error {
	if object.CreatedBy == actor.AccountID {
		return nil
	}
	if len(paths) == 0 {
		return mediaNotFound(object.ID)
	}

	allowed, err := h.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Action:     MediaReadAction,
		TokenScope: mediaRead,
		TargetType: mediaTarget,
		TargetID:   object.ID,
	}, paths)
	if err != nil {
		return err
	}
	for _, may := range allowed {
		if may {
			return nil
		}
	}
	return mediaNotFound(object.ID)
}

// containerPath is the path a membership is resolved along, from the tenant down to the container
// (domain-model.md §3.2).
//
// A copy of the work package's rather than an import of it: this package may not import that one -
// they are siblings in the application layer and neither is the other's dependency - and the
// alternative is a shared helper package for six lines that have not changed since 0.1.0.
func containerPath(container domain.Container) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !container.ParentID.IsZero() {
		path = append(path, identity.HubScope(container.ParentID))
	}
	if container.Type == domain.ContainerHub {
		return append(path, identity.HubScope(container.ID))
	}
	return append(path, identity.CollectionScope(container.ID))
}

// mediaIDRequired is the one refusal for a call that named no object, so that every operation here
// spells it the same way.
func mediaIDRequired() error {
	return shared.ErrValidation.
		WithDetail("media.media_id_required").
		WithFields(shared.FieldError{Path: "/media_id", Code: "media.media_id_required"})
}

// viewedOutput adds the download target to the projection. Absent rather than null for an object
// that has none, for the reason the upload target is absent on a READY one: the key says a
// capability was minted, and there is nothing to mint here.
func viewedOutput(viewed ViewedMedia) usecase.Output {
	out := mediaOutput(viewed.Object)
	if viewed.Download.URL != "" {
		out["download"] = transferOutput(viewed.Download)
	}
	return out
}

// Descriptor is the catalogue entry.
func (h GetMedia) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: GetMediaName,
		Summary: "Reads one media object's record, with a download target when it is READY. " +
			"Readable by whoever staged it, and by anybody who may read an entry it covers or is " +
			"attached to - an object nobody may reach is reported missing rather than forbidden. " +
			"The download target is a short-lived capability: a presigned object-storage URL, or " +
			"this server's token-protected content route on a local-storage installation.",
		SideEffects: "None. Reads only, and mints an expiring download target.",
		TokenScope:  mediaRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "media_id", Kind: usecase.KindID, Required: true,
				Description: "The media object to read.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: MediaReadAction, TargetType: mediaTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A media object is not a work item and has no history of its own; what a person " +
				"reads is the entry it ends up covering or attached to, and that entry's history " +
				"records the attachment rather than the upload.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetMedia) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	mediaID, err := in.ID("media_id")
	if err != nil {
		return nil, err
	}

	viewed, err := h.Execute(ctx, actor, MediaQuery{MediaID: mediaID})
	if err != nil {
		return nil, err
	}
	return viewedOutput(viewed), nil
}
