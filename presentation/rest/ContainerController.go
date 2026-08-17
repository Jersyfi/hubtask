// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// CreateContainer answers POST /containers.
//
// The handler holds no rules. It reads the request, hands it to the catalogue, and maps the
// result - the permission check, the invariants, the event, the change log entry and the audit
// entry all happen in the application layer, once, for every channel (ADR-0005, arc42 §4).
//
// It goes through the catalogue rather than calling the use case directly, which is what makes
// this identical to the same operation reached through MCP or from an automation rule: one
// handler, one audit entry, one metric, whichever door the call came in by (test AT-6).
func (c *RestController) CreateContainer(w http.ResponseWriter, r *http.Request, _ openapi.CreateContainerParams) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	var body openapi.ContainerCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), createContainerUseCase, actor, usecase.Input{
		"type":        string(body.Type),
		"name":        body.Name,
		"parent_id":   optionalUUIDField(body.ParentId),
		"description": optionalStringField(body.Description),
		"icon":        optionalStringField(body.Icon),
		"color_token": optionalStringField(body.ColorToken),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	container := containerResponse(out)
	// Location and ETag are what let a client follow up without guessing: where the container is,
	// and which version it may write against (api-guidelines.md §5).
	w.Header().Set("Location", APIBasePath+"/containers/"+container.Id.String())
	w.Header().Set("ETag", etag(out.Int("version")))
	writeJSON(w, r, http.StatusCreated, container)
}

// createContainerUseCase is the catalogue name. The route it is reached through comes from the
// specification; the two are reconciled by the parity test rather than by this constant.
const createContainerUseCase = "CreateContainer"

// etag is the version as a strong entity tag. Strong rather than weak because If-Match requires
// strong comparison (RFC 9110 §13.1.1), and this is what optimistic locking is built on.
func etag(version int) string { return `"` + strconv.Itoa(version) + `"` }

// decodeJSON reads a request body into the generated type.
//
// Unknown fields are refused rather than ignored, which matches what the catalogue does with the
// same input arriving through MCP: a client that misspells `parent_id` and receives a 201 has
// created something in the wrong place with no way to find out (api-guidelines.md §5, tolerance
// applies to what a client *reads*, not to what it sends).
func decodeJSON(r *http.Request, into any) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(into); err != nil {
		if field, ok := unknownField(err); ok {
			return shared.ErrValidation.
				WithDetail("usecase.input_invalid").
				WithFields(shared.FieldError{Path: "/" + field, Code: "usecase.field_unknown"})
		}
		if errors.Is(err, io.EOF) {
			return shared.ErrMalformedRequest.WithDetail("request.body_missing")
		}
		// Nothing of the decoder's message reaches the client: it quotes the input, and the input
		// may contain anything (security.md §9).
		return shared.ErrMalformedRequest.WithDetail("request.body_malformed").WithCause(err)
	}
	return nil
}

// unknownField pulls the field name out of encoding/json's message. Reading a message rather than
// a typed error is unpleasant, and it is what the standard library offers - the alternative is to
// accept the field silently, which is the failure this exists to prevent.
func unknownField(err error) (string, bool) {
	const marker = `unknown field "`

	message := err.Error()
	start := strings.Index(message, marker)
	if start < 0 {
		return "", false
	}
	name := message[start+len(marker):]
	end := strings.IndexByte(name, '"')
	if end < 0 {
		return "", false
	}
	return name[:end], true
}

// containerResponse maps the catalogue's output onto the generated schema. The mapping lives here
// because the generated types are the contract's shape rather than the domain's
// (project-structure.md §3).
func containerResponse(out usecase.Output) openapi.Container {
	orderKey := out.String("order_key")
	createdAt := timeValue(out["created_at"])
	updatedAt := timeValue(out["updated_at"])

	container := openapi.Container{
		Id:        uuidValue(out.String("id")),
		Type:      openapi.ContainerType(out.String("type")),
		Name:      out.String("name"),
		Version:   out.Int("version"),
		OrderKey:  &orderKey,
		CreatedAt: &createdAt,
		UpdatedAt: &updatedAt,
	}
	if parent := out.String("parent_id"); parent != "" {
		parentID := uuidValue(parent)
		container.ParentId = &parentID
	}
	if description := out.String("description"); description != "" {
		container.Description = &description
	}
	if icon := out.String("icon"); icon != "" {
		container.Icon = &icon
	}
	if colorToken := out.String("color_token"); colorToken != "" {
		container.ColorToken = &colorToken
	}
	return container
}

// optionalStringField turns an absent field into an absent entry rather than an empty string, so
// that the catalogue sees the same thing whichever channel the call came through.
func optionalStringField(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func optionalUUIDField(value *openapi_types.UUID) any {
	if value == nil {
		return nil
	}
	return value.String()
}

// uuidValue is the generated type's form of an identifier. A value the use case produced is
// always canonical, so a parse failure here would be a defect rather than input - and an empty
// identifier in a response is more visible than a swallowed error.
func uuidValue(value string) openapi_types.UUID {
	var id openapi_types.UUID
	if err := id.UnmarshalText([]byte(value)); err != nil {
		return openapi_types.UUID{}
	}
	return id
}

func timeValue(value any) time.Time {
	at, _ := value.(time.Time)
	return at
}
