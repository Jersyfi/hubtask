// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// SCIM 2.0's wire format (RFC 7643, RFC 7644): reading what Entra ID, Okta, Keycloak and Google
// send, and writing what they read. Translation only - what a directory may do is the Directory
// service's (UC-ID-28).

const (
	scimContentType = "application/scim+json"
	scimUserSchema  = "urn:ietf:params:scim:schemas:core:2.0:User"
	scimGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
	scimListSchema  = "urn:ietf:params:scim:api:messages:2.0:ListResponse"
	scimErrorSchema = "urn:ietf:params:scim:api:messages:2.0:Error"
	scimPatchSchema = "urn:ietf:params:scim:api:messages:2.0:PatchOp"
	scimMaxBody     = 1 << 20
)

func malformedScim() error {
	return shared.ErrMalformedRequest.WithDetail("directory.request_malformed")
}

// scimFilter reads the one form of filter served: `attribute eq "value"` (RFC 7644 §3.4.2.2), the
// attribute and operator in any case. Anything else is refused, never answered as everybody.
func scimFilter(raw string, allowed ...string) (identityservice.DirectoryFilter, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return identityservice.DirectoryFilter{}, nil
	}
	attribute, rest, ok := strings.Cut(raw, " ")
	if !ok {
		return identityservice.DirectoryFilter{}, unsupportedScimFilter()
	}
	op, value, ok := strings.Cut(strings.TrimSpace(rest), " ")
	if !ok || !strings.EqualFold(op, "eq") {
		return identityservice.DirectoryFilter{}, unsupportedScimFilter()
	}
	value = strings.TrimSpace(value)
	unquoted, err := strconv.Unquote(value)
	if err != nil {
		return identityservice.DirectoryFilter{}, unsupportedScimFilter()
	}
	for _, name := range allowed {
		if strings.EqualFold(attribute, name) {
			return identityservice.DirectoryFilter{Attribute: name, Value: unquoted}, nil
		}
	}
	return identityservice.DirectoryFilter{}, unsupportedScimFilter()
}

func unsupportedScimFilter() error {
	return shared.ErrValidation.WithDetail("directory.filter_unsupported")
}

// scimBool reads a boolean as directories send it: a JSON boolean, or Entra's "True" and "False".
func scimBool(v any) (bool, error) {
	switch b := v.(type) {
	case bool:
		return b, nil
	case string:
		parsed, err := strconv.ParseBool(strings.ToLower(strings.TrimSpace(b)))
		if err == nil {
			return parsed, nil
		}
	}
	return false, shared.ErrValidation.WithDetail("directory.value_invalid").
		WithFields(shared.FieldError{Path: "/active", Code: "directory.value_invalid"})
}

func scimString(v any) string {
	s, _ := v.(string)
	return s
}

// scimUserIn is a user as a directory posts or puts it.
type scimUserIn struct {
	UserName    string `json:"userName"`
	DisplayName string `json:"displayName"`
	ExternalID  string `json:"externalId"`
	Active      any    `json:"active"`
	Name        struct {
		Formatted  string `json:"formatted"`
		GivenName  string `json:"givenName"`
		FamilyName string `json:"familyName"`
	} `json:"name"`
	Emails []struct {
		Value   string `json:"value"`
		Primary bool   `json:"primary"`
	} `json:"emails"`
}

func decodeScim(body []byte, into any) error {
	if len(body) > scimMaxBody {
		return malformedScim()
	}
	d := json.NewDecoder(bytes.NewReader(body))
	if err := d.Decode(into); err != nil {
		return malformedScim()
	}
	return nil
}

// address is the address the person signs in with: `userName` where it is one, else the primary
// address, else the first.
func (u scimUserIn) address() string {
	if strings.Contains(u.UserName, "@") {
		return strings.TrimSpace(u.UserName)
	}
	for _, e := range u.Emails {
		if e.Primary {
			return strings.TrimSpace(e.Value)
		}
	}
	if len(u.Emails) > 0 {
		return strings.TrimSpace(u.Emails[0].Value)
	}
	return strings.TrimSpace(u.UserName)
}

// shown is the name shown: `displayName`, else the formatted name, else given and family name.
func (u scimUserIn) shown() string {
	if s := strings.TrimSpace(u.DisplayName); s != "" {
		return s
	}
	if s := strings.TrimSpace(u.Name.Formatted); s != "" {
		return s
	}
	return strings.TrimSpace(strings.TrimSpace(u.Name.GivenName) + " " + strings.TrimSpace(u.Name.FamilyName))
}

