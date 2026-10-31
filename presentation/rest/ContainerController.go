// SPDX-License-Identifier: Apache-2.0
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

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

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
	return decodeFrom(r.Body, into)
}

func decodeFrom(body io.Reader, into any) error {
	decoder := json.NewDecoder(body)
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
	container.ArchivedAt = optionalTimeField(out["archived_at"])
	container.DeletedAt = optionalTimeField(out["deleted_at"])

	// Always present, both of them. `effective_archived` is what a client reads to know whether it
	// may write - a collection in an archived hub carries no stamp of its own (I-C3) - and a boolean
	// that only appeared once something was archived is one a client cannot rely on.
	effectiveArchived, _ := out["effective_archived"].(bool)
	container.EffectiveArchived = &effectiveArchived
	private, _ := out["private"].(bool)
	container.Private = &private
	if handle := out.String("handle"); handle != "" {
		container.Handle = &handle
	}
	if policies, ok := out["policies"].(map[string]any); ok {
		container.Policies = &openapi.ContainerPolicies{}
		if policy, ok := policies["completion_policy"].(string); ok {
			completionPolicy := openapi.CompletionPolicy(policy)
			container.Policies.CompletionPolicy = &completionPolicy
		}
		container.Policies.AutoAssign = autoAssignResponse(policies["auto_assign"])
	}
	// The structure in force travels in the contract's words already; mapped through its JSON
	// spelling rather than field by field (throughJSON). A shape that does not map is a defect of
	// this build, answered without the structure rather than without the container.
	if structure, held := out["structure"]; held {
		var answered openapi.EffectiveStructure
		if throughJSON(structure, &answered) == nil {
			container.Structure = &answered
		}
	}
	// What counts in a collection, the same way: numbers and dates in the contract's words.
	if figure, held := out["figure"]; held {
		var answered openapi.CollectionFigure
		if throughJSON(figure, &answered) == nil {
			container.Figure = &answered
		}
	}
	if milestone, held := out["milestone"].(map[string]any); held {
		container.Milestone = &openapi.ContainerMilestone{ReachedAt: timeValue(milestone["reached_at"])}
	}
	return container
}

// autoAssignResponse maps the auto_assign key of the catalogue's policies document onto the
// generated schema. An absent policy is an absent key - the generated pointer with omitempty
// cannot say null, and the schema allows either spelling of "none".
func autoAssignResponse(value any) *openapi.AutoAssignPolicy {
	document, ok := value.(map[string]any)
	if !ok {
		return nil
	}

	policy := &openapi.AutoAssignPolicy{}
	if strategy, ok := document["strategy"].(string); ok {
		policy.Strategy = openapi.AutoAssignStrategy(strategy)
	}
	if enabled, ok := document["enabled"].(bool); ok {
		policy.Enabled = &enabled
	}
	if list, ok := document["candidates"].([]any); ok {
		candidates := make([]openapi.AutoAssignCandidate, 0, len(list))
		for _, entry := range list {
			candidate, ok := entry.(map[string]any)
			if !ok {
				continue
			}
			kind, _ := candidate["kind"].(string)
			id, _ := candidate["id"].(string)
			candidates = append(candidates, openapi.AutoAssignCandidate{
				Kind: openapi.AutoAssignCandidateKind(kind),
				Id:   uuidValue(id),
			})
		}
		policy.Candidates = candidates
	}
	return policy
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
