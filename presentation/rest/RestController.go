// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// UseCaseRegistry is the catalogue as this layer needs it: run the operation named, with the
// actor of the request and the input the body carried.
//
// An interface rather than *usecase.Registry so that a controller test needs no wiring, and
// because this is the whole of what the REST layer is allowed to do with the catalogue - it
// executes entries, it does not add any.
type UseCaseRegistry interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// RestController implements the server interface generated from api/openapi.yaml.
//
// It holds no business logic. An operation reads the request, calls one use case, and maps the
// result - the rules live in the application layer, and the authorisation check lives there and
// nowhere else (ADR-0005, project-structure.md §2).
//
// Everything not overridden here comes from the embedded pending set and answers 404: the route
// exists because the specification declares it, the implementation arrives with its own task.
type RestController struct {
	pending

	// SAML serves the assertion consumer service and the metadata (identity.md §26).
	SAML *SAMLDoor

	// Meta serves the self-description and the health report (MetaDoor). Nil leaves both routes
	// answering the pending 404.
	Meta *MetaDoor

	// UseCases is the catalogue every business operation goes through. One field rather than one
	// per use case: the operations that arrive from here on are entries in it, not new
	// dependencies of this controller (arc42 §4).
	UseCases UseCaseRegistry

	// Media serves the two content routes (MediaDoor). Nil leaves them answering the pending 404.
	Media *MediaDoor

	// BaseURL is this installation's public address, and the only thing this controller composes
	// rather than maps: a calendar feed is handed to a client as a URL to subscribe to, and a URL
	// is a fact about the installation rather than about the domain. Empty answers a
	// relative address, which is what a client that just called this server can still use.
	BaseURL string

	// Feeds serves the routes a token alone opens (FeedDoor). Nil leaves them answering the
	// pending 404.
	Feeds *FeedDoor

	// Scim serves the SCIM 2.0 door a directory provisions people and groups through
	// (UC-ID-28). Nil leaves the routes answering the pending 404.
	Scim *ScimDoor

	// Intake serves the public doors a sender outside holds the credential of (IntakeDoor). Nil
	// leaves them answering the pending 404.
	Intake *IntakeDoor

	// Stream serves `GET /stream`, which is not a catalogue entry either: it is a connection being
	// held rather than an operation being invoked, so there is nothing for MCP or an automation
	// rule to call. Nil leaves the route answering the pending 404, which is what an
	// installation built without it should say.
	Stream *StreamController

	// Realtime serves a live document's WebSocket, a held connection for the stream's reason. Nil
	// leaves the route answering the pending 404.
	Realtime *RealtimeController

	// Sync serves `POST /sync:pull`, which is not a catalogue entry for the stream's reason: a
	// pull is the stream served in pages. Nil leaves the route answering the pending 404.
	Sync *SyncController
}

func NewRestController() *RestController { return &RestController{} }

// errNotWired is a defect in the composition root, not a request the client got wrong: the route
// is registered and its use case is missing. It answers 500 with nothing in it but the code and
// the request ID, like every other internal error (security.md §9).
var errNotWired = shared.ErrInternal.WithDetail("rest.use_case_not_wired")

// Routes builds the router from the generated registration list. Nothing here names a path: the
// paths come from the specification through the generated code, which is what keeps the router
// from drifting away from the contract (ADR-0004).
func (c *RestController) Routes() *Mux {
	mux := NewMux()
	openapi.HandlerFromMuxWithBaseURL(c, mux, APIBasePath)
	return mux
}
