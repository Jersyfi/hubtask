// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

const (
	AutoAssignWorkItemName = "AutoAssignWorkItem"

	// The stable codes of this use case. `unavailable` refuses an explicit ask that nothing can
	// answer - the collection has no policy - where `no_candidate` is not a refusal at all: the
	// policy ran, nobody was eligible, and the entry stays as it was with the reason in the
	// result (C-02's acceptance).
	autoAssignUnavailable = "items.auto_assign_unavailable"
	autoAssignNoCandidate = "items.auto_assign_no_candidate"
)

// AutoAssignWorkItem hands an entry out by the collection's assignment policy instead of a named
// person (domain-model.md §3.6).
//
// The split of work between the layers is the strategy port's contract: this use case resolves
// the material - which candidates may actually receive the entry, how loaded they are, where the
// rotation stands - and the domain's strategy only chooses. Eligibility is the same question the
// manual assignment asks about its one account, asked about every candidate: somebody who cannot
// see the entry is skipped, not assigned, and never named in an answer (T-04).
type AutoAssignWorkItem struct {
	Assignment AssignmentWriter
	Policies   repository.AutoAssignPolicies
	Groups     identityrepo.Groups
	Random     clock.RandomSource
}

// AutoAssignOutcome is what the run did, beside the entry it did it to. A result rather than an
// error, because "the policy found nobody" is an answer, not a failure: the create that carries
// the same machinery must not fail the creation over it, and the two callers should not read two
// different shapes.
type AutoAssignOutcome struct {
	Assigned bool
	Strategy domain.AutoAssignStrategy
	// Code is the stable reason when nobody was assigned, empty otherwise.
	Code string
}

// Execute hands the entry out and returns it together with what happened.
func (h AutoAssignWorkItem) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd AssignmentCommand,
) (domain.WorkItem, AutoAssignOutcome, error) {
	if cmd.ItemID.IsZero() {
		return domain.WorkItem{}, AutoAssignOutcome{}, shared.ErrValidation.
			WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	w := h.Assignment
	subject, collection, err := w.readCollectionOf(ctx, actor, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems,
		Path:       containerPath(collection),
		Action:     ItemAssignedAction,
		TokenScope: itemsWrite,
		TargetType: itemTarget,
		TargetID:   cmd.ItemID,
		On:         changing(subject),
	}); err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}

	if collection.AutoAssign == nil {
		// An explicit ask that nothing can answer. A refusal rather than a quiet no-op: the
		// caller asked for a policy to run, and "there is none" is a configuration fact they can
		// fix, not an outcome of running one.
		return domain.WorkItem{}, AutoAssignOutcome{}, autoAssignUnavailableError()
	}

	// The pool is filtered before the write transaction, for the reason ensureAccountCanSee runs
	// there on the manual path: eligibility reads through the authorisation service, which opens
	// its own transactions. A candidate who loses access between here and the commit is the same
	// race the manual assignment accepts about its one account.
	pool, err := h.eligible(ctx, actor, collection, *collection.AutoAssign)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}

	var changed domain.WorkItem
	var outcome AutoAssignOutcome
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, applied, err := h.apply(ctx, actor, cmd, pool)
		changed, outcome = item, applied
		return err
	})
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	return changed, outcome, nil
}

// eligiblePool is the policy's candidates filtered to who may actually receive the entry, in the
// forms the strategies consume.
type eligiblePool struct {
	strategy domain.AutoAssignStrategy
	// accounts and positions are the account-strategy material: the eligible accounts in the
	// policy's order, each with its index in the configured list.
	accounts  []shared.ID
	positions []int
	// eligible is the same set as a lookup, for the rotation, whose walk follows the locked row
	// rather than this snapshot.
	eligible map[shared.ID]bool
	// groups is RANDOM_GROUP_MEMBER's material instead.
	groups []service.AssignmentGroup
}

