// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Jersyfi/hubtask/core/application/service/jumble"
	quotaservice "github.com/Jersyfi/hubtask/core/application/service/quota"
	"github.com/Jersyfi/hubtask/core/application/service/suggestion"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	jumbledomain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	suggestiondomain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	portai "github.com/Jersyfi/hubtask/core/port/ai"
	"github.com/Jersyfi/hubtask/core/port/environment"
	"github.com/Jersyfi/hubtask/core/port/queue"
	aiadapter "github.com/Jersyfi/hubtask/infrastructure/ai"
	clockadapter "github.com/Jersyfi/hubtask/infrastructure/clock"
	"github.com/Jersyfi/hubtask/infrastructure/postgres"
	"github.com/Jersyfi/hubtask/presentation/worker"
)

// UC-JUM-07 check 10 end to end against PostgreSQL as the application role: an arrival, the real
// dispatcher round with the seed, the job it queued, and the worker's run of it with the real
// arrival source, the real day's cap and the real suggestion store - only the provider is a stub.

// arrivalWorld is a workspace with a shared collection, its switch and its intake.
type arrivalWorld struct {
	jumbleWorld
	collection shared.ID
}

func seedArrivalWorld(ctx context.Context, t *testing.T) arrivalWorld {
	t.Helper()
	w := arrivalWorld{jumbleWorld: seedJumbleWorld(ctx, t)}
	_, w.collection = hubWithCollection(ctx, t, w.tenant, w.owner)
	return w
}

func (w arrivalWorld) switchTo(ctx context.Context, t *testing.T, prepare bool) {
	t.Helper()
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET settings = settings || jsonb_build_object('ai_arrival_proposals', $2::boolean) WHERE id = $1`,
		w.tenant.String(), prepare); err != nil {
		t.Fatalf("switching: %v", err)
	}
}

// arrive stores an entry and announces it in one transaction, as every door does.
func (w arrivalWorld) arrive(ctx context.Context, t *testing.T, actor event.Actor, subject string) shared.ID {
	t.Helper()
	entry, err := jumbledomain.NewEntry(jumbledomain.NewEntryInput{
		ID: freshID(t), TenantID: w.tenant, Channel: jumbledomain.ChannelWebhook,
		Sender: "orders@example.org", RawSubject: subject, RawBody: "Please ship by Friday.",
		Now: time.Now().UTC().Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("building the entry: %v", err)
	}
	if err := write(ctx, t, w.tenant, func(ctx context.Context) error {
		if err := jumbleRepo().Insert(ctx, entry); err != nil {
			return err
		}
		envelope, err := event.NewJumbleEntryReceived(freshID(t), entry, actor, entry.ReceivedAt, event.Cause{})
		if err != nil {
			return err
		}
		return postgres.NewOutbox(jobQueue(t)).Append(ctx, envelope)
	}); err != nil {
		t.Fatalf("arriving: %v", err)
	}
	return entry.ID
}

func (w arrivalWorld) seedRound(ctx context.Context, t *testing.T) {
	t.Helper()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	dispatchOnce(ctx, t, w.tenant, jumble.ArrivalProposals{
		Workspaces: postgres.NewWorkspaceSettingsRepository(), Jobs: jobQueue(t),
		Audit: postgres.NewAuditSink(ids), Clock: clockadapter.System{},
	})
}

// queuedFor answers the arrival job about the entry, or false.
func (w arrivalWorld) queuedFor(ctx context.Context, t *testing.T, entry shared.ID) (queue.Job, bool) {
	t.Helper()
	var id, payload string
	err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT id::text, payload::text FROM job WHERE tenant_id = $1 AND kind = 'ai.suggest_arrival' AND dedupe_key = $2`,
		w.tenant.String(), entry.String()).Scan(&id, &payload)
	if err != nil {
		return queue.Job{}, false
	}
	job := queue.Job{ID: shared.MustParseID(id), TenantID: w.tenant, Kind: queue.KindAiSuggestArrival, Attempts: 1, MaxAttempts: 20}
	if err := json.Unmarshal([]byte(payload), &job.Payload); err != nil {
		t.Fatalf("the payload: %v", err)
	}
	return job, true
}

