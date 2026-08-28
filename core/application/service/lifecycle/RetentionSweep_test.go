// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
)

// The engine of data-retention.md §5 (E-07): two phases, the safeguards in their order, and the
// chain whose second stage counts from what the first one did.

// exportSpy is the archive a rule writes before it removes anything.
type exportSpy struct {
	targets []shared.ID
	err     error
}

func (s *exportSpy) Export(_ context.Context, targetID shared.ID) (shared.ID, error) {
	if s.err != nil {
		return "", s.err
	}
	s.targets = append(s.targets, targetID)
	return targetID, nil
}

type sweepHarness struct {
	*purgeHarness
	rules   *ruleStore
	marking *markedItems
	changes *changeLog
	export  *exportSpy
}

func newSweepHarness() *sweepHarness {
	base := newPurgeHarness()
	return &sweepHarness{
		purgeHarness: base,
		rules:        &ruleStore{},
		marking: &markedItems{
			markingStore: &markingStore{},
			pending:      map[shared.ID]repository.Candidate{},
		},
		changes: &changeLog{},
		export:  &exportSpy{},
	}
}

func (h *sweepHarness) sweeper() Sweeper {
	return Sweeper{
		Rules: h.rules, Marking: h.marking, Holds: h.holds, Items: h.items,
		Purger: h.purger, Changes: h.changes, Export: h.export,
		Clock: clock.Fixed(now), IDs: &idSource{}, HLC: &hlcSource{}, Batch: 100,
	}
}

// ruleIn writes one rule into the store, built through the domain so that the test cannot express a
// rule the model would refuse.
func (h *sweepHarness) ruleIn(t *testing.T, change func(*domain.NewRuleInput)) domain.Rule {
	t.Helper()
	in := domain.NewRuleInput{
		ID: shared.MustParseID("0192f000-0000-7000-8000-0000000000e1"), TenantID: tenantID,
		Scope: domain.Scope{Kind: domain.ScopeTenant}, DataKind: domain.KindCompletedItem,
		RetainDays: 365, Action: domain.ActionArchive, CreatedBy: accountID, Now: now,
	}
	change(&in)
	rule, err := domain.NewRule(in)
	if err != nil {
		t.Fatalf("building the rule: %v", err)
	}
	if err := h.rules.Insert(context.Background(), rule); err != nil {
		t.Fatalf("storing the rule: %v", err)
	}
	return rule
}

// candidate is one entry past its period.
func candidate(id shared.ID, anchoredAt time.Time) repository.Candidate {
	return repository.Candidate{
		ID: id, Type: work.ItemTask, Path: work.RootPath(id),
		CollectionID: collectionID, HubID: hubID, AnchoredAt: anchoredAt, Title: "Weekly shop",
	}
}

func TestPhaseOneAnnouncesWhatIsDueAndActsOnNothing(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.marked) != 1 || h.marking.marked[0] != taskID {
		t.Fatalf("the entry was not announced: %+v", h.marking.marked)
	}
	if outcome.Removed != 0 {
		t.Errorf("phase one removed %d entries", outcome.Removed)
	}
	if len(h.marking.archived) != 0 {
		t.Error("phase one acted")
	}
	// The device is told what is coming, because §6's point is that the object says so - and
	// because a device that hears the announcement has the grace period to react.
	if len(h.changes.recorded) != 1 {
		t.Fatalf("%d change log entries", len(h.changes.recorded))
	}
	announced, ok := h.changes.recorded[0].Payload["retention"].(map[string]any)
	if !ok || announced["policy_id"] != rule.ID.String() {
		t.Fatalf("the change log says %+v", h.changes.recorded[0].Payload)
	}
}

// An entry caught by the loosest cutoff and not by its own rule's is not blocked - its period
// simply has not run out.
func TestAnEntryOnlyItsOwnRulesCutoffCatchesIsLeftAlone(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.RetainDays = 30 })
	h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.ID = shared.MustParseID("0192f000-0000-7000-8000-0000000000e2")
		in.Scope = domain.Scope{Kind: domain.ScopeCollection, ID: collectionID}
		in.RetainDays = 365
	})
	// Ninety days old: past the tenant's thirty and inside the collection's three hundred and
	// sixty-five, and the collection's rule is the one that applies.
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -90))}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.marked) != 0 {
		t.Fatalf("an entry inside its own rule's period was announced")
	}
	if len(outcome.Blocked) != 0 {
		t.Errorf("it was reported as blocked: %+v", outcome.Blocked)
	}
}

