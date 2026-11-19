// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"context"
	"testing"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/port/coverimages"
)

// library answers one picture and counts what it was asked.
type library struct{ searched, fetched int }

func (l *library) Search(context.Context, string) ([]coverimages.Picture, error) {
	l.searched++
	return []coverimages.Picture{{ID: "11111111-1111-1111-1111-111111111111", Creator: "Ada", Preview: []byte("p"), PreviewType: "image/jpeg"}}, nil
}

func (l *library) Fetch(context.Context, string) (coverimages.Image, error) {
	l.fetched++
	return coverimages.Image{Content: []byte("big"), ContentType: "image/jpeg", Creator: "Ada"}, nil
}

// UC-DOC-16 check 5: an image library is offered only where the workspace chooses one; until then
// a writer's search asks nothing outside and a fetch is refused. Chosen, a writer of the page gets
// pictures with who made them, previews fetched by the server; somebody who cannot read the page's
// hub gets nothing; an agent never chooses the library.
func TestAnImageLibraryIsOfferedOnlyWhereTheWorkspaceChoosesOne(t *testing.T) {
	f := newFixture()
	pictures := &library{}
	f.space.Covers = CoverLibrary{Library: pictures, Settings: &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}}}
	ctx := context.Background()
	page := f.create(t, anna, hub, "", "Handbook", "")
	search := SearchCoverImages{Space: f.space}.Descriptor().Handler
	fetch := FetchCoverImage{Space: f.space}.Descriptor().Handler
	ask := usecase.Input{"page_id": page.ID.String(), "query": "fjord"}

	out, err := search.Invoke(ctx, as(anna), ask)
	if err != nil || out["offered"] != false || len(out["pictures"].([]any)) != 0 || pictures.searched != 0 {
		t.Fatalf("searched while off: %v %v (asked %d)", out, err, pictures.searched)
	}
	if _, err := fetch.Invoke(ctx, as(anna), usecase.Input{"page_id": page.ID.String(), "picture_id": "x"}); code(err) != "pages.cover_images_off" {
		t.Fatalf("fetched while off: %v", err)
	}

	agent := as(anna)
	agent.Kind = appshared.ActorAIAgent
	if _, err := (SetCoverImagesPolicy{Space: f.space}).Execute(ctx, agent, domain.CoverImagesOpenLibrary); code(err) != "pages.agent_decides_policy_not" {
		t.Fatalf("an agent chose the library: %v", err)
	}
	if r, err := (SetCoverImagesPolicy{Space: f.space}).Execute(ctx, as(anna), domain.CoverImagesOpenLibrary); err != nil || domain.CoverImagesOf(r) != domain.CoverImagesOpenLibrary {
		t.Fatalf("SetCoverImagesPolicy: %v %v", r, err)
	}

	out, err = search.Invoke(ctx, as(anna), usecase.Input{"page_id": page.ID.String()})
	if err != nil || out["offered"] != true || pictures.searched != 0 {
		t.Fatalf("a search without a query asked the library: %v %v", out, err)
	}
	out, err = search.Invoke(ctx, as(anna), ask)
	found, _ := out["pictures"].([]any)
	if err != nil || len(found) != 1 {
		t.Fatalf("search: %v %v", out, err)
	}
	first := found[0].(usecase.Output)
	if first["creator"] != "Ada" || first["preview"] != "data:image/jpeg;base64,cA==" {
		t.Fatalf("picture: %v", first)
	}
	got, err := fetch.Invoke(ctx, as(anna), usecase.Input{"page_id": page.ID.String(), "picture_id": "11111111-1111-1111-1111-111111111111"})
	if err != nil || got["credit"] != "Ada" || got["content_base64"] != "Ymln" || got["content_type"] != "image/jpeg" {
		t.Fatalf("fetch: %v %v", got, err)
	}

	secret := f.create(t, anna, private, "", "Private", "")
	before := pictures.searched
	if _, err := search.Invoke(ctx, as(bert), usecase.Input{"page_id": secret.ID.String(), "query": "fjord"}); err == nil || pictures.searched != before {
		t.Fatalf("somebody outside the hub searched for its page: %v", err)
	}
}

// UC-DOC-16 check 5: a plan that locks the library off leaves the workspace no choice.
func TestAPlanLocksTheImageLibraryOff(t *testing.T) {
	f := newFixture()
	f.space.Covers = CoverLibrary{Library: &library{}, Settings: &settingsStore{plan: setting.Layer{Level: setting.LevelPlan, Entries: map[setting.Key]setting.Entry{
		domain.CoverImagesKey: {Value: domain.CoverImagesOff, Hold: setting.HoldLock},
	}}}}
	if _, err := (SetCoverImagesPolicy{Space: f.space}).Execute(context.Background(), as(anna), domain.CoverImagesOpenLibrary); err == nil {
		t.Fatal("the workspace chose what its plan locked off")
	}
}
