// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package quota turns multi-tenancy.md §4's table into enforcement: per-tenant limits
// from `tenant.settings.quotas` with the mode's defaults, one resolution, one refusal shape.
//
// The refusal split the backlog fixes: a *rate* is a 429 with Retry-After, because waiting
// helps; a *capacity* quota is a 422-shaped refusal naming the quota and the ceiling, because
// waiting does not. Every capacity code is `capacity.<quota>`, documented in
// api-guidelines.md §6.
package quota

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/quota"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	env "github.com/Jersyfi/hubtask/core/port/environment"
)

// The quota names, exactly the multi-tenancy.md §4 rows enforced per tenant. They are the
// contract's enum, the settings document's keys, the capacity codes' suffixes and the metric's
// `quota` label - one vocabulary, spelled once.
const (
	APIRequestsPerMinute  = "api_requests_per_minute"
	Items                 = "items"
	MediaBytes            = "media_bytes"
	AutomationRunsPerHour = "automation_runs_per_hour"
	WebhookTargets        = "webhook_targets"
	ExportJobs            = "export_jobs"
	// Accounts is the people a workspace may hold: active and managed accounts and open
	// invitations, never service accounts (§4.2).
	Accounts = "accounts"
	// AiTokensPerDay is what a workspace may spend on AI in a day, counted in the tokens
	// every provider reports - the one unit that means the same thing across OpenAI-compatible
	// endpoints and a local Ollama, which is why `ai.Usage` carries it.
	//
	//nolint:gosec // G101: a quota's name, not a credential - "token" here is a model's unit of text
	AiTokensPerDay = "ai_tokens_per_day"
	// AiOwnTokensPerDay is the same day's spend on the workspace's **own** model (ADR-0072 §4):
	// limited only by the workspace's own optional budget, off by default, and never by the
	// installation (UC-AI-06 check 6). AiTokensPerDay bounds the offered source alone.
	//
	//nolint:gosec // G101: a quota's name, not a credential
	AiOwnTokensPerDay = "ai_own_tokens_per_day"
	// AiArrivalProposalsPerDay is how many jumble arrivals a workspace that prepares proposals on
	// arrival has asked about in a day (UC-JUM-07 check 10). Beside the token budget rather than
	// inside it: whoever holds the workspace's intake address can make things arrive, and the day's
	// arrivals are what this counts (security.md T-27).
	AiArrivalProposalsPerDay = "ai_arrival_proposals_per_day"
)

// Names is the vocabulary in the contract's order.
func Names() []string {
	return []string{
		APIRequestsPerMinute, Items, MediaBytes, AutomationRunsPerHour, WebhookTargets, ExportJobs,
		AiTokensPerDay, AiOwnTokensPerDay, AiArrivalProposalsPerDay, Accounts,
	}
}

// Declaration is every quota's as the installation's level answers it: a whole number, 0 being
// unlimited, which a workspace may raise with a ceiling of its own while it is open.
func Declaration() identity.ValueDeclaration {
	least := 0
	return identity.ValueDeclaration{Kind: identity.KindCount, Minimum: &least, May: identity.MayRaise}
}

// OperatorNames is the vocabulary an operator writes - per workspace and as the installation's
// defaults: every quota but the workspace's own AI budget, which is the workspace's alone
// (UC-AI-06 check 6).
func OperatorNames() []string {
	names := make([]string, 0, len(Names()))
	for _, name := range Names() {
		if name != AiOwnTokensPerDay {
			names = append(names, name)
		}
	}
	return names
}

// Unlimited is the ceiling that never engages. It is a documented value, not an absence: a
// workspace may be *configured* unlimited, which survives a change of the mode's default.
const Unlimited int64 = 0

// Limits is the resolved table: every quota, one ceiling each, 0 = unlimited.
type Limits struct {
	APIRequestsPerMinute  int64
	Items                 int64
	MediaBytes            int64
	AutomationRunsPerHour int64
	WebhookTargets        int64
	ExportJobs            int64
	Accounts              int64
	AiTokensPerDay        int64
	AiOwnTokensPerDay     int64
	// AiArrivalProposalsPerDay is a number of arrivals, not of tokens.
	AiArrivalProposalsPerDay int64
}

