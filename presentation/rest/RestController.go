// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
