// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package agent

import (
	"context"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/agent"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The use case names.
const (
	RegisterAgentName = "RegisterAgent"
	ListAgentsName    = "ListAgents"
	ReadAgentName     = "ReadAgent"
	ChangeAgentName   = "ChangeAgent"
	ApproveAgentName  = "ApproveAgent"
	ActivateAgentName = "ActivateAgent"
	SuspendAgentName  = "SuspendAgent"
	RetireAgentName   = "RetireAgent"
	SetAgentHaltName  = "SetAgentHalt"
	ReadAgentHaltName = "ReadAgentHalt"

	createServiceAccount = "CreateServiceAccount"
)

func agentFields() []usecase.Field {
	return []usecase.Field{
		{Name: "name", Kind: usecase.KindString, Required: true, Description: "What the registry and the audit trail show."},
		{Name: "purpose", Kind: usecase.KindString, Required: true, Description: "What it is for, in a sentence or two."},
		{Name: "kind", Kind: usecase.KindString, Required: true, Enum: []string{"TICKET_TRIAGE", "CI_SUGGESTION", "DUPLICATE_REVIEW", "KNOWLEDGE_DRAFT", "EXTERNAL"}, Description: "One of the shipped agents, or one built elsewhere that reaches the workspace through MCP or the API."},
		{Name: "owner_id", Kind: usecase.KindID, Description: "The person it acts for; the caller without. It never does what they may not."},
		{Name: "risk_class", Kind: usecase.KindString, Enum: []string{"MINIMAL", "LIMITED", "HIGH"}, Description: "Its class under the AI Act: LIMITED for one that talks to a person. HIGH is refused. MINIMAL without."},
		{Name: "scopes", Kind: usecase.KindList, Description: "The token scopes it may use; a token of its account holding more is narrowed to these."},
		{Name: "model_id", Kind: usecase.KindID, Description: "The registered model it may call."},
		{Name: "policy", Kind: usecase.KindObject, Description: "reversible (ALLOW, NOTIFY, APPROVAL, NEVER) and irreversible (APPROVAL, NEVER); both APPROVAL without. A read always runs."},
		{Name: "budget", Kind: usecase.KindObject, Description: "tokens_per_day, actions_per_hour, concurrent_runs; a small budget without."},
		{Name: "min_accuracy", Kind: usecase.KindInt, Description: "The share of its evaluation cases, in percent, a run must pass before it goes live; 0 asks for none."},
	}
}

func agentFrom(in usecase.Input, actor appshared.ActorContext) (domain.Agent, error) {
	owner, err := optionalID(in, "owner_id")
	if err != nil {
		return domain.Agent{}, err
	}
	if owner.IsZero() {
		owner = actor.AccountID
	}
	model, err := optionalID(in, "model_id")
	if err != nil {
		return domain.Agent{}, err
	}
	scopes, err := in.StringList("scopes")
	if err != nil {
		return domain.Agent{}, err
	}
	risk := domain.RiskClass(in.String("risk_class"))
	if risk == "" {
		risk = domain.RiskMinimal
	}
	a := domain.Agent{
		Name: in.String("name"), Purpose: in.String("purpose"), Kind: domain.Kind(in.String("kind")), OwnerID: owner,
		RiskClass: risk, Scopes: scopes, ModelID: model, Policy: domain.DefaultPolicy(), Budget: domain.DefaultBudget(),
		MinAccuracy: in.Int("min_accuracy"),
	}
	if policy, ok := in["policy"].(map[string]any); ok {
		if v, ok := policy["reversible"].(string); ok {
			a.Policy.Reversible = domain.Rule(v)
		}
		if v, ok := policy["irreversible"].(string); ok {
			a.Policy.Irreversible = domain.Rule(v)
		}
	}
	if budget, ok := in["budget"].(map[string]any); ok {
		if v, ok := number(budget["tokens_per_day"]); ok {
			a.Budget.TokensPerDay = int64(v)
		}
		if v, ok := number(budget["actions_per_hour"]); ok {
			a.Budget.ActionsPerHour = int(v)
		}
		if v, ok := number(budget["concurrent_runs"]); ok {
			a.Budget.ConcurrentRuns = int(v)
		}
	}
	return a, nil
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	}
	return 0, false
}

// usable checks that a named model is registered and not retired.
func (d Desk) usable(ctx context.Context, model shared.ID) error {
	if model.IsZero() {
		return nil
	}
	m, err := d.Store.FindModel(ctx, model)
	if err != nil {
		return shared.ErrValidation.WithDetail("agents.model_unknown").
			WithFields(shared.FieldError{Path: "/model_id", Code: "agents.model_unknown"})
	}
	if m.Retired {
		return shared.ErrValidation.WithDetail("agents.model_retired").
			WithFields(shared.FieldError{Path: "/model_id", Code: "agents.model_retired"})
	}
	return nil
}

