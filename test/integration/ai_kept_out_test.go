// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	integrationservice "github.com/Jersyfi/hubtask/core/application/service/integration"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	privacydomain "github.com/Jersyfi/hubtask/core/domain/model/privacy"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	portai "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/queue"
	aiadapter "github.com/Jersyfi/hubtask/infrastructure/ai"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
)

// PH-03's acceptance (UC-PRV-05 checks 5, 7, 9, 10): an objecting person's comment never reaches a
// prompt the adapter receives. Against PostgreSQL as the application role, through the real
// readers, the real exclusion and the real producer, with a provider that records every request
// it is handed - what is asserted is what reached the adapter, not what a builder meant to send.

// recordingProvider is the adapter boundary: every completion and every embedding input, kept.
type recordingProvider struct {
	mu     sync.Mutex
	inputs []string
}

func (r *recordingProvider) For(context.Context, appshared.ActorContext) (portai.Provider, error) {
	return r, nil
}

func (r *recordingProvider) Complete(_ context.Context, request portai.CompletionRequest) (portai.CompletionResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, message := range request.Messages {
		if message.Role == portai.RoleUser {
			r.inputs = append(r.inputs, message.Content)
		}
	}
	return portai.CompletionResult{
		Text: `{"notes": "A summary.", "title": "A title"}`, Model: "recorder",
		PromptID: request.PromptID, PromptVersion: request.PromptVersion, ProducedAt: time.Now().UTC(),
	}, nil
}

func (r *recordingProvider) Embed(_ context.Context, texts []string) (portai.EmbeddingResult, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.inputs = append(r.inputs, texts...)
	vectors := make([][]float32, len(texts))
	for i := range vectors {
		vectors[i] = []float32{1, 0, 0}
	}
	return portai.EmbeddingResult{Vectors: vectors, Model: "recorder", Dimensions: 3}, nil
}

func (r *recordingProvider) Capabilities() portai.ProviderCapabilities {
	return portai.ProviderCapabilities{Kind: "RECORDER", Completion: true, Embedding: true,
		CompletionModel: "recorder", EmbeddingModel: "recorder", EmbeddingDimensions: 3}
}

func (r *recordingProvider) take() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	taken := r.inputs
	r.inputs = nil
	return taken
}

// keptWorld is two people: Ada asks, Kim objects. Kim commented on Ada's entry, created an entry
// of her own, and is named in Ada's text; Kim's comment names Ada.
type keptWorld struct {
	tenant, ada, kim             shared.ID
	hub, collection              shared.ID
	adaEntry, kimEntry, empty    shared.ID
	kimComment, adaComment       string
	kimTitle, kimNotes, adaTitle string
}

const (
	kimName  = "Kim Keeper"
	kimMail  = "kim.keeper@example.org"
	kimLogin = "kimk"
)