// runner is the worker's handler as main.go wires it, around the provider given.
func (w arrivalWorld) runner(ctx context.Context, t *testing.T, providers suggestion.Providers) worker.AiSuggestionArrival {
	t.Helper()
	prompts, err := aiadapter.NewStore()
	if err != nil {
		t.Fatalf("prompts: %v", err)
	}
	store := postgres.NewQuotaRepository()
	ids := clockadapter.NewUUIDv7(clockadapter.System{})
	return worker.AiSuggestionArrival{Produce: suggestion.Produce{
		Providers: providers, Prompts: prompts,
		Suggestions: postgres.NewSuggestionRepository(pageCursors()),
		Fields:      arrivalFields{},
		UnitOfWork:  postgres.NewUnitOfWork(appPool(ctx, t)), Clock: clockadapter.System{}, IDs: ids,
		Arrivals: suggestion.ArrivalSources{
			Entries: jumbleRepo(), Workspaces: postgres.NewWorkspaceSettingsRepository(),
			Collections: containerRepo(), Kept: keptOutExclusion(),
		},
		Cap: quotaservice.Guard{Store: store, Usage: store, Meter: store, Tenancy: environment.TenancyMulti},
	}}
}

// arrivalFields is ConvertJumbleEntry's inputs, as the registry answers them.
type arrivalFields struct{}

func (arrivalFields) InputsOf(name string) ([]string, bool) {
	if name == "ConvertJumbleEntry" {
		return []string{"entry_id", "collection_id", "bucket_id", "title", "type"}, true
	}
	return nil, false
}

func TestANewEntryGetsAProposalWithoutAnybodyAsking(t *testing.T) {
	ctx := context.Background()
	w := seedArrivalWorld(ctx, t)

	// Off by default: an arrival is asked about by nobody.
	quiet := w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Before the switch")
	w.seedRound(ctx, t)
	if _, queued := w.queuedFor(ctx, t, quiet); queued {
		t.Fatal("an arrival was asked about in a workspace that never switched it on")
	}

	w.switchTo(ctx, t, true)
	entry := w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Order 42")
	w.seedRound(ctx, t)
	job, queued := w.queuedFor(ctx, t, entry)
	if !queued {
		t.Fatal("switched on, the arrival was not asked about")
	}
	if _, again := w.queuedFor(ctx, t, quiet); again {
		t.Error("an entry from before the switch was asked about")
	}

	// While it runs, the entry names the job a card follows.
	readers := jumbleReaders(ctx, t)
	read, err := readers.Invoke(ctx, jumble.GetJumbleEntryName, w.as(w.owner), usecase.Input{"entry_id": entry.String()})
	if err != nil || read.String("arrival_job_id") != job.ID.String() {
		t.Errorf("the entry names job %q (%v), want %s", read.String("arrival_job_id"), err, job.ID)
	}

	provider := &recordingProvider{}
	result, err := w.runner(ctx, t, provider).Run(ctx, job)
	if err != nil || result.Outcome != suggestion.OutcomeProposed {
		t.Fatalf("the run ended %+v, %v", result, err)
	}
	if sent := strings.Join(provider.take(), "\n"); !strings.Contains(sent, "Order 42") || !strings.Contains(sent, w.collection.String()) {
		t.Errorf("the material did not carry the entry and the shared set:\n%s", sent)
	}
	out, err := readers.Invoke(ctx, suggestion.ListSuggestionsName, w.as(w.owner), usecase.Input{
		"target_type": string(suggestiondomain.TargetJumbleEntry), "target_id": entry.String(),
	})
	if items, _ := out["items"].([]usecase.Output); err != nil || len(items) != 1 {
		t.Errorf("listed %v, %v; want the one proposal nobody asked for", out, err)
	}

	var actor string
	if err := adminPool(ctx, t).QueryRow(ctx,
		`SELECT actor_type FROM audit_log WHERE tenant_id = $1 AND action = 'jumble.suggestion_asked' AND target_id = $2`,
		w.tenant.String(), entry.String()).Scan(&actor); err != nil || actor != "SYSTEM" {
		t.Errorf("the trail's asker is %q (%v), want the system", actor, err)
	}
}

