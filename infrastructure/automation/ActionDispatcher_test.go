// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"slices"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type catalogue struct {
	descriptors []usecase.Descriptor
	invokedName string
	invokedIn   usecase.Input
	invokedBy   appshared.ActorContext
	out         usecase.Output
	err         error
}

func (c *catalogue) All() []usecase.Descriptor { return c.descriptors }

func (c *catalogue) ByAutomationAction(kind string) (usecase.Descriptor, bool) {
	for _, descriptor := range c.descriptors {
		if descriptor.AutomationAction() == kind {
			return descriptor, true
		}
	}
	return usecase.Descriptor{}, false
}

func (c *catalogue) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	c.invokedName, c.invokedBy, c.invokedIn = name, actor, in
	return c.out, c.err
}

func store() *catalogue {
	return &catalogue{
		descriptors: []usecase.Descriptor{
			{Name: "CreateContainer", Summary: "Creates a hub or a collection."},
			{Name: "CompleteWorkItem", Summary: "Marks an item as done."},
		},
		out: usecase.Output{"id": "0192f000-0000-7000-8000-00000000000b"},
	}
}

func serviceAccount() appshared.ActorContext {
	return appshared.ActorContext{
		Kind:      appshared.ActorAutomation,
		TenantID:  shared.MustParseID("0192f000-0000-7000-8000-00000000000a"),
		AccountID: shared.MustParseID("0192f000-0000-7000-8000-00000000000d"),
		Scopes:    []string{"containers:write"},
	}
}

// The list of actions is the catalogue, which is what "the list grows automatically with the
// catalogue" means in practice (automation.md §1.3).
func TestTheActionsAreTheCatalogue(t *testing.T) {
	actions := NewActionDispatcher(store()).Actions()

	if !slices.Contains(actions, "CREATE_CONTAINER") || !slices.Contains(actions, "COMPLETE_WORK_ITEM") {
		t.Errorf("actions = %v", actions)
	}
	if len(actions) != 2 {
		t.Errorf("%d actions for two use cases", len(actions))
	}
}

// An action is a use case call as the rule's own account - which is what makes a rule unable to
// do more than the account it runs as.
func TestAnActionRunsTheUseCaseAsTheRulesAccount(t *testing.T) {
	catalogue := store()

	out, err := NewActionDispatcher(catalogue).Dispatch(context.Background(), serviceAccount(),
		Action{Kind: "CREATE_CONTAINER", Params: map[string]any{"type": "COLLECTION", "name": "Escalations"}})
	if err != nil {
		t.Fatalf("the action failed: %v", err)
	}

	if catalogue.invokedName != "CreateContainer" {
		t.Errorf("the catalogue was asked for %q", catalogue.invokedName)
	}
	if catalogue.invokedBy.Kind != appshared.ActorAutomation || catalogue.invokedBy.AccountID.IsZero() {
		t.Errorf("the rule's account did not reach the use case: %+v", catalogue.invokedBy)
	}
	if catalogue.invokedIn["name"] != "Escalations" {
		t.Errorf("the parameters did not arrive: %v", catalogue.invokedIn)
	}
	if out["id"] == nil {
		t.Errorf("the result did not come back: %v", out)
	}
}

// A rule naming an action this installation does not have is a rule to correct, not a defect of
// the server.
func TestAnUnknownActionIsAValidationError(t *testing.T) {
	_, err := NewActionDispatcher(store()).Dispatch(context.Background(), serviceAccount(),
		Action{Kind: "DELETE_EVERYTHING"})

	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want a validation error", err)
	}
	if code := shared.AsError(err).DetailCode; code != "automation.action_unknown" {
		t.Errorf("detail code %s, want automation.action_unknown", code)
	}
}

// Fail closed: a rule whose run_as account is unusable runs as nobody, and nobody has no tenant.
func TestARuleWithoutAUsableAccountIsRefused(t *testing.T) {
	catalogue := store()

	_, err := NewActionDispatcher(catalogue).Dispatch(context.Background(),
		appshared.Anonymous("en", "UTC"), Action{Kind: "CREATE_CONTAINER"})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want forbidden", err)
	}
	if catalogue.invokedName != "" {
		t.Error("the use case ran without an account")
	}
}

// The dispatcher grants nothing: a refusal from the application layer reaches the rule unchanged,
// so a rule cannot do more than its account may.
func TestARefusalReachesTheRule(t *testing.T) {
	catalogue := store()
	catalogue.err = shared.ErrForbidden.WithDetail("access.not_permitted")

	_, err := NewActionDispatcher(catalogue).Dispatch(context.Background(), serviceAccount(),
		Action{Kind: "CREATE_CONTAINER", Params: map[string]any{"type": "HUB", "name": "Private"}})

	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want the refusal to reach the caller", err)
	}
	if code := shared.AsError(err).DetailCode; code != "access.not_permitted" {
		t.Errorf("detail code %s - the dispatcher rewrote the refusal", code)
	}
}
