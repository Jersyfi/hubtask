// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"errors"
	"strconv"
	"time"

	metarepo "github.com/Jersyfi/hubtask/core/application/repository/meta"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	CreateTemplateName = "CreateTemplate"
	ListTemplatesName  = "ListTemplates"
	GetTemplateName    = "GetTemplate"

	templateTarget = "template"
	templatesWrite = "templates:write"
	templatesRead  = "templates:read"

	// The audit codes. Stable: an auditor filters on them and a SIEM rule matches on them
	// (audit.md §2).
	TemplateCreatedAction audit.Action = "template.created"
	// TemplatesReadAction is the audit code of an attempted read, declared for the reason
	// CommentsReadAction is: an ordinary read writes no entry, a refused one does.
	TemplatesReadAction audit.Action = "template.read"
)

// TemplateWriter is what every use case that writes a template shares: the same scope resolution,
// the same permission question, and the same records.
//
// The permission is STRUCTURE rather than WRITE_ITEMS, and that is the decision the backlog left
// open ("who may define at which scope is the application layer's question"): a template is
// vocabulary for a whole workspace or a whole collection, like a label or a board column, and
// defining one is shaping how everybody there works. Which scope somebody may shape follows the
// role matrix at that scope - so a workspace-wide template needs STRUCTURE at the tenant, which
// is an administrator, and a collection's needs it there.
type TemplateWriter struct {
	Templates  repository.Templates
	Containers repository.Containers
	Profiles   metarepo.CapabilityProfiles
	Authorizer Authorizer
	Changes    changelog.ChangeLog
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	HLC        clock.HLCSource
}

// CreateTemplate defines a template.
type CreateTemplate struct {
	Writer TemplateWriter
}

// CreateTemplateCommand is the input, typed.
type CreateTemplateCommand struct {
	Spec domain.TemplateSpec
}

// Execute writes the template and returns it.
func (h CreateTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CreateTemplateCommand,
) (domain.Template, error) {
	w := h.Writer

	scope := domain.TemplateScope(cmd.Spec.Scope)
	path, err := w.scopePath(ctx, actor, scope, cmd.Spec.ScopeID)
	if err != nil {
		return domain.Template{}, err
	}

	// Before the transaction, deliberately: a refusal writes an audit entry, and an entry written
	// inside this transaction would be rolled back together with the refusal (audit.md §7).
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionStructure,
		Path:       path,
		Action:     TemplateCreatedAction,
		TokenScope: templatesWrite,
		TargetType: templateTarget,
		TargetID:   cmd.Spec.ScopeID,
	}); err != nil {
		return domain.Template{}, err
	}

	var written domain.Template
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		now := w.Clock.Now()

		template, err := domain.NewTemplate(domain.NewTemplateInput{
			ID:       w.IDs.NewID(),
			TenantID: actor.TenantID,
			Spec:     cmd.Spec,
			Now:      now,
		})
		if err != nil {
			return err
		}
		if err := w.ensureTreeIsPossible(ctx, template); err != nil {
			return err
		}
		if err := w.Templates.Insert(ctx, template); err != nil {
			return err
		}

		if err := w.recordWhole(ctx, actor, template); err != nil {
			return err
		}
		if err := w.recordAudit(
			ctx, actor, template, TemplateCreatedAction, wholeTemplateAudit(template), now,
		); err != nil {
			return err
		}
		written = template
		return nil
	})
	if err != nil {
		return domain.Template{}, err
	}
	return written, nil
}

// ensureTreeIsPossible asks the hierarchy what the document cannot answer: whether each type may
// sit where the tree puts it, in this installation's profiles (I-W1).
//
// Asked at the definition rather than at the instantiation, which is the whole point of the check:
// a template that cannot produce anything is one nobody can fix from a failed instantiation, and
// the refusal names the node rather than the tree.
func (w TemplateWriter) ensureTreeIsPossible(
	ctx context.Context, template domain.Template,
) error {
	rows, err := w.Profiles.List(ctx)
	if err != nil {
		return err
	}
	hierarchy, err := service.NewHierarchy(rows, rows)
	if err != nil {
		return err
	}

	if !hierarchy.IsRoot(template.RootType) {
		// A template's root is what lands directly in a collection. A type that only sits under
		// another one cannot be one, whatever the tree under it says.
		return shared.ErrValidation.
			WithDetail("templates.root_not_permitted").
			WithParams(map[string]string{"item_type": string(template.RootType)}).
			WithFields(shared.FieldError{
				Path: "/root_type", Code: "templates.root_not_permitted",
				Params: map[string]string{"item_type": string(template.RootType)},
			})
	}
	return checkTemplateChildren(hierarchy, template.Root, "/nodes/0")
}

