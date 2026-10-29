// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package cmdb

import (
	"context"
	"errors"
	"testing"

	knowledgerepo "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	whiteboardrepo "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/cmdb"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

var (
	note      = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e001")
	runbook   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e002")
	hidden    = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e003")
	diagram   = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e004")
	collected = shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e005")
)

type pointingTexts struct{ kind string }

func (p *pointingTexts) Backlinks(_ context.Context, kind string, _ shared.ID, _ int) ([]knowledgerepo.Backlink, error) {
	p.kind = kind
	return []knowledgerepo.Backlink{
		{SourceKind: "ENTRY", SourceID: note, EntryID: note, Strength: "MENTION"},
		{SourceKind: "PAGE", SourceID: runbook, HubID: hub, Strength: "CARD"},
		{SourceKind: "PAGE", SourceID: hidden, HubID: private, Strength: "MENTION"},
	}, nil
}

func (p *pointingTexts) Summaries(_ context.Context, ids []shared.ID) ([]knowledgerepo.Summary, error) {
	return []knowledgerepo.Summary{{ID: runbook, HubID: hub, Title: "Runbook"}, {ID: hidden, HubID: private, Title: "Hidden"}}, nil
}

type pointingBoards struct{}

func (pointingBoards) Referencing(_ context.Context, _ string, _ shared.ID, _ int) ([]whiteboardrepo.Pointing, error) {
	return []whiteboardrepo.Pointing{{ID: diagram, HubID: hub, Title: "Network", Strength: "CARD"}}, nil
}

type pointingEntries struct{}

func (pointingEntries) Entries(_ context.Context, _ []shared.ID) ([]workrepo.EntrySummary, error) {
	return []workrepo.EntrySummary{{ID: note, CollectionID: collected, HubID: hub, Title: "Renew the certificate"}}, nil
}

// Bert reads nothing in the private hub; Anna reads everything.
type seeing struct{}

func (seeing) Permitted(_ context.Context, actor appshared.ActorContext, _ access.Request, paths [][]identity.Scope) ([]bool, error) {
	out := make([]bool, len(paths))
	for i, path := range paths {
		out[i] = true
		for _, s := range path {
			if s.ID == private && actor.AccountID != anna {
				out[i] = false
			}
		}
	}
	return out, nil
}

// UC-AST-01 check 9: a CI lists the entries, pages and whiteboards that point at it, each only
// where the reader may see it, and a reader of another hub learns nothing of the CI.
func TestACIListsWhatPointsAtItThatTheReaderMaySee(t *testing.T) {
	f := newFixture()
	ci := domain.CI{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e010"), HubID: hub, Class: "server", Name: "web1"}
	f.m.cis[ci.ID] = ci
	texts := &pointingTexts{}
	use := ListCiBacklinks{Inventory: f.inv, Pointers: Pointers{Texts: texts, Boards: pointingBoards{}, Entries: pointingEntries{}, Reader: seeing{}}}
	out, err := call(t, use.Descriptor(), as(bert), usecase.Input{"ci_id": ci.ID.String()})
	if err != nil {
		t.Fatal(err)
	}
	if texts.kind != "CI" {
		t.Errorf("the backlinks were asked of kind %q", texts.kind)
	}
	data := out["data"].([]usecase.Output)
	var got []string
	for _, row := range data {
		got = append(got, row["source_kind"].(string)+":"+row["title"].(string))
	}
	want := []string{"ENTRY:Renew the certificate", "PAGE:Runbook", "WHITEBOARD:Network"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("Bert sees %v, want %v (the private hub's page left out)", got, want)
	}

	secret := domain.CI{ID: shared.MustParseID("01936f2a-7c1e-7000-8000-00000000e011"), HubID: private, Class: "server", Name: "vault"}
	f.m.cis[secret.ID] = secret
	if _, err := call(t, use.Descriptor(), as(bert), usecase.Input{"ci_id": secret.ID.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a CI of a hub out of reach answers %v", err)
	}
}
