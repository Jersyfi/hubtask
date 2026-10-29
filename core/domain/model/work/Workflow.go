// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// StatusCategory is what a status means to everything that does not know the workflow: a board's
// grouping, a figure, an SLA, and completion (UC-FLW-03 check 4) - an entry is done exactly while its status is
// in DONE.
type StatusCategory string

const (
	CategoryBacklog    StatusCategory = "BACKLOG"
	CategoryTodo       StatusCategory = "TODO"
	CategoryInProgress StatusCategory = "IN_PROGRESS"
	CategoryDone       StatusCategory = "DONE"
)

var statusCategories = [...]StatusCategory{CategoryBacklog, CategoryTodo, CategoryInProgress, CategoryDone}

// StatusCategories answers the four, in the order a process runs through them.
func StatusCategories() []StatusCategory { return statusCategories[:] }

// Valid reports whether the category is one of the four.
func (c StatusCategory) Valid() bool { return slices.Contains(statusCategories[:], c) }

// Completes reports whether an entry in a status of this category is done.
func (c StatusCategory) Completes() bool { return c == CategoryDone }

// SystemWorkflowKey is the key of the workflow every workspace has: To do, In progress, Done.
const SystemWorkflowKey = "default"

// Bounds of a workflow: generous for a process somebody draws, far short of a pasted document.
const (
	MaxWorkflowStatuses = 30
	MaxWorkflowSteps    = 100
	MaxStepChecks       = 20
)

var workflowKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,49}$`)

// WorkflowStatus is one place an entry can be in.
type WorkflowStatus struct {
	Key string
	// Name is empty on a status of the system workflow: a client shows its own word for it
	// (rule 8).
	Name       string
	Category   StatusCategory
	ColorToken string
}

// StepWho limits who may take a step (UC-FLW-04 check 1). Empty is everybody who may change the
// entry; otherwise the person must meet one of the roles, groups or person fields, and the
// condition.
type StepWho struct {
	// Roles are built-in role names or custom role keys held along the entry's path.
	Roles []string
	// PersonFields are `assignee`, `created_by` or `custom_fields.<key>` of a person or people
	// field: the person must be the one it names.
	PersonFields []string
	// Groups are the identifiers of the workspace's groups whose members may take the step.
	Groups []string
	// Condition is CEL over the entry and the actor, in the form's environment.
	Condition string
}

// IsOpen reports whether the step limits nobody.
func (w StepWho) IsOpen() bool {
	return len(w.Roles) == 0 && len(w.PersonFields) == 0 && len(w.Groups) == 0 &&
		strings.TrimSpace(w.Condition) == ""
}

// StepCheck is a condition that must hold before a step is taken, with the team's own words for
// the refusal (UC-FLW-04 check 2): one text, the same per language (`Messages`, a language tag to
// its words), or a message picked from the shipped list (`MessageCode`), which a client renders in
// the reader's language. A reader's language with no words of its own falls back to Message, then
// to the picked message, then to the check's key.
type StepCheck struct {
	Key         string
	Holds       string
	Message     string
	Messages    map[string]string
	MessageCode string
}

// StepCheckMessageCodes are the shipped messages a check may pick, each a catalogue entry
// `workflows.check_message.<code>` in every language.
var StepCheckMessageCodes = []string{
	"fill_required", "attach_evidence", "needs_approval", "amount_positive", "dates_in_order", "needs_assignee",
}

// MaxStepCheckLanguages bounds the languages one check's words are written in.
const MaxStepCheckLanguages = 20

var languageTagPattern = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

// WorkflowStep is a transition: from one or more statuses - or from any - to one (UC-FLW-02 check
// 1), with who may take it, what must hold, what it asks for, and the fields it requires.
type WorkflowStep struct {
	Key  string
	Name string
	// From is empty for "from any status but its own target".
	From []string
	To   string
	Who  StepWho
	// Requires are form keys (`due_date`, `custom_fields.<key>`) that must carry a value when the
	// step is taken.
	Requires []string
	// Asks are form keys a client asks for in a small form when the step is taken (UC-FLW-04
	// check 3); asked is not required unless Requires names it too.
	Asks   []string
	Checks []StepCheck
}

// LeavesFrom reports whether the step may be taken from a status.
func (s WorkflowStep) LeavesFrom(status string) bool {
	if status == s.To {
		return false
	}
	return len(s.From) == 0 || slices.Contains(s.From, status)
}

// WorkflowShape is what a workflow is: its statuses, the one a new entry starts in, and its steps.
type WorkflowShape struct {
	Statuses []WorkflowStatus
	Initial  string
	Steps    []WorkflowStep
}

// Status answers a status by key.
func (s WorkflowShape) Status(key string) (WorkflowStatus, bool) {
	for _, status := range s.Statuses {
		if status.Key == key {
			return status, true
		}
	}
	return WorkflowStatus{}, false
}

// Step answers a step by key.
func (s WorkflowShape) Step(key string) (WorkflowStep, bool) {
	for _, step := range s.Steps {
		if step.Key == key {
			return step, true
		}
	}
	return WorkflowStep{}, false
}

// StepsFrom answers the steps that leave a status, in the workflow's order.
func (s WorkflowShape) StepsFrom(status string) []WorkflowStep {
	var steps []WorkflowStep
	for _, step := range s.Steps {
		if step.LeavesFrom(status) {
			steps = append(steps, step)
		}
	}
	return steps
}

// FirstDone answers the first status of the DONE category: where a completed entry lands when it
// enters the workflow.
func (s WorkflowShape) FirstDone() string {
	for _, status := range s.Statuses {
		if status.Category.Completes() {
			return status.Key
		}
	}
	return ""
}

// Completes reports whether a status of the shape completes an entry; an unknown one does not.
func (s WorkflowShape) Completes(key string) bool {
	status, found := s.Status(key)
	return found && status.Category.Completes()
}

// Entering answers the status an entry takes when it enters the workflow: its status where the
// workflow has it on the same side of completion, else the first done status for a completed
// entry and the initial one for an open entry. Entering never changes completion.
func (s WorkflowShape) Entering(status string, completed bool) string {
	if known, found := s.Status(status); found && known.Category.Completes() == completed {
		return status
	}
	if completed {
		return s.FirstDone()
	}
	return s.Initial
}

// Columns answers the status keys in order: a board's columns in a workflow collection.
func (s WorkflowShape) Columns() []string {
	keys := make([]string, 0, len(s.Statuses))
	for _, status := range s.Statuses {
		keys = append(keys, status.Key)
	}
	return keys
}

// Workflow is a workspace's description of how its work moves (UC-FLW-02): a shape in force and,
// while somebody changes it, a draft that changes nothing until it is published (UC-FLW-02 check 3,
// UC-FLW-05 check 1).
type Workflow struct {
	// ID is zero for the system workflow while the workspace has never used or changed it.
	ID       shared.ID
	TenantID shared.ID
	Key      string
	System   bool
	// Name is empty on the system workflow nobody named (rule 8).
	Name        string
	Published   WorkflowShape
	Draft       *WorkflowShape
	PublishedAt *time.Time
	PublishedBy shared.ID
	CreatedAt   time.Time
	UpdatedAt   time.Time
	Version     int
}

// Stored reports whether the workflow has a row.
func (w Workflow) Stored() bool { return !w.ID.IsZero() }

// SystemWorkflow is the workflow Hubtask ships, which works without editing (UC-FLW-02 check 7):
// To do, In progress and Done, and a step into each from any other.
func SystemWorkflow() Workflow {
	return Workflow{Key: SystemWorkflowKey, System: true, Published: WorkflowShape{
		Statuses: []WorkflowStatus{
			{Key: "todo", Category: CategoryTodo, ColorToken: string(shared.LabelTokenSlate)},
			{Key: "in_progress", Category: CategoryInProgress, ColorToken: string(shared.LabelTokenBlue)},
			{Key: "done", Category: CategoryDone, ColorToken: string(shared.LabelTokenGreen)},
		},
		Initial: "todo",
		Steps: []WorkflowStep{
			{Key: "to_todo", To: "todo"},
			{Key: "start", To: "in_progress"},
			{Key: "finish", To: "done"},
		},
	}}
}

// Workflows is a workspace's set: its rows over the system workflow.
type Workflows []Workflow

// NewWorkflows merges a workspace's rows over the system workflow.
func NewWorkflows(rows []Workflow) Workflows {
	out := Workflows{}
	system := SystemWorkflow()
	for _, row := range rows {
		if row.Key == SystemWorkflowKey {
			system = row
			system.System = true
		}
	}
	out = append(out, system)
	for _, row := range rows {
		if row.Key != SystemWorkflowKey {
			out = append(out, row)
		}
	}
	return out
}

// Find answers a workflow by key.
func (ws Workflows) Find(key string) (Workflow, bool) {
	for _, w := range ws {
		if w.Key == key {
			return w, true
		}
	}
	return Workflow{}, false
}

// NewWorkflowInput is what a workspace's own workflow is made of.
type NewWorkflowInput struct {
	ID       shared.ID
	TenantID shared.ID
	Key      string
	Name     string
	Shape    WorkflowShape
	By       shared.ID
	Now      time.Time
	Text     text.Normalizer
}

// NewWorkflow builds a workspace's own workflow, published as it is made: a new workflow is in
// force nowhere until a collection or hub uses it, so its first shape needs no draft.
func NewWorkflow(in NewWorkflowInput) (Workflow, error) {
	if in.ID.IsZero() || in.TenantID.IsZero() {
		return Workflow{}, shared.ErrInternal.WithDetail("workflows.identity_incomplete")
	}
	key := strings.TrimSpace(in.Key)
	if !workflowKeyPattern.MatchString(key) {
		return Workflow{}, workflowInvalid("/key", "workflows.key_malformed", nil)
	}
	if key == SystemWorkflowKey {
		return Workflow{}, workflowInvalid("/key", "workflows.key_reserved", nil)
	}
	name, err := workflowName(in.Name, false, in.Text)
	if err != nil {
		return Workflow{}, err
	}
	shape, err := NewWorkflowShape(in.Shape, false, in.Text)
	if err != nil {
		return Workflow{}, err
	}
	now := in.Now
	return Workflow{
		ID: in.ID, TenantID: in.TenantID, Key: key, Name: name, Published: shape,
		PublishedAt: &now, PublishedBy: in.By, CreatedAt: in.Now, UpdatedAt: in.Now, Version: 1,
	}, nil
}

// Renamed answers the workflow under a new name; empty is allowed only on the system workflow.
func (w Workflow) Renamed(raw string, now time.Time, form text.Normalizer) (Workflow, error) {
	name, err := workflowName(raw, w.System, form)
	if err != nil {
		return Workflow{}, err
	}
	next := w
	next.Name, next.UpdatedAt = name, now
	return next, nil
}

// WithDraft answers the workflow with a checked draft beside the shape in force.
func (w Workflow) WithDraft(shape WorkflowShape, now time.Time, form text.Normalizer) (Workflow, error) {
	checked, err := NewWorkflowShape(shape, w.System, form)
	if err != nil {
		return Workflow{}, err
	}
	next := w
	next.Draft, next.UpdatedAt = &checked, now
	return next, nil
}

// Discarded answers the workflow without its draft.
func (w Workflow) Discarded(now time.Time) Workflow {
	next := w
	next.Draft, next.UpdatedAt = nil, now
	return next
}

// Publish answers the workflow with its draft in force. The moves of the entries are the
// application's (PlanStatusMoves), in the same transaction.
func (w Workflow) Publish(by shared.ID, now time.Time) (Workflow, error) {
	if w.Draft == nil {
		return Workflow{}, shared.ErrConflict.WithDetail("workflows.no_draft")
	}
	next := w
	next.Published, next.Draft = *w.Draft, nil
	next.PublishedAt, next.PublishedBy, next.UpdatedAt = &now, by, now
	return next, nil
}

// NewWorkflowShape checks a shape (UC-FLW-02 check 1): statuses with unique keys and known
// categories and colours, at least one done status, an initial status that is open, steps with
// unique keys into a known status from known ones. A status needs a name, except one the system
// workflow ships, which a client names in its own words (rule 8).
func NewWorkflowShape(in WorkflowShape, system bool, form text.Normalizer) (WorkflowShape, error) {
	shipped := SystemWorkflow().Published
	if len(in.Statuses) == 0 {
		return WorkflowShape{}, workflowInvalid("/statuses", "workflows.statuses_empty", nil)
	}
	if len(in.Statuses) > MaxWorkflowStatuses {
		return WorkflowShape{}, workflowInvalid("/statuses", "workflows.too_many_statuses",
			map[string]string{"maximum": strconv.Itoa(MaxWorkflowStatuses)})
	}
	if len(in.Steps) > MaxWorkflowSteps {
		return WorkflowShape{}, workflowInvalid("/steps", "workflows.too_many_steps",
			map[string]string{"maximum": strconv.Itoa(MaxWorkflowSteps)})
	}
	out := WorkflowShape{}
	for i, status := range in.Statuses {
		at := "/statuses/" + strconv.Itoa(i)
		status.Key = strings.TrimSpace(status.Key)
		if !workflowKeyPattern.MatchString(status.Key) {
			return WorkflowShape{}, workflowInvalid(at+"/key", "workflows.key_malformed", nil)
		}
		if _, taken := out.Status(status.Key); taken {
			return WorkflowShape{}, workflowInvalid(at+"/key", "workflows.key_taken", map[string]string{"key": status.Key})
		}
		if !status.Category.Valid() {
			return WorkflowShape{}, workflowInvalid(at+"/category", "workflows.category_unknown", nil)
		}
		if status.ColorToken == "" {
			status.ColorToken = string(shared.LabelTokenSlate)
		}
		if !shared.IsLabelToken(status.ColorToken) {
			return WorkflowShape{}, workflowInvalid(at+"/color_token", "workflows.color_token_unknown", nil)
		}
		_, unnamed := shipped.Status(status.Key)
		name, err := workflowName(status.Name, system && unnamed, form)
		if err != nil {
			return WorkflowShape{}, atFormPath(err, at+"/name")
		}
		status.Name = name
		out.Statuses = append(out.Statuses, status)
	}
	if out.FirstDone() == "" {
		return WorkflowShape{}, workflowInvalid("/statuses", "workflows.no_done_status", nil)
	}
	initial := strings.TrimSpace(in.Initial)
	if initial == "" {
		for _, status := range out.Statuses {
			if !status.Category.Completes() {
				initial = status.Key
				break
			}
		}
	}
	if start, found := out.Status(initial); !found || start.Category.Completes() {
		return WorkflowShape{}, workflowInvalid("/initial", "workflows.initial_invalid", nil)
	}
	out.Initial = initial
	for i, step := range in.Steps {
		checked, err := newWorkflowStep(step, out, form, "/steps/"+strconv.Itoa(i))
		if err != nil {
			return WorkflowShape{}, err
		}
		out.Steps = append(out.Steps, checked)
	}
	return out, nil
}

func newWorkflowStep(step WorkflowStep, shape WorkflowShape, form text.Normalizer, at string) (WorkflowStep, error) {
	step.Key = strings.TrimSpace(step.Key)
	if !workflowKeyPattern.MatchString(step.Key) {
		return WorkflowStep{}, workflowInvalid(at+"/key", "workflows.key_malformed", nil)
	}
	if _, taken := shape.Step(step.Key); taken {
		return WorkflowStep{}, workflowInvalid(at+"/key", "workflows.key_taken", map[string]string{"key": step.Key})
	}
	name, err := workflowName(step.Name, true, form)
	if err != nil {
		return WorkflowStep{}, atFormPath(err, at+"/name")
	}
	step.Name = name
	if _, found := shape.Status(step.To); !found {
		return WorkflowStep{}, workflowInvalid(at+"/to", "workflows.status_unknown", map[string]string{"key": step.To})
	}
	from := []string{}
	for _, key := range step.From {
		if _, found := shape.Status(key); !found {
			return WorkflowStep{}, workflowInvalid(at+"/from", "workflows.status_unknown", map[string]string{"key": key})
		}
		if key != step.To && !slices.Contains(from, key) {
			from = append(from, key)
		}
	}
	if len(step.From) > 0 && len(from) == 0 {
		return WorkflowStep{}, workflowInvalid(at+"/from", "workflows.step_goes_nowhere", nil)
	}
	step.From = from
	for _, list := range []*[]string{&step.Requires, &step.Asks} {
		kept := []string{}
		for _, key := range *list {
			if !validFormFieldKey(key) {
				return WorkflowStep{}, workflowInvalid(at+"/requires", "workflows.field_unknown", map[string]string{"key": key})
			}
			if !slices.Contains(kept, key) {
				kept = append(kept, key)
			}
		}
		*list = kept
	}
	who, err := newStepWho(step.Who, at+"/who")
	if err != nil {
		return WorkflowStep{}, err
	}
	step.Who = who
	if len(step.Checks) > MaxStepChecks {
		return WorkflowStep{}, workflowInvalid(at+"/checks", "workflows.too_many_checks",
			map[string]string{"maximum": strconv.Itoa(MaxStepChecks)})
	}
	checks := []StepCheck{}
	for i, check := range step.Checks {
		checkAt := at + "/checks/" + strconv.Itoa(i)
		check.Key = strings.TrimSpace(check.Key)
		if !workflowKeyPattern.MatchString(check.Key) {
			return WorkflowStep{}, workflowInvalid(checkAt+"/key", "workflows.key_malformed", nil)
		}
		for _, other := range checks {
			if other.Key == check.Key {
				return WorkflowStep{}, workflowInvalid(checkAt+"/key", "workflows.key_taken", map[string]string{"key": check.Key})
			}
		}
		if strings.TrimSpace(check.Holds) == "" {
			return WorkflowStep{}, workflowInvalid(checkAt+"/holds", "workflows.check_empty", nil)
		}
		if len(check.Holds) > MaxFormExpressionLength {
			return WorkflowStep{}, workflowInvalid(checkAt+"/holds", "expression.too_long", nil)
		}
		message, err := formHelp(check.Message, form, checkAt+"/message")
		if err != nil {
			return WorkflowStep{}, err
		}
		check.Message = message
		if check.Messages, err = stepCheckMessages(check.Messages, form, checkAt+"/messages"); err != nil {
			return WorkflowStep{}, err
		}
		check.MessageCode = strings.TrimSpace(check.MessageCode)
		if check.MessageCode != "" && !slices.Contains(StepCheckMessageCodes, check.MessageCode) {
			return WorkflowStep{}, workflowInvalid(checkAt+"/message_code", "workflows.message_code_unknown",
				map[string]string{"code": check.MessageCode})
		}
		checks = append(checks, check)
	}
	step.Checks = checks
	return step, nil
}

// stepRoles are the built-in roles a step may name.
var stepRoles = []string{"OWNER", "ADMIN", "MEMBER", "CONTRIBUTOR", "GUEST"}

func newStepWho(who StepWho, at string) (StepWho, error) {
	roles := []string{}
	for _, role := range who.Roles {
		// A custom role's key is lower case and cannot be mistaken for a built-in one; whether the
		// workspace defines it is the use case's question, which reads the roles.
		if !slices.Contains(stepRoles, role) && !identity.ValidCustomRoleKey(role) {
			return StepWho{}, workflowInvalid(at+"/roles", "workflows.role_unknown", map[string]string{"role": role})
		}
		if !slices.Contains(roles, role) {
			roles = append(roles, role)
		}
	}
	people := []string{}
	for _, field := range who.PersonFields {
		_, custom := strings.CutPrefix(field, FormCustomPrefix)
		if field != "assignee" && field != "created_by" && !(custom && validFormFieldKey(field)) {
			return StepWho{}, workflowInvalid(at+"/person_fields", "workflows.field_unknown", map[string]string{"key": field})
		}
		if !slices.Contains(people, field) {
			people = append(people, field)
		}
	}
	groups := []string{}
	for _, group := range who.Groups {
		// Whether the workspace holds the group is the use case's question, as it is for a role.
		id, err := shared.ParseID(group)
		if err != nil || id.IsZero() {
			return StepWho{}, workflowInvalid(at+"/groups", "workflows.group_unknown", map[string]string{"group": group})
		}
		if !slices.Contains(groups, id.String()) {
			groups = append(groups, id.String())
		}
	}
	condition := strings.TrimSpace(who.Condition)
	if len(condition) > MaxFormExpressionLength {
		return StepWho{}, workflowInvalid(at+"/condition", "expression.too_long", nil)
	}
	return StepWho{Roles: roles, PersonFields: people, Groups: groups, Condition: condition}, nil
}

// Expressions answers every condition of the shape, for compiling it when it is saved.
func (s WorkflowShape) Expressions() []FormExpression {
	var out []FormExpression
	for i, step := range s.Steps {
		at := "/steps/" + strconv.Itoa(i)
		if step.Who.Condition != "" {
			out = append(out, FormExpression{Path: at + "/who/condition", Text: step.Who.Condition})
		}
		for j, check := range step.Checks {
			out = append(out, FormExpression{Path: at + "/checks/" + strconv.Itoa(j) + "/holds", Text: check.Holds})
		}
	}
	return out
}

// CustomRoleKeys answers every custom role key a step's *who* names, with the step's path, for
// the use case that holds them to the roles the workspace defines.
func (s WorkflowShape) CustomRoleKeys() []FormExpression {
	var out []FormExpression
	for i, step := range s.Steps {
		for j, role := range step.Who.Roles {
			if identity.ValidCustomRoleKey(role) {
				out = append(out, FormExpression{Path: "/steps/" + strconv.Itoa(i) + "/who/roles/" + strconv.Itoa(j), Text: role})
			}
		}
	}
	return out
}

// GroupKeys answers every group a step's *who* names, where it names it: what the use case
// holds to the workspace's groups.
func (s WorkflowShape) GroupKeys() []FormExpression {
	var out []FormExpression
	for i, step := range s.Steps {
		for j, group := range step.Who.Groups {
			out = append(out, FormExpression{Path: "/steps/" + strconv.Itoa(i) + "/who/groups/" + strconv.Itoa(j), Text: group})
		}
	}
	return out
}

func workflowName(raw string, mayBeEmpty bool, form text.Normalizer) (string, error) {
	name, err := shared.NFC(strings.TrimSpace(raw), form)
	if err != nil {
		return "", err
	}
	switch {
	case name == "" && !mayBeEmpty:
		return "", workflowInvalid("/name", "workflows.name_empty", nil)
	case utf8.RuneCountInString(name) > MaxStructureNameLength:
		return "", workflowInvalid("/name", "workflows.name_too_long", map[string]string{"maximum": "120"})
	case hasControlCharacter(name):
		return "", workflowInvalid("/name", "workflows.name_malformed", nil)
	}
	return name, nil
}

func workflowInvalid(path, code string, params map[string]string) error {
	return shared.ErrValidation.WithDetail(code).WithParams(params).
		WithFields(shared.FieldError{Path: path, Code: code, Params: params})
}

// StepRequirementsMissing answers the form keys a step requires that the entry carries no value
// for, in the step's order.
func StepRequirementsMissing(step WorkflowStep, item WorkItem) []string {
	var missing []string
	for _, key := range step.Requires {
		if !hasValue(FormValue(item, key)) {
			missing = append(missing, key)
		}
	}
	return missing
}

// stepCheckMessages holds a check's words per language: a language tag (`de`, `pt-BR`) to text no
// longer than a form's help; an empty text drops its language.
func stepCheckMessages(raw map[string]string, form text.Normalizer, at string) (map[string]string, error) {
	if len(raw) > MaxStepCheckLanguages {
		return nil, workflowInvalid(at, "workflows.too_many_languages",
			map[string]string{"maximum": strconv.Itoa(MaxStepCheckLanguages)})
	}
	var out map[string]string
	for language, words := range raw {
		if !languageTagPattern.MatchString(language) {
			return nil, workflowInvalid(at+"/"+language, "workflows.language_malformed", map[string]string{"language": language})
		}
		message, err := formHelp(words, form, at+"/"+language)
		if err != nil {
			return nil, err
		}
		if message == "" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[language] = message
	}
	return out, nil
}

// StepRefused is the refusal of a step whose requirements or checks do not hold (UC-FLW-03 check
// 3): one field error per missing field or failing check, the check's own words as its message.
func StepRefused(step WorkflowStep, missing []string, failed []StepCheck) error {
	problems := make([]shared.FieldError, 0, len(missing)+len(failed))
	for _, key := range missing {
		problems = append(problems, shared.FieldError{
			Path: "/" + strings.ReplaceAll(key, ".", "/"), Code: "fields.value_required",
			Params: map[string]string{"key": key},
		})
	}
	for _, check := range failed {
		params := map[string]string{"check": check.Key}
		if check.Message != "" {
			params["message"] = check.Message
		}
		if check.MessageCode != "" {
			params["message_code"] = check.MessageCode
		}
		for language, words := range check.Messages {
			params["message."+language] = words
		}
		problems = append(problems, shared.FieldError{Path: "", Code: "workflows.check_failed", Params: params})
	}
	return shared.ErrValidation.WithDetail("workflows.step_refused").
		WithParams(map[string]string{"step": step.Key, "count": strconv.Itoa(len(problems))}).
		WithFields(problems...)
}

// StatusGroup is a count of entries in one collection, of one type, in one status, done or open:
// what a publish or a change of scheme moves.
type StatusGroup struct {
	CollectionID shared.ID
	EntryType    string
	Status       string
	Completed    bool
	Count        int
}

// StatusMove is what a publish or a change of scheme does to one group: its entries go from one
// status - empty for none - to another, empty for leaving every workflow.
type StatusMove struct {
	CollectionID shared.ID
	EntryType    string
	From         string
	Completed    bool
	To           string
	Count        int
}

// PlanStatusMoves answers how the entries of the groups move when the workflow in force for them
// changes from before to after - nil for none (UC-FLW-02 check 4, UC-FLW-05 check 2). A status the
// next shape keeps is kept; a removed one goes where mapping says, else where Entering puts it; a
// mapping that would change an entry's completion is refused, because a publish takes no step
// (UC-FLW-05 check 6). Groups that do not move are left out.
func PlanStatusMoves(groups []StatusGroup, before, after *WorkflowShape, mapping map[string]string) ([]StatusMove, error) {
	var moves []StatusMove
	for _, group := range groups {
		to := ""
		if after != nil {
			to = after.Entering(group.Status, group.Completed)
			if _, kept := after.Status(group.Status); !kept {
				if target, mapped := mapping[group.Status]; mapped && target != "" {
					status, found := after.Status(target)
					if !found {
						return nil, workflowInvalid("/mapping/"+group.Status, "workflows.status_unknown", map[string]string{"key": target})
					}
					if status.Category.Completes() != group.Completed {
						return nil, workflowInvalid("/mapping/"+group.Status, "workflows.mapping_changes_completion",
							map[string]string{"key": target})
					}
					to = target
				}
			}
		}
		if to == group.Status {
			continue
		}
		moves = append(moves, StatusMove{
			CollectionID: group.CollectionID, EntryType: group.EntryType, From: group.Status,
			Completed: group.Completed, To: to, Count: group.Count,
		})
	}
	return moves, nil
}

// RemovedStatuses answers the statuses of before that after no longer has and that entries are
// in, each with its count: what a publish asks a target for (UC-FLW-05 check 2).
func RemovedStatuses(before, after WorkflowShape, groups []StatusGroup) map[string]int {
	out := map[string]int{}
	for _, group := range groups {
		if _, known := before.Status(group.Status); !known {
			continue
		}
		if _, kept := after.Status(group.Status); !kept {
			out[group.Status] += group.Count
		}
	}
	return out
}

// WorkflowScheme names the workflow a container uses for one type, or for every type when the
// type is empty (the suite plan's model.md §2): a collection's own row wins over its hub's, and a
// type's row over the row for every type.
type WorkflowScheme struct {
	ContainerID shared.ID
	EntryType   string
	WorkflowKey string
}

// ResolveWorkflow answers the workflow key in force for an entry type in a collection under a hub,
// and false where none is: the collection does not run a workflow for that type.
func ResolveWorkflow(schemes []WorkflowScheme, collectionID, hubID shared.ID, entryType string) (string, bool) {
	for _, container := range []shared.ID{collectionID, hubID} {
		if container.IsZero() {
			continue
		}
		for _, wanted := range []string{entryType, ""} {
			for _, scheme := range schemes {
				if scheme.ContainerID == container && scheme.EntryType == wanted {
					return scheme.WorkflowKey, true
				}
			}
		}
	}
	return "", false
}

// EntryTypeKeyOf answers the key of an entry's type: its own, else its level's system type.
func EntryTypeKeyOf(item WorkItem) string {
	if item.EntryType != "" {
		return item.EntryType
	}
	return SystemEntryTypeKey(item.Type)
}
