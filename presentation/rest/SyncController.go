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
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
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
