// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"strconv"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	AddAgentCaseName          = "AddAgentCase"
	ListAgentCasesName        = "ListAgentCases"
	RemoveAgentCaseName       = "RemoveAgentCase"
	RecordAgentEvaluationName = "RecordAgentEvaluation"
	ListAgentEvaluationsName  = "ListAgentEvaluations"
)

// AddAgentCase adds a golden case to an agent's evaluation set.
type AddAgentCase struct{ Desk Desk }

func (h AddAgentCase) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       AddAgentCaseName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Adds a golden case to an agent's evaluation set: a ticket's title and notes and what the " +
			"agent should answer for them - a collection_id, labels, a ci_id, any key its answer carries. " +
			"At most 500 cases. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the case and an audit entry.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true},
			{Name: "title", Kind: usecase.KindString, Required: true},
			{Name: "notes", Kind: usecase.KindString},
			{Name: "expected", Kind: usecase.KindObject, Required: true, Description: "The answer expected: each key compared, a list as a set."},
		},
		Audit:    usecase.AuditDeclaration{Action: CaseAddedAction, TargetType: agentTarget, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a case is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			if err := d.administer(ctx, actor, CaseAddedAction, scopeWrite, agentTarget, id); err != nil {
				return nil, err
			}
			expected, _ := in["expected"].(map[string]any)
			c, err := domain.CheckCase(domain.Case{
				ID: d.IDs.NewID(), AgentID: id, Title: in.String("title"), Notes: in.String("notes"), Expected: expected,
				CreatedBy: actor.AccountID, CreatedAt: d.Clock.Now(),
			}, d.Text)
			if err != nil {
				return nil, err
			}
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if _, err := d.Store.FindAgent(ctx, id); err != nil {
					return err
				}
				cases, err := d.Store.ListCases(ctx, id, domain.MaxCases)
				if err != nil {
					return err
				}
				if len(cases) >= domain.MaxCases {
					return shared.ErrConflict.WithDetail("agent_cases.too_many")
				}
				if err := d.Store.InsertCase(ctx, c); err != nil {
					return err
				}
				return d.record(ctx, actor, CaseAddedAction, agentTarget, id, audit.SeverityInfo, open("case_id", c.ID.String()))
			})
			if err != nil {
				return nil, err
			}
			return CaseOutput(c), nil
		}),
	}
}

// ListAgentCases answers an agent's evaluation set.
type ListAgentCases struct{ Desk Desk }

func (h ListAgentCases) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentCasesName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "An agent's evaluation set, oldest first: each case's words and expected answer. Whoever " +
			"administers agents reads it, and so does the agent itself, before it is live, to answer it.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "agent_id", Kind: usecase.KindID, Required: true}},
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			if err := d.itselfOrAdminister(ctx, actor, id, ReadAction, scopeRead); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				cases, err := d.Store.ListCases(ctx, id, domain.MaxCases)
				for _, c := range cases {
					rows = append(rows, CaseOutput(c))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// itselfOrAdminister lets the agent itself through, and asks anybody else what administering asks.
func (d Desk) itselfOrAdminister(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action, scope string) error {
	a, err := d.find(ctx, actor, id)
	if err != nil {
		return err
	}
	if actor.AccountID == a.AccountID {
		return actor.RequireScope(scope)
	}
	return d.administer(ctx, actor, action, scope, agentTarget, id)
}

// RemoveAgentCase takes a case out of the set.
type RemoveAgentCase struct{ Desk Desk }

func (h RemoveAgentCase) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        RemoveAgentCaseName,
		Module:      moduleOf(),
		Permission:  service.PermissionManageMembers,
		Summary:     "Takes a case out of an agent's evaluation set. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Deletes the case and writes an audit entry.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true},
			{Name: "case_id", Kind: usecase.KindID, Required: true},
		},
		Audit:    usecase.AuditDeclaration{Action: CaseRemovedAction, TargetType: agentTarget, Severity: audit.SeverityInfo, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a case is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			caseID, err := in.ID("case_id")
			if err != nil {
				return nil, err
			}
			if err := d.administer(ctx, actor, CaseRemovedAction, scopeWrite, agentTarget, id); err != nil {
				return nil, err
			}
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				removed, err := d.Store.DeleteCase(ctx, id, caseID)
				if err != nil {
					return err
				}
				if !removed {
					return shared.ErrNotFound.WithDetail("agent_cases.not_found")
				}
				return d.record(ctx, actor, CaseRemovedAction, agentTarget, id, audit.SeverityInfo, open("case_id", caseID.String()))
			})
			return nil, err
		}),
	}
}

