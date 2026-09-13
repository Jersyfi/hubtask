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
	"github.com/Jersyfi/hubtask/core/domain/service"
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

	roles := make([]openapi.RoleDescription, 0, len(source.Roles))
	for _, described := range source.Roles {
		permissions := make([]openapi.RoleDescriptionPermissions, 0, len(described.Permissions))
		for _, permission := range described.Permissions {
			permissions = append(permissions, openapi.RoleDescriptionPermissions(permission))
		}

		role := openapi.MembershipRole(described.Role)
		reach := roleItemAccess(described.ItemAccess)
		roles = append(roles, openapi.RoleDescription{
			Role: &role, Permissions: &permissions, ItemAccess: &reach,
		})
	}

	limits := make(map[string]any, len(source.Limits))
	for name, value := range source.Limits {
		limits[name] = value
	}

	// Always an array, empty for an installation whose PostgreSQL can index nothing beyond exact
	// words: absent would read as "this server does not know about languages", which is a
	// different statement and one a client would act on.
	textLanguages := source.TextLanguages
	if textLanguages == nil {
		textLanguages = []string{}
	}

	// The set a saved view's layout is validated against (D-07): stored, echoed, never consulted.
	viewLayouts := make([]string, 0, len(source.ViewLayouts))
	for _, layout := range source.ViewLayouts {
		viewLayouts = append(viewLayouts, string(layout))
	}

	// Every type this build emits, and therefore every type a subscription may name (F4-15). An
	// array rather than absent, for the same reason: a client that read nothing here would offer
	// a picker of its own, and the server refuses a type it does not emit.
	eventTypes := make([]string, 0, len(source.EventTypes))
	for _, eventType := range source.EventTypes {
		eventTypes = append(eventTypes, string(eventType))
	}

	// What starts a rule and what a rule may do (issue 542). Both always arrays, for the reason
	// the event types are: a client that read nothing here would offer a picker of its own, and
	// the server refuses a kind it does not serve.
	triggers := make([]string, 0, len(source.AutomationTriggers))
	for _, kind := range source.AutomationTriggers {
		triggers = append(triggers, kind.String())
	}
	automationActions := source.AutomationActions
	if automationActions == nil {
		automationActions = []string{}
	}
	automationManifest := struct {
		Actions  *[]string `json:"actions,omitempty"`
		Triggers *[]string `json:"triggers,omitempty"`
	}{Actions: &automationActions, Triggers: &triggers}

	// The catalogue of §3, with what this build can do to each. `actions` is always an array,
	// including the empty one: a kind nothing removes is named here on purpose, and an absent key
	// would be indistinguishable from a kind that does not exist (F4-18).
	dataKinds := make([]openapi.RetentionDataKind, 0, len(source.RetentionDataKinds))
	for _, kind := range source.RetentionDataKinds {
		name := string(kind.Name)
		defaultDays := kind.DefaultDays
		minDays := kind.MinDays
		actions := make([]string, 0, len(kind.Actions))
		for _, action := range kind.Actions {
			actions = append(actions, string(action))
		}
		entry := openapi.RetentionDataKind{
			DataKind: &name, DefaultDays: &defaultDays, MinDays: &minDays, Actions: &actions,
		}
		if kind.MaxDays != nil {
			ceiling := *kind.MaxDays
			entry.MaxDays = &ceiling
		}
		dataKinds = append(dataKinds, entry)
	}

	// Always arrays, like the languages: absent would read as "this server does not know about
	// notification preferences", and a client would hide the form.
	notificationCategories := source.NotificationCategories
	if notificationCategories == nil {
		notificationCategories = []string{}
	}
	notificationChannels := source.NotificationChannels
	if notificationChannels == nil {
		notificationChannels = []string{}
	}
	// Same reasoning one more time: an absent list would read as "this build declares no scopes",
	// and a client would offer none rather than every one it may ask for.
	tokenScopes := source.TokenScopes
	if tokenScopes == nil {
		tokenScopes = []string{}
	}

	productVersion := source.ProductVersion
	apiVersion := source.APIVersion
	tenancy := openapi.CapabilitiesTenancyMode(source.TenancyMode)
	features := source.Features

	return openapi.Capabilities{
		ProductVersion:         &productVersion,
		ApiVersion:             &apiVersion,
		TenancyMode:            &tenancy,
		ItemTypes:              &itemTypes,
		QueryFields:            &queryFields,
		ViewLayouts:            &viewLayouts,
		EventTypes:             &eventTypes,
		Automation:             &automationManifest,
		RetentionDataKinds:     &dataKinds,
		TextLanguages:          &textLanguages,
		NotificationCategories: &notificationCategories,
		NotificationChannels:   &notificationChannels,
		TokenScopes:            &tokenScopes,
		Roles:                  &roles,
		Limits:                 &limits,
		Features:               &features,
	}
}

// roleItemAccess renders the per-entry half of one matrix row.
//
// Every kind is written, including the ones that are NONE: an absent key would leave a client
// guessing, and the permissive guess is the one this endpoint exists to prevent.
func roleItemAccess(reach map[service.ItemAction]service.ItemAccess) openapi.RoleItemAccess {
	value := func(action service.ItemAction) *openapi.ItemAccess {
		rendered := openapi.ItemAccess(reach[action])
		return &rendered
	}
	return openapi.RoleItemAccess{
		Read:    value(service.ItemRead),
		Create:  value(service.ItemCreate),
		Change:  value(service.ItemChange),
		Comment: value(service.ItemComment),
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