// eligible resolves the policy's candidate list against who may see the collection. Group
// candidates are resolved to their members here, at draw time rather than at definition time,
// because a team's membership moves and the policy should follow it (domain-model.md §3.6).
func (h AutoAssignWorkItem) eligible(
	ctx context.Context, actor appshared.ActorContext, collection domain.Container,
	definition domain.AutoAssignDefinition,
) (eligiblePool, error) {
	w := h.Assignment
	path := containerPath(collection)
	pool := eligiblePool{strategy: definition.Strategy, eligible: map[shared.ID]bool{}}

	for position, candidate := range definition.Candidates {
		switch candidate.Kind {
		case domain.CandidateAccount:
			seen, err := w.Visibility.CanSee(ctx, actor, candidate.ID, path)
			if err != nil {
				return eligiblePool{}, err
			}
			if !seen {
				continue
			}
			pool.accounts = append(pool.accounts, candidate.ID)
			pool.positions = append(pool.positions, position)
			pool.eligible[candidate.ID] = true

		case domain.CandidateGroup:
			members, err := h.groupMembers(ctx, actor, candidate.ID)
			if err != nil {
				return eligiblePool{}, err
			}
			seen := make([]shared.ID, 0, len(members))
			for _, member := range members {
				visible, err := w.Visibility.CanSee(ctx, actor, member, path)
				if err != nil {
					return eligiblePool{}, err
				}
				if visible {
					seen = append(seen, member)
				}
			}
			if len(seen) == 0 {
				// A group with nobody eligible is out of the draw entirely, not an empty pocket
				// the draw could land in (see the strategy).
				continue
			}
			pool.groups = append(pool.groups, service.AssignmentGroup{
				GroupID: candidate.ID, Members: seen,
			})
		}
	}
	return pool, nil
}

// groupMembers reads one candidate group's members. A group that is gone - deleted since the
// policy named it - contributes nobody rather than an error: the policy follows the tenant's
// structure, and a stale reference is a candidate who cannot receive work, not a broken policy.
func (h AutoAssignWorkItem) groupMembers(
	ctx context.Context, actor appshared.ActorContext, groupID shared.ID,
) ([]shared.ID, error) {
	var members []shared.ID
	err := h.Assignment.UnitOfWork.WithinReadOnly(
		ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			found, err := h.Groups.Members(ctx, groupID)
			members = found
			return err
		})
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return nil, err
	}
	return members, nil
}

// apply runs inside the write transaction: re-read, guard, choose, and - when somebody was
// chosen - write the assignment with its strategy and advance the rotation under its lock.
func (h AutoAssignWorkItem) apply(
	ctx context.Context, actor appshared.ActorContext, cmd AssignmentCommand, pool eligiblePool,
) (domain.WorkItem, AutoAssignOutcome, error) {
	w := h.Assignment
	now := w.Clock.Now()

	item, err := findItem(ctx, w.Items, cmd.ItemID)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	collection, err := findCollection(ctx, w.Containers, item.CollectionID)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	if err := collection.EnsureAcceptsItems(); err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	profile, err := profileOf(ctx, w.Profiles, item.Type)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	if err := item.EnsureAssignable(profile); err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}

	choice, chosen, locked, err := h.choose(ctx, collection, pool)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}
	if !chosen {
		// Nobody eligible: the entry stays as it is and the result says why, with a stable code
		// rather than a failure (C-02's acceptance). The If-Match is still honoured - the state
		// the caller reasoned about has to be the state that is there, outcome or not.
		if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
			return domain.WorkItem{}, AutoAssignOutcome{}, err
		}
		return item, AutoAssignOutcome{
			Strategy: pool.strategy, Code: autoAssignNoCandidate,
		}, nil
	}

	outcome := AutoAssignOutcome{Assigned: true, Strategy: pool.strategy}
	if item.AssigneeID == choice.AccountID {
		// The policy picked whoever already has it. Nothing is written and nothing is announced,
		// exactly like the manual repeat - and the rotation does not advance, because no
		// assignment happened to spend the turn on.
		if err := ensureExpectedVersion(item, cmd.ExpectedVersion); err != nil {
			return domain.WorkItem{}, AutoAssignOutcome{}, err
		}
		return item, outcome, nil
	}

	written, err := w.write(ctx, actor, item, item.Assigned(choice.AccountID, now),
		cmd.ExpectedVersion, profile, assigning, now, pool.strategy)
	if err != nil {
		return domain.WorkItem{}, AutoAssignOutcome{}, err
	}

	if choice.Advanced {
		locked.State.Cursor = choice.NextCursor
		if err := h.Policies.SaveState(ctx, locked); err != nil {
			return domain.WorkItem{}, AutoAssignOutcome{}, err
		}
	}
	return written, outcome, nil
}