// RegisterAgent registers an agent: a service account of its own to act as, drafted until a person
// approves it.
type RegisterAgent struct{ Desk Desk }

func (h RegisterAgent) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       RegisterAgentName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Registers an agent: a non-human principal with a service account of its own, acting " +
			"for a person and never doing what they may not. It states its purpose, kind, risk class " +
			"(HIGH is refused), the scopes it may use, its model, what it may do without a person and " +
			"its budget. It starts as a draft: nothing it attempts runs until somebody other than its " +
			"owner approves it and it is put live. Give its account roles and a token as for any " +
			"service account. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes a service account, the agent and audit entries.",
		TokenScope:  scopeWrite,
		Input:       agentFields(),
		Audit:       usecase.AuditDeclaration{Action: RegisteredAction, TargetType: agentTarget, Severity: audit.SeverityNotice, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "an agent is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			if err := d.administer(ctx, actor, RegisteredAction, scopeWrite, agentTarget, ""); err != nil {
				return nil, err
			}
			a, err := agentFrom(in, actor)
			if err != nil {
				return nil, err
			}
			a, err = domain.Check(a, d.Text)
			if err != nil {
				return nil, err
			}
			now := d.Clock.Now()
			a.ID, a.TenantID, a.Status, a.CreatedBy, a.CreatedAt, a.UpdatedAt, a.Version = d.IDs.NewID(), actor.TenantID, domain.StatusDraft, actor.AccountID, now, now, 1
			writing, refusals := access.HoldRefusals(ctx)
			defer refusals.Record(ctx)
			err = d.UnitOfWork.Within(writing, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := d.usable(ctx, a.ModelID); err != nil {
					return err
				}
				account, err := d.Catalogue.Invoke(ctx, createServiceAccount, actor, usecase.Input{"display_name": a.Name})
				if err != nil {
					return err
				}
				if a.AccountID, err = shared.ParseID(account.String("id")); err != nil {
					return shared.ErrInternal.WithDetail("agents.account_unreadable").WithCause(err)
				}
				if err := d.Store.InsertAgent(ctx, a); err != nil {
					return err
				}
				if err := d.agentChanged(ctx, actor, a); err != nil {
					return err
				}
				return d.record(ctx, actor, RegisteredAction, agentTarget, a.ID, audit.SeverityNotice,
					open("kind", string(a.Kind)), open("owner_id", a.OwnerID.String()),
					open("account_id", a.AccountID.String()), open("risk_class", string(a.RiskClass)),
					open("scopes", strings.Join(a.Scopes, " ")))
			})
			if err != nil {
				return nil, err
			}
			return AgentOutput(a, 0), nil
		}),
	}
}

// ListAgents answers the registry.
type ListAgents struct{ Desk Desk }

func (h ListAgents) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ListAgentsName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "The workspace's agents by name: kind, owner, status, model, policy, budget and whether " +
			"its approval still covers its model. Whoever does not administer agents is answered the " +
			"agents they own.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "owner_id", Kind: usecase.KindID, Description: "Only the agents acting for this person."},
			{Name: "model_id", Kind: usecase.KindID, Description: "Only the agents using this model."},
		},
		Audit:    usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			owner, err := optionalID(in, "owner_id")
			if err != nil {
				return nil, err
			}
			model, err := optionalID(in, "model_id")
			if err != nil {
				return nil, err
			}
			if err := actor.RequireScope(scopeRead); err != nil {
				return nil, err
			}
			if err := d.administer(ctx, actor, ReadAction, scopeRead, agentTarget, ""); err != nil {
				// Their own agents only, for whoever is no administrator.
				if owner != "" && owner != actor.AccountID {
					return nil, err
				}
				owner = actor.AccountID
			}
			rows := []usecase.Output{}
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				agents, err := d.Store.ListAgents(ctx, owner, model, maxListed)
				if err != nil {
					return err
				}
				for _, a := range agents {
					revision, _, err := d.modelRevision(ctx, a)
					if err != nil {
						return err
					}
					rows = append(rows, AgentOutput(a, revision))
				}
				return nil
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// ReadAgent answers one agent with its spending now.
type ReadAgent struct{ Desk Desk }

