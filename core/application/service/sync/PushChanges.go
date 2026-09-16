// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"log/slog"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The push (N-04, offline-sync.md §3.2): a device's queue of mutations, applied one at a time
// through the ordinary use cases as the pushing person, each answered on its own.
//
// Not a catalogue use case, for the pull's reason - and what it *applies* is never its own code
// path: an `ITEM_CREATE` is `CreateWorkItem` performed as the pushing person, exactly as accepting
// a suggestion is the ordinary use case performed as the accepting person (J-05). A push grants
// nothing, and somebody who could not make a change by hand cannot make it by pushing.

// pushScope is the token scope a push needs at the door. Every use case it performs checks its
// own; this is the bound on the door itself, so that a read-only token is refused before its first
// mutation rather than five hundred times.
const pushScope = "items:write"

// PushLimit is the most mutations one push may carry (api/openapi.yaml, SyncPushRequest).
const PushLimit = 500

// Catalogue is the slice of the use case registry a push writes through: run one use case by
// name. The only way this package writes anything.
type Catalogue interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Mutation is one thing a device did offline, in the contract's shape.
type Mutation struct {
	OpID        shared.ID
	Kind        domain.MutationKind
	ItemID      shared.ID
	BaseVersion *int
	// Fields are per-field changes, each under its own clock (ITEM_PATCH).
	Fields map[string]FieldChange
	// Set and Element name a set change (SET_ADD, SET_REMOVE); HLC is its tag, and a creation's
	// or a deletion's reading.
	Set     string
	Element shared.ID
	HLC     string
	Payload map[string]any
}

// FieldChange is one field's new value and the reading it was written under.
type FieldChange struct {
	Value any
	HLC   string
}

// PushRequest is a device's queue.
type PushRequest struct {
	DeviceID    shared.ID
	Platform    string
	DisplayName string
	Mutations   []Mutation
}

// Result is what became of one mutation.
type Result struct {
	OpID     shared.ID
	Result   domain.ResultKind
	EntityID shared.ID
	// ServerState is the object as the server holds it after the mutation, on APPLIED and
	// MERGED, so that a client adopts the server's answer (offline-sync.md §9, requirement 5).
	ServerState map[string]any
	Conflict    *ConflictDetail
	Error       *ResultError
}

// ConflictDetail is both versions of a free-text field that lost (offline-sync.md §5).
type ConflictDetail struct {
	Field              string
	Mine               any
	Theirs             any
	PreservedCommentID shared.ID
}

// ResultError is why a mutation was rejected: the use case's own code, which the client
// localises the way it localises any problem document.
type ResultError struct {
	Code        string
	MessageCode string
}

// PushResponse is the answer to a push: one result per mutation in the order they were sent, and
// where the log stands afterwards, saving a pull.
type PushResponse struct {
	Results    []Result
	Cursor     Position
	ServerTime time.Time
}

// StepRecorder is the slice of the history the push writes through: one step, named by the entry
// it is about (work.ActivityJournal.RecordStep).
type StepRecorder interface {
	RecordStep(ctx context.Context, actor appshared.ActorContext, tenantID, itemID, collectionID shared.ID,
		verb activity.Verb, changeSet map[string]any, at time.Time) error
}

// DisplacedFiler files the version of a free-text field that lost a merge as a system comment
// (work.AddComment.FileDisplaced).
type DisplacedFiler interface {
	FileDisplaced(ctx context.Context, actor appshared.ActorContext, itemID shared.ID, body string,
		params map[string]string) (work.Comment, error)
}

// applier applies one mutation of a kind through the catalogue and answers the result, or an
// error that ends the push - never a client's refusal, which is a result.
type applier func(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error)

// PushChanges serves `POST /sync:push`.
type PushChanges struct {
	Stream     StreamChanges
	Devices    repository.Devices
	Ops        repository.OpLog
	Tombstones repository.Tombstones
	// Clocks is the server's clock per field, which ITEM_PATCH decides against (N-05).
	Clocks    repository.FieldClocks
	Catalogue Catalogue
	// Activity writes the two steps a merge owes the history - a change with meaning that lost,
	// and a merge that displaced free text (N-06). Nil writes neither, a test's convenience.
	Activity StepRecorder
	// Displaced files the version of a free-text field that lost as a comment (§5). Nil files
	// nothing, and the conflict still carries both values.
	Displaced DisplacedFiler
	// Sets is where each set's tags are read for the OR-set merge (N-07).
	Sets Sets
	// Skew is how far a device's clock may stand from the server's before its readings are
	// replaced by server readings (offline-sync.md §4.1). Zero means the contract's five minutes.
	Skew time.Duration
}

// DefaultSkew is §4.1's default.
const DefaultSkew = 5 * time.Minute

