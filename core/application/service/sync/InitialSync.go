// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package sync

import (
	"context"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/sync"
	agentservice "github.com/Jersyfi/hubtask/core/application/service/agent"
	cmdbservice "github.com/Jersyfi/hubtask/core/application/service/cmdb"
	forgeservice "github.com/Jersyfi/hubtask/core/application/service/forge"
	goalservice "github.com/Jersyfi/hubtask/core/application/service/goal"
	identityservice "github.com/Jersyfi/hubtask/core/application/service/identity"
	knowledgeservice "github.com/Jersyfi/hubtask/core/application/service/knowledge"
	planningservice "github.com/Jersyfi/hubtask/core/application/service/planning"
	servicedeskservice "github.com/Jersyfi/hubtask/core/application/service/servicedesk"
	timeservice "github.com/Jersyfi/hubtask/core/application/service/timekeeping"
	whiteboardservice "github.com/Jersyfi/hubtask/core/application/service/whiteboard"
	workservice "github.com/Jersyfi/hubtask/core/application/service/work"
	workflowservice "github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
)

// The initial synchronisation (offline-sync.md §3.1): a device with no cursor is told
// everything it may read, as UPSERT records carrying whole objects, one kind at a time in an
// order a client can apply without a forward reference. The log's position is read before the
// first page and becomes the cursor the last page hands back, so that a change landing mid-walk
// is the first delta's business rather than a gap.

// walkKinds is the order of the walk. Each name is the entity the change log records under, so
// that a record from the walk and a record from the delta about the same object say the same
// thing in the same place. `set` is the exception: set elements travel as records of the entry
// they belong to, exactly as a delta's set change does.
var walkKinds = []string{
	entityContainer, entityBucket, entityLabel, entityItem, kindSet,
	entityComment, entityReminder, entityRecurrence, entityTemplate, entityEntryType,
	entityLinkType, entityWorkflow, entityCustomRole, entityEntryLink, entityRoleGrant, entityPage, entityWhiteboard,
	entityCI, entityCIRelation, entityCIAffected, entityCIGroup, entityCIAsset, entityCICatalogue, entityEntrySchedule,
	entityGoalPeriod, entityGoal, entityGoalCheckIn, entityDashboard,
	entitySprint, entitySprintEntry, entityEstimate, entityRelease, entityReleaseEntry, entityCustomField,
	entityTimeRecord, entityTimer, entityWorkTypes, entityTimePolicy,
	entityTimesheet, entityCapacities, entityAbsence, entityCostRates, entityBudget,
	entityPortfolio,
	entityAgentModel, entityAgent, entityAgentRequest, entityAgentCase, entityAgentRun,
	entityApproval, entityEntrySla, entitySatisfaction, entityCodeLink,
}

