// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

var hubID = shared.MustParseID("01936f2a-7c1e-7000-8000-0000000005f6")

type formLinks struct {
	rows map[shared.ID]domain.PublicForm
	keys map[string]shared.ID
}

func (l *formLinks) Put(_ context.Context, form domain.PublicForm, token integration.FormLinkToken) error {
	for key, id := range l.keys {
		if id == form.CollectionID {
			delete(l.keys, key)
		}
	}
	l.rows[form.CollectionID], l.keys[token.Secret()] = form, form.CollectionID
	return nil
}

func (l *formLinks) Find(_ context.Context, id shared.ID) (domain.PublicForm, bool, error) {
	form, found := l.rows[id]
	return form, found, nil
}

func (l *formLinks) FindByToken(_ context.Context, token integration.FormLinkToken) (domain.PublicForm, error) {
	form, found := l.rows[l.keys[token.Secret()]]
	if !found {
		return domain.PublicForm{}, shared.ErrNotFound
	}
	return form, nil
}

func (l *formLinks) Delete(_ context.Context, id shared.ID) (bool, error) {
	_, found := l.rows[id]
	delete(l.rows, id)
	return found, nil
}

type switchStore struct {
	settingrepo.Settings
	on bool
}

func (s *switchStore) Above(context.Context, string) (setting.Layer, setting.Layer, error) {
	return setting.Layer{Level: setting.LevelInstance}, setting.Layer{Level: setting.LevelPlan}, nil
}

func (s *switchStore) At(_ context.Context, places []settingrepo.Place, _ string) ([]settingrepo.Stored, error) {
	return []settingrepo.Stored{{Place: places[0], Key: domain.PublicFormsKey, Entry: setting.Entry{Value: s.on, Hold: setting.HoldSet}}}, nil
}

func (s *switchStore) Put(_ context.Context, _ shared.ID, _ settingrepo.Place, _ setting.Key, entry setting.Entry, _ time.Time) error {
	s.on, _ = entry.Value.(bool)
	return nil
}

type makerAccounts struct {
	identityrepo.Accounts
	status identity.AccountStatus
}

func (a makerAccounts) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	return identity.Account{ID: id, TenantID: tenantID, DisplayName: "Jonas", Status: a.status}, nil
}

type deskModules struct{ on bool }

func (m deskModules) On(context.Context, appshared.ActorContext, shared.ID) (module.Set, error) {
	if m.on {
		return module.Set{module.ServiceDesk}, nil
	}
	return module.Set{}, nil
}

type permits struct{ creates bool }

func (p *permits) Permits(context.Context, appshared.ActorContext, access.Request) (bool, error) {
	return p.creates, nil
}

type publicTypes struct{ workrepo.EntryTypes }

func (publicTypes) List(context.Context) ([]work.EntryType, error) {
	return []work.EntryType{
		{Key: "feedback", Level: work.ItemTask, Name: "Feedback",
			Form: work.Form{Channels: []work.FormChannelRule{{Channel: work.ChannelPublic}}}},
		{Key: "internal", Level: work.ItemTask, Name: "Internal"},
	}, nil
}

type collections struct{ workrepo.Containers }

func (collections) Find(_ context.Context, id shared.ID) (work.Container, error) {
	return work.Container{ID: id, TenantID: tenantID, Type: work.ContainerCollection, ParentID: hubID}, nil
}

type creations struct {
	calls  []usecase.Input
	actors []appshared.ActorContext
	refuse error
}

func (c *creations) Invoke(_ context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	if name != createEntryUseCase {
		return nil, errors.New("not CreateWorkItem")
	}
	if c.refuse != nil {
		return nil, c.refuse
	}
	c.calls, c.actors = append(c.calls, in), append(c.actors, actor)
	return usecase.Output{"id": itemID.String()}, nil
}

type contacts struct{ addresses []string }

// ContactsSince counts every recorded contact of the address: the scene's clock stands still.
func (c *contacts) ContactsSince(_ context.Context, address string, _ time.Time) (int, error) {
	n := 0
	for _, each := range c.addresses {
		if strings.HasSuffix(each, "<"+address+">") {
			n++
		}
	}
	return n, nil
}

// ledger is the installation's record of spent challenges, one map shared by every "server".
type ledger struct{ spent map[string]bool }

func (l *ledger) Take(_ context.Context, nonce string, _, _ time.Time) (bool, error) {
	if l.spent[nonce] {
		return false, nil
	}
	l.spent[nonce] = true
	return true, nil
}

