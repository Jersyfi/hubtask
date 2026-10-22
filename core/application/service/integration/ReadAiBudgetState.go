// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/integration"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ReadAiBudgetStateName is the catalogue name.
const ReadAiBudgetStateName = "ReadAiBudgetState"

// aiBudgetStateRead names the read where a refusal is recorded; the read itself writes nothing.
const aiBudgetStateRead audit.Action = "ai.budget_state_read"

// budgetStateScope is the scope the read asks of a token: the one every person's token carries to
// read entries, because this is read by whoever is about to ask AI about one.
const budgetStateScope = "items:read"

// nearlySpent is A-18's number: more than nine tenths of a ceiling used is a workspace that will
// meet the wall on a normal afternoon. The same condition, so a person and an operator are told
// at the same moment.
const nearlySpent = 0.9

// AiBudgetState is how the day's budget of the source a workspace chose stands.
type AiBudgetState string

const (
	// AiBudgetOK is room, or no ceiling at all.
	AiBudgetOK AiBudgetState = "OK"
	// AiBudgetNearlySpent is more than nine tenths used.
	AiBudgetNearlySpent AiBudgetState = "NEARLY_SPENT"
	// AiBudgetSpent is none left until the workspace's next midnight.
	AiBudgetSpent AiBudgetState = "SPENT"
	// AiBudgetOff is a workspace that chose no source: there is nothing to spend.
	AiBudgetOff AiBudgetState = "OFF"
)

// AiBudgetStanding is the slice of the quota guard this read needs.
type AiBudgetStanding interface {
	// AiStanding answers the day's spend of one source, its ceiling, and whether there is one.
	AiStanding(ctx context.Context, own bool, now time.Time) (spent, limit int64, bounded bool, err error)
	// AiRenewal is the workspace's next midnight after `now`.
	AiRenewal(ctx context.Context, now time.Time) (time.Time, error)
}

// ReadAiBudgetState answers, to every signed-in person of the workspace, whether AI can still be
// asked today (UC-AI-04 check 9): said before asking, not after the refusal.
//
// **No permission beyond being signed in here**, like the manifest's `ai_suggestions` beside it: a
// person of any role may be about to ask, a reading aid included. What it answers is one of four
// words and a moment - no number, and nothing about who spent what, which stays the
// administrator's (`GET /quotas`).
//
// **The workspace's choice, not the resolver.** Which source answers is what the workspace chose;
// building the provider to learn it would unseal a key to answer a question about a counter.
type ReadAiBudgetState struct {
	Choices    repository.AiChoices
	Budget     AiBudgetStanding
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
}

// BudgetReading is the answer: the state, and when the day's budget has room again.
type BudgetReading struct {
	State     AiBudgetState
	RenewsAt  time.Time
	HasRenews bool
}

// Execute reads the state.
func (h ReadAiBudgetState) Execute(ctx context.Context, actor appshared.ActorContext) (BudgetReading, error) {
	if !actor.IsAuthenticated() {
		return BudgetReading{}, shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	if err := actor.RequireScope(budgetStateScope); err != nil {
		return BudgetReading{}, err
	}

	var reading BudgetReading
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		choice, err := h.Choices.Find(ctx)
		if err != nil {
			return err
		}
		if choice.Source != integration.AiSourceOwn && choice.Source != integration.AiSourceOffered {
			reading = BudgetReading{State: AiBudgetOff}
			return nil
		}
		now := h.Clock.Now()
		spent, limit, bounded, err := h.Budget.AiStanding(ctx, choice.Source == integration.AiSourceOwn, now)
		if err != nil {
			return err
		}
		renews, err := h.Budget.AiRenewal(ctx, now)
		if err != nil {
			return err
		}
		reading = BudgetReading{State: stateOf(spent, limit, bounded), RenewsAt: renews, HasRenews: true}
		return nil
	})
	if err != nil {
		return BudgetReading{}, err
	}
	return reading, nil
}

// stateOf turns a day's spend into the word a person reads.
func stateOf(spent, limit int64, bounded bool) AiBudgetState {
	switch {
	case !bounded || limit <= 0:
		return AiBudgetOK
	case spent >= limit:
		return AiBudgetSpent
	case float64(spent)/float64(limit) > nearlySpent:
		return AiBudgetNearlySpent
	default:
		return AiBudgetOK
	}
}

// Descriptor registers the read in all three channels.
func (h ReadAiBudgetState) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ReadAiBudgetStateName,
		Summary: "Answers whether the AI source this workspace chose can still be asked today: OK, " +
			"NEARLY_SPENT (more than nine tenths of the day's budget used), SPENT (none left until " +
			"renews_at), or OFF (no source chosen). Answered to every signed-in person of the " +
			"workspace; it names no number and nobody's use.",
		SideEffects: "None.",
		TokenScope:  budgetStateScope,
		ReadOnly:    true,
		Audit: usecase.AuditDeclaration{
			Action: aiBudgetStateRead, TargetType: "ai_budget", Severity: audit.SeverityInfo,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "A read; nothing changes."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadAiBudgetState) invoke(
	ctx context.Context, actor appshared.ActorContext, _ usecase.Input,
) (usecase.Output, error) {
	reading, err := h.Execute(ctx, actor)
	if err != nil {
		return nil, err
	}
	out := usecase.Output{"state": string(reading.State), "renews_at": nil}
	if reading.HasRenews {
		out["renews_at"] = reading.RenewsAt.UTC().Format(time.RFC3339)
	}
	return out, nil
}
