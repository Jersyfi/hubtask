// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The routes C-06 adds beside the three-step upload: the record, its removal, the cover, and the
// attachments. What each of these tests is about is the mapping - the catalogue's projection into
// the contract's schema - because that is the layer where a field quietly goes missing.

var coveredItemID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")

func readyProjection() usecase.Output {
	out := stagedProjection()
	out["status"] = "READY"
	out["ref_count"] = 2
	delete(out, "upload")
	out["download"] = map[string]any{
		"url": "https://storage.example/media/x?sig=1", "method": "GET",
		"expires_at": mediaNow.Add(5 * time.Minute),
	}
	return out
}

func TestReadingAMediaObjectAnswersWithItsDownloadTarget(t *testing.T) {
	cat := &catalogue{out: readyProjection()}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/media/"+mediaID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != getMediaUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}

	var answered struct {
		Status   string `json:"status"`
		RefCount int    `json:"ref_count"`
		Download *struct {
			URL    string `json:"url"`
			Method string `json:"method"`
		} `json:"download"`
		Upload *struct{} `json:"upload"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if answered.Status != "READY" || answered.RefCount != 2 {
		t.Errorf("the answer is %+v", answered)
	}
	if answered.Download == nil || answered.Download.Method != "GET" {
		t.Fatalf("the download target is %+v", answered.Download)
	}
	// A sealed object has nowhere to upload to, and the key is absent rather than null.
	if answered.Upload != nil {
		t.Error("a READY object was given an upload target")
	}
}

func TestRemovingAMediaObjectAnswersWithoutABody(t *testing.T) {
	cat := &catalogue{out: usecase.Output{}}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		APIBasePath+"/media/"+mediaID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusNoContent {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != deleteMediaUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	// The record is on its way out, so there is no state a body could describe.
	if response.Body.Len() != 0 {
		t.Errorf("the answer carries %q", response.Body.String())
	}
}

// coveredProjection is an entry wearing an image cover, in the catalogue's words.
func coveredProjection() usecase.Output {
	return usecase.Output{
		"id":            coveredItemID.String(),
		"type":          "TASK",
		"collection_id": mediaTenant.String(),
		"title":         "Plan the trip",
		"completion":    map[string]any{"is_completed": false},
		"cover": map[string]any{
			"kind": "IMAGE", "color_token": nil, "media_id": mediaID.String(),
		},
		"created_by": mediaTenant.String(),
		"created_at": mediaNow,
		"updated_at": mediaNow,
		"version":    4,
	}
}

func TestSettingACoverAnswersWithTheEntryAndItsNewVersion(t *testing.T) {
	cat := &catalogue{out: coveredProjection()}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	body := `{"kind":"IMAGE","media_id":"` + mediaID.String() + `"}`
	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/items/"+coveredItemID.String()+"/cover", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("If-Match", `"3"`)

	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != setCoverUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	// The If-Match reaches the catalogue as the version the caller read.
	if cat.in["expected_version"] != 3 {
		t.Errorf("the expected version is %v", cat.in["expected_version"])
	}
	if cat.in["media_id"] != mediaID.String() {
		t.Errorf("the media object named is %v", cat.in["media_id"])
	}
	// The ETag is the version after the change, which is what a client needs in order to follow
	// the cover with an edit.
	if got := response.Header().Get("ETag"); got != `"4"` {
		t.Errorf("ETag is %q, want the version after the change", got)
	}

	var answered struct {
		Cover *struct {
			Kind       string  `json:"kind"`
			MediaID    string  `json:"media_id"`
			ColorToken *string `json:"color_token"`
		} `json:"cover"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if answered.Cover == nil || answered.Cover.Kind != "IMAGE" ||
		answered.Cover.MediaID != mediaID.String() {
		t.Fatalf("the cover is %+v", answered.Cover)
	}
	if answered.Cover.ColorToken != nil {
		t.Error("an image cover carries a colour token")
	}
}

// An entry with no cover carries no `cover` key at all: a client reading it as present would draw
// a card with a picture nobody chose.
func TestAnEntryWithoutACoverCarriesNoCoverKey(t *testing.T) {
	projection := coveredProjection()
	projection["cover"] = nil
	cat := &catalogue{out: projection}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		APIBasePath+"/items/"+coveredItemID.String()+"/cover", nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != clearCoverUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if strings.Contains(response.Body.String(), `"cover"`) {
		t.Errorf("the answer names a cover: %s", response.Body.String())
	}
}

func TestAttachingAnswersWithWhatTheEntryNowCarries(t *testing.T) {
	cat := &catalogue{out: usecase.Output{
		"item_id": coveredItemID.String(), "media_ids": []string{mediaID.String()},
	}}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodPut,
		APIBasePath+"/items/"+coveredItemID.String()+"/attachments/"+mediaID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != attachMediaUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	// No ETag: neither direction touches the entry's own row, so no version was spent and there is
	// none to report.
	if got := response.Header().Get("ETag"); got != "" {
		t.Errorf("the attachment answered with ETag %q", got)
	}

	var answered struct {
		ItemID   string   `json:"item_id"`
		MediaIDs []string `json:"media_ids"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if answered.ItemID != coveredItemID.String() || len(answered.MediaIDs) != 1 {
		t.Errorf("the answer is %+v", answered)
	}
}

func TestDetachingAnswersWithAnEmptyArrayRatherThanNull(t *testing.T) {
	cat := &catalogue{out: usecase.Output{
		"item_id": coveredItemID.String(), "media_ids": []string{},
	}}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		APIBasePath+"/items/"+coveredItemID.String()+"/attachments/"+mediaID.String(), nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != detachMediaUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	// An empty set is `[]`, never null: a client reading the field unconditionally is the point.
	if !strings.Contains(response.Body.String(), `"media_ids":[]`) {
		t.Errorf("the answer is %s", response.Body.String())
	}
}

func TestListingAttachmentsAnswersAPageOfMediaRecords(t *testing.T) {
	cat := &catalogue{out: usecase.Output{
		"data": []usecase.Output{readyProjection()},
		"page": map[string]any{"next_cursor": "abc", "has_more": true},
	}}
	controller := mediaController(cat, &mediaContent{}, &mediaTokens{})

	request := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		APIBasePath+"/items/"+coveredItemID.String()+"/attachments?size=25", nil)
	response := httptest.NewRecorder()
	controller.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status %d, body %s", response.Code, response.Body.String())
	}
	if cat.name != listAttachmentsUseCase {
		t.Errorf("the controller invoked %q", cat.name)
	}
	if cat.in["size"] != 25 {
		t.Errorf("the page size asked for is %v", cat.in["size"])
	}

	var answered struct {
		Data []struct {
			ID     string `json:"id"`
			Status string `json:"status"`
		} `json:"data"`
		Page struct {
			NextCursor *string `json:"next_cursor"`
			HasMore    bool    `json:"has_more"`
		} `json:"page"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &answered); err != nil {
		t.Fatalf("the answer is not the schema's shape: %v", err)
	}
	if len(answered.Data) != 1 || answered.Data[0].Status != "READY" {
		t.Fatalf("the page is %+v", answered.Data)
	}
	if !answered.Page.HasMore || answered.Page.NextCursor == nil {
		t.Errorf("the walk's state is %+v", answered.Page)
	}
}
