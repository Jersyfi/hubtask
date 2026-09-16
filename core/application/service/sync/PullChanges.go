// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Depth says how far below a scope's container a device wants to hold, counted in containers
// (offline-sync.md §3.1). The values are the contract's.
type Depth string

const (
	// DepthSelf is the container and what it holds directly: its entries, labels and buckets. A
	// change is filed under the container of the object it concerns, so an entry's change is in
	// its collection's SELF and not in its hub's.
	DepthSelf Depth = "SELF"
	// DepthChildren adds the container's child containers and what they hold.
	DepthChildren Depth = "CHILDREN"
	// DepthSubtree is every level below. The hierarchy has two levels of containers today, so
	// CHILDREN and SUBTREE coincide; they are two values because the contract promised both, and
	// a third level would part them without a client noticing.
	DepthSubtree Depth = "SUBTREE"
)

// Valid reports whether the depth is one the contract names.
func (d Depth) Valid() bool {
	switch d {
	case DepthSelf, DepthChildren, DepthSubtree:
		return true
	}
	return false
}

// Scope is one container a device wants to hold, to a depth. No scope at all means everything
// the caller may read; several are a union.
type Scope struct {
	ContainerID shared.ID
	Depth       Depth
}

// covers reports whether a change filed under the container is inside this scope. A change filed
// under none is workspace-wide - a template defined at the workspace (#626) - and stands above
// every scope, so every scope holds it: a device holding one hub still needs the templates it
// can apply there.
func (s Scope) covers(container work.Container) bool {
	if container.ID.IsZero() {
		return true
	}
	if container.ID == s.ContainerID {
		return true
	}
	if s.Depth == DepthSelf {
		return false
	}
	return container.ParentID == s.ContainerID
}

// PullRequest is one page's worth of question: which device asks, where it stands, what it wants
// to hold, and how much it can take at once.
type PullRequest struct {
	DeviceID shared.ID
	// Platform and DisplayName are what the device says about itself, both optional (N-03).
	Platform    string
	DisplayName string
	// Cursor is empty for an initial synchronisation.
	Cursor string
	Scopes []Scope
	// Limit is the page size. Zero means the contract's default; more than the maximum is refused
	// rather than clamped, because a client that asked for more than the contract allows has a
	// defect worth hearing about.
	Limit int
}

// Page is one answer: the records, where the walk now stands, and the two facts a client needs
// beside them - the server's clock, to bound its own, and the window, to know when a cursor it
// holds has expired.
type Page struct {
	Records    []Record
	Cursor     Position
	More       bool
	ServerTime time.Time
	Window     time.Duration
}

const (
	// PullLimitDefault and PullLimitMax are the contract's (api/openapi.yaml, SyncPullRequest).
	PullLimitDefault = 500
	PullLimitMax     = 2000
)

// PullChanges serves `POST /sync:pull`, the paged form of the stream (N-01, ADR-0021).
//
// Not a catalogue use case, for the stream's reason: the catalogue lists what a person, an agent
// or a rule can ask for, and a pull is a connection being served in pages rather than an action -
// an agent has no offline queue, and a rule that pulled would be reading its own effects. What a
// push applies, by contrast, is always a catalogue use case (N-04).
//
// The reading, the cursor and the per-record permission are the stream's, shared rather than
// copied - which is what makes the stream an accelerator over this rather than a second source of
// truth: the same records, the same order, the same cursor.
type PullChanges struct {
	Stream StreamChanges
	// Devices registers the device on its first contact and records every one after (N-03).
	// Nil registers nothing, which is a test's convenience and not an installation's.
	Devices repository.Devices
	// Snapshot reads the current state for an initial synchronisation (N-02). Nil means this
	// installation does not serve one, and a device with no cursor is told so.
	Snapshot repository.Snapshot
}

// Pull answers one page.
func (p PullChanges) Pull(
	ctx context.Context, actor appshared.ActorContext, request PullRequest,
) (Page, error) {
	if request.DeviceID.IsZero() {
		return Page{}, shared.ErrValidation.
			WithDetail("sync.device_required").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_required"})
	}
	limit, err := pullLimit(request.Limit)
	if err != nil {
		return Page{}, err
	}
	keep, err := scopeFilter(request.Scopes)
	if err != nil {
		return Page{}, err
	}

	from, err := p.resume(ctx, actor, request.Cursor)
	if err != nil {
		return Page{}, err
	}
	if err := p.touch(ctx, actor, request, from); err != nil {
		return Page{}, err
	}

	var batch Batch
	if from.Walking() {
		batch, err = p.walk(ctx, actor, from, limit, keep)
	} else {
		batch, err = p.Stream.page(ctx, actor, from, limit, keep)
	}
	if err != nil {
		return Page{}, err
	}
	return Page{
		Records:    batch.Records,
		Cursor:     batch.Cursor,
		More:       batch.More,
		ServerTime: p.Stream.Clock.Now(),
		Window:     p.Stream.Window,
	}, nil
}

