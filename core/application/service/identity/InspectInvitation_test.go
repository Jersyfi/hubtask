// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-ID-07 check 1: the invitation card names the workspace and the invited address before anything
// is typed - answered only for a token that would redeem, with the redemption's one refusal for
// every other, and nothing spent.

func inspectThroughTheRegistry(t *testing.T, writer SessionWriter, token string) (usecase.Output, error) {
	t.Helper()
	registry, err := usecase.NewRegistry(nil, InspectInvitation{Writer: writer}.Descriptor())
	if err != nil {
		t.Fatalf("building the registry: %v", err)
	}
	return registry.Invoke(t.Context(), InspectInvitationName, appshared.Anonymous("en", "UTC"),
		usecase.Input{"token": token})
}

func TestAnInvitationNamesItsWorkspaceAndAddress(t *testing.T) {
	fixture := newSessionFixture(now)
	token := redemptionToken(t)
	waiting := waitingRedemption()
	waiting.TenantName = "Family Winkel"
	fixture.accounts.redemption[token.Secret()] = waiting

	out, err := inspectThroughTheRegistry(t, fixture.writer, token.Secret())
	if err != nil {
		t.Fatalf("inspecting: %v", err)
	}
	if out["workspace_name"] != "Family Winkel" || out["email"] != "bert@example.org" {
		t.Errorf("the invitation is named as %v", out)
	}
	if fixture.accounts.redeemedHash != "" || len(fixture.sessions.inserted) != 0 {
		t.Error("inspecting redeemed the invitation or opened a session")
	}
	// Still redeemable after it was named.
	if _, err := inspectThroughTheRegistry(t, fixture.writer, token.Secret()); err != nil {
		t.Errorf("a second look was refused: %v", err)
	}
}

func TestAnInvitationThatWouldNotRedeemIsTheOneRefusal(t *testing.T) {
	cases := map[string]func(*sessionFixture, domain.Token){
		"unknown": func(*sessionFixture, domain.Token) {},
		"expired": func(f *sessionFixture, token domain.Token) {
			waiting := waitingRedemption()
			waiting.ExpiresAt = now.Add(-time.Minute)
			f.accounts.redemption[token.Secret()] = waiting
		},
		"accepted": func(f *sessionFixture, token domain.Token) {
			accepted := waitingRedemption()
			accepted.Account.Status = domain.AccountActive
			f.accounts.redemption[token.Secret()] = accepted
		},
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newSessionFixture(now)
			token := redemptionToken(t)
			arrange(fixture, token)
			_, err := inspectThroughTheRegistry(t, fixture.writer, token.Secret())
			var problem *shared.Error
			if !errors.As(err, &problem) || problem.DetailCode != "auth.redemption_failed" {
				t.Fatalf("err = %v, want the redemption's one refusal", err)
			}
		})
	}
	fixture := newSessionFixture(now)
	if _, err := inspectThroughTheRegistry(t, fixture.writer, "not-a-token"); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("a malformed token answered %v", err)
	}
}