func (h ReadAgent) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadAgentName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "One agent with what it spent this hour and today. Its owner, an administrator of " +
			"agents, and the agent itself read it.",
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
			if actor.AccountID == a.AccountID {
				if err := actor.RequireScope(scopeRead); err != nil {
					return nil, err
				}
			} else if err := d.ownerOrAdminister(ctx, actor, a, ReadAction, scopeRead); err != nil {
				return nil, err
			}
			var out usecase.Output
			err = d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				revision, _, err := d.modelRevision(ctx, a)
				if err != nil {
					return err
				}
				hour, day := windows(d.Clock.Now())
				usage, err := d.Store.Usage(ctx, a.ID, hour, day)
				if err != nil {
					return err
				}
				out = AgentOutput(a, revision)
				out["usage"] = usecase.Output{"actions_this_hour": usage.ActionsLastHour, "tokens_today": usage.TokensToday}
				return nil
			})
			return out, err
		}),
	}
}

// ChangeAgent changes what an agent is and may do. A change that widens it - scopes, model, a
// looser rule, a lower bar, another owner, kind or class - takes it back to a draft.
type ChangeAgent struct{ Desk Desk }

func (h ChangeAgent) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ChangeAgentName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "Changes an agent's purpose, scopes, model, policy, budget or evaluation bar. A change " +
			"that lets it do more - wider scopes, another model, a looser rule, a lower bar, another " +
			"owner, kind or risk class - takes it back to a draft until it is approved again; a smaller " +
			"budget does not. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "Writes the agent and an audit entry.",
		TokenScope:  scopeWrite,
		Input: append([]usecase.Field{
			{Name: "agent_id", Kind: usecase.KindID, Required: true},
			{Name: "expected_version", Kind: usecase.KindInt, CallerOnly: true, Description: "The version last read."},
		}, agentFields()...),
		Audit:    usecase.AuditDeclaration{Action: ChangedAction, TargetType: agentTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "an agent is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			id, err := in.ID("agent_id")
			if err != nil {
				return nil, err
			}
			if err := d.administer(ctx, actor, ChangedAction, scopeWrite, agentTarget, id); err != nil {
				return nil, err
			}
			next, err := agentFrom(in, actor)
			if err != nil {
				return nil, err
			}
			var out domain.Agent
			var revision int
			err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				a, err := d.Store.FindAgent(ctx, id)
				if err != nil {
					return err
				}
				if a.Status == domain.StatusRetired {
					return shared.ErrConflict.WithDetail("agents.retired")
				}
				if in.Present("expected_version") && in.Int("expected_version") != a.Version {
					return shared.ErrVersionConflict
				}
				if in.String("owner_id") == "" {
					next.OwnerID = a.OwnerID
				}
				next, err = domain.Check(next, d.Text)
				if err != nil {
					return err
				}
				if err := d.usable(ctx, next.ModelID); err != nil {
					return err
				}
				changed := a.Changed(next)
				now := d.Clock.Now()
				if err := d.Store.UpdateAgent(ctx, changed, a.Version, now); err != nil {
					return err
				}
				changed.Version = a.Version + 1
				out = changed
				if err := d.agentChanged(ctx, actor, changed); err != nil {
					return err
				}
				if revision, _, err = d.modelRevision(ctx, changed); err != nil {
					return err
				}
				return d.record(ctx, actor, ChangedAction, agentTarget, id, audit.SeverityNotice,
					open("status", string(changed.Status)), open("scopes", strings.Join(changed.Scopes, " ")))
			})
			if err != nil {
				return nil, err
			}
			return AgentOutput(out, revision), nil
		}),
	}
}

// transition is the shape of the lifecycle's use cases: read, decide, write, record.
func (d Desk) transition(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input, action audit.Action,
	authorize func(domain.Agent) error, step func(ctx context.Context, a domain.Agent, revision int) (domain.Agent, error),
) (usecase.Output, error) {
	id, err := in.ID("agent_id")
	if err != nil {
		return nil, err
	}
	a, err := d.find(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	if err := authorize(a); err != nil {
		return nil, err
	}
	var out domain.Agent
	var revision int
	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		a, err := d.Store.FindAgent(ctx, id)
		if err != nil {
			return err
		}
		revision, _, err = d.modelRevision(ctx, a)
		if err != nil {
			return err
		}
		next, err := step(ctx, a, revision)
		if err != nil {
			return err
		}
		if err := d.Store.UpdateAgent(ctx, next, a.Version, d.Clock.Now()); err != nil {
			return err
		}
		next.Version = a.Version + 1
		out = next
		if err := d.agentChanged(ctx, actor, next); err != nil {
			return err
		}
		return d.record(ctx, actor, action, agentTarget, id, audit.SeverityNotice,
			open("status", string(next.Status)), open("model_revision", strconv.Itoa(revision)))
	})
	if err != nil {
		return nil, err
	}
	return AgentOutput(out, revision), nil
}

