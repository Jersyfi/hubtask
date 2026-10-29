// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The role operations: the permission catalogue, the workspace's own roles and their grants, and
// what a person holds at a place. Their outputs carry the contract's field names.
const (
	listRolesUseCase       = "ListRoles"
	createRoleUseCase      = "CreateRole"
	changeRoleUseCase      = "ChangeRole"
	deleteRoleUseCase      = "DeleteRole"
	grantRoleUseCase       = "GrantRole"
	revokeRoleGrantUseCase = "RevokeRoleGrant"
	explainAccessUseCase   = "ExplainAccess"
)

// ListRoles answers GET /roles.
func (c *RestController) ListRoles(w http.ResponseWriter, r *http.Request) {
	if out, ok := c.read(w, r, listRolesUseCase, usecase.Input{}); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

// CreateRole answers POST /roles.
func (c *RestController) CreateRole(w http.ResponseWriter, r *http.Request, _ openapi.CreateRoleParams) {
	var body openapi.CustomRoleCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, createRoleUseCase, usecase.Input{
		"key": body.Key, "name": body.Name, "permissions": anyList(body.Permissions),
	})
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, out)
}

// ChangeRole answers PATCH /roles/{key}.
func (c *RestController) ChangeRole(w http.ResponseWriter, r *http.Request, key string, _ openapi.ChangeRoleParams) {
	var body openapi.CustomRoleChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key, "name": body.Name, "permissions": anyList(body.Permissions)}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, changeRoleUseCase, in)
	if !ok {
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, out)
}

// DeleteRole answers DELETE /roles/{key}.
func (c *RestController) DeleteRole(w http.ResponseWriter, r *http.Request, key string) {
	if _, ok := c.read(w, r, deleteRoleUseCase, usecase.Input{"key": key}); ok {
		w.WriteHeader(http.StatusNoContent)
	}
}

// GrantRole answers POST /role-grants.
func (c *RestController) GrantRole(w http.ResponseWriter, r *http.Request, _ openapi.GrantRoleParams) {
	var body openapi.CustomRoleGrantCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, grantRoleUseCase, usecase.Input{
		"role": body.Role, "scope_type": string(body.ScopeType),
		"scope_id":   optionalUUIDField(body.ScopeId),
		"account_id": optionalUUIDField(body.AccountId),
		"group_id":   optionalUUIDField(body.GroupId),
	})
	if ok {
		writeJSON(w, r, http.StatusCreated, out)
	}
}

// RevokeRoleGrant answers DELETE /role-grants/{grantId}.
func (c *RestController) RevokeRoleGrant(w http.ResponseWriter, r *http.Request, grantID openapi_types.UUID) {
	if _, ok := c.read(w, r, revokeRoleGrantUseCase, usecase.Input{"grant_id": grantID.String()}); ok {
		w.WriteHeader(http.StatusNoContent)
	}
}

// ExplainAccess answers GET /access/explain.
func (c *RestController) ExplainAccess(w http.ResponseWriter, r *http.Request, params openapi.ExplainAccessParams) {
	in := usecase.Input{
		"scope_type": string(params.ScopeType),
		"account_id": optionalUUIDField(params.AccountId),
		"scope_id":   optionalUUIDField(params.ScopeId),
	}
	if params.Permission != nil {
		in["permission"] = *params.Permission
	}
	if out, ok := c.read(w, r, explainAccessUseCase, in); ok {
		writeJSON(w, r, http.StatusOK, out)
	}
}

func anyList(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}
