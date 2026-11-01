// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The approvals' use cases (UC-SVC-08 to 10).
const (
	DecideApprovalName         = "DecideApproval"
	ListMyApprovalsName        = "ListMyApprovals"
	ReadEntryApprovalsName     = "ReadEntryApprovals"
	SetApprovalDelegationName  = "SetApprovalDelegation"
	ReadApprovalDelegationName = "ReadApprovalDelegation"
	EndApprovalDelegationName  = "EndApprovalDelegation"

	approvalTarget   = "approval"
	delegationTarget = "approval_delegation"
	takeStepUseCase  = "TakeStep"
	itemsWriteScope  = "items:write"
	// pendingPage is one read of the waiting approvals that may be a reader's; maxPendingPages
	// bounds how many one list walks through (rule 7's spirit: no read without an end).
	pendingPage     = 200
	maxPendingPages = 25
)

const (
	ApprovalRequestedAction audit.Action = "approval.requested"
	ApprovalDecidedAction   audit.Action = "approval.decided"
	ApprovalsReadAction     audit.Action = "approval.read"
	DelegationSetAction     audit.Action = "approval.delegation_set"
	DelegationEndedAction   audit.Action = "approval.delegation_ended"
	DelegationReadAction    audit.Action = "approval.delegation_read"
)

// Approvals is what the approvals' use cases share; it is also the step's StepApprovals.
type Approvals struct {
	Store      repository.Approvals
	Items      workrepo.Items
	Containers workrepo.Containers
	Groups     work.StepGroups
	Roles      work.FormRoles
	// Steps is the registry: a decision takes the step through TakeStep itself.
	Steps EntryInvoker
	Desk  Desk
	// Timers counts a level's hours down; Notify tells its people. Either may be nil: no reminder
	// is armed, nobody is told.
	Timers ApprovalTimers
	Notify ApprovalNotifier
}

var _ work.StepApprovals = Approvals{}

// Open opens the step's approval with the people each level counts now (UC-SVC-08 check 1).
func (a Approvals) Open(
	ctx context.Context, actor appshared.ActorContext, item workdomain.WorkItem, _ workdomain.Container,
	workflow string, step workdomain.WorkflowStep,
) error {
	if step.Approval == nil {
		return shared.ErrConflict.WithDetail("approvals.step_without_approval")
	}
	counted := make([][]shared.ID, 0, len(step.Approval.Levels))
	for _, level := range step.Approval.Levels {
		people, err := a.counted(ctx, item, level)
		if err != nil {
			return err
		}
		counted = append(counted, people)
	}
	now := a.Desk.Clock.Now()
	approval, err := domain.NewApproval(a.Desk.IDs.NewID(), item, workflow, step, counted, actor.AccountID, now)
	if err != nil {
		return err
	}
	if err := a.Store.Insert(ctx, approval); err != nil {
		return err
	}
	if err := a.levelBegan(ctx, item.TenantID, approval, actor.AccountID); err != nil {
		return err
	}
	return a.audit(ctx, actor, ApprovalRequestedAction, approval, audit.Change{Field: "step", Classification: audit.Open, To: step.Key})
}

// Moved withdraws the approval an entry waits on when another step moved it.
func (a Approvals) Moved(ctx context.Context, item workdomain.WorkItem) error {
	pending, err := a.Store.PendingOf(ctx, item.ID)
	if errors.Is(err, shared.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if err := a.Store.Update(ctx, domain.Withdrawn(pending, a.Desk.Clock.Now()), pending.Version); err != nil {
		return err
	}
	return a.levelEnded(ctx, pending, pending.Current)
}

// counted answers the people a level names, in its groups and by its person fields.
func (a Approvals) counted(ctx context.Context, item workdomain.WorkItem, level workdomain.ApprovalLevel) ([]shared.ID, error) {
	var people []shared.ID
	for _, person := range level.People {
		people = append(people, shared.ID(person))
	}
	for _, group := range level.Groups {
		if a.Groups == nil {
			break
		}
		members, err := a.Groups.Members(ctx, shared.ID(group))
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return nil, err
		}
		people = append(people, members...)
	}
	for _, field := range level.PersonFields {
		people = append(people, named(item, field)...)
	}
	return people, nil
}

