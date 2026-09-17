// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The last of ai-first.md §2's seven rows (P-11): a template drafted from a description in the
// caller's words, for the collection it would belong to.
const (
	AiGenerateTemplateName = "AiGenerateTemplate"
	templatePrompt         = "generate-template"
	// MaxTemplateDescription bounds the words a template is drafted from: the contract's
	// `TemplateGeneration.description`, and the request row's own check.
	MaxTemplateDescription = 2000
)

// TemplateAskedAction is the audit code: what was sent to a provider, and what for (ADR-0018
// decision 7). Its own code rather than the fields', because a template is asked for from words
// the person typed rather than from an entry the workspace holds.
const TemplateAskedAction audit.Action = "ai.template_asked"

// AiGenerateTemplate asks the workspace's provider for a template.
//
// It queues and answers like every other question a provider is asked - the permission, the
// consent and the budget, the read that proves the collection exists, the job, the audit entry -
// and differs in one thing: its material is the description rather than a row, so the words are
// held for the job (`Cases.Requests`) and the job's payload names them.
type AiGenerateTemplate struct {
	Cases Cases
	AI    AiAvailability
	Queue Jobs
}

// Execute checks, holds the words, then queues.
func (h AiGenerateTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, description string,
) error {
	description = strings.TrimSpace(description)
	if description == "" || len([]rune(description)) > MaxTemplateDescription {
		return shared.ErrValidation.WithDetail("suggestions.description_invalid").
			WithFields(shared.FieldError{Path: "/description", Code: "suggestions.description_invalid"})
	}
	return Ask(h).queue(ctx, actor, domain.TargetContainer, collectionID,
		TemplateAskedAction, domain.KindTemplate, templatePrompt, false, description)
}

