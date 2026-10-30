// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The catalogue names. The routes they are reached through come from the specification; the two
// are reconciled by the parity test rather than by these constants.
const (
	inviteAccountUseCase               = "InviteAccount"
	renewInvitationUseCase             = "RenewInvitation"
	getOwnAccountUseCase               = "GetOwnAccount"
	getAccountUseCase                  = "GetAccount"
	updateAccountPreferencesUseCase    = "UpdateAccountPreferences"
	updateOwnAccountUseCase            = "UpdateOwnAccount"
	setOwnAvatarUseCase                = "SetOwnAvatar"
	removeOwnAvatarUseCase             = "RemoveOwnAvatar"
	listNotificationPreferencesUseCase = "ListNotificationPreferences"
	setNotificationPreferenceUseCase   = "SetNotificationPreference"
	listMembershipsUseCase             = "ListMemberships"
	listGroupsUseCase                  = "ListGroups"
	getGroupUseCase                    = "GetGroup"
	updateGroupUseCase                 = "UpdateGroup"
	deleteGroupUseCase                 = "DeleteGroup"
)

// invitedAccount is the InvitedAccount schema written as the account plus the link, so that the
// account's own fields are mapped in one place (accountResponse) rather than twice.
type invitedAccount struct {
	openapi.Account
	InvitationLink *string `json:"invitation_link,omitempty"`
}

// invitedAccountResponse maps an invitation's answer, with the link where it carries one.
func invitedAccountResponse(out usecase.Output) invitedAccount {
	invited := invitedAccount{Account: accountResponse(out)}
	if link := out.String("invitation_link"); link != "" {
		invited.InvitationLink = &link
	}
	return invited
}

// managedAccountCreatedResponse maps a managed account with its membership and the one answer its start password ever appears in (T-18).
func managedAccountCreatedResponse(out usecase.Output) openapi.ManagedAccountCreated {
	account, _ := out["account"].(usecase.Output)
	membership, _ := out["membership"].(usecase.Output)
	return openapi.ManagedAccountCreated{
		Account:       accountResponse(account),
		Membership:    membershipResponse(membership),
		StartPassword: out.String("start_password"),
	}
}

// UpdateAccountPreferences answers PATCH /accounts/{accountId}/preferences.
func (c *RestController) UpdateAccountPreferences(w http.ResponseWriter, r *http.Request, accountID openapi.AccountId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		var body openapi.AccountPreferences
		present, err := decodeJSONWithPresence(r, &body)
		if err != nil {
			return nil, err
		}
		// The absent/empty distinction survives the mapping: a field the client omitted is absent
		// from the input, and one it sent empty is present and empty. That is the difference
		// between "leave my time zone" and "clear it". A null is the empty one: the contract
		// declares it for all three, and the generated pointer cannot tell it from an omission,
		// so presence is read from the bytes and a null is written as "".
		in := usecase.Input{"account_id": accountID.String()}
		if present["locale"] {
			in["locale"] = stringOrEmpty(body.Locale)
		}
		if present["time_zone"] {
			in["time_zone"] = stringOrEmpty(body.TimeZone)
		}
		if present["week_start"] {
			in["week_start"] = weekStartOrEmpty(body.WeekStart)
		}
		// The moments travel as the words the use case reads - "true"/"false", an RFC
		// 3339 instant - and a null as the empty one, like the three above.
		if present["celebrations"] {
			in["celebrations"] = boolWordOrEmpty(body.Celebrations)
		}
		if present["onboarding_completed_at"] {
			in["onboarding_completed_at"] = instantOrEmpty(body.OnboardingCompletedAt)
		}
		return c.UseCases.Invoke(r.Context(), updateAccountPreferencesUseCase, actor, in)
	}, func(out usecase.Output) {
		writeJSON(w, r, http.StatusOK, accountResponse(out))
	})
}

// notificationPreferenceResponse maps one row of the settings form. The timestamp is a pointer
// with no omitempty, so a default carries an explicit null rather than no field at all.
func notificationPreferenceResponse(out usecase.Output) openapi.NotificationPreference {
	return openapi.NotificationPreference{
		Category:     out.String("category"),
		Channel:      out.String("channel"),
		Enabled:      boolOf(out["enabled"]),
		IncludeTitle: boolOf(out["include_title"]),
		IsDefault:    boolOf(out["is_default"]),
		UpdatedAt:    timePointer(out["updated_at"]),
	}
}