// named answers the people a person field of the entry names.
func named(item workdomain.WorkItem, field string) []shared.ID {
	switch field {
	case "assignee":
		return []shared.ID{item.AssigneeID}
	case "created_by":
		return []shared.ID{item.CreatedBy}
	}
	key, _ := strings.CutPrefix(field, workdomain.FormCustomPrefix)
	switch value := item.CustomFields[key].(type) {
	case string:
		return []shared.ID{shared.ID(value)}
	case []any:
		out := []shared.ID{}
		for _, each := range value {
			if id, ok := each.(string); ok {
				out = append(out, shared.ID(id))
			}
		}
		return out
	case []string:
		out := []shared.ID{}
		for _, id := range value {
			out = append(out, shared.ID(id))
		}
		return out
	}
	return nil
}

// eligibility answers how the actor stands at an approval's current level: counted there, holding
// one of its roles along the entry's path, or the delegate of a counted person away now.
func (a Approvals) eligibility(ctx context.Context, actor appshared.ActorContext, approval domain.Approval, at time.Time) (domain.Eligibility, error) {
	return a.standingOf(actor, at).eligibility(ctx, approval)
}

func (a Approvals) standingOf(actor appshared.ActorContext, at time.Time) *standing {
	return &standing{approvals: a, actor: actor, at: at, items: map[shared.ID]workdomain.WorkItem{}, held: map[shared.ID][]string{}}
}

// standing is one reader's eligibility across many approvals: their delegations are read once, an
// entry once, and the roles they hold once per collection.
type standing struct {
	approvals   Approvals
	actor       appshared.ActorContext
	at          time.Time
	delegations []domain.Delegation
	delegated   bool
	items       map[shared.ID]workdomain.WorkItem
	held        map[shared.ID][]string
}

// decidesFor answers the reader and every person whose approvals the reader decides now.
func (s *standing) decidesFor(ctx context.Context) ([]shared.ID, error) {
	delegations, err := s.delegationsTo(ctx)
	if err != nil {
		return nil, err
	}
	people := []shared.ID{s.actor.AccountID}
	for _, delegation := range delegations {
		if delegation.Covers(s.at) && !slices.Contains(people, delegation.AccountID) {
			people = append(people, delegation.AccountID)
		}
	}
	return people, nil
}

func (s *standing) delegationsTo(ctx context.Context) ([]domain.Delegation, error) {
	if !s.delegated {
		delegations, err := s.approvals.Store.DelegationsTo(ctx, s.actor.AccountID)
		if err != nil {
			return nil, err
		}
		s.delegations, s.delegated = delegations, true
	}
	return s.delegations, nil
}

func (s *standing) item(ctx context.Context, id shared.ID) (workdomain.WorkItem, error) {
	if item, ok := s.items[id]; ok {
		return item, nil
	}
	item, err := s.approvals.Items.Find(ctx, id)
	if err != nil {
		return workdomain.WorkItem{}, err
	}
	s.items[id] = item
	return item, nil
}