// checkTemplateChildren walks the tree, asking the profiles about each parent and child pair and
// carrying the path so a refusal points at the node that caused it.
func checkTemplateChildren(
	hierarchy service.Hierarchy, node domain.TemplateNode, path string,
) error {
	profile, err := hierarchy.Profile(node.Type)
	if err != nil {
		return err
	}

	for index, child := range node.Children {
		childPath := path + "/children/" + strconv.Itoa(index)
		if !profile.AllowsChild(child.Type) {
			return shared.ErrValidation.
				WithDetail("templates.node_not_permitted").
				WithParams(map[string]string{
					"item_type": string(child.Type), "parent_type": string(node.Type),
				}).
				WithFields(shared.FieldError{
					Path: childPath + "/type", Code: "templates.node_not_permitted",
					Params: map[string]string{
						"item_type": string(child.Type), "parent_type": string(node.Type),
					},
				})
		}
		if err := checkTemplateChildren(hierarchy, child, childPath); err != nil {
			return err
		}
	}
	return nil
}

// scopePath answers the path a template's scope is judged against: the tenant for a workspace-wide
// one, the container's own path for the other two.
func (w TemplateWriter) scopePath(
	ctx context.Context, actor appshared.ActorContext,
	scope domain.TemplateScope, scopeID shared.ID,
) ([]identity.Scope, error) {
	if !scope.Valid() {
		return nil, shared.ErrValidation.
			WithDetail("templates.scope_unknown").
			WithParams(map[string]string{"value": string(scope)}).
			WithFields(shared.FieldError{Path: "/scope_type", Code: "templates.scope_unknown"})
	}
	if !scope.NeedsContainer() {
		return []identity.Scope{identity.TenantScope()}, nil
	}
	if scopeID.IsZero() {
		return nil, shared.ErrValidation.
			WithDetail("templates.scope_id_required").
			WithFields(shared.FieldError{Path: "/scope_id", Code: "templates.scope_id_required"})
	}

	var container domain.Container
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		container, err = w.Containers.Find(ctx, scopeID)
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return nil, shared.ErrNotFound.
				WithDetail("containers.not_found").
				WithParams(map[string]string{"container_id": scopeID.String()})
		}
		return nil, err
	}

	wanted := domain.ContainerHub
	if scope == domain.TemplateScopeCollection {
		wanted = domain.ContainerCollection
	}
	if container.Type != wanted {
		// A hub-scoped template on a collection would be visible to the wrong set of people, and
		// the scope is what decides that set.
		return nil, shared.ErrValidation.
			WithDetail("templates.scope_container_mismatched").
			WithParams(map[string]string{"scope_type": scope.String()}).
			WithFields(shared.FieldError{
				Path: "/scope_id", Code: "templates.scope_container_mismatched",
			})
	}
	return containerPath(container), nil
}

// findTemplate reads a template, and answers "there is none" for one that is not there or belongs
// to another tenant - the same answer, deliberately (multi-tenancy.md §2).
func (w TemplateWriter) findTemplate(
	ctx context.Context, id shared.ID,
) (domain.Template, error) {
	template, err := w.Templates.Find(ctx, id)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return domain.Template{}, templateNotFound(id)
		}
		return domain.Template{}, err
	}
	if template.DeletedAt != nil {
		// A deleted template is read back by its identifier - the row is there - but it is not a
		// template anybody can use, and answering it as one would offer a tree that cannot be
		// stamped out.
		return domain.Template{}, templateNotFound(id)
	}
	return template, nil
}

func templateNotFound(id shared.ID) error {
	return shared.ErrNotFound.
		WithDetail("templates.not_found").
		WithParams(map[string]string{"template_id": id.String()})
}

// recordWhole writes what an offline client has to be told about a template that arrived or
// changed: one entry carrying the whole document, because a tree is one shape.
func (w TemplateWriter) recordWhole(
	ctx context.Context, actor appshared.ActorContext, template domain.Template,
) error {
	return w.Changes.Record(ctx, changelog.Change{
		TenantID: template.TenantID,
		Entity:   templateTarget,
		EntityID: template.ID,
		Op:       changelog.Upsert,
		// The container the template belongs to, so that a device subscribed to it learns about
		// the template the same way it learns about everything else there. A workspace-wide one
		// carries none, which is what an entry with no container means: everybody in the tenant.
		ContainerID: template.ScopeID,
		ActorID:     actor.AccountID,
		HLC:         w.HLC.Next(),
		Payload:     templateOutput(template),
	})
}

