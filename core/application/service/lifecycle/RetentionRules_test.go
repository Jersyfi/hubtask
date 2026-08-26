// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package lifecycle

import (
	"context"
	"errors"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/lifecycle"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/lifecycle"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domainservice "github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The three rule use cases (E-07): what a rule costs to write down, what a preview says, and the
// switch that makes a broad first run an announcement rather than a mass deletion.

// ruleStore is the rules, in memory, with the unique index the schema has.
type ruleStore struct {
	stored     []domain.Rule
	carried    []domain.DataKind
	insertFail error
}

func (s *ruleStore) Insert(_ context.Context, rule domain.Rule) error {
	if s.insertFail != nil {
		return s.insertFail
	}
	for _, existing := range s.stored {
		if existing.DataKind == rule.DataKind && existing.Scope == rule.Scope {
			return shared.ErrConflict.WithDetail(domain.CodeRuleAlreadyExists)
		}
	}
	s.stored = append(s.stored, rule)
	return nil
}

func (s *ruleStore) List(context.Context) ([]domain.Rule, error) { return s.stored, nil }

func (s *ruleStore) Find(_ context.Context, id shared.ID) (domain.Rule, error) {
	for _, rule := range s.stored {
		if rule.ID == id {
			return rule, nil
		}
	}
	return domain.Rule{}, shared.ErrNotFound.WithDetail(domain.CodeRuleNotFound)
}

func (s *ruleStore) CarryOver(_ context.Context, _ shared.ID, kind domain.DataKind, _ time.Time) error {
	s.carried = append(s.carried, kind)
	return nil
}

var _ repository.Rules = (*ruleStore)(nil)

// markingStore is the objects a pass judges, as numbers a test sets.
type markingStore struct {
	due        []repository.Candidate
	dueMarked  []repository.Candidate
	countDue   int
	scopeCount int
	marked     []shared.ID
	cleared    []shared.ID
	archived   []shared.ID
	trashed    []shared.ID
	keptRule   bool
	descendant map[shared.ID]int
}

func (s *markingStore) Due(
	_ context.Context, _ domain.Anchor, _ time.Time, batch int,
) ([]repository.Candidate, error) {
	if batch < len(s.due) {
		return s.due[:batch], nil
	}
	return s.due, nil
}

func (s *markingStore) DueInChain(
	_ context.Context, _ domain.Anchor, _ shared.ID, _ time.Time, _ int,
) ([]repository.Candidate, error) {
	return nil, nil
}

func (s *markingStore) Mark(
	_ context.Context, ids []shared.ID, _ shared.ID, _ domain.Action, _ time.Time,
) (int, error) {
	s.marked = append(s.marked, ids...)
	return len(ids), nil
}

func (s *markingStore) MarkedDue(_ context.Context, _ time.Time, _ int) ([]repository.Candidate, error) {
	return s.dueMarked, nil
}

func (s *markingStore) Marking(_ context.Context, id shared.ID) (repository.Candidate, error) {
	return repository.Candidate{ID: id}, nil
}

func (s *markingStore) Clear(_ context.Context, ids []shared.ID, keepRule bool, _ time.Time) (int, error) {
	s.cleared = append(s.cleared, ids...)
	s.keptRule = keepRule
	return len(ids), nil
}

func (s *markingStore) Archive(_ context.Context, ids []shared.ID, _ time.Time) (int, error) {
	s.archived = append(s.archived, ids...)
	return len(ids), nil
}

func (s *markingStore) Trash(_ context.Context, ids []shared.ID, _ shared.ID, _ time.Time) (int, error) {
	s.trashed = append(s.trashed, ids...)
	return len(ids), nil
}

func (s *markingStore) RetainedDescendants(
	_ context.Context, _, _ []shared.ID,
) (map[shared.ID]int, error) {
	return s.descendant, nil
}

func (s *markingStore) CountDue(_ context.Context, _ domain.Anchor, _ domain.Scope, _ time.Time) (int, error) {
	return s.countDue, nil
}

func (s *markingStore) CountScope(_ context.Context, _ domain.Scope) (int, error) {
	return s.scopeCount, nil
}

var _ repository.Marking = (*markingStore)(nil)

type rulesHarness struct {
	rules      *ruleStore
	marking    *markingStore
	holds      *holdStore
	authorizer *authorizerDouble
	audit      *auditSink
	uow        *unitOfWork
}

func newRulesHarness() *rulesHarness {
	return &rulesHarness{
		rules: &ruleStore{}, marking: &markingStore{scopeCount: 1000},
		holds: &holdStore{}, authorizer: &authorizerDouble{},
		audit: &auditSink{}, uow: &unitOfWork{},
	}
}

func (h *rulesHarness) service() Rules {
	return Rules{
		Rules: h.rules, Marking: h.marking, Holds: h.holds,
		Authorizer: h.authorizer, Audit: h.audit, UnitOfWork: h.uow,
		Clock: clock.Fixed(now), IDs: &idSource{},
	}
}

func createCommand(change func(*CreateRetentionPolicyCommand)) CreateRetentionPolicyCommand {
	cmd := CreateRetentionPolicyCommand{
		Scope:    domain.Scope{Kind: domain.ScopeTenant},
		DataKind: domain.KindCompletedItem, RetainDays: 365, Action: domain.ActionArchive,
	}
	change(&cmd)
	return cmd
}

func TestCreatingARuleWritesItAndAsksForTheOwnersRight(t *testing.T) {
	h := newRulesHarness()

	rule, preview, err := (CreateRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(), createCommand(func(*CreateRetentionPolicyCommand) {}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	if len(h.rules.stored) != 1 || h.rules.stored[0].ID != rule.ID {
		t.Fatalf("the rule was not written: %+v", h.rules.stored)
	}
	if preview.Broad {
		t.Error("a rule matching nothing was called broad")
	}
	// A retention rule is a standing instruction to destroy work, which is the one thing an
	// administrator cannot do.
	request := h.authorizer.requests[0]
	if request.Permission != domainservice.PermissionDeleteContainer {
		t.Errorf("creating a rule asked for %q", request.Permission)
	}
	if request.TokenScope != retentionManage {
		t.Errorf("the token scope is %q", request.TokenScope)
	}
	// One entry, and the numbers in it are numbers rather than anybody's content.
	if len(h.audit.entries) != 1 {
		t.Fatalf("%d audit entries", len(h.audit.entries))
	}
}

// §5's switch: a rule whose first run would take more than a twentieth of the scope is stored as an
// announcement, and the caller is told which mode it is in.
func TestABroadFirstRunIsStoredAsAnAnnouncement(t *testing.T) {
	h := newRulesHarness()
	h.marking.scopeCount, h.marking.countDue = 1000, 51 // 5.1 per cent

	rule, preview, err := (CreateRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(),
			createCommand(func(cmd *CreateRetentionPolicyCommand) {
				cmd.Action = domain.ActionHardDelete
			}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	if !preview.Broad {
		t.Fatalf("a rule taking %.1f per cent was not called broad", preview.ShareOfScope*100)
	}
	if rule.Action != domain.ActionNotifyOnly {
		t.Fatalf("the stored rule is a %s rather than an announcement", rule.Action)
	}
	if h.rules.stored[0].Action != domain.ActionNotifyOnly {
		t.Fatal("the rule that was written is not the one that came back")
	}
	// RE-7's second half: the share the creation reports is the share a preview would report.
	if preview.ShareOfScope <= BroadFirstRunShare {
		t.Errorf("the share is %v", preview.ShareOfScope)
	}
}

// Exactly the threshold is not over it. A rule at five per cent is what somebody meant.
func TestAtTheThresholdARuleIsNotBroad(t *testing.T) {
	h := newRulesHarness()
	h.marking.scopeCount, h.marking.countDue = 1000, 50

	rule, preview, err := (CreateRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(),
			createCommand(func(cmd *CreateRetentionPolicyCommand) {
				cmd.Action = domain.ActionHardDelete
			}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if preview.Broad || rule.Action != domain.ActionHardDelete {
		t.Fatalf("a rule at exactly five per cent was turned into %s", rule.Action)
	}
}

// A rule the model refuses never reaches the store, and the refusal is the domain's.
func TestARuleTheModelRefusesIsNotWritten(t *testing.T) {
	h := newRulesHarness()

	_, _, err := (CreateRetentionPolicy{Rules: h.service()}).
		Execute(context.Background(), actor(), createCommand(func(cmd *CreateRetentionPolicyCommand) {
			cmd.DataKind = domain.KindComment
		}))

	var domainErr *shared.Error
	if !errors.As(err, &domainErr) || domainErr.DetailCode != domain.CodeKindNotSwept {
		t.Fatalf("refused with %v", err)
	}
	if len(h.rules.stored) != 0 {
		t.Error("a refused rule was written")
	}
	if len(h.audit.entries) != 0 {
		t.Error("a refused rule wrote an audit entry")
	}
}

// §6's question: which rules actually apply here, and where each came from.
func TestTheListingSaysWhichRuleIsInForce(t *testing.T) {
	h := newRulesHarness()
	service := h.service()

	if _, _, err := (CreateRetentionPolicy{Rules: service}).Execute(context.Background(), actor(),
		createCommand(func(*CreateRetentionPolicyCommand) {})); err != nil {
		t.Fatalf("the tenant-wide rule: %v", err)
	}
	if _, _, err := (CreateRetentionPolicy{Rules: service}).Execute(context.Background(), actor(),
		createCommand(func(cmd *CreateRetentionPolicyCommand) {
			cmd.Scope = domain.Scope{Kind: domain.ScopeCollection, ID: collectionID}
			cmd.RetainDays = 30
		})); err != nil {
		t.Fatalf("the collection rule: %v", err)
	}

	listed, err := (ListRetentionPolicies{Rules: service}).
		Execute(context.Background(), actor(), collectionID, false)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("%d rules listed", len(listed))
	}

	for _, entry := range listed {
		wantInForce := entry.Rule.Scope.Kind == domain.ScopeCollection
		if entry.InForce != wantInForce {
			t.Errorf("the %s rule reports in_force %v", entry.Rule.Scope.Kind, entry.InForce)
		}
	}

	only, err := (ListRetentionPolicies{Rules: service}).
		Execute(context.Background(), actor(), collectionID, true)
	if err != nil {
		t.Fatalf("listing the effective ones: %v", err)
	}
	if len(only) != 1 || only[0].Rule.Scope.Kind != domain.ScopeCollection {
		t.Fatalf("the effective listing answered %d rules", len(only))
	}
}

// Without a container the question is "what exists", and nothing is in force anywhere in
// particular. Answering something else would be answering a question nobody asked.
func TestWithoutAContainerNothingIsInForce(t *testing.T) {
	h := newRulesHarness()
	service := h.service()
	if _, _, err := (CreateRetentionPolicy{Rules: service}).Execute(context.Background(), actor(),
		createCommand(func(*CreateRetentionPolicyCommand) {})); err != nil {
		t.Fatalf("creating: %v", err)
	}

	listed, err := (ListRetentionPolicies{Rules: service}).
		Execute(context.Background(), actor(), shared.ID(""), false)
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(listed) != 1 || listed[0].InForce {
		t.Fatalf("a rule reports in_force with no container named: %+v", listed)
	}
}

// A preview counts, shows and changes nothing.
func TestAPreviewCountsAndChangesNothing(t *testing.T) {
	h := newRulesHarness()
	h.marking.scopeCount, h.marking.countDue = 400, 20
	h.marking.due = []repository.Candidate{
		{
			ID: taskID, CollectionID: collectionID, HubID: hubID,
			Path: "/" + taskID.String() + "/", Title: "Weekly shop",
			AnchoredAt: now.Add(-400 * 24 * time.Hour),
		},
	}
	service := h.service()

	rule, _, err := (CreateRetentionPolicy{Rules: service}).Execute(context.Background(), actor(),
		createCommand(func(*CreateRetentionPolicyCommand) {}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}

	preview, err := (PreviewRetentionPolicy{Rules: service}).
		Execute(context.Background(), actor(), rule.ID)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}

	if preview.Matched != 20 {
		t.Errorf("the preview matched %d", preview.Matched)
	}
	if preview.ShareOfScope != 0.05 {
		t.Errorf("the share is %v, want 0.05", preview.ShareOfScope)
	}
	if len(preview.Samples) != 1 || preview.Samples[0].Title != "Weekly shop" {
		t.Fatalf("the samples are %+v", preview.Samples)
	}
	// The date is this object's own anchor plus the period and the grace, rather than one date for
	// the whole set.
	want := rule.EffectiveAt(h.marking.due[0].AnchoredAt)
	if !preview.Samples[0].EffectiveAt.Equal(want) {
		t.Errorf("the sample falls due at %s, want %s", preview.Samples[0].EffectiveAt, want)
	}
	// Nothing was marked, nothing was cleared, nothing was written.
	if len(h.marking.marked) != 0 || len(h.marking.archived) != 0 {
		t.Error("a preview changed something")
	}
}

// A legal hold is reported in the preview rather than counted as a sample: what an operator needs to
// see is that the rule has caught something it cannot act on.
func TestAPreviewReportsWhatALegalHoldWouldKeepBack(t *testing.T) {
	h := newRulesHarness()
	h.marking.scopeCount, h.marking.countDue = 400, 1
	h.marking.due = []repository.Candidate{{
		ID: taskID, CollectionID: collectionID, HubID: hubID,
		Path: "/" + taskID.String() + "/", Title: "Weekly shop",
		AnchoredAt: now.Add(-400 * 24 * time.Hour),
	}}
	h.holds.holds = domain.Holds{{
		ID: holdID, Scope: domain.HoldTenant, Reason: "Pending litigation", PlacedAt: now,
	}}
	service := h.service()

	rule, _, err := (CreateRetentionPolicy{Rules: service}).Execute(context.Background(), actor(),
		createCommand(func(*CreateRetentionPolicyCommand) {}))
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	preview, err := (PreviewRetentionPolicy{Rules: service}).
		Execute(context.Background(), actor(), rule.ID)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}

	if preview.Blocked[domain.BlockedByLegalHold] != 1 {
		t.Fatalf("the preview reports %+v", preview.Blocked)
	}
	if len(preview.Samples) != 0 {
		t.Error("an object a hold keeps back was shown as a sample of what would go")
	}
}

// The descriptors are what the parity gate compares against the routes and the catalogue, and the
// registry refuses an input key none of them declares.
func TestTheRuleDescriptorsTakeWhatTheControllerSends(t *testing.T) {
	full := usecase.Input{
		"data_kind": "COMPLETED_ITEM", "retain_days": 365, "action": "ARCHIVE",
		"scope": "COLLECTION", "scope_id": collectionID.String(),
		"then_after_days": 730, "then_action": "HARD_DELETE",
		"grace_days": 14, "notify_before_days": 7,
		"notify_recipients": []any{},
		"justification":     "The works council agreed a longer period",
		"condition":         "", "enabled": true,
		"export_target_id": collectionID.String(),
	}
	if err := (CreateRetentionPolicy{}).Descriptor().ValidateInput(full); err != nil {
		t.Fatalf("the input the controller builds was refused: %v", err)
	}
	if err := (ListRetentionPolicies{}).Descriptor().ValidateInput(usecase.Input{
		"container_id": collectionID.String(), "effective": true,
	}); err != nil {
		t.Fatalf("the listing's input was refused: %v", err)
	}
	if err := (PreviewRetentionPolicy{}).Descriptor().ValidateInput(usecase.Input{
		"policy_id": collectionID.String(),
	}); err != nil {
		t.Fatalf("the preview's input was refused: %v", err)
	}
	if (CreateRetentionPolicy{}).Descriptor().ReadOnly {
		t.Error("creating a rule is registered as read-only")
	}
	if !(PreviewRetentionPolicy{}).Descriptor().ReadOnly {
		t.Error("a preview is not registered as read-only")
	}
}

// The three channels reach these through the descriptor's handler rather than through Execute, so
// the mapping in between is exercised by the test that says the fields are right.
func TestTheHandlersMapWhatTheChannelsSendAndAnswer(t *testing.T) {
	h := newRulesHarness()
	h.marking.scopeCount, h.marking.countDue = 400, 4
	service := h.service()
	ctx := context.Background()

	created, err := (CreateRetentionPolicy{Rules: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{
			"data_kind": "COMPLETED_ITEM", "retain_days": 365, "action": "ARCHIVE",
			"then_after_days": 730, "then_action": "HARD_DELETE",
			"grace_days": 21, "enabled": true,
		})
	if err != nil {
		t.Fatalf("creating through the handler: %v", err)
	}

	switch {
	case created.String("data_kind") != "COMPLETED_ITEM":
		t.Errorf("the answer says %q", created.String("data_kind"))
	case created["retain_days"] != 365:
		t.Errorf("the period came back as %v", created["retain_days"])
	case created["grace_days"] != 21:
		t.Errorf("the grace period came back as %v", created["grace_days"])
	case created.String("then_action") != "HARD_DELETE":
		t.Errorf("the chain came back as %q", created.String("then_action"))
	}
	// Nothing warns anybody yet, so the rule carries no warning rather than one nothing sends.
	if _, present := created["notify"]; present {
		t.Errorf("the rule came back with a warning: %+v", created["notify"])
	}
	if preview, ok := created["preview"].(map[string]any); !ok || preview["matched"] != 4 {
		t.Fatalf("the answer carries no preview: %+v", created["preview"])
	}

	listed, err := (ListRetentionPolicies{Rules: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{"container_id": collectionID.String()})
	if err != nil {
		t.Fatalf("listing through the handler: %v", err)
	}
	rows, ok := listed["data"].([]usecase.Output)
	if !ok || len(rows) != 1 {
		t.Fatalf("the listing answered %+v", listed)
	}
	if rows[0]["in_force"] != true {
		t.Errorf("the only rule is not in force in the collection")
	}

	previewed, err := (PreviewRetentionPolicy{Rules: service}).Descriptor().Handler.Invoke(ctx, actor(),
		usecase.Input{"policy_id": created.String("id")})
	if err != nil {
		t.Fatalf("previewing through the handler: %v", err)
	}
	if previewed["matched"] != 4 {
		t.Errorf("the preview answered %+v", previewed)
	}
	if _, present := previewed["samples"]; !present {
		t.Error("the preview answers no samples at all, not even an empty list")
	}
}

// A rule with an export target and a justification round-trips through the handler too - both are
// fields a caller sends and reads back, and a mapping that dropped either would be silent.
func TestARuleKeepsItsExportTargetAndItsJustification(t *testing.T) {
	h := newRulesHarness()
	kind, _ := domain.FindKind(domain.KindTrash)
	if kind.Ceiling() > 0 {
		t.Skip("the trash has an upper bound, and this case is about a kind without one")
	}

	out, err := (CreateRetentionPolicy{Rules: h.service()}).Descriptor().Handler.Invoke(
		context.Background(), actor(), usecase.Input{
			"data_kind": "COMPLETED_ITEM", "retain_days": 365,
			"action": "EXPORT_THEN_DELETE", "export_target_id": hubID.String(),
			"justification": "Agreed with the works council",
		})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	if out.String("export_target_id") != hubID.String() {
		t.Errorf("the export target came back as %q", out.String("export_target_id"))
	}
	if out.String("justification") == "" {
		t.Error("the justification was dropped")
	}
}

// A rule that names no scope is the workspace's, which is the default a caller relies on.
func TestARuleWithNoScopeIsTheWorkspaces(t *testing.T) {
	h := newRulesHarness()

	out, err := (CreateRetentionPolicy{Rules: h.service()}).Descriptor().Handler.Invoke(
		context.Background(), actor(), usecase.Input{
			"data_kind": "COMPLETED_ITEM", "retain_days": 365, "action": "ARCHIVE",
		})
	if err != nil {
		t.Fatalf("creating: %v", err)
	}
	scope, present := out["scope"].(map[string]any)
	if !present || scope["kind"] != string(domain.ScopeTenant) {
		t.Fatalf("the scope came back as %+v", out["scope"])
	}
}
