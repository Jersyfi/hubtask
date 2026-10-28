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

const (
	listEntryTypesUseCase   = "ListEntryTypes"
	createEntryTypeUseCase  = "CreateEntryType"
	changeEntryTypeUseCase  = "ChangeEntryType"
	deleteEntryTypeUseCase  = "DeleteEntryType"
	setItemEntryTypeUseCase = "SetItemEntryType"
)

// ListEntryTypes answers GET /entry-types (UC-FLW-01).
func (c *RestController) ListEntryTypes(w http.ResponseWriter, r *http.Request) {
	out, ok := c.read(w, r, listEntryTypesUseCase, usecase.Input{})
	if !ok {
		return
	}
	rows := rowsOf(out)
	list := openapi.EntryTypeList{Data: make([]openapi.EntryType, 0, len(rows))}
	for _, row := range rows {
		list.Data = append(list.Data, entryTypeResponse(row))
	}
	writeJSON(w, r, http.StatusOK, list)
}

// CreateEntryType answers POST /entry-types.
func (c *RestController) CreateEntryType(w http.ResponseWriter, r *http.Request) {
	var body openapi.EntryTypeCreate
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{
		"key": body.Key, "level": string(body.Level), "name": body.Name, "icon": body.Icon,
		"color_token": body.ColorToken,
	}
	putBehaviours(in, body.Completable, body.Workflow, body.Record, body.Schedulable, body.Estimable, body.AllowedChildren, body.Position)
	out, ok := c.read(w, r, createEntryTypeUseCase, in)
	if !ok {
		return
	}
	created := entryTypeResponse(out)
	w.Header().Set("ETag", etag(created.Version))
	writeJSON(w, r, http.StatusCreated, created)
}

// ChangeEntryType answers PATCH /entry-types/{key}.
func (c *RestController) ChangeEntryType(w http.ResponseWriter, r *http.Request, key string, _ openapi.ChangeEntryTypeParams) {
	var body openapi.EntryTypeChange
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, correlation.RequestIDFrom(r.Context()))
		return
	}
	in := usecase.Input{"key": key}
	if body.Name != nil {
		in["name"] = *body.Name
	}
	if body.Icon != nil {
		in["icon"] = *body.Icon
	}
	if body.ColorToken != nil {
		in["color_token"] = *body.ColorToken
	}
	if body.Archived != nil {
		in["archived"] = *body.Archived
	}
	putBehaviours(in, body.Completable, body.Workflow, body.Record, body.Schedulable, body.Estimable, body.AllowedChildren, body.Position)
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	out, ok := c.read(w, r, changeEntryTypeUseCase, in)
	if !ok {
		return
	}
	changed := entryTypeResponse(out)
	w.Header().Set("ETag", etag(changed.Version))
	writeJSON(w, r, http.StatusOK, changed)
}

// DeleteEntryType answers DELETE /entry-types/{key}.
func (c *RestController) DeleteEntryType(w http.ResponseWriter, r *http.Request, key string) {
	if _, ok := c.read(w, r, deleteEntryTypeUseCase, usecase.Input{"key": key}); !ok {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// SetItemEntryType answers PUT /items/{itemId}/entry-type.
func (c *RestController) SetItemEntryType(w http.ResponseWriter, r *http.Request, itemID openapi.ItemId, _ openapi.SetItemEntryTypeParams) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	var body openapi.SetItemEntryTypeJSONRequestBody
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	in := usecase.Input{"item_id": itemID.String(), "entry_type": body.EntryType}
	if version, ok := versionFromIfMatch(ifMatchOf(r)); ok {
		in["expected_version"] = version
	}
	actor, _ := appshared.ActorFrom(r.Context())
	out, err := c.UseCases.Invoke(r.Context(), setItemEntryTypeUseCase, actor, in)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	item := workItemResponse(out)
	w.Header().Set("ETag", etag(item.Version))
	writeJSON(w, r, http.StatusOK, item)
}

func putBehaviours(in usecase.Input, completable, workflow, record, schedulable, estimable *bool, children *[]string, position *int) {
	for field, value := range map[string]*bool{
		"completable": completable, "workflow": workflow, "record": record,
		"schedulable": schedulable, "estimable": estimable,
	} {
		if value != nil {
			in[field] = *value
		}
	}
	if children != nil {
		in["allowed_children"] = *children
	}
	if position != nil {
		in["position"] = *position
	}
}

func entryTypeResponse(out usecase.Output) openapi.EntryType {
	flag := func(field string) bool {
		value, _ := out[field].(bool)
		return value
	}
	children, _ := out["allowed_children"].([]string)
	if children == nil {
		children = []string{}
	}
	t := openapi.EntryType{
		Key: out.String("key"), Level: openapi.ItemType(out.String("level")), System: flag("system"),
		Icon: out.String("icon"), ColorToken: out.String("color_token"),
		Completable: flag("completable"), Workflow: flag("workflow"), Record: flag("record"),
		Schedulable: flag("schedulable"), Estimable: flag("estimable"),
		AllowedChildren: children, Position: out.Int("position"), Archived: flag("archived"),
		Version: out.Int("version"),
	}
	if name := out.String("name"); name != "" {
		t.Name = &name
	}
	return t
}