// active is `active`, true where it is absent (RFC 7643 §4.1.1 leaves it to the provider).
func (u scimUserIn) active() (bool, error) {
	if u.Active == nil {
		return true, nil
	}
	return scimBool(u.Active)
}

// scimPatch is a PATCH request's operations (RFC 7644 §3.5.2).
type scimPatch struct {
	Operations []struct {
		Op    string `json:"op"`
		Path  string `json:"path"`
		Value any    `json:"value"`
	} `json:"Operations"`
}

// userChange reads a PATCH of a user into the change it asks: `active`, `displayName`, `name`,
// `externalId` and `userName` (held to the address the person has), with or without a path.
func userChange(id shared.ID, patch scimPatch) (identityservice.ChangePersonCommand, error) {
	cmd := identityservice.ChangePersonCommand{AccountID: id}
	var given, family string
	set := func(path string, value any) error {
		switch strings.ToLower(path) {
		case "active":
			active, err := scimBool(value)
			if err != nil {
				return err
			}
			cmd.Active = &active
		case "displayname":
			s := scimString(value)
			cmd.DisplayName = &s
		case "externalid":
			s := scimString(value)
			cmd.ExternalID = &s
		case "username":
			s := scimString(value)
			if strings.Contains(s, "@") {
				cmd.Email = &s
			}
		case "name.formatted":
			s := scimString(value)
			if cmd.DisplayName == nil {
				cmd.DisplayName = &s
			}
		case "name.givenname":
			given = scimString(value)
		case "name.familyname":
			family = scimString(value)
		case "name":
			parts, _ := value.(map[string]any)
			given, family = scimString(parts["givenName"]), scimString(parts["familyName"])
			if formatted := scimString(parts["formatted"]); formatted != "" && cmd.DisplayName == nil {
				cmd.DisplayName = &formatted
			}
		}
		return nil
	}
	for _, op := range patch.Operations {
		switch strings.ToLower(op.Op) {
		case "add", "replace":
			if op.Path != "" {
				if err := set(op.Path, op.Value); err != nil {
					return cmd, err
				}
				continue
			}
			values, ok := op.Value.(map[string]any)
			if !ok {
				return cmd, malformedScim()
			}
			for key, value := range values {
				if err := set(key, value); err != nil {
					return cmd, err
				}
			}
		case "remove":
			if strings.EqualFold(op.Path, "externalId") {
				empty := ""
				cmd.ExternalID = &empty
			}
		default:
			return cmd, malformedScim()
		}
	}
	if cmd.DisplayName == nil && (given != "" || family != "") {
		joined := strings.TrimSpace(given + " " + family)
		cmd.DisplayName = &joined
	}
	return cmd, nil
}

// scimGroupIn is a group as a directory posts or puts it.
type scimGroupIn struct {
	DisplayName string           `json:"displayName"`
	ExternalID  *string          `json:"externalId"`
	Members     []scimMemberWire `json:"members"`
}

type scimMemberWire struct {
	Value string `json:"value"`
}

func memberIDs(members []scimMemberWire) []shared.ID {
	ids := make([]shared.ID, 0, len(members))
	for _, m := range members {
		if v := strings.TrimSpace(m.Value); v != "" {
			ids = append(ids, shared.ID(v))
		}
	}
	return ids
}

// membersOf reads a PATCH value naming members: a list of `{value}`, or one.
func membersOf(value any) []shared.ID {
	var ids []shared.ID
	add := func(v any) {
		if m, ok := v.(map[string]any); ok {
			if s := strings.TrimSpace(scimString(m["value"])); s != "" {
				ids = append(ids, shared.ID(s))
			}
		}
	}
	switch v := value.(type) {
	case []any:
		for _, item := range v {
			add(item)
		}
	case map[string]any:
		add(v)
	}
	return ids
}

// memberInPath reads `members[value eq "…"]`, the form Entra and Okta remove one member with.
func memberInPath(path string) (shared.ID, bool) {
	inner, ok := strings.CutPrefix(path, "members[")
	if !ok || !strings.HasSuffix(inner, "]") {
		return "", false
	}
	filter, err := scimFilter(strings.TrimSuffix(inner, "]"), "value")
	if err != nil || filter.Value == "" {
		return "", false
	}
	return shared.ID(filter.Value), true
}