const (
	entityContainer  = "container"
	entityBucket     = "bucket"
	entityLabel      = "label"
	entityItem       = "item"
	entityComment    = "comment"
	entityReminder   = "reminder"
	entityRecurrence = "recurrence_rule"
	entityTemplate   = "template"
	// entityEntryType is workspace-wide: its records name no container (UC-FLW-01).
	entityEntryType = "entry_type"
	// entityLinkType, entityWorkflow and entityCustomRole are workspace-wide too: the vocabulary a
	// device draws links, boards and a form's `actor.roles` with.
	entityLinkType   = "link_type"
	entityWorkflow   = "workflow"
	entityCustomRole = "custom_role"
	// entityEntryLink is filed under its source's collection and reaches a reader only when they
	// may read the target's as well: a link never tells of an entry its reader cannot see.
	entityEntryLink = "entry_link"
	// entityEntrySchedule is an entry's scheduling mode and milestone mark, filed under its
	// collection (M2): walked last, so a cursor from a build that ended before it still resumes.
	entityEntrySchedule = planningservice.ScheduleEntity
	// entityPage is a page with its body, filed under its hub: a page is read where its hub is.
	entityPage = knowledge.DocumentKind
	// entityWhiteboard is a board with its scene, filed under its hub: a board is read where its
	// hub is (M4).
	entityWhiteboard = whiteboard.DocumentKind
	// entityCI and entityCIRelation are a configuration item and a relation between two, each
	// filed under its hub: an inventory is read where its hub is (M6).
	entityCI         = cmdbservice.CIEntity
	entityCIRelation = cmdbservice.RelationEntity
	// entityCIAffected, entityCIGroup and entityCIAsset are an entry's affected CI, filed under the
	// CI's hub, and a hub's CI groups and asset records (UC-AST-09, 11, 12).
	entityCIAffected = cmdbservice.AffectedEntity
	entityCIGroup    = cmdbservice.GroupEntity
	entityCIAsset    = cmdbservice.AssetEntity
	// entityCICatalogue is the workspace's classes and relation types, one record under the
	// workspace's identifier: what a device names and groups an inventory by.
	entityCICatalogue = cmdbservice.CatalogueEntity
	// entityRoleGrant is a grant of a custom role, walked for its holder alone: what a device
	// may predict offline is what its own person holds (offline-sync.md §4.3).
	entityRoleGrant = identityservice.RoleGrantEntity
	// entityGoalPeriod is workspace-wide; a goal, a check-in and a dashboard are filed under their
	// hub - none for the workspace's -, a private dashboard reaching its owner alone (M8). Walked
	// last, for the schedule's reason.
	entityGoalPeriod  = goalservice.PeriodEntity
	entityGoal        = goalservice.GoalEntity
	entityGoalCheckIn = goalservice.CheckInEntity
	entityDashboard   = goalservice.DashboardEntity
	// entityPortfolio is workspace-wide, its projects narrowed to the reader's (UC-GOL-03).
	entityPortfolio = goalservice.PortfolioEntity
	// The registry's rows (M9), under no container: an agent, a held act and a run for the agent's
	// owner and whoever administers agents, a model and a case for the administrators alone.
	entityAgentModel   = agentservice.ModelEntity
	entityAgent        = agentservice.AgentEntity
	entityAgentRequest = agentservice.RequestEntity
	entityAgentCase    = agentservice.CaseEntity
	entityAgentRun     = agentservice.RunEntity
	// entitySprint is a sprint, entitySprintEntry an entry's open sprint and entityEstimate an
	// entry's estimate, each filed under the collection (M2); walked last, for the schedule's
	// reason.
	entitySprint      = planningservice.SprintEntity
	entitySprintEntry = planningservice.SprintEntryEntity
	entityEstimate    = planningservice.EstimateEntity
	// entityRelease is a release and entityReleaseEntry an entry's release, filed under the hub
	// (M2).
	entityRelease      = planningservice.ReleaseEntity
	entityReleaseEntry = planningservice.ReleaseEntryEntity
	// entityCustomField is a custom field definition, filed under its collection - none for a
	// workspace-wide one, which reaches every member as the types do: a definition draws a form,
	// a read-restricted field narrows its values, never its definition.
	entityCustomField = workservice.CustomFieldEntity
	// entityTimeRecord and entityTimer are a person's own, walked for that person alone and
	// delivered to them alone; entityWorkTypes and entityTimePolicy one record each under the
	// workspace's identifier, for every member (M7).
	entityTimeRecord = timeservice.RecordEntity
	entityTimer      = timeservice.TimerEntity
	entityWorkTypes  = timeservice.WorkTypesEntity
	entityTimePolicy = timeservice.PolicyEntity
	// entityTimesheet and entityCapacities are a person's own as a record is; entityAbsence the
	// shared calendar every member reads; entityCostRates one record under the workspace and
	// entityBudget filed under its plan, both for MANAGE_COSTS alone (UC-TIM-08).
	entityTimesheet  = timeservice.SheetEntity
	entityCapacities = timeservice.CapacitiesEntity
	entityAbsence    = timeservice.AbsenceEntity
	entityCostRates  = timeservice.RatesEntity
	entityBudget     = timeservice.BudgetEntity
	// entityApproval, entityEntrySla and entitySatisfaction are an entry's approvals with their
	// trail, its SLA goals and its score, each filed under the entry's collection (M5); walked
	// last, for the schedule's reason.
	entityApproval     = servicedeskservice.ApprovalEntity
	entityEntrySla     = servicedeskservice.EntrySlaEntity
	entitySatisfaction = servicedeskservice.SatisfactionEntity
	// entityCodeLink is an entry's commit, branch or pull request, filed under the entry's
	// collection (UC-INT-13).
	entityCodeLink = forgeservice.LinkEntity
	// kindSet is the walk's own name for the set element pass. Not an entity: its records are the
	// entry's.
	kindSet = "set"
)

// setKeySeparator joins the three parts of a set element's key inside the cursor. A bar, because
// neither an identifier nor a set name contains one, and the cursor's own separator is a full stop.
const setKeySeparator = "|"

// walkRow is one object as the walk hands it out: the record's facts, and the key the next page
// resumes after.
type walkRow struct {
	entity      string
	entityID    shared.ID
	containerID shared.ID
	hlc         shared.HLC
	payload     map[string]any
	key         string
}