// ListMemberships answers GET /memberships.
//
// Through the shared read helper rather than the identity one: a list is mapped row by row from
// the catalogue's page shape, which is what `rowsOf` and `pageResponse` exist for.
func (c *RestController) ListMemberships(w http.ResponseWriter, r *http.Request, params openapi.ListMembershipsParams) {
	out, ok := c.read(w, r, listMembershipsUseCase, usecase.Input{
		"scope_type": string(params.ScopeType),
		"scope_id":   optionalUUIDField(params.ScopeId),
		"cursor":     optionalStringField(params.Cursor),
		"size":       optionalIntField(params.Size),
	})
	if !ok {
		return
	}

	page := openapi.MembershipPage{
		Data: []openapi.Membership{}, Page: pageResponse(out), GrantableRoles: []openapi.MembershipRole{},
	}
	for _, role := range stringsOf(out["grantable_roles"]) {
		page.GrantableRoles = append(page.GrantableRoles, openapi.MembershipRole(role))
	}
	for _, row := range rowsOf(out) {
		page.Data = append(page.Data, membershipResponse(row))
	}
	writeJSON(w, r, http.StatusOK, page)
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

// accountSummaryResponse maps the narrower projection. Built from its own four fields rather than
// from `accountResponse` with members cleared: a mapping that starts from everything and removes
// what must not travel leaks the next field somebody adds to the wider one.
func accountSummaryResponse(out usecase.Output) openapi.AccountSummary {
	return openapi.AccountSummary{
		Id:            uuidValue(out.String("id")),
		Kind:          openapi.AccountSummaryKind(out.String("kind")),
		DisplayName:   out.String("display_name"),
		Status:        openapi.AccountSummaryStatus(out.String("status")),
		HasAddress:    boolPointer(out["has_address"]),
		Managed:       boolPointer(out["managed"]),
		AvatarMediaId: optionalUUID(out.String("avatar_media_id")),
	}
}

// optionalUUID answers a projected identifier, or nil where the projection carried none.
func optionalUUID(value string) *openapi_types.UUID {
	if value == "" {
		return nil
	}
	id := uuidValue(value)
	return &id
}

// boolPointer answers a projected flag, or nil where the projection did not carry it.
func boolPointer(value any) *bool {
	flag, held := value.(bool)
	if !held {
		return nil
	}
	return &flag
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
	if name := out.String("sign_in_name"); name != "" {
		account.SignInName = &name
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
	if celebrations, ok := out["celebrations"].(bool); ok {
		account.Celebrations = &celebrations
	}
	account.OnboardingCompletedAt = timePointer(out["onboarding_completed_at"])
	account.AvatarMediaId = optionalUUID(out.String("avatar_media_id"))
	if armed, held := out["has_second_factor"].(bool); held {
		// Answered on the caller's own account and on nothing else, which is why it is conditional
		// here rather than a plain field: the same projection renders a colleague's account.
		account.HasSecondFactor = &armed
	}
	if app, held := out["has_authenticator_app"].(bool); held {
		// The caller's own account only, for has_second_factor's reason.
		account.HasAuthenticatorApp = &app
	}
	if removable, held := out["authenticator_app_removable"].(bool); held {
		// The caller's own account only, for has_second_factor's reason.
		account.AuthenticatorAppRemovable = &removable
	}
	if required, held := out["second_factor_required"].(bool); held {
		// The caller's own account only, for has_second_factor's reason.
		account.SecondFactorRequired = &required
	}
	if password, held := out["has_password"].(bool); held {
		// The caller's own account only, for has_second_factor's reason.
		account.HasPassword = &password
	}
	if access, held := out["has_access"].(bool); held {
		// The caller's own account only, for has_second_factor's reason.
		account.HasAccess = &access
	}
	if remaining, held := out["recovery_codes_remaining"].(int); held {
		// Present exactly where a factor is armed, and then zero is answered as zero, because there
		// zero is the number to act on. Absent is not zero: an account holding no factor
		// has no codes to count, and a screen told zero offers ten it cannot make.
		account.RecoveryCodesRemaining = &remaining
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

func weekStartOrEmpty(value *openapi.AccountPreferencesWeekStart) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func boolWordOrEmpty(value *bool) string {
	if value == nil {
		return ""
	}
	return strconv.FormatBool(*value)
}

func instantOrEmpty(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

// groupDetailResponse is a group with its members, and whether this reader may change them.
func groupDetailResponse(out usecase.Output) openapi.GroupDetail {
	group := groupResponse(out)
	detail := openapi.GroupDetail{
		Id: group.Id, Name: group.Name, Description: group.Description, Version: group.Version,
		Members: []openapi_types.UUID{},
	}
	detail.MembersChangeable, _ = out["members_changeable"].(bool)
	for _, member := range stringsOf(out["members"]) {
		detail.Members = append(detail.Members, uuidValue(member))
	}
	return detail
}

// notificationPreferenceListResponse is every preference of an account, unpaged.
func notificationPreferenceListResponse(out usecase.Output) openapi.NotificationPreferenceList {
	list := openapi.NotificationPreferenceList{Data: []openapi.NotificationPreference{}}
	for _, row := range rowsOf(out) {
		list.Data = append(list.Data, notificationPreferenceResponse(row))
	}
	return list
}

// addressRequestedResponse says until when the confirmation link holds - and nothing about the
// address.
func addressRequestedResponse(out usecase.Output) openapi.AddressRequested {
	expires, _ := out["expires_at"].(time.Time)
	return openapi.AddressRequested{ExpiresAt: expires}
}

// startPasswordIssuedResponse maps a start password issued: the account and the password, once.
func startPasswordIssuedResponse(out usecase.Output) openapi.StartPasswordIssued {
	account, _ := out["account"].(usecase.Output)
	return openapi.StartPasswordIssued{Account: accountResponse(account), StartPassword: out.String("start_password")}
}
