// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package security_test

import (
	"errors"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/shared/secret"
	"github.com/Jersyfi/hubtask/infrastructure/security"
)

// The MCP session identifier (J-13). What is asked here is the property that makes it work on a
// horizontally scaled role at all - any pod can check it - and the four ways it must not work.

var (
	sessionTenant  = shared.MustParseID("0192f000-0000-7000-8000-00000000000a")
	otherTenant    = shared.MustParseID("0192f000-0000-7000-8000-00000000000b")
	sessionAccount = shared.MustParseID("0192f000-0000-7000-8000-00000000000d")
	otherAccount   = shared.MustParseID("0192f000-0000-7000-8000-00000000000e")
	sessionNow     = time.Date(2026, 9, 9, 10, 0, 0, 0, time.UTC)
)

func issuer() security.McpSessionIssuer {
	return security.NewMcpSessionIssuer(secret.New("an installation secret for the tests"))
}

// The whole point: a session minted by one process is accepted by another, because nothing about it
// lives in a process. A map would bind a client to whichever pod answered its handshake.
func TestASessionMintedByOneProcessIsAcceptedByAnother(t *testing.T) {
	session := issuer().Issue(sessionTenant, sessionAccount, sessionNow)

	// A second issuer, as a second pod builds one: same installation secret, no shared state.
	other := security.NewMcpSessionIssuer(secret.New("an installation secret for the tests"))
	if err := other.Validate(session, sessionTenant, sessionAccount, sessionNow.Add(time.Hour)); err != nil {
		t.Errorf("another process refused a session this one minted: %v", err)
	}
}

// The four refusals, and all four are one answer. Saying which check failed tells whoever is
// probing how far their forgery got.
func TestEveryWayASessionIsNotOneIsTheSameAnswer(t *testing.T) {
	minted := issuer().Issue(sessionTenant, sessionAccount, sessionNow)

	for name, c := range map[string]struct {
		session         string
		tenant, account shared.ID
		at              time.Time
	}{
		"a forgery": {
			session: "not-a-session", tenant: sessionTenant, account: sessionAccount, at: sessionNow,
		},
		"a truncation": {
			session: minted[:len(minted)-4], tenant: sessionTenant, account: sessionAccount, at: sessionNow,
		},
		"one that has expired": {
			session: minted, tenant: sessionTenant, account: sessionAccount,
			at: sessionNow.Add(security.McpSessionLifetime + time.Second),
		},
		"another account's": {
			session: minted, tenant: sessionTenant, account: otherAccount, at: sessionNow,
		},
		"another workspace's": {
			session: minted, tenant: otherTenant, account: sessionAccount, at: sessionNow,
		},
		"one signed by another installation": {
			session: security.NewMcpSessionIssuer(secret.New("somebody else's secret")).
				Issue(sessionTenant, sessionAccount, sessionNow),
			tenant: sessionTenant, account: sessionAccount, at: sessionNow,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := issuer().Validate(c.session, c.tenant, c.account, c.at)
			if !errors.Is(err, security.ErrMcpSessionUnknown) {
				t.Errorf("answered %v, want the one indistinguishable refusal", err)
			}
		})
	}
}

// The identity is signed rather than merely carried, which is what stops a client editing the
// workspace out of its own session identifier.
func TestTheIdentityInASessionCannotBeSwapped(t *testing.T) {
	minted := issuer().Issue(sessionTenant, sessionAccount, sessionNow)

	// The identity is in the clear at the end of the token - that is what lets any pod read it -
	// so an attacker's first move is to rewrite it. The tag is over the same bytes.
	raw := []byte(minted)
	raw[len(raw)-1] ^= 0x01
	forged := string(raw)

	if err := issuer().Validate(forged, sessionTenant, sessionAccount, sessionNow); !errors.Is(
		err, security.ErrMcpSessionUnknown) {
		t.Errorf("a rewritten session was accepted: %v", err)
	}
}

// A session is derived under its own label, so one minted here can never be presented as a media
// token or a page cursor - the domain separation every derived value in this package has.
func TestASessionIsNotAnythingElseDerivedFromTheSameSecret(t *testing.T) {
	installation := secret.New("an installation secret for the tests")
	session := security.NewMcpSessionIssuer(installation).
		Issue(sessionTenant, sessionAccount, sessionNow)

	media := security.NewMediaTokenIssuer(installation)
	if _, err := media.ValidateDownload(session, sessionAccount, sessionNow); err == nil {
		t.Error("a session identifier opened a media download")
	}
}

// It stays usable for as long as it says it does, which is what stops an agent re-handshaking in
// the middle of a task.
func TestASessionLastsItsStatedLifetime(t *testing.T) {
	session := issuer().Issue(sessionTenant, sessionAccount, sessionNow)

	if err := issuer().Validate(session, sessionTenant, sessionAccount,
		sessionNow.Add(security.McpSessionLifetime-time.Minute)); err != nil {
		t.Errorf("a session expired before its lifetime: %v", err)
	}
}