// prefetchItems reads the entries a page names in one query where the store reads many at once.
func prefetchItems(ctx context.Context, s *standing, ids []shared.ID) error {
	batch, ok := s.approvals.Items.(workrepo.ItemBatch)
	if !ok {
		return nil
	}
	missing := make([]shared.ID, 0, len(ids))
	for _, id := range ids {
		if _, held := s.items[id]; !held && !slices.Contains(missing, id) {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	found, err := batch.FindMany(ctx, missing)
	if err != nil {
		return err
	}
	for _, item := range found {
		s.items[item.ID] = item
	}
	return nil
}

func (s *standing) rolesIn(ctx context.Context, collectionID shared.ID) ([]string, error) {
	if roles, ok := s.held[collectionID]; ok {
		return roles, nil
	}
	collection, err := s.approvals.Containers.Find(ctx, collectionID)
	if err != nil {
		return nil, err
	}
	held, err := s.approvals.Roles.HeldAlong(ctx, s.actor, service.ContainerScopes(collection))
	if err != nil {
		return nil, err
	}
	s.held[collectionID] = held.Roles
	return held.Roles, nil
}

func (s *standing) eligibility(ctx context.Context, approval domain.Approval) (domain.Eligibility, error) {
	if approval.State != domain.ApprovalPending || s.actor.AccountID.IsZero() {
		return domain.Eligibility{}, nil
	}
	if approval.Counts(s.actor.AccountID) {
		return domain.Eligibility{Direct: true}, nil
	}
	level := approval.Levels[approval.Current].Level
	if len(level.Roles) > 0 && s.approvals.Roles != nil {
		item, err := s.item(ctx, approval.ItemID)
		if err != nil {
			return domain.Eligibility{}, err
		}
		held, err := s.rolesIn(ctx, item.CollectionID)
		if err != nil {
			return domain.Eligibility{}, err
		}
		for _, role := range level.Roles {
			if slices.Contains(held, role) {
				return domain.Eligibility{Direct: true}, nil
			}
		}
	}
	delegations, err := s.delegationsTo(ctx)
	if err != nil {
		return domain.Eligibility{}, err
	}
	for _, delegation := range delegations {
		if delegation.Covers(s.at) && approval.Counts(delegation.AccountID) {
			return domain.Eligibility{OnBehalfOf: delegation.AccountID}, nil
		}
	}
	return domain.Eligibility{}, nil
}

func (a Approvals) audit(ctx context.Context, actor appshared.ActorContext, action audit.Action, approval domain.Approval, changes ...audit.Change) error {
	changes = append(changes, audit.Change{Field: "item_id", Classification: audit.Open, To: approval.ItemID.String()},
		audit.Change{Field: "state", Classification: audit.Open, To: string(approval.State)})
	return a.Desk.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: a.Desk.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: approvalTarget, TargetID: approval.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// ApprovalOutput is an approval as every door answers it (schema Approval).
func ApprovalOutput(approval domain.Approval, decisions []domain.Decision, mayDecide bool) usecase.Output {
	levels := make([]usecase.Output, 0, len(approval.Levels))
	for _, level := range approval.Levels {
		counted := make([]string, 0, len(level.Counted))
		for _, id := range level.Counted {
			counted = append(counted, id.String())
		}
		roles := level.Level.Roles
		if roles == nil {
			roles = []string{}
		}
		levels = append(levels, usecase.Output{
			"rule": string(level.Level.Rule), "count": level.Level.Count, "approvers": counted, "roles": roles,
			"needed": level.Level.Needed(len(level.Counted)),
		})
	}
	trail := make([]usecase.Output, 0, len(decisions))
	for _, d := range decisions {
		trail = append(trail, usecase.Output{
			"level": d.Level, "approver_id": d.ApproverID.String(), "on_behalf_of": idOrNil(d.OnBehalfOf),
			"verdict": string(d.Verdict), "reason": d.Reason, "item_version": d.ItemVersion, "decided_at": d.DecidedAt.UTC(),
		})
	}
	out := usecase.Output{
		"id": approval.ID.String(), "item_id": approval.ItemID.String(), "workflow": approval.Workflow, "step": approval.Step,
		"item_version": approval.ItemVersion, "state": string(approval.State), "current_level": approval.Current,
		"levels": levels, "decisions": trail, "requested_by": approval.RequestedBy.String(),
		"requested_at": approval.RequestedAt.UTC(), "decided_at": nil, "due_at": nil, "may_decide": mayDecide,
		"version": approval.Version,
	}
	if approval.DecidedAt != nil {
		out["decided_at"] = approval.DecidedAt.UTC()
	}
	if due := approval.DueAt(); !due.IsZero() {
		out["due_at"] = due.UTC()
	}
	return out
}

func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

// DecideApproval answers an approval at its current level (UC-SVC-09): an approver counted there,
// holding its role, or deciding for a counted person away. The last yes takes the step; a decline
// moves the entry to the step's decline status where it names one.
type DecideApproval struct{ Approvals Approvals }

// DecideApprovalCommand is the input.
type DecideApprovalCommand struct {
	ApprovalID shared.ID
	Verdict    domain.Verdict
	Reason     string
}

// Execute decides and answers the approval as it now stands.
func (h DecideApproval) Execute(ctx context.Context, actor appshared.ActorContext, cmd DecideApprovalCommand) (usecase.Output, error) {
	a := h.Approvals
	var (
		decided   domain.Approval
		decisions []domain.Decision
	)
	err := a.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		approval, err := a.Store.Find(ctx, cmd.ApprovalID)
		if err != nil {
			return err
		}
		now := a.Desk.Clock.Now()
		who, err := a.eligibility(ctx, actor, approval, now)
		if err != nil {
			return err
		}
		if !who.Direct && who.OnBehalfOf.IsZero() {
			// Somebody who may not decide is told nothing about the approval (P-05).
			return shared.ErrNotFound.WithDetail("approvals.not_found")
		}
		item, err := a.Items.Find(ctx, approval.ItemID)
		if err != nil {
			return err
		}
		earlier, err := a.Store.Decisions(ctx, approval.ID)
		if err != nil {
			return err
		}
		next, decision, err := domain.Decide(approval, earlier, a.Desk.IDs.NewID(), actor.AccountID, who, cmd.Verdict, cmd.Reason, item.Version, now)
		if err != nil {
			return err
		}
		if err := a.Store.Record(ctx, decision); err != nil {
			return err
		}
		if err := a.Store.Update(ctx, next, approval.Version); err != nil {
			return err
		}
		if err := a.moved(ctx, actor.TenantID, approval, next, actor.AccountID); err != nil {
			return err
		}
		decided, decisions = next, append(earlier, decision)
		return a.audit(ctx, actor, ApprovalDecidedAction, next,
			audit.Change{Field: "verdict", Classification: audit.Open, To: string(decision.Verdict)},
			audit.Change{Field: "level", Classification: audit.Open, To: decision.Level})
	})
	if err != nil {
		return nil, err
	}
	if err := h.move(ctx, actor, decided); err != nil {
		return nil, err
	}
	return ApprovalOutput(decided, decisions, false), nil
}

// move takes the step once approved, or the way to the decline status once declined, as an
// automation with the rights of whoever asked for the approval: they were allowed to take the step.
func (h DecideApproval) move(ctx context.Context, actor appshared.ActorContext, approval domain.Approval) error {
	in := usecase.Input{"item_id": approval.ItemID.String()}
	switch {
	case approval.State == domain.ApprovalApproved:
		in["step"] = approval.Step
	case approval.State == domain.ApprovalDeclined && approval.DeclineTo != "":
		in["to_status"] = approval.DeclineTo
	default:
		return nil
	}
	asker := appshared.ActorContext{
		Kind: appshared.ActorAutomation, TenantID: actor.TenantID, TenantSlug: actor.TenantSlug,
		AccountID: approval.RequestedBy, Scopes: []string{itemsWriteScope},
	}
	_, err := h.Approvals.Steps.Invoke(work.Approved(ctx), takeStepUseCase, asker, in)
	return err
}

// Descriptor is the catalogue entry.
func (h DecideApproval) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: DecideApprovalName,
		Summary: "Approves or declines an approval at its current level, as an approver counted there, holding its " +
			"role, or deciding for a counted person who is away. A decline needs a reason. The last yes takes the " +
			"step; a decline moves the entry to the step's decline status where it names one.",
		SideEffects: "Records the decision with the entry's version, moves the approval on, takes the step or the way " +
			"to the decline status through TakeStep, and writes an audit entry.",
		TokenScope: itemsWriteScope,
		Input: []usecase.Field{
			{Name: "approval_id", Kind: usecase.KindID, Required: true, Description: "The approval."},
			{Name: "verdict", Kind: usecase.KindString, Required: true, Description: "APPROVE or DECLINE."},
			{Name: "reason", Kind: usecase.KindString, Description: "Why; required with DECLINE."},
		},
		Audit:    usecase.AuditDeclaration{Action: ApprovalDecidedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "the decision is the approval's own trail; the step it takes writes the entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("approval_id")
			if err != nil {
				return nil, err
			}
			return h.Execute(ctx, actor, DecideApprovalCommand{ApprovalID: id, Verdict: domain.Verdict(in.String("verdict")), Reason: in.String("reason")})
		}),
	}, approvalTarget)
	d.Permission = ""
	return d
}

