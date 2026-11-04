// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// links keeps public links by the token's whole text, as the adapter keeps them by its hash.
type links struct {
	byPage map[shared.ID]domain.PublicLink
	token  map[shared.ID]string
}

func (l *links) Put(_ context.Context, link domain.PublicLink, token integration.PageLinkToken) error {
	l.byPage[link.PageID], l.token[link.PageID] = link, token.Secret()
	return nil
}
func (l *links) Find(_ context.Context, id shared.ID) (domain.PublicLink, bool, error) {
	link, ok := l.byPage[id]
	return link, ok, nil
}
func (l *links) FindByToken(_ context.Context, token integration.PageLinkToken) (domain.PublicLink, error) {
	for id, held := range l.token {
		if held == token.Secret() {
			return l.byPage[id], nil
		}
	}
	return domain.PublicLink{}, shared.ErrNotFound.WithDetail("pages.link_not_found")
}
func (l *links) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, ok := l.byPage[id]
	delete(l.byPage, id)
	delete(l.token, id)
	return ok, nil
}

type settingsStore struct {
	plan   setting.Layer
	stored []settingrepo.Stored
}

func (s *settingsStore) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance}, s.plan, nil
}
func (s *settingsStore) At(context.Context, []settingrepo.Place, string) ([]settingrepo.Stored, error) {
	return s.stored, nil
}
func (s *settingsStore) Put(_ context.Context, _ shared.ID, place settingrepo.Place, key setting.Key, entry setting.Entry, _ time.Time) error {
	s.stored = []settingrepo.Stored{{Place: place, Key: key, Entry: entry}}
	return nil
}
func (s *settingsStore) Remove(context.Context, settingrepo.Place, setting.Key) (bool, error) {
	return false, nil
}

type people struct {
	identityrepo.Accounts
	status map[shared.ID]identity.AccountStatus
}

func (p people) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	status, ok := p.status[id]
	if !ok {
		return identity.Account{}, shared.ErrNotFound
	}
	return identity.Account{ID: id, TenantID: tenant, Kind: identity.AccountUser, Status: status}, nil
}

type fixedEntropy struct{ fill byte }

func (e *fixedEntropy) Bytes(n int) ([]byte, error) {
	e.fill++
	return bytes.Repeat([]byte{e.fill}, n), nil
}

type knowledgeOn struct{ off bool }

func (k knowledgeOn) On(context.Context, appshared.ActorContext, shared.ID) (module.Set, error) {
	if k.off {
		return module.Set{}, nil
	}
	return module.Set{module.Knowledge}, nil
}

