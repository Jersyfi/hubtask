// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
)

// heldValues answers a fixed count for every definition, and remembers the ones it was asked about.
type heldValues struct {
	count int
	asked []string
}

func (h *heldValues) EntriesHolding(_ context.Context, definition domain.CustomFieldDefinition) (int, error) {
	h.asked = append(h.asked, definition.Key)
	return h.count, nil
}

// A plan's reach for a field it removes or hides from readers: how many entries hold a value
// there, asked as a change of the field is - a refusal says nothing of the entries, and a field
// out of use already shows nothing that could stop showing.
func TestAFieldsReachCountsTheEntriesHoldingIt(t *testing.T) {
	h := newCustomFieldHarness(t)
	defined := h.defined(t, DefineCustomFieldCommand{
		CollectionID: collectionID, Key: "severity", Kind: domain.CustomFieldText,
	})
	counts := &heldValues{count: 7}
	preview := PreviewCustomFieldReach{Writer: h.writer(), Counts: counts}

	entries, err := preview.Execute(t.Context(), actor(), defined.ID)
	if err != nil || entries != 7 || len(counts.asked) != 1 || counts.asked[0] != "severity" {
		t.Fatalf("entries %d, asked %v: %v", entries, counts.asked, err)
	}

	if err := (DeleteCustomField{Writer: h.writer()}).Execute(t.Context(), actor(), DeleteCustomFieldCommand{FieldID: defined.ID}); err != nil {
		t.Fatal(err)
	}
	if entries, err := preview.Execute(t.Context(), actor(), defined.ID); err != nil || entries != 0 || len(counts.asked) != 1 {
		t.Fatalf("a field out of use counted %d (%v)", entries, err)
	}

	h.authorizer.err = shared.ErrForbidden.WithDetail("access.not_permitted")
	if _, err := preview.Execute(t.Context(), actor(), defined.ID); !errors.Is(err, shared.ErrForbidden) || len(counts.asked) != 1 {
		t.Fatalf("a refused reader was counted for: %v", err)
	}
}
