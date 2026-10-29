// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"

	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/entitlement"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

// DescribeRuleName is the catalogue name.
const DescribeRuleName = "DescribeRule"

// RuleAskedAction records that somebody asked for a rule to be drafted: what was sent to a provider
// and what for (ADR-0018 decision 7), with the scope and never the words.
const RuleAskedAction audit.Action = "ai.rule_asked"

// ruleTarget is what the asking's audit entry is about: a rule to be, at the scope it was asked
// for.
const ruleTarget = "automation_rule"

// DescribeRule asks the workspace's provider to draft a rule from a sentence (UC-AUT-10).
//
// It asks what writing a rule asks - the automation permission at the scope, and a plan that
// includes rules - because a draft names configuration that only somebody who may write it should
// see (automation.md §2.1). What comes back is a RULE suggestion about the asker's own account:
// theirs to read, to save switched off, or to dismiss.
type DescribeRule struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute checks, holds the words, then queues.
func (h DescribeRule) Execute(
	ctx context.Context, actor appshared.ActorContext, scope automation.Scope, description string,
) (shared.ID, error) {
	description = strings.TrimSpace(description)
	if description == "" || len([]rune(description)) > MaxTemplateDescription {
		return shared.ID(""), shared.ErrValidation.WithDetail("suggestions.description_invalid").
			WithFields(shared.FieldError{Path: "/description", Code: "suggestions.description_invalid"})
	}
	scope, err := automation.ValidScope(scope)
	if err != nil {
		return shared.ID(""), err
	}
	if scope.Type != automation.ScopeTenant {
		// A hub or a collection named is one the asker can see, or the answer is that there is no
		// such container - before any permission is asked about it.
		if _, err := h.Cases.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{
			"container_id": scope.ID.String(),
		}); err != nil {
			return shared.ID(""), err
		}
	}
	return Ask(h).queue(ctx, actor, asking{
		TargetType: domain.TargetAccount, TargetID: actor.AccountID,
		Action: RuleAskedAction, Kind: domain.KindRule, PromptID: draftRulePrompt,
		Text: description, Scope: scope,
		Access: &access.Request{
			Permission: service.PermissionAutomation,
			Path:       scope.Path(),
			TokenScope: automationScope,
			TargetType: ruleTarget,
			TargetID:   scope.ID,
		},
		JobKind: queue.KindAiDraftRule, Attempts: queue.AiDraftRuleAttempts,
	})
}

// Descriptor is the catalogue entry.
func (h DescribeRule) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        DescribeRuleName,
		Permission:  service.PermissionAutomation,
		Entitlement: entitlement.AutomationRules,
		Summary: "Asks the workspace's AI provider to draft an automation rule from one sentence " +
			"in the caller's own words. The draft uses only the triggers the workspace serves, " +
			"conditions written as the rule editor's sentences, a fixed set of entry-level " +
			"actions, and only the people, collections and labels the sentence names and the " +
			"caller may read; what it could not draft is listed. It is a RULE suggestion about " +
			"the caller's own account, which only they read; accepting it is CreateRule as the " +
			"caller, with a name and the account it runs as, so the rule is stored switched off.",
		SideEffects: "Holds the sentence for the job that will read it, queues one question to the " +
			"provider and writes an audit entry. Writes no rule until somebody accepts.",
		TokenScope: automationScope,
		Input: []usecase.Field{
			{Name: "scope", Kind: usecase.KindObject, Required: true,
				Description: "Where the rule would apply: {type: TENANT|HUB|COLLECTION, id}. The " +
					"draft may narrow it to a hub or collection the sentence names."},
			{Name: "description", Kind: usecase.KindString, Required: true,
				Description: "What the rule should do, in the caller's own words. It travels to the " +
					"provider as content, never as instruction."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleAskedAction, TargetType: ruleTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes nothing; the rule an acceptance writes is audited as its own act.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h DescribeRule) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scope := automation.Scope{}
	if document, held := in["scope"].(map[string]any); held {
		kind, _ := document["type"].(string)
		scope.Type = automation.ScopeType(kind)
		if raw, named := document["id"].(string); named && raw != "" {
			id, err := shared.ParseID(raw)
			if err != nil {
				return nil, shared.ErrValidation.WithDetail("usecase.field_type_invalid").
					WithFields(shared.FieldError{Path: "/scope/id", Code: "usecase.field_type_invalid"})
			}
			scope.ID = id
		}
	}
	jobID, err := h.Execute(ctx, actor, scope, in.String("description"))
	if err != nil {
		return nil, err
	}
	return queued(jobID), nil
}
