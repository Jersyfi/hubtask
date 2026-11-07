// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// BuildHeader names the server's build on every answer; ClientBuildHeader is the build a
// first-party client was loaded from. The build is the bundle's entry script, the one value the
// binary and the bundle share by construction (ADR-0028).
const (
	BuildHeader       = "X-Hubtask-Build"
	ClientBuildHeader = "X-Hubtask-Client-Build"
)

// BuildMarked marks every answer with the server's build and, during a rolling update, turns a
// route this build does not know into a retry when the client comes from another build
// (api-guidelines.md §8). A pod of the older release does not know the newer one's routes; a 404
// would tell the client the operation does not exist, while the next attempt may land on a pod
// that serves it. Nothing ran for an unrouted request, so the retry is safe for every method.
type BuildMarked struct {
	// Build is this binary's; empty for a binary without a bundle, which then compares nothing.
	Build  string
	Routes Router
	Next   http.Handler
}

func (b BuildMarked) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if b.Build == "" {
		b.Next.ServeHTTP(w, r)
		return
	}
	w.Header().Set(BuildHeader, b.Build)
	client := r.Header.Get(ClientBuildHeader)
	if client != "" && client != b.Build {
		if _, route := b.Routes.Handler(r); route == "" {
			w.Header().Set("Retry-After", "1")
			WriteProblem(w, shared.ErrUnavailable.WithDetail("route.build_mismatch"), correlation.RequestIDFrom(r.Context()))
			return
		}
	}
	b.Next.ServeHTTP(w, r)
}
