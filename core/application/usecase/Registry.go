// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package usecase is the catalogue: every use case the system has, once.
//
// The point is parity. A feature is registered here, and it is thereby reachable through REST,
// through MCP, and as an automation action - not because three teams remembered, but because the
// three channels are built from this one list (arc42 §4, ADR-0012, automation.md §1.3). The
// alternative, which every system that grew a second interface has lived through, is an API that
// can do things the agent interface cannot and an automation engine that lags both.
//
// The three channel identities are derived from the use case name rather than declared. A name
// that could be spelled differently per channel is a name that eventually is - and then the
// parity test compares two lists that were never the same list.
package usecase

import (
	"context"
	"slices"
	"strings"
	"unicode"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/activity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// Handler executes one use case. Every channel calls this, so the authorisation check, the audit
// entry, the event and the change log happen once, in the application layer, whichever way the
// call arrived (audit.md §7, test AT-6).
type Handler interface {
	Invoke(ctx context.Context, actor appshared.ActorContext, in Input) (Output, error)
}

// HandlerFunc adapts a function to Handler.
type HandlerFunc func(context.Context, appshared.ActorContext, Input) (Output, error)

func (f HandlerFunc) Invoke(ctx context.Context, actor appshared.ActorContext, in Input) (Output, error) {
	return f(ctx, actor, in)
}

// Middleware wraps every handler on the way into the registry. It is how observability becomes
// structural rather than remembered: the composition root passes one that produces the metric and
// the span, and no entry in the catalogue can then be executed without them (gate RT-12).
type Middleware func(descriptor Descriptor, next Handler) Handler

// AuditDeclaration is what the trail records for this use case (audit.md §7).
//
// A use case with security or privacy relevance and no declaration fails the build (gate SG-13),
// which is why Required is stated rather than inferred: "does this matter to an auditor" is a
// judgement, and one that has to be made deliberately per use case.
type AuditDeclaration struct {
	Action     audit.Action
	TargetType string
	Severity   audit.Severity
	Required   bool
}

// ActivityDeclaration is what the item's own history records for this use case
// (domain-model.md §3.5, audit.md §1).
//
// Every work-management use case that writes declares one of the two, and the architecture gate
// refuses a third state. A verb is what the history records; an exemption is a reason, in prose,
// that this change leaves no step in an item's history - and it is a field rather than a list in a
// test file because the reason belongs where the use case is, next to the person adding the next
// one.
type ActivityDeclaration struct {
	Verb activity.Verb
	// Exempt is why this use case writes no history entry. Empty where it writes one.
	Exempt string
}

// Descriptor is one entry of the catalogue.
type Descriptor struct {
	// Name is the stable use case name in PascalCase, as the catalogue in domain-model.md §5
	// writes it: CreateContainer. Every channel identity is derived from it.
	Name string
	// Summary and SideEffects are what an agent reads before deciding to call the tool
	// (ai-first.md §1.1). English prose, and protocol documentation rather than display text -
	// like the descriptions in api/openapi.yaml, it never reaches an end user's screen.
	Summary     string
	SideEffects string
	// TokenScope is the scope a credential needs (api-guidelines.md §7). Empty for an operation
	// that needs none.
	TokenScope string
	// ReadOnly and Destructive become the MCP annotations `readOnlyHint` and `destructiveHint`,
	// so that an agent client can ask for confirmation before the dangerous ones (ai-first.md).
	ReadOnly    bool
	Destructive bool

	Input    []Field
	Audit    AuditDeclaration
	Activity ActivityDeclaration
	Handler  Handler
}

// RESTOperation is the operationId in api/openapi.yaml: the name in lowerCamelCase.
func (d Descriptor) RESTOperation() string { return lowerFirst(d.Name) }

// MCPTool is the tool name: the name in snake_case (ai-first.md §1.1).
func (d Descriptor) MCPTool() string { return snakeCase(d.Name) }

// AutomationAction is the action kind of the rule engine: the name in SCREAMING_SNAKE_CASE
// (automation.md §1.3).
func (d Descriptor) AutomationAction() string { return strings.ToUpper(snakeCase(d.Name)) }

// ValidateInput checks an input against the declared fields. The registry calls it before every
// invocation, so a handler can trust the shape of what it is given.
func (d Descriptor) ValidateInput(in Input) error { return validate(d.Input, in) }

// Registry is the catalogue itself: immutable once built, because a catalogue that can grow at
// run time is one where the parity test proves nothing about what is actually running.
type Registry struct {
	descriptors map[string]Descriptor
	order       []string
}

// NewRegistry builds the catalogue and refuses an incomplete entry.
//
// The refusal happens at startup, in the composition root, so a use case missing its audit
// declaration or its handler stops the process rather than being discovered by whoever needed the
// audit entry (fail closed, ADR-0015).
func NewRegistry(observe Middleware, descriptors ...Descriptor) (*Registry, error) {
	registry := &Registry{descriptors: make(map[string]Descriptor, len(descriptors))}

	for _, descriptor := range descriptors {
		if err := check(descriptor); err != nil {
			return nil, err
		}
		if _, duplicate := registry.descriptors[descriptor.Name]; duplicate {
			return nil, shared.ErrInternal.
				WithDetail("usecase.duplicate_registration").
				WithParams(map[string]string{"use_case": descriptor.Name})
		}
		if observe != nil {
			descriptor.Handler = observe(descriptor, descriptor.Handler)
		}
		registry.descriptors[descriptor.Name] = descriptor
		registry.order = append(registry.order, descriptor.Name)
	}

	slices.Sort(registry.order)
	return registry, nil
}

func check(d Descriptor) error {
	incomplete := func(reason string) error {
		return shared.ErrInternal.
			WithDetail("usecase.registration_incomplete").
			WithParams(map[string]string{"use_case": d.Name, "missing": reason})
	}

	switch {
	case d.Name == "" || !isPascalCase(d.Name):
		return incomplete("name")
	case strings.TrimSpace(d.Summary) == "":
		// An MCP tool without a description is a tool an agent has to guess at, and a guessing
		// agent is what the whole capability manifest exists to avoid (ai-first.md §1.1).
		return incomplete("summary")
	case d.Handler == nil:
		return incomplete("handler")
	case d.Audit.Required && (d.Audit.Action == "" || d.Audit.TargetType == ""):
		// Gate SG-13, at the earliest moment it can be checked.
		return incomplete("audit")
	case d.Activity.Verb != "" && d.Activity.Exempt != "":
		// A use case that both writes a history entry and explains why it writes none has had two
		// people answer the same question. Which of the two is true is not for the registry to
		// guess, so it refuses at startup rather than picking one.
		return incomplete("activity")
	case d.Activity.Verb != "" && !d.Activity.Verb.Valid():
		return incomplete("activity verb " + string(d.Activity.Verb))
	}

	seen := make(map[string]bool, len(d.Input))
	for _, field := range d.Input {
		if field.Name == "" || seen[field.Name] {
			return incomplete("input field " + field.Name)
		}
		seen[field.Name] = true
	}
	return nil
}

// All returns every entry, in a stable order.
func (r *Registry) All() []Descriptor {
	all := make([]Descriptor, 0, len(r.order))
	for _, name := range r.order {
		all = append(all, r.descriptors[name])
	}
	return all
}

// Lookup finds an entry by its use case name.
func (r *Registry) Lookup(name string) (Descriptor, bool) {
	descriptor, found := r.descriptors[name]
	return descriptor, found
}

// ByMCPTool finds the entry an MCP tool call names.
func (r *Registry) ByMCPTool(tool string) (Descriptor, bool) { return r.by(tool, Descriptor.MCPTool) }

// ByAutomationAction finds the entry an automation action names.
func (r *Registry) ByAutomationAction(kind string) (Descriptor, bool) {
	return r.by(kind, Descriptor.AutomationAction)
}

func (r *Registry) by(identity string, of func(Descriptor) string) (Descriptor, bool) {
	for _, name := range r.order {
		if descriptor := r.descriptors[name]; of(descriptor) == identity {
			return descriptor, true
		}
	}
	return Descriptor{}, false
}

// Invoke runs a use case by name, after checking the input against its declaration.
//
// A name that is not in the catalogue is a not-found rather than an internal error: for MCP and
// automation it comes from a client naming a tool or an action, and a client's mistake is not the
// server's defect.
func (r *Registry) Invoke(ctx context.Context, name string, actor appshared.ActorContext, in Input) (Output, error) {
	descriptor, found := r.Lookup(name)
	if !found {
		return nil, shared.ErrNotFound.
			WithDetail("usecase.unknown").
			WithParams(map[string]string{"use_case": name})
	}
	if err := descriptor.ValidateInput(in); err != nil {
		return nil, err
	}
	return descriptor.Handler.Invoke(ctx, actor, in)
}

func lowerFirst(name string) string {
	if name == "" {
		return ""
	}
	return strings.ToLower(name[:1]) + name[1:]
}

// snakeCase turns CreateContainer into create_container. It handles runs of capitals, so that
// SearchItemsAI does not become search_items_a_i.
func snakeCase(name string) string {
	var out strings.Builder
	runes := []rune(name)

	for i, r := range runes {
		if unicode.IsUpper(r) {
			previousIsLower := i > 0 && unicode.IsLower(runes[i-1])
			nextIsLower := i+1 < len(runes) && unicode.IsLower(runes[i+1])
			if i > 0 && (previousIsLower || nextIsLower) {
				out.WriteByte('_')
			}
			out.WriteRune(unicode.ToLower(r))
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

func isPascalCase(name string) bool {
	if !unicode.IsUpper(rune(name[0])) {
		return false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}