// recordAudit writes the evidence, inside the same transaction as the change (test AT-5).
//
// The names travel and the tree does not: a template's node titles are user content, and the trail
// records that the shape changed rather than what somebody called every step (rule 10).
func (w TemplateWriter) recordAudit(
	ctx context.Context, actor appshared.ActorContext, template domain.Template,
	action audit.Action, changes []audit.Change, now time.Time,
) error {
	return w.Audit.Append(ctx, audit.Entry{
		TenantID:   template.TenantID,
		OccurredAt: now,
		Action:     action,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: templateTarget,
		TargetID:   template.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes:    audit.Changes(changes...),
	})
}

// wholeTemplateAudit records what a definition and a deletion are about: the template as a whole,
// with its shape as a count rather than as its content.
func wholeTemplateAudit(template domain.Template) []audit.Change {
	return []audit.Change{
		{Field: "name", Classification: audit.Sensitive, To: template.Name},
		{Field: "scope_type", Classification: audit.Open, To: template.Scope.String()},
		{Field: "scope_id", Classification: audit.Open, To: template.ScopeID.String()},
		{Field: "root_type", Classification: audit.Open, To: string(template.RootType)},
		{Field: "nodes", Classification: audit.Open, To: strconv.Itoa(template.NodeCount())},
	}
}

// templateOutput is the shape every channel returns: the field names of the contract
// (api/openapi.yaml, schema Template).
func templateOutput(template domain.Template) usecase.Output {
	return usecase.Output{
		"id":          template.ID.String(),
		"scope_type":  template.Scope.String(),
		"scope_id":    idOrNil(template.ScopeID),
		"name":        template.Name,
		"description": textOrNil(template.Description),
		"root_type":   string(template.RootType),
		"nodes":       []usecase.Output{templateNodeOutput(template.Root)},
		"created_at":  template.CreatedAt,
		"updated_at":  timeOrNil(template.UpdatedAt),
		"version":     template.Version,
	}
}

// templateNodeOutput is one node and everything under it, in the shape the contract declares.
func templateNodeOutput(node domain.TemplateNode) usecase.Output {
	out := usecase.Output{
		"type":          string(node.Type),
		"title":         node.Title,
		"notes":         textOrNil(node.Notes),
		"due_offset":    nil,
		"due_date_only": node.DueDateOnly,
		"assignee_id":   idOrNil(node.AssigneeID),
	}
	if node.DueOffset != nil {
		out["due_offset"] = domain.SpellTemplateOffset(*node.DueOffset)
	}

	children := make([]usecase.Output, 0, len(node.Children))
	for _, child := range node.Children {
		children = append(children, templateNodeOutput(child))
	}
	out["children"] = children
	return out
}

// textOrNil spells an optional text the way the contract carries it: the value, or an explicit
// null rather than an empty string a client would have to interpret.
func textOrNil(text string) any {
	if text == "" {
		return nil
	}
	return text
}

