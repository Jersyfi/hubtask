// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package rest

import (
	"net"
	"net/http"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/presentation/openapi"
)

// The use case names of the session surface (H-01). Constants for the reason the token ones are:
// a name spelled twice is a name that eventually drifts.
const (
	signInUseCase            = "SignIn"
	refreshSessionUseCase    = "RefreshSession"
	listSessionsUseCase      = "ListSessions"
	revokeSessionUseCase     = "RevokeSession"
	revokeAllSessionsUseCase = "RevokeAllSessions"
	redeemInvitationUseCase  = "RedeemInvitation"
)

// SignIn answers POST /auth/sessions.
//
// Written out rather than through the identity helper: the route is public, so there is no actor
// to resolve - the whole call exists to produce one.
func (c *RestController) SignIn(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SignIn
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), signInUseCase, actorOf(r), usecase.Input{
		"email":    string(body.Email),
		"password": body.Password,
		// The client hints and the tenant sources are the request's, and only this adapter has
		// the request - which is why they travel as declared inputs rather than being re-derived
		// somewhere that never saw the connection.
		"user_agent":    r.UserAgent(),
		"remote_addr":   r.RemoteAddr,
		"tenant_slug":   c.tenantSlug(r),
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// RefreshSession answers POST /auth/sessions:refresh.
func (c *RestController) RefreshSession(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.SessionRefresh
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), refreshSessionUseCase, actorOf(r), usecase.Input{
		"refresh_token": body.RefreshToken,
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusOK, sessionTokensResponse(out))
}

// ListSessions answers GET /auth/sessions.
//
// Written out rather than through the identity helper, for the reason ListServiceAccounts is:
// the operation has no parameters, and the helper's closure gives the linter nothing to trace
// the request's context through.
func (c *RestController) ListSessions(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), listSessionsUseCase, actorOf(r), usecase.Input{})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	rows, _ := out["data"].([]usecase.Output)
	sessions := make([]openapi.Session, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, sessionResponse(row))
	}
	writeJSON(w, r, http.StatusOK, sessions)
}

// RevokeSession answers DELETE /auth/sessions/{sessionId}.
func (c *RestController) RevokeSession(w http.ResponseWriter, r *http.Request, sessionID openapi.SessionId) {
	c.identity(w, r, func(actor appshared.ActorContext) (usecase.Output, error) {
		return c.UseCases.Invoke(r.Context(), revokeSessionUseCase, actor, usecase.Input{
			"session_id": sessionID.String(),
		})
	}, func(usecase.Output) {
		w.WriteHeader(http.StatusNoContent)
	})
}

// RevokeAllSessions answers DELETE /auth/sessions.
func (c *RestController) RevokeAllSessions(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	if _, err := c.UseCases.Invoke(
		r.Context(), revokeAllSessionsUseCase, actorOf(r), usecase.Input{},
	); err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// RedeemInvitation answers POST /auth/invitations:redeem.
//
// Written out for SignIn's reason: the route is public, and the token in the body is the whole
// of what authenticates the call.
func (c *RestController) RedeemInvitation(w http.ResponseWriter, r *http.Request) {
	requestID := correlation.RequestIDFrom(r.Context())
	if c.UseCases == nil {
		WriteProblem(w, errNotWired, requestID)
		return
	}

	var body openapi.InvitationRedemption
	if err := decodeJSON(r, &body); err != nil {
		WriteProblem(w, err, requestID)
		return
	}

	out, err := c.UseCases.Invoke(r.Context(), redeemInvitationUseCase, actorOf(r), usecase.Input{
		"token":         body.Token,
		"password":      body.Password,
		"user_agent":    r.UserAgent(),
		"remote_addr":   r.RemoteAddr,
		"tenant_header": r.Header.Get(TenantHeader),
	})
	if err != nil {
		WriteProblem(w, err, requestID)
		return
	}
	writeJSON(w, r, http.StatusCreated, sessionTokensResponse(out))
}

// tenantSlug reads the subdomain off the request's host, when this installation knows its own.
// One label and no more: a nested subdomain names nothing here.
func (c *RestController) tenantSlug(r *http.Request) string {
	if c.BaseHost == "" {
		return ""
	}
	host := r.Host
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	}
	host = strings.ToLower(host)
	label, found := strings.CutSuffix(host, "."+c.BaseHost)
	if !found || label == "" || strings.Contains(label, ".") {
		return ""
	}
	return label
}

// sessionResponse maps one session projection onto the contract's shape.
func sessionResponse(row usecase.Output) openapi.Session {
	current, _ := row["current"].(bool)
	return openapi.Session{
		Id:         uuidValue(row.String("id")),
		CreatedAt:  timeValue(row["created_at"]),
		LastUsedAt: optionalTimeField(row["last_used_at"]),
		UserAgent:  optionalTextField(row["user_agent"]),
		IpClass:    optionalTextField(row["ip_class"]),
		Current:    current,
	}
}

func sessionTokensResponse(out usecase.Output) openapi.SessionTokens {
	session, _ := out["session"].(usecase.Output)
	return openapi.SessionTokens{
		TokenType:             openapi.Bearer,
		AccessToken:           out.String("access_token"),
		AccessTokenExpiresAt:  timeValue(out["access_token_expires_at"]),
		RefreshToken:          out.String("refresh_token"),
		RefreshTokenExpiresAt: timeValue(out["refresh_token_expires_at"]),
		Session:               sessionResponse(session),
	}
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
