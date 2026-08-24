// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"io"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
)

// MediaContent moves the bytes on a local-storage installation: this server standing in for the
// bucket that an object-storage installation would have carried them to (arc42 §8.4).
//
// It is not a use case and is deliberately not in the catalogue. A use case takes a named input
// and answers a projection over three channels; this takes a stream and answers a stream, which is
// neither what MCP nor what an automation rule can do with it. What it shares with a use case is
// the rule that matters: the tenant comes from the credential and from nowhere else.
//
// The credential is the URL. There is no bearer token on these routes - that is the contract's
// decision, and the same one a presigned bucket URL makes - so the adapter turns the signed token
// into a Grant and this decides what may be done with it. Validating a signature is
// authentication, which an adapter may do; everything after it is here (ADR-0005,
// presentation/CLAUDE.md).
type MediaContent struct {
	Objects    repository.Objects
	Store      storage.ObjectStore
	Guard      storage.Guard
	UnitOfWork persistence.UnitOfWork
	Config     env.Config
}

// Grant is a content-route capability the adapter has already verified: which object, in which
// tenant. The tenant is in it because nothing else on such a request could say - and a tenant read
// from a path or a header would be a way around row level security (multi-tenancy.md §2.2).
type Grant struct {
	TenantID shared.ID
	MediaID  shared.ID
}

// scope is what the transaction runs as. There is no account: the holder of the URL is whoever the
// staging handed it to, and the byte movement performs no auditable action of its own - the
// staging and the confirmation are the audited steps, and both name a person (audit.md §4).
func (g Grant) scope() persistence.Scope { return persistence.Scope{TenantID: g.TenantID} }

// Receive stores the bytes of a staged object.
//
// The guard runs inline here, which is the difference between the two installations rather than an
// inconsistency: on object storage the bytes never pass this server, so nothing can judge them
// until the confirmation reads them back; on a local one they pass through here, and judging them
// on the way in costs nothing and means unacceptable bytes never touch the disk (T-11, T-17).
// The confirmation judges them again either way, because it is the step that decides READY.
func (h MediaContent) Receive(ctx context.Context, grant Grant, content io.Reader) error {
	object, err := h.pending(ctx, grant)
	if err != nil {
		return err
	}

	inspection, err := h.Guard.Inspect(content, object.ContentType, h.Config.Request.MaxUploadBytes)
	if err != nil {
		return err
	}

	// Outside any transaction: storage is an external dependency, and none of it belongs inside
	// one (observability-reliability.md §8). Nothing is written to the record here - the
	// confirmation is what moves the object to READY, and a route that sealed it would make the
	// confirmation optional.
	return h.Store.Put(ctx, storage.Upload{
		Key:     object.StorageKey,
		Content: inspection.Content,
		// The declared size, so a sender that promised one thing and sent another is refused by
		// the store rather than silently stored short.
		Size:        object.ByteSize,
		ContentType: inspection.ContentType,
	})
}

// Served is one object on its way out, with what the download needs beside the bytes.
type Served struct {
	Content storage.Object
	// FileName is the name the disposition carries, empty when the object arrived without one. It
	// is read off the record rather than taken from the URL, so there is nothing a holder of the
	// URL could tamper with (T-11).
	FileName string
}

// Send opens the bytes of a sealed object. The caller owns the stream and closes it.
func (h MediaContent) Send(ctx context.Context, grant Grant) (Served, error) {
	object, err := h.readable(ctx, grant)
	if err != nil {
		return Served{}, err
	}

	content, err := h.Store.Get(ctx, object.StorageKey)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return Served{}, mediaNotFound(object.ID)
		}
		return Served{}, err
	}
	return Served{Content: content, FileName: object.FileName}, nil
}

// pending reads the object an upload is for, and refuses one that is not waiting for its bytes.
func (h MediaContent) pending(ctx context.Context, grant Grant) (media.Object, error) {
	object, err := h.find(ctx, grant)
	if err != nil {
		return media.Object{}, err
	}
	if object.Status != media.StatusPending {
		// The bytes are already sealed. Overwriting them would change what a confirmed object is
		// after it was judged, which is the one thing READY promises it cannot be.
		return media.Object{}, shared.ErrConflict.WithDetail("media.already_confirmed")
	}
	return object, nil
}

// readable reads the object a download is for, and refuses one that has not been judged yet.
func (h MediaContent) readable(ctx context.Context, grant Grant) (media.Object, error) {
	object, err := h.find(ctx, grant)
	if err != nil {
		return media.Object{}, err
	}
	if object.Status != media.StatusReady {
		// Nothing unjudged is ever served. What is in storage at this point passed no sniff that
		// decided how it may be delivered, and serving it would be exactly the rendering path T-11
		// forbids.
		return media.Object{}, mediaNotFound(object.ID)
	}
	return object, nil
}

func (h MediaContent) find(ctx context.Context, grant Grant) (media.Object, error) {
	var object media.Object

	err := h.UnitOfWork.WithinReadOnly(ctx, grant.scope(), func(ctx context.Context) error {
		found, err := h.Objects.Find(ctx, grant.MediaID)
		if err != nil {
			return err
		}
		object = found
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return media.Object{}, mediaNotFound(grant.MediaID)
		}
		return media.Object{}, err
	}
	if object.DeletedAt != nil {
		// Marked for the reconciliation to remove. A capability minted before that is not a way
		// back in.
		return media.Object{}, mediaNotFound(grant.MediaID)
	}
	return object, nil
}