// walk serves one page of the initial synchronisation from the position, filling it across kinds
// until it is full or the walk is over.
func (p PullChanges) walk(
	ctx context.Context, actor appshared.ActorContext, from Position, limit int,
	keep func(work.Container) bool,
) (Batch, error) {
	kind := indexOfKind(from.Kind)
	if kind < 0 {
		// A kind this build does not walk: a cursor from a build that walked more, or a forged
		// one that got past the signature. Either way the walk cannot be resumed.
		return Batch{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}

	resolved := map[shared.ID]visibility{}
	costed := map[shared.ID]bool{}
	var lens *workservice.FieldLens
	records := make([]Record, 0, limit)
	position := from
	for kind < len(walkKinds) {
		position.Kind = walkKinds[kind]
		batch := limit - len(records)
		rows, err := p.readKind(ctx, actor, position.Kind, position.After, batch)
		if err != nil {
			return Batch{}, err
		}
		for _, row := range rows {
			position.After = row.key
			seen, err := p.Stream.mayRead(ctx, actor, row.entity, row.containerID, resolved)
			if err != nil {
				return Batch{}, err
			}
			if !seen.allowed || (keep != nil && !keep(seen.container)) {
				continue
			}
			if other, err := p.Stream.mayReadOtherEnd(ctx, actor, row.entity, row.payload, resolved); err != nil || !other {
				if err != nil {
					return Batch{}, err
				}
				continue
			}
			if beyond, err := p.Stream.mayReadBeyond(ctx, actor, row.entity, seen, row.payload, costed); err != nil || !beyond {
				if err != nil {
					return Batch{}, err
				}
				continue
			}
			if row.payload, err = p.Stream.narrowed(ctx, actor, &lens, row.entity, seen, row.payload, resolved); err != nil {
				return Batch{}, err
			}
			records = append(records, Record{
				Recorded: repository.Recorded{
					Change: repository.Change{
						TenantID: actor.TenantID, Entity: row.entity, EntityID: row.entityID,
						Op: repository.Upsert, ContainerID: row.containerID,
						HLC: row.hlc, Payload: row.payload,
					},
					Seq: from.Seq, OccurredAt: from.IssuedAt,
				},
				Cursor: position,
			})
		}
		if len(records) >= limit {
			return Batch{Records: records, Cursor: position, More: true}, nil
		}
		if len(rows) == batch {
			// A full batch, and the page is not full: every row in it was withheld or out of
			// scope. The kind is not exhausted - the walk stays on it, past those rows. Moving on
			// here would skip whatever this kind still holds behind a run of somebody else's.
			continue
		}
		// The kind is exhausted - fewer rows than asked for - and the walk moves on to the next
		// one from its start.
		kind++
		position.After = ""
	}

	// The walk is over. The cursor is the delta position taken before the first page: everything
	// that happened since is in the log past it, and the first delta delivers it.
	return Batch{
		Records: records,
		Cursor:  Position{Seq: from.Seq, IssuedAt: from.IssuedAt, Epoch: from.Epoch},
		More:    false,
	}, nil
}

// readKind pages one kind after the key, inside one read-only transaction.
func (p PullChanges) readKind(
	ctx context.Context, actor appshared.ActorContext, kind, after string, batch int,
) ([]walkRow, error) {
	var rows []walkRow
	err := p.Stream.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if kind == entityRoleGrant {
			rows, err = p.ownGrants(ctx, actor, after, batch)
			return err
		}
		if kind == entityCICatalogue {
			rows, err = p.catalogue(ctx, actor, after)
			return err
		}
		if timeKind(kind) {
			rows, err = p.time(ctx, actor, kind, after, batch)
			return err
		}
		rows, err = p.pageOf(ctx, kind, after, batch)
		return err
	})
	return rows, err
}

// ownGrants pages the reader's own grants of custom roles; the only kind read for one person.
func (p PullChanges) ownGrants(
	ctx context.Context, actor appshared.ActorContext, after string, batch int,
) ([]walkRow, error) {
	grants, err := p.Snapshot.OwnRoleGrants(ctx, actor.AccountID, shared.ID(after), batch)
	if err != nil {
		return nil, err
	}
	rows := make([]walkRow, 0, len(grants))
	for _, grant := range grants {
		rows = append(rows, walkRow{
			entity: entityRoleGrant, entityID: grant.Grant.ID,
			payload: identityservice.CustomGrantOutput(grant), key: grant.Grant.ID.String(),
		})
	}
	return rows, nil
}

// catalogue is the CMDB's one workspace record; a resumed walk has delivered it already.
func (p PullChanges) catalogue(ctx context.Context, actor appshared.ActorContext, after string) ([]walkRow, error) {
	if after != "" {
		return nil, nil
	}
	classes, types, err := p.Snapshot.CmdbCatalogue(ctx)
	if err != nil {
		return nil, err
	}
	return []walkRow{{
		entity: entityCICatalogue, entityID: actor.TenantID,
		payload: cmdbservice.CatalogueOutput(classes, types), key: actor.TenantID.String(),
	}}, nil
}

// timeKind reports whether the time module's walk reads the kind.
func timeKind(kind string) bool {
	switch kind {
	case entityTimeRecord, entityTimer, entityWorkTypes, entityTimePolicy,
		entityTimesheet, entityCapacities, entityAbsence, entityCostRates, entityBudget:
		return true
	}
	return false
}

