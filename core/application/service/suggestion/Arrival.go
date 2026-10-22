// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package suggestion

import (
	"context"
	"strings"
	"time"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	jumble "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/suggestion"
	aiprovider "github.com/Jersyfi/hubtask/core/port/ai"
)

// The outcomes only a proposal prepared on arrival ends with (UC-JUM-07 check 10). Nobody follows
// such a job but the card of the entry it is about, and every one of them leaves the arrival as it
// was: waiting for somebody to ask.
const (
	// OutcomeArrivalOff: the workspace switched proposals on arrival off between the arrival and
	// the run.
	OutcomeArrivalOff = "ai.arrival_proposals_off"
	// OutcomeArrivalCapReached: the day's number of arrivals asked about on arrival was reached
	// (`ai_arrival_proposals_per_day`).
	OutcomeArrivalCapReached = "ai.arrival_cap_reached"
	// OutcomeEntrySettled: somebody converted or dismissed the entry before the run reached it.
	OutcomeEntrySettled = "jumble.entry_already_settled"
)

// arrivalPrompt is the question asked about an arrival: the one a person asks about a jumble entry.
const arrivalPrompt = "suggest-fields"

// Arrival is what a proposal prepared on arrival is made from, read in one transaction.
type Arrival struct {
	// Prepare is the workspace's switch as it stands at the run.
	Prepare bool
	// Settled is true where the entry is no longer undecided.
	Settled bool
	// Material is the entry's text with the shared set, or no content where nothing may be sent.
	Material Material
	// TimeZone is the workspace's default zone: the system asks, and a system lives nowhere.
	TimeZone string
}

// ArrivalSource reads an arrival for the workspace, as the system acting for it.
type ArrivalSource interface {
	Arrival(ctx context.Context, request Request) (Arrival, error)
}

// ArrivalCap takes one of the day's proposals on arrival, inside the caller's transaction.
type ArrivalCap interface {
	ArrivalProposal(ctx context.Context, tenant string, now time.Time) (bool, error)
}

// EntryFinder reads one jumble entry inside the caller's transaction, judging no permission.
type EntryFinder interface {
	Find(ctx context.Context, id shared.ID) (jumble.Entry, error)
}

// WorkspaceFinder reads the transaction's workspace.
type WorkspaceFinder interface {
	Find(ctx context.Context) (identity.Workspace, error)
}

// ArrivalSources reads an arrival through the repositories rather than the catalogue.
//
// The one producer that does, and for the embedding pass's reason: the asker is the workspace,
// acting as the system (ai-first.md §2), and a system actor holds no role a catalogue read could
// be authorised against. What it may read is decided by the switch the workspace set under
// `STRUCTURE`, and what it reads is the workspace's own jumble - which every member who may read the
// workspace reads - its default zone, and the shared set of collections.
type ArrivalSources struct {
	Entries     EntryFinder
	Workspaces  WorkspaceFinder
	Collections workrepo.Collections
	// Kept is whose content stays out of AI, read in the same transaction (data-protection.md
	// §4.1). Nil keeps nobody out - a build without the rule.
	Kept KeptOutReader
}

var _ ArrivalSource = ArrivalSources{}

// Arrival reads the switch, the entry, the exclusion and the shared set together.
func (s ArrivalSources) Arrival(ctx context.Context, request Request) (Arrival, error) {
	workspace, err := s.Workspaces.Find(ctx)
	if err != nil {
		return Arrival{}, err
	}
	arrival := Arrival{
		Prepare:  workspace.Settings.AiArrivalProposals,
		TimeZone: workspace.DefaultTimeZone,
	}
	if !arrival.Prepare {
		return arrival, nil
	}
	entry, err := s.Entries.Find(ctx, request.TargetID)
	if err != nil {
		return Arrival{}, err
	}
	if entry.Status != jumble.StatusNew {
		arrival.Settled = true
		return arrival, nil
	}

	var kept integration.KeptOut
	if s.Kept != nil {
		if kept, err = s.Kept.Read(ctx, workspace.ID); err != nil {
			return Arrival{}, err
		}
	}
	// Who submitted it, where an account did. For a mail or a webhook arrival nobody did, and the
	// sender address `entryMaterial` checks is the only signal there is.
	if kept.Has(request.SubmittedBy) || kept.Has(request.OnBehalfOf) {
		arrival.Material = Material{Digest: domain.Digest(entry.RawSubject, entry.RawBody), Kept: kept}
		return arrival, nil
	}
	material := entryMaterial(entry.RawSubject, entry.RawBody, entry.Sender, kept)
	material.Kept = kept
	if arrival.Material, err = withDestinations(ctx, s.Collections, material, arrivalPrompt, kept); err != nil {
		return Arrival{}, err
	}
	return arrival, nil
}