// §4.1 outranks everything, and the object is counted rather than acted on.
func TestALegalHoldKeepsAnEntryBackAndIsReported(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}
	h.holds.holds = domain.Holds{{
		ID: holdID, Scope: domain.HoldTenant, Reason: "Pending litigation", PlacedAt: now,
	}}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.marked) != 0 {
		t.Error("an entry under a legal hold was announced")
	}
	if outcome.Blocked[domain.BlockedByLegalHold] != 1 {
		t.Fatalf("the pass reported %+v", outcome.Blocked)
	}
	// And the object is told: a count in a run's report is what an operator reads, and "this would
	// have gone and a hold is holding it back" is what the person looking at the entry needs.
	if len(h.marking.blocked) != 1 || h.marking.blocked[0] != taskID {
		t.Fatalf("the entry was not told what is stopping it: %+v", h.marking.blocked)
	}
}

// Phase two acts on what the grace period has run out on, and the marking goes with the act while
// the rule's claim stays - the chain's next stage is what owns the entry then.
func TestPhaseTwoActsAndKeepsTheRulesClaim(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionArchive,
	}}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.archived) != 1 || h.marking.archived[0] != taskID {
		t.Fatalf("the entry was not archived: %+v", h.marking.archived)
	}
	if !h.marking.keptRule {
		t.Error("the rule lost its claim, so a chain's second stage would never find the entry")
	}
	if outcome.Removed != 1 {
		t.Errorf("the pass reports %d acted on", outcome.Removed)
	}
}

// An announcement acts on nothing, and the marking stands so the object keeps saying so.
func TestAnAnnouncementNeverActs(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) { in.Action = domain.ActionNotifyOnly })
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionNotifyOnly,
	}}

	if _, err := h.sweeper().Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.archived) != 0 || len(h.marking.trashed) != 0 {
		t.Error("an announcement acted")
	}
	if len(h.marking.cleared) != 0 {
		t.Error("an announcement cleared its own marking, so the object stopped saying what is coming")
	}
}

// §4.6: a parent whose children are staying is kept back and goes on the pass after the last of
// them.
func TestAParentIsKeptBackWhileSomethingBelowItIsRetained(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionArchive,
	}}
	h.marking.descendant = map[shared.ID]int{taskID: 2}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.archived) != 0 {
		t.Error("a parent went while something below it stayed")
	}
	if outcome.Blocked[domain.BlockedByDescendant] != 1 {
		t.Fatalf("the pass reported %+v", outcome.Blocked)
	}
}

// §6: the export comes before the deletion, once per target per pass.
func TestAnExportThenDeleteWritesTheArchiveFirst(t *testing.T) {
	h := newSweepHarness()
	target := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.Action, in.ExportTargetID = domain.ActionExportThenDelete, target
	})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionExportThenDelete,
	}}
	// The subtree the removal reads: the entry itself, which is what a purge writes a journal
	// entry and a tombstone for.
	h.trash.subtree = []shared.ID{taskID}

	if _, err := h.sweeper().Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.export.targets) != 1 || h.export.targets[0] != target {
		t.Fatalf("the archive was written to %+v", h.export.targets)
	}
	// And the entry went, through the one engine every removal goes through: a journal entry and a
	// tombstone for every row.
	if len(h.removals.recorded) == 0 {
		t.Error("the entry was exported and not removed")
	}
}

// "Export then delete" with the export missing is just a deletion, so the act is refused rather
// than quietly downgraded.
func TestAnExportThatCannotBeWrittenStopsTheDeletion(t *testing.T) {
	h := newSweepHarness()
	target := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.Action, in.ExportTargetID = domain.ActionExportThenDelete, target
	})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionExportThenDelete,
	}}
	h.export.err = errors.New("the target did not answer")

	_, err := h.sweeper().Pass(context.Background(), actor())

	if err == nil {
		t.Fatal("the pass carried on after the export failed")
	}
	if len(h.removals.recorded) != 0 {
		t.Error("the entry was removed although its export failed")
	}
}

// A rule that exports where nothing can write one is refused at the act, with a code that says so.
func TestAnExportWithNoWriterAtAllIsRefused(t *testing.T) {
	h := newSweepHarness()
	target := shared.MustParseID("0192f000-0000-7000-8000-0000000000f2")
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.Action, in.ExportTargetID = domain.ActionExportThenDelete, target
	})
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionExportThenDelete,
	}}

	sweeper := h.sweeper()
	sweeper.Export = nil

	_, err := sweeper.Pass(context.Background(), actor())

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeExportUnavailable {
		t.Fatalf("refused with %v", err)
	}
}

