// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"
	"net/url"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The assertion consumer service and the metadata are delegated to SAMLDoor: one takes a form a provider
// posts and answers a redirect, the other an XML document, and neither is a use case MCP or an
// automation rule could call (identity.md §26).

// SAMLAnswerKeeper keeps a SAML provider's answer on its sign-in and mints the handoff code.
type SAMLAnswerKeeper interface {
	Execute(ctx context.Context, state, answer string) (secret.Secret, error)
}

// SAMLDoor is what the two SAML routes need, wired in cmd/.
type SAMLDoor struct {
	Answers SAMLAnswerKeeper
	// CallbackURL is this installation's own callback, where every provider's return finishes.
	CallbackURL string
	// Metadata is the service provider metadata, built once for the installation.
	Metadata []byte
}

// maxSAMLForm bounds the posted form ahead of the use case's own bound.
const maxSAMLForm = 400 << 10

// ReceiveSamlResponse keeps the answer and sends the browser on to the callback.
func (c *SAMLDoor) ReceiveSamlResponse(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Answers == nil || c.CallbackURL == "" {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxSAMLForm)
	if err := r.ParseForm(); err != nil {
		WriteProblem(w, shared.ErrNotFound.WithDetail("auth.oidc_flow_not_found"), requestID)
		return
	}
	state := r.PostForm.Get("RelayState")
	code, err := c.Answers.Execute(r.Context(), state, r.PostForm.Get("SAMLResponse"))
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	target, err := url.Parse(c.CallbackURL)
	if err != nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	query := url.Values{}
	query.Set("code", code.Reveal())
	query.Set("state", state)
	target.RawQuery = query.Encode()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	http.Redirect(w, r, target.String(), http.StatusSeeOther)
}

// GetSamlMetadata answers this installation's service provider metadata.
func (c *SAMLDoor) GetSamlMetadata(w http.ResponseWriter, r *http.Request) {
	if len(c.Metadata) == 0 {
		WriteProblem(w, errNotWired, correlation.RequestIDFrom(r.Context()))
		return
	}
	w.Header().Set("Content-Type", "application/samlmetadata+xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(c.Metadata)
}
