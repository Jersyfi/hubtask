// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// ScimDirectory is the application's side of the SCIM door (UC-ID-28). Not a catalogue entry: its
// operations are SCIM's resource semantics, which no other door speaks; the same acts are on every
// door as invitations and groups.
type ScimDirectory interface {
	ProvisionPerson(ctx context.Context, actor appshared.ActorContext, cmd identityservice.ProvisionPersonCommand) (identityservice.DirectoryPerson, error)
	Person(ctx context.Context, actor appshared.ActorContext, id shared.ID) (identityservice.DirectoryPerson, error)
	People(ctx context.Context, actor appshared.ActorContext, filter identityservice.DirectoryFilter, start, count int) (identityservice.DirectoryPage[identityservice.DirectoryPerson], error)
	ChangePerson(ctx context.Context, actor appshared.ActorContext, cmd identityservice.ChangePersonCommand) (identityservice.DirectoryPerson, error)
	Deprovision(ctx context.Context, actor appshared.ActorContext, id shared.ID) error
	ProvisionGroup(ctx context.Context, actor appshared.ActorContext, name, externalID string, members []shared.ID) (identityservice.DirectoryGroup, error)
	Group(ctx context.Context, actor appshared.ActorContext, id shared.ID, withMembers bool) (identityservice.DirectoryGroup, error)
	GroupsPage(ctx context.Context, actor appshared.ActorContext, filter identityservice.DirectoryFilter, start, count int, withMembers bool) (identityservice.DirectoryPage[identityservice.DirectoryGroup], error)
	ChangeGroup(ctx context.Context, actor appshared.ActorContext, cmd identityservice.ChangeGroupCommand) (identityservice.DirectoryGroup, error)
	RemoveGroup(ctx context.Context, actor appshared.ActorContext, id shared.ID) error
}

func writeScim(w http.ResponseWriter, r *http.Request, status int, body any) {
	w.Header().Set("Content-Type", scimContentType)
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	if body == nil {
		return
	}
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.WarnContext(r.Context(), "writing the response body failed", slog.String("error", err.Error()))
	}
}

func writeScimError(w http.ResponseWriter, r *http.Request, err error) {
	status, body := scimErrorOf(err)
	writeScim(w, r, status, body)
}

// scimServed answers whether the door is wired, answering a SCIM 404 where it is not.
func (c *RestController) scimServed(w http.ResponseWriter, r *http.Request) bool {
	if c.Directory == nil {
		writeScimError(w, r, shared.ErrNotFound.WithDetail("directory.not_served"))
		return false
	}
	return true
}

func scimBody(r *http.Request, into any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, scimMaxBody+1))
	if err != nil {
		return malformedScim()
	}
	return decodeScim(raw, into)
}

func scimPaging(start, count *int) (int, int) {
	s, n := 1, identityservice.MaxDirectoryPage
	if start != nil {
		s = *start
	}
	if count != nil {
		n = *count
	}
	return s, n
}

func membersExcluded(excluded *string) bool {
	if excluded == nil {
		return false
	}
	for _, name := range strings.Split(*excluded, ",") {
		if strings.EqualFold(strings.TrimSpace(name), "members") {
			return true
		}
	}
	return false
}

func (c *RestController) GetScimServiceProviderConfig(w http.ResponseWriter, r *http.Request) {
	if c.scimServed(w, r) {
		writeScim(w, r, http.StatusOK, scimServiceProviderConfig())
	}
}

func (c *RestController) ListScimResourceTypes(w http.ResponseWriter, r *http.Request) {
	if c.scimServed(w, r) {
		types := scimResourceTypes(c.BaseURL)
		writeScim(w, r, http.StatusOK, scimList(len(types), 1, types))
	}
}

func (c *RestController) ListScimSchemas(w http.ResponseWriter, r *http.Request) {
	if c.scimServed(w, r) {
		schemas := scimSchemas()
		writeScim(w, r, http.StatusOK, scimList(len(schemas), 1, schemas))
	}
}