// Defaults answers §4's two columns. Single mode's defaults stay effectively unlimited - the
// enforcement exists everywhere, the numbers differ, one code path (§1). "Plan-bound" rows default
// to unlimited in both modes: a bound is a plan's or the operator's own write.
func Defaults(mode env.TenancyMode) Limits {
	if mode == env.TenancyMulti {
		return Limits{
			APIRequestsPerMinute:  600,
			Items:                 Unlimited,
			MediaBytes:            Unlimited,
			AutomationRunsPerHour: 1_000,
			WebhookTargets:        50,
			ExportJobs:            2,
			// A real number in multi mode, because AI is the one feature whose marginal cost
			// leaves the installation: every token is somebody's invoice. Two hundred thousand is
			// a working day of suggestions for a busy workspace and a wall a runaway loop reaches
			// in minutes - which is the shape a budget wants, since what it protects against is a
			// mistake rather than use.
			AiTokensPerDay: 200_000,
			// A busy workspace's day of arrivals, and a wall a flood of deliveries to the intake
			// address reaches long before the token budget would.
			AiArrivalProposalsPerDay: DefaultArrivalProposalsPerDay,
		}
	}
	return Limits{
		APIRequestsPerMinute:  6_000,
		Items:                 Unlimited,
		MediaBytes:            Unlimited,
		AutomationRunsPerHour: 100_000,
		WebhookTargets:        Unlimited,
		ExportJobs:            5,
		// Unlimited for a self-hoster, who is either running a local model - where a token costs
		// electricity - or paying their own provider directly. A default ceiling there would be
		// this project deciding how much of somebody's own machine they may use.
		AiTokensPerDay: Unlimited,
		// Not unlimited here, unlike the tokens: the tokens are a self-hoster's own to spend, but
		// what arrives is not theirs to choose - anybody holding the intake address decides it.
		AiArrivalProposalsPerDay: DefaultArrivalProposalsPerDay,
	}
}

// DefaultArrivalProposalsPerDay is both modes' default for the proposals prepared on arrival.
const DefaultArrivalProposalsPerDay = 200

// The levels a limit resolves through and the words a standing says it with: which level set the
// value in force, and which locked it.
const (
	SourceDefault   = "DEFAULT"
	SourceInstance  = "INSTANCE"
	SourcePlan      = "PLAN"
	SourceException = "EXCEPTION"
	// SourceWorkspace is the workspace's own AI budget, the one ceiling a workspace sets itself.
	SourceWorkspace = "WORKSPACE"

	LockInstance = "INSTANCE"
	LockPlan     = "PLAN"
)

// Levels is everything a workspace's limits resolve from (multi-tenancy.md §4.2): the installation's
// defaults and locks, the workspace's plan's, and the operator's exception for this one workspace,
// which also holds the workspace's own AI budget.
type Levels struct {
	Instance      repository.Overrides
	InstanceLocks map[string]bool
	Plan          repository.PlanLimits
	Exception     repository.Overrides
}

// ReadLevels reads the levels in the caller's transaction, under the workspace's scope.
func ReadLevels(ctx context.Context, store repository.Store) (Levels, error) {
	var levels Levels
	var err error
	if levels.Exception, err = store.Overrides(ctx); err != nil {
		return Levels{}, err
	}
	if levels.Instance, err = store.InstanceOverrides(ctx); err != nil {
		return Levels{}, err
	}
	if levels.InstanceLocks, err = store.InstanceLocks(ctx); err != nil {
		return Levels{}, err
	}
	if levels.Plan, err = store.PlanLimits(ctx); err != nil {
		return Levels{}, err
	}
	return levels, nil
}

// planApplies reports whether the plan's value for a quota is read: never under the installation's
// lock, which binds the plan as it binds every workspace.
func (l Levels) planApplies(name string) bool {
	return !l.InstanceLocks[name] && valueOf(l.Plan.Overrides, name) != nil
}

// SourceOf answers which level set the value in force.
func (l Levels) SourceOf(name string) string {
	switch {
	case valueOf(l.Exception, name) != nil:
		if name == AiOwnTokensPerDay {
			return SourceWorkspace
		}
		return SourceException
	case name == AiOwnTokensPerDay:
		return SourceDefault
	case l.planApplies(name):
		return SourcePlan
	case valueOf(l.Instance, name) != nil:
		return SourceInstance
	default:
		return SourceDefault
	}
}

