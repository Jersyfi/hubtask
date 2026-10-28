// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import (
	"context"
	"errors"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A signed-in caller reads the registry itself: every definition, in its order, with its levels,
// writers and preview, so a screen and the settings search draw from one list.
func TestTheCatalogueIsReadAsDeclared(t *testing.T) {
	actor := appshared.ActorContext{Kind: shared.ActorUser, TenantID: shared.ID("00000000-0000-4000-8000-000000000001")}
	out, err := ReadSettingsCatalogue{}.Descriptor().Handler.Invoke(context.Background(), actor, usecase.Input{})
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	rows, _ := out["settings"].([]usecase.Output)
	definitions := catalogue.Definitions()
	if len(rows) != len(definitions) {
		t.Fatalf("%d rows for %d definitions", len(rows), len(definitions))
	}
	previews := 0
	for index, row := range rows {
		definition := definitions[index]
		if row.String("key") != string(definition.Key) {
			t.Fatalf("row %d is %s, declared %s", index, row.String("key"), definition.Key)
		}
		levels, _ := row["levels"].([]string)
		writers, _ := row["writers"].([]usecase.Output)
		if len(levels) != len(definition.Levels) || len(writers) != len(definition.Levels) {
			t.Errorf("%s: %d levels and %d writers for %d declared levels",
				definition.Key, len(levels), len(writers), len(definition.Levels))
		}
		if preview := row.String("preview"); preview != "" {
			previews++
			if preview != "previewContainerStructure" {
				t.Errorf("%s previews through %q", definition.Key, preview)
			}
		}
	}
	if previews == 0 {
		t.Error("no setting names its preview")
	}
}

func TestTheCatalogueNeedsASignedInCaller(t *testing.T) {
	_, err := ReadSettingsCatalogue{}.Execute(context.Background(), appshared.Anonymous("en", "UTC"))
	if !errors.Is(err, shared.ErrUnauthenticated) {
		t.Fatalf("an anonymous read answered %v", err)
	}
}