func TestEachJudgementAtAnArrivalsRunAgainstTheDatabase(t *testing.T) {
	ctx := context.Background()
	w := seedArrivalWorld(ctx, t)
	w.switchTo(ctx, t, true)

	// Switched off between the arrival and the run: nothing is sent.
	first := w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Order 1")
	w.seedRound(ctx, t)
	job, _ := w.queuedFor(ctx, t, first)
	w.switchTo(ctx, t, false)
	provider := &recordingProvider{}
	if result, err := w.runner(ctx, t, provider).Run(ctx, job); err != nil || result.Outcome != suggestion.OutcomeArrivalOff {
		t.Errorf("switched off since: %+v, %v", result, err)
	}
	if sent := provider.take(); len(sent) != 0 {
		t.Errorf("switched off, %d prompts were sent", len(sent))
	}

	// The day's cap at one: the first arrival is asked about, the second waits.
	w.switchTo(ctx, t, true)
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET settings = jsonb_set(settings, '{quotas}', '{"ai_arrival_proposals_per_day": 1}'::jsonb) WHERE id = $1`,
		w.tenant.String()); err != nil {
		t.Fatalf("setting the cap: %v", err)
	}
	second, third := w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Order 2"),
		w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Order 3")
	w.seedRound(ctx, t)
	// In order: a map's random order would let the third take the one place.
	for _, run := range []struct {
		entry shared.ID
		want  string
	}{{second, suggestion.OutcomeProposed}, {third, suggestion.OutcomeArrivalCapReached}} {
		job, _ := w.queuedFor(ctx, t, run.entry)
		if result, err := w.runner(ctx, t, &recordingProvider{}).Run(ctx, job); err != nil || result.Outcome != run.want {
			t.Errorf("under a cap of one: %+v, %v; want %s", result, err, run.want)
		}
	}

	// The token budget spent: the job ends refused, the arrival stays.
	fourth := w.arrive(ctx, t, event.Actor{Kind: shared.ActorSystem}, "Order 4")
	if _, err := adminPool(ctx, t).Exec(ctx,
		`UPDATE tenant SET settings = jsonb_set(settings, '{quotas}', '{"ai_arrival_proposals_per_day": 0}'::jsonb) WHERE id = $1`,
		w.tenant.String()); err != nil {
		t.Fatalf("lifting the cap: %v", err)
	}
	w.seedRound(ctx, t)
	job, _ = w.queuedFor(ctx, t, fourth)
	result, err := w.runner(ctx, t, spentProvider{}).Run(ctx, job)
	if err != nil || result.Refusal == nil || result.Refusal.Params["reason"] != portai.ReasonBudgetSpent {
		t.Errorf("the budget spent: %+v, %v", result, err)
	}
	if err := read(ctx, t, w.tenant, func(ctx context.Context) error {
		stored, err := jumbleRepo().Find(ctx, fourth)
		if err == nil && stored.Status != jumbledomain.StatusNew {
			t.Errorf("the arrival is %s after the refusal", stored.Status)
		}
		return err
	}); err != nil {
		t.Fatalf("reading the arrival: %v", err)
	}
}

// spentProvider is a workspace whose day's tokens are gone.
type spentProvider struct{}

func (spentProvider) For(context.Context, appshared.ActorContext) (portai.Provider, error) {
	return spentRefusal{}, nil
}

type spentRefusal struct{}

func (spentRefusal) Capabilities() portai.ProviderCapabilities {
	return portai.ProviderCapabilities{Kind: "noop"}
}
func (spentRefusal) Refusal() error { return portai.BudgetSpent(time.Now().Add(time.Hour)) }
func (s spentRefusal) Complete(context.Context, portai.CompletionRequest) (portai.CompletionResult, error) {
	return portai.CompletionResult{}, s.Refusal()
}
func (s spentRefusal) Embed(context.Context, []string) (portai.EmbeddingResult, error) {
	return portai.EmbeddingResult{}, s.Refusal()
}
