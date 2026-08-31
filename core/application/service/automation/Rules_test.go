// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package automation

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/automation"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	expression "github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

var (
	tenant    = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")
	writerID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000b2")
	serviceID = shared.ID("01936f2a-7c1e-7000-8000-0000000000b3")
	personID  = shared.ID("01936f2a-7c1e-7000-8000-0000000000b4")
	newRuleID = shared.ID("01936f2a-7c1e-7000-8000-0000000000b5")
	hubID     = shared.ID("01936f2a-7c1e-7000-8000-0000000000b6")
	now       = time.Date(2026, 8, 28, 9, 0, 0, 0, time.UTC)
)

// ruleStore is the repository in memory, keyed the way the table is.
type ruleStore struct {
	rows      map[shared.ID]domain.Rule
	order     []shared.ID
	insertErr error
}

func newRuleStore(existing ...domain.Rule) *ruleStore {
	store := &ruleStore{rows: map[shared.ID]domain.Rule{}}
	for _, rule := range existing {
		store.rows[rule.ID] = rule
		store.order = append(store.order, rule.ID)
	}
	return store
}

func (s *ruleStore) Insert(_ context.Context, rule domain.Rule) error {
	if s.insertErr != nil {
		return s.insertErr
	}
	s.rows[rule.ID] = rule
	s.order = append(s.order, rule.ID)
	return nil
}

func (s *ruleStore) Find(_ context.Context, id shared.ID) (domain.Rule, error) {
	rule, found := s.rows[id]
	if !found || rule.IsDeleted() {
		return domain.Rule{}, shared.ErrNotFound.WithDetail("automation.rule_not_found")
	}
	return rule, nil
}

func (s *ruleStore) List(_ context.Context, query repository.Query) (repository.Page, error) {
	var rules []domain.Rule
	for _, id := range s.order {
		rule := s.rows[id]
		if rule.IsDeleted() {
			continue
		}
		if query.Enabled != nil && rule.Enabled != *query.Enabled {
			continue
		}
		rules = append(rules, rule)
	}
	return repository.Page{Rules: rules}, nil
}

func (s *ruleStore) Update(_ context.Context, rule domain.Rule, expected int) error {
	current, found := s.rows[rule.ID]
	if !found {
		return shared.ErrNotFound.WithDetail("automation.rule_not_found")
	}
	if current.Version != expected {
		return shared.ErrConflict.WithDetail("automation.version_conflict")
	}
	rule.Version = current.Version + 1
	s.rows[rule.ID] = rule
	return nil
}

func (s *ruleStore) SetEnabled(
	_ context.Context, id shared.ID, enabled bool, expected int, at time.Time,
) error {
	current, found := s.rows[id]
	if !found {
		return shared.ErrNotFound.WithDetail("automation.rule_not_found")
	}
	if current.Version != expected {
		return shared.ErrConflict.WithDetail("automation.version_conflict")
	}
	if enabled {
		s.rows[id] = current.Enable(at)
	} else {
		s.rows[id] = current.Disable(at)
	}
	return nil
}

func (s *ruleStore) Delete(_ context.Context, id shared.ID, at time.Time) (bool, error) {
	current, found := s.rows[id]
	if !found || current.IsDeleted() {
		return false, nil
	}
	stamp := at
	current.DeletedAt, current.Enabled = &stamp, false
	s.rows[id] = current
	return true, nil
}

// accounts answers what a rule would run as.
type accounts struct {
	rows map[shared.ID]identity.Account
}

func (a accounts) Find(_ context.Context, id shared.ID) (identity.Account, error) {
	account, found := a.rows[id]
	if !found {
		return identity.Account{}, shared.ErrNotFound.WithDetail("accounts.not_found")
	}
	return account, nil
}

// memberships answers what each account holds along a path.
type memberships struct {
	rows map[shared.ID][]identity.Membership
}

func (m memberships) Along(
	_ context.Context, accountID shared.ID, _ []identity.Scope,
) ([]identity.Membership, error) {
	return m.rows[accountID], nil
}

func (m memberships) SharedItemsIn(
	context.Context, shared.ID, shared.ID,
) ([]shared.ID, error) {
	return nil, nil
}