// touch registers the device or records its contact, before anything is read: a device the
// account may not use gets no page. The position recorded is the one the request stands at -
// where the device last got to - and the credential is the request's own, so that forgetting the
// device later ends the sign-in it synchronised under (offline-sync.md §6).
func (p PullChanges) touch(
	ctx context.Context, actor appshared.ActorContext, request PullRequest, from Position,
) error {
	if p.Devices == nil {
		return nil
	}
	contact, err := domain.Contact{
		DeviceID: request.DeviceID, AccountID: actor.AccountID, CredentialID: actor.TokenID,
		Platform: request.Platform, DisplayName: request.DisplayName,
		LastSeq: from.Seq, Now: p.Stream.Clock.Now(),
	}.Validate()
	if err != nil {
		return err
	}
	return p.Stream.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, err := p.Devices.Touch(ctx, contact)
		return err
	})
}

// resume decides where a pull starts: a fresh walk for a device with no cursor, or wherever the
// cursor says - in the log, or in the middle of a walk.
func (p PullChanges) resume(
	ctx context.Context, actor appshared.ActorContext, cursor string,
) (Position, error) {
	if err := actor.RequireScope(streamScope); err != nil {
		return Position{}, err
	}
	if cursor != "" {
		return p.Stream.decode(ctx, actor, cursor)
	}
	if p.Snapshot == nil {
		// Refused rather than answered with an empty page and a fresh cursor: that page would be
		// a client believing it is current when it holds nothing, which is the one state a
		// synchronisation must never leave a device in.
		return Position{}, shared.ErrUnavailable.WithDetail("sync.initial_sync_unavailable")
	}
	// The log's position first, then the walk: a change that lands while the walk runs is past
	// this position and the first delta delivers it. Taken the other way round, a change landing
	// between the last page and the position would be in neither.
	latest, err := p.Stream.latest(ctx, actor)
	if err != nil {
		return Position{}, err
	}
	latest.Kind = walkKinds[0]
	return latest, nil
}

// pullLimit settles the page size against the contract's bounds.
func pullLimit(limit int) (int, error) {
	switch {
	case limit == 0:
		return PullLimitDefault, nil
	case limit < 0, limit > PullLimitMax:
		return 0, shared.ErrValidation.
			WithDetail("sync.limit_out_of_range").
			WithParams(map[string]string{"max": strconv.Itoa(PullLimitMax)}).
			WithFields(shared.FieldError{Path: "/limit", Code: "sync.limit_out_of_range"})
	}
	return limit, nil
}

// scopeFilter turns the scopes into the page reader's filter: nil for no scope, a union otherwise.
func scopeFilter(scopes []Scope) (func(work.Container) bool, error) {
	if len(scopes) == 0 {
		return nil, nil
	}
	for i, scope := range scopes {
		if scope.ContainerID.IsZero() {
			return nil, shared.ErrValidation.
				WithDetail("sync.scope_container_required").
				WithFields(shared.FieldError{Path: "/scopes", Code: "sync.scope_container_required"})
		}
		if scope.Depth == "" {
			// The contract's default, applied here rather than at the door so that every caller
			// of this service - and there will be a second one in hubctl - reads it the same way.
			scopes[i].Depth = DepthSubtree
			continue
		}
		if !scope.Depth.Valid() {
			return nil, shared.ErrValidation.
				WithDetail("sync.scope_depth_unknown").
				WithParams(map[string]string{"depth": string(scope.Depth)}).
				WithFields(shared.FieldError{Path: "/scopes", Code: "sync.scope_depth_unknown"})
		}
	}
	return func(container work.Container) bool {
		for _, scope := range scopes {
			if scope.covers(container) {
				return true
			}
		}
		return false
	}, nil
}

// Encode is how the presentation layer spells a position on the wire - the stream's codec, so that
// a cursor a pull handed out resumes a stream and the other way round.
func (p PullChanges) Encode(position Position) string { return p.Stream.Encode(position) }