// ListMyApprovals answers the approvals waiting for the reader, directly or for somebody away
// whose approvals they decide (UC-SVC-09 check 1, UC-SVC-10 check 2).
type ListMyApprovals struct{ Approvals Approvals }

// Execute lists them, oldest first.
func (h ListMyApprovals) Execute(ctx context.Context, actor appshared.ActorContext) ([]usecase.Output, error) {
	a := h.Approvals
	out := []usecase.Output{}
	err := a.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		reader := a.standingOf(actor, a.Desk.Clock.Now())
		people, err := reader.decidesFor(ctx)
		if err != nil {
			return err
		}
		// The selection is the database's: only approvals counting one of these people, or naming
		// a role, come back - never the workspace's oldest ones with the reader's past them.
		query := repository.PendingQuery{People: people, Limit: pendingPage}
		for range maxPendingPages {
			pending, err := a.Store.PendingFor(ctx, query)
			if err != nil {
				return err
			}
			rows, err := h.rows(ctx, reader, pending)
			if err != nil {
				return err
			}
			out = append(out, rows...)
			if len(pending) < pendingPage {
				return nil
			}
			last := pending[len(pending)-1]
			query.AfterRequestedAt, query.AfterID = last.RequestedAt, last.ID
		}
		slog.WarnContext(ctx, "a reader's approval list stopped at its bound", slog.Int("pages", maxPendingPages))
		return nil
	})
	if err != nil {
		return nil, err
	}
	// The sink writes through the transaction in the context; a read-only one cannot take it.
	return out, a.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return a.Desk.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: a.Desk.Clock.Now(), Action: ApprovalsReadAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo, ActorKind: actor.Kind, ActorID: actor.AccountID,
			ActorLabel: actor.AccountName, TargetType: approvalTarget, TargetID: actor.AccountID,
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	})
}