// UC-DOC-08 check 4: a page's public link is made only where the workspace allows it and never
// for a restricted page; whoever holds it reads the page alone without signing in, as its maker
// reads it now; a new link, switching public links off, a restriction, the module off, the maker
// losing the hub or the page going to the trash each leave the link answering nothing.
func TestAPublicLinkOpensThePageAloneWhileTheWorkspaceAllowsIt(t *testing.T) {
	f := newFixture()
	store := &links{byPage: map[shared.ID]domain.PublicLink{}, token: map[shared.ID]string{}}
	settings := &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}}
	accounts := people{status: map[shared.ID]identity.AccountStatus{anna: identity.AccountActive, bert: identity.AccountActive}}
	modules := &knowledgeOn{}
	f.space.Public = PublicLinking{Links: store, Settings: settings, Entropy: &fixedEntropy{}, Accounts: accounts, Modules: modules}
	ctx := context.Background()
	page := f.create(t, anna, hub, "", "Handbook", "open words")

	if _, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID); code(err) != "pages.public_links_off" {
		t.Fatalf("a link was made while the workspace allows none: %v", err)
	}
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), true); err != nil {
		t.Fatal(err)
	}
	minted, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil || minted.Token.IsZero() || minted.Link.CreatedBy != anna {
		t.Fatalf("CreatePagePublicLink: %+v %v", minted.Link, err)
	}
	read := ReadPublicPage{Space: f.space}
	shown, err := read.Execute(ctx, minted.Token)
	if err != nil || shown.Page.ID != page.ID || shown.Page.Body != "open words" {
		t.Fatalf("the link did not open the page: %+v %v", shown, err)
	}
	access, err := (ReadPageAccess{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil || access.PublicLink == nil || !access.PublicLinks {
		t.Fatalf("the access does not name the link: %+v %v", access, err)
	}
	if out := AccessOutput(access); out["public_link"].(usecase.Output)["token"] != nil {
		t.Fatal("the access answered the token")
	}
	gone := func(why string) {
		t.Helper()
		if _, err := read.Execute(ctx, minted.Token); !errors.Is(err, shared.ErrNotFound) || code(err) != "pages.link_not_found" {
			t.Fatalf("%s: the link still answers (%v)", why, err)
		}
	}

	modules.off = true
	gone("the module off")
	modules.off = false

	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), false); err != nil {
		t.Fatal(err)
	}
	gone("public links switched off")
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), true); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Execute(ctx, minted.Token); err != nil {
		t.Fatalf("switched back on, the link does not answer: %v", err)
	}

	accounts.status[anna] = identity.AccountDisabled
	gone("its maker disabled")
	accounts.status[anna] = identity.AccountActive

	again, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil {
		t.Fatal(err)
	}
	gone("replaced by a new link")
	minted = again

	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: page.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	gone("the page restricted")
	if _, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID); code(err) != "pages.public_link_restricted" {
		t.Fatalf("a restricted page got a link: %v", err)
	}

	open := f.create(t, anna, private, "", "Mine", "private words")
	link, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), open.ID)
	if err != nil {
		t.Fatal(err)
	}
	store.byPage[open.ID] = domain.PublicLink{PageID: open.ID, CreatedBy: bert, CreatedAt: start}
	minted = link
	gone("its maker does not read the hub")

	if err := (DeletePagePublicLink{Space: f.space}).Execute(ctx, as(anna), open.ID); err != nil {
		t.Fatal(err)
	}
	if err := (DeletePagePublicLink{Space: f.space}).Execute(ctx, as(anna), open.ID); code(err) != "pages.link_not_found" {
		t.Fatalf("a second removal answered %v", err)
	}
}

// UC-DOC-08 check 4: a plan that locks public links off leaves the workspace no switch; an agent
// never decides it.
func TestAPlanLocksPublicLinksOff(t *testing.T) {
	f := newFixture()
	settings := &settingsStore{plan: setting.Layer{Level: setting.LevelPlan, Entries: map[setting.Key]setting.Entry{
		domain.PublicLinksKey: {Value: false, Hold: setting.HoldLock},
	}}}
	f.space.Public = PublicLinking{Settings: settings}
	ctx := context.Background()
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), true); err == nil {
		t.Fatal("the workspace switched on what its plan locked off")
	}
	agent := as(anna)
	agent.Kind = appshared.ActorAIAgent
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, agent, true); code(err) != "pages.agent_decides_policy_not" {
		t.Fatalf("an agent decided public links: %v", err)
	}
}