func lifecycle(name, summary string, action audit.Action, h func(context.Context, appshared.ActorContext, usecase.Input) (usecase.Output, error)) usecase.Descriptor {
	return usecase.Descriptor{
		Name: name, Module: moduleOf(), Permission: service.PermissionManageMembers, Summary: summary,
		SideEffects: "Writes the agent and an audit entry.", TokenScope: scopeWrite,
		Input:    []usecase.Field{{Name: "agent_id", Kind: usecase.KindID, Required: true}},
		Audit:    usecase.AuditDeclaration{Action: action, TargetType: agentTarget, Severity: audit.SeverityNotice, Required: true},
		Activity: usecase.ActivityDeclaration{Exempt: "an agent is no entry."},
		Handler:  usecase.HandlerFunc(h),
	}
}

// ApproveAgent records a person's approval: of a draft, or of an agent whose model changed.
type ApproveAgent struct{ Desk Desk }

func (h ApproveAgent) Descriptor() usecase.Descriptor {
	return lifecycle(ApproveAgentName,
		"Approves an agent for its model as it now stands: a draft becomes approved, and one whose "+
			"model changed since is covered again. Its owner does not approve it, and no agent "+
			"approves one. Needs MANAGE_MEMBERS at the workspace.",
		ApprovedAction,
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			return d.transition(ctx, actor, in, ApprovedAction,
				func(a domain.Agent) error {
					if err := notAgent(actor, "agents.agent_approves_not"); err != nil {
						return err
					}
					return d.administer(ctx, actor, ApprovedAction, scopeWrite, agentTarget, a.ID)
				},
				func(_ context.Context, a domain.Agent, revision int) (domain.Agent, error) {
					return a.Approve(actor.AccountID, revision, d.Clock.Now())
				})
		})
}

// ActivateAgent puts an approved agent live, or back live after its kill switch.
type ActivateAgent struct{ Desk Desk }

func (h ActivateAgent) Descriptor() usecase.Descriptor {
	return lifecycle(ActivateAgentName,
		"Puts an approved agent live, or back live after it was suspended. Its approval must cover "+
			"its model as it now stands, and where it asks for an evaluation, the newest run on that "+
			"model must have reached its bar. No agent puts one live. Needs MANAGE_MEMBERS at the "+
			"workspace.",
		ActivatedAction,
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			return d.transition(ctx, actor, in, ActivatedAction,
				func(a domain.Agent) error {
					if err := notAgent(actor, "agents.agent_activates_not"); err != nil {
						return err
					}
					return d.administer(ctx, actor, ActivatedAction, scopeWrite, agentTarget, a.ID)
				},
				func(ctx context.Context, a domain.Agent, revision int) (domain.Agent, error) {
					runs, err := d.Store.ListRuns(ctx, a.ID, 1)
					if err != nil {
						return domain.Agent{}, err
					}
					var latest *domain.Run
					if len(runs) > 0 {
						latest = &runs[0]
					}
					return a.Activate(revision, latest)
				})
		})
}

// SuspendAgent is an agent's kill switch: it stops at once.
type SuspendAgent struct{ Desk Desk }

func (h SuspendAgent) Descriptor() usecase.Descriptor {
	return lifecycle(SuspendAgentName,
		"The kill switch of one agent: from this moment every act it attempts is refused with "+
			"agents.suspended, through every door. Its owner may pull it as well as whoever "+
			"administers agents; putting it live again is ActivateAgent.",
		SuspendedAction,
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			return d.transition(ctx, actor, in, SuspendedAction,
				func(a domain.Agent) error { return d.ownerOrAdminister(ctx, actor, a, SuspendedAction, scopeWrite) },
				func(_ context.Context, a domain.Agent, _ int) (domain.Agent, error) {
					return a.Suspend(actor.AccountID, d.Clock.Now())
				})
		})
}

// RetireAgent ends an agent for good.
type RetireAgent struct{ Desk Desk }

func (h RetireAgent) Descriptor() usecase.Descriptor {
	return lifecycle(RetireAgentName,
		"Retires an agent for good: it never acts again and is kept, with its ledger, for the record. "+
			"Its service account stays; revoke its tokens there. Needs MANAGE_MEMBERS at the workspace.",
		RetiredAction,
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			return d.transition(ctx, actor, in, RetiredAction,
				func(a domain.Agent) error {
					return d.administer(ctx, actor, RetiredAction, scopeWrite, agentTarget, a.ID)
				},
				func(_ context.Context, a domain.Agent, _ int) (domain.Agent, error) { return a.Retire() })
		})
}