func (c *RestController) ListScimUsers(w http.ResponseWriter, r *http.Request, params openapi.ListScimUsersParams) {
	if !c.scimServed(w, r) {
		return
	}
	raw := ""
	if params.Filter != nil {
		raw = *params.Filter
	}
	filter, err := scimFilter(raw, "userName", "externalId", "id", "emails.value")
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	start, count := scimPaging(params.StartIndex, params.Count)
	page, err := c.Directory.People(r.Context(), actorOf(r), filter, start, count)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	resources := make([]map[string]any, 0, len(page.Items))
	for _, person := range page.Items {
		resources = append(resources, scimUserOut(c.BaseURL, person))
	}
	writeScim(w, r, http.StatusOK, scimList(page.Total, page.Start, resources))
}

func (c *RestController) CreateScimUser(w http.ResponseWriter, r *http.Request) {
	if !c.scimServed(w, r) {
		return
	}
	var in scimUserIn
	if err := scimBody(r, &in); err != nil {
		writeScimError(w, r, err)
		return
	}
	active, err := in.active()
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	person, err := c.Directory.ProvisionPerson(r.Context(), actorOf(r), identityservice.ProvisionPersonCommand{
		Email: in.address(), DisplayName: in.shown(), ExternalID: in.ExternalID, Active: active,
	})
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	out := scimUserOut(c.BaseURL, person)
	w.Header().Set("Location", out["meta"].(map[string]any)["location"].(string))
	writeScim(w, r, http.StatusCreated, out)
}

func (c *RestController) GetScimUser(w http.ResponseWriter, r *http.Request, id openapi.ScimUserId) {
	if !c.scimServed(w, r) {
		return
	}
	person, err := c.Directory.Person(r.Context(), actorOf(r), scimID(id))
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	writeScim(w, r, http.StatusOK, scimUserOut(c.BaseURL, person))
}

func (c *RestController) ReplaceScimUser(w http.ResponseWriter, r *http.Request, id openapi.ScimUserId) {
	if !c.scimServed(w, r) {
		return
	}
	var in scimUserIn
	if err := scimBody(r, &in); err != nil {
		writeScimError(w, r, err)
		return
	}
	active, err := in.active()
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	address, shown, external := in.address(), in.shown(), in.ExternalID
	cmd := identityservice.ChangePersonCommand{AccountID: scimID(id), Active: &active, ExternalID: &external}
	if strings.Contains(address, "@") {
		cmd.Email = &address
	}
	if shown != "" {
		cmd.DisplayName = &shown
	}
	c.changeScimUser(w, r, cmd)
}

func (c *RestController) PatchScimUser(w http.ResponseWriter, r *http.Request, id openapi.ScimUserId) {
	if !c.scimServed(w, r) {
		return
	}
	var patch scimPatch
	if err := scimBody(r, &patch); err != nil {
		writeScimError(w, r, err)
		return
	}
	cmd, err := userChange(scimID(id), patch)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	c.changeScimUser(w, r, cmd)
}

func (c *RestController) changeScimUser(w http.ResponseWriter, r *http.Request, cmd identityservice.ChangePersonCommand) {
	person, err := c.Directory.ChangePerson(r.Context(), actorOf(r), cmd)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	writeScim(w, r, http.StatusOK, scimUserOut(c.BaseURL, person))
}

