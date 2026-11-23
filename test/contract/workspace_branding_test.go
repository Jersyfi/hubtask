// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build contract

package contract

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	mediarepo "github.com/Jersyfi/hubtask/core/application/repository/media"
	"github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	catalogue "github.com/Jersyfi/hubtask/core/application/usecase"
	identitydomain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/media"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	portclock "github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/presentation/rest"
)

// brandedWorkspace is the workspace's row in memory, read by the workspace's door and written by
// the branding's.
type brandedWorkspace struct{ row identitydomain.Workspace }

func (w *brandedWorkspace) Find(context.Context) (identitydomain.Workspace, error) { return w.row, nil }
func (w *brandedWorkspace) Update(context.Context, identitydomain.Workspace, int, time.Time) (bool, error) {
	return false, nil
}
func (w *brandedWorkspace) Lock(context.Context) (identitydomain.Branding, error) {
	return w.row.Branding, nil
}
func (w *brandedWorkspace) Set(_ context.Context, branding identitydomain.Branding, _ time.Time) error {
	w.row.Branding = branding
	return nil
}

// uploads is the media port as far as a picture needs it.
type uploads struct {
	mediarepo.Objects
	stored map[shared.ID]media.Object
}

func (u uploads) Find(_ context.Context, id shared.ID) (media.Object, error) {
	if object, held := u.stored[id]; held {
		return object, nil
	}
	return media.Object{}, shared.ErrNotFound.WithDetail("media.not_found")
}
func (u uploads) AdjustRefCount(context.Context, shared.ID, int) error { return nil }

// UC-INS-22 checks 1, 2 and 3 at the REST door: a picture is set and taken away, the choice is set,
// each answer is the contract's WorkspaceBranding, and the workspace's own answer carries it; an SVG,
// a video, a sound, a document and markup are refused with their code, and a body naming a key the operation does not declare is refused.
func TestAWorkspacesBrandingAtTheRestDoor(t *testing.T) {
	spec := contractSpec(t)
	tenant := shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	admin := shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	logo := shared.MustParseID("0192f000-0000-7000-8000-0000000000a1")
	svg := shared.MustParseID("0192f000-0000-7000-8000-0000000000a2")
	workspace := &brandedWorkspace{row: identitydomain.Workspace{
		Tenant: identitydomain.Tenant{
			ID: tenant, Slug: "acme", DisplayName: "Acme", Status: identitydomain.TenantActive,
			DefaultLocale: "en", DefaultTimeZone: "UTC",
		},
		Version: 1,
	}}
	picture := media.Object{
		ID: logo, TenantID: tenant, Usage: media.UsageWorkspaceLogo, Status: media.StatusReady,
		ContentType: "image/png", CreatedBy: admin,
	}
	drawing := picture
	drawing.ID, drawing.ContentType = svg, "image/svg+xml"
	stored := map[shared.ID]media.Object{logo: picture, svg: drawing}
	// Each family a browser renders or plays that is no PNG, JPEG, GIF or WebP.
	refused := map[string]shared.ID{"image/svg+xml": svg}
	for i, family := range []string{"video/mp4", "audio/mpeg", "application/pdf", "text/html"} {
		other := picture
		other.ID, other.ContentType = shared.MustParseID(fmt.Sprintf("0192f000-0000-7000-8000-0000000000b%d", i)), family
		stored[other.ID], refused[family] = other, other.ID
	}
	writer := identity.WorkspaceWriter{
		Workspaces: workspace, Authorizer: permitted{}, Audit: &kept{},
		UnitOfWork: directly{}, Clock: portclock.Fixed(time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)),
	}
	brander := identity.WorkspaceBrander{
		Writer: writer, Branding: workspace,
		Objects: uploads{stored: stored},
	}
	registry, err := catalogue.NewRegistry(nil,
		identity.ReadWorkspace{Writer: writer}.Descriptor(),
		identity.SetWorkspaceHeader{Brander: brander}.Descriptor(),
		identity.SetWorkspacePicture{Brander: brander}.Descriptor(),
		identity.RemoveWorkspacePicture{Brander: brander}.Descriptor(),
	)
	if err != nil {
		t.Fatal(err)
	}
	controller := rest.NewRestController()
	controller.UseCases = registry
	routes := controller.Routes()
	ctx := appshared.ContextWithActor(context.Background(), appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: tenant, AccountID: admin,
		Scopes: []string{"workspace:read", "workspace:manage"},
	})
	call := func(method, path, body string) (int, []byte) {
		t.Helper()
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		request := httptest.NewRequestWithContext(ctx, method, rest.APIBasePath+path, reader)
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		response := httptest.NewRecorder()
		routes.ServeHTTP(response, request)
		return response.Code, response.Body.Bytes()
	}
	branding := func(status int, raw []byte) map[string]any {
		t.Helper()
		if status != http.StatusOK {
			t.Fatalf("answered %d %s", status, raw)
		}
		problems, err := spec.validateAgainst("WorkspaceBranding", raw)
		if err != nil {
			t.Fatal(err)
		}
		for _, problem := range problems {
			t.Errorf("WorkspaceBranding: %s", problem)
		}
		var answer map[string]any
		_ = json.Unmarshal(raw, &answer)
		return answer
	}

	set := branding(call(http.MethodPut, "/tenant/branding/LOGO_LIGHT", `{"media_id":"`+logo.String()+`"}`))
	if set["logo_light_media_id"] != logo.String() || set["header"] != "PRODUCT" || set["allowed"] != true {
		t.Errorf("setting the light logo answered %v", set)
	}
	if chosen := branding(call(http.MethodPut, "/tenant/branding", `{"header":"LOGO"}`)); chosen["header"] != "LOGO" {
		t.Errorf("choosing the logo alone answered %v", chosen)
	}

	status, raw := call(http.MethodGet, "/tenant", "")
	if status != http.StatusOK {
		t.Fatalf("reading the workspace: %d %s", status, raw)
	}
	problems, err := spec.validateAgainst("Workspace", raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, problem := range problems {
		t.Errorf("Workspace: %s", problem)
	}
	var read struct {
		Branding map[string]any `json:"branding"`
	}
	if err := json.Unmarshal(raw, &read); err != nil || read.Branding["header"] != "LOGO" ||
		read.Branding["logo_light_media_id"] != logo.String() {
		t.Errorf("the workspace answers branding %v (%v)", read.Branding, err)
	}

	removed := branding(call(http.MethodDelete, "/tenant/branding/LOGO_LIGHT", ""))
	if removed["logo_light_media_id"] != nil || removed["header"] != "PRODUCT" {
		t.Errorf("removing the last picture answered %v", removed)
	}

	for family, id := range refused {
		status, raw = call(http.MethodPut, "/tenant/branding/SYMBOL", `{"media_id":"`+id.String()+`"}`)
		if status != http.StatusUnprocessableEntity && status != http.StatusBadRequest ||
			!strings.Contains(string(raw), "media.workspace_logo_not_an_image") {
			t.Errorf("a file stored as %s answered %d %s", family, status, raw)
		}
	}
	status, raw = call(http.MethodPut, "/tenant/branding/SYMBOL", `{"media_id":"`+logo.String()+`","file_name":"x.png"}`)
	if status < 400 || status >= 500 {
		t.Errorf("an undeclared key answered %d %s", status, raw)
	}
	status, raw = call(http.MethodPut, "/tenant/branding/FAVICON", `{"media_id":"`+logo.String()+`"}`)
	if status < 400 || status >= 500 {
		t.Errorf("a fourth picture answered %d %s", status, raw)
	}
}