func seedKeptWorld(ctx context.Context, t *testing.T) keptWorld {
	t.Helper()
	w := keptWorld{
		tenant: freshID(t), ada: freshID(t), kim: freshID(t),
		adaEntry: freshID(t), kimEntry: freshID(t), empty: freshID(t),
		kimComment: "KIMSAYS the venue is booked, ask Ada",
		adaComment: "ADASAYS thanks Kim, and Kimberley agrees",
		kimTitle:   "KIMOWN secret plan", kimNotes: "KIMNOTES nobody else wrote this",
		adaTitle: "Summer party with Kim",
	}
	admin := adminPool(ctx, t)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO tenant (id, slug, display_name) VALUES ($1, $2, 'Kept out')`,
			[]any{w.tenant.String(), slugOf(w.tenant)}},
		{`INSERT INTO account (id, tenant_id, email, display_name) VALUES ($1, $2, 'ada@example.org', 'Ada Asker')`,
			[]any{w.ada.String(), w.tenant.String()}},
		{`INSERT INTO account (id, tenant_id, email, display_name, sign_in_name) VALUES ($1, $2, $3, $4, $5)`,
			[]any{w.kim.String(), w.tenant.String(), kimMail, kimName, kimLogin}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'OWNER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.ada.String()}},
		{`INSERT INTO membership (id, tenant_id, account_id, scope_type, role) VALUES ($1, $2, $3, 'TENANT', 'MEMBER')`,
			[]any{freshID(t).String(), w.tenant.String(), w.kim.String()}},
	} {
		if _, err := admin.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding: %v\n%s", err, statement.sql)
		}
	}
	w.hub, w.collection = hubWithCollection(ctx, t, w.tenant, w.ada)
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		ada := taskIn(w.tenant, w.ada, w.collection, w.adaEntry, w.adaTitle, "a0")
		ada.Notes = "ADANOTES " + kimName + " brings drinks, write to " + kimMail
		if err := itemRepo().Insert(ctx, ada); err != nil {
			return err
		}
		kim := taskIn(w.tenant, w.kim, w.collection, w.kimEntry, w.kimTitle, "a1")
		kim.Notes = w.kimNotes
		if err := itemRepo().Insert(ctx, kim); err != nil {
			return err
		}
		// An entry nobody has discussed: a thread with nothing to work with.
		return itemRepo().Insert(ctx, taskIn(w.tenant, w.ada, w.collection, w.empty, "Quiet entry", "a2"))
	}); err != nil {
		t.Fatalf("seeding the entries: %v", err)
	}
	for author, body := range map[shared.ID]string{w.kim: w.kimComment, w.ada: w.adaComment} {
		if _, err := admin.Exec(ctx,
			`INSERT INTO comment (id, tenant_id, item_id, author_id, body) VALUES ($1, $2, $3, $4, $5)`,
			freshID(t).String(), w.tenant.String(), w.adaEntry.String(), author.String(), body); err != nil {
			t.Fatalf("seeding a comment: %v", err)
		}
	}
	return w
}

// keptOutExclusion is the exclusion the way main.go builds it.
func keptOutExclusion() integrationservice.Exclusion {
	rules := &integrationservice.AiRules{
		Instance: postgres.NewInstanceSettingRepository(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
		Offered: postgres.NewOfferedAiRepository(), Choices: postgres.NewAiChoiceRepository(),
		Own: postgres.NewAiProviderRepository(),
	}
	return integrationservice.Exclusion{Sources: []integrationservice.KeptOutSource{
		integrationservice.RestrictionSource{Restrictions: privacyRepo()},
		integrationservice.ObjectionSource{
			Rules: rules, People: postgres.NewPrivateHubRepository(), Consents: privacyRepo(),
		},
	}}
}

func (w keptWorld) as(account shared.ID) appshared.ActorContext {
	return appshared.ActorContext{
		Kind: appshared.ActorUser, TenantID: w.tenant, AccountID: account, AccountName: "Someone",
		Locale: "en", TimeZone: "UTC",
		Scopes: []string{"items:read", "containers:read", "comments:read", "items:write", "suggestions:write"},
	}
}

func (w keptWorld) record(ctx context.Context, t *testing.T, granted bool, at time.Time) {
	t.Helper()
	consent := privacydomain.Consent{
		ID: freshID(t), AccountID: w.kim, Purpose: privacydomain.PurposeAi,
		Granted: granted, GrantedAt: at, Source: privacydomain.SourceUser,
	}
	if !granted {
		consent.RevokedAt = at
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error { return privacyRepo().Record(ctx, consent) }); err != nil {
		t.Fatalf("recording Kim's consent: %v", err)
	}
}

func (w keptWorld) position(ctx context.Context, t *testing.T, notOffered bool) {
	t.Helper()
	settings := `{"ai_person_opt_out": "OFFERED", "ai_person_opt_out_basis": "", "ai_person_opt_out_since": ""}`
	if notOffered {
		settings = `{"ai_person_opt_out": "NOT_OFFERED", "ai_person_opt_out_basis": "WORKS_AGREEMENT",
			"ai_person_opt_out_since": "2026-10-12T09:00:00Z"}`
	}
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET settings = settings || $2::jsonb WHERE id = $1`, w.tenant.String(), settings); err != nil {
		t.Fatalf("setting the position: %v", err)
	}
}

