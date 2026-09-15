// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	syncservice "github.com/Jersyfi/hubtask/core/application/service/sync"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	syncdomain "github.com/Jersyfi/hubtask/core/domain/model/sync"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// Puller is the slice of the pull service this controller drives: one page in, one page out, and
// how a position is spelled on the wire. The same reason StreamController holds an interface -
// this is the whole of what the transport may do with the log, and a controller test needs no
// database to prove the mapping.
type Puller interface {
	Pull(ctx context.Context, actor appshared.ActorContext, request syncservice.PullRequest) (
		syncservice.Page, error)
	Encode(position syncservice.Position) string
}

// SyncSignals is the slice of the metrics adapter the pull reports through. Records delivered,
// beside the stream's own counter: against the change log's growth the two together say whether
// devices are keeping up, and apart they say by which door.
type SyncSignals interface {
	PullRecords(ctx context.Context, count int)
}

// SyncController serves `POST /sync:pull` (N-01). Not a catalogue entry, for the stream's reason:
// a pull is a connection served in pages rather than an action a rule or an agent could invoke.
type SyncController struct {
	Pull    Puller
	Signals SyncSignals
	// Push serves `POST /sync:push` (N-04). Nil leaves the route answering the pending 404.
	Push        Pusher
	PushSignals PushSignals
}

// SyncPull answers one page of the delta.
func (c SyncController) SyncPull(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	actor, _ := appshared.ActorFrom(r.Context())
	if !actor.IsAuthenticated() {
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}

	var body openapi.SyncPullRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	page, err := c.Pull.Pull(r.Context(), actor, pullRequest(body))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if c.Signals != nil && len(page.Records) > 0 {
		c.Signals.PullRecords(r.Context(), len(page.Records))
	}

	writeJSON(w, r, http.StatusOK, pullResponse(page, c.Pull.Encode(page.Cursor)))
}

// pullRequest maps the body onto the service's request. Nothing is defaulted here: the page size
// and the depth a client did not state are the service's to settle, so that hubctl reads them the
// same way.
func pullRequest(body openapi.SyncPullRequest) syncservice.PullRequest {
	request := syncservice.PullRequest{DeviceID: shared.ID(body.DeviceId.String())}
	if body.Platform != nil {
		request.Platform = *body.Platform
	}
	if body.DisplayName != nil {
		request.DisplayName = *body.DisplayName
	}
	if body.Cursor != nil {
		request.Cursor = *body.Cursor
	}
	if body.Limit != nil {
		request.Limit = *body.Limit
	}
	if body.Scopes != nil {
		for _, scope := range *body.Scopes {
			mapped := syncservice.Scope{}
			if scope.ContainerId != nil {
				mapped.ContainerID = shared.ID(scope.ContainerId.String())
			}
			if scope.Depth != nil {
				mapped.Depth = syncservice.Depth(*scope.Depth)
			}
			request.Scopes = append(request.Scopes, mapped)
		}
	}
	return request
}

// pullResponse is the wire shape of one page: the contract's `SyncPullResponse`.
func pullResponse(page syncservice.Page, cursor string) openapi.SyncPullResponse {
	changes := make([]openapi.SyncChange, 0, len(page.Records))
	for _, record := range page.Records {
		changes = append(changes, syncChange(record))
	}
	serverTime := page.ServerTime.UTC()
	windowDays := int(page.Window / (24 * time.Hour))
	return openapi.SyncPullResponse{
		Changes:             changes,
		Cursor:              cursor,
		HasMore:             page.More,
		ServerTime:          &serverTime,
		TombstoneWindowDays: &windowDays,
	}
}

// syncChange is one record as a client receives it - the same facts the stream's event carries
// (changePayload), in the contract's typed shape.
func syncChange(record syncservice.Record) openapi.SyncChange {
	change := openapi.SyncChange{
		Op:       openapi.SyncChangeOp(record.Op),
		Entity:   record.Entity,
		EntityId: uuidValue(record.EntityID.String()),
	}
	if !record.ContainerID.IsZero() {
		change.ContainerId = uuidPointer(record.ContainerID)
	}
	if !record.HLC.IsZero() {
		hlc := record.HLC.String()
		change.Hlc = &hlc
	}
	occurred := record.OccurredAt.UTC()
	change.OccurredAt = &occurred
	if !record.ActorID.IsZero() {
		change.ActorId = uuidPointer(record.ActorID)
	}
	if !record.DeviceID.IsZero() {
		change.DeviceId = uuidPointer(record.DeviceID)
	}
	// Absent rather than null on a deletion, as on the stream: a tombstone carries no content by
	// design, and `null` would say "the change set is empty", which is a different statement.
	if record.Payload != nil {
		payload := record.Payload
		change.Payload = &payload
	}
	return change
}

func uuidPointer(id shared.ID) *openapi_types.UUID {
	value := uuidValue(id.String())
	return &value
}

const (
	listSyncDevicesUseCase  = "ListSyncDevices"
	forgetSyncDeviceUseCase = "ForgetSyncDevice"
)

// ListSyncDevices answers GET /sync/devices (N-03). Written out rather than through the identity
// helper, for the reason ListSessions is.
func (c *RestController) ListSyncDevices(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listSyncDevicesUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	devices := make([]openapi.SyncDevice, 0, len(rows))
	for _, row := range rows {
		devices = append(devices, deviceResponse(row))
	}
	writeJSON(w, r, http.StatusOK, devices)
}

