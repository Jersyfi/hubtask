// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"context"
	"testing"
	"time"

	privacyrepo "github.com/Jersyfi/hubtask/core/application/repository/privacy"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// A person's own view of keeping their content out of AI (UC-PRV-05 checks 4, 8 and 9).

var (
	objectorID  = shared.MustParseID("0192f000-0000-7000-8000-0000000000b1")
	colleagueID = shared.MustParseID("0192f000-0000-7000-8000-0000000000b2")
)

// consentBook is an in-memory consent store and notice book: the newest record per person decides.
type consentBook struct {
	records []privacydomain.Consent
	noticed map[shared.ID]time.Time
}

func (b *consentBook) Withdraw(context.Context, shared.ID, string, time.Time) (int, error) {
	return 0, nil
}

func (b *consentBook) Record(_ context.Context, consent privacydomain.Consent) error {
	b.records = append(b.records, consent)
	return nil
}

func (b *consentBook) Latest(_ context.Context, account shared.ID, purpose string) (privacydomain.Consent, error) {
	for i := len(b.records) - 1; i >= 0; i-- {
		if b.records[i].AccountID == account && b.records[i].Purpose == purpose {
			return b.records[i], nil
		}
	}
	return privacydomain.Consent{}, shared.ErrNotFound.WithDetail(privacydomain.CodeConsentNotFound)
}

func (b *consentBook) Standing(ctx context.Context, purpose string) ([]privacyrepo.Objector, error) {
	seen := map[shared.ID]bool{}
	var standing []privacyrepo.Objector
	for i := len(b.records) - 1; i >= 0; i-- {
		record := b.records[i]
		if record.Purpose != purpose || seen[record.AccountID] {
			continue
		}
		seen[record.AccountID] = true
		if !record.Granted {
			standing = append(standing, privacyrepo.Objector{
				AccountID: record.AccountID, DisplayName: "Kim Keeper", Email: "kim@example.org",
				Since: record.GrantedAt, Source: record.Source,
			})
		}
	}
	return standing, nil
}

func (b *consentBook) NoticedAt(_ context.Context, account shared.ID) (time.Time, error) {
	return b.noticed[account], nil
}

func (b *consentBook) Notice(_ context.Context, account shared.ID, at time.Time) error {
	if b.noticed == nil {
		b.noticed = map[shared.ID]time.Time{}
	}
	if at.After(b.noticed[account]) {
		b.noticed[account] = at
	}
	return nil
}

func (b *consentBook) withdraw(account shared.ID, at time.Time) {
	b.records = append(b.records, privacydomain.Consent{
		AccountID: account, Purpose: privacydomain.PurposeAi, GrantedAt: at, RevokedAt: at,
		Source: privacydomain.SourceUser,
	})
}

type headcount int

func (h headcount) CountPeople(context.Context) (int, error) { return int(h), nil }

// aiOn is a workspace whose own model is configured and consented: AI answers.
func aiOn() *providerStore {
	store := &providerStore{active: "k1"}
	store.stored = &domain.AiProvider{
		Kind: domain.AiOpenAiCompatible, BaseURL: "https://own.example/v1", CompletionModel: "m",
		Jurisdiction: domain.AiEEA, ProcessingAllowed: true,
	}
	return store
}

func objectionRegistry(t *testing.T, world *sourceWorld, own *providerStore, people int, book *consentBook) *usecase.Registry {
	t.Helper()
	rules := AiRules{
		Instance: world, Workspaces: sourceWorkspaces{world},
		Offered: sourceOffered{world}, Choices: sourceChoices{world}, Own: own,
	}
	reader := AiObjectionReader{
		Rules: &rules, People: headcount(people), Consents: book, Notices: book,
		UnitOfWork: providerDirect{}, Clock: providerClock{},
	}
	registry, err := usecase.NewRegistry(nil,
		ReadOwnAiPosition{Reader: reader}.Descriptor(), AcknowledgeAiPosition{Reader: reader}.Descriptor())
	if err != nil {
		t.Fatalf("not registrable: %v", err)
	}
	return registry
}

func person(id shared.ID) appshared.ActorContext {
	actor := providerActor()
	actor.AccountID = id
	actor.Scopes = []string{"privacy:read", "privacy:manage"}
	return actor
}

func readOwn(t *testing.T, registry *usecase.Registry, id shared.ID) usecase.Output {
	t.Helper()
	out, err := registry.Invoke(t.Context(), ReadOwnAiPositionName, person(id), usecase.Input{})
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	return out
}