// keptHarness is what asks: the producer, the translation and the embedding pass.
type keptHarness struct {
	produce   suggestion.Produce
	translate work.AiTranslate
	recorder  *recordingProvider
}

func newKeptHarness(ctx context.Context, t *testing.T) keptHarness {
	t.Helper()
	readers := readerRegistry(ctx, t)
	prompts, err := aiadapter.NewStore()
	if err != nil {
		t.Fatalf("prompts: %v", err)
	}
	recorder := &recordingProvider{}
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	exclusion := keptOutExclusion()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	store := postgres.NewSuggestionRepository(pageCursors())
	return keptHarness{
		recorder: recorder,
		produce: suggestion.Produce{
			Providers: recorder, Prompts: prompts,
			Sources: suggestion.CatalogueSources{
				Catalogue: readers, Profiles: postgres.NewCapabilityProfileRepository(), Kept: exclusion,
			},
			Suggestions: store, Requests: store,
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, IDs: ids,
		},
		translate: work.AiTranslate{
			Reader: work.GetWorkItem{
				Items: itemRepo(), ItemLabels: postgres.NewItemLabelRepository(), Containers: containerRepo(),
				Authorizer: realAuthoriser(ctx, t), UnitOfWork: unitOfWork,
			},
			Providers: recorder, Prompts: prompts, Audit: postgres.NewAuditSink(ids),
			UnitOfWork: unitOfWork, Clock: clockadapter.System{}, Kept: exclusion,
		},
	}
}

// askEverything asks every question a colleague can ask about the world, as Ada, and answers what
// reached the adapter.
func (h keptHarness) askEverything(ctx context.Context, t *testing.T, w keptWorld) string {
	t.Helper()
	asker := w.as(w.ada)
	for _, request := range []suggestion.Request{
		{TargetType: suggestiondomain.TargetWorkItem, TargetID: w.adaEntry, Kind: suggestiondomain.KindFields, PromptID: "summarize-thread"},
		{TargetType: suggestiondomain.TargetWorkItem, TargetID: w.adaEntry, Kind: suggestiondomain.KindFields, PromptID: "summarize"},
		{TargetType: suggestiondomain.TargetWorkItem, TargetID: w.kimEntry, Kind: suggestiondomain.KindFields, PromptID: "summarize"},
		{TargetType: suggestiondomain.TargetWorkItem, TargetID: w.kimEntry, Kind: suggestiondomain.KindFields, PromptID: "summarize-thread"},
		{TargetType: suggestiondomain.TargetContainer, TargetID: w.collection, Kind: suggestiondomain.KindFields, PromptID: "summarize-collection"},
	} {
		if _, err := h.produce.Ask(ctx, asker, request); err != nil {
			// A malformed answer from the recorder is beside the point: what was sent is recorded first.
			t.Logf("%s about %s: %v", request.PromptID, request.TargetID, err)
		}
	}
	for _, item := range []shared.ID{w.adaEntry, w.kimEntry} {
		if _, err := h.translate.Execute(ctx, asker, work.TranslateCommand{ItemID: item, TargetLocale: "de"}); err != nil {
			t.Logf("translating %s: %v", item, err)
		}
	}
	return strings.Join(h.recorder.take(), "\n----\n")
}

// kimTraces is everything of Kim's that must never reach the adapter: what she wrote, and every
// spelling of her.
func (w keptWorld) kimTraces() []string {
	return []string{"KIMSAYS", "KIMOWN", "KIMNOTES", kimName, kimMail, kimLogin, "Keeper"}
}