// rows answers the page's approvals the reader decides, with their decisions read in one query.
func (h ListMyApprovals) rows(ctx context.Context, reader *standing, pending []domain.Approval) ([]usecase.Output, error) {
	type mine struct {
		approval domain.Approval
		who      domain.Eligibility
	}
	var theirs []mine
	ids := make([]shared.ID, 0, len(pending))
	for _, approval := range pending {
		who, err := reader.eligibility(ctx, approval)
		if err != nil {
			return nil, err
		}
		if who.Direct || !who.OnBehalfOf.IsZero() {
			theirs = append(theirs, mine{approval, who})
			ids = append(ids, approval.ID)
		}
	}
	if len(theirs) == 0 {
		return nil, nil
	}
	decisions, err := h.Approvals.Store.DecisionsOf(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]shared.ID, 0, len(theirs))
	for _, each := range theirs {
		items = append(items, each.approval.ItemID)
	}
	if err := prefetchItems(ctx, reader, items); err != nil {
		return nil, err
	}
	out := make([]usecase.Output, 0, len(theirs))
	for _, each := range theirs {
		row := ApprovalOutput(each.approval, decisions[each.approval.ID], true)
		if item, err := reader.item(ctx, each.approval.ItemID); err == nil {
			// The approver reads what they decide on: the entry's title and number, never more.
			row["item_title"], row["item_number"] = item.Title, item.Number
		}
		row["on_behalf_of"] = idOrNil(each.who.OnBehalfOf)
		out = append(out, row)
	}
	return out, nil
}

// Descriptor is the catalogue entry.
func (h ListMyApprovals) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: ListMyApprovalsName,
		Summary: "The approvals waiting for the reader, oldest first: those whose current level counts them or names a " +
			"role they hold there, and those of a person away whose approvals they decide - each with the entry's title.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: ApprovalsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			rows, err := h.Execute(ctx, actor)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": rows}, nil
		}),
	}, approvalTarget)
	d.Permission = ""
	return d
}

// ReadEntryApprovals answers an entry's approvals with their trail for its readers (UC-SVC-09
// check 3).
type ReadEntryApprovals struct{ Approvals Approvals }

// Execute reads them, newest first.
func (h ReadEntryApprovals) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) ([]usecase.Output, error) {
	a := h.Approvals
	var collection workdomain.Container
	err := a.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		item, err := a.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		collection, err = a.Containers.Find(ctx, item.CollectionID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) {
		return nil, appshared.ItemNotFound(itemID)
	}
	if err != nil {
		return nil, err
	}
	if err := a.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead, Path: service.ContainerScopes(collection),
		Action: ApprovalsReadAction, TokenScope: deskRead, TargetType: approvalTarget, TargetID: itemID,
	}); err != nil {
		return nil, err
	}
	out := []usecase.Output{}
	err = a.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		approvals, err := a.Store.OfItem(ctx, itemID)
		if err != nil {
			return err
		}
		now := a.Desk.Clock.Now()
		for _, approval := range approvals {
			decisions, err := a.Store.Decisions(ctx, approval.ID)
			if err != nil {
				return err
			}
			who, err := a.eligibility(ctx, actor, approval, now)
			if err != nil {
				return err
			}
			out = append(out, ApprovalOutput(approval, decisions, who.Direct || !who.OnBehalfOf.IsZero()))
		}
		return nil
	})
	return out, err
}

// Descriptor is the catalogue entry.
func (h ReadEntryApprovals) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadEntryApprovalsName, Permission: service.PermissionRead,
		Summary: "An entry's approvals, newest first, each with its levels, the people counted, where it stands and " +
			"its trail: who decided, for whom, when, why and on which version of the entry.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."}},
		Audit: usecase.AuditDeclaration{Action: ApprovalsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			rows, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return usecase.Output{"data": rows}, nil
		}),
	}, approvalTarget)
}
