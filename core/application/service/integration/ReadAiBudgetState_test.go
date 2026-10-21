// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"errors"
	"testing"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type choiceOf struct{ source integration.AiSourceKind }

func (c choiceOf) Find(context.Context) (integration.AiChoice, error) {
	return integration.AiChoice{Source: c.source}, nil
}

func (choiceOf) Save(context.Context, integration.AiChoice) (integration.AiChoice, error) {
	return integration.AiChoice{}, errors.New("not in this test")
}

type standingOf struct {
	spent, limit int64
	bounded      bool
	askedOwn     *bool
}

func (s standingOf) AiStanding(_ context.Context, own bool, _ time.Time) (int64, int64, bool, error) {
	if s.askedOwn != nil {
		*s.askedOwn = own
	}
	return s.spent, s.limit, s.bounded, nil
}

func (standingOf) AiRenewal(_ context.Context, now time.Time) (time.Time, error) {
	return time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.UTC), nil
}

type clockAt struct{ at time.Time }

func (c clockAt) Now() time.Time { return c.at }

func budgetReader(source integration.AiSourceKind, standing standingOf) ReadAiBudgetState {
	return ReadAiBudgetState{
		Choices: choiceOf{source}, Budget: standing, UnitOfWork: providerDirect{},
		Clock: clockAt{time.Date(2030, 3, 30, 15, 0, 0, 0, time.UTC)},
	}
}

func signedIn(scopes ...string) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: shared.ID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.ID("0192f000-0000-7000-8000-00000000000d"), Scopes: scopes,
	}
}

// The four words, at A-18's condition exactly: more than nine tenths.
func TestTheBudgetIsReadAsFourWords(t *testing.T) {
	for name, c := range map[string]struct {
		source   integration.AiSourceKind
		standing standingOf
		want     AiBudgetState
		renews   bool
	}{
		"no ceiling":             {integration.AiSourceOwn, standingOf{spent: 9_000_000}, AiBudgetOK, true},
		"plenty left":            {integration.AiSourceOffered, standingOf{spent: 100, limit: 1000, bounded: true}, AiBudgetOK, true},
		"exactly nine tenths":    {integration.AiSourceOffered, standingOf{spent: 900, limit: 1000, bounded: true}, AiBudgetOK, true},
		"more than nine tenths":  {integration.AiSourceOffered, standingOf{spent: 901, limit: 1000, bounded: true}, AiBudgetNearlySpent, true},
		"at the ceiling":         {integration.AiSourceOffered, standingOf{spent: 1000, limit: 1000, bounded: true}, AiBudgetSpent, true},
		"past the ceiling":       {integration.AiSourceOwn, standingOf{spent: 1200, limit: 1000, bounded: true}, AiBudgetSpent, true},
		"no source chosen (OFF)": {integration.AiSourceNone, standingOf{}, AiBudgetOff, false},
	} {
		t.Run(name, func(t *testing.T) {
			reading, err := budgetReader(c.source, c.standing).Execute(context.Background(), signedIn("items:read"))
			if err != nil {
				t.Fatalf("reading: %v", err)
			}
			if reading.State != c.want || reading.HasRenews != c.renews {
				t.Errorf("read %+v, want %s (renews %v)", reading, c.want, c.renews)
			}
			if c.renews && !reading.RenewsAt.Equal(time.Date(2030, 3, 31, 0, 0, 0, 0, time.UTC)) {
				t.Errorf("renews at %v, want the next midnight", reading.RenewsAt)
			}
		})
	}
}

// The source read is the workspace's choice: its own model's budget for OWN, the offered one's
// for OFFERED.
func TestTheBudgetReadIsTheChosenSources(t *testing.T) {
	for _, c := range []struct {
		source  integration.AiSourceKind
		wantOwn bool
	}{{integration.AiSourceOwn, true}, {integration.AiSourceOffered, false}} {
		asked := !c.wantOwn
		standing := standingOf{askedOwn: &asked}
		if _, err := budgetReader(c.source, standing).Execute(context.Background(), signedIn("items:read")); err != nil {
			t.Fatalf("reading: %v", err)
		}
		if asked != c.wantOwn {
			t.Errorf("%s read own=%v", c.source, asked)
		}
	}
}

// Signed in is the whole permission; a token must still carry the scope that reads entries.
func TestTheBudgetIsReadBySignedInPeopleWithTheScope(t *testing.T) {
	reader := budgetReader(integration.AiSourceOffered, standingOf{})
	if _, err := reader.Execute(context.Background(), appshared.ActorContext{}); !errors.Is(err, shared.ErrUnauthenticated) {
		t.Errorf("anonymous: %v", err)
	}
	token := signedIn("jobs:read")
	token.Kind = appshared.ActorUser
	if _, err := reader.Execute(context.Background(), token); err == nil {
		t.Error("a token without items:read read the budget")
	}
}

// The registry's answer: the word, and the moment or null.
func TestTheBudgetAnswerCarriesTheMomentOrNull(t *testing.T) {
	out, err := budgetReader(integration.AiSourceNone, standingOf{}).Descriptor().Handler.Invoke(
		context.Background(), signedIn("items:read"), nil)
	if err != nil {
		t.Fatalf("invoking: %v", err)
	}
	if out["state"] != "OFF" || out["renews_at"] != nil {
		t.Errorf("answered %v", out)
	}
}