// UC-DOC-08 check 5: the public page draws the pictures the page holds and names a mentioned page
// only where that page has a public link of its own; a file that is no picture, another page's
// picture and a picture the body does not draw open nothing through the link.
func TestAPublicLinkShowsThePagesPicturesAndItsPublicMentions(t *testing.T) {
	f := newFixture()
	store := &links{byPage: map[shared.ID]domain.PublicLink{}, token: map[shared.ID]string{}}
	settings := &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}}
	accounts := people{status: map[shared.ID]identity.AccountStatus{anna: identity.AccountActive}}
	f.space.Public = PublicLinking{Links: store, Settings: settings, Entropy: &fixedEntropy{}, Accounts: accounts, Modules: &knowledgeOn{}}
	f.space.Media = uploads{store: f.store}
	ctx := context.Background()
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), true); err != nil {
		t.Fatal(err)
	}
	public := f.create(t, anna, hub, "", "Getting started", "")
	internal := f.create(t, anna, hub, "", "Salaries", "")
	elsewhere := f.create(t, anna, hub, "", "Other", "")
	page := f.create(t, anna, hub, "", "Guide", "See [](hubtask:page/"+public.ID.String()+") and [](hubtask:page/"+internal.ID.String()+").\n\n"+
		"![](hubtask:media/"+annasPicture.String()+")\n\n![](hubtask:media/"+annasFile.String()+")\n\n![](hubtask:media/"+privatePic.String()+")\n")
	f.store.media = map[shared.ID][]shared.ID{page.ID: {annasPicture, annasFile}, elsewhere.ID: {privatePic}}
	if _, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), public.ID); err != nil {
		t.Fatal(err)
	}
	minted, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil {
		t.Fatal(err)
	}
	read := ReadPublicPage{Space: f.space}
	shown, err := read.Execute(ctx, minted.Token)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(shown.Pictures, []shared.ID{annasPicture}) {
		t.Errorf("pictures = %v", shown.Pictures)
	}
	if len(shown.Titles) != 1 || shown.Titles[public.ID] != "Getting started" {
		t.Errorf("titles = %v", shown.Titles)
	}
	if tenant, err := read.Picture(ctx, minted.Token, annasPicture); err != nil || tenant != minted.Token.TenantID() {
		t.Fatalf("the page's picture: %v %v", tenant, err)
	}
	for _, id := range []shared.ID{annasFile, privatePic, pendingPic} {
		if _, err := read.Picture(ctx, minted.Token, id); code(err) != "pages.link_not_found" {
			t.Errorf("%v opened through the link: %v", id, err)
		}
	}
	if _, err := (RestrictPage{Space: f.space}).Execute(ctx, as(anna), RestrictPageCommand{PageID: page.ID, AccountIDs: []shared.ID{anna}}); err != nil {
		t.Fatal(err)
	}
	if _, err := read.Picture(ctx, minted.Token, annasPicture); code(err) != "pages.link_not_found" {
		t.Fatalf("a restricted page's picture opened: %v", err)
	}
}

// A mentioned page's title is shown only while that page's own link answers: once its maker is
// disabled, its title is as hidden as its words.
func TestAPublicPageShowsNoTitleOfAMentionWhoseLinkNoLongerAnswers(t *testing.T) {
	f := newFixture()
	store := &links{byPage: map[shared.ID]domain.PublicLink{}, token: map[shared.ID]string{}}
	accounts := people{status: map[shared.ID]identity.AccountStatus{anna: identity.AccountActive, bert: identity.AccountActive}}
	f.space.Public = PublicLinking{
		Links: store, Settings: &settingsStore{plan: setting.Layer{Level: setting.LevelPlan}},
		Entropy: &fixedEntropy{}, Accounts: accounts, Modules: &knowledgeOn{},
	}
	ctx := context.Background()
	if _, err := (SetPublicLinksPolicy{Space: f.space}).Execute(ctx, as(anna), true); err != nil {
		t.Fatal(err)
	}
	mentioned := f.create(t, bert, hub, "", "Salary bands 2027", "")
	page := f.create(t, anna, hub, "", "Guide", "See [](hubtask:page/"+mentioned.ID.String()+").")
	if _, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(bert), mentioned.ID); err != nil {
		t.Fatal(err)
	}
	minted, err := (CreatePagePublicLink{Space: f.space}).Execute(ctx, as(anna), page.ID)
	if err != nil {
		t.Fatal(err)
	}
	accounts.status[bert] = identity.AccountDisabled
	shown, err := (ReadPublicPage{Space: f.space}).Execute(ctx, minted.Token)
	if err != nil || len(shown.Titles) != 0 {
		t.Fatalf("titles %v, %v", shown.Titles, err)
	}
}
