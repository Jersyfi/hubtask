// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net/http"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the session surface. Constants for the reason the token ones are:
// a name spelled twice is a name that eventually drifts.
const (
	refreshSessionUseCase = "RefreshSession"
	// revokeOtherSessionsUseCase is *Sign out everywhere else* (UC-ID-06 check 4).
	revokeOtherSessionsUseCase = "RevokeOtherSessions"
	redeemInvitationUseCase    = "RedeemInvitation"
	setUpInstallationUseCase   = "SetUpInstallation"
)

// SetUpInstallation answers POST /auth/installation:set-up.
//
// Written out for SignIn's reason: the route is public, and the setup code in the body is the whole
// of what authenticates the call.
func (c *RestController) SetUpInstallation(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.InstallationSetUp
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), setUpInstallationUseCase, actorOf(r), usecase.Input{
		"code":               body.Code,
		"display_name":       body.DisplayName,
		"slug":               stringOrEmpty(body.Slug),
		"owner_email":        body.OwnerEmail,
		"owner_display_name": stringOrEmpty(body.OwnerDisplayName),
		"password":           body.Password,
		"default_locale":     stringOrEmpty(body.DefaultLocale),
		"default_time_zone":  stringOrEmpty(body.DefaultTimeZone),
		"user_agent":         r.UserAgent(),
		"remote_addr":        r.RemoteAddr,
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// sessionResponse maps one session projection onto the contract's shape.
func sessionResponse(row usecase.Output) openapi.Session {
	current, _ := row["current"].(bool)
	answer := openapi.Session{
		Id:         uuidValue(row.String("id")),
		CreatedAt:  timeValue(row["created_at"]),
		LastUsedAt: optionalTimeField(row["last_used_at"]),
		UserAgent:  optionalTextField(row["user_agent"]),
		IpClass:    optionalTextField(row["ip_class"]),
		Current:    current,
	}
	if method := optionalTextField(row["signed_in_with"]); method != nil {
		// Absent for a session opened before this was recorded, which is not "unknown method": a
		// client leaves the line out rather than printing a word nobody can act on.
		opened := openapi.SessionSignedInWith(*method)
		answer.SignedInWith = &opened
	}
	answer.SignedInProvider = optionalTextField(row["signed_in_provider"])
	return answer
}

func sessionTokensResponse(out usecase.Output) openapi.SessionTokens {
	session, _ := out["session"].(usecase.Output)
	tokens := openapi.SessionTokens{
		TokenType:             openapi.Bearer,
		AccessToken:           out.String("access_token"),
		AccessTokenExpiresAt:  timeValue(out["access_token_expires_at"]),
		RefreshToken:          out.String("refresh_token"),
		RefreshTokenExpiresAt: timeValue(out["refresh_token_expires_at"]),
		Session:               sessionResponse(session),
	}
	if remaining, ok := out["recovery_codes_remaining"].(int); ok {
		tokens.RecoveryCodesRemaining = &remaining
	}
	return tokens
}

// optionalTextField is optionalTimeField's shape for strings: a projection's nil or empty
// becomes an absent member rather than an empty one.
func optionalTextField(value any) *string {
	text, ok := value.(string)
	if !ok || text == "" {
		return nil
	}
	return &text
}