func (m memberships) Administrators(context.Context, []identity.Scope) ([]shared.ID, error) {
	return nil, nil
}

// authorizer answers the permission question, and records what it was asked.
type authorizer struct {
	refuse   bool
	requests []access.Request
}

func (a *authorizer) Authorize(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) error {
	a.requests = append(a.requests, request)
	if a.refuse {
		return shared.ErrForbidden.WithDetail("access.not_permitted")
	}
	return nil
}

func (a *authorizer) Permits(
	_ context.Context, _ appshared.ActorContext, request access.Request,
) (bool, error) {
	a.requests = append(a.requests, request)
	return !a.refuse, nil
}

type auditSink struct{ entries []audit.Entry }

func (s *auditSink) Append(_ context.Context, entry audit.Entry) error {
	s.entries = append(s.entries, entry)
	return nil
}

type unitOfWork struct{}

func (unitOfWork) Within(
	ctx context.Context, _ persistence.Scope, fn func(context.Context) error,
) error {
	return fn(ctx)
}

func (u unitOfWork) WithinReadOnly(
	ctx context.Context, scope persistence.Scope, fn func(context.Context) error,
) error {
	return u.Within(ctx, scope, fn)
}

type ids struct{ next shared.ID }

func (i ids) NewID() shared.ID { return i.next }

// catalogue is the use case registry as this package reads it: which action kinds exist, and what
// each declares.
type catalogue struct{ known map[string]usecase.Descriptor }

func (c catalogue) ByAutomationAction(kind string) (usecase.Descriptor, bool) {
	descriptor, found := c.known[kind]
	return descriptor, found
}

func defaultCatalogue() catalogue {
	return catalogue{known: map[string]usecase.Descriptor{
		"ADD_LABEL": {
			Name: "AddLabelToItem", TokenScope: "items:write",
			Input: []usecase.Field{{Name: "item_id"}, {Name: "label_id"}},
		},
		"CREATE_BUCKET": {
			Name: "CreateBucket", TokenScope: "containers:write",
			Input: []usecase.Field{{Name: "collection_id"}, {Name: "name"}},
		},
		"HTTP_REQUEST": {
			Name: "HttpRequest", TokenScope: "automation:manage",
			Input: []usecase.Field{
				{Name: "method"}, {Name: "url"}, {Name: "headers"},
				{Name: "secret_header_name"}, {Name: "secret_header_value"},
				{Name: "secret_header_sealed"}, {Name: "signature_header"},
				{Name: "body_template"}, {Name: "event_id"},
			},
		},
	}}
}

type harness struct {
	writer Writer
	store  *ruleStore
	auth   *authorizer
	sink   *auditSink
	people *accounts
	held   *memberships
	sealer *sealer
}

func newHarness(existing ...domain.Rule) *harness {
	h := &harness{
		store: newRuleStore(existing...),
		auth:  &authorizer{},
		sink:  &auditSink{},
		people: &accounts{rows: map[shared.ID]identity.Account{
			serviceID: {
				ID: serviceID, TenantID: tenant, Kind: identity.AccountServiceAccount,
				Status: identity.AccountActive,
			},
			personID: {
				ID: personID, TenantID: tenant, Kind: identity.AccountUser,
				Status: identity.AccountActive,
			},
		}},
		held:   &memberships{rows: map[shared.ID][]identity.Membership{}},
		sealer: &sealer{},
	}
	h.writer = Writer{
		Rules: h.store, Accounts: h.people, Memberships: h.held,
		Catalogue: defaultCatalogue(), Authorizer: h.auth, Audit: h.sink,
		Encryptor:  h.sealer,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: newRuleID},
	}
	return h
}

// writes gives an account a role at the tenant, which is the scope every fixture rule uses.
func (h *harness) roleOf(account shared.ID, role identity.Role) {
	h.held.rows[account] = []identity.Membership{
		{AccountID: account, Scope: identity.TenantScope(), Role: role},
	}
}

func writerActor(scopes ...string) appshared.ActorContext {
	if len(scopes) == 0 {
		scopes = []string{automationScope, "items:write", "containers:write"}
	}
	return appshared.ActorContext{
		TenantID: tenant, AccountID: writerID, AccountName: "Anna",
		Kind: shared.ActorUser, Scopes: scopes,
	}
}

