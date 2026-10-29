// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The configuration document's doors (F-6) that carry a step-up proof or answer an ETag of their
// own; the others are answered by the generated mapping (Doors.gen.go). Its blocks are free-form
// by section, described by the schema at /meta/config-schema, so the answers are the use cases'
// own shapes - the contract's Config* schemas - rather than mapped field by field.

// configCall decodes a body, adds what the path names, and answers the use case's output.
func (c *RestController) configCall(w http.ResponseWriter, r *http.Request, name string, extra usecase.Input, status int) (usecase.Output, bool) {
	in := usecase.Input{}
	if r.Body != nil && r.ContentLength != 0 {
		var body map[string]any
		if err := decodeJSON(r, &body); err != nil {
			WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
			return nil, false
		}
		for k, v := range body {
			in[k] = v
		}
	}
	for k, v := range extra {
		in[k] = v
	}
	out, ok := c.read(w, r, name, in)
	if !ok {
		return nil, false
	}
	writeJSON(w, r, status, out)
	return out, true
}

// withHub names the hub whose own document a read is for, where the query names one.
func withHub(in usecase.Input, hub *openapi_types.UUID) usecase.Input {
	if hub != nil {
		in["hub_id"] = hub.String()
	}
	return in
}

func (c *RestController) ExportConfig(w http.ResponseWriter, r *http.Request, params openapi.ExportConfigParams) {
	out, ok := c.read(w, r, "ExportConfig", withHub(usecase.Input{}, params.HubId))
	if !ok {
		return
	}
	if hash, _ := out["hash"].(string); hash != "" {
		w.Header().Set("ETag", `"`+hash+`"`)
	}
	writeJSON(w, r, http.StatusOK, out)
}

// ApplyConfig answers POST /config:apply; If-Match carries the hash of the configuration the plan
// was made on, as the export's ETag gave it.
func (c *RestController) ApplyConfig(w http.ResponseWriter, r *http.Request, params openapi.ApplyConfigParams) {
	extra := usecase.Input{}
	if token := stepUpHeaderField(params.XHubtaskStepUp); token != nil {
		extra["step_up_token"] = token
	}
	if match := ifMatchOf(r); match != nil {
		extra["base_hash"] = strings.Trim(strings.TrimPrefix(*match, "W/"), `"`)
	}
	c.configCall(w, r, "ApplyConfig", extra, http.StatusOK)
}

func (c *RestController) DecideConfigProposal(w http.ResponseWriter, r *http.Request, proposalID openapi_types.UUID, params openapi.DecideConfigProposalParams) {
	extra := usecase.Input{"proposal_id": proposalID.String()}
	if token := stepUpHeaderField(params.XHubtaskStepUp); token != nil {
		extra["step_up_token"] = token
	}
	c.configCall(w, r, "DecideConfigProposal", extra, http.StatusOK)
}
