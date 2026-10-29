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

// The configuration document's doors (F-6). Its blocks are free-form by section, described by the
// schema at /meta/config-schema, so the answers are the use cases' own shapes - the contract's
// Config* schemas - rather than mapped field by field.

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

func (c *RestController) ReadConfigSchema(w http.ResponseWriter, r *http.Request) {
	if out, ok := c.read(w, r, "ReadConfigSchema", usecase.Input{}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) ExportConfig(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, "ExportConfig", usecase.Input{})
	if !ok {
		return
	}
	if hash, _ := out["hash"].(string); hash != "" {
		w.Header().Set("ETag", `"`+hash+`"`)
	}
	writeJSON(w, r, http.StatusOK, out)
}

func (c *RestController) ExplainConfig(w http.ResponseWriter, r *http.Request, params openapi.ExplainConfigParams) {
	if out, ok := c.read(w, r, "ExplainConfig", usecase.Input{"key": params.Key}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) ValidateConfig(w http.ResponseWriter, r *http.Request) {
	c.configCall(w, r, "ValidateConfig", nil, http.StatusOK)
}

func (c *RestController) PlanConfig(w http.ResponseWriter, r *http.Request) {
	c.configCall(w, r, "PlanConfig", nil, http.StatusOK)
}

func (c *RestController) ProposeConfig(w http.ResponseWriter, r *http.Request) {
	c.configCall(w, r, "ProposeConfig", nil, http.StatusOK)
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

func (c *RestController) ListConfigVersions(w http.ResponseWriter, r *http.Request, params openapi.ListConfigVersionsParams) {
	in := usecase.Input{}
	if params.Before != nil {
		in["before"] = *params.Before
	}
	if out, ok := c.read(w, r, "ListConfigVersions", in); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) ReadConfigVersion(w http.ResponseWriter, r *http.Request, version int) {
	if out, ok := c.read(w, r, "ReadConfigVersion", usecase.Input{"version": version}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) ListConfigProposals(w http.ResponseWriter, r *http.Request, params openapi.ListConfigProposalsParams) {
	in := usecase.Input{}
	if params.State != nil {
		in["state"] = string(*params.State)
	}
	if out, ok := c.read(w, r, "ListConfigProposals", in); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) DecideConfigProposal(w http.ResponseWriter, r *http.Request, proposalID openapi_types.UUID, params openapi.DecideConfigProposalParams) {
	extra := usecase.Input{"proposal_id": proposalID.String()}
	if token := stepUpHeaderField(params.XHubtaskStepUp); token != nil {
		extra["step_up_token"] = token
	}
	c.configCall(w, r, "DecideConfigProposal", extra, http.StatusOK)
}

func (c *RestController) SetConfigPolicy(w http.ResponseWriter, r *http.Request) {
	c.configCall(w, r, "SetConfigPolicy", nil, http.StatusOK)
}

func (c *RestController) ListConfigTemplates(w http.ResponseWriter, r *http.Request) {
	if out, ok := c.read(w, r, "ListConfigTemplates", usecase.Input{}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func (c *RestController) PublishConfigTemplate(w http.ResponseWriter, r *http.Request) {
	c.configCall(w, r, "PublishConfigTemplate", nil, http.StatusCreated)
}

func (c *RestController) PlanConfigTemplate(w http.ResponseWriter, r *http.Request, key string) {
	c.configCall(w, r, "PlanConfigTemplate", usecase.Input{"key": key}, http.StatusOK)
}