func validCommand() CreateRuleCommand {
	return CreateRuleCommand{
		Name:  "Escalate overdue approvals",
		Scope: domain.Scope{Type: domain.ScopeTenant},
		RunAs: serviceID,
		Trigger: domain.Trigger{
			Kind: domain.TriggerEvent, EventType: event.ItemOverdue,
		},
		Actions: []domain.Action{
			{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}},
		},
	}
}

func detailOf(t *testing.T, err error) string {
	t.Helper()

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the error carries no code: %v", err)
	}
	return coded.DetailCode
}

func fieldCodes(t *testing.T, err error) map[string]string {
	t.Helper()

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no fields: %v", err)
	}
	found := map[string]string{}
	for _, finding := range coded.Fields {
		found[finding.Path] = finding.Code
	}
	return found
}

func TestARuleIsWrittenSwitchedOffAndAudited(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if rule.Enabled {
		t.Error("the rule was written switched on")
	}
	if _, found := h.store.rows[rule.ID]; !found {
		t.Fatal("the rule was not stored")
	}

	if len(h.sink.entries) != 1 {
		t.Fatalf("%d audit entries, want one", len(h.sink.entries))
	}
	entry := h.sink.entries[0]
	if entry.Action != RuleCreatedAction {
		t.Errorf("action %q, want %q", entry.Action, RuleCreatedAction)
	}
	// Whose rights the rule would act with, which is not the person who pressed the button.
	if entry.OnBehalfOf != serviceID {
		t.Errorf("on behalf of %s, want the run_as %s", entry.OnBehalfOf, serviceID)
	}
	// A title is something somebody wrote (rule 10).
	if entry.TargetLabel != "" {
		t.Errorf("the trail carries the rule's name: %q", entry.TargetLabel)
	}
}

// The composition rule, in the shape the task names it: a member with automation rights and nothing
// else must not write a rule that does what they may not.
func TestAWriterCannotLaunderARightThroughAServiceAccount(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleMember)
	h.roleOf(serviceID, identity.RoleAdmin)

	cmd := validCommand()
	cmd.Actions = []domain.Action{{Kind: "CREATE_BUCKET", Params: map[string]any{"name": "Doing"}}}

	// The writer holds the automation permission - the authoriser says yes - and the structure
	// scope their credential does not carry is what stops them.
	_, err := CreateRule{Writer: h.writer}.Execute(context.Background(),
		writerActor(automationScope, "items:write"), cmd)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if code := detailOf(t, err); code != "automation.writer_lacks_action_right" {
		t.Errorf("code %q, want automation.writer_lacks_action_right", code)
	}
	if len(h.store.rows) != 0 {
		t.Error("the rule was stored despite the refusal")
	}
}

// The general form of the same leak, and the half no list of scopes could close: you cannot make a
// rule act as an account that can do more at the scope than you can.
func TestARuleMayNotRunAsAnAccountThatOutranksItsWriter(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleMember)
	h.roleOf(serviceID, identity.RoleOwner)

	_, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if code := detailOf(t, err); code != "automation.run_as_exceeds_writer" {
		t.Errorf("code %q, want automation.run_as_exceeds_writer", code)
	}
}

func TestARuleMayRunAsAnAccountThatDoesNotOutrankItsWriter(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleAdmin)
	h.roleOf(serviceID, identity.RoleMember)

	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), validCommand()); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
}

// Acting as a colleague is impersonation, and no amount of automation permission is a grant of it -
// not even an owner's.
func TestARuleMayNotRunAsAnotherPerson(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(personID, identity.RoleViewer)

	cmd := validCommand()
	cmd.RunAs = personID

	_, err := CreateRule{Writer: h.writer}.Execute(context.Background(), writerActor(), cmd)
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if code := detailOf(t, err); code != "automation.run_as_not_delegable" {
		t.Errorf("code %q, want automation.run_as_not_delegable", code)
	}
}

// A rule that acts as its own writer delegates nothing, so there is nothing to compare.
func TestARuleMayRunAsItsWriter(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleMember)

	cmd := validCommand()
	cmd.RunAs = writerID

	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), cmd); err != nil {
		t.Fatalf("a rule acting as its own writer was refused: %v", err)
	}
}