// Descriptor is the catalogue entry. Registering it is what makes the use case reachable through
// REST, MCP and automation at once (arc42 §4).
func (h CreateTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateTemplateName,
		Summary: "Defines a template: a name, a scope, and the tree it stamps out. The tree is " +
			"validated here rather than at instantiation - the types have to be permitted under " +
			"one another by this installation's profiles, and the whole tree has to stay inside " +
			"the node cap in /meta/capabilities. A node's due date is a duration from the anchor " +
			"of an instantiation, such as P3D; years and months are refused, because they are " +
			"calendar arithmetic rather than a length of time. Defining a template needs " +
			"STRUCTURE at its scope, which for a workspace-wide one is an administrator.",
		SideEffects: "Writes the template, records the change for offline clients and writes an " +
			"audit entry.",
		TokenScope: templatesWrite,
		Input: []usecase.Field{
			{
				Name: "scope_type", Kind: usecase.KindString, Required: true,
				Description: "TENANT, HUB or COLLECTION.",
			},
			{
				Name: "scope_id", Kind: usecase.KindID,
				Description: "The container the template is defined in. Omitted for a " +
					"workspace-wide one.",
			},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What it is called."},
			{Name: "description", Kind: usecase.KindString, Description: "What it is for."},
			{
				Name: "root_type", Kind: usecase.KindString, Required: true,
				Description: "What the template produces: TASK, WORK_PACKAGE or ACTIVITY.",
			},
			{
				Name: "nodes", Kind: usecase.KindList, Required: true,
				Description: "The tree, as one root node carrying its children.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplateCreatedAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CreateTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	scopeID, err := in.ID("scope_id")
	if err != nil {
		return nil, err
	}
	root, err := templateTreeOf(in)
	if err != nil {
		return nil, err
	}

	template, err := h.Execute(ctx, actor, CreateTemplateCommand{
		Spec: domain.TemplateSpec{
			Scope:       in.String("scope_type"),
			ScopeID:     scopeID,
			Name:        in.String("name"),
			Description: in.String("description"),
			RootType:    in.String("root_type"),
			Root:        root,
		},
	})
	if err != nil {
		return nil, err
	}
	return templateOutput(template), nil
}

// ListTemplates reads the templates reachable from a container's path.
type ListTemplates struct {
	Templates  repository.Templates
	Containers repository.Containers
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// ListTemplatesQuery is the input, typed.
type ListTemplatesQuery struct {
	ContainerID shared.ID
	Cursor      string
	Size        int
}

// Execute returns one page of what the caller may use.
func (h ListTemplates) Execute(
	ctx context.Context, actor appshared.ActorContext, query ListTemplatesQuery,
) (repository.TemplatePage, error) {
	path := []identity.Scope{identity.TenantScope()}
	var scopes []shared.ID

	if !query.ContainerID.IsZero() {
		var container domain.Container
		err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			container, err = h.Containers.Find(ctx, query.ContainerID)
			return err
		})
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				return repository.TemplatePage{}, shared.ErrNotFound.
					WithDetail("containers.not_found").
					WithParams(map[string]string{"container_id": query.ContainerID.String()})
			}
			return repository.TemplatePage{}, err
		}
		path = containerPath(container)
		scopes = []shared.ID{container.ID}
		if !container.ParentID.IsZero() {
			scopes = append(scopes, container.ParentID)
		}
	}

	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       path,
		Action:     TemplatesReadAction,
		TokenScope: templatesRead,
		TargetType: templateTarget,
		TargetID:   query.ContainerID,
	}); err != nil {
		return repository.TemplatePage{}, err
	}

	var page repository.TemplatePage
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = h.Templates.ListInScopes(ctx, scopes, repository.Page{
			Cursor: query.Cursor, Size: PageSize(query.Size),
		})
		return err
	})
	if err != nil {
		return repository.TemplatePage{}, err
	}
	return page, nil
}

// Descriptor is the catalogue entry.
func (h ListTemplates) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListTemplatesName,
		Summary: "The templates a person can pick from, newest first. Naming a container answers " +
			"the ones defined along its path and the workspace-wide ones; naming none answers " +
			"the workspace-wide ones alone.",
		SideEffects: "None. Reads only.",
		TokenScope:  templatesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "container_id", Kind: usecase.KindID,
				Description: "The container whose templates are wanted, together with the ones " +
					"above it.",
			},
			{Name: "cursor", Kind: usecase.KindString, Description: "The previous page's cursor."},
			{Name: "size", Kind: usecase.KindInt, Description: "How many to return."},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplatesReadAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ListTemplates) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	containerID, err := in.ID("container_id")
	if err != nil {
		return nil, err
	}

	page, err := h.Execute(ctx, actor, ListTemplatesQuery{
		ContainerID: containerID, Cursor: in.String("cursor"), Size: in.Int("size"),
	})
	if err != nil {
		return nil, err
	}

	rows := make([]usecase.Output, 0, len(page.Templates))
	for _, template := range page.Templates {
		rows = append(rows, templateOutput(template))
	}
	return pageOutput(rows, page.Info), nil
}

// GetTemplate reads one template.
type GetTemplate struct {
	Writer TemplateWriter
}

// Execute returns the template, if the caller may see where it is defined.
func (h GetTemplate) Execute(
	ctx context.Context, actor appshared.ActorContext, id shared.ID,
) (domain.Template, error) {
	w := h.Writer
	if id.IsZero() {
		return domain.Template{}, templateIDRequired()
	}

	var template domain.Template
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		template, err = w.findTemplate(ctx, id)
		return err
	})
	if err != nil {
		return domain.Template{}, err
	}

	path, err := w.scopePath(ctx, actor, template.Scope, template.ScopeID)
	if err != nil {
		return domain.Template{}, err
	}
	if err := w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRead,
		Path:       path,
		Action:     TemplatesReadAction,
		TokenScope: templatesRead,
		TargetType: templateTarget,
		TargetID:   template.ID,
	}); err != nil {
		// What the caller may not see answers exactly what a template that is not there answers
		// (T-04): a refusal would tell them one exists.
		return domain.Template{}, templateNotFound(id)
	}
	return template, nil
}