func TestAnObjectingPersonsCommentNeverReachesAPrompt(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	h := newKeptHarness(ctx, t)
	start := time.Now().UTC().Add(-time.Hour)

	// Before anything: everything is sent, Kim's comment included - the test can see it.
	sent := h.askEverything(ctx, t, w)
	if !strings.Contains(sent, "KIMSAYS") || !strings.Contains(sent, "KIMOWN") {
		t.Fatalf("before the objection Kim's content did not reach the adapter, so the test proves nothing:\n%s", sent)
	}

	// Kim keeps her content out.
	w.record(ctx, t, false, start)
	sent = h.askEverything(ctx, t, w)
	for _, trace := range w.kimTraces() {
		if strings.Contains(strings.ToLower(sent), strings.ToLower(trace)) {
			t.Errorf("%q reached the adapter after Kim objected:\n%s", trace, sent)
		}
	}
	for _, own := range []string{"ADASAYS", "ADANOTES", "[person]", "Kimberley"} {
		if !strings.Contains(sent, own) {
			t.Errorf("%q is missing: Ada's own content must still be sent, with Kim's name replaced:\n%s", own, sent)
		}
	}

	// Kim lets her content back in: her comment is sent again.
	w.record(ctx, t, true, start.Add(time.Minute))
	if sent = h.askEverything(ctx, t, w); !strings.Contains(sent, "KIMSAYS") {
		t.Errorf("after Kim gave consent again her comment did not reach the adapter:\n%s", sent)
	}

	// Kim withdraws again, and the workspace makes AI part of everybody's work: her withdrawal
	// stops taking effect (check 9).
	w.record(ctx, t, false, start.Add(2*time.Minute))
	w.position(ctx, t, true)
	if sent = h.askEverything(ctx, t, w); !strings.Contains(sent, "KIMSAYS") {
		t.Errorf("under NOT_OFFERED Kim's earlier withdrawal still took effect:\n%s", sent)
	}
	w.position(ctx, t, false)
	if sent = h.askEverything(ctx, t, w); strings.Contains(sent, "KIMSAYS") {
		t.Errorf("back to OFFERED, Kim's standing withdrawal did not take effect again:\n%s", sent)
	}
}

// Check 10: a formal Art. 21 objection is recorded as a case and decided by the controller;
// recording it switches nothing off - Kim's comment still reaches the adapter.
func TestAFormalObjectionCaseSwitchesNothingOff(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	h := newKeptHarness(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`INSERT INTO data_subject_request (id, tenant_id, kind, subject_account_id, status, received_at, due_at)
		 VALUES ($1, $2, 'OBJECTION', $3, 'COMPLETED', now(), now() + interval '30 days')`,
		freshID(t).String(), w.tenant.String(), w.kim.String()); err != nil {
		t.Fatalf("recording the objection case: %v", err)
	}
	if sent := h.askEverything(ctx, t, w); !strings.Contains(sent, "KIMSAYS") {
		t.Errorf("an objection case switched Kim's content off by itself:\n%s", sent)
	}
}

// Check 8 against the database: alone in a workspace a withdrawal keeps nothing out - the
// workspace's decision is the person's own.
func TestAWithdrawalAloneKeepsNothingOut(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	if _, err := adminPool(ctx, t).Exec(ctx, `UPDATE account SET deleted_at = now() WHERE id = $1`, w.ada.String()); err != nil {
		t.Fatalf("leaving Kim alone: %v", err)
	}
	w.record(ctx, t, false, time.Now().UTC())
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		kept, err := keptOutExclusion().Read(ctx, w.tenant)
		if err != nil {
			return err
		}
		if kept.Has(w.kim) {
			t.Error("alone, Kim's withdrawal kept her own content out of her own AI")
		}
		return nil
	}); err != nil {
		t.Fatalf("reading: %v", err)
	}
}