// ForgetSyncDevice answers DELETE /sync/devices/{deviceId}.
func (c *RestController) ForgetSyncDevice(w http.ResponseWriter, r *http.Request, deviceID openapi.DeviceId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), forgetSyncDeviceUseCase, actor, usecase.Input{
			"device_id": deviceID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

func deviceResponse(row usecase.Output) openapi.SyncDevice {
	blocked, _ := row["blocked"].(bool)
	created := timeValue(row["created_at"])
	return openapi.SyncDevice{
		Id:          uuidValue(row.String("id")),
		Platform:    optionalTextField(row["platform"]),
		DisplayName: optionalTextField(row["display_name"]),
		LastSeenAt:  optionalTimeField(row["last_seen_at"]),
		LastCursor:  optionalTextField(row["last_cursor"]),
		Blocked:     blocked,
		CreatedAt:   &created,
	}
}

// Pusher is the slice of the push service this controller drives.
type Pusher interface {
	Push(ctx context.Context, actor appshared.ActorContext, request syncservice.PushRequest) (
		syncservice.PushResponse, error)
	Encode(position syncservice.Position) string
}

// PushSignals counts what a push did, by result: the closed set the contract names, never an
// identifier.
type PushSignals interface {
	PushResult(ctx context.Context, result string)
}

// SyncPush applies a device's queue (N-04).
func (c SyncController) SyncPush(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	actor, _ := appshared.ActorFrom(r.Context())
	if !actor.IsAuthenticated() {
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}
	if c.Push == nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("route.operation_not_available"), requestID)
		return
	}

	var body openapi.SyncPushRequest
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	response, err := c.Push.Push(r.Context(), actor, pushRequest(body))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	if c.PushSignals != nil {
		for _, result := range response.Results {
			c.PushSignals.PushResult(r.Context(), string(result.Result))
		}
	}

	writeJSON(w, r, http.StatusOK, pushResponse(response, c.Push.Encode(response.Cursor)))
}

// pushRequest maps the body onto the service's request, field for field and defaulting nothing.
func pushRequest(body openapi.SyncPushRequest) syncservice.PushRequest {
	request := syncservice.PushRequest{
		DeviceID:  shared.ID(body.DeviceId.String()),
		Mutations: make([]syncservice.Mutation, 0, len(body.Mutations)),
	}
	if body.Platform != nil {
		request.Platform = *body.Platform
	}
	if body.DisplayName != nil {
		request.DisplayName = *body.DisplayName
	}
	for _, m := range body.Mutations {
		mutation := syncservice.Mutation{
			OpID:        shared.ID(m.OpId.String()),
			Kind:        syncdomain.MutationKind(m.Kind),
			BaseVersion: m.BaseVersion,
		}
		if m.ItemId != nil {
			mutation.ItemID = shared.ID(m.ItemId.String())
		}
		if m.Hlc != nil {
			mutation.HLC = *m.Hlc
		}
		if m.Set != nil {
			mutation.Set = string(*m.Set)
		}
		if m.Element != nil {
			mutation.Element = shared.ID(m.Element.String())
		}
		if m.Payload != nil {
			mutation.Payload = *m.Payload
		}
		if m.Fields != nil {
			mutation.Fields = make(map[string]syncservice.FieldChange, len(*m.Fields))
			for name, field := range *m.Fields {
				change := syncservice.FieldChange{Value: field.Value}
				if field.Hlc != nil {
					change.HLC = *field.Hlc
				}
				mutation.Fields[name] = change
			}
		}
		request.Mutations = append(request.Mutations, mutation)
	}
	return request
}

// pushResponse is the wire shape: the contract's `SyncPushResponse`.
func pushResponse(response syncservice.PushResponse, cursor string) openapi.SyncPushResponse {
	results := make([]openapi.SyncMutationResult, 0, len(response.Results))
	for _, result := range response.Results {
		results = append(results, mutationResult(result))
	}
	serverTime := response.ServerTime.UTC()
	return openapi.SyncPushResponse{Results: results, Cursor: &cursor, ServerTime: &serverTime}
}

func mutationResult(result syncservice.Result) openapi.SyncMutationResult {
	out := openapi.SyncMutationResult{
		OpId:   uuidValue(result.OpID.String()),
		Result: openapi.SyncMutationResultResult(result.Result),
	}
	if !result.EntityID.IsZero() {
		out.EntityId = uuidPointer(result.EntityID)
	}
	if result.ServerState != nil {
		state := result.ServerState
		out.ServerState = &state
	}
	if result.Conflict != nil {
		field := result.Conflict.Field
		out.Conflict = &struct {
			Field              *string             `json:"field,omitempty"`
			Mine               interface{}         `json:"mine,omitempty"`
			PreservedCommentId *openapi_types.UUID `json:"preserved_comment_id,omitempty"` //nolint:revive // the generated type's own spelling, which the literal has to match
			Theirs             interface{}         `json:"theirs,omitempty"`
		}{Field: &field, Mine: result.Conflict.Mine, Theirs: result.Conflict.Theirs}
		if !result.Conflict.PreservedCommentID.IsZero() {
			out.Conflict.PreservedCommentId = uuidPointer(result.Conflict.PreservedCommentID)
		}
	}
	if result.Error != nil {
		code, message := result.Error.Code, result.Error.MessageCode
		out.Error = &struct {
			Code        *string `json:"code,omitempty"`
			MessageCode *string `json:"message_code,omitempty"`
		}{Code: &code, MessageCode: &message}
	}
	return out
}
