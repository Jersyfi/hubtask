// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"

	service "github.com/Jersyfi/hubtask/core/application/repository/importer"
	domain "github.com/Jersyfi/hubtask/core/domain/model/importer"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/infrastructure/text"
)

// IssueTracker converts an issue tracker's search answer - `issues` with `names`, `schema` and
// each issue's `changelog` expanded - into the projects' collections with their configuration
// (D-10): each issue type an entry type, each project's statuses with their categories a workflow
// bound to its collection, its custom fields (with priority, components and versions) field
// definitions with their values, labels, comments, links, the change log as each entry's history
// and, from a ZIP that carries them under `attachments/<id>/`, the attachments.
//
// A ZIP's other `.json` members are further pages of the same answer. Sub-tasks are the work under
// their issue; an issue under an epic is linked to it as `implements`, because both are top-level
// work here. People are no accounts: their names travel in text fields, comments and the history.
//
// Identities derive from the projects' keys, so that a fresh export of the same projects into the
// same hub lands on the rows the first one made and creates nothing twice.
type IssueTracker struct{}

var _ service.Converter = IssueTracker{}

func (IssueTracker) Kind() domain.Kind { return domain.KindIssueTracker }

// The bounds of a ZIP: what it may unpack to, so that a small file cannot become a large memory.
const (
	maxUnpackedBytes = 1 << 30
	maxMemberBytes   = 256 << 20
)

type trackerAnswer struct {
	Issues []trackerIssue           `json:"issues"`
	Names  map[string]string        `json:"names"`
	Schema map[string]trackerSchema `json:"schema"`
	// workflows and boards are what a ZIP carries beside the answers (IssueTrackerPlanning.go).
	workflows []trackerWorkflow
	boards    []trackerBoard
}

type trackerSchema struct {
	Type   string `json:"type"`
	Items  string `json:"items"`
	Custom string `json:"custom"`
}

type trackerIssue struct {
	ID        string                     `json:"id"`
	Key       string                     `json:"key"`
	Fields    map[string]json.RawMessage `json:"fields"`
	Changelog struct {
		Histories []trackerHistory `json:"histories"`
	} `json:"changelog"`
}

type trackerHistory struct {
	ID      string        `json:"id"`
	Author  *trackerUser  `json:"author"`
	Created string        `json:"created"`
	Items   []trackerItem `json:"items"`
}

type trackerItem struct {
	Field      string `json:"field"`
	FromString string `json:"fromString"`
	ToString   string `json:"toString"`
}

type trackerUser struct {
	DisplayName string `json:"displayName"`
}

type trackerNamed struct {
	ID             string `json:"id"`
	Key            string `json:"key"`
	Name           string `json:"name"`
	Value          string `json:"value"`
	Subtask        bool   `json:"subtask"`
	HierarchyLevel int    `json:"hierarchyLevel"`
	StatusCategory struct {
		Key string `json:"key"`
	} `json:"statusCategory"`
}

type trackerComment struct {
	ID      string          `json:"id"`
	Author  *trackerUser    `json:"author"`
	Body    json.RawMessage `json:"body"`
	Created string          `json:"created"`
}

type trackerAttachment struct {
	ID       string `json:"id"`
	Filename string `json:"filename"`
	Created  string `json:"created"`
}

type trackerLink struct {
	Type struct {
		Name    string `json:"name"`
		Inward  string `json:"inward"`
		Outward string `json:"outward"`
	} `json:"type"`
	OutwardIssue *struct {
		Key string `json:"key"`
	} `json:"outwardIssue"`
}

type trackerParent struct {
	Key    string `json:"key"`
	Fields struct {
		IssueType trackerNamed `json:"issuetype"`
	} `json:"fields"`
}

// file is one attachment the ZIP carried.
type file struct {
	name    string
	content []byte
}

