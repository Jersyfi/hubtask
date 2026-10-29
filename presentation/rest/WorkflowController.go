// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

const (
	listWorkflowsUseCase          = "ListWorkflows"
	createWorkflowUseCase         = "CreateWorkflow"
	saveWorkflowDraftUseCase      = "SaveWorkflowDraft"
	deleteWorkflowUseCase         = "DeleteWorkflow"
	previewWorkflowPublishUseCase = "PreviewWorkflowPublish"
	publishWorkflowUseCase        = "PublishWorkflow"
	tryWorkflowStepUseCase        = "TryWorkflowStep"
	setWorkflowSchemeUseCase      = "SetWorkflowScheme"
	previewWorkflowSchemeUseCase  = "PreviewWorkflowScheme"
	listEntryStepsUseCase         = "ListEntrySteps"
	takeStepUseCase               = "TakeStep"
)

// answerAs writes a use case's answer through its generated type, so the response holds to the
// contract's shape.
func answerAs[T any](w http.ResponseWriter, r *http.Request, status int, out usecase.Output, withETag bool) {
	var typed T
	if err := remarshal(out, &typed); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	if withETag {
		w.Header().Set("ETag", etag(out.Int("version")))
	}
	writeJSON(w, r, status, typed)
}

// ListWorkflows answers GET /workflows (UC-FLW-02).
func (c *RestController) ListWorkflows(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listWorkflowsUseCase, usecase.Input{})
	if !ok {
		return
	}
	answerAs[openapi.WorkflowList](w, r, http.StatusOK, out, false)
}

// CreateWorkflow answers POST /workflows.
func (c *RestController) CreateWorkflow(w http.ResponseWriter, r *http.Request) {
	var body openapi.WorkflowCreate
	document, err := decodeDocument(r, &body)
	if err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, createWorkflowUseCase, usecase.Input{
		"key": body.Key, "name": body.Name, "shape": document["shape"],
	})
	if !ok {
		return
	}
	answerAs[openapi.Workflow](w, r, http.StatusCreated, out, true)
}

// SaveWorkflowDraft answers PATCH /workflows/{key}.
func (c *RestController) SaveWorkflowDraft(w http.ResponseWriter, r *http.Request, key string, _ openapi.SaveWorkflowDraftParams) {
	var body openapi.WorkflowDraft
	document, err := decodeDocument(r, &body)
	if err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key}
	if body.Name != nil {
		in["name"] = *body.Name
	}
	if shape, sent := document["shape"]; sent && shape != nil {
		in["shape"] = shape
	}
	if body.Discard != nil {
		in["discard"] = *body.Discard
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, saveWorkflowDraftUseCase, in)
	if !ok {
		return
	}
	answerAs[openapi.Workflow](w, r, http.StatusOK, out, true)
}

// DeleteWorkflow answers DELETE /workflows/{key}.
func (c *RestController) DeleteWorkflow(w http.ResponseWriter, r *http.Request, key string) {
	if _, ok := c.read(w, r, deleteWorkflowUseCase, usecase.Input{"key": key}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func mappingOf(r *http.Request) (map[string]any, error) {
	var body openapi.WorkflowPublish
	if r.ContentLength == 0 {
		return map[string]any{}, nil
	}
	document, err := decodeDocument(r, &body)
	if err != nil {
		return nil, err
	}
	mapping, _ := document["mapping"].(map[string]any)
	if mapping == nil {
		mapping = map[string]any{}
	}
	return mapping, nil
}

// PreviewWorkflowPublish answers POST /workflows/{key}:preview-publish (UC-FLW-05 check 2).
func (c *RestController) PreviewWorkflowPublish(w http.ResponseWriter, r *http.Request, key string) {
	in := usecase.Input{"key": key, "mapping": map[string]any{}}
	if r.ContentLength != 0 {
		var body openapi.WorkflowPublishPreview
		document, err := decodeDocument(r, &body)
		if err != nil {
			WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
			return
		}
		if mapping, held := document["mapping"].(map[string]any); held {
			in["mapping"] = mapping
		}
		if shape, held := document["shape"]; held && shape != nil {
			in["shape"] = shape
		}
	}
	out, ok := c.read(w, r, previewWorkflowPublishUseCase, in)
	if !ok {
		return
	}
	answerAs[openapi.WorkflowPreview](w, r, http.StatusOK, out, false)
}

// PublishWorkflow answers POST /workflows/{key}:publish.
func (c *RestController) PublishWorkflow(w http.ResponseWriter, r *http.Request, key string, _ openapi.PublishWorkflowParams) {
	mapping, err := mappingOf(r)
	if err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key, "mapping": mapping}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, publishWorkflowUseCase, in)
	if !ok {
		return
	}
	answerAs[openapi.Workflow](w, r, http.StatusOK, out, true)
}

// TryWorkflowStep answers POST /workflows/{key}/steps/{step}:try (UC-FLW-04 check 5).
func (c *RestController) TryWorkflowStep(w http.ResponseWriter, r *http.Request, key string, step string) {
	var body openapi.TryWorkflowStepJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key, "step": step, "item_id": body.ItemId.String()}
	if body.Draft != nil {
		in["draft"] = *body.Draft
	}
	out, ok := c.read(w, r, tryWorkflowStepUseCase, in)
	if !ok {
		return
	}
	answerAs[openapi.StepVerdict](w, r, http.StatusOK, out, false)
}

func schemeInput(body openapi.WorkflowSchemeChange) usecase.Input {
	in := usecase.Input{"container_id": body.ContainerId.String()}
	if body.EntryType != nil {
		in["entry_type"] = *body.EntryType
	}
	if body.Workflow != nil {
		in["workflow"] = *body.Workflow
	}
	return in
}

// SetWorkflowScheme answers PUT /workflow-scheme (UC-FLW-02 check 4).
func (c *RestController) SetWorkflowScheme(w http.ResponseWriter, r *http.Request) {
	var body openapi.WorkflowSchemeChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := schemeInput(body)
	if body.Confirm != nil {
		in["confirm"] = *body.Confirm
	}
	out, ok := c.read(w, r, setWorkflowSchemeUseCase, in)
	if !ok {
		return
	}
	answerAs[openapi.WorkflowSchemeResult](w, r, http.StatusOK, out, false)
}

// PreviewWorkflowScheme answers POST /workflow-scheme:preview.
func (c *RestController) PreviewWorkflowScheme(w http.ResponseWriter, r *http.Request) {
	var body openapi.WorkflowSchemeChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	out, ok := c.read(w, r, previewWorkflowSchemeUseCase, schemeInput(body))
	if !ok {
		return
	}
	answerAs[openapi.WorkflowPreview](w, r, http.StatusOK, out, false)
}

// ListEntrySteps answers GET /items/{itemId}/steps (UC-FLW-03 check 1).
func (c *RestController) ListEntrySteps(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId) {
	out, ok := c.read(w, r, listEntryStepsUseCase, usecase.Input{"item_id": itemID.String()})
	if !ok {
		return
	}
	answerAs[openapi.EntrySteps](w, r, http.StatusOK, out, false)
}

// TakeStep answers POST /items/{itemId}:step (UC-FLW-03 check 2).
func (c *RestController) TakeStep(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.TakeStepParams) {
	var body openapi.TakeStepJSONBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"item_id": itemID.String()}
	if body.Step != nil {
		in["step"] = *body.Step
	}
	if body.ToStatus != nil {
		in["to_status"] = *body.ToStatus
	}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, takeStepUseCase, in)
	if !ok {
		return
	}
	item := workItemResponse(out)
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, r, http.StatusOK, item)
}
