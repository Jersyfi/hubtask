// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"time"

	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AgentApplyKey is the setting that decides what an agent's apply does (D-16).
const AgentApplyKey setting.Key = "config.agent_apply"

// AgentApply is the setting's value.
type AgentApply string

const (
	// AgentApplyAllowed applies an agent's plan as a person's.
	AgentApplyAllowed AgentApply = "ALLOWED"
	// AgentApplyApproval keeps an agent's plan as a proposal a person applies or declines: the
	// default.
	AgentApplyApproval AgentApply = "APPROVAL"
	// AgentApplyNever refuses an agent's apply; it may still plan.
	AgentApplyNever AgentApply = "NEVER"
)

// AgentApplyDefinition is the setting's entry in the catalogue. The installation and a plan may
// lock it; a workspace may only tighten it - from allowed to approval to never.
func AgentApplyDefinition() setting.Definition {
	options := []string{string(AgentApplyAllowed), string(AgentApplyApproval), string(AgentApplyNever)}
	return setting.Definition{
		Key: AgentApplyKey, Kind: setting.KindChoice, Options: options,
		Levels:   []setting.Level{setting.LevelInstance, setting.LevelPlan, setting.LevelWorkspace},
		Lockable: true, May: setting.MayTighten, Default: string(AgentApplyApproval),
		Writers: map[setting.Level]string{
			setting.LevelInstance: "admin:instance", setting.LevelPlan: "admin:instance",
			setting.LevelWorkspace: "CONFIGURE",
		},
		Tightens: func(inForce, proposed any) bool {
			return strictness(proposed) >= strictness(inForce)
		},
	}
}

func strictness(v any) int {
	switch AgentApply(textOf(v)) {
	case AgentApplyAllowed:
		return 0
	case AgentApplyNever:
		return 2
	default:
		return 1
	}
}

func textOf(v any) string {
	s, _ := v.(string)
	return s
}

// AgentApplyOf reads a resolution's value, approval where it is unreadable: the safe direction.
func AgentApplyOf(r setting.Resolution) AgentApply {
	switch value := AgentApply(textOf(r.Effective)); value {
	case AgentApplyAllowed, AgentApplyNever:
		return value
	default:
		return AgentApplyApproval
	}
}

// Revision is one configuration version: the document in force after an apply, who applied it,
// who proposed it where that was an agent, and the counts of what changed - never content (rule 10).
type Revision struct {
	TenantID shared.ID
	Version  int
	Document Document
	Hash     string
	PlanHash string
	// ActorKind and ActorID are who applied it; ProposedBy the agent whose plan it was, if any.
	ActorKind  shared.ActorKind
	ActorID    shared.ID
	ProposedBy shared.ID
	// RolledBackTo is the version a rollback restored, 0 for every other apply.
	RolledBackTo int
	// Template is the template@version an apply took, empty for every other one.
	Template string
	Counts   Counts
	At       time.Time
}

// Counts is how many blocks a plan adds, changes and removes.
type Counts struct {
	Added   int
	Changed int
	Removed int
}

// CountsOf counts the changes.
func CountsOf(changes []BlockChange) Counts {
	var c Counts
	for _, change := range changes {
		switch change.Op {
		case OpAdd:
			c.Added++
		case OpChange:
			c.Changed++
		case OpRemove:
			c.Removed++
		}
	}
	return c
}

// ProposalState is where an agent's plan stands.
type ProposalState string

const (
	ProposalWaiting  ProposalState = "WAITING"
	ProposalApplied  ProposalState = "APPLIED"
	ProposalDeclined ProposalState = "DECLINED"
)

// Proposal is an agent's plan waiting for a person (D-16): the document it asks for, read in its
// mode against the configuration it was planned on, and the plan's hash a person applies by.
type Proposal struct {
	ID         shared.ID
	TenantID   shared.ID
	State      ProposalState
	Document   Document
	Mode       Mode
	BaseHash   string
	PlanHash   string
	Template   string
	Counts     Counts
	Flags      Flags
	ProposedBy shared.ID
	DecidedBy  shared.ID
	CreatedAt  time.Time
	DecidedAt  *time.Time
}

// ErrProposalDecided refuses a decision on a proposal that is no longer waiting.
func ErrProposalDecided() error {
	return shared.ErrConflict.WithDetail("config.proposal_decided")
}

// ErrAgentApplyNever refuses an agent's apply where the workspace allows none.
func ErrAgentApplyNever() error {
	return shared.ErrForbidden.WithDetail("config.agent_apply_never")
}