// Descriptor is the catalogue entry.
func (h AiGenerateTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AiGenerateTemplateName,
		Summary: "Asks the workspace's AI provider to draft a template from a description in the " +
			"caller's own words, for the collection the template would belong to: a name and a " +
			"tree of nodes, each with a type, a title, optional notes and a relative due offset, " +
			"held to what the collection's capability profile permits - a node the profile " +
			"refuses is dropped from the draft rather than stored. The answer is a TEMPLATE " +
			"suggestion whose payload is a template's input; accepting it defines the template " +
			"through CreateTemplate as the accepting person, with their rights at the collection.",
		SideEffects: "Holds the description for the job that will read it, queues one question to " +
			"the provider and writes an audit entry. Defines nothing until somebody accepts.",
		TokenScope: suggestionsWrite,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true,
				Description: "The collection the template is drafted for and would be defined in."},
			{Name: "description", Kind: usecase.KindString, Required: true,
				Description: "What the template should produce, in the caller's own words. It " +
					"travels to the provider as content, never as instruction."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplateAskedAction, TargetType: suggestionTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "Asking changes nothing; the template an acceptance defines is audited as " +
				"its own act.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AiGenerateTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	collectionID, err := in.ID("collection_id")
	if err != nil {
		return nil, err
	}
	if err := h.Execute(ctx, actor, collectionID, in.String("description")); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// templateMaterial is what a template is drafted against: the collection by name, and the shape
// a template may take in this installation - which types sit directly in a collection and what
// may sit under each - so that the prompt can hold the answer to the profile and the narrowing
// can check it against the same profile.
//
// The description itself is not here: it is the request's, read by the producer beside this and
// appended as content. The digest is the collection's, the same one acceptance recomputes.
func (s CatalogueSources) templateMaterial(
	ctx context.Context, actor appshared.ActorContext, containerID shared.ID,
) (Material, error) {
	container, err := s.Catalogue.Invoke(ctx, "GetContainer", actor, usecase.Input{
		"container_id": containerID.String(),
	})
	if err != nil {
		return Material{}, err
	}
	if s.Profiles == nil {
		return Material{}, shared.ErrInternal.WithDetail("suggestions.profiles_not_wired")
	}
	rows, err := s.Profiles.List(ctx)
	if err != nil {
		return Material{}, err
	}
	// The roots are read off the system defaults, as everywhere the hierarchy is built: a
	// narrowing that took a task's children away must not thereby promote the work package.
	system, err := s.Profiles.ListSystem(ctx)
	if err != nil {
		return Material{}, err
	}
	hierarchy, err := service.NewHierarchy(rows, system)
	if err != nil {
		return Material{}, err
	}

	var written strings.Builder
	written.WriteString("Collection: " + container.String("name") + "\n")
	if description := strings.TrimSpace(container.String("description")); description != "" {
		written.WriteString(description + "\n")
	}
	written.WriteString("\nThe shape a template may take here. A root node is one of: ")
	var roots []string
	for _, row := range rows {
		if hierarchy.IsRoot(row.Type) {
			roots = append(roots, string(row.Type))
		}
	}
	written.WriteString(strings.Join(roots, ", ") + ".\nWhat may sit directly under each type:\n")
	for _, row := range rows {
		children := make([]string, 0, len(row.AllowedChildTypes))
		for _, child := range row.AllowedChildTypes {
			children = append(children, string(child))
		}
		under := "nothing"
		if len(children) > 0 {
			under = strings.Join(children, ", ")
		}
		written.WriteString("- " + string(row.Type) + ": " + under + "\n")
	}
	return Material{
		Content: written.String(), Digest: domain.Digest(container.String("name"), ""),
		Hierarchy: &hierarchy,
	}, nil
}

// maxProposedTemplateNodes bounds a draft. The prompt asks for twenty at most; this is what
// happens when a model ignores it, and it is a bound rather than a truncation for `keptTree`'s
// reason.
const maxProposedTemplateNodes = 64

// templateFrom reads a model's answer as a template's input for the target collection (P-11).
//
// The name is required and the tree is one root node the profile lets sit in a collection; a node
// of a type the profile refuses under its parent is dropped with everything under it and counted,
// so that the person reads a draft they could accept and the job records how much the model
// proposed outside the shape it was shown. A relative due offset is checked the way CreateTemplate
// checks it, and one it would refuse is left off the node rather than taking the node with it.
//
// The scope is written here and never read from the answer: a model cannot name a destination, so
// `scope_type` and `scope_id` are the target's, and the acceptance writes the target over them
// again.
func templateFrom(
	text string, hierarchy *service.Hierarchy, target shared.ID,
) (payload map[string]any, dropped int, ok bool) {
	answered, ok := objectFrom(text)
	if !ok || hierarchy == nil {
		return nil, 0, false
	}
	name, _ := answered["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > work.MaxTemplateNameLength {
		return nil, 0, false
	}
	nodes, isList := answered["nodes"].([]any)
	if !isList || len(nodes) != 1 {
		return nil, 0, false
	}
	root, isNode := nodes[0].(map[string]any)
	if !isNode {
		return nil, 0, false
	}
	rootType := work.ItemType(textOf(root["type"]))
	if !hierarchy.IsRoot(rootType) {
		return nil, 0, false
	}
	kept, count, dropped, ok := keptTemplateNode(root, hierarchy)
	if !ok || count > maxProposedTemplateNodes {
		return nil, dropped, false
	}

	payload = map[string]any{
		"scope_type": string(work.TemplateScopeCollection), "scope_id": target.String(),
		"name": name, "root_type": string(rootType), "nodes": []any{kept},
	}
	if description, held := answered["description"].(string); held && strings.TrimSpace(description) != "" {
		payload["description"] = strings.TrimSpace(description)
	}
	return payload, dropped, true
}

// keptTemplateNode reads one node and what sits under it, keeping what a node may carry and
// what the profile lets sit where the tree puts it.
func keptTemplateNode(
	node map[string]any, hierarchy *service.Hierarchy,
) (kept map[string]any, count, dropped int, ok bool) {
	kind := work.ItemType(textOf(node["type"]))
	title := strings.TrimSpace(textOf(node["title"]))
	profile, err := hierarchy.Profile(kind)
	if err != nil || title == "" {
		return nil, 0, 0, false
	}
	kept = map[string]any{"type": string(kind), "title": title}
	if notes := strings.TrimSpace(textOf(node["notes"])); notes != "" {
		kept["notes"] = notes
	}
	if offset := strings.TrimSpace(textOf(node["due_offset"])); offset != "" {
		if _, err := work.ParseTemplateOffset(offset); err == nil {
			kept["due_offset"] = offset
			kept["due_date_only"] = true
		}
	}
	count = 1

	children, _ := node["children"].([]any)
	var under []any
	for _, entry := range children {
		child, isNode := entry.(map[string]any)
		if !isNode {
			return nil, 0, 0, false
		}
		childType := work.ItemType(textOf(child["type"]))
		if !profile.AllowsChild(childType) {
			// Dropped with its subtree and counted: the profile refuses it here, and a person
			// accepting the draft would have been refused the same way with their name on it.
			dropped += 1 + nodesUnder(child)
			continue
		}
		read, n, d, ok := keptTemplateNode(child, hierarchy)
		if !ok {
			return nil, 0, 0, false
		}
		under = append(under, read)
		count += n
		dropped += d
	}
	if len(under) > 0 {
		kept["children"] = under
	}
	return kept, count, dropped, true
}

// nodesUnder counts a subtree the narrowing drops, so the count says what was lost rather than
// how many branches were cut.
func nodesUnder(node map[string]any) int {
	children, _ := node["children"].([]any)
	total := 0
	for _, entry := range children {
		if child, isNode := entry.(map[string]any); isNode {
			total += 1 + nodesUnder(child)
		}
	}
	return total
}

func textOf(value any) string {
	text, _ := value.(string)
	return text
}