// SetAgentHalt is the workspace's kill switch: every agent stops, or may go on.
type SetAgentHalt struct{ Desk Desk }

func (h SetAgentHalt) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       SetAgentHaltName,
		Module:     moduleOf(),
		Permission: service.PermissionManageMembers,
		Summary: "The workspace's kill switch for every agent at once: halted, every act any registered " +
			"agent attempts is refused with agents.halted until it is released. Needs MANAGE_MEMBERS " +
			"at the workspace.",
		SideEffects: "Writes the switch and an audit entry.",
		TokenScope:  scopeWrite,
		Input:       []usecase.Field{{Name: "halted", Kind: usecase.KindBool, Required: true}},
		Audit:       usecase.AuditDeclaration{Action: HaltSetAction, TargetType: agentTarget, Severity: audit.SeverityWarning, Required: true},
		Activity:    usecase.ActivityDeclaration{Exempt: "the switch is no entry."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			d := h.Desk
			if err := d.administer(ctx, actor, HaltSetAction, scopeWrite, agentTarget, ""); err != nil {
				return nil, err
			}
			now := d.Clock.Now()
			halted := in.Bool("halted")
			err := d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := d.Store.PutHalt(ctx, d.IDs.NewID(), haltOf(halted, actor.AccountID, now)); err != nil {
					return err
				}
				return d.record(ctx, actor, HaltSetAction, agentTarget, "", audit.SeverityWarning,
					open("halted", strconv.FormatBool(halted)))
			})
			if err != nil {
				return nil, err
			}
			return usecase.Output{"halted": halted, "set_by": actor.AccountID.String(), "set_at": now.UTC()}, nil
		}),
	}
}

// ReadAgentHalt answers the workspace's kill switch.
type ReadAgentHalt struct{ Desk Desk }

func (h ReadAgentHalt) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ReadAgentHaltName,
		Module:      moduleOf(),
		Permission:  service.PermissionManageMembers,
		Summary:     "Whether the workspace's agents are halted, by whom and since when. Needs MANAGE_MEMBERS at the workspace.",
		SideEffects: "None. Reads only.",
		TokenScope:  scopeRead,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: ReadAction, TargetType: agentTarget, Severity: audit.SeverityInfo},
		Activity:    usecase.ActivityDeclaration{Exempt: "a read changes nothing."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			d := h.Desk
			if err := d.administer(ctx, actor, ReadAction, scopeRead, agentTarget, ""); err != nil {
				return nil, err
			}
			var out usecase.Output
			err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				halt, err := d.Store.Halt(ctx)
				out = usecase.Output{"halted": halt.Halted, "set_by": idOrNil(halt.SetBy), "set_at": nil}
				if !halt.SetAt.IsZero() {
					out["set_at"] = halt.SetAt.UTC()
				}
				return err
			})
			return out, err
		}),
	}
}

// AgentOutput is an agent as every door answers it; revision is its model's revision now.
func AgentOutput(a domain.Agent, revision int) usecase.Output {
	scopes := a.Scopes
	if scopes == nil {
		scopes = []string{}
	}
	return usecase.Output{
		"id": a.ID.String(), "account_id": idOrNil(a.AccountID), "name": a.Name, "purpose": a.Purpose,
		"kind": string(a.Kind), "owner_id": idOrNil(a.OwnerID), "risk_class": string(a.RiskClass), "scopes": scopes,
		"model_id": idOrNil(a.ModelID),
		"policy":   usecase.Output{"read": string(domain.RuleAllow), "reversible": string(a.Policy.Reversible), "irreversible": string(a.Policy.Irreversible)},
		"budget": usecase.Output{
			"tokens_per_day": a.Budget.TokensPerDay, "actions_per_hour": a.Budget.ActionsPerHour,
			"concurrent_runs": a.Budget.ConcurrentRuns,
		},
		"min_accuracy": a.MinAccuracy, "status": string(a.Status), "needs_approval": a.NeedsApproval(revision),
		"approved_by": idOrNil(a.ApprovedBy), "approved_at": timeOrNil(a.ApprovedAt),
		"suspended_by": idOrNil(a.SuspendedBy), "suspended_at": timeOrNil(a.SuspendedAt),
		"created_at": a.CreatedAt.UTC(), "version": a.Version,
	}
}
