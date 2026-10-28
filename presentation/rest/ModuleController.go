// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The module switches of the suite (UC-INS-21). The handlers translate; which level decides and
// who may switch is the application layer's (rule 2).

const (
	listModulesUseCase = "ListModules"
	setModuleUseCase   = "SetModule"
)

// ListModules answers GET /tenant/modules.
func (c *RestController) ListModules(w http.ResponseWriter, r *http.Request, params openapi.ListModulesParams) {
	in := usecase.Input{}
	if params.HubId != nil {
		in["hub_id"] = params.HubId.String()
	}
	out, ok := c.read(w, r, listModulesUseCase, in)
	if !ok {
		return
	}
	writeJSON(w, r, http.StatusOK, moduleListResponse(out))
}

// SetModule answers PUT /tenant/modules/{module}.
func (c *RestController) SetModule(w http.ResponseWriter, r *http.Request, module openapi.Module) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())
	var body openapi.ModuleSwitch
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"module": string(module), "switch": string(body.Switch)}
	if body.HubId != nil {
		in["hub_id"] = body.HubId.String()
	}
	out, err := c.UseCases.Invoke(r.Context(), setModuleUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, moduleListResponse(out))
}

func moduleListResponse(out usecase.Output) openapi.ModuleList {
	rows, _ := out["modules"].([]usecase.Output)
	list := openapi.ModuleList{Modules: make([]openapi.ModuleState, 0, len(rows))}
	for _, row := range rows {
		on, _ := row["on"].(bool)
		state := openapi.ModuleState{
			Module: openapi.Module(row.String("module")),
			On:     on,
			Source: openapi.SettingSource(row.String("source")),
		}
		if lockedBy := row.String("locked_by"); lockedBy != "" {
			source := openapi.SettingSource(lockedBy)
			state.LockedBy = &source
		}
		if person := row.String("locked_by_person"); person != "" {
			id := uuidValue(person)
			state.LockedByPerson = &id
		}
		list.Modules = append(list.Modules, state)
	}
	return list
}