// Push applies the queue.
func (p PushChanges) Push(
	ctx context.Context, actor appshared.ActorContext, request PushRequest,
) (PushResponse, error) {
	if err := actor.RequireScope(pushScope); err != nil {
		return PushResponse{}, err
	}
	if len(request.Mutations) > PushLimit {
		return PushResponse{}, shared.ErrValidation.
			WithDetail("sync.push_too_large").
			WithParams(map[string]string{"max": strconv.Itoa(PushLimit)}).
			WithFields(shared.FieldError{Path: "/mutations", Code: "sync.push_too_large"})
	}
	if err := p.touch(ctx, actor, request); err != nil {
		return PushResponse{}, err
	}

	// Everything applied under this context names the device: that is what lets the device skip
	// its own echo on the next pull (offline-sync.md §10).
	ctx = appshared.ContextWithDevice(ctx, request.DeviceID)

	results := make([]Result, 0, len(request.Mutations))
	for _, mutation := range request.Mutations {
		result, err := p.apply(ctx, actor, request.DeviceID, mutation)
		if err != nil {
			// Not a refusal - a refusal is a result - but a dependency that could not answer.
			// The results so far are committed and answered nowhere; the client pushes the
			// whole queue again and the operation log makes the repeat of those exact.
			return PushResponse{}, err
		}
		results = append(results, result)
	}

	latest, err := p.Stream.latest(ctx, actor)
	if err != nil {
		return PushResponse{}, err
	}
	return PushResponse{
		Results:    results,
		Cursor:     latest,
		ServerTime: latest.IssuedAt,
	}, nil
}

// touch registers the device or records its contact, the pull's rule (N-03). A push carries no
// cursor, so the position recorded is whatever the row already holds.
func (p PushChanges) touch(ctx context.Context, actor appshared.ActorContext, request PushRequest) error {
	if p.Devices == nil {
		return nil
	}
	contact, err := domain.Contact{
		DeviceID: request.DeviceID, AccountID: actor.AccountID, CredentialID: actor.TokenID,
		Platform: request.Platform, DisplayName: request.DisplayName, Now: p.Stream.Clock.Now(),
	}.Validate()
	if err != nil {
		return err
	}
	return p.Stream.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		_, err := p.Devices.Touch(ctx, contact)
		return err
	})
}

// apply answers one mutation: from the operation log if it was seen before, otherwise by
// performing it and recording what became of it.
func (p PushChanges) apply(
	ctx context.Context, actor appshared.ActorContext, deviceID shared.ID, m Mutation,
) (Result, error) {
	if m.OpID.IsZero() || !m.OpID.IsUUIDv7() {
		// Without an operation identifier there is nothing to be idempotent about, and the
		// mutation cannot be recorded; refused without a record, which is the honest answer.
		return rejected(m, shared.ErrValidation.WithDetail("sync.op_id_required")), nil
	}
	if !m.Kind.Valid() {
		return rejected(m, shared.ErrValidation.
			WithDetail("sync.kind_unknown").
			WithParams(map[string]string{"kind": string(m.Kind)})), nil
	}

	// Seen before: the answer is the answer (SY-7).
	var seen *Result
	err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		record, found, err := p.Ops.Find(ctx, m.OpID)
		if err != nil || !found {
			return err
		}
		stored := resultFromRecord(record)
		seen = &stored
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	if seen != nil {
		return *seen, nil
	}

	bounded, err := p.bound(ctx, m)
	if err != nil {
		return rejected(m, err), nil
	}

	apply, served := p.appliers()[bounded.Kind]
	if !served {
		// A kind the contract names and this build does not apply yet. Not recorded: the client
		// keeps the mutation and a later build applies it.
		return rejected(m, shared.ErrUnavailable.
			WithDetail("sync.kind_unavailable").
			WithParams(map[string]string{"kind": string(m.Kind)})), nil
	}

	// Judged before anything is read or written: what the mutation names is either a refusal
	// worth recording or a "not yet" the client keeps, and neither needs a transaction.
	if err := p.judge(bounded); err != nil {
		if !refusal(err) {
			return rejected(m, err), nil
		}
		return p.record(ctx, actor, deviceID, rejected(m, err))
	}

	held, err := p.purged(ctx, actor, bounded)
	if err != nil {
		return Result{}, err
	}
	if held {
		return p.record(ctx, actor, deviceID, rejected(m, shared.ErrGone.WithDetail("sync.gone")))
	}

	// The effect and its record commit together, so that a push that dies halfway leaves neither.
	var result Result
	err = p.Stream.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		result, err = apply(ctx, actor, bounded)
		if err != nil {
			return err
		}
		return p.Ops.Record(ctx, recordOf(deviceID, result, p.Stream.Clock.Now()))
	})
	if err == nil {
		return result, nil
	}
	if !refusal(err) {
		return Result{}, err
	}
	// The use case refused. The transaction it ran in is gone with whatever it half-did; the
	// refusal is recorded in a fresh one, so that the repeat answers the same refusal rather
	// than trying again (SY-7).
	return p.record(ctx, actor, deviceID, rejected(m, err))
}

