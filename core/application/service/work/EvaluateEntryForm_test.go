// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

func evaluation(t *testing.T, form domain.Form, compiler *answers) (*itemHarness, *usecase.Registry) {
	t.Helper()
	h := newItemHarness()
	evaluate := EvaluateEntryForm{
		Items: h.items, Containers: h.containers, Authorizer: h.authorizer, UnitOfWork: h.uow,
		Gate: EntryGate{Forms: FormGate{Types: formedTypes(form), Compiler: compiler}},
	}
	registry, err := usecase.NewRegistry(nil, evaluate.Descriptor())
	if err != nil {
		t.Fatalf("the catalogue refused the entry: %v", err)
	}
	return h, registry
}

// UC-FLW-01 check 2: a type's form answers which of its fields are required and which
// validations a draft does not meet - for a new entry and for one being edited - and nothing is
// written.
func TestTheFormAnswersHowADraftStands(t *testing.T) {
	compiler := &answers{of: map[string]bool{"short": false}}
	h, doors := evaluation(t, domain.Form{
		Fields:      []domain.FormField{{Key: "title"}, {Key: "custom_fields.severity", Required: true}},
		Validations: []domain.FormValidation{{Key: "short_title", Field: "title", Holds: "short"}},
	}, compiler)
	ctx := context.Background()

	fresh, err := doors.Invoke(ctx, EvaluateEntryFormName, itemActor(), usecase.Input{
		"collection_id": collectionID.String(), "type": "TASK", "title": "A very long title",
	})
	if err != nil {
		t.Fatalf("evaluating a new entry: %v", err)
	}
	severity, _ := fresh["fields"].(map[string]any)["custom_fields.severity"].(map[string]any)
	if severity["required"] != true {
		t.Fatalf("the required field is not named: %v", fresh["fields"])
	}
	if failed := fresh["failed"].([]string); len(failed) != 1 || failed[0] != "short_title" {
		t.Fatalf("the failed validation: %v", fresh["failed"])
	}

	task := h.withTask()
	compiler.of["short"] = true
	edited, err := doors.Invoke(ctx, EvaluateEntryFormName, itemActor(), usecase.Input{
		"item_id": task.ID.String(), "custom_fields": map[string]any{"severity": "high"},
	})
	if err != nil {
		t.Fatalf("evaluating an edit: %v", err)
	}
	if failed := edited["failed"].([]string); len(failed) != 0 {
		t.Fatalf("a draft meeting the form failed %v", failed)
	}
	if len(h.changes.recorded) != 0 || len(h.audit.entries) != 0 || h.items.stored[task.ID].Version != task.Version {
		t.Fatal("an evaluation wrote something")
	}
}

// A draft without a place, or of a level that does not exist, is refused; whoever may not read
// the collection is refused before the form is asked (rule 2).
func TestTheFormIsAskedOnlyForAReadableDraft(t *testing.T) {
	h, doors := evaluation(t, domain.Form{}, &answers{})
	ctx := context.Background()

	if _, err := doors.Invoke(ctx, EvaluateEntryFormName, itemActor(), usecase.Input{"type": "TASK"}); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "items.collection_or_parent_required" {
		t.Fatalf("a draft without a collection: %v", err)
	}
	if _, err := doors.Invoke(ctx, EvaluateEntryFormName, itemActor(), usecase.Input{
		"collection_id": collectionID.String(), "type": "EPIC",
	}); shared.AsError(err) == nil || shared.AsError(err).DetailCode != "items.type_unknown" {
		t.Fatalf("a level that does not exist: %v", err)
	}

	h.authorizer.err = shared.ErrForbidden
	if _, err := doors.Invoke(ctx, EvaluateEntryFormName, itemActor(), usecase.Input{
		"collection_id": collectionID.String(), "type": "TASK",
	}); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("a collection out of reach: %v", err)
	}
}
