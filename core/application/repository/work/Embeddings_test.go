// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// The refusal names the model and the two widths, because the model is a configuration somebody
// chose and the answer should say what to choose instead (ADR-0054).
func TestTheRefusalForAWideVectorNamesTheModelAndBothWidths(t *testing.T) {
	err := EmbeddingTooWide("embed-wide", EmbeddingWidth+512)

	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("the refusal is %v, want a validation error", err)
	}
	refusal := shared.AsError(err)
	if refusal.Params["model"] != "embed-wide" ||
		refusal.Params["dimensions"] != "2048" || refusal.Params["width"] != "1536" {
		t.Errorf("the refusal carries %v", refusal.Params)
	}
	if !IsEmbeddingTooWide(err) {
		t.Error("the predicate does not recognise its own refusal")
	}
}

// The predicate reads the detail code, not the category: `Is` on the typed error would match
// every validation error, and a caller finishing a job on that would finish it for a malformed
// title too.
func TestThePredicateDoesNotMistakeAnotherValidationErrorForTheRefusal(t *testing.T) {
	if IsEmbeddingTooWide(shared.ErrValidation.WithDetail("items.title_too_long")) {
		t.Error("another validation error was taken for the width refusal")
	}
	if IsEmbeddingTooWide(nil) {
		t.Error("nil was taken for the width refusal")
	}
	if IsEmbeddingTooWide(errors.New("plain")) {
		t.Error("a plain error was taken for the width refusal")
	}
}

// An empty vector is a provider defect, not a configuration, so it is internal rather than
// validation - and it is not the width refusal, which a job finishes on.
func TestAnEmptyVectorIsItsOwnRefusal(t *testing.T) {
	err := EmbeddingEmpty("embed-3")
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("the refusal is %v, want an internal error", err)
	}
	if IsEmbeddingTooWide(err) {
		t.Error("an empty vector was taken for a wide one")
	}
}
