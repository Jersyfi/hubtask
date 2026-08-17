// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
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
}

func NewRestController() *RestController { return &RestController{} }

// Routes builds the router from the generated registration list. Nothing here names a path: the
// paths come from the specification through the generated code, which is what keeps the router
// from drifting away from the contract (ADR-0004).
func (c *RestController) Routes() *Mux {
	mux := NewMux()
	openapi.HandlerFromMuxWithBaseURL(c, mux, APIBasePath)
	return mux
}
