// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strconv"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The catalogue names. The routes they are reached through come from the specification; the two
// are reconciled by the parity test rather than by these constants.
const (
	inviteAccountUseCase            = "InviteAccount"
	updateAccountPreferencesUseCase = "UpdateAccountPreferences"
	grantMembershipUseCase          = "GrantMembership"
	revokeMembershipUseCase         = "RevokeMembership"
	createGroupUseCase              = "CreateGroup"
	updateGroupUseCase              = "UpdateGroup"
	deleteGroupUseCase              = "DeleteGroup"
)

// InviteAccount answers POST /accounts:invite.
func (c *RestController) InviteAccount(w http.ResponseWriter, r *http.Request, _ openapi.InviteAccountParams) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AccountInvite
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), inviteAccountUseCase, actor, usecase.Input{
			"email":        string(body.Email),
			"display_name": optionalStringField(body.DisplayName),
		})
	}, func(out usecase.Output) {
		account := accountResponse(out)
		w.Header().Set("Location", APIBasePath+"/accounts/"+account.Id.String())
		writeJSON(w, r, http.StatusCreated, account)
	})
}

// UpdateAccountPreferences answers PATCH /accounts/{accountId}/preferences.
func (c *RestController) UpdateAccountPreferences(w http.ResponseWriter, r *http.Request, accountID openapi.AccountId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AccountPreferences
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		// The absent/empty distinction survives the mapping: a field the client omitted is absent
		// from the input, and one it sent empty is present and empty. That is the difference
		// between "leave my time zone" and "clear it".
		return c.UseCases.Invoke(r.Context(), updateAccountPreferencesUseCase, actor, usecase.Input{
			"account_id": accountID.String(),
			"locale":     optionalStringField(body.Locale),
			"time_zone":  optionalStringField(body.TimeZone),
			"week_start": optionalWeekStart(body.WeekStart),
		})
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, accountResponse(out))
	})
}

// GrantMembership answers POST /memberships.
func (c *RestController) GrantMembership(w http.ResponseWriter, r *http.Request, _ openapi.GrantMembershipParams) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.MembershipGrant
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), grantMembershipUseCase, actor, usecase.Input{
			"scope_type": string(body.ScopeType),
			"role":       string(body.Role),
			"account_id": optionalUUIDField(body.AccountId),
			"group_id":   optionalUUIDField(body.GroupId),
			"scope_id":   optionalUUIDField(body.ScopeId),
		})
	}, func(out usecase.Output) {
		membership := membershipResponse(out)
		w.Header().Set("Location", APIBasePath+"/memberships/"+membership.Id.String())
		writeJSON(w, r, http.StatusCreated, membership)
	})
}

// RevokeMembership answers DELETE /memberships/{membershipId}.
func (c *RestController) RevokeMembership(w http.ResponseWriter, r *http.Request, membershipID openapi.MembershipId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), revokeMembershipUseCase, actor, usecase.Input{
			"membership_id": membershipID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// CreateGroup answers POST /groups.
func (c *RestController) CreateGroup(w http.ResponseWriter, r *http.Request, _ openapi.CreateGroupParams) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.GroupCreate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}
		return c.UseCases.Invoke(r.Context(), createGroupUseCase, actor, usecase.Input{
			"name":        body.Name,
			"description": optionalStringField(body.Description),
			"members":     optionalUUIDList(body.Members),
		})
	}, func(out usecase.Output) {
		group := groupResponse(out)
		w.Header().Set("Location", APIBasePath+"/groups/"+group.Id.String())
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusCreated, group)
	})
}

// UpdateGroup answers PATCH /groups/{groupId}.
func (c *RestController) UpdateGroup(w http.ResponseWriter, r *http.Request, groupID openapi.GroupId, params openapi.UpdateGroupParams) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.GroupUpdate
		if err := decodeJSON(r, &body); err != nil {
			return nil, err
		}

		input := usecase.Input{
			"group_id":    groupID.String(),
			"name":        optionalStringField(body.Name),
			"description": optionalStringField(body.Description),
		}
		// Absent leaves the membership alone; an empty array empties the group. Setting the key
		// only when the client sent one is what carries that distinction into the catalogue.
		if body.Members != nil {
			input["members"] = optionalUUIDList(body.Members)
		}
		if version, ok := versionFromIfMatch(params.IfMatch); ok {
			input["expected_version"] = version
		}
		return c.UseCases.Invoke(r.Context(), updateGroupUseCase, actor, input)
	}, func(out usecase.Output) {
		w.Header().Set("ETag", etag(out.Int("version")))
		writeJSON(w, r, http.StatusOK, groupResponse(out))
	})
}

