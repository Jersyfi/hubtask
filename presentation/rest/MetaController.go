// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	usecase "github.com/Jersyfi/hubtask/core/application/service/meta"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// CapabilityReader is the slice of the self-description use case this controller needs.
type CapabilityReader interface {
	Execute(context.Context, appshared.ActorContext) (usecase.Capabilities, error)
}

// GetCapabilities answers the manifest a client configures itself from (api-guidelines.md §1).
//
// Public by the contract (`security: []`), and answered for an anonymous caller from the
// system-defined profiles alone. A caller that did present a credential sees its tenant's
// overrides where it has any - the same endpoint, a different answer, decided by the scope the
// use case opens rather than by a branch here.
func (c *RestController) GetCapabilities(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.Capabilities == nil {
		// The composition root wires it; a controller without it is a programming error, not a
		// request the client got wrong.
		WriteProblem(w, errNotWired, requestID)
		return
	}

	actor, _ := appshared.ActorFrom(r.Context())
	capabilities, err := c.Capabilities.Execute(r.Context(), actor)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	writeJSON(w, r, http.StatusOK, capabilityManifest(capabilities))
}

// capabilityItemType is the anonymous struct the generator produced for one entry of item_types.
// An alias rather than a copy: written out twice it would be two things to keep in step with a
// regenerated file, and the compiler would only complain about one of them.
type capabilityItemType = struct {
	AllowedChildTypes *[]openapi.ItemType `json:"allowed_child_types,omitempty"`
	Capabilities      *[]string           `json:"capabilities,omitempty"`
	MaxDepth          *int                `json:"max_depth,omitempty"`

	// Type Extensible; /meta/capabilities returns the valid values.
	Type *openapi.ItemType `json:"type,omitempty"`
}

// capabilityManifest maps the use case's result onto the generated schema. The mapping is here
// and not in the application layer, because the generated types are the contract's shape rather
// than the domain's (project-structure.md §3).
func capabilityManifest(source usecase.Capabilities) openapi.Capabilities {
	itemTypes := make([]capabilityItemType, 0, len(source.ItemTypes))

	for _, profile := range source.ItemTypes {
		capabilities := make([]string, 0, len(profile.Capabilities))
		for _, capability := range profile.Capabilities {
			capabilities = append(capabilities, string(capability))
		}
		children := make([]openapi.ItemType, 0, len(profile.AllowedChildTypes))
		for _, child := range profile.AllowedChildTypes {
			children = append(children, openapi.ItemType(child))
		}

		itemType := openapi.ItemType(profile.Type)
		depth := profile.MaxDepth
		itemTypes = append(itemTypes, capabilityItemType{
			AllowedChildTypes: &children,
			Capabilities:      &capabilities,
			MaxDepth:          &depth,
			Type:              &itemType,
		})
	}

	queryFields := make([]openapi.QueryField, 0, len(source.QueryFields))
	for _, field := range source.QueryFields {
		operators := make([]string, 0, len(field.Operators))
		for _, operator := range field.Operators {
			operators = append(operators, string(operator))
		}

		rendered := openapi.QueryField{
			Field:     field.Name,
			Kind:      openapi.QueryFieldKind(field.Kind),
			Operators: operators,
			Nullable:  field.Nullable,
			Sortable:  field.Sortable,
			Groupable: field.Groupable,
		}
		if len(field.Values) > 0 {
			values := field.Values
			rendered.Values = &values
		}
		queryFields = append(queryFields, rendered)
	}

	limits := make(map[string]any, len(source.Limits))
	for name, value := range source.Limits {
		limits[name] = value
	}

	productVersion := source.ProductVersion
	apiVersion := source.APIVersion
	tenancy := openapi.CapabilitiesTenancyMode(source.TenancyMode)
	features := source.Features

	return openapi.Capabilities{
		ProductVersion: &productVersion,
		ApiVersion:     &apiVersion,
		TenancyMode:    &tenancy,
		ItemTypes:      &itemTypes,
		QueryFields:    &queryFields,
		Limits:         &limits,
		Features:       &features,
	}
}

// writeJSON is the one place a successful body is written. Cache-Control is no-store by default:
// an answer scoped to a tenant has no business in a shared cache, and the endpoints that may be
// cached will say so themselves.
func writeJSON(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(body); err != nil {
		// The status is already on the wire, so there is nothing left to tell the client. It is
		// worth a log line, because a body that cannot be encoded is a defect rather than a
		// network event.
		slog.WarnContext(r.Context(), "writing the response body failed",
			slog.String("error", err.Error()))
	}
}
