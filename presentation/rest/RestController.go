// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
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

	// Capabilities answers /meta/capabilities. One field per use case as they land; nil means the
	// operation falls back to the pending answer rather than panicking.
	Capabilities CapabilityReader

	// UseCases is the catalogue every business operation goes through. One field rather than one
	// per use case: the operations that arrive from here on are entries in it, not new
	// dependencies of this controller (arc42 §4).
	UseCases UseCaseRegistry

	// MediaContent and MediaTokens serve the two content routes, which are not catalogue entries:
	// they take a stream and answer a stream, which is neither what MCP nor what an automation
	// rule could do with them (C-06, core/application/service/media.MediaContent).
	MediaContent MediaContentService
	MediaTokens  MediaTokenValidator
	// Clock judges an expiring capability. The one place this layer needs the time, and a port
	// rather than time.Now so that a test can stand at the moment a token expires.
	Clock clock.Clock

	// BaseURL is this installation's public address, and the only thing this controller composes
	// rather than maps: a calendar feed is handed to a client as a URL to subscribe to, and a URL
	// is a fact about the installation rather than about the domain (D-08). Empty answers a
	// relative address, which is what a client that just called this server can still use.
	BaseURL string

	// Stream serves `GET /stream`, which is not a catalogue entry either: it is a connection being
	// held rather than an operation being invoked, so there is nothing for MCP or an automation
	// rule to call (C-10). Nil leaves the route answering the pending 404, which is what an
	// installation built without it should say.
	Stream *StreamController
}

// StreamChanges opens the change stream. Delegated rather than embedded, so that the field being
// nil is an answer rather than a panic.
func (c *RestController) StreamChanges(
	w http.ResponseWriter, r *http.Request, params openapi.StreamChangesParams,
) {
	if c.Stream == nil {
		c.pending.StreamChanges(w, r, params)
		return
	}
	c.Stream.StreamChanges(w, r, params)
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
