// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// UC-AGT-04 check 8: a case's text is classified against the collection's labels as an entry
// would be, the chosen labels answered by name, and nothing is recorded.
func TestACaseIsLabelledFromTheCollectionsVocabulary(t *testing.T) {
	produce, world := producer(`{"label_ids":["` + homeLabel + `","invented"]}`)
	world.labels = vocabulary()
	collection := shared.MustParseID("0192f000-0000-7000-8000-0000000000c1")

	labels, tokens, err := CaseLabels{Produce: produce}.Labels(context.Background(), person(), collection,
		"The kitchen tap drips", "Since Monday.")
	if err != nil {
		t.Fatalf("labelling: %v", err)
	}
	if len(labels) != 1 || labels[0] != "home" || tokens < 0 {
		t.Fatalf("labels = %v", labels)
	}
	shown := world.asked[0].Messages[1].Content
	if !strings.Contains(shown, "The kitchen tap drips") || !strings.Contains(shown, "Labels this collection uses") {
		t.Fatalf("the material is:\n%s", shown)
	}
	if len(world.store.proposals) != 0 {
		t.Fatalf("an evaluation recorded %d suggestions", len(world.store.proposals))
	}
	for _, p := range world.performed {
		if p.name == "ListLabels" && p.in.String("collection_id") != collection.String() {
			t.Fatalf("read the labels of %v", p.in)
		}
	}
}

// AI switched off answers the refusal asking would give, not an empty set that fails each case.
func TestACaseIsNotLabelledWhereAIIsOff(t *testing.T) {
	produce, world := producer(`{}`)
	world.completion = false
	if _, _, err := (CaseLabels{Produce: produce}).Labels(context.Background(), person(),
		shared.MustParseID("0192f000-0000-7000-8000-0000000000c1"), "A title", ""); err == nil {
		t.Fatal("labelled with AI switched off")
	}
}
