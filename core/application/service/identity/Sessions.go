// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ListSessionsName      = "ListSessions"
	RevokeSessionName     = "RevokeSession"
	RevokeAllSessionsName = "RevokeAllSessions"
	// RevokeOtherSessionsName is *Sign out everywhere else* (UC-ID-06 check 4).
	RevokeOtherSessionsName = "RevokeOtherSessions"
)

// The audit codes of session management. Ending a way into the workspace is the class of event a
// review looks for, exactly as opening one is.
const (
	SessionRevokedAction  audit.Action = "auth.session_revoked"
	SessionsRevokedAction audit.Action = "auth.sessions_revoked"
	SessionReadAction     audit.Action = "auth.session_read"
)

// ListSessions answers the caller's own open sessions - and never anybody else's, whatever the
// role: a session is the person's, and an administrator who suspects one acts by disabling the
// account.
type ListSessions struct{ Writer SessionWriter }

// Execute reads them, newest first, and marks the one answering this very call.
//
// Open is what the next request accepts: a session past the workspace's maximum age, its idle time
// or a required new password is refused there, and so it is not listed here (UC-ID-06 check 5). The
// row stays until the sweep takes it - no job walks sessions, because nothing may enumerate tenants.
func (h ListSessions) Execute(
	ctx context.Context, actor appshared.ActorContext,
) ([]domain.Session, shared.ID, error) {
	if err := actor.RequireScope(accountsRead); err != nil {
		return nil, "", err
	}
	if actor.AccountID.IsZero() {
		return nil, "", shared.ErrForbidden.WithDetail("access.token_owner_required")
	}

	w := h.Writer
	var sessions []domain.Session
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		found, err := w.Sessions.ForAccount(ctx, actor.AccountID, now)
		if err != nil {
			return err
		}
		for _, session := range found.Sessions {
			if session.VerifyOpen(now, found.RotationFrom) == nil {
				sessions = append(sessions, session)
			}
		}
		return nil
	})
	if err != nil {
		return nil, "", err
	}
	// TokenID is the session for a session-authenticated actor and a token row otherwise; a
	// token row's identifier matches no session, so `current` is simply false for a PAT caller.
	return sessions, actor.TokenID, nil
}

// RevokeSession ends one of the caller's own sessions.
type RevokeSession struct{ Writer SessionWriter }

// Execute revokes. Somebody else's session is not found rather than forbidden - whether a
// session identifier exists is nobody's business but its holder's - and revoking twice is
// somebody making sure, not an error.
func (h RevokeSession) Execute(
	ctx context.Context, actor appshared.ActorContext, sessionID shared.ID,
) error {
	if err := actor.RequireScope(accountRead); err != nil {
		return err
	}
	if sessionID.IsZero() {
		return shared.ErrValidation.
			WithDetail("auth.session_not_found").
			WithFields(shared.FieldError{Path: "/session_id", Code: "auth.session_not_found"})
	}

	w := h.Writer
	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		changed, err := w.Sessions.Revoke(ctx, sessionID, actor.AccountID, now)
		if err != nil {
			return err
		}
		if changed {
			return w.recordRevocation(ctx, actor, SessionRevokedAction, sessionID, 1, now)
		}

		// Nothing changed: the session is already over, or it is not the caller's. The first is
		// idempotent success, the second is the indistinguishable not-found - and the row says
		// which, without saying it to the caller any louder than that.
		credential, err := w.Sessions.FindForAuth(ctx, sessionID)
		if err != nil || credential.Session.AccountID != actor.AccountID {
			return shared.ErrNotFound.WithDetail("auth.session_not_found")
		}
		return nil
	})
}

// RevokeAllSessions signs the caller out everywhere: every refresh family dies, and every
// access token still in flight refuses on its next request.
type RevokeAllSessions struct{ Writer SessionWriter }

// Execute ends them all, the current one included - the answer to "I left myself signed in
// somewhere" must not spare the device it was asked from, because that device may be the one.
func (h RevokeAllSessions) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := actor.RequireScope(accountRead); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("access.token_owner_required")
	}

	w := h.Writer
	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		ended, err := w.Sessions.RevokeAll(ctx, actor.AccountID, now)
		if err != nil {
			return err
		}
		if ended == 0 {
			// Nothing was live - a PAT caller tidying up, or a repeat. No entry: nothing
			// happened, and an entry saying it did would be a false one.
			return nil
		}
		return w.recordRevocation(ctx, actor, SessionsRevokedAction, actor.AccountID, ended, now)
	})
}

