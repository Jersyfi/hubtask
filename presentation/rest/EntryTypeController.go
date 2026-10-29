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

// The registry's own operations are answered by the generated mapping (Doors.gen.go); changing an
// entry's type is written here because its answer is the entry, drawn by workItemResponse.
const setItemEntryTypeUseCase = "SetItemEntryType"

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
	if form, held := out["form"]; held {
		var typed openapi.EntryForm
		if remarshal(form, &typed) == nil {
			t.Form = &typed
		}
	}
	return t
}