func (c *contacts) RecordContact(_ context.Context, address, name string, _ shared.ID, _ time.Time) error {
	c.addresses = append(c.addresses, name+" <"+address+">")
	return nil
}

type fixedEntropy struct{ n byte }

func (e *fixedEntropy) Bytes(n int) ([]byte, error) {
	e.n++
	return bytes.Repeat([]byte{e.n}, n), nil
}

type signer struct{}

func (signer) Hash(presented string) []byte {
	mac := hmac.New(sha256.New, []byte("installation"))
	mac.Write([]byte(presented))
	return mac.Sum(nil)
}

type publicScene struct {
	forms    PublicForms
	links    *formLinks
	settings *switchStore
	entries  *creations
	contacts *contacts
	permits  *permits
}

func newPublicScene() publicScene {
	links := &formLinks{rows: map[shared.ID]domain.PublicForm{}, keys: map[string]shared.ID{}}
	settings := &switchStore{on: true}
	entries, recorded, may := &creations{}, &contacts{}, &permits{creates: true}
	ids := &seqIDs{}
	return publicScene{
		forms: PublicForms{
			Portal: Portal{
				Entries: entries, Containers: collections{}, EntryTypes: publicTypes{},
				Desk: Desk{Authorizer: &gate{allow: true}, UnitOfWork: inline{}, Audit: discard{}, Clock: at(monday), IDs: ids},
			},
			Links: links, Settings: settings, Entropy: &fixedEntropy{}, Accounts: makerAccounts{status: identity.AccountActive},
			Modules: deskModules{on: true}, Permits: may, Requesters: recorded, Signer: signer{}, Spent: &ledger{spent: map[string]bool{}}, History: recorded,
		},
		links: links, settings: settings, entries: entries, contacts: recorded, permits: may,
	}
}

func (s publicScene) mint(t *testing.T) integration.FormLinkToken {
	t.Helper()
	minted, err := CreatePublicForm{Forms: s.forms}.Execute(context.Background(), reader(), collection)
	if err != nil {
		t.Fatal(err)
	}
	return minted.Token
}

func solved(t *testing.T, s publicScene, token integration.FormLinkToken) (string, string) {
	t.Helper()
	view, err := OpenPublicForm{Forms: s.forms}.Execute(context.Background(), token, "")
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; ; n++ {
		if candidate := strconv.Itoa(n); domain.Solves(view.Challenge.Value, candidate, view.Challenge.Bits) {
			return view.Challenge.Value, candidate
		}
	}
}

func detailOf(err error) string {
	var coded *shared.Error
	if errors.As(err, &coded) {
		return coded.DetailCode
	}
	return ""
}

// UC-SVC-13 checks 1 and 3: a public form shows the types whose form opens PUBLIC, and answers
// nothing once the workspace switches public forms off, its maker leaves, the service desk is off
// at the hub, the maker may no longer create there, or the link is replaced or taken away.
func TestAPublicFormAnswersOnlyWhileEverythingBehindItHolds(t *testing.T) {
	ctx := context.Background()
	s := newPublicScene()
	token := s.mint(t)
	view, err := OpenPublicForm{Forms: s.forms}.Execute(ctx, token, "")
	if err != nil || len(view.Catalogue) != 1 || view.Catalogue[0].Key != "feedback" || view.Challenge.Bits != domain.PublicFormWorkBits {
		t.Fatalf("the form shows %+v, %v", view, err)
	}
	for name, change := range map[string]func(*PublicForms) func(){
		"switched off": func(f *PublicForms) func() {
			s.settings.on = false
			return func() { s.settings.on = true }
		},
		"maker gone": func(f *PublicForms) func() {
			f.Accounts = makerAccounts{status: identity.AccountDisabled}
			return func() { f.Accounts = makerAccounts{status: identity.AccountActive} }
		},
		"module off": func(f *PublicForms) func() {
			f.Modules = deskModules{on: false}
			return func() { f.Modules = deskModules{on: true} }
		},
		"maker may not create": func(f *PublicForms) func() {
			s.permits.creates = false
			return func() { s.permits.creates = true }
		},
	} {
		forms := s.forms
		undo := change(&forms)
		if _, err := (OpenPublicForm{Forms: forms}).Execute(ctx, token, ""); detailOf(err) != "public_forms.link_not_found" {
			t.Errorf("%s: the form answered %v", name, err)
		}
		undo()
	}
	replaced := s.mint(t)
	if _, err := (OpenPublicForm{Forms: s.forms}).Execute(ctx, token, ""); detailOf(err) != "public_forms.link_not_found" {
		t.Errorf("a replaced link answered %v", err)
	}
	if err := (DeletePublicForm{Forms: s.forms}).Execute(ctx, reader(), collection); err != nil {
		t.Fatal(err)
	}
	if _, err := (OpenPublicForm{Forms: s.forms}).Execute(ctx, replaced, ""); detailOf(err) != "public_forms.link_not_found" {
		t.Errorf("a removed link answered %v", err)
	}
	s.settings.on = false
	if _, err := (CreatePublicForm{Forms: s.forms}).Execute(ctx, reader(), collection); detailOf(err) != "public_forms.off" {
		t.Errorf("a link was made where the workspace allows none: %v", err)
	}
}