// RecordAgentEvaluation judges an agent's answers to its set on its model as it now stands (G7).
// The server compares; whoever sends the answers - the agent itself, or a person running it -
// cannot state a score.
type RecordAgentEvaluation struct{ Desk Desk }

func (h RecordAgentEvaluation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RecordAgentEvaluationName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Records an evaluation run of an agent: its answer to each case of its set, compared " +
			"here with the expected answer - each expected key, a list as a set; a case left " +
			"unanswered fails. The run counts for the model's revision as it now stands, and " +
			"ActivateAgent asks the newest one to reach the agent's bar. The agent itself answers its " +
			"set before it is live, or whoever administers agents.",
		SideEffects: "Writes the run's counts and an audit entry; no answer is kept.",
		TokenScope:  scopeWrite,
		Input: []usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true},
			{Name: "answers", Kind: usecase.KindList, Required: true, Description: "One object per case: case_id and answer."},
		},
		Audit:    usecase.AuditDeclaration{Action: EvaluatedAction, TargetType: agentTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "a run is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			if err := d.itselfOrAdminister(ctx, actor, id, EvaluatedAction, scopeWrite); err != nil {
				return nil, err
			}
			answers := map[string]map[string]any{}
			raw, _ := in["answers"].([]any)
			for i, item := range raw {
				row, _ := item.(map[string]any)
				caseID, _ := row["case_id"].(string)
				answer, _ := row["answer"].(map[string]any)
				if caseID == "" || answer == nil {
					return nil, shared.ErrValidation.WithDetail("agent_cases.answer_invalid").
						WithFields(shared.FieldError{Path: "/answers/" + strconv.Itoa(i), Code: "agent_cases.answer_invalid"})
				}
				answers[caseID] = answer
			}
			var run domain.Run
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := d.Store.FindAgent(ctx, id)
				if err != nil {
					return err
				}
				revision, _, err := d.modelRevision(ctx, a)
				if err != nil {
					return err
				}
				cases, err := d.Store.ListCases(ctx, id, domain.MaxCases)
				if err != nil {
					return err
				}
				if len(cases) == 0 {
					return shared.ErrConflict.WithDetail("agent_cases.none")
				}
				run = Score(cases, answers)
				run.ID, run.AgentID, run.ModelRevision, run.By, run.At = d.IDs.NewID(), id, revision, actor.AccountID, d.Clock.Now()
				if err := d.Store.InsertRun(ctx, run); err != nil {
					return err
				}
				return d.record(ctx, actor, EvaluatedAction, agentTarget, id, audit.SeverityNotice,
					open("cases", strconv.Itoa(run.Cases)), open("passed", strconv.Itoa(run.Passed)),
					open("model_revision", strconv.Itoa(revision)))
			})
			if err != nil {
				return nil, err
			}
			return RunOutput(run), nil
		}),
	}
}

// Score compares answers, keyed by case, with a set: a case unanswered fails.
func Score(cases []domain.Case, answers map[string]map[string]any) domain.Run {
	run := domain.Run{Cases: len(cases)}
	for _, c := range cases {
		if answer, found := answers[c.ID.String()]; found && domain.Matches(c.Expected, answer) {
			run.Passed++
		}
	}
	return run
}

// ListAgentEvaluations answers an agent's runs, newest first.
type ListAgentEvaluations struct{ Desk Desk }

func (h ListAgentEvaluations) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentEvaluationsName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "An agent's evaluation runs, newest first: the model revision, cases, passed and " +
			"accuracy, who ran it and when. Its owner and whoever administers agents read them.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input:       []usecase.Field{{Name: "agent_id", Kind: usecase.KindID, Required: true}},
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			a, err := d.find(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := d.ownerOrAdminister(ctx, actor, a, ReadAction, scopeRead); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				runs, err := d.Store.ListRuns(ctx, id, maxActions)
				for _, r := range runs {
					rows = append(rows, RunOutput(r))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// CaseOutput is a case as every door answers it.
func CaseOutput(c domain.Case) usecase.Output {
	return usecase.Output{
		"id": c.ID.String(), "agent_id": c.AgentID.String(), "title": c.Title, "notes": c.Notes,
		"expected": c.Expected, "created_by": idOrNil(c.CreatedBy), "created_at": c.CreatedAt.UTC(),
	}
}

// RunOutput is a run as every door answers it.
func RunOutput(r domain.Run) usecase.Output {
	return usecase.Output{
		"id": r.ID.String(), "agent_id": r.AgentID.String(), "model_revision": r.ModelRevision, "cases": r.Cases,
		"passed": r.Passed, "accuracy": r.Accuracy(), "run_by": idOrNil(r.By), "at": r.At.UTC(),
	}
}
