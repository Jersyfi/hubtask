// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// PublicFormDoor is the slice of the public form this controller needs.
type PublicFormDoor interface {
	Open(ctx context.Context, token integration.FormLinkToken) (servicedesk.PublicFormView, error)
	Submit(ctx context.Context, token integration.FormLinkToken, in servicedesk.PublicSubmission) error
}

// PublicFormService joins the two halves the application keeps apart.
type PublicFormService struct {
	Opener    servicedesk.OpenPublicForm
	Submitter servicedesk.SubmitPublicForm
}

// Open answers what the form shows.
func (s PublicFormService) Open(ctx context.Context, token integration.FormLinkToken) (servicedesk.PublicFormView, error) {
	return s.Opener.Execute(ctx, token)
}

// Submit files a request.
func (s PublicFormService) Submit(ctx context.Context, token integration.FormLinkToken, in servicedesk.PublicSubmission) error {
	return s.Submitter.Execute(ctx, token, in)
}

// publicFormToken reads a presented token; every malformed one answers what an unknown one does
// (T-21).
func publicFormToken(presented string) (integration.FormLinkToken, error) {
	token, err := integration.ParseFormLinkToken(presented)
	if err != nil {
		return integration.FormLinkToken{}, shared.ErrNotFound.WithDetail("public_forms.link_not_found")
	}
	return token, nil
}

// GetPublicForm answers GET /public/forms/{token}: a public form opened by its link, with no
// bearer credential. Parsing happens here and nothing else; every question is asked inwards.
func (c *FeedDoor) GetPublicForm(w http.ResponseWriter, r *http.Request, presented string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Forms == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := publicFormToken(presented)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	view, err := c.Forms.Open(r.Context(), token)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	data := make([]map[string]any, 0, len(view.Catalogue))
	for _, entry := range view.Catalogue {
		questions := make([]map[string]any, 0, len(entry.Questions))
		for _, question := range entry.Questions {
			options := question.Options
			if options == nil {
				options = []string{}
			}
			questions = append(questions, map[string]any{"key": question.Key, "kind": string(question.Kind), "options": options})
		}
		data = append(data, map[string]any{
			"key": entry.Key, "name": entry.Name, "icon": entry.Icon,
			"form": workflow.FormOutput(entry.Form), "questions": questions,
		})
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Robots-Tag", "noindex, nofollow")
	writeJSON(w, r, http.StatusOK, map[string]any{
		"data": data,
		"challenge": map[string]any{
			"value": view.Challenge.Value, "bits": view.Challenge.Bits, "expires_at": view.Challenge.ExpiresAt.UTC(),
		},
	})
}

// SubmitPublicForm answers POST /public/forms/{token}: a visitor's request, answered 202 whoever
// the address belongs to (P-01).
func (c *FeedDoor) SubmitPublicForm(w http.ResponseWriter, r *http.Request, presented string) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.Forms == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	token, err := publicFormToken(presented)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	var body openapi.PublicFormSubmission
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := servicedesk.PublicSubmission{
		EntryType: body.EntryType, Title: body.Title, Challenge: body.Challenge, Solution: body.Solution,
	}
	if body.Notes != nil {
		in.Notes = *body.Notes
	}
	if body.CustomFields != nil {
		in.CustomFields = *body.CustomFields
	}
	if body.RequesterName != nil {
		in.RequesterName = *body.RequesterName
	}
	if body.RequesterEmail != nil {
		in.RequesterEmail = *body.RequesterEmail
	}
	if err := c.Forms.Submit(r.Context(), token, in); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusAccepted)
}