// A chain's second stage asks a different question of a different column: what did *this rule*
// archive long enough ago. The restriction to its own work is what keeps a chain from sweeping up an
// entry somebody archived by hand.
func TestAChainsSecondStageAnnouncesWhatItsFirstOneArchived(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.ThenAfterDays, in.ThenAction = 730, domain.ActionHardDelete
	})
	h.marking.inChain = []repository.Candidate{
		candidate(taskID, now.AddDate(0, 0, -800)),
	}

	if _, err := h.sweeper().Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.marked) != 1 || h.marking.marked[0] != taskID {
		t.Fatalf("the second stage announced %+v", h.marking.marked)
	}
	// What it announced is the second stage's action, not the first one's - an object that said
	// "will be archived" while it was about to be deleted would be the announcement lying.
	announced, ok := h.changes.recorded[0].Payload["retention"].(map[string]any)
	if !ok || announced["action"] != string(domain.ActionHardDelete) {
		t.Fatalf("the change log says %+v", h.changes.recorded[0].Payload)
	}
	if announced["policy_id"] != rule.ID.String() {
		t.Errorf("the announcement names %v", announced["policy_id"])
	}
}

// A rule with no second stage asks the question at all, which is what keeps a one-stage rule from
// re-announcing what it has already acted on.
func TestARuleWithNoChainNeverAsksTheSecondQuestion(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.inChain = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -800))}

	if _, err := h.sweeper().Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(h.marking.marked) != 0 {
		t.Fatalf("a rule with no chain announced a second stage: %+v", h.marking.marked)
	}
}

// Phase two trashes as well as archives, and one act is one batch identifier - which is what makes
// one act one restore (F-09).
func TestATrashStageMovesTheEntryIntoTheTrash(t *testing.T) {
	h := newSweepHarness()
	rule := h.ruleIn(t, func(in *domain.NewRuleInput) { in.Action = domain.ActionTrash })
	h.marking.dueMarked = []repository.Candidate{{
		ID: taskID, Type: work.ItemTask, Path: work.RootPath(taskID),
		CollectionID: collectionID, HubID: hubID,
		Pending: now.Add(-time.Hour), Rule: rule.ID, Action: domain.ActionTrash,
	}}

	if _, err := h.sweeper().Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(h.marking.trashed) != 1 || h.marking.trashed[0] != taskID {
		t.Fatalf("the entry was not trashed: %+v", h.marking.trashed)
	}
}

// A kind with no rule at all is not read: a pass over a workspace that has configured nothing costs
// one listing and no query per kind.
func TestAKindWithNoRuleIsNotRead(t *testing.T) {
	h := newSweepHarness()
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	outcome, err := h.sweeper().Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}
	if len(h.marking.marked) != 0 {
		t.Fatal("an entry was announced with no rule to announce it")
	}
	if outcome.Matched != 0 {
		t.Errorf("the pass matched %d with nothing configured", outcome.Matched)
	}
}

// The second consumer of the expression port, and the reason it is a port (G-06): the retention
// sweep reads the same language the automation rules do, through the same interface, with the same
// limits. E-07 refused a condition outright because nothing could evaluate one; these are what
// replaced that refusal.

// conditions is the expression port as this package sees it. A fake rather than the CEL adapter,
// because core/application may not import one (ADR-0001) - what these tests are about is that the
// sweep asks, honours the answer, and reads the entry only when the expression names it.
type conditions struct {
	// answer decides per candidate, by identifier, so a test can make one entry match and another
	// not without writing an expression.
	answer   map[shared.ID]bool
	compiles bool
	// touched records which names each evaluation resolved, which is how the laziness is proved.
	touched  []string
	compiled int
}

func newConditions() *conditions {
	return &conditions{answer: map[shared.ID]bool{}, compiles: true}
}

func (c *conditions) Compile(
	text string, _ expression.Environment, _ expression.Result,
) (expression.Program, error) {
	c.compiled++
	if !c.compiles {
		return nil, expression.Refusal{Code: expression.CodeSyntax}.Error()
	}
	// The fake honours one expression shape: naming `item` means the entry is read, and naming
	// only `now` means it is not.
	return &program{owner: c, readsItem: strings.Contains(text, "item")}, nil
}

type program struct {
	owner     *conditions
	readsItem bool
}

func (p *program) Evaluate(ctx context.Context, in expression.Activation) (expression.Value, error) {
	if _, _, err := in.Resolve(ctx, "now"); err != nil {
		return expression.Value{}, err
	}
	p.owner.touched = append(p.owner.touched, "now")

	if !p.readsItem {
		return expression.Value{Bool: true}, nil
	}

	value, found, err := in.Resolve(ctx, "item")
	if err != nil {
		return expression.Value{}, err
	}
	p.owner.touched = append(p.owner.touched, "item")
	if !found {
		return expression.Value{}, nil
	}

	document, _ := value.(map[string]any)
	id, _ := document["id"].(string)
	return expression.Value{Bool: p.owner.answer[shared.ID(id)]}, nil
}