// Descriptor is the catalogue entry.
func (h GetTemplate) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        GetTemplateName,
		Summary:     "Reads one template, tree and all. What the caller may not see answers that there is no such template.",
		SideEffects: "None. Reads only.",
		TokenScope:  templatesRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{
				Name: "template_id", Kind: usecase.KindID, Required: true,
				Description: "The template wanted.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: TemplatesReadAction, TargetType: templateTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h GetTemplate) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	id, err := in.ID("template_id")
	if err != nil {
		return nil, err
	}

	template, err := h.Execute(ctx, actor, id)
	if err != nil {
		return nil, err
	}
	return templateOutput(template), nil
}

func templateIDRequired() error {
	return shared.ErrValidation.
		WithDetail("templates.template_id_required").
		WithFields(shared.FieldError{Path: "/template_id", Code: "templates.template_id_required"})
}

// templateTreeOf reads the tree out of the catalogue's untyped input.
//
// One root, because a template is one tree (domain.Template): a document with two would be two
// templates wearing one name, and the instantiation answers one root identifier.
func templateTreeOf(in usecase.Input) (domain.TemplateNode, error) {
	raw, present := in["nodes"]
	if !present {
		return domain.TemplateNode{}, shared.ErrValidation.
			WithDetail("usecase.field_required").
			WithFields(shared.FieldError{Path: "/nodes", Code: "usecase.field_required"})
	}

	nodes, ok := raw.([]any)
	if !ok || len(nodes) != 1 {
		return domain.TemplateNode{}, shared.ErrValidation.
			WithDetail("templates.root_type_mismatch").
			WithFields(shared.FieldError{Path: "/nodes", Code: "templates.root_type_mismatch"})
	}
	return templateNodeOf(nodes[0], "/nodes/0")
}

// templateNodeOf reads one node and its children, carrying the path so that a refusal points at
// the node rather than at the document.
func templateNodeOf(raw any, path string) (domain.TemplateNode, error) {
	document, ok := raw.(map[string]any)
	if !ok {
		return domain.TemplateNode{}, shared.ErrValidation.
			WithDetail("usecase.field_type_invalid").
			WithFields(shared.FieldError{Path: path, Code: "usecase.field_type_invalid"})
	}

	node := domain.TemplateNode{
		Type:  domain.ItemType(textOf(document["type"])),
		Title: textOf(document["title"]),
		Notes: textOf(document["notes"]),
	}
	if flag, isBool := document["due_date_only"].(bool); isBool {
		node.DueDateOnly = flag
	}
	if offset := textOf(document["due_offset"]); offset != "" {
		parsed, err := domain.ParseTemplateOffset(offset)
		if err != nil {
			return domain.TemplateNode{}, withPath(err, path+"/due_offset")
		}
		node.DueOffset = &parsed
	}
	if assignee := textOf(document["assignee_id"]); assignee != "" {
		id, err := shared.ParseID(assignee)
		if err != nil {
			return domain.TemplateNode{}, shared.ErrValidation.
				WithDetail("shared.id_malformed").
				WithFields(shared.FieldError{
					Path: path + "/assignee_id", Code: "shared.id_malformed",
				})
		}
		node.AssigneeID = id
	}

	children, _ := document["children"].([]any)
	for index, child := range children {
		read, err := templateNodeOf(child, path+"/children/"+strconv.Itoa(index))
		if err != nil {
			return domain.TemplateNode{}, err
		}
		node.Children = append(node.Children, read)
	}
	return node, nil
}

// textOf reads a string out of an untyped document, and the empty string for anything else: what
// a value is not a string *means* is the domain's question, and every one of those refusals is
// already written down there.
func textOf(value any) string {
	text, _ := value.(string)
	return text
}

// withPath re-points a refusal at the node that caused it. The domain names the field it knows
// about - "/due_offset" - and only the caller knows which node that was.
func withPath(err error, path string) error {
	refusal := shared.AsError(err)
	if refusal == nil || len(refusal.Fields) == 0 {
		return err
	}
	fields := make([]shared.FieldError, 0, len(refusal.Fields))
	for _, field := range refusal.Fields {
		field.Path = path
		fields = append(fields, field)
	}
	return shared.ErrValidation.
		WithDetail(refusal.DetailCode).
		WithParams(refusal.Params).
		WithFields(fields...)
}