func TestADisabledServiceAccountIsRefused(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.people.rows[serviceID] = identity.Account{
		ID: serviceID, TenantID: tenant, Kind: identity.AccountServiceAccount,
		Status: identity.AccountDisabled,
	}

	_, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if code := fieldCodes(t, err)["/run_as"]; code != "automation.run_as_inactive" {
		t.Errorf("the refusal says %q, want automation.run_as_inactive", code)
	}
}

// The vocabulary a rule may use is the vocabulary that can be executed, at every commit. G-09's
// pull request flips this one.
func TestAnActionNoReleaseServesIsRefusedByName(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	for _, kind := range DeferredActions() {
		t.Run(kind, func(t *testing.T) {
			cmd := validCommand()
			cmd.Actions = []domain.Action{{Kind: kind}}

			_, err := CreateRule{Writer: h.writer}.Execute(
				context.Background(), writerActor(), cmd)
			if !errors.Is(err, shared.ErrValidation) {
				t.Fatalf("error %v, want ErrValidation", err)
			}
			code := fieldCodes(t, err)["/actions/0/kind"]
			if code != "automation.action_not_available_yet" {
				t.Errorf("the refusal says %q, want automation.action_not_available_yet", code)
			}
		})
	}
}

// A kind nobody has ever named is a typo, and gets the code that says so rather than the one that
// sends its author to the milestone.
func TestAnActionNobodyDeclaresIsRefusedAsUnknown(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Actions = []domain.Action{{Kind: "ADD_LABLE"}}

	_, err := CreateRule{Writer: h.writer}.Execute(context.Background(), writerActor(), cmd)
	if code := fieldCodes(t, err)["/actions/0/kind"]; code != "automation.action_unknown" {
		t.Errorf("the refusal says %q, want automation.action_unknown", code)
	}
}

// The same refusal the call itself would give (C-07): a rule saved with a misspelled parameter
// would fail at a moment nobody is watching.
func TestAParameterTheUseCaseDoesNotDeclareIsRefused(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Actions = []domain.Action{
		{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x", "lable": "y"}},
	}

	_, err := CreateRule{Writer: h.writer}.Execute(context.Background(), writerActor(), cmd)
	if code := fieldCodes(t, err)["/actions/0/params/lable"]; code != "automation.param_unknown" {
		t.Errorf("the refusal says %q, want automation.param_unknown", code)
	}
}

// A rule supplies some parameters and the run supplies the rest - the entry an event is about is
// not a value a rule can carry - so requiring them at write time would refuse every correct rule.
func TestARequiredParameterIsNotDemandedAtWriteTime(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Actions = []domain.Action{{Kind: "ADD_LABEL", Params: map[string]any{"label_id": "x"}}}

	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), cmd); err != nil {
		t.Fatalf("a rule leaving item_id to the run was refused: %v", err)
	}
}

func TestThePermissionIsAskedAtTheRulesOwnScope(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Scope = domain.Scope{Type: domain.ScopeHub, ID: hubID}

	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), cmd); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	if len(h.auth.requests) == 0 {
		t.Fatal("the authoriser was not asked")
	}
	path := h.auth.requests[0].Path
	if len(path) != 2 || path[1] != identity.HubScope(hubID) {
		t.Errorf("asked about %v, want the path down to the hub", path)
	}
}

func TestAWriterWithoutTheAutomationPermissionIsRefused(t *testing.T) {
	h := newHarness()
	h.auth.refuse = true

	_, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if len(h.store.rows) != 0 {
		t.Error("the rule was stored despite the refusal")
	}
}