// A rule with a condition sweeps only what matches, and the entry it does not match is not blocked
// either - nothing is keeping it, its rule was never about it.
func TestAConditionedRuleSweepsOnlyWhatMatches(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) {
		in.Condition = "item.completed_at != null"
	})

	other := shared.MustParseID("0192f000-0000-7000-8000-0000000000f9")
	h.items.stored[taskID] = work.WorkItem{ID: taskID, Type: work.ItemTask}
	h.items.stored[other] = work.WorkItem{ID: other, Type: work.ItemTask}
	h.marking.due = []repository.Candidate{
		candidate(taskID, now.AddDate(0, 0, -400)),
		candidate(other, now.AddDate(0, 0, -400)),
	}

	engine := newConditions()
	engine.answer[taskID] = true
	engine.answer[other] = false

	sweeper := h.sweeper()
	sweeper.Conditions = engine
	outcome, err := sweeper.Pass(context.Background(), actor())
	if err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if len(h.marking.marked) != 1 || h.marking.marked[0] != taskID {
		t.Fatalf("announced %+v, want only the entry the condition matches", h.marking.marked)
	}
	if outcome.Matched != 1 {
		t.Errorf("matched %d, want 1", outcome.Matched)
	}
	// Not a block: the entry the condition excluded is not being held back by anything.
	if len(outcome.Blocked) != 0 {
		t.Errorf("the excluded entry was reported as blocked: %v", outcome.Blocked)
	}
}

// A rule with no condition costs nothing: the expression is never compiled, and the entry is never
// read for a question nobody asked.
func TestARuleWithoutAConditionNeverReachesTheEngine(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(*domain.NewRuleInput) {})
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	engine := newConditions()
	sweeper := h.sweeper()
	sweeper.Conditions = engine
	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if engine.compiled != 0 {
		t.Errorf("the engine compiled %d expressions for a rule with none", engine.compiled)
	}
	if len(h.marking.marked) != 1 {
		t.Errorf("the unconditioned rule announced %d entries, want one", len(h.marking.marked))
	}
}

// One compilation per rule per pass rather than one per candidate: a pass judges a thousand entries
// against a handful of rules, and compiling is the expensive half.
func TestAConditionIsCompiledOncePerPass(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })

	var due []repository.Candidate
	engine := newConditions()
	for i := range 5 {
		id := shared.MustParseID("0192f000-0000-7000-8000-00000000010" + string(rune('0'+i)))
		h.items.stored[id] = work.WorkItem{ID: id, Type: work.ItemTask}
		due = append(due, candidate(id, now.AddDate(0, 0, -400)))
		engine.answer[id] = true
	}
	h.marking.due = due

	sweeper := h.sweeper()
	sweeper.Conditions = engine
	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	if engine.compiled != 1 {
		t.Errorf("the engine compiled %d times for one rule over five candidates", engine.compiled)
	}
}

// The whole point of the lazy activation: `item` is a query, and a pass over a thousand candidates
// must not make a thousand of them for a condition that only asks about the clock.
func TestTheEntryIsReadOnlyWhenTheConditionNamesIt(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "now.getHours() < 23" })
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	engine := newConditions()
	sweeper := h.sweeper()
	sweeper.Conditions = engine
	if _, err := sweeper.Pass(context.Background(), actor()); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	for _, name := range engine.touched {
		if name == "item" {
			t.Error("the entry was read for a condition that only names the clock")
		}
	}
}

// Neither direction is an answer. Defaulting to true would delete what the condition was written to
// protect; defaulting to false would quietly retain everything and look like a working rule.
func TestAConditionThatCannotBeCompiledStopsThePass(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	engine := newConditions()
	engine.compiles = false

	sweeper := h.sweeper()
	sweeper.Conditions = engine
	if _, err := sweeper.Pass(context.Background(), actor()); err == nil {
		t.Fatal("a rule whose condition cannot be compiled was acted on")
	}
	if len(h.marking.marked) != 0 {
		t.Errorf("%d entries were announced anyway", len(h.marking.marked))
	}
}

// Fail closed: a build with no evaluator refuses to act on a conditioned rule rather than acting on
// all of it, which is the safe direction for a pass whose job is deleting.
func TestAConditionedRuleIsRefusedWhenNoEngineIsWired(t *testing.T) {
	h := newSweepHarness()
	h.ruleIn(t, func(in *domain.NewRuleInput) { in.Condition = "item.completed_at != null" })
	h.marking.due = []repository.Candidate{candidate(taskID, now.AddDate(0, 0, -400))}

	if _, err := h.sweeper().Pass(context.Background(), actor()); !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error %v, want ErrInternal", err)
	}
	if len(h.marking.marked) != 0 {
		t.Errorf("%d entries were announced anyway", len(h.marking.marked))
	}
}