// groupChange reads a PATCH of a group into the change it asks.
func groupChange(id shared.ID, patch scimPatch) (identityservice.ChangeGroupCommand, error) {
	cmd := identityservice.ChangeGroupCommand{GroupID: id}
	for _, op := range patch.Operations {
		kind := strings.ToLower(op.Op)
		path := strings.TrimSpace(op.Path)
		if member, ok := memberInPath(path); ok {
			if kind != "remove" {
				return cmd, malformedScim()
			}
			cmd.Remove = append(cmd.Remove, member)
			continue
		}
		switch {
		case strings.EqualFold(path, "members"):
			switch kind {
			case "add":
				cmd.Add = append(cmd.Add, membersOf(op.Value)...)
			case "remove":
				if op.Value == nil {
					empty := []shared.ID{}
					cmd.Replace, cmd.Add, cmd.Remove = &empty, nil, nil
				} else {
					cmd.Remove = append(cmd.Remove, membersOf(op.Value)...)
				}
			case "replace":
				members := membersOf(op.Value)
				cmd.Replace, cmd.Add, cmd.Remove = &members, nil, nil
			default:
				return cmd, malformedScim()
			}
		case strings.EqualFold(path, "displayName"):
			if kind == "remove" {
				return cmd, malformedScim()
			}
			s := scimString(op.Value)
			cmd.Name = &s
		case strings.EqualFold(path, "externalId"):
			s := ""
			if kind != "remove" {
				s = scimString(op.Value)
			}
			cmd.ExternalID = &s
		case path == "" && (kind == "add" || kind == "replace"):
			values, ok := op.Value.(map[string]any)
			if !ok {
				return cmd, malformedScim()
			}
			for key, value := range values {
				switch strings.ToLower(key) {
				case "displayname":
					s := scimString(value)
					cmd.Name = &s
				case "externalid":
					s := scimString(value)
					cmd.ExternalID = &s
				case "members":
					if kind == "add" {
						cmd.Add = append(cmd.Add, membersOf(value)...)
					} else {
						members := membersOf(value)
						cmd.Replace = &members
					}
				}
			}
		default:
			return cmd, malformedScim()
		}
	}
	return cmd, nil
}

// scimUserOut is a person as a directory reads them.
func scimUserOut(base string, p identityservice.DirectoryPerson) map[string]any {
	a := p.Account
	userName := a.Email
	if userName == "" {
		userName = a.SignInName
	}
	out := map[string]any{
		"schemas":     []string{scimUserSchema},
		"id":          a.ID.String(),
		"userName":    userName,
		"displayName": a.DisplayName,
		"name":        map[string]any{"formatted": a.DisplayName},
		"active":      a.DirectoryActive(),
		"meta": map[string]any{
			"resourceType": "User",
			"location":     base + "/api/v1/scim/v2/Users/" + a.ID.String(),
		},
	}
	if a.Email != "" {
		out["emails"] = []map[string]any{{"value": a.Email, "type": "work", "primary": true}}
	}
	if p.ExternalID != "" {
		out["externalId"] = p.ExternalID
	}
	return out
}

// scimLocation is the location a resource's meta names.
func scimLocation(out map[string]any) string {
	meta, _ := out["meta"].(map[string]any)
	location, _ := meta["location"].(string)
	return location
}

// scimGroupOut is a group as a directory reads it; members only where they were read.
func scimGroupOut(base string, g identityservice.DirectoryGroup, withMembers bool) map[string]any {
	out := map[string]any{
		"schemas":     []string{scimGroupSchema},
		"id":          g.Group.ID.String(),
		"displayName": g.Group.Name,
		"meta": map[string]any{
			"resourceType": "Group",
			"location":     base + "/api/v1/scim/v2/Groups/" + g.Group.ID.String(),
		},
	}
	if g.ExternalID != "" {
		out["externalId"] = g.ExternalID
	}
	if withMembers {
		members := make([]map[string]any, 0, len(g.Members))
		for _, id := range g.Members {
			members = append(members, map[string]any{
				"value": id.String(), "$ref": base + "/api/v1/scim/v2/Users/" + id.String(),
			})
		}
		out["members"] = members
	}
	return out
}