func TestEnablingAndDisablingAreSeparateAndSeparatelyAudited(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	enabled, err := EnableRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), rule.ID)
	if err != nil {
		t.Fatalf("enabling: %v", err)
	}
	if !enabled.Enabled {
		t.Error("the rule is still off")
	}

	disabled, err := DisableRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), rule.ID)
	if err != nil {
		t.Fatalf("disabling: %v", err)
	}
	if disabled.Enabled {
		t.Error("the rule is still on")
	}

	var actions []audit.Action
	for _, entry := range h.sink.entries {
		actions = append(actions, entry.Action)
	}
	want := []audit.Action{RuleCreatedAction, RuleEnabledAction, RuleDisabledAction}
	if len(actions) != len(want) {
		t.Fatalf("audit actions %v, want %v", actions, want)
	}
	for i := range want {
		if actions[i] != want[i] {
			t.Errorf("entry %d is %q, want %q", i, actions[i], want[i])
		}
	}
}

// Switching a rule on is what makes its actions happen, so it needs the rights writing them did -
// and asked again, because the writer's may have narrowed since.
func TestEnablingReChecksTheCompositionRule(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	// The service account is promoted past the writer after the rule was written.
	h.roleOf(serviceID, identity.RoleOwner)
	h.roleOf(writerID, identity.RoleMember)

	if _, err := (EnableRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), rule.ID); !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
}

// Stopping a rule is taking a power away, and somebody who may manage rules here should never be
// unable to stop one.
func TestDisablingNeedsOnlyTheAutomationPermission(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if _, err := (EnableRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), rule.ID); err != nil {
		t.Fatalf("enabling: %v", err)
	}

	// The writer loses the rights the rule's actions need, and the service account outranks them.
	h.roleOf(serviceID, identity.RoleOwner)
	h.roleOf(writerID, identity.RoleMember)

	if _, err := (DisableRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(automationScope), rule.ID); err != nil {
		t.Fatalf("stopping a rule was refused: %v", err)
	}
}

// A rule may not be edited into doing something its writer may not do - the same laundering, in two
// steps.
func TestAnEditIsCheckedAgainstTheRuleAsItWouldBe(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	actions := []domain.Action{{Kind: "CREATE_BUCKET", Params: map[string]any{"name": "Doing"}}}
	_, err = UpdateRule{Writer: h.writer}.Execute(context.Background(),
		writerActor(automationScope, "items:write"),
		UpdateRuleCommand{ID: rule.ID, Actions: &actions})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Fatalf("error %v, want ErrForbidden", err)
	}
	if code := detailOf(t, err); code != "automation.writer_lacks_action_right" {
		t.Errorf("code %q, want automation.writer_lacks_action_right", code)
	}
}

// An omitted field is left alone, and the constructor's own decisions do not leak into an edit.
func TestAnEditLeavesWhatItDidNotName(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}
	if _, err := (EnableRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), rule.ID); err != nil {
		t.Fatalf("enabling: %v", err)
	}

	renamed := "Escalate approvals, quietly"
	updated, err := UpdateRule{Writer: h.writer}.Execute(context.Background(), writerActor(),
		UpdateRuleCommand{ID: rule.ID, Name: &renamed})
	if err != nil {
		t.Fatalf("editing: %v", err)
	}
	if updated.Name != renamed {
		t.Errorf("name %q, want %q", updated.Name, renamed)
	}
	// An edit must not switch a rule off as a side effect of the constructor minting a fresh one.
	if !updated.Enabled {
		t.Error("editing the name switched the rule off")
	}
	if updated.Trigger.EventType != event.ItemOverdue {
		t.Errorf("the trigger changed to %q", updated.Trigger.EventType)
	}
	if !updated.CreatedAt.Equal(rule.CreatedAt) {
		t.Errorf("created_at moved to %v", updated.CreatedAt)
	}
}

func TestAnEditWithAStaleVersionIsAConflict(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	renamed := "Something else"
	_, err = UpdateRule{Writer: h.writer}.Execute(context.Background(), writerActor(),
		UpdateRuleCommand{ID: rule.ID, Name: &renamed, ExpectedVersion: 99})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("error %v, want ErrConflict", err)
	}
}

// The deletion is soft, and removing a rule takes a power away rather than granting one - so it
// asks for the plain permission and not the composition rule.
func TestDeletingIsSoftAndNeedsOnlyTheAutomationPermission(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := CreateRule{Writer: h.writer}.Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	h.roleOf(serviceID, identity.RoleOwner)
	h.roleOf(writerID, identity.RoleMember)

	if err := (DeleteRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(automationScope), rule.ID); err != nil {
		t.Fatalf("deleting: %v", err)
	}

	stored := h.store.rows[rule.ID]
	if !stored.IsDeleted() {
		t.Error("the row is gone rather than stamped")
	}
	if stored.Enabled {
		t.Error("a deleted rule is still marked enabled")
	}
	if _, err := (GetRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), rule.ID); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("a deleted rule is still found: %v", err)
	}
}