// time pages the time module's kinds: a person's own records and timer, and the workspace's two
// records, each delivered once and only once there is something to tell.
func (p PullChanges) time(ctx context.Context, actor appshared.ActorContext, kind, after string, batch int) ([]walkRow, error) {
	switch kind {
	case entityTimesheet, entityCapacities, entityAbsence, entityCostRates, entityBudget:
		return p.timeResources(ctx, actor, kind, after, batch)
	case entityTimeRecord:
		records, err := p.Snapshot.OwnTimeRecords(ctx, actor.AccountID, shared.ID(after), batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(records))
		for _, r := range records {
			rows = append(rows, walkRow{entity: entityTimeRecord, entityID: r.ID, payload: timeservice.RecordOutput(r), key: r.ID.String()})
		}
		return rows, nil
	case entityTimer:
		if after != "" {
			return nil, nil
		}
		timer, running, err := p.Snapshot.OwnTimer(ctx, actor.AccountID)
		if err != nil || !running {
			return nil, err
		}
		return []walkRow{{entity: entityTimer, entityID: actor.AccountID, payload: timeservice.TimerRecord(timer), key: actor.AccountID.String()}}, nil
	case entityWorkTypes:
		if after != "" {
			return nil, nil
		}
		kinds, err := p.Snapshot.TimeWorkTypes(ctx)
		if err != nil || len(kinds) == 0 {
			return nil, err
		}
		return []walkRow{{entity: entityWorkTypes, entityID: actor.TenantID, payload: timeservice.WorkTypesRecord(kinds), key: actor.TenantID.String()}}, nil
	default:
		if after != "" {
			return nil, nil
		}
		policy, err := p.Snapshot.TimePolicy(ctx)
		if err != nil || policy.Version == 0 {
			// Never set: a device without the record holds the default policy.
			return nil, err
		}
		return []walkRow{{entity: entityTimePolicy, entityID: actor.TenantID, payload: timeservice.PolicyOutput(policy), key: actor.TenantID.String()}}, nil
	}
}

// timeResources pages a person's weeks and capacities, the absences, the rates and the budgets;
// the rates and the budgets are judged by MANAGE_COSTS where every row is judged.
func (p PullChanges) timeResources(ctx context.Context, actor appshared.ActorContext, kind, after string, batch int) ([]walkRow, error) {
	switch kind {
	case entityTimesheet:
		sheets, err := p.Snapshot.OwnTimesheets(ctx, actor.AccountID, shared.ID(after), batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(sheets))
		for _, s := range sheets {
			rows = append(rows, walkRow{entity: entityTimesheet, entityID: s.ID, payload: timeservice.SheetOutput(s), key: s.ID.String()})
		}
		return rows, nil
	case entityCapacities:
		if after != "" {
			return nil, nil
		}
		list, err := p.Snapshot.OwnCapacities(ctx, actor.AccountID, timeservice.CapacitiesKept)
		if err != nil || len(list) == 0 {
			return nil, err
		}
		return []walkRow{{entity: entityCapacities, entityID: actor.AccountID, payload: timeservice.CapacitiesRecord(actor.AccountID, list), key: actor.AccountID.String()}}, nil
	case entityAbsence:
		list, err := p.Snapshot.TimeAbsences(ctx, shared.ID(after), batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(list))
		for _, a := range list {
			rows = append(rows, walkRow{entity: entityAbsence, entityID: a.ID, payload: timeservice.AbsenceOutput(a), key: a.ID.String()})
		}
		return rows, nil
	case entityCostRates:
		if after != "" {
			return nil, nil
		}
		list, err := p.Snapshot.TimeCostRates(ctx, timeservice.RatesKept)
		if err != nil || len(list) == 0 {
			return nil, err
		}
		return []walkRow{{entity: entityCostRates, entityID: actor.TenantID, payload: timeservice.RatesRecord(list), key: actor.TenantID.String()}}, nil
	default:
		list, err := p.Snapshot.TimeBudgets(ctx, shared.ID(after), batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(list))
		for _, b := range list {
			rows = append(rows, walkRow{entity: entityBudget, entityID: b.PlanID, containerID: b.PlanID, payload: timeservice.BudgetRecord(b), key: b.PlanID.String()})
		}
		return rows, nil
	}
}

// maxModelsKnown bounds the models an agent page reads its model's revision from.
const maxModelsKnown = 1000

