// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"context"
	"errors"
	"io"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/storage"
	"github.com/Jersyfi/hubtask/core/port/wopi"
)

const (
	// WopiTokenWindow is how long an access token opens its file. Short, because the document
	// server holds it for the whole editing session and presents it on every call; the person who
	// edits longer opens the file again (security.md §8).
	WopiTokenWindow = 2 * time.Hour

	// MediaEditAction is the audit code of an attempted edit refused (audit.md §4).
	MediaEditAction audit.Action = "media.edit"
)

// WopiHost is the host side of WOPI (UC-MED-06, ADR-0104): it starts an editing session for a
// person, and answers a document server's CheckFileInfo, GetFile, PutFile and lock calls.
//
// Every call is the person's: the access token names them, and each call asks again whether they
// may still read the file - and, to write, whether they may still change an entry or a board that
// holds it (rule 2). The token's write flag only narrows; it never stands in for that question.
type WopiHost struct {
	Read       GetMedia
	Wopi       repository.Wopi
	Store      storage.ObjectStore
	Guard      storage.Guard
	Tokens     wopi.Tokens
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	// SizeLimit bounds a saved file, as an upload is bounded; zero is no limit.
	SizeLimit int64
	// Files is where a document server reaches a file, server to server: the installation's
	// address and the WOPI route, to which the file's id is appended. The token names the
	// workspace, so it is the installation's address and not a workspace's host.
	Files string
	// Server converts; nil converts nothing.
	Server wopi.Server
}

// EditingSession is what a person needs to open a file in the document server: the token, its
// expiry and the server. How the browser hands them over is the launch, which is open (ADR-0104).
type EditingSession struct {
	Token     string
	ExpiresAt time.Time
	Server    media.DocumentServerChoice
	WopiSrc   string
}

// FileInfo is CheckFileInfo's answer.
type FileInfo struct {
	Object   media.Object
	Write    bool
	Version  string
	PersonID shared.ID
}

// StartEditing mints a write token for a person who may change the file.
func (h WopiHost) StartEditing(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (EditingSession, error) {
	if err := actor.RequireScope(mediaWrite); err != nil {
		return EditingSession{}, err
	}
	object, served, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return EditingSession{}, err
	}
	if !media.IsOfficeFile(object.ContentType, object.FileName) {
		return EditingSession{}, shared.ErrValidation.WithDetail("media.not_an_office_file")
	}
	choice, err := h.documentServer(ctx, actor)
	if err != nil {
		return EditingSession{}, err
	}
	if !choice.IsOn() {
		return EditingSession{}, shared.ErrConflict.WithDetail("media.document_server_none")
	}
	if may, err := h.mayWrite(ctx, actor, object, served); err != nil {
		return EditingSession{}, err
	} else if !may {
		return EditingSession{}, shared.ErrForbidden.WithDetail("media.edit_forbidden")
	}
	expires := h.Clock.Now().Add(WopiTokenWindow)
	token := h.Tokens.Issue(wopi.Grant{
		TenantID: actor.TenantID, AccountID: actor.AccountID, MediaID: object.ID, Write: true, ExpiresAt: expires,
	})
	return EditingSession{Token: token, ExpiresAt: expires, Server: choice, WopiSrc: h.Files + object.ID.String()}, nil
}

// ConvertToPDF answers an office file as a PDF converted by the workspace's document server, for
// the viewer (UC-MED-05). The caller closes the stream.
func (h WopiHost) ConvertToPDF(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (io.ReadCloser, error) {
	if err := actor.RequireScope(mediaRead); err != nil {
		return nil, err
	}
	object, _, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return nil, err
	}
	if !media.IsOfficeFile(object.ContentType, object.FileName) {
		return nil, shared.ErrValidation.WithDetail("media.not_an_office_file")
	}
	choice, err := h.documentServer(ctx, actor)
	if err != nil {
		return nil, err
	}
	if !choice.IsOn() || h.Server == nil {
		return nil, wopi.ErrUnsupported()
	}
	stored, err := h.Store.Get(ctx, object.StorageKey)
	if err != nil {
		return nil, err
	}
	defer func() { _ = stored.Content.Close() }()
	// A server that fetches rather than receives reads the file through GetFile, with a read
	// token as short as the conversion.
	fetch := h.Files + object.ID.String() + "/contents?access_token=" + h.Tokens.Issue(wopi.Grant{
		TenantID: actor.TenantID, AccountID: actor.AccountID, MediaID: object.ID,
		ExpiresAt: h.Clock.Now().Add(conversionWindow),
	})
	return h.Server.ConvertToPDF(ctx, choice, wopi.Source{
		Key: object.ID.String() + "-" + versionOf(object), Name: object.FileName,
		Content: stored.Content, Size: stored.Size, FetchAt: fetch,
	})
}

