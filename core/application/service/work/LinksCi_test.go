// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"slices"
	"testing"

	"github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

type cisByID []cmdb.CI

func (c cisByID) CIsByID(_ context.Context, ids []shared.ID) ([]cmdb.CI, error) {
	var out []cmdb.CI
	for _, ci := range c {
		if slices.Contains(ids, ci.ID) {
			out = append(out, ci)
		}
	}
	return out, nil
}

// UC-AST-01 check 9: `hubtask:ci/<id>` resolves to the CI's name, class, stage and hub for a reader
// of its hub, and to its address alone for anyone else - never to its attributes (P-05, P-19).
func TestAConfigurationItemResolvesOnlyForAReaderOfItsHub(t *testing.T) {
	ctx := context.Background()
	h := newLinkHarness()
	open := shared.MustParseID("0192f000-0000-7000-8000-0000000007a1")
	closed := shared.MustParseID("0192f000-0000-7000-8000-0000000007a2")
	shop := shared.MustParseID("0192f000-0000-7000-8000-0000000007b1")
	vault := shared.MustParseID("0192f000-0000-7000-8000-0000000007b2")
	h.reader.hidden[closed] = true
	linking := h.linking()
	linking.Cis = cisByID{
		{ID: shop, HubID: open, Class: "application_service", Name: "Shop", Stage: cmdb.StageInUse, Attributes: map[string]any{"url": "https://shop"}},
		{ID: vault, HubID: closed, Class: "server", Name: "Vault", Stage: cmdb.StageInUse},
	}
	if !reference.KindCI.Valid() {
		t.Fatal("CI is not a reference kind")
	}
	resolved, err := ResolveReferences{Linking: linking}.Execute(ctx, linkActor(), []reference.Reference{
		{Kind: reference.KindCI, ID: shop}, {Kind: reference.KindCI, ID: vault},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resolved[0]["available"] != true || resolved[0]["title"] != "Shop" || resolved[0]["ci_class"] != "application_service" ||
		resolved[0]["stage"] != "IN_USE" || resolved[0]["hub_id"] != open.String() || resolved[0]["attributes"] != nil {
		t.Errorf("a readable CI resolves to %+v", resolved[0])
	}
	if resolved[1]["available"] != false || len(resolved[1]) != 4 {
		t.Errorf("a CI out of reach resolves to %+v", resolved[1])
	}
	found := reference.Extract("runs on [the shop](hubtask:ci/" + shop.String() + ") and ![](hubtask:ci/" + vault.String() + ")")
	if len(found) != 2 || found[0].Reference.Kind != reference.KindCI || found[1].Strength != reference.Card {
		t.Errorf("a text's CI addresses %+v", found)
	}
}