// record writes a result's record in its own transaction and answers the result.
func (p PushChanges) record(
	ctx context.Context, actor appshared.ActorContext, deviceID shared.ID, result Result,
) (Result, error) {
	err := p.Stream.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return p.Ops.Record(ctx, recordOf(deviceID, result, p.Stream.Clock.Now()))
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// bound parses every reading the mutation carries and applies §4.1's rule to each; a reading that
// was moved is logged with the device and the drift, never the content.
func (p PushChanges) bound(ctx context.Context, m Mutation) (Mutation, error) {
	skew := p.Skew
	if skew == 0 {
		skew = DefaultSkew
	}
	now := p.Stream.Clock.Now()
	boundOne := func(raw string) (string, error) {
		if raw == "" {
			return "", nil
		}
		reading, err := shared.ParseHLC(raw)
		if err != nil {
			return "", err
		}
		bounded, drift := domain.Bound(reading, now, skew)
		if drift > 0 {
			// The drift and nothing of the request: the device, the operation and the request
			// identifier are request values, and a log line shaped by one is the injection T-06
			// names. The correlating handler adds the request identifier from the context, the
			// request log beside it names the route and the actor, and the device is on the
			// operation log's row.
			slog.WarnContext(ctx, "a device's clock reading was bounded to server time",
				"drift", drift.String())
		}
		return bounded.String(), nil
	}

	var err error
	if m.HLC, err = boundOne(m.HLC); err != nil {
		return Mutation{}, err
	}
	if len(m.Fields) > 0 {
		fields := make(map[string]FieldChange, len(m.Fields))
		for name, field := range m.Fields {
			if field.HLC, err = boundOne(field.HLC); err != nil {
				return Mutation{}, err
			}
			fields[name] = field
		}
		m.Fields = fields
	}
	return m, nil
}

// purged asks whether the entry the mutation names is a tombstone (offline-sync.md §7): a
// mutation on a purged entry, and a creation under a purged identifier, are `sync.gone`.
func (p PushChanges) purged(ctx context.Context, actor appshared.ActorContext, m Mutation) (bool, error) {
	if p.Tombstones == nil || m.ItemID.IsZero() {
		return false, nil
	}
	var held bool
	err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		held, err = p.Tombstones.Holds(ctx, "work_item", m.ItemID)
		return err
	})
	return held, err
}

// judge answers what can be said about a mutation from its shape alone - the field names of a
// patch - before it is applied. A refusal is recorded like any other; an "unavailable" is not,
// so that a later build applies what this one does not (patchable).
func (p PushChanges) judge(m Mutation) error {
	switch m.Kind {
	case domain.ItemPatch:
		for field := range m.Fields {
			if err := patchable(field); err != nil {
				return err
			}
		}
	case domain.SetAdd, domain.SetRemove:
		if m.Set == string(work.SetWatchers) {
			// In the contract's enum, and no use case writes a watcher yet: "not yet" rather than
			// "never", so that a client keeps the mutation for a build that does.
			return shared.ErrUnavailable.
				WithDetail("sync.set_unavailable").
				WithParams(map[string]string{"set": m.Set})
		}
		if _, served := p.Sets.owner(work.SetName(m.Set)); !served {
			return shared.ErrValidation.
				WithDetail("sync.set_unknown").
				WithParams(map[string]string{"set": m.Set}).
				WithFields(shared.FieldError{Path: "/set", Code: "sync.set_unknown"})
		}
	}
	return nil
}

// appliers is the table of kinds this build applies. A table in code rather than a name on the
// mutation, for the suggestion package's reason: a stored use case name would be a stored
// capability. Every kind the contract names is here.
func (p PushChanges) appliers() map[domain.MutationKind]applier {
	return map[domain.MutationKind]applier{
		domain.ItemCreate: p.create,
		domain.ItemPatch:  p.patch,
		domain.ItemDelete: p.trash,
		domain.Move:       p.move,
		domain.SetAdd:     p.setChange,
		domain.SetRemove:  p.setChange,
		domain.CommentAdd: p.comment,
	}
}

// create is `CreateWorkItem` with the client's identifier: the payload is the use case's input,
// and the identifier is written after it so that the payload cannot move it.
func (p PushChanges) create(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	in := inputOf(m.Payload)
	in["id"] = m.ItemID.String()
	out, err := p.Catalogue.Invoke(ctx, "CreateWorkItem", actor, in)
	if err != nil {
		return Result{}, err
	}
	return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.ItemID, ServerState: out}, nil
}

