// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/realtime"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/realtime"
	"github.com/Jersyfi/hubtask/presentation/stream"
)

// RealtimeController serves `GET /realtime/documents/{kind}/{documentId}`: a live document held
// as a WebSocket (offline-sync.md §13). Counted with the change stream's connections, because a
// held socket is the same resource whoever holds it.
type RealtimeController struct {
	Handler  realtime.Handler
	Registry *stream.Registry
}

// OpenRealtimeDocument admits the connection and hands it to the handler.
func (c RealtimeController) OpenRealtimeDocument(
	w http.ResponseWriter, r *http.Request, kind string, documentID openapi_types.UUID,
) {
	requestID := correlation.RequestIDFrom(r.Context())
	actor, _ := appshared.ActorFrom(r.Context())
	if !actor.IsAuthenticated() {
		WriteProblem(w, shared.ErrUnauthenticated.WithDetail("access.credential_required"), requestID)
		return
	}
	ref, err := domain.NewRef(kind, shared.ID(documentID.String()))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	slot, refusal := c.Registry.Admit(stream.Credential(r), actor.TenantID.String())
	if refusal != stream.RefusedNone {
		w.Header().Set("Retry-After", strconv.Itoa(streamRefusalRetryAfter))
		WriteProblem(w, shared.ErrUnavailable.WithDetail("sync.stream_unavailable").
			WithParams(map[string]string{"reason": refusal.String()}), requestID)
		return
	}
	defer slot.Release()
	if err := c.Handler.Serve(w, r, actor, ref, slot.Closing); err != nil {
		WriteProblem(w, err, requestID)
	}
}