// registry pages the agents' kinds; who may hold each is decided where every row is judged.
func (p PullChanges) registry(ctx context.Context, kind string, after shared.ID, batch int) ([]walkRow, error) {
	switch kind {
	case entityAgentModel:
		models, err := p.Snapshot.AgentModels(ctx, after, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(models))
		for _, m := range models {
			rows = append(rows, walkRow{entity: kind, entityID: m.ID, payload: agentservice.ModelOutput(m), key: m.ID.String()})
		}
		return rows, nil
	case entityAgent:
		agents, err := p.Snapshot.Agents(ctx, after, batch)
		if err != nil || len(agents) == 0 {
			return nil, err
		}
		models, err := p.Snapshot.AgentModels(ctx, "", maxModelsKnown)
		if err != nil {
			return nil, err
		}
		revisions := make(map[shared.ID]int, len(models))
		for _, m := range models {
			revisions[m.ID] = m.Revision
		}
		rows := make([]walkRow, 0, len(agents))
		for _, a := range agents {
			rows = append(rows, walkRow{entity: kind, entityID: a.ID, payload: agentservice.AgentOutput(a, revisions[a.ModelID]), key: a.ID.String()})
		}
		return rows, nil
	case entityAgentRequest:
		held, err := p.Snapshot.AgentRequests(ctx, after, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(held))
		for _, h := range held {
			rows = append(rows, walkRow{entity: kind, entityID: h.Request.ID, payload: agentservice.RequestRecord(h.Request, h.OwnerID), key: h.Request.ID.String()})
		}
		return rows, nil
	case entityAgentCase:
		cases, err := p.Snapshot.AgentCases(ctx, after, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(cases))
		for _, c := range cases {
			rows = append(rows, walkRow{entity: kind, entityID: c.ID, payload: agentservice.CaseOutput(c), key: c.ID.String()})
		}
		return rows, nil
	default:
		runs, err := p.Snapshot.AgentRuns(ctx, after, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(runs))
		for _, r := range runs {
			rows = append(rows, walkRow{entity: kind, entityID: r.Run.ID, payload: agentservice.RunRecord(r.Run, r.OwnerID), key: r.Run.ID.String()})
		}
		return rows, nil
	}
}