// askArrival is a proposal the workspace prepares about one arrival (UC-JUM-07 check 10), asked as
// the system for it.
//
// Every judgement is made here, at the run, rather than where the arrival was seeded: the switch,
// whether the entry is still undecided, whose content stays out, the source and its consent, the
// day's cap and the token budget may each have changed since, and each ends the job with its own
// outcome. In that order, so that nothing is counted against the cap for an arrival that could not
// have been asked about anyway, and nothing is sent for one past it.
func (h Produce) askArrival(
	ctx context.Context, actor appshared.ActorContext, request Request,
) (Outcome, error) {
	if h.Arrivals == nil {
		return Outcome{}, shared.ErrInternal.WithDetail("suggestions.arrivals_not_wired")
	}
	prompt, err := h.Prompts.Get(arrivalPrompt)
	if err != nil {
		return Outcome{}, err
	}

	var arrival Arrival
	if err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		read, err := h.Arrivals.Arrival(ctx, request)
		arrival = read
		return err
	}); err != nil {
		return Outcome{}, err
	}
	switch {
	case !arrival.Prepare:
		return Outcome{Code: OutcomeArrivalOff}, nil
	case arrival.Settled:
		return Outcome{Code: OutcomeEntrySettled}, nil
	case strings.TrimSpace(arrival.Material.Content) == "":
		return Outcome{Code: OutcomeNothingToWorkWith}, nil
	}
	actor.TimeZone = arrival.TimeZone

	provider, err := h.Providers.For(ctx, actor)
	if err != nil {
		return Outcome{}, err
	}
	if !provider.Capabilities().Completion {
		return Outcome{}, aiprovider.RefusalOf(provider)
	}

	if h.Cap != nil {
		var room bool
		if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			room, err = h.Cap.ArrivalProposal(ctx, actor.TenantID.String(), h.Clock.Now())
			return err
		}); err != nil {
			return Outcome{}, err
		}
		if !room {
			return Outcome{Code: OutcomeArrivalCapReached}, nil
		}
	}

	answer, err := provider.Complete(ctx, prompt.Ask(withOptions(arrival.Material, h.today(actor, arrivalPrompt))))
	if err != nil {
		return Outcome{}, err
	}
	payload, ok := payloadFrom(domain.KindFields, arrivalPrompt, answer.Text, h.applicable(request), arrival.Material, h.Text)
	if !ok || len(payload) == 0 {
		return Outcome{Code: OutcomeNothingToChange}, nil
	}
	proposal, err := domain.New(domain.NewInput{
		ID: h.IDs.NewID(), TenantID: actor.TenantID,
		TargetType: domain.TargetJumbleEntry, TargetID: request.TargetID, Kind: domain.KindFields,
		Payload: payload,
		Provenance: domain.Provenance{
			Model: answer.Model, PromptID: answer.PromptID,
			PromptVersion: answer.PromptVersion, ProducedAt: answer.ProducedAt,
		},
		InputDigest: arrival.Material.Digest,
		Now:         h.Clock.Now(),
	})
	if err != nil {
		return Outcome{}, err
	}
	if err := h.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Suggestions.Record(ctx, proposal)
	}); err != nil {
		return Outcome{}, err
	}
	return Outcome{Recorded: true, Code: OutcomeProposed}, nil
}