// trash is `TrashWorkItem`. A device deleting an entry that is already in the trash is somebody
// making sure, and the use case answers it as it answers a repeated call over the API.
func (p PushChanges) trash(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	out, err := p.Catalogue.Invoke(ctx, "TrashWorkItem", actor, usecase.Input{"item_id": m.ItemID.String()})
	if err != nil {
		return Result{}, err
	}
	return Result{OpID: m.OpID, Result: domain.Applied, EntityID: m.ItemID, ServerState: out}, nil
}

// comment is `AddComment` with the client's identifier for the comment, taken from the payload's
// `id`, on the entry the mutation names.
func (p PushChanges) comment(ctx context.Context, actor appshared.ActorContext, m Mutation) (Result, error) {
	if m.ItemID.IsZero() {
		return Result{}, shared.ErrValidation.WithDetail("sync.item_required")
	}
	in := inputOf(m.Payload)
	in["item_id"] = m.ItemID.String()
	out, err := p.Catalogue.Invoke(ctx, "AddComment", actor, in)
	if err != nil {
		return Result{}, err
	}
	commentID, _ := out["id"].(string)
	return Result{OpID: m.OpID, Result: domain.Applied, EntityID: shared.ID(commentID), ServerState: out}, nil
}

// inputOf is the payload as the catalogue's input. Copied, so that what an applier adds never
// reaches the mutation the results are answered from.
func inputOf(payload map[string]any) usecase.Input {
	in := make(usecase.Input, len(payload)+1)
	for field, value := range payload {
		in[field] = value
	}
	return in
}

// refusal reports whether an error is the client's - a refusal that is a result rather than a
// dependency that could not answer.
func refusal(err error) bool {
	switch shared.AsError(err).Category {
	case shared.CategoryValidation, shared.CategoryNotFound, shared.CategoryConflict,
		shared.CategoryForbidden, shared.CategoryGone:
		return true
	}
	return false
}

// rejected is the result of a refusal.
func rejected(m Mutation, err error) Result {
	failure := shared.AsError(err)
	code := failure.DetailCode
	if code == "" {
		code = failure.Code
	}
	return Result{
		OpID: m.OpID, Result: domain.Rejected, EntityID: m.ItemID,
		Error: &ResultError{Code: failure.Code, MessageCode: code},
	}
}

// recordOf is the result as the operation log keeps it.
func recordOf(deviceID shared.ID, result Result, at time.Time) repository.OpRecord {
	return repository.OpRecord{
		OpID: result.OpID, DeviceID: deviceID, Result: result.Result, EntityID: result.EntityID,
		Response: resultOutput(result), AppliedAt: at,
	}
}

// resultOutput is the result in the contract's field names - what the operation log stores and
// what a repeat answers, so that the two cannot differ.
func resultOutput(result Result) map[string]any {
	out := map[string]any{
		"op_id":        result.OpID.String(),
		"result":       string(result.Result),
		"entity_id":    nil,
		"server_state": nil,
		"conflict":     nil,
		"error":        nil,
	}
	if !result.EntityID.IsZero() {
		out["entity_id"] = result.EntityID.String()
	}
	if result.ServerState != nil {
		out["server_state"] = result.ServerState
	}
	if result.Conflict != nil {
		conflict := map[string]any{
			"field": result.Conflict.Field, "mine": result.Conflict.Mine, "theirs": result.Conflict.Theirs,
			"preserved_comment_id": nil,
		}
		if !result.Conflict.PreservedCommentID.IsZero() {
			conflict["preserved_comment_id"] = result.Conflict.PreservedCommentID.String()
		}
		out["conflict"] = conflict
	}
	if result.Error != nil {
		out["error"] = map[string]any{"code": result.Error.Code, "message_code": result.Error.MessageCode}
	}
	return out
}

// resultFromRecord reads a stored result back. The record's own columns are the truth for the
// fields they hold; the response carries what the columns do not.
func resultFromRecord(record repository.OpRecord) Result {
	result := Result{OpID: record.OpID, Result: record.Result, EntityID: record.EntityID}
	if state, ok := record.Response["server_state"].(map[string]any); ok {
		result.ServerState = state
	}
	if conflict, ok := record.Response["conflict"].(map[string]any); ok {
		detail := &ConflictDetail{Mine: conflict["mine"], Theirs: conflict["theirs"]}
		detail.Field, _ = conflict["field"].(string)
		if id, ok := conflict["preserved_comment_id"].(string); ok {
			detail.PreservedCommentID = shared.ID(id)
		}
		result.Conflict = detail
	}
	if failure, ok := record.Response["error"].(map[string]any); ok {
		code, _ := failure["code"].(string)
		message, _ := failure["message_code"].(string)
		result.Error = &ResultError{Code: code, MessageCode: message}
	}
	return result
}

// Encode is how the presentation layer spells the cursor after a push.
func (p PushChanges) Encode(position Position) string { return p.Stream.Encode(position) }