// RevokeOtherSessions signs the caller out everywhere else (UC-ID-06 check 4): every session of the
// account ends but the one making the call - the answer to a lost phone, asked from the laptop still
// in hand. RevokeAllSessions stays beside it for the case where this device may be the one.
type RevokeOtherSessions struct{ Writer SessionWriter }

// Execute ends the others. The session to spare is the caller's own, and only when TokenID names
// one of this account's sessions: a personal access token has no session, so nothing is spared.
func (h RevokeOtherSessions) Execute(ctx context.Context, actor appshared.ActorContext) error {
	if err := actor.RequireScope(accountRead); err != nil {
		return err
	}
	if actor.AccountID.IsZero() {
		return shared.ErrForbidden.WithDetail("access.token_owner_required")
	}

	w := h.Writer
	return w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()
		var keep shared.ID
		if !actor.TokenID.IsZero() {
			credential, err := w.Sessions.FindForAuth(ctx, actor.TokenID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			if err == nil && credential.Session.AccountID == actor.AccountID {
				keep = credential.Session.ID
			}
		}
		ended, err := w.Sessions.RevokeOthers(ctx, actor.AccountID, keep, now)
		if err != nil {
			return err
		}
		if ended == 0 {
			// Nothing else was live. No entry, as for every revocation that changed nothing.
			return nil
		}
		return w.recordRevocation(ctx, actor, SessionsRevokedAction, actor.AccountID, ended, now)
	})
}

// Descriptor is the catalogue entry.
func (h RevokeOtherSessions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeOtherSessionsName,
		Summary: "Ends every session of the caller's account except the one making the call: every " +
			"other refresh family dies, and every other access token still in flight refuses on " +
			"its next request. The answer to a lost device, asked from the one still in hand. A " +
			"caller with a personal access token has no session to keep, and every session ends.",
		SideEffects: "Stamps every other live session and writes one audit entry saying how many.",
		TokenScope:  accountRead,
		Destructive: true,
		Audit: usecase.AuditDeclaration{
			Action: SessionsRevokedAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeOtherSessions) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// recordRevocation writes the evidence for one ending or all of them.
func (w SessionWriter) recordRevocation(
	ctx context.Context, actor appshared.ActorContext,
	action audit.Action, target shared.ID, count int, at time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: at,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: sessionTarget,
		TargetID:   target,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(audit.Change{
			Field: "sessions_ended", Classification: audit.Open, To: strconv.Itoa(count),
		}),
	})
}

// Descriptor is the catalogue entry.
func (h ListSessions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListSessionsName,
		Summary: "The caller's own open sessions, newest first - those the next request accepts: where each was opened - the " +
			"user agent and the coarsened network recorded at sign-in - when it was created, " +
			"when it last acted, and which one is answering this very call. Never anybody " +
			"else's, whatever the role.",
		SideEffects: "None. Reads only.",
		TokenScope:  accountsRead,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: SessionReadAction, TargetType: sessionTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListSessions) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	sessions, currentID, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(sessions))
	for _, session := range sessions {
		rows = append(rows, sessionOutput(session, currentID))
	}
	return usecase.Output{"data": rows}, nil
}

// Descriptor is the catalogue entry.
func (h RevokeSession) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeSessionName,
		Summary: "Ends one of the caller's own sessions: its refresh family dies and its access " +
			"token refuses on its next request, immediately rather than at the end of the " +
			"token's fifteen minutes. Somebody else's session is not found rather than " +
			"forbidden; revoking twice is not an error.",
		SideEffects: "Stamps the session and writes an audit entry. A repeat writes neither.",
		TokenScope:  accountRead,
		Destructive: true,
		Input: []usecase.Field{
			{
				Name: "session_id", Kind: usecase.KindID, Required: true,
				Description: "The session to end, from the caller's own list.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: SessionRevokedAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeSession) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	sessionID, err := in.ID("session_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, sessionID); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// Descriptor is the catalogue entry.
func (h RevokeAllSessions) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RevokeAllSessionsName,
		Summary: "Ends every session of the caller's account, the current one included: every " +
			"refresh family dies, and every access token still in flight refuses on its next " +
			"request. The answer to \"I left myself signed in somewhere\".",
		SideEffects: "Stamps every live session and writes one audit entry saying how many.",
		TokenScope:  accountRead,
		Destructive: true,
		Audit: usecase.AuditDeclaration{
			Action: SessionsRevokedAction, TargetType: sessionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A sign-in is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RevokeAllSessions) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}