// A rule the caller may not see at its own scope is left out rather than refusing the page: a 403
// for the whole listing would hide the rules they may read.
func TestTheListingLeavesOutWhatTheCallerMayNotSee(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), validCommand()); err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	h.auth.refuse = true
	page, err := ListRules{Writer: h.writer}.Execute(
		context.Background(), writerActor(), repository.Query{})
	if err != nil {
		t.Fatalf("listing: %v", err)
	}
	if len(page.Rules) != 0 {
		t.Errorf("%d rules answered, want none", len(page.Rules))
	}
}

func TestTheListingNeedsTheAutomationScope(t *testing.T) {
	h := newHarness()

	_, err := ListRules{Writer: h.writer}.Execute(context.Background(),
		writerActor("items:read"), repository.Query{})
	if !errors.Is(err, shared.ErrForbidden) {
		t.Errorf("error %v, want ErrForbidden", err)
	}
}

// The list shrinks as the tasks land, and this is what makes removing an entry something nobody has
// to remember: a kind the catalogue serves and this list still names would be a kind refused for a
// reason that stopped being true.
func TestNoDeferredActionIsAlreadyServed(t *testing.T) {
	catalogue := defaultCatalogue()
	for _, kind := range DeferredActions() {
		if _, served := catalogue.ByAutomationAction(kind); served {
			t.Errorf("%s is served and still listed as deferred", kind)
		}
	}
}

// compiler is the expression port as this package sees it: a fake, because core/application may not
// import the adapter that holds the engine (ADR-0001) - and because what these tests are about is
// the mapping from a refusal to a field error, not whether CEL parses. That the real engine reports
// the position, and refuses an undeclared name, is proved where the engine is.
//
// The grammar it knows is deliberately tiny and deliberately shaped like CEL's failures: an empty
// tail is a syntax error with a position, and a name outside the environment is its own code.
type compiler struct{}

func (compiler) Compile(
	text string, env expression.Environment, _ expression.Result,
) (expression.Program, error) {
	declared := map[string]bool{}
	for _, variable := range env.Variables {
		declared[variable.Name] = true
	}

	trimmed := strings.TrimSpace(text)
	switch {
	case trimmed == "" || strings.HasSuffix(trimmed, "==") || strings.HasSuffix(trimmed, "===") ||
		strings.HasSuffix(trimmed, "."):
		return nil, expression.Refusal{
			Code: expression.CodeSyntax, Position: expression.Position{Line: 1, Column: len(trimmed)},
		}.Error()
	}

	root := trimmed
	if cut := strings.IndexAny(root, " ."); cut > 0 {
		root = root[:cut]
	}
	// The two literals are not names, and an environment does not declare them.
	if root != "true" && root != "false" && !declared[root] {
		return nil, expression.Refusal{
			Code: expression.CodeUnknownName, Position: expression.Position{Line: 1, Column: 1},
		}.Error()
	}
	return answer{no: trimmed == "false"}, nil
}

// answer is the compiled half of the fake. `false` is the only expression it reads as false, which
// is enough for the engine's tests to distinguish a condition that held from one that did not
// without teaching this stub a language.
type answer struct{ no bool }

func (a answer) Evaluate(context.Context, expression.Activation) (expression.Value, error) {
	return expression.Value{Bool: !a.no}, nil
}

// The other half of G-06's flip, and the half that proves the language is really wired: a rule's
// condition is compiled when it is written, against exactly the names automation.md §1.2 declares.