func (c IssueTracker) Convert(_ context.Context, source service.Source) (service.Result, error) {
	raw, err := io.ReadAll(source.Content)
	if err != nil {
		return service.Result{}, err
	}
	answer, files, err := readTracker(raw)
	if err != nil {
		return service.Result{}, err
	}
	if len(answer.Issues) == 0 {
		return service.Result{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
	}

	projects, order := map[string][]int{}, []string{}
	for i, issue := range answer.Issues {
		project := projectOf(issue)
		if _, seen := projects[project.Key]; !seen {
			order = append(order, project.Key)
		}
		projects[project.Key] = append(projects[project.Key], i)
	}
	sorted := append([]string(nil), order...)
	sort.Strings(sorted)
	b := newBuilder(source, "issue-tracker:"+strings.Join(sorted, ","))

	zone, err := time.LoadLocation(source.Zone)
	if err != nil || source.Zone == "" {
		zone = time.UTC
	}
	conv := trackerRun{b: b, answer: answer, files: files, zone: zone, unmapped: map[string]int{}}
	for _, key := range order {
		conv.project(key, projects[key])
	}
	conv.links()
	result := b.result(conv.refused)
	result.Unmapped = conv.unmapped
	return result, nil
}

// readTracker reads a JSON answer, an array of answers, or a ZIP of them with the attachments.
func readTracker(raw []byte) (trackerAnswer, map[string]file, error) {
	if bytes.HasPrefix(raw, []byte("PK\x03\x04")) {
		return readTrackerZip(raw)
	}
	answer, err := parseAnswers(raw)
	return answer, nil, err
}

func parseAnswers(raw []byte) (trackerAnswer, error) {
	trimmed := bytes.TrimSpace(raw)
	var pages []trackerAnswer
	if bytes.HasPrefix(trimmed, []byte("[")) {
		if json.Unmarshal(trimmed, &pages) != nil {
			return trackerAnswer{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
		}
	} else {
		var one trackerAnswer
		if json.Unmarshal(trimmed, &one) != nil || one.Issues == nil {
			return trackerAnswer{}, shared.ErrValidation.WithDetail(domain.CodeFileNotKind)
		}
		pages = []trackerAnswer{one}
	}
	merged := trackerAnswer{Names: map[string]string{}, Schema: map[string]trackerSchema{}}
	for _, page := range pages {
		merged.Issues = append(merged.Issues, page.Issues...)
		for k, v := range page.Names {
			merged.Names[k] = v
		}
		for k, v := range page.Schema {
			merged.Schema[k] = v
		}
	}
	return merged, nil
}

func readTrackerZip(raw []byte) (trackerAnswer, map[string]file, error) {
	unreadable := shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		return trackerAnswer{}, nil, unreadable
	}
	merged := trackerAnswer{Names: map[string]string{}, Schema: map[string]trackerSchema{}}
	files := map[string]file{}
	var total int64
	documents := 0
	members := append([]*zip.File(nil), reader.File...)
	sort.Slice(members, func(i, j int) bool { return members[i].Name < members[j].Name })
	for _, member := range members {
		if member.FileInfo().IsDir() {
			continue
		}
		content, err := readMember(member, &total)
		if err != nil {
			return trackerAnswer{}, nil, err
		}
		name := path.Clean(strings.ReplaceAll(member.Name, "\\", "/"))
		parts := strings.Split(name, "/")
		if at := indexOf(parts, "attachments"); at >= 0 && at+1 < len(parts) {
			id := parts[at+1]
			files[id] = file{name: parts[len(parts)-1], content: content}
			continue
		}
		if strings.EqualFold(path.Ext(name), ".json") {
			if merged.readPlanning(content) {
				continue
			}
			page, err := parseAnswers(content)
			if err != nil {
				continue
			}
			documents++
			merged.Issues = append(merged.Issues, page.Issues...)
			for k, v := range page.Names {
				merged.Names[k] = v
			}
			for k, v := range page.Schema {
				merged.Schema[k] = v
			}
		}
	}
	if documents == 0 {
		return trackerAnswer{}, nil, unreadable
	}
	return merged, files, nil
}

// readMember unpacks one member within the bounds: what it declares is not trusted, what it
// yields is counted.
func readMember(member *zip.File, total *int64) ([]byte, error) {
	tooLarge := shared.ErrValidation.WithDetail(domain.CodeArchiveUnreadable)
	opened, err := member.Open()
	if err != nil {
		return nil, tooLarge
	}
	defer func() { _ = opened.Close() }()
	content, err := io.ReadAll(io.LimitReader(opened, maxMemberBytes+1))
	if err != nil || int64(len(content)) > maxMemberBytes {
		return nil, tooLarge
	}
	*total += int64(len(content))
	if *total > maxUnpackedBytes {
		return nil, tooLarge
	}
	return content, nil
}

func indexOf(parts []string, want string) int {
	for i, part := range parts {
		if strings.EqualFold(part, want) {
			return i
		}
	}
	return -1
}

func projectOf(issue trackerIssue) trackerNamed {
	var project trackerNamed
	_ = json.Unmarshal(issue.Fields["project"], &project)
	if project.Key == "" {
		if dash := strings.LastIndex(issue.Key, "-"); dash > 0 {
			project.Key = issue.Key[:dash]
		}
	}
	if project.Name == "" {
		project.Name = project.Key
	}
	return project
}

// trackerRun is one conversion's state across the projects.
type trackerRun struct {
	b        *builder
	answer   trackerAnswer
	files    map[string]file
	zone     *time.Location
	refused  []refusal
	unmapped map[string]int
	// entries is each issue's entry identity by its key, for links between projects.
	entries map[string]shared.ID
	// epics is each issue's epic by key, linked once every project is in.
	epics map[string]string
	// position numbers the entry and link types in the order they were met.
	position int
	// fieldDefs are the fields of the project being converted.
	fieldDefs []fieldDef
	// pending are the issue links, drawn once every project is in.
	pending []pendingLink
}

func (r *trackerRun) project(key string, indexes []int) {
	if r.entries == nil {
		r.entries, r.epics = map[string]shared.ID{}, map[string]string{}
	}
	b := r.b
	first := r.answer.Issues[indexes[0]]
	project := projectOf(first)
	collection := b.collection("project:"+key, project.Name, "")

	// The workflow: the statuses the issues are in, in the order of their categories. Into a
	// collection that runs its own, the statuses are not carried - the completion is.
	shape, statusKeys := r.workflowOf(key, indexes)
	workflowKey := keyOf("imported_" + key)
	if b.source.Into == nil && workflowKey != "" && len(shape.Statuses) > 0 {
		b.workflow(workflowKey, project.Name, shape, collection)
	} else {
		statusKeys = nil
	}

	_, refs := r.fieldsOf(indexes, collection)

	// Top-level issues first, so that a sub-task finds its parent.
	ordered := append([]int(nil), indexes...)
	sort.SliceStable(ordered, func(i, j int) bool {
		return !r.issueType(ordered[i]).Subtask && r.issueType(ordered[j]).Subtask
	})
	for _, index := range ordered {
		r.issue(index, collection, statusKeys, refs)
	}
	r.sprints(key, indexes, collection)
	if statusKeys != nil {
		r.boardViews(key, collection)
	}
}

func (r *trackerRun) issueType(index int) trackerNamed {
	var kind trackerNamed
	_ = json.Unmarshal(r.answer.Issues[index].Fields["issuetype"], &kind)
	return kind
}

func (r *trackerRun) status(index int) trackerNamed {
	var status trackerNamed
	_ = json.Unmarshal(r.answer.Issues[index].Fields["status"], &status)
	return status
}

func categoryOf(key string) work.StatusCategory {
	switch key {
	case "indeterminate":
		return work.CategoryInProgress
	case "done":
		return work.CategoryDone
	}
	return work.CategoryTodo
}

// workflowOf answers the project's workflow - one status per status the issues are in, in the
// order of its board's columns or of their categories, its steps the tracker's transitions or,
// without them, each status reachable from any other - and the status key per source status name.
func (r *trackerRun) workflowOf(project string, indexes []int) (work.WorkflowShape, map[string]string) {
	keys := map[string]string{}
	idKeys := map[string]string{}
	var statuses []work.WorkflowStatus
	taken := map[string]bool{}
	for _, index := range indexes {
		status := r.status(index)
		name := strings.TrimSpace(status.Name)
		if known := keys[strings.ToLower(name)]; known != "" && status.ID != "" {
			idKeys[status.ID] = known
		}
		if name == "" || keys[strings.ToLower(name)] != "" {
			continue
		}
		key := keyOf(name)
		if key == "" || taken[key] {
			key = keyOf("status_" + status.ID + "_" + strconv.Itoa(len(statuses)))
		}
		taken[key] = true
		keys[strings.ToLower(name)] = key
		if status.ID != "" {
			idKeys[status.ID] = key
		}
		category := categoryOf(status.StatusCategory.Key)
		token := map[work.StatusCategory]shared.LabelToken{
			work.CategoryTodo: shared.LabelTokenSlate, work.CategoryInProgress: shared.LabelTokenBlue, work.CategoryDone: shared.LabelTokenGreen,
		}[category]
		statuses = append(statuses, work.WorkflowStatus{Key: key, Name: clip(name, 120), Category: category, ColorToken: string(token)})
	}
	if len(statuses) == 0 {
		return work.WorkflowShape{}, nil
	}
	// A workflow answer names the statuses by identifier as the issues do; its own names fill in a
	// status the issues name by another identifier.
	for _, workflow := range r.answer.workflows {
		for _, status := range workflow.Statuses {
			if key := keys[strings.ToLower(strings.TrimSpace(status.Name))]; key != "" && idKeys[status.ID] == "" {
				idKeys[status.ID] = key
			}
		}
	}
	if !orderByBoard(statuses, r.boardsOf(project), idKeys) {
		rank := map[work.StatusCategory]int{work.CategoryBacklog: 0, work.CategoryTodo: 1, work.CategoryInProgress: 2, work.CategoryDone: 3}
		sort.SliceStable(statuses, func(i, j int) bool { return rank[statuses[i].Category] < rank[statuses[j].Category] })
	}
	if len(statuses) > work.MaxWorkflowStatuses {
		r.unmapped["statuses"] += len(statuses) - work.MaxWorkflowStatuses
		statuses = statuses[:work.MaxWorkflowStatuses]
	}
	shape := work.WorkflowShape{Statuses: statuses, Initial: statuses[0].Key}
	for _, status := range statuses {
		shape.Steps = append(shape.Steps, work.WorkflowStep{Key: clip("to_"+status.Key, 50), Name: status.Name, To: status.Key})
	}
	valid, err := work.NewWorkflowShape(shape, false, text.Forms{})
	if steps, initial, ok := r.stepsOf(statuses, idKeys); ok && err == nil {
		traced := shape
		traced.Steps = steps
		if initial != "" {
			traced.Initial = initial
		}
		// A workflow the transitions leave unsound - a status nothing leads to - keeps every step.
		if checked, failed := work.NewWorkflowShape(traced, false, text.Forms{}); failed == nil {
			valid = checked
		} else {
			r.unmapped["transitions"] += len(steps)
		}
	}
	if err != nil {
		r.unmapped["workflows"]++
		return work.WorkflowShape{}, nil
	}
	known := map[string]bool{}
	for _, status := range valid.Statuses {
		known[status.Key] = true
	}
	for name, key := range keys {
		if !known[key] {
			delete(keys, name)
		}
	}
	return valid, keys
}

// The built-in fields carried as custom fields of the collection, by the key they get.
const (
	fieldIssueKey    = "issue_key"
	fieldPriority    = "priority"
	fieldComponents  = "components"
	fieldFixVersions = "fix_versions"
	fieldAssignee    = "source_assignee"
	fieldReporter    = "source_reporter"
)

// fieldDef is one field the project's issues carry: how its value is read out of an issue.
type fieldDef struct {
	Field
	source string
	read   func(raw json.RawMessage) any
}

// fieldsOf defines the project's fields - the custom fields that hold a value in one of its
// issues, and the built-in ones the product has no own place for - and answers them with their
// identities by key.
func (r *trackerRun) fieldsOf(indexes []int, collection shared.ID) ([]fieldDef, map[string]shared.ID) {
	defs := []fieldDef{
		{Field: Field{Key: fieldIssueKey, Kind: work.CustomFieldText}, source: "key"},
		{Field: Field{Key: fieldPriority, Kind: work.CustomFieldSelect}, source: "priority", read: readName},
		{Field: Field{Key: fieldComponents, Kind: work.CustomFieldMultiSelect}, source: "components", read: readNames},
		{Field: Field{Key: fieldFixVersions, Kind: work.CustomFieldMultiSelect}, source: "fixVersions", read: readNames},
		{Field: Field{Key: fieldAssignee, Kind: work.CustomFieldText}, source: "assignee", read: readUser},
		{Field: Field{Key: fieldReporter, Kind: work.CustomFieldText}, source: "reporter", read: readUser},
	}
	taken := map[string]bool{}
	for _, def := range defs {
		taken[def.Key] = true
	}
	var custom []string
	for id := range r.answer.Names {
		if strings.HasPrefix(id, "customfield_") {
			custom = append(custom, id)
		}
	}
	sort.Strings(custom)
	for _, id := range custom {
		if r.sprintField(id) {
			continue
		}
		kind, read, ok := kindOf(r.answer.Schema[id])
		if !ok {
			if r.holdsValue(indexes, id) {
				r.unmapped["fields"]++
			}
			continue
		}
		if !r.holdsValue(indexes, id) {
			continue
		}
		key := keyOf(r.answer.Names[id])
		if key == "" || taken[key] {
			key = keyOf(id)
		}
		taken[key] = true
		defs = append(defs, fieldDef{Field: Field{Key: key, Kind: kind}, source: id, read: read})
	}

	// The options of a pick are what the issues picked, in the order they were met.
	kept := defs[:0]
	refs := map[string]shared.ID{}
	for _, def := range defs {
		if def.Kind.TakesOptions() {
			seen := map[string]bool{}
			for _, index := range indexes {
				for _, option := range optionsOf(def.value(r.answer.Issues[index])) {
					if !seen[option] && len(def.Options) < work.MaxCustomFieldOptions {
						seen[option] = true
						def.Options = append(def.Options, option)
					}
				}
			}
			if len(def.Options) == 0 {
				continue
			}
		}
		refs[def.Key] = r.b.field(collection, def.Field)
		kept = append(kept, def)
	}
	r.fieldDefs = kept
	return kept, refs
}

func (r *trackerRun) holdsValue(indexes []int, id string) bool {
	for _, index := range indexes {
		raw := r.answer.Issues[index].Fields[id]
		if len(raw) > 0 && string(raw) != "null" && string(raw) != "[]" && string(raw) != `""` {
			return true
		}
	}
	return false
}

func (d fieldDef) value(issue trackerIssue) any {
	if d.source == "key" {
		return issue.Key
	}
	raw := issue.Fields[d.source]
	if len(raw) == 0 || string(raw) == "null" || d.read == nil {
		return nil
	}
	return d.read(raw)
}

func optionsOf(value any) []string {
	switch v := value.(type) {
	case string:
		return []string{v}
	case []string:
		return v
	}
	return nil
}

// kindOf maps a custom field's schema onto a kind and a reader, or answers false for one the
// product has no kind for (a sprint, a cascade, an object of an add-on).
func kindOf(schema trackerSchema) (work.CustomFieldKind, func(json.RawMessage) any, bool) {
	custom := schema.Custom[strings.LastIndex(schema.Custom, ":")+1:]
	switch schema.Type {
	case "string":
		if custom == "url" {
			return work.CustomFieldURL, readString, true
		}
		return work.CustomFieldText, readString, true
	case "number":
		return work.CustomFieldNumber, readNumber, true
	case "date", "datetime":
		return work.CustomFieldDate, readDay, true
	case "option":
		return work.CustomFieldSelect, readName, true
	case "user":
		return work.CustomFieldText, readUser, true
	case "array":
		switch schema.Items {
		case "option", "string", "version", "component":
			return work.CustomFieldMultiSelect, readNames, true
		}
	}
	return "", nil, false
}

func readString(raw json.RawMessage) any {
	text := richText(raw)
	if text == "" {
		return nil
	}
	return clip(text, 2000)
}

func readNumber(raw json.RawMessage) any {
	var n float64
	if json.Unmarshal(raw, &n) != nil {
		return nil
	}
	return n
}

func readDay(raw json.RawMessage) any {
	var day string
	if json.Unmarshal(raw, &day) != nil || len(day) < 10 {
		return nil
	}
	if _, err := time.Parse("2006-01-02", day[:10]); err != nil {
		return nil
	}
	return day[:10]
}

func readName(raw json.RawMessage) any {
	var named trackerNamed
	if json.Unmarshal(raw, &named) != nil {
		return nil
	}
	for _, label := range []string{named.Value, named.Name} {
		if label = strings.TrimSpace(label); label != "" {
			return clip(label, 120)
		}
	}
	return nil
}

func readNames(raw json.RawMessage) any {
	var list []json.RawMessage
	if json.Unmarshal(raw, &list) != nil {
		return nil
	}
	var names []string
	seen := map[string]bool{}
	for _, item := range list {
		var plain string
		name := ""
		if json.Unmarshal(item, &plain) == nil {
			name = clip(strings.TrimSpace(plain), 120)
		} else if named, ok := readName(item).(string); ok {
			name = named
		}
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		return nil
	}
	return names
}

func readUser(raw json.RawMessage) any {
	var user trackerUser
	if json.Unmarshal(raw, &user) != nil || strings.TrimSpace(user.DisplayName) == "" {
		return nil
	}
	return clip(strings.TrimSpace(user.DisplayName), 200)
}

// issue adds one issue: its entry, type, status, values, labels, comments, history and files.
func (r *trackerRun) issue(index int, collection shared.ID, statusKeys map[string]string, refs map[string]shared.ID) {
	b := r.b
	issue := r.answer.Issues[index]
	row := index + 1
	var summary string
	_ = json.Unmarshal(issue.Fields["summary"], &summary)
	kind := r.issueType(index)

	item := Item{Key: "issue:" + issue.Key, Collection: collection, Title: summary, Notes: richText(issue.Fields["description"]), Row: row}
	if created, ok := r.instant(issue.Fields["created"]); ok {
		item.CreatedAt = &created
	}
	var parent trackerParent
	_ = json.Unmarshal(issue.Fields["parent"], &parent)
	if kind.Subtask {
		if _, known := b.entries["issue:"+parent.Key]; parent.Key == "" || !known {
			r.refused = append(r.refused, refusal{row: row, code: domain.CodeRowParentUnknown})
			return
		}
		item.ParentKey, item.Type = "issue:"+parent.Key, string(work.ItemWorkPackage)
	} else if parent.Key != "" {
		r.epics[issue.Key] = parent.Key
	}
	if day := readDay(issue.Fields["duedate"]); day != nil {
		at, err := time.ParseInLocation("2006-01-02", day.(string), r.zone)
		if err == nil {
			at = at.UTC()
			item.DueAt, item.DueDateOnly, item.DueZone = &at, true, r.zone.String()
		}
	}
	status := r.status(index)
	done := categoryOf(status.StatusCategory.Key) == work.CategoryDone
	if done {
		if resolved, ok := r.instant(issue.Fields["resolutiondate"]); ok {
			item.CompletedAt = &resolved
		}
	}
	id, ok := b.item(item)
	if !ok {
		r.refused = append(r.refused, refusal{row: row, code: domain.CodeRowTitleMissing})
		return
	}
	r.entries[issue.Key] = id

	// The type: the issue type's own, at the level the entry sits.
	level := work.ItemTask
	icon := "square-check"
	if kind.Subtask {
		level, icon = work.ItemWorkPackage, "check"
	} else if kind.HierarchyLevel > 0 {
		icon = "zap"
	} else if strings.EqualFold(kind.Name, "bug") {
		icon = "bug"
	}
	typeKey := keyOf(kind.Name)
	if typeKey != "" {
		if typeKey == work.SystemEntryTypeKey(work.ItemTask) || typeKey == work.SystemEntryTypeKey(work.ItemWorkPackage) ||
			typeKey == work.SystemEntryTypeKey(work.ItemActivity) {
			typeKey = work.SystemEntryTypeKey(level)
		}
		r.position++
		b.entryType(typeKey, kind.Name, icon, level, b.source.Into == nil, r.position)
	}
	values := map[string]any{}
	for _, def := range r.fieldDefs {
		if value := def.value(issue); value != nil {
			values[def.Key] = value
		}
	}
	b.classify(id, typeKey, statusKeys[strings.ToLower(strings.TrimSpace(status.Name))], done, values, refs)

	var labels []string
	_ = json.Unmarshal(issue.Fields["labels"], &labels)
	for _, name := range labels {
		b.link(id, b.label("label:"+strings.ToLower(name), collection, name, tokenFor(name)))
	}

	var comments struct {
		Comments []trackerComment `json:"comments"`
	}
	_ = json.Unmarshal(issue.Fields["comment"], &comments)
	for i, comment := range comments.Comments {
		at, ok := r.instant(jsonString(comment.Created))
		var when *time.Time
		if ok {
			when = &at
		}
		author := ""
		if comment.Author != nil {
			author = comment.Author.DisplayName
		}
		b.comment("comment:"+issue.Key+":"+comment.ID+":"+strconv.Itoa(i), id, author, richText(comment.Body), when)
	}

	for _, history := range issue.Changelog.Histories {
		at, ok := r.instant(jsonString(history.Created))
		if !ok {
			continue
		}
		changes := make([]RecordedChange, 0, len(history.Items))
		for _, change := range history.Items {
			changes = append(changes, RecordedChange{Field: change.Field, From: change.FromString, To: change.ToString})
		}
		author := ""
		if history.Author != nil {
			author = history.Author.DisplayName
		}
		b.history("history:"+issue.Key+":"+history.ID, id, collection, author, at, changes)
	}

	var attachments []trackerAttachment
	_ = json.Unmarshal(issue.Fields["attachment"], &attachments)
	for _, attachment := range attachments {
		stored, ok := r.files[attachment.ID]
		if !ok {
			r.unmapped["attachments"]++
			continue
		}
		at := b.source.Now
		if created, ok := r.instant(jsonString(attachment.Created)); ok {
			at = created
		}
		name := attachment.Filename
		if strings.TrimSpace(name) == "" {
			name = stored.name
		}
		b.attachment(id, name, stored.content, at)
	}

	var links []trackerLink
	_ = json.Unmarshal(issue.Fields["issuelinks"], &links)
	for _, link := range links {
		if link.OutwardIssue == nil {
			continue
		}
		r.pending = append(r.pending, pendingLink{from: issue.Key, to: link.OutwardIssue.Key, name: link.Type.Name, outward: link.Type.Outward, inward: link.Type.Inward})
	}
}

type pendingLink struct {
	from, to, name, outward, inward string
}

// links draws the issue links and the epics once every project is in: either end may be in
// another project of the file. A link to an issue the file does not carry is counted.
func (r *trackerRun) links() {
	for _, link := range r.pending {
		source, ok := r.entries[link.from]
		target, okTarget := r.entries[link.to]
		if !ok || !okTarget {
			r.unmapped["links"]++
			continue
		}
		r.b.entryLink(r.linkTypeOf(link), source, target)
	}
	children := make([]string, 0, len(r.epics))
	for child := range r.epics {
		children = append(children, child)
	}
	sort.Strings(children)
	for _, child := range children {
		source, ok := r.entries[child]
		target, okTarget := r.entries[r.epics[child]]
		if !ok || !okTarget {
			r.unmapped["links"]++
			continue
		}
		r.b.entryLink(work.LinkTypeImplements, source, target)
	}
}

func (r *trackerRun) linkTypeOf(link pendingLink) string {
	switch strings.ToLower(strings.TrimSpace(link.name)) {
	case "blocks":
		return work.LinkTypeBlocks
	case "duplicate", "duplicates":
		return work.LinkTypeDuplicates
	}
	key := keyOf(link.name)
	if key == "" {
		key = "relates"
	}
	for _, shipped := range []string{work.LinkTypeBlocks, work.LinkTypeDependsOn, work.LinkTypeDuplicates, work.LinkTypeRunsOn, work.LinkTypeAffects, work.LinkTypeImplements} {
		if key == shipped {
			return key
		}
	}
	name := link.outward
	if strings.TrimSpace(name) == "" {
		name = link.name
	}
	r.position++
	r.b.linkType(key, name, link.inward, r.position)
	return key
}

func jsonString(value string) json.RawMessage {
	raw, _ := json.Marshal(value)
	return raw
}

// instant reads the source's moments: with milliseconds and a numeric zone, or RFC 3339.
func (r *trackerRun) instant(raw json.RawMessage) (time.Time, bool) {
	var value string
	if json.Unmarshal(raw, &value) != nil || value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{"2006-01-02T15:04:05.000-0700", "2006-01-02T15:04:05-0700", time.RFC3339Nano, time.RFC3339} {
		if at, err := time.Parse(layout, value); err == nil {
			return at.UTC(), true
		}
	}
	return time.Time{}, false
}
