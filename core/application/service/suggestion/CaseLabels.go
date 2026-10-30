// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"

	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

// CaseLabels is how a triage answers the labels of its own evaluation set (G7): the workspace's
// classifier asked about a case's text as about an entry of the collection, as the person who
// runs the set. Nothing is recorded - an evaluation keeps no answer (UC-AGT-04 check 2).
type CaseLabels struct{ Produce Produce }

var _ agentservice.CaseLabeller = CaseLabels{}

// Labels answers the names of the collection's labels the classifier chose for a text, and the
// tokens it spent. AI switched off, consent withdrawn or the asker kept out is refused as asking
// would be.
func (c CaseLabels) Labels(
	ctx context.Context, actor appshared.ActorContext, collection shared.ID, title, notes string,
) ([]string, int64, error) {
	h := c.Produce
	prompt, err := h.Prompts.Get(classifyPrompt)
	if err != nil {
		return nil, 0, err
	}
	if actor, err = h.located(ctx, actor); err != nil {
		return nil, 0, err
	}
	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return nil, 0, err
	}
	if !provider.Capabilities().Completion {
		return nil, 0, aiprovider.RefusalOf(provider)
	}
	var material Material
	err = h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		material, err = h.Sources.Material(ctx, actor, Question{
			TargetType: domain.TargetWorkItem, PromptID: classifyPrompt,
			Case: &CaseText{CollectionID: collection, Title: title, Notes: notes},
		})
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	if refused := material.Kept.Refusal(actor.AccountID); refused != nil {
		return nil, 0, refused
	}
	if strings.TrimSpace(material.Content) == "" {
		return []string{}, 0, nil
	}
	answer, err := provider.Complete(ctx, prompt.Ask(withOptions(material, h.today(actor, classifyPrompt))))
	if err != nil {
		return nil, 0, err
	}
	tokens := int64(answer.Usage.InputTokens + answer.Usage.OutputTokens)
	payload, _ := payloadFrom(domain.KindFields, classifyPrompt, answer.Text,
		map[string]bool{labelsKey: true}, material, h.Text)
	names := map[string]string{}
	for _, set := range material.Choices {
		if set.Key == labelsKey {
			for _, o := range set.Options {
				names[o.ID] = o.Name
			}
		}
	}
	labels := []string{}
	chosen, _ := payload[labelsKey].([]any)
	if one, ok := payload[labelsKey].(string); ok {
		chosen = []any{one}
	}
	for _, id := range chosen {
		if name, ok := names[id.(string)]; ok {
			labels = append(labels, name)
		}
	}
	return labels, tokens, nil
}