// UC-SVC-13 checks 4-6: a request is filed through CreateWorkItem on the PUBLIC door as the
// maker, with the From line above the words and the requester recorded; one solved challenge
// files one request, a refused one may be corrected and sent with the same work, and a type the
// form does not offer is refused.
func TestAPublicRequestIsFiledOnceAsTheMakerForTheVisitor(t *testing.T) {
	ctx := context.Background()
	s := newPublicScene()
	token := s.mint(t)
	submit := SubmitPublicForm{Forms: s.forms}
	challenge, solution := solved(t, s, token)
	request := PublicSubmission{
		EntryType: "feedback", Title: "Drucker druckt leer", Notes: "Seit heute früh.",
		RequesterName: "Frau Kaya", RequesterEmail: "Kaya@Example.org", Challenge: challenge, Solution: solution,
	}

	wrong := request
	wrong.EntryType = "internal"
	if err := submit.Execute(ctx, token, wrong); detailOf(err) != "requests.type_not_offered" {
		t.Fatalf("a type the form does not offer: %v", err)
	}
	if err := submit.Execute(ctx, token, request); err != nil {
		t.Fatalf("the corrected request with the same work was refused: %v", err)
	}
	if len(s.entries.calls) != 1 {
		t.Fatalf("%d creations", len(s.entries.calls))
	}
	in, actor := s.entries.calls[0], s.entries.actors[0]
	if actor.AccountID != handlerID || actor.Channel != string(work.ChannelPublic) || !actor.ForSender ||
		len(actor.Scopes) != 1 || actor.Scopes[0] != itemsWriteScope {
		t.Errorf("filed as %+v", actor)
	}
	if in["collection_id"] != collection.String() || in["entry_type"] != "feedback" ||
		in["notes"] != "From: Frau Kaya <kaya@example.org>\n\nSeit heute früh." {
		t.Errorf("filed %+v", in)
	}
	if len(s.contacts.addresses) != 1 || s.contacts.addresses[0] != "Frau Kaya <kaya@example.org>" {
		t.Errorf("recorded %v", s.contacts.addresses)
	}
	if err := submit.Execute(ctx, token, request); detailOf(err) != "public_forms.work_spent" {
		t.Errorf("one solved challenge filed a second request: %v", err)
	}
	if err := submit.Execute(ctx, token, PublicSubmission{EntryType: "feedback", Title: "x", Challenge: challenge, Solution: "x"}); detailOf(err) != "public_forms.work_invalid" {
		t.Errorf("an unsolved challenge: %v", err)
	}

	anonymous, solution := solved(t, s, token)
	if err := submit.Execute(ctx, token, PublicSubmission{EntryType: "feedback", Title: "Lob", Challenge: anonymous, Solution: solution}); err != nil {
		t.Fatal(err)
	}
	if last := s.entries.calls[len(s.entries.calls)-1]; last["notes"] != nil || s.entries.actors[len(s.entries.actors)-1].ForSender {
		t.Errorf("an anonymous request carries %+v", last)
	}
	if len(s.contacts.addresses) != 1 {
		t.Errorf("an anonymous request recorded a requester")
	}
}

// watchedAccounts answers the maker like makerAccounts and remembers every account it was asked
// for; any other way into the accounts - by address above all - panics through the nil embed.
type watchedAccounts struct {
	makerAccounts
	asked *[]shared.ID
}

func (a watchedAccounts) Find(ctx context.Context, id shared.ID) (identity.Account, error) {
	*a.asked = append(*a.asked, id)
	return a.makerAccounts.Find(ctx, id)
}