// conversionWindow is how long a conversion's read token holds.
const conversionWindow = 5 * time.Minute

// Editable answers whether the person may open the file in the document server: an office file,
// a server chosen, and a right to change what holds it. Asked for the viewer's *Bearbeiten*.
func (h WopiHost) Editable(
	ctx context.Context, actor appshared.ActorContext, object media.Object, served reach,
) (bool, error) {
	if object.Status != media.StatusReady || !media.IsOfficeFile(object.ContentType, object.FileName) {
		return false, nil
	}
	choice, err := h.documentServer(ctx, actor)
	if err != nil || !choice.IsOn() {
		return false, err
	}
	return h.mayWrite(ctx, actor, object, served)
}

// CheckFileInfo answers what the document server shows of the file and whether it may save.
func (h WopiHost) CheckFileInfo(ctx context.Context, token string, mediaID shared.ID) (FileInfo, error) {
	actor, grant, err := h.caller(token, mediaID)
	if err != nil {
		return FileInfo{}, err
	}
	object, served, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return FileInfo{}, err
	}
	write := false
	if grant.Write {
		if write, err = h.mayWrite(ctx, actor, object, served); err != nil {
			return FileInfo{}, err
		}
	}
	return FileInfo{Object: object, Write: write, Version: versionOf(object), PersonID: actor.AccountID}, nil
}

// GetFile streams the file's bytes. The caller closes them.
func (h WopiHost) GetFile(ctx context.Context, token string, mediaID shared.ID) (storage.Object, error) {
	actor, _, err := h.caller(token, mediaID)
	if err != nil {
		return storage.Object{}, err
	}
	object, _, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return storage.Object{}, err
	}
	return h.Store.Get(ctx, object.StorageKey)
}

// LockRequest is one of the protocol's lock calls.
type LockRequest struct {
	// Op is LOCK, REFRESH_LOCK, UNLOCK, UNLOCK_AND_RELOCK or GET_LOCK.
	Op   string
	Lock string
	Old  string
}

// Locks answers a lock call: the lock that holds afterwards, or a media.LockConflict naming the
// one that holds.
func (h WopiHost) Locks(ctx context.Context, token string, mediaID shared.ID, request LockRequest) (string, error) {
	actor, grant, err := h.caller(token, mediaID)
	if err != nil {
		return "", err
	}
	object, served, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return "", err
	}
	if request.Op != "GET_LOCK" {
		if err := h.requireWrite(ctx, actor, grant, object, served); err != nil {
			return "", err
		}
	}
	now := h.Clock.Now()
	var answer string
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := h.Wopi.Lock(ctx, mediaID)
		if err != nil {
			return err
		}
		switch request.Op {
		case "GET_LOCK":
			if current := media.Held(held, now); current != nil {
				answer = current.ID
			}
			return nil
		case "LOCK", "REFRESH_LOCK":
			lock, err := media.TakeLock(held, request.Lock, now, request.Op == "REFRESH_LOCK")
			if err != nil {
				return err
			}
			answer = lock.ID
			return h.Wopi.PutLock(ctx, mediaID, lock)
		case "UNLOCK_AND_RELOCK":
			lock, err := media.Relock(held, request.Old, request.Lock, now)
			if err != nil {
				return err
			}
			answer = lock.ID
			return h.Wopi.PutLock(ctx, mediaID, lock)
		case "UNLOCK":
			if err := media.Release(held, request.Lock, now); err != nil {
				return err
			}
			return h.Wopi.DropLock(ctx, mediaID)
		default:
			return shared.ErrValidation.WithDetail("wopi.operation_unknown")
		}
	})
	return answer, err
}