// LockOf answers which level locked a quota, or "" - the installation's lock first, which binds the
// plan's. The operator's exception still decides under either: a lock says what a workspace may not
// ask for, not what the operator may not grant.
func (l Levels) LockOf(name string) string {
	switch {
	case name == AiOwnTokensPerDay:
		return ""
	case l.InstanceLocks[name]:
		return LockInstance
	case l.Plan.Locks[name]:
		return LockPlan
	default:
		return ""
	}
}

// Resolve answers the limits in force: the mode's table, the installation's defaults, the plan's
// where the installation does not lock them, and the operator's exception as the last word. The own
// model's budget is the workspace's alone: no product default, installation or plan reaches it.
func Resolve(levels Levels, mode env.TenancyMode) Limits {
	limits := Defaults(mode)
	for _, name := range OperatorNames() {
		if value := valueOf(levels.Instance, name); value != nil {
			limits.set(name, *value)
		}
		if levels.planApplies(name) {
			limits.set(name, *valueOf(levels.Plan.Overrides, name))
		}
		if value := valueOf(levels.Exception, name); value != nil {
			limits.set(name, *value)
		}
	}
	if value := levels.Exception.AiOwnTokensPerDay; value != nil {
		limits.AiOwnTokensPerDay = *value
	}
	return limits
}

// valueOf answers one quota's value at a level, nil where the level set none.
func valueOf(overrides repository.Overrides, name string) *int64 {
	switch name {
	case APIRequestsPerMinute:
		return overrides.APIRequestsPerMinute
	case Items:
		return overrides.Items
	case MediaBytes:
		return overrides.MediaBytes
	case AutomationRunsPerHour:
		return overrides.AutomationRunsPerHour
	case WebhookTargets:
		return overrides.WebhookTargets
	case ExportJobs:
		return overrides.ExportJobs
	case Accounts:
		return overrides.Accounts
	case AiTokensPerDay:
		return overrides.AiTokensPerDay
	case AiOwnTokensPerDay:
		return overrides.AiOwnTokensPerDay
	case AiArrivalProposalsPerDay:
		return overrides.AiArrivalProposalsPerDay
	default:
		return nil
	}
}

// set writes one quota's ceiling by name.
func (l *Limits) set(name string, value int64) {
	switch name {
	case APIRequestsPerMinute:
		l.APIRequestsPerMinute = value
	case Items:
		l.Items = value
	case MediaBytes:
		l.MediaBytes = value
	case AutomationRunsPerHour:
		l.AutomationRunsPerHour = value
	case WebhookTargets:
		l.WebhookTargets = value
	case ExportJobs:
		l.ExportJobs = value
	case Accounts:
		l.Accounts = value
	case AiTokensPerDay:
		l.AiTokensPerDay = value
	case AiOwnTokensPerDay:
		l.AiOwnTokensPerDay = value
	case AiArrivalProposalsPerDay:
		l.AiArrivalProposalsPerDay = value
	}
}

// Of answers one quota's ceiling by name.
func (l Limits) Of(quota string) int64 {
	switch quota {
	case APIRequestsPerMinute:
		return l.APIRequestsPerMinute
	case Items:
		return l.Items
	case MediaBytes:
		return l.MediaBytes
	case AutomationRunsPerHour:
		return l.AutomationRunsPerHour
	case WebhookTargets:
		return l.WebhookTargets
	case ExportJobs:
		return l.ExportJobs
	case Accounts:
		return l.Accounts
	case AiTokensPerDay:
		return l.AiTokensPerDay
	case AiOwnTokensPerDay:
		return l.AiOwnTokensPerDay
	case AiArrivalProposalsPerDay:
		return l.AiArrivalProposalsPerDay
	default:
		return Unlimited
	}
}

// Exceeded is the capacity refusal: 422-shaped, naming the quota and the ceiling, because
// waiting does not help - what helps is removing something, or the operator moving the wall.
func Exceeded(quota string, limit, used int64) error {
	return shared.ErrValidation.
		WithDetail("capacity." + quota).
		WithParams(map[string]string{
			"quota": quota,
			"limit": strconv.FormatInt(limit, 10),
			"used":  strconv.FormatInt(used, 10),
		})
}

// Ratio is used/limit, the number the metric reports; 0 while the limit is unlimited.
func Ratio(limit, used int64) float64 {
	if limit <= 0 {
		return 0
	}
	return float64(used) / float64(limit)
}
