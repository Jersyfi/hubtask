// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
)

// classifyPrompt is the workspace's classifier: the question AiClassify asks.
const classifyPrompt = "classify"

// AgentProposals is how a shipped agent asks the classifier (G11): the question a person's
// AiClassify queues, asked at once as the agent and never applied - what it proposes is a
// suggestion a person accepts, with the agent's provenance kept when they do.
type AgentProposals struct{ Produce Produce }

var _ agentservice.Proposer = AgentProposals{}

// Classify asks for an entry's labels.
func (p AgentProposals) Classify(ctx context.Context, actor appshared.ActorContext, item shared.ID) (agentservice.Proposal, error) {
	outcome, err := p.Produce.Ask(ctx, actor, Request{
		TargetType: domain.TargetWorkItem, TargetID: item, Kind: domain.KindFields, PromptID: classifyPrompt,
	})
	if IsUnavailable(err) {
		// AI switched off, consent withdrawn or the workspace's budget spent: the triage still
		// proposes what needs no model.
		return agentservice.Proposal{Code: shared.AsError(err).DetailCode}, nil
	}
	if err != nil {
		return agentservice.Proposal{}, err
	}
	return agentservice.Proposal{Recorded: outcome.Recorded, Code: outcome.Code, Tokens: outcome.Tokens}, nil
}
