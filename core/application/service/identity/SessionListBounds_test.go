// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	cryptoport "github.com/Jersyfi/hubtask/core/port/crypto"
)

// UC-ID-06 check 5: a session that outlived the workspace's session rules - its maximum age, its
// idle time, a required new password - is not listed as open. Before SC-19 the list dropped only
// the revoked and the run-out, so a session the next request refused was still offered to be ended.
//
// One definition of "open": each bound is proved against the same row both ways - the next request
// refuses it, and the list leaves it out.
func TestTheListShowsOnlyWhatTheNextRequestAccepts(t *testing.T) {
	cases := []struct {
		name         string
		outlived     func(domain.Session) domain.Session
		rotationFrom time.Time
		code         string
	}{
		{
			name: "past its maximum age",
			outlived: func(s domain.Session) domain.Session {
				s.HardExpiresAt = now.Add(-time.Minute)
				return s
			},
			code: "auth.session_too_old",
		},
		{
			name: "past its idle time",
			outlived: func(s domain.Session) domain.Session {
				s.IdleMinutes = 30
				s.LastSeenAt = now.Add(-31 * time.Minute)
				return s
			},
			code: "auth.session_idle",
		},
		{
			name:         "opened before a required new password",
			outlived:     func(s domain.Session) domain.Session { return s },
			rotationFrom: now.Add(-time.Minute),
			code:         "auth.session_rotated",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			credential, _ := refreshCredential(now)
			past := tc.outlived(credential.Session)

			// A second session of the same account, opened after the cutoff and used a moment ago:
			// the one every bound leaves alone, so the list is not simply empty.
			open := credential.Session
			open.ID = refreshRowID
			open.CreatedAt = now.Add(-time.Second)
			open.LastSeenAt = now.Add(-time.Second)

			fixture := newSessionFixture(now)
			fixture.sessions.sessions[sessionRowID] = repository.SessionCredential{
				TenantStatus: domain.TenantActive, Session: past, Account: credential.Account,
				RotationFrom: tc.rotationFrom,
			}
			fixture.sessions.listed = []domain.Session{open, past}
			fixture.sessions.rotationFrom = tc.rotationFrom

			signer := validatingSigner{token: "hbt_sat_x", claims: cryptoport.SessionClaims{
				TenantID: tenant, SessionID: sessionRowID, AccountID: account,
				ExpiresAt: now.Add(10 * time.Minute),
			}}
			_, err := sessionAuthenticator(fixture.sessions, signer, now).Execute(t.Context(),
				AuthenticateTokenCommand{Credential: "hbt_sat_x"})
			if err == nil || !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("the next request answered %v, want %s", err, tc.code)
			}

			listed, _, err := ListSessions{Writer: fixture.writer}.
				Execute(t.Context(), sessionActor(accountsRead))
			if err != nil {
				t.Fatalf("listing: %v", err)
			}
			if len(listed) != 1 || listed[0].ID != open.ID {
				t.Errorf("the list holds %v, want only the open session %v", sessionIDs(listed), open.ID)
			}
		})
	}
}

func sessionIDs(sessions []domain.Session) []string {
	out := make([]string, 0, len(sessions))
	for _, session := range sessions {
		out = append(out, session.ID.String())
	}
	return out
}