// PutFile saves the document server's bytes as the file's new version, under the lock that holds.
// It answers the new version.
func (h WopiHost) PutFile(
	ctx context.Context, token string, mediaID shared.ID, lock string, content io.Reader, size int64,
) (string, error) {
	actor, grant, err := h.caller(token, mediaID)
	if err != nil {
		return "", err
	}
	object, served, err := h.reachable(ctx, actor, mediaID)
	if err != nil {
		return "", err
	}
	if err := h.requireWrite(ctx, actor, grant, object, served); err != nil {
		return "", err
	}
	if size < 0 || (h.SizeLimit > 0 && size > h.SizeLimit) {
		return "", shared.ErrValidation.WithDetail("media.too_large")
	}
	if err := h.underLock(ctx, actor, mediaID, lock, object.ByteSize); err != nil {
		return "", err
	}

	inspection, err := h.Guard.Inspect(content, object.ContentType, h.SizeLimit)
	if err != nil {
		return "", err
	}
	// Outside any transaction: storage is an external dependency (observability-reliability.md §8).
	key := media.StorageKeyFor(actor.TenantID, h.IDs.NewID())
	if err := h.Store.Put(ctx, storage.Upload{
		Key: key, Content: inspection.Content, Size: size, ContentType: inspection.ContentType,
	}); err != nil {
		return "", err
	}

	earlier := object
	earlier.ID = h.IDs.NewID()
	earlier.Usage = media.UsageAttachment
	var saved media.Object
	err = h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Again inside the save: a lock taken by another session between the check and here wins.
		held, err := h.Wopi.Lock(ctx, mediaID)
		if err != nil {
			return err
		}
		if err := media.MayPut(held, lock, object.ByteSize, h.Clock.Now()); err != nil {
			return err
		}
		if _, err := h.Wopi.SaveVersion(ctx, mediaID, repository.Save{
			Earlier: earlier, StorageKey: key, ContentType: inspection.ContentType, ByteSize: size,
			SavedBy: actor.AccountID, SavedAt: h.Clock.Now(),
		}); err != nil {
			return err
		}
		saved, err = h.Read.Objects.Find(ctx, mediaID)
		return err
	})
	if err != nil {
		// The new bytes are nobody's: removed now rather than left for a sweep that never sees them.
		_ = h.Store.Delete(ctx, key)
		if errors.Is(err, shared.ErrNotFound) {
			return "", mediaNotFound(mediaID)
		}
		return "", err
	}
	return versionOf(saved), nil
}

// caller reads the token and makes the person it names the actor: their own rights, never more
// than reading and writing files.
func (h WopiHost) caller(token string, mediaID shared.ID) (appshared.ActorContext, wopi.Grant, error) {
	if token == "" {
		return appshared.ActorContext{}, wopi.Grant{}, wopi.ErrInvalidToken()
	}
	grant, err := h.Tokens.Read(token, mediaID, h.Clock.Now())
	if err != nil {
		return appshared.ActorContext{}, wopi.Grant{}, err
	}
	scopes := []string{mediaRead}
	if grant.Write {
		scopes = append(scopes, mediaWrite)
	}
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: grant.TenantID, AccountID: grant.AccountID, Scopes: scopes,
	}, grant, nil
}

// reachable is GetMedia's question: the file, READY, and readable by the actor.
func (h WopiHost) reachable(
	ctx context.Context, actor appshared.ActorContext, mediaID shared.ID,
) (media.Object, reach, error) {
	object, served, err := h.Read.read(ctx, actor, mediaID)
	if err != nil {
		return media.Object{}, reach{}, err
	}
	if err := h.Read.ensureReadable(ctx, actor, object, served); err != nil {
		return media.Object{}, reach{}, err
	}
	if object.Status != media.StatusReady {
		return media.Object{}, reach{}, shared.ErrConflict.WithDetail("media.not_ready")
	}
	return object, served, nil
}

func (h WopiHost) requireWrite(
	ctx context.Context, actor appshared.ActorContext, grant wopi.Grant, object media.Object, served reach,
) error {
	if !grant.Write {
		return shared.ErrForbidden.WithDetail("media.edit_forbidden")
	}
	may, err := h.mayWrite(ctx, actor, object, served)
	if err != nil {
		return err
	}
	if !may {
		return shared.ErrForbidden.WithDetail("media.edit_forbidden")
	}
	return nil
}

// mayWrite answers whether the person may change an entry or a board that holds the file. A
// page's file is not edited in the document server: a page has its own writers, which this
// module does not know (UC-MED-06 *Where it ends*).
func (h WopiHost) mayWrite(
	ctx context.Context, actor appshared.ActorContext, object media.Object, served reach,
) (bool, error) {
	if len(served.paths) == 0 {
		return false, nil
	}
	allowed, err := h.Read.Reader.Permitted(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Action:     MediaEditAction,
		TokenScope: mediaWrite,
		TargetType: mediaTarget,
		TargetID:   object.ID,
	}, served.paths)
	if err != nil {
		return false, err
	}
	for _, may := range allowed {
		if may {
			return true, nil
		}
	}
	return false, nil
}

func (h WopiHost) underLock(ctx context.Context, actor appshared.ActorContext, mediaID shared.ID, lock string, size int64) error {
	return h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		held, err := h.Wopi.Lock(ctx, mediaID)
		if err != nil {
			return err
		}
		return media.MayPut(held, lock, size, h.Clock.Now())
	})
}

func (h WopiHost) documentServer(ctx context.Context, actor appshared.ActorContext) (media.DocumentServerChoice, error) {
	var choice media.DocumentServerChoice
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		choice, err = h.Wopi.DocumentServer(ctx)
		return err
	})
	return choice, err
}

// versionOf is the file's version as the protocol shows it: it changes with every save, because
// a save gives the file new bytes under a new key.
func versionOf(object media.Object) string {
	return object.StorageKey[len(object.StorageKey)-min(len(object.StorageKey), 36):] + "-" +
		strconv.FormatInt(object.ByteSize, 10)
}