// DeleteGroup answers DELETE /groups/{groupId}.
func (c *RestController) DeleteGroup(w http.ResponseWriter, r *http.Request, groupID openapi.GroupId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), deleteGroupUseCase, actor, usecase.Input{
			"group_id": groupID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// versionFromIfMatch reads the version out of an entity tag.
//
// The tag is what the client last read, quoted and strong (RFC 9110 §13.1.1); the version inside
// it is what the optimistic lock compares. An unreadable tag is treated as absent rather than as
// an error: a client that sends `*` means "whatever is there", and one that sends nonsense gets
// the same answer as one that sent nothing - the write proceeds against the current version, and
// it is the concurrent writer, not the malformed header, that a conflict is worth reporting for.
func versionFromIfMatch(header *string) (int, bool) {
	if header == nil {
		return 0, false
	}
	tag := strings.Trim(strings.TrimSpace(*header), `"`)
	version, err := strconv.Atoi(tag)
	if err != nil || version < 1 {
		return 0, false
	}
	return version, true
}

// identity is the shape every handler above shares: the catalogue has to be wired, the actor comes
// from the context, a failure becomes a problem document, and a success is written by the caller.
//
// A helper rather than seven copies, because the copy that gets it wrong is the one that forgets
// to check whether the catalogue is wired and panics on a nil interface.
func (c *RestController) identity(
	w http.ResponseWriter, r *http.Request,
	invoke func(appshared.ActorContext) (usecase.Output, error),
	respond func(usecase.Output),
) {
	requestID := correlation.RequestIDFrom(r.Context())

	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}
	actor, _ := appshared.ActorFrom(r.Context())

	out, err := invoke(actor)
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	respond(out)
}

func accountResponse(out usecase.Output) openapi.Account {
	account := openapi.Account{
		Id:          uuidValue(out.String("id")),
		Kind:        openapi.AccountKind(out.String("kind")),
		DisplayName: out.String("display_name"),
		Status:      openapi.AccountStatus(out.String("status")),
	}
	if email := out.String("email"); email != "" {
		address := openapi_types.Email(email)
		account.Email = &address
	}
	if locale := out.String("locale"); locale != "" {
		account.Locale = &locale
	}
	if zone := out.String("time_zone"); zone != "" {
		account.TimeZone = &zone
	}
	if weekStart := out.String("week_start"); weekStart != "" {
		day := openapi.AccountWeekStart(weekStart)
		account.WeekStart = &day
	}
	return account
}

func membershipResponse(out usecase.Output) openapi.Membership {
	membership := openapi.Membership{
		Id:        uuidValue(out.String("id")),
		ScopeType: openapi.MembershipScope(out.String("scope_type")),
		Role:      openapi.MembershipRole(out.String("role")),
	}
	if accountID := out.String("account_id"); accountID != "" {
		id := uuidValue(accountID)
		membership.AccountId = &id
	}
	if groupID := out.String("group_id"); groupID != "" {
		id := uuidValue(groupID)
		membership.GroupId = &id
	}
	if scopeID := out.String("scope_id"); scopeID != "" {
		id := uuidValue(scopeID)
		membership.ScopeId = &id
	}
	return membership
}

func groupResponse(out usecase.Output) openapi.Group {
	group := openapi.Group{
		Id:      uuidValue(out.String("id")),
		Name:    out.String("name"),
		Version: out.Int("version"),
	}
	if description := out.String("description"); description != "" {
		group.Description = &description
	}
	return group
}

// optionalUUIDList maps a list of identifiers for the catalogue. An absent list is absent; an
// empty one is an empty list, because emptying a group is an instruction rather than a no-op.
func optionalUUIDList(values *[]openapi_types.UUID) any {
	if values == nil {
		return nil
	}
	ids := make([]any, 0, len(*values))
	for _, value := range *values {
		ids = append(ids, value.String())
	}
	return ids
}

func optionalWeekStart(value *openapi.AccountPreferencesWeekStart) any {
	if value == nil {
		return nil
	}
	return string(*value)
}
