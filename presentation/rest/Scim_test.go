// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

func TestScimFilterReadsOneEquality(t *testing.T) {
	got, err := scimFilter(`UserName EQ "lena@example.org"`, "userName", "externalId")
	if err != nil || got.Attribute != "userName" || got.Value != "lena@example.org" {
		t.Fatalf("got %+v, %v", got, err)
	}
	for _, refused := range []string{`title eq "x"`, `userName co "x"`, `userName eq x`, `userName`} {
		if _, err := scimFilter(refused, "userName"); err == nil {
			t.Errorf("%q was answered, want refused", refused)
		}
	}
}

// Entra ID switches a person off with a string, and names given and family name by path.
func TestAUserPatchReadsWhatEntraSends(t *testing.T) {
	var patch scimPatch
	body := `{"schemas":["urn:ietf:params:scim:api:messages:2.0:PatchOp"],"Operations":[
		{"op":"Replace","path":"active","value":"False"},
		{"op":"Add","path":"name.givenName","value":"Lena"},
		{"op":"Add","path":"name.familyName","value":"Berg"},
		{"op":"Replace","path":"externalId","value":"lena.berg"}]}`
	if err := decodeScim([]byte(body), &patch); err != nil {
		t.Fatal(err)
	}
	cmd, err := userChange("id", patch)
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Active == nil || *cmd.Active {
		t.Errorf("active %v, want false", cmd.Active)
	}
	if cmd.DisplayName == nil || *cmd.DisplayName != "Lena Berg" {
		t.Errorf("display name %v", cmd.DisplayName)
	}
	if cmd.ExternalID == nil || *cmd.ExternalID != "lena.berg" {
		t.Errorf("external id %v", cmd.ExternalID)
	}
}

// Okta sends a replace without a path, its value the attributes.
func TestAUserPatchWithoutAPathReadsTheValue(t *testing.T) {
	var patch scimPatch
	if err := decodeScim([]byte(`{"Operations":[{"op":"replace","value":{"active":false}}]}`), &patch); err != nil {
		t.Fatal(err)
	}
	cmd, err := userChange("id", patch)
	if err != nil || cmd.Active == nil || *cmd.Active {
		t.Fatalf("%+v, %v", cmd, err)
	}
}

func TestAGroupPatchAddsAndRemovesMembers(t *testing.T) {
	var patch scimPatch
	body := `{"Operations":[
		{"op":"add","path":"members","value":[{"value":"a"},{"value":"b"}]},
		{"op":"remove","path":"members[value eq \"c\"]"},
		{"op":"remove","path":"members","value":[{"value":"d"}]},
		{"op":"replace","path":"displayName","value":"Design"}]}`
	if err := decodeScim([]byte(body), &patch); err != nil {
		t.Fatal(err)
	}
	cmd, err := groupChange("g", patch)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cmd.Add, []shared.ID{"a", "b"}) || !slices.Equal(cmd.Remove, []shared.ID{"c", "d"}) {
		t.Errorf("add %v remove %v", cmd.Add, cmd.Remove)
	}
	if cmd.Name == nil || *cmd.Name != "Design" {
		t.Errorf("name %v", cmd.Name)
	}
}

func TestScimErrorsCarryTheirTypeAndCode(t *testing.T) {
	cases := map[error][2]string{
		shared.ErrConflict.WithDetail("accounts.email_taken"):           {"409", "uniqueness"},
		shared.ErrValidation.WithDetail("directory.filter_unsupported"): {"400", "invalidFilter"},
		shared.ErrValidation.WithDetail("directory.address_immutable"):  {"400", "mutability"},
	}
	for err, want := range cases {
		_, body := scimErrorOf(err)
		if body["status"] != want[0] || body["scimType"] != want[1] {
			t.Errorf("%v: %v, want %v", err, body, want)
		}
		if detail, _ := body["detail"].(string); strings.Contains(detail, " ") {
			t.Errorf("detail %q is a sentence, want a message code", detail)
		}
	}
}

// fakeDirectory answers one person, as the service would.
type fakeDirectory struct {
	ScimDirectory
	provisioned identityservice.ProvisionPersonCommand
	filter      identityservice.DirectoryFilter
}

func (f *fakeDirectory) ProvisionPerson(_ context.Context, _ appshared.ActorContext, cmd identityservice.ProvisionPersonCommand) (identityservice.DirectoryPerson, error) {
	f.provisioned = cmd
	return identityservice.DirectoryPerson{
		Account:    domain.Account{ID: "01936f2a-7c1e-7000-8000-0000000000a2", Email: "lena@example.org", DisplayName: "Lena", Status: domain.AccountInvited},
		ExternalID: cmd.ExternalID,
	}, nil
}

func (f *fakeDirectory) People(_ context.Context, _ appshared.ActorContext, filter identityservice.DirectoryFilter, start, _ int) (identityservice.DirectoryPage[identityservice.DirectoryPerson], error) {
	f.filter = filter
	return identityservice.DirectoryPage[identityservice.DirectoryPerson]{Start: start}, nil
}

func TestScimCreateUserAnswersTheResource(t *testing.T) {
	directory := &fakeDirectory{}
	c := &RestController{Directory: directory, BaseURL: "https://hub.example.org"}
	body := `{"schemas":["urn:ietf:params:scim:schemas:core:2.0:User"],"userName":"lena@example.org",
		"name":{"givenName":"Lena","familyName":"Berg"},"externalId":"00u1","active":true}`
	w := httptest.NewRecorder()
	c.CreateScimUser(w, httptest.NewRequest(http.MethodPost, "/api/v1/scim/v2/Users", strings.NewReader(body)))

	if w.Code != http.StatusCreated || w.Header().Get("Content-Type") != scimContentType {
		t.Fatalf("status %d, type %q", w.Code, w.Header().Get("Content-Type"))
	}
	if directory.provisioned.DisplayName != "Lena Berg" || !directory.provisioned.Active {
		t.Errorf("provisioned %+v", directory.provisioned)
	}
	var out map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out["externalId"] != "00u1" || out["active"] != true || out["userName"] != "lena@example.org" {
		t.Errorf("answered %v", out)
	}
	if !strings.HasSuffix(w.Header().Get("Location"), "/api/v1/scim/v2/Users/01936f2a-7c1e-7000-8000-0000000000a2") {
		t.Errorf("location %q", w.Header().Get("Location"))
	}
}

func TestScimListUsersPassesTheFilterOn(t *testing.T) {
	directory := &fakeDirectory{}
	c := &RestController{Directory: directory}
	filter := `userName eq "lena@example.org"`
	w := httptest.NewRecorder()
	c.ListScimUsers(w, httptest.NewRequest(http.MethodGet, "/api/v1/scim/v2/Users", nil), openapi.ListScimUsersParams{Filter: &filter})
	if w.Code != http.StatusOK || directory.filter.Attribute != "userName" {
		t.Fatalf("status %d, filter %+v", w.Code, directory.filter)
	}
	var out map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if out["totalResults"] != float64(0) || out["Resources"] == nil {
		t.Errorf("answered %v", out)
	}
}

func TestScimIsNotFoundWhereNotWired(t *testing.T) {
	w := httptest.NewRecorder()
	(&RestController{}).GetScimServiceProviderConfig(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status %d, want 404", w.Code)
	}
}
