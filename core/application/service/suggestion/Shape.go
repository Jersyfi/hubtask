// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// Shape is what a collection offers its entries - its levels, and each level's fields - read from
// its structure in force as the container's read answers it (domain-model.md §2). A proposal is
// held to it so that what a person reads is what accepting it may write: a piece at a level the
// collection hides, or a value it hides, would be refused at acceptance.
type Shape struct {
	levels map[work.ItemType]bool
	fields map[work.ItemType]map[work.Capability]bool
}

// Offers reports whether the collection offers the level. Without a shape - a container read that
// carried no structure - every level is offered, which is the behaviour from before structures.
func (s *Shape) Offers(itemType work.ItemType) bool {
	return s == nil || s.levels[itemType]
}

// Carries reports whether the collection offers the capability on the level. A level its structure
// leaves out carries everything but what is opt-in - an amount is carried only where a layer names
// it (Capability.OptIn, domain-model.md §2).
func (s *Shape) Carries(itemType work.ItemType, capability work.Capability) bool {
	if s == nil {
		return !capability.OptIn()
	}
	fields, decided := s.fields[itemType]
	if !decided {
		return !capability.OptIn()
	}
	return fields[capability]
}

// collectionShape reads a collection's shape through its own read, as the person asking: nil where the read
// carries no structure.
func (s CatalogueSources) collectionShape(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
) (*Shape, error) {
	if containerID.IsZero() {
		return nil, nil
	}
	out, err := s.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{
		"container_id": containerID.String(),
	})
	if err != nil {
		return nil, err
	}
	return shapeFrom(out), nil
}

// shapeOf is the shape of the collection a conversion names (`collection_id`), read as the person
// accepting. Unknown - no collection named, or a read that failed - is nil, which offers every
// level: the create that follows is then what answers, as it would have without a shape.
func shapeOf(ctx context.Context, catalogue Catalogue, actor appshared.ActorContext, in usecase.Input) *Shape {
	named, _ := in["collection_id"].(string)
	containerID, err := shared.ParseID(named)
	if err != nil {
		return nil
	}
	out, err := catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{"container_id": containerID.String()})
	if err != nil {
		return nil
	}
	return shapeFrom(out)
}

// shapeFrom reads the levels and fields out of a container's answer.
func shapeFrom(container usecase.Output) *Shape {
	structure, held := container["structure"].(map[string]any)
	if !held {
		return nil
	}
	settings, _ := structure["settings"].(map[string]any)
	levels, _ := settings["levels"].([]any)
	if len(levels) == 0 {
		return nil
	}
	shape := &Shape{levels: map[work.ItemType]bool{}, fields: map[work.ItemType]map[work.Capability]bool{}}
	for _, level := range levels {
		entry, _ := level.(map[string]any)
		if itemType, isText := entry["type"].(string); isText {
			shape.levels[work.ItemType(itemType)] = true
		}
	}
	fields, _ := settings["fields"].(map[string]any)
	for itemType, value := range fields {
		entry, _ := value.(map[string]any)
		carried := map[work.Capability]bool{}
		for _, capability := range textsOf(entry["capabilities"]) {
			carried[work.Capability(capability)] = true
		}
		shape.fields[work.ItemType(itemType)] = carried
	}
	return shape
}

// hiddenOf reads the capabilities an entry's read marks hidden (WorkItem.hidden).
func hiddenOf(item usecase.Output) map[work.Capability]bool {
	hidden := map[work.Capability]bool{}
	mark, held := item["hidden"].(map[string]any)
	if !held {
		return hidden
	}
	for _, capability := range textsOf(mark["capabilities"]) {
		hidden[work.Capability(capability)] = true
	}
	return hidden
}

// textsOf reads a list of strings as an in-process answer or a decoded document carries it.
func textsOf(value any) []string {
	switch list := value.(type) {
	case []string:
		return list
	case []any:
		texts := make([]string, 0, len(list))
		for _, entry := range list {
			if text, isText := entry.(string); isText {
				texts = append(texts, text)
			}
		}
		return texts
	}
	return nil
}

// hiddenKeys are the answer keys of a fields proposal and the capability each writes: a key whose
// capability the entry's collection hides is not proposed, because accepting it would be refused.
var hiddenKeys = map[string]work.Capability{
	notesKey:  work.CapabilityNotes,
	dueKey:    work.CapabilityDueDate,
	bucketKey: work.CapabilityBucket,
	labelsKey: work.CapabilityLabels,
	fieldsKey: work.CapabilityCustomFields,
}

// withoutHidden drops the keys of a proposal whose capability is hidden.
func withoutHidden(payload map[string]any, hidden map[work.Capability]bool) map[string]any {
	for key, capability := range hiddenKeys {
		if hidden[capability] {
			delete(payload, key)
		}
	}
	return payload
}