// pageOf is the table of kinds: what each one reads, and how its rows become records.
func (p PullChanges) pageOf(ctx context.Context, kind, after string, batch int) ([]walkRow, error) {
	afterID := shared.ID(after)
	switch kind {
	case entityContainer:
		containers, err := p.Snapshot.Containers(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		payloads, err := workservice.ContainerSnapshots(ctx, p.Structure, containers)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(containers))
		for i, container := range containers {
			rows = append(rows, walkRow{
				entity: entityContainer, entityID: container.ID, containerID: container.ID,
				payload: payloads[i], key: container.ID.String(),
			})
		}
		return rows, nil
	case entityBucket:
		buckets, err := p.Snapshot.Buckets(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(buckets))
		for _, bucket := range buckets {
			rows = append(rows, walkRow{
				entity: entityBucket, entityID: bucket.ID, containerID: bucket.CollectionID,
				payload: workservice.BucketSnapshot(bucket), key: bucket.ID.String(),
			})
		}
		return rows, nil
	case entityLabel:
		labels, err := p.Snapshot.Labels(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(labels))
		for _, label := range labels {
			rows = append(rows, walkRow{
				entity: entityLabel, entityID: label.ID, containerID: label.CollectionID,
				payload: workservice.LabelSnapshot(label), key: label.ID.String(),
			})
		}
		return rows, nil
	case entityItem:
		items, err := p.Snapshot.Items(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(items))
		for _, item := range items {
			rows = append(rows, walkRow{
				entity: entityItem, entityID: item.ID, containerID: item.CollectionID,
				payload: workservice.ItemSnapshot(item), key: item.ID.String(),
			})
		}
		return rows, nil
	case kindSet:
		key, err := parseSetKey(after)
		if err != nil {
			return nil, err
		}
		elements, err := p.Snapshot.SetElements(ctx, key, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(elements))
		for _, element := range elements {
			rows = append(rows, setRow(element))
		}
		return rows, nil
	case entityComment:
		comments, err := p.Snapshot.Comments(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(comments))
		for _, comment := range comments {
			rows = append(rows, walkRow{
				entity: entityComment, entityID: comment.Value.ID, containerID: comment.CollectionID,
				payload: workservice.CommentSnapshot(comment.Value), key: comment.Value.ID.String(),
			})
		}
		return rows, nil
	case entityReminder:
		reminders, err := p.Snapshot.Reminders(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(reminders))
		for _, reminder := range reminders {
			rows = append(rows, walkRow{
				entity: entityReminder, entityID: reminder.Value.ID, containerID: reminder.CollectionID,
				payload: workservice.ReminderSnapshot(reminder.Value), key: reminder.Value.ID.String(),
			})
		}
		return rows, nil
	case entityRecurrence:
		rules, err := p.Snapshot.Recurrences(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(rules))
		for _, rule := range rules {
			rows = append(rows, walkRow{
				entity: entityRecurrence, entityID: rule.Value.ID, containerID: rule.CollectionID,
				payload: workservice.RecurrenceSnapshot(rule.Value), key: rule.Value.ID.String(),
			})
		}
		return rows, nil
	case entityTemplate:
		templates, err := p.Snapshot.Templates(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(templates))
		for _, template := range templates {
			rows = append(rows, walkRow{
				entity: entityTemplate, entityID: template.ID, containerID: template.ScopeID,
				payload: workservice.TemplateSnapshot(template), key: template.ID.String(),
			})
		}
		return rows, nil
	case entityEntryType:
		types, err := p.Snapshot.EntryTypes(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(types))
		for _, entryType := range types {
			rows = append(rows, walkRow{
				entity: entityEntryType, entityID: entryType.ID,
				payload: workflowservice.EntryTypeOutput(entryType), key: entryType.ID.String(),
			})
		}
		return rows, nil
	case entityLinkType:
		types, err := p.Snapshot.LinkTypes(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(types))
		for _, linkType := range types {
			rows = append(rows, walkRow{
				entity: entityLinkType, entityID: linkType.ID,
				payload: workservice.LinkTypeOutput(linkType), key: linkType.ID.String(),
			})
		}
		return rows, nil
	case entityWorkflow:
		workflows, err := p.Snapshot.Workflows(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(workflows))
		for _, workflow := range workflows {
			rows = append(rows, walkRow{
				entity: entityWorkflow, entityID: workflow.ID,
				payload: workflowservice.WorkflowOutput(workflow), key: workflow.ID.String(),
			})
		}
		return rows, nil
	case entityCustomRole:
		roles, err := p.Snapshot.CustomRoles(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(roles))
		for _, role := range roles {
			rows = append(rows, walkRow{
				entity: entityCustomRole, entityID: role.Role.ID,
				payload: identityservice.CustomRoleOutput(role), key: role.Role.ID.String(),
			})
		}
		return rows, nil
	case entityPage:
		pages, err := p.Snapshot.Pages(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		now := p.Stream.Clock.Now()
		rows := make([]walkRow, 0, len(pages))
		for _, page := range pages {
			rows = append(rows, walkRow{
				entity: entityPage, entityID: page.ID, containerID: page.HubID,
				payload: knowledgeservice.PageOutput(page, true, now), key: page.ID.String(),
			})
		}
		return rows, nil
	case entityWhiteboard:
		boards, err := p.Snapshot.Whiteboards(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(boards))
		for _, board := range boards {
			rows = append(rows, walkRow{
				entity: entityWhiteboard, entityID: board.ID, containerID: board.HubID,
				payload: whiteboardservice.BoardOutput(board, len(board.Scene.Elements), true), key: board.ID.String(),
			})
		}
		return rows, nil
	case entityCI:
		cis, err := p.Snapshot.CIs(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(cis))
		for _, ci := range cis {
			rows = append(rows, walkRow{
				entity: entityCI, entityID: ci.ID, containerID: ci.HubID,
				payload: cmdbservice.CIOutput(ci, nil), key: ci.ID.String(),
			})
		}
		return rows, nil
	case entityCIAffected:
		affected, err := p.Snapshot.CIAffected(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(affected))
		for _, a := range affected {
			rows = append(rows, walkRow{
				entity: entityCIAffected, entityID: a.ID, containerID: a.HubID,
				payload: cmdbservice.AffectedOutput(a.Affected), key: a.ID.String(),
			})
		}
		return rows, nil
	case entityCIGroup:
		groups, err := p.Snapshot.CIGroups(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(groups))
		for _, g := range groups {
			rows = append(rows, walkRow{
				entity: entityCIGroup, entityID: g.ID, containerID: g.HubID,
				payload: cmdbservice.GroupOutput(g, -1), key: g.ID.String(),
			})
		}
		return rows, nil
	case entityCIAsset:
		assets, err := p.Snapshot.CIAssets(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(assets))
		for _, a := range assets {
			rows = append(rows, walkRow{
				entity: entityCIAsset, entityID: a.CIID, containerID: a.HubID,
				payload: cmdbservice.AssetOutput(a, a.UpdatedAt), key: a.CIID.String(),
			})
		}
		return rows, nil
	case entityCIRelation:
		relations, err := p.Snapshot.CIRelations(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(relations))
		for _, r := range relations {
			rows = append(rows, walkRow{
				entity: entityCIRelation, entityID: r.ID, containerID: r.HubID,
				payload: cmdbservice.RelationOutput(r), key: r.ID.String(),
			})
		}
		return rows, nil
	case entityGoalPeriod:
		periods, err := p.Snapshot.GoalPeriods(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(periods))
		for _, period := range periods {
			rows = append(rows, walkRow{
				entity: entityGoalPeriod, entityID: period.ID, payload: goalservice.PeriodOutput(period), key: period.ID.String(),
			})
		}
		return rows, nil
	case entityGoal:
		goals, err := p.Snapshot.Goals(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(goals))
		for _, g := range goals {
			rows = append(rows, walkRow{
				entity: entityGoal, entityID: g.ID, containerID: g.HubID, payload: goalservice.GoalRecord(g), key: g.ID.String(),
			})
		}
		return rows, nil
	case entityGoalCheckIn:
		checkIns, err := p.Snapshot.GoalCheckIns(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(checkIns))
		for _, c := range checkIns {
			rows = append(rows, walkRow{
				entity: entityGoalCheckIn, entityID: c.CheckIn.ID, containerID: c.HubID,
				payload: goalservice.CheckInRecord(c.CheckIn), key: c.CheckIn.ID.String(),
			})
		}
		return rows, nil
	case entityAgentModel, entityAgent, entityAgentRequest, entityAgentCase, entityAgentRun:
		return p.registry(ctx, kind, afterID, batch)
	case entityPortfolio:
		portfolios, err := p.Snapshot.Portfolios(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(portfolios))
		for _, portfolio := range portfolios {
			rows = append(rows, walkRow{
				entity: entityPortfolio, entityID: portfolio.ID, payload: goalservice.PortfolioRecord(portfolio), key: portfolio.ID.String(),
			})
		}
		return rows, nil
	case entityDashboard:
		dashboards, err := p.Snapshot.Dashboards(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(dashboards))
		for _, d := range dashboards {
			rows = append(rows, walkRow{
				entity: entityDashboard, entityID: d.ID, containerID: d.HubID, payload: goalservice.DashboardOutput(d), key: d.ID.String(),
			})
		}
		return rows, nil
	case entityEntrySchedule:
		schedules, err := p.Snapshot.EntrySchedules(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(schedules))
		for _, s := range schedules {
			rows = append(rows, walkRow{
				entity: entityEntrySchedule, entityID: s.EntryID, containerID: s.CollectionID,
				payload: planningservice.ScheduleRecord(s.EntryID, s.CollectionID, planning.Mode(s.Mode), s.Milestone, s.Version),
				key:     s.EntryID.String(),
			})
		}
		return rows, nil
	case entitySprint:
		sprints, err := p.Snapshot.Sprints(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(sprints))
		for _, s := range sprints {
			rows = append(rows, walkRow{
				entity: entitySprint, entityID: s.ID, containerID: s.CollectionID,
				payload: planningservice.SprintOutput(s), key: s.ID.String(),
			})
		}
		return rows, nil
	case entityRelease:
		releases, err := p.Snapshot.Releases(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(releases))
		for _, r := range releases {
			rows = append(rows, walkRow{
				entity: entityRelease, entityID: r.ID, containerID: r.HubID,
				payload: planningservice.ReleaseOutput(r), key: r.ID.String(),
			})
		}
		return rows, nil
	case entityReleaseEntry:
		members, err := p.Snapshot.ReleaseEntries(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(members))
		for _, m := range members {
			rows = append(rows, walkRow{
				entity: entityReleaseEntry, entityID: m.EntryID, containerID: m.HubID,
				payload: planningservice.ReleaseEntryRecord(m.EntryID, m.ReleaseID, m.HubID), key: m.EntryID.String(),
			})
		}
		return rows, nil
	case entitySprintEntry:
		members, err := p.Snapshot.SprintEntries(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(members))
		for _, m := range members {
			rows = append(rows, walkRow{
				entity: entitySprintEntry, entityID: m.EntryID, containerID: m.CollectionID,
				payload: planningservice.MembershipRecord(m.EntryID, m.SprintID, m.CollectionID, m.Committed),
				key:     m.EntryID.String(),
			})
		}
		return rows, nil
	case entityEstimate:
		estimates, err := p.Snapshot.EntryEstimates(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(estimates))
		for _, e := range estimates {
			rows = append(rows, walkRow{
				entity: entityEstimate, entityID: e.Estimate.EntryID, containerID: e.CollectionID,
				payload: planningservice.EstimateRecord(e.Estimate, e.CollectionID), key: e.Estimate.EntryID.String(),
			})
		}
		return rows, nil
	case entityCustomField:
		definitions, err := p.Snapshot.CustomFields(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(definitions))
		for _, d := range definitions {
			rows = append(rows, walkRow{
				entity: entityCustomField, entityID: d.ID, containerID: d.CollectionID,
				payload: workservice.CustomFieldRecord(d), key: d.ID.String(),
			})
		}
		return rows, nil
	case entityApproval:
		approvals, err := p.Snapshot.Approvals(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(approvals))
		for _, a := range approvals {
			rows = append(rows, walkRow{
				entity: entityApproval, entityID: a.Approval.ID, containerID: a.CollectionID,
				payload: servicedeskservice.ApprovalRecord(a.Approval, a.Decisions), key: a.Approval.ID.String(),
			})
		}
		return rows, nil
	case entityEntrySla:
		records, err := p.Snapshot.EntrySlas(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(records))
		for _, r := range records {
			rows = append(rows, walkRow{
				entity: entityEntrySla, entityID: r.Record.ID, containerID: r.CollectionID,
				payload: servicedeskservice.SlaRecord(r.Record), key: r.Record.ID.String(),
			})
		}
		return rows, nil
	case entitySatisfaction:
		ratings, err := p.Snapshot.SatisfactionRatings(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(ratings))
		for _, r := range ratings {
			rows = append(rows, walkRow{
				entity: entitySatisfaction, entityID: r.Rating.ID, containerID: r.CollectionID,
				payload: servicedeskservice.RatingRecord(r.Rating), key: r.Rating.ID.String(),
			})
		}
		return rows, nil
	case entityCodeLink:
		links, err := p.Snapshot.CodeLinks(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(links))
		for _, l := range links {
			rows = append(rows, walkRow{
				entity: entityCodeLink, entityID: l.Link.ID, containerID: l.CollectionID,
				payload: forgeservice.LinkOutput(l.Link, l.CollectionID), key: l.Link.ID.String(),
			})
		}
		return rows, nil
	case entityEntryLink:
		links, err := p.Snapshot.EntryLinks(ctx, afterID, batch)
		if err != nil {
			return nil, err
		}
		rows := make([]walkRow, 0, len(links))
		for _, between := range links {
			rows = append(rows, walkRow{
				entity: entityEntryLink, entityID: between.Link.ID, containerID: between.SourceCollectionID,
				payload: workservice.LinkRecord(between.Link, between.SourceCollectionID, between.TargetCollectionID),
				key:     between.Link.ID.String(),
			})
		}
		return rows, nil
	}
	return nil, shared.ErrValidation.WithDetail("sync.cursor_invalid")
}

// setRow is one tag row as a record of its entry: the same shape a delta's set change carries
// (AddLabel.recordChange), with the tag that decides the element as the record's clock. A present
// element is an addition under its add tag; an absent one a removal under its remove tag, so that
// a device merging a later re-add has the removal to compare it against.
func setRow(element repository.ItemSetElement) walkRow {
	operation, tag := "add", element.Element.AddedAt
	if !element.Element.IsPresent() {
		operation, tag = "remove", element.Element.RemovedAt
	}
	return walkRow{
		entity: entityItem, entityID: element.ItemID, containerID: element.CollectionID, hlc: tag,
		payload: map[string]any{
			"set":        string(element.Set),
			"element_id": element.Element.ElementID.String(),
			"op":         operation,
		},
		key: strings.Join([]string{
			element.ItemID.String(), string(element.Set), element.Element.ElementID.String(),
		}, setKeySeparator),
	}
}

// parseSetKey reads a set element's key back out of the cursor. Empty is the start.
func parseSetKey(after string) (repository.SetElementKey, error) {
	if after == "" {
		return repository.SetElementKey{}, nil
	}
	parts := strings.Split(after, setKeySeparator)
	if len(parts) != 3 {
		return repository.SetElementKey{}, shared.ErrValidation.WithDetail("sync.cursor_invalid")
	}
	return repository.SetElementKey{
		ItemID: shared.ID(parts[0]), Set: work.SetName(parts[1]), ElementID: shared.ID(parts[2]),
	}, nil
}

func indexOfKind(kind string) int {
	for i, known := range walkKinds {
		if known == kind {
			return i
		}
	}
	return -1
}

// SnapshotRequest is what `:pull` takes for an initial synchronisation without a cursor or a
// page size (SY-C): the device, what it says about itself, and what it wants to hold.
type SnapshotRequest struct {
	DeviceID    shared.ID
	Platform    string
	DisplayName string
	Scopes      []Scope
}

// snapshotBatch is how many rows one read of the walk takes while a snapshot streams. The
// contract's largest page: the reads are the page sequence's, and the sequence has already
// decided how much one transaction should carry.
const snapshotBatch = PullLimitMax

// WalkAll serves the initial synchronisation as one walk rather than a page sequence (SY-C):
// every record the page sequence would answer, in the same order, handed to `emit` as it
// is read, and the delta cursor the sequence would end on answered last.
//
// The same reads, the same permission per record, the same scope filter and the same cursor - the
// snapshot is the sequence with the pages joined, which is what makes a test able to hold the two
// against each other. The device registers by turning up exactly as it does on a pull. An `emit`
// that fails ends the walk with its error and the cursor is not answered: what was written before
// the failure is a device's problem to start over from, never something it may resume.
func (p PullChanges) WalkAll(
	ctx context.Context, actor appshared.ActorContext, request SnapshotRequest,
	emit func(Record) error,
) (Position, error) {
	if request.DeviceID.IsZero() {
		return Position{}, shared.ErrValidation.
			WithDetail("sync.device_required").
			WithFields(shared.FieldError{Path: "/device_id", Code: "sync.device_required"})
	}
	keep, err := scopeFilter(request.Scopes)
	if err != nil {
		return Position{}, err
	}
	from, err := p.resume(ctx, actor, "")
	if err != nil {
		return Position{}, err
	}
	pull := PullRequest{
		DeviceID: request.DeviceID, Platform: request.Platform, DisplayName: request.DisplayName,
		Scopes: request.Scopes,
	}
	if err := p.touch(ctx, actor, pull, from); err != nil {
		return Position{}, err
	}

	position := from
	for {
		if err := ctx.Err(); err != nil {
			return Position{}, err
		}
		batch, err := p.walk(ctx, actor, position, snapshotBatch, keep)
		if err != nil {
			return Position{}, err
		}
		for _, record := range batch.Records {
			if err := emit(record); err != nil {
				return Position{}, err
			}
		}
		if !batch.More {
			return batch.Cursor, nil
		}
		position = batch.Cursor
	}
}