func (c *RestController) DeleteScimUser(w http.ResponseWriter, r *http.Request, id openapi.ScimUserId) {
	if !c.scimServed(w, r) {
		return
	}
	if err := c.Directory.Deprovision(r.Context(), actorOf(r), scimID(id)); err != nil {
		writeScimError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *RestController) ListScimGroups(w http.ResponseWriter, r *http.Request, params openapi.ListScimGroupsParams) {
	if !c.scimServed(w, r) {
		return
	}
	raw := ""
	if params.Filter != nil {
		raw = *params.Filter
	}
	filter, err := scimFilter(raw, "displayName", "externalId", "id")
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	start, count := scimPaging(params.StartIndex, params.Count)
	withMembers := !membersExcluded(params.ExcludedAttributes)
	page, err := c.Directory.GroupsPage(r.Context(), actorOf(r), filter, start, count, withMembers)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	resources := make([]map[string]any, 0, len(page.Items))
	for _, group := range page.Items {
		resources = append(resources, scimGroupOut(c.BaseURL, group, withMembers))
	}
	writeScim(w, r, http.StatusOK, scimList(page.Total, page.Start, resources))
}

func (c *RestController) CreateScimGroup(w http.ResponseWriter, r *http.Request) {
	if !c.scimServed(w, r) {
		return
	}
	var in scimGroupIn
	if err := scimBody(r, &in); err != nil {
		writeScimError(w, r, err)
		return
	}
	external := ""
	if in.ExternalID != nil {
		external = *in.ExternalID
	}
	group, err := c.Directory.ProvisionGroup(r.Context(), actorOf(r), in.DisplayName, external, memberIDs(in.Members))
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	out := scimGroupOut(c.BaseURL, group, true)
	w.Header().Set("Location", out["meta"].(map[string]any)["location"].(string))
	writeScim(w, r, http.StatusCreated, out)
}

func (c *RestController) GetScimGroup(w http.ResponseWriter, r *http.Request, id openapi.ScimGroupId, params openapi.GetScimGroupParams) {
	if !c.scimServed(w, r) {
		return
	}
	withMembers := !membersExcluded(params.ExcludedAttributes)
	group, err := c.Directory.Group(r.Context(), actorOf(r), scimID(id), withMembers)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	writeScim(w, r, http.StatusOK, scimGroupOut(c.BaseURL, group, withMembers))
}

func (c *RestController) ReplaceScimGroup(w http.ResponseWriter, r *http.Request, id openapi.ScimGroupId) {
	if !c.scimServed(w, r) {
		return
	}
	var in scimGroupIn
	if err := scimBody(r, &in); err != nil {
		writeScimError(w, r, err)
		return
	}
	members := memberIDs(in.Members)
	cmd := identityservice.ChangeGroupCommand{GroupID: scimID(id), Replace: &members, ExternalID: in.ExternalID}
	if in.DisplayName != "" {
		cmd.Name = &in.DisplayName
	}
	c.changeScimGroup(w, r, cmd, true)
}

func (c *RestController) PatchScimGroup(w http.ResponseWriter, r *http.Request, id openapi.ScimGroupId) {
	if !c.scimServed(w, r) {
		return
	}
	var patch scimPatch
	if err := scimBody(r, &patch); err != nil {
		writeScimError(w, r, err)
		return
	}
	cmd, err := groupChange(scimID(id), patch)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	c.changeScimGroup(w, r, cmd, true)
}

func (c *RestController) changeScimGroup(w http.ResponseWriter, r *http.Request, cmd identityservice.ChangeGroupCommand, withMembers bool) {
	group, err := c.Directory.ChangeGroup(r.Context(), actorOf(r), cmd)
	if err != nil {
		writeScimError(w, r, err)
		return
	}
	writeScim(w, r, http.StatusOK, scimGroupOut(c.BaseURL, group, withMembers))
}

func (c *RestController) DeleteScimGroup(w http.ResponseWriter, r *http.Request, id openapi.ScimGroupId) {
	if !c.scimServed(w, r) {
		return
	}
	if err := c.Directory.RemoveGroup(r.Context(), actorOf(r), scimID(id)); err != nil {
		writeScimError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// scimID is the path's identifier as the service reads it; one that is no identifier here is not
// found there, as SCIM expects.
func scimID(raw string) shared.ID {
	id, err := shared.ParseID(raw)
	if err != nil {
		return shared.ID("00000000-0000-0000-0000-000000000000")
	}
	return id
}