// choose builds the selection the strategy consumes and lets it pick.
//
// The rotation is the one strategy that reads through the lock: its cursor and the candidate
// order it indexes come off the row held FOR UPDATE, so two assignments arriving together queue
// rather than both spending the same turn - the eligibility snapshot only says who is in, never
// where the rotation stands.
func (h AutoAssignWorkItem) choose(
	ctx context.Context, collection domain.Container, pool eligiblePool,
) (service.AssignmentChoice, bool, domain.AutoAssignPolicy, error) {
	strategy, err := service.StrategyFor(pool.strategy)
	if err != nil {
		return service.AssignmentChoice{}, false, domain.AutoAssignPolicy{}, err
	}

	selection := service.AssignmentSelection{
		Accounts:       pool.accounts,
		Positions:      pool.positions,
		CandidateCount: len(pool.accounts),
		Groups:         pool.groups,
		Random:         h.Random,
	}
	var locked domain.AutoAssignPolicy

	switch pool.strategy {
	case domain.AssignRoundRobin:
		locked, err = h.Policies.Lock(ctx, domain.AutoAssignScopeCollection, collection.ID)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// The policy was removed between the read and the lock. Nobody is chosen - the
				// same shape as an empty pool, and deliberately not a failure: on the create
				// path this races a configuration change, and the creation must not lose to it.
				return service.AssignmentChoice{}, false, domain.AutoAssignPolicy{}, nil
			}
			return service.AssignmentChoice{}, false, domain.AutoAssignPolicy{}, err
		}
		// The walk follows the locked row: the configured order and the cursor are what the lock
		// protects, and the eligibility snapshot is only consulted per candidate.
		selection.Accounts, selection.Positions = nil, nil
		for position, candidate := range locked.Candidates {
			if pool.eligible[candidate.ID] {
				selection.Accounts = append(selection.Accounts, candidate.ID)
				selection.Positions = append(selection.Positions, position)
			}
		}
		selection.CandidateCount = len(locked.Candidates)
		selection.Cursor = locked.State.Cursor

	case domain.AssignLeastLoaded:
		load, err := h.Assignment.Items.CountOpenByAssignee(ctx, pool.accounts)
		if err != nil {
			return service.AssignmentChoice{}, false, domain.AutoAssignPolicy{}, err
		}
		selection.OpenItems = load
	}

	choice, ok := strategy.Choose(selection)
	return choice, ok, locked, nil
}

func autoAssignUnavailableError() error {
	return shared.ErrValidation.
		WithDetail(autoAssignUnavailable).
		WithFields(shared.FieldError{Path: "/item_id", Code: autoAssignUnavailable})
}

// output is the entry plus what the run did, in the field names of the contract
// (schema AutoAssignOutcome).
func (o AutoAssignOutcome) output(item usecase.Output) usecase.Output {
	outcome := map[string]any{
		"assigned": o.Assigned,
		"strategy": string(o.Strategy),
	}
	if o.Code != "" {
		outcome["code"] = o.Code
	}
	item["auto_assign"] = outcome
	return item
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h AutoAssignWorkItem) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: AutoAssignWorkItemName,
		Summary: "Hands an entry out by the collection's assignment policy - FIXED, " +
			"RANDOM_MEMBER, RANDOM_GROUP_MEMBER, ROUND_ROBIN or LEAST_LOADED - instead of a " +
			"named person. Refused when the collection has no policy. Candidates who cannot see " +
			"the entry are skipped; with nobody eligible the entry stays as it is and the " +
			"result's auto_assign object says so with a stable code. An explicit call uses the " +
			"policy whether or not it is enabled - enabled only decides whether it applies " +
			"itself to what is created in the collection.",
		SideEffects: "Writes the assignee, announces " + string(event.ItemAssigned) +
			" carrying the strategy, records the change for offline clients, writes an audit " +
			"entry and a step of the entry's history, and advances the rotation state of a " +
			"ROUND_ROBIN policy.",
		TokenScope: itemsWrite,
		Input:      assignmentInput("The entry to hand out by the collection's policy."),
		Audit: usecase.AuditDeclaration{
			Action: ItemAssignedAction, TargetType: itemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "it writes the history all the same - the entry lands on somebody, and the " +
				"shared write path records `item.assigned` exactly as a manual assignment does " +
				"(domain-model.md §3.5 keeps assignment one verb). The verb is declared by " +
				"AssignWorkItem; declaring it here too would be two use cases claiming one " +
				"sentence, which the vocabulary gate rightly refuses.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h AutoAssignWorkItem) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	cmd, err := assignmentCommand(in)
	if err != nil {
		return nil, err
	}

	item, outcome, err := h.Execute(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return outcome.output(itemOutput(item)), nil
}