// UC-SVC-13 check 7: the visitor's address is never looked up among the workspace's accounts - the
// only account read is the link's maker - and the answer is the same whoever the address belongs
// to; neither the forms nor the portal hold a mail port, so nothing is mailed on submission.
func TestAPublicRequestLooksUpNoAccountByItsAddress(t *testing.T) {
	ctx := context.Background()
	s := newPublicScene()
	var asked []shared.ID
	watched := watchedAccounts{makerAccounts: makerAccounts{status: identity.AccountActive}, asked: &asked}
	s.forms.Accounts, s.forms.Portal.Accounts = watched, watched
	token := s.mint(t)
	submit := SubmitPublicForm{Forms: s.forms}
	for _, address := range []string{"jonas@example.org", "nobody@example.net"} {
		challenge, solution := solved(t, s, token)
		err := submit.Execute(ctx, token, PublicSubmission{
			EntryType: "feedback", Title: "Drucker", RequesterEmail: address, Challenge: challenge, Solution: solution,
		})
		if err != nil {
			t.Fatalf("%s: %v", address, err)
		}
	}
	if len(asked) == 0 {
		t.Fatal("the maker was never read; the watch saw nothing")
	}
	for _, id := range asked {
		if id != handlerID {
			t.Errorf("an account other than the maker was read: %s", id)
		}
	}
}

// UC-SVC-13 check 8: a challenge solved once files one request on whichever server it is sent
// to, and one requester's address files at most PublicFormRequesterPerHour requests an hour.
func TestAChallengeIsSpentOnceAcrossServersAndARequesterIsBounded(t *testing.T) {
	ctx := context.Background()
	first := newPublicScene()
	token := first.mint(t)
	second := first
	second.forms.Spent = first.forms.Spent // a second replica, the same database
	challenge, solution := solved(t, first, token)
	request := PublicSubmission{EntryType: "feedback", Title: "Lob", Challenge: challenge, Solution: solution}
	if err := (SubmitPublicForm{Forms: first.forms}).Execute(ctx, token, request); err != nil {
		t.Fatal(err)
	}
	if err := (SubmitPublicForm{Forms: second.forms}).Execute(ctx, token, request); detailOf(err) != "public_forms.work_spent" {
		t.Fatalf("a second server took the same challenge again: %v", err)
	}

	submit := SubmitPublicForm{Forms: first.forms}
	for i := 0; i < domain.PublicFormRequesterPerHour; i++ {
		c, sol := solved(t, first, token)
		err := submit.Execute(ctx, token, PublicSubmission{
			EntryType: "feedback", Title: "Lob", RequesterName: "Kaya", RequesterEmail: "kaya@example.org",
			Challenge: c, Solution: sol,
		})
		if err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	c, sol := solved(t, first, token)
	filed := len(first.entries.calls)
	err := submit.Execute(ctx, token, PublicSubmission{
		EntryType: "feedback", Title: "Lob", RequesterName: "Kaya", RequesterEmail: "KAYA@example.org",
		Challenge: c, Solution: sol,
	})
	if detailOf(err) != "public_forms.requester_limit" || !errors.Is(err, shared.ErrRateLimited) {
		t.Fatalf("one address past its hour's bound: %v", err)
	}
	if len(first.entries.calls) != filed {
		t.Errorf("the refused request was filed")
	}
	c, sol = solved(t, first, token)
	if err := submit.Execute(ctx, token, PublicSubmission{
		EntryType: "feedback", Title: "Lob", RequesterName: "Ilse", RequesterEmail: "ilse@example.org",
		Challenge: c, Solution: sol,
	}); err != nil {
		t.Errorf("another address was bounded with the first: %v", err)
	}

	without := first.forms
	without.Spent = nil
	c, sol = solved(t, first, token)
	if err := (SubmitPublicForm{Forms: without}).Execute(ctx, token, PublicSubmission{EntryType: "feedback", Title: "Lob", Challenge: c, Solution: sol}); !errors.Is(err, shared.ErrUnavailable) {
		t.Errorf("without a ledger a request was filed: %v", err)
	}
}

// UC-SVC-13 check 3: a maker who may no longer create in the collection makes the form gone to
// the visitor, not a refusal naming them.
func TestARefusedMakerIsAGoneForm(t *testing.T) {
	s := newPublicScene()
	token := s.mint(t)
	challenge, solution := solved(t, s, token)
	s.entries.refuse = shared.ErrForbidden.WithDetail("access.denied")
	err := SubmitPublicForm{Forms: s.forms}.Execute(context.Background(), token, PublicSubmission{
		EntryType: "feedback", Title: "Lob", Challenge: challenge, Solution: solution,
	})
	if detailOf(err) != "public_forms.link_not_found" || strings.Contains(err.Error(), "access") {
		t.Errorf("answered %v", err)
	}
}