// F1: nothing a third person is answered names or implies the objector. Asking about Kim's entry's
// discussion answers what asking about an undiscussed entry answers, word for word, and asking for
// a summary of Kim's entry answers the same words; asking about Ada's thread succeeds and says
// nothing about anybody left out. Kim herself is told her own choice.
func TestNoAnswerToAColleagueNamesOrImpliesTheObjector(t *testing.T) {
	ctx := context.Background()
	w := seedKeptWorld(ctx, t)
	w.record(ctx, t, false, time.Now().UTC().Add(-time.Minute))
	readers := readerRegistry(ctx, t)
	unitOfWork := postgres.NewUnitOfWork(appPool(ctx, t))
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	queued := &keptQueue{}
	cases := suggestion.Cases{
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Targets:     suggestion.EntryTargets{Catalogue: readers},
		Authorizer:  realAuthoriser(ctx, t), Catalogue: readers,
		Audit: postgres.NewAuditSink(ids), UnitOfWork: unitOfWork, Clock: clockadapter.System{},
		Material: suggestion.CatalogueSources{Catalogue: readers, Kept: keptOutExclusion()},
	}
	registry, err := usecase.NewRegistry(nil,
		suggestion.AiSummarize{Cases: cases, AI: aiAvailable{}, Queue: queued}.Descriptor(),
		suggestion.AiSummarizeThread{Cases: cases, AI: aiAvailable{}, Queue: queued}.Descriptor(),
	)
	if err != nil {
		t.Fatalf("building: %v", err)
	}
	ask := func(name string, actor appshared.ActorContext, item shared.ID) (string, error) {
		out, err := registry.Invoke(ctx, name, actor, usecase.Input{"item_id": item.String()})
		encoded, _ := json.Marshal(out)
		if err != nil {
			encoded, _ = json.Marshal(shared.AsError(err))
		}
		return string(encoded), err
	}

	aboutKims, kimsErr := ask(suggestion.AiSummarizeThreadName, w.as(w.ada), w.kimEntry)
	aboutEmpty, emptyErr := ask(suggestion.AiSummarizeThreadName, w.as(w.ada), w.empty)
	if got := shared.AsError(kimsErr); got == nil || got.DetailCode != "ai.nothing_to_work_with" {
		t.Fatalf("asking about Kim's entry answered %v", kimsErr)
	}
	if emptyErr == nil || aboutKims != aboutEmpty || kimsErr.Error() != emptyErr.Error() {
		t.Errorf("the two refusals differ, so one tells who wrote it:\n%s\n%s", aboutKims, aboutEmpty)
	}
	summary, summaryErr := ask(suggestion.AiSummarizeName, w.as(w.ada), w.kimEntry)
	if summaryErr == nil || summary != aboutEmpty {
		t.Errorf("a summary of Kim's entry answered differently from nothing to work with: %s", summary)
	}

	thread, err := ask(suggestion.AiSummarizeThreadName, w.as(w.ada), w.adaEntry)
	if err != nil {
		t.Fatalf("a thread with Ada's comment left is not refused: %v", err)
	}
	for _, answer := range []string{aboutKims, thread, queued.payloads()} {
		for _, trace := range []string{w.kim.String(), kimName, kimMail, kimLogin, "kept", "objec"} {
			if strings.Contains(strings.ToLower(answer), strings.ToLower(trace)) {
				t.Errorf("an answer to Ada carries %q: %s", trace, answer)
			}
		}
	}

	if _, err := ask(suggestion.AiSummarizeName, w.as(w.kim), w.adaEntry); shared.AsError(err) == nil ||
		shared.AsError(err).DetailCode != "ai.kept_out" {
		t.Errorf("Kim asking answered %v, want ai.kept_out", err)
	}
}

type aiAvailable struct{}

func (aiAvailable) CanSuggest(context.Context, appshared.ActorContext) (bool, error) {
	return true, nil
}

type keptQueue struct{ requests []queue.Request }

func (q *keptQueue) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	return shared.ID(""), nil
}

func (q *keptQueue) payloads() string {
	encoded, _ := json.Marshal(q.requests)
	return string(encoded)
}