// Check 8: the switch is there only where AI answers and more than one person works here.
func TestTheSwitchAppearsOnlyWithAiOnAndMoreThanOnePerson(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		own    *providerStore
		people int
		shown  bool
	}{
		{"AI on, two people", aiOn(), 2, true},
		{"AI on, alone", aiOn(), 1, false},
		{"AI off, two people", &providerStore{active: "k1"}, 2, false},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			out := readOwn(t, objectionRegistry(t, &sourceWorld{}, testCase.own, testCase.people, &consentBook{}), objectorID)
			if out["shown"] != testCase.shown || out["person_opt_out"] != "OFFERED" {
				t.Errorf("answered %v", out)
			}
		})
	}
}

// Checks 7 and 8: alone, a recorded withdrawal takes no effect - there is no switch to undo it.
func TestAnObjectionTakesEffectOnlyWhereItMeansSomething(t *testing.T) {
	book := &consentBook{}
	book.withdraw(objectorID, providerClock{}.Now())
	if out := readOwn(t, objectionRegistry(t, &sourceWorld{}, aiOn(), 2, book), objectorID); out["keeping_out"] != true || out["in_effect"] != true {
		t.Errorf("two people: %v", out)
	}
	if out := readOwn(t, objectionRegistry(t, &sourceWorld{}, aiOn(), 1, book), objectorID); out["keeping_out"] != true || out["in_effect"] != false {
		t.Errorf("alone: %v", out)
	}
	if out := readOwn(t, objectionRegistry(t, &sourceWorld{}, aiOn(), 2, book), colleagueID); out["keeping_out"] != false {
		t.Errorf("a colleague reads somebody else's choice as theirs: %v", out)
	}
}

// Check 9: under NOT_OFFERED the person sees the basis and whom to ask, their withdrawal stops
// taking effect, and they are told exactly once - again after a second switch.
func TestNotOfferedShowsTheBasisAndTellsAnObjectorOnce(t *testing.T) {
	book := &consentBook{}
	book.withdraw(objectorID, providerClock{}.Now().Add(-48*time.Hour))
	world := &sourceWorld{}
	world.workspace.Settings.AiPersonOptOut = "NOT_OFFERED"
	world.workspace.Settings.AiOptOutBasis = "WORKS_AGREEMENT"
	world.workspace.Settings.AiOptOutContact = "The works council"
	world.workspace.Settings.AiOptOutSince = providerClock{}.Now().Add(-time.Hour)
	registry := objectionRegistry(t, world, aiOn(), 2, book)

	out := readOwn(t, registry, objectorID)
	if out["person_opt_out"] != "NOT_OFFERED" || out["basis"] != "WORKS_AGREEMENT" || out["contact"] != "The works council" ||
		out["keeping_out"] != true || out["in_effect"] != false || out["notice_pending"] != true {
		t.Fatalf("answered %v", out)
	}
	if colleague := readOwn(t, registry, colleagueID); colleague["notice_pending"] != false || colleague["basis"] != "WORKS_AGREEMENT" {
		t.Errorf("somebody who never objected is told, or not shown the basis: %v", colleague)
	}

	acknowledged, err := registry.Invoke(t.Context(), AcknowledgeAiPositionName, person(objectorID), usecase.Input{})
	if err != nil || acknowledged["notice_pending"] != false {
		t.Fatalf("acknowledging answered %v (%v)", acknowledged, err)
	}
	if again := readOwn(t, registry, objectorID); again["notice_pending"] != false {
		t.Errorf("told twice about one position: %v", again)
	}

	// The workspace goes back and comes again: a new moment, and a new notice.
	world.workspace.Settings.AiOptOutSince = providerClock{}.Now().Add(time.Hour)
	if later := readOwn(t, registry, objectorID); later["notice_pending"] != true {
		t.Errorf("a second switch tells nobody: %v", later)
	}
}

func TestTheSystemHasNoOwnPosition(t *testing.T) {
	registry := objectionRegistry(t, &sourceWorld{}, aiOn(), 2, &consentBook{})
	_, err := registry.Invoke(t.Context(), ReadOwnAiPositionName, providerActor(), usecase.Input{})
	if got := shared.AsError(err); got == nil || got.DetailCode != "accounts.self_required" {
		t.Errorf("answered %v", err)
	}
}

var (
	_ privacyrepo.Consents  = (*consentBook)(nil)
	_ privacyrepo.AiNotices = (*consentBook)(nil)
)
