// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The workspace's own configuration (F4-01).
const (
	readWorkspaceUseCase   = "ReadWorkspace"
	updateWorkspaceUseCase = "UpdateWorkspace"
)

// ReadWorkspace answers GET /tenant.
//
// Written out rather than through the identity helper, for `ListWebhookSubscriptions`' reason:
// the helper's closure takes no context, and an operation with no parameters gives the linter
// nothing to trace the request's context through.
func (c *RestController) ReadWorkspace(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), readWorkspaceUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusOK, workspaceResponse(out))
}

// UpdateWorkspace answers PATCH /tenant.
func (c *RestController) UpdateWorkspace(
	w http.ResponseWriter, r *http.Request, params openapi.UpdateWorkspaceParams,
) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.WorkspaceUpdate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		in := usecase.Input{
			"display_name":      optionalStringField(body.DisplayName),
			"default_locale":    optionalStringField(body.DefaultLocale),
			"default_time_zone": optionalStringField(body.DefaultTimeZone),
		}
		if body.RequireAdminTotp != nil {
			// Set only when the caller sent one: absent has to reach the catalogue as absent,
			// because false is a value this field legitimately holds.
			in["require_admin_totp"] = *body.RequireAdminTotp
		}
		if version, ok := versionFromIfMatch(params.IfMatch); ok {
			in["expected_version"] = version
		}
		return c.UseCases.Invoke(r.Context(), updateWorkspaceUseCase, actor, in)
	}, func(out usecase.Output) {
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusOK, workspaceResponse(out))
	})
}

// workspaceResponse maps the use case's answer.
func workspaceResponse(out usecase.Output) openapi.Workspace {
	enforced, _ := out["require_admin_totp"].(bool)
	answer := openapi.Workspace{
		Id:               uuidValue(out.String("id")),
		Slug:             out.String("slug"),
		DisplayName:      out.String("display_name"),
		Status:           openapi.WorkspaceStatus(out.String("status")),
		DefaultLocale:    out.String("default_locale"),
		DefaultTimeZone:  out.String("default_time_zone"),
		RequireAdminTotp: enforced,
		CreatedAt:        timeValue(out["created_at"]),
		Version:          out.Int("version"),
	}
	if updated, held := out["updated_at"].(time.Time); held && !updated.IsZero() {
		answer.UpdatedAt = &updated
	}
	return answer
}