func TestAValidConditionIsAcceptedAndCompiled(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Conditions = []domain.Condition{
		{Expr: "item.type == 'TASK'"},
		{Expr: "now.getHours() >= 8 && now.getHours() < 18"},
	}
	cmd.Throttle = domain.Throttle{MaxRunsPerHour: 100, DedupeKeyExpr: "item.id"}

	rule, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), cmd)
	if err != nil {
		t.Fatalf("a valid condition was refused: %v", err)
	}
	if len(rule.Conditions) != 2 {
		t.Errorf("%d conditions stored, want two", len(rule.Conditions))
	}
	if rule.Throttle.DedupeKeyExpr != "item.id" {
		t.Errorf("the dedupe key was not kept: %q", rule.Throttle.DedupeKeyExpr)
	}
}

// A typo is answered to its author with a line and a column while they are still looking at it,
// rather than to a log at three in the morning.
func TestAParseErrorBecomesAFieldErrorWithItsPosition(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Conditions = []domain.Condition{
		{Expr: "item.type == 'TASK'"},
		{Expr: "item.type == "},
	}

	_, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), cmd)
	if !errors.Is(err, shared.ErrValidation) {
		t.Fatalf("error %v, want ErrValidation", err)
	}

	var coded *shared.Error
	if !errors.As(err, &coded) {
		t.Fatalf("the refusal carries no fields: %v", err)
	}
	var found bool
	for _, finding := range coded.Fields {
		if finding.Path != "/conditions/1/expr" {
			continue
		}
		found = true
		if finding.Code != "expression.syntax" {
			t.Errorf("code %q, want expression.syntax", finding.Code)
		}
		if finding.Params["line"] == "" || finding.Params["column"] == "" {
			t.Errorf("the finding carries no position: %v", finding.Params)
		}
	}
	if !found {
		t.Errorf("no finding names the condition that is wrong: %+v", coded.Fields)
	}
	if len(h.store.rows) != 0 {
		t.Error("the rule was stored despite the refusal")
	}
}

// The check that makes the documented variable list a contract: a condition naming something the
// engine will never provide fails when it is written.
func TestAConditionNamingAnUndeclaredVariableIsRefused(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Conditions = []domain.Condition{{Expr: "secret.value == 1"}}

	_, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), cmd)
	if code := fieldCodes(t, err)["/conditions/0/expr"]; code != "expression.unknown_name" {
		t.Errorf("the refusal says %q, want expression.unknown_name", code)
	}
}

// A dedupe key is compiled with the conditions, and its refusal names its own field.
func TestAnInvalidDedupeKeyIsRefusedAtItsOwnField(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Throttle = domain.Throttle{DedupeKeyExpr: "item."}

	_, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), cmd)
	if code := fieldCodes(t, err)["/throttle/dedupe_key_expr"]; code == "" {
		t.Errorf("no finding names the dedupe key: %v", err)
	}
}

// An edit is checked the same way, against the rule as it would be.
func TestAnEditWithABrokenConditionIsRefused(t *testing.T) {
	h := newHarness()
	h.writer.Conditions = compiler{}
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	rule, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), validCommand())
	if err != nil {
		t.Fatalf("writing the rule: %v", err)
	}

	broken := []domain.Condition{{Expr: "item.type ==="}}
	if _, err := (UpdateRule{Writer: h.writer}).Execute(context.Background(), writerActor(),
		UpdateRuleCommand{ID: rule.ID, Conditions: &broken},
	); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("error %v, want ErrValidation", err)
	}
}

// Fail closed. A build with no evaluator wired cannot promise that a condition means what it says,
// and storing one on that promise is exactly the failure the old refusal existed for.
func TestAConditionIsRefusedWhenNoEngineIsWired(t *testing.T) {
	h := newHarness()
	h.roleOf(writerID, identity.RoleOwner)
	h.roleOf(serviceID, identity.RoleMember)

	cmd := validCommand()
	cmd.Conditions = []domain.Condition{{Expr: "item.type == 'TASK'"}}

	_, err := (CreateRule{Writer: h.writer}).Execute(context.Background(), writerActor(), cmd)
	if !errors.Is(err, shared.ErrInternal) {
		t.Fatalf("error %v, want ErrInternal", err)
	}

	// And a rule with no conditions is unaffected: nothing needs an evaluator.
	if _, err := (CreateRule{Writer: h.writer}).Execute(
		context.Background(), writerActor(), validCommand()); err != nil {
		t.Errorf("a rule with no conditions needed an evaluator: %v", err)
	}
}