func scimList(total, start int, resources []map[string]any) map[string]any {
	return map[string]any{
		"schemas":      []string{scimListSchema},
		"totalResults": total,
		"startIndex":   start,
		"itemsPerPage": len(resources),
		"Resources":    resources,
	}
}

// scimErrorOf is a failure as SCIM writes it (RFC 7644 §3.12): the status the problem would
// carry, the scimType the RFC names for it, and the message code as `detail` - no sentence
// (rule 8).
func scimErrorOf(err error) (int, map[string]any) {
	problem := ProblemFrom(err, "")
	// SCIM knows no 422: a value a directory sent wrong is its 400 (RFC 7644 §3.12).
	if problem.Status == http.StatusUnprocessableEntity {
		problem.Status = http.StatusBadRequest
	}
	body := map[string]any{
		"schemas": []string{scimErrorSchema},
		"status":  strconv.Itoa(problem.Status),
	}
	detail := problem.DetailCode
	if detail == "" {
		detail = problem.Code
	}
	body["detail"] = detail
	switch {
	case problem.Status == http.StatusConflict:
		body["scimType"] = "uniqueness"
	case detail == "directory.filter_unsupported":
		body["scimType"] = "invalidFilter"
	case detail == "directory.address_immutable":
		body["scimType"] = "mutability"
	case detail == "directory.request_malformed":
		body["scimType"] = "invalidSyntax"
	case problem.Status == http.StatusBadRequest:
		body["scimType"] = "invalidValue"
	}
	return problem.Status, body
}

// scimServiceProviderConfig is what this service supports (RFC 7643 §5). The names are the
// protocol's identifiers, not text a person reads.
func scimServiceProviderConfig() map[string]any {
	return map[string]any{
		"schemas":        []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"},
		"patch":          map[string]any{"supported": true},
		"bulk":           map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0},
		"filter":         map[string]any{"supported": true, "maxResults": identityservice.MaxDirectoryPage},
		"changePassword": map[string]any{"supported": false},
		"sort":           map[string]any{"supported": false},
		"etag":           map[string]any{"supported": false},
		"authenticationSchemes": []map[string]any{{
			"type": "oauthbearertoken", "name": "OAuth Bearer Token",
			"description": "A service account token", "primary": true,
		}},
		"meta": map[string]any{"resourceType": "ServiceProviderConfig"},
	}
}

func scimResourceTypes(base string) []map[string]any {
	kind := func(id, endpoint, schema string) map[string]any {
		return map[string]any{
			"schemas":  []string{"urn:ietf:params:scim:schemas:core:2.0:ResourceType"},
			"id":       id,
			"name":     id,
			"endpoint": endpoint,
			"schema":   schema,
			"meta": map[string]any{
				"resourceType": "ResourceType", "location": base + "/api/v1/scim/v2/ResourceTypes/" + id,
			},
		}
	}
	return []map[string]any{kind("User", "/Users", scimUserSchema), kind("Group", "/Groups", scimGroupSchema)}
}

func scimSchemas() []map[string]any {
	attribute := func(name, kind string, required bool, mutability, uniqueness string) map[string]any {
		return map[string]any{
			"name": name, "type": kind, "multiValued": false, "required": required,
			"caseExact": false, "mutability": mutability, "returned": "default", "uniqueness": uniqueness,
		}
	}
	emails := attribute("emails", "complex", false, "readOnly", "none")
	emails["multiValued"] = true
	members := attribute("members", "complex", false, "readWrite", "none")
	members["multiValued"] = true
	schema := func(id, name string, attributes ...map[string]any) map[string]any {
		return map[string]any{
			"schemas":    []string{"urn:ietf:params:scim:schemas:core:2.0:Schema"},
			"id":         id,
			"name":       name,
			"attributes": attributes,
			"meta":       map[string]any{"resourceType": "Schema"},
		}
	}
	return []map[string]any{
		schema(scimUserSchema, "User",
			attribute("userName", "string", true, "immutable", "server"),
			attribute("displayName", "string", false, "readWrite", "none"),
			attribute("name", "complex", false, "readWrite", "none"),
			emails,
			attribute("active", "boolean", false, "readWrite", "none"),
			attribute("externalId", "string", false, "readWrite", "server"),
		),
		schema(scimGroupSchema, "Group",
			attribute("displayName", "string", true, "readWrite", "server"),
			attribute("externalId", "string", false, "readWrite", "server"),
			members,
		),
	}
}
