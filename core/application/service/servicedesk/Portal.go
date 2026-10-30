// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"

	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	"github.com/Jersyfi/hubtask/core/application/service/work"
	"github.com/Jersyfi/hubtask/core/application/service/workflow"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/view"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
)

// The portal's use cases (UC-SVC-05): what a requester - REQUEST without READ, or anybody holding
// REQUEST - does at a collection that serves as a service. Each asks REQUEST at the collection and
// answers only the requester's own requests, never the team's notes.
const (
	ListRequestCatalogueName = "ListRequestCatalogue"
	RaiseRequestName         = "RaiseRequest"
	ListMyRequestsName       = "ListMyRequests"
	ReadMyRequestName        = "ReadMyRequest"
	CommentOnMyRequestName   = "CommentOnMyRequest"

	requestTarget = "request"
	// The registry's names of the entry use cases the portal raises and answers through.
	createEntryUseCase = "CreateWorkItem"
	addCommentUseCase  = "AddComment"
	// MaxPortalComments bounds the discussion one read answers; a longer one reads its newest.
	MaxPortalComments  = 200
	defaultRequestPage = 50
	maxRequestPage     = 100
)

const (
	CatalogueReadAction audit.Action = "request.catalogue_read"
	RequestsReadAction  audit.Action = "request.read"
	RequestRaisedAction audit.Action = "request.raised"
	// RequestCommentedAction names the door; AddComment's own entry holds the comment.
	RequestCommentedAction audit.Action = "request.commented"
)

// EntryInvoker is the registry: the portal raises and answers through the entry use cases
// themselves, so a request is created and commented exactly as an entry is.
type EntryInvoker interface {
	Invoke(ctx context.Context, name string, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error)
}

// Portal is what the portal's use cases share.
type Portal struct {
	Entries    EntryInvoker
	Items      workrepo.Items
	Containers workrepo.Containers
	Comments   workrepo.Comments
	EntryTypes workrepo.EntryTypes
	// Fields answers the custom fields in force at the collection, so the catalogue says what
	// each question is; nil asks none.
	Fields workrepo.CustomFields
	Gate   work.EntryGate
	Desk       Desk
}

// portal marks the act as the portal's door, which the entry use cases read as REQUEST.
func portal(actor appshared.ActorContext) appshared.ActorContext {
	actor.Channel = string(workdomain.ChannelPortal)
	return actor
}

// collectionOf reads a collection and asks REQUEST there.
func (p Portal) collectionOf(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, action audit.Action,
) (workdomain.Container, error) {
	if collectionID.IsZero() {
		return workdomain.Container{}, malformed("/collection_id", "requests.collection_required")
	}
	var collection workdomain.Container
	err := p.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Containers.Find(ctx, collectionID)
		collection = found
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && collection.Type != workdomain.ContainerCollection) {
		return workdomain.Container{}, appshared.ContainerNotFound(collectionID)
	}
	if err != nil {
		return workdomain.Container{}, err
	}
	err = p.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRequest, Path: service.ContainerScopes(collection),
		Action: action, TokenScope: deskRead, TargetType: requestTarget, TargetID: collectionID,
	})
	return collection, err
}

// ownRequest reads an entry the actor raised and asks REQUEST at its collection; anybody else's
// entry is not found, so the portal tells nobody what others asked.
func (p Portal) ownRequest(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID,
) (workdomain.WorkItem, error) {
	var (
		item       workdomain.WorkItem
		collection workdomain.Container
	)
	err := p.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := p.Items.Find(ctx, itemID)
		if err != nil {
			return err
		}
		item = found
		collection, err = p.Containers.Find(ctx, found.CollectionID)
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (item.CreatedBy != actor.AccountID || item.DeletedAt != nil)) {
		return workdomain.WorkItem{}, appshared.ItemNotFound(itemID)
	}
	if err != nil {
		return workdomain.WorkItem{}, err
	}
	if err := p.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionRequest, Path: service.ContainerScopes(collection),
		Action: RequestsReadAction, TokenScope: deskRead, TargetType: requestTarget, TargetID: itemID,
	}); err != nil {
		return workdomain.WorkItem{}, err
	}
	return item, nil
}

// catalogueOf answers what the collection offers through the portal.
func (p Portal) catalogueOf(ctx context.Context, actor appshared.ActorContext, collection workdomain.Container) ([]domain.CatalogueEntry, error) {
	var catalogue []domain.CatalogueEntry
	err := p.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := p.EntryTypes.List(ctx)
		if err != nil {
			return err
		}
		offered, err := p.Gate.OfferedTaskTypes(ctx, collection)
		if err != nil {
			return err
		}
		var definitions []workdomain.CustomFieldDefinition
		if p.Fields != nil {
			if definitions, err = p.Fields.ListInScope(ctx, collection.ID); err != nil {
				return err
			}
		}
		catalogue = domain.RequestCatalogue(workdomain.NewEntryTypes(rows), offered, definitions)
		return nil
	})
	return catalogue, err
}

// RequestOutput is a request as its requester reads it (api/openapi.yaml, schema PortalRequest):
// what they asked and where it stands, never the team's fields.
func RequestOutput(item workdomain.WorkItem) usecase.Output {
	out := usecase.Output{
		"id": item.ID.String(), "number": item.Number, "collection_id": item.CollectionID.String(),
		"entry_type": item.EntryType, "title": item.Title, "notes": nil, "status": nil,
		"is_completed": item.Completion.IsCompleted, "completed_at": nil,
		"created_at": item.CreatedAt.UTC(), "updated_at": item.UpdatedAt.UTC(),
	}
	if item.EntryType == "" {
		out["entry_type"] = workdomain.SystemEntryTypeKey(item.Type)
	}
	if item.Notes != "" {
		out["notes"] = item.Notes
	}
	if item.Status != "" {
		out["status"] = item.Status
	}
	if item.Completion.CompletedAt != nil {
		out["completed_at"] = item.Completion.CompletedAt.UTC()
	}
	return out
}

// ListRequestCatalogue answers the kinds of request a collection offers (UC-SVC-05 check 1).
type ListRequestCatalogue struct{ Portal Portal }

// Execute answers the catalogue.
func (h ListRequestCatalogue) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) ([]domain.CatalogueEntry, error) {
	collection, err := h.Portal.collectionOf(ctx, actor, collectionID, CatalogueReadAction)
	if err != nil {
		return nil, err
	}
	return h.Portal.catalogueOf(ctx, actor, collection)
}

// Descriptor is the catalogue entry.
func (h ListRequestCatalogue) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListRequestCatalogueName, Permission: service.PermissionRequest,
		Summary: "The kinds of request a collection offers through the portal: each type whose form " +
			"names the PORTAL channel and leaves it open, with its form as the portal shows it.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."}},
		Audit: usecase.AuditDeclaration{Action: CatalogueReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			catalogue, err := h.Execute(ctx, actor, collectionID)
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(catalogue))
			for _, entry := range catalogue {
				questions := make([]usecase.Output, 0, len(entry.Questions))
				for _, question := range entry.Questions {
					options := question.Options
					if options == nil {
						options = []string{}
					}
					questions = append(questions, usecase.Output{
						"key": question.Key, "kind": string(question.Kind), "options": options,
					})
				}
				data = append(data, usecase.Output{
					"key": entry.Key, "name": entry.Name, "icon": entry.Icon,
					"form": workflow.FormOutput(entry.Form), "questions": questions,
				})
			}
			return usecase.Output{"data": data}, nil
		}),
	}, requestTarget)
}

// RaiseRequest raises a request of a type the catalogue offers (UC-SVC-05 check 2): created as an
// entry through the PORTAL door, held to the type's form there, raised in the requester's name.
type RaiseRequest struct{ Portal Portal }

// RaiseRequestCommand is what the requester sends.
type RaiseRequestCommand struct {
	CollectionID shared.ID
	EntryType    string
	Title        string
	Notes        string
	CustomFields map[string]any
}

// Execute raises the request and answers it as its requester reads it.
func (h RaiseRequest) Execute(ctx context.Context, actor appshared.ActorContext, cmd RaiseRequestCommand) (usecase.Output, error) {
	collection, err := h.Portal.collectionOf(ctx, actor, cmd.CollectionID, RequestRaisedAction)
	if err != nil {
		return nil, err
	}
	catalogue, err := h.Portal.catalogueOf(ctx, actor, collection)
	if err != nil {
		return nil, err
	}
	if !domain.InCatalogue(catalogue, cmd.EntryType) {
		return nil, malformed("/entry_type", "requests.type_not_offered")
	}
	in := usecase.Input{
		"type": string(workdomain.ItemTask), "collection_id": cmd.CollectionID.String(),
		"entry_type": cmd.EntryType, "title": cmd.Title,
	}
	if cmd.Notes != "" {
		in["notes"] = cmd.Notes
	}
	if len(cmd.CustomFields) > 0 {
		in["custom_fields"] = cmd.CustomFields
	}
	out, err := h.Portal.Entries.Invoke(ctx, createEntryUseCase, portal(actor), in)
	if err != nil {
		return nil, err
	}
	id, err := shared.ParseID(out.String("id"))
	if err != nil {
		return nil, err
	}
	var item workdomain.WorkItem
	err = h.Portal.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		if item, err = h.Portal.Items.Find(ctx, id); err != nil {
			return err
		}
		return h.Portal.Desk.record(ctx, actor, RequestRaisedAction, requestTarget, id, h.Portal.Desk.Clock.Now(),
			audit.Change{Field: "entry_type", Classification: audit.Open, To: cmd.EntryType})
	})
	if err != nil {
		return nil, err
	}
	return RequestOutput(item), nil
}

// Descriptor is the catalogue entry. The entry use case audits and announces the creation.
func (h RaiseRequest) Descriptor() usecase.Descriptor {
	d := descriptor(usecase.Descriptor{
		Name: RaiseRequestName, Permission: service.PermissionRequest,
		Summary: "Raises a request of a kind the collection's portal catalogue offers, in the " +
			"requester's name, held to the type's form as the portal narrows it.",
		SideEffects: "Creates the entry through CreateWorkItem on the PORTAL door: its event, " +
			"history and audit entry; the SLA clocks start when a policy measures its type.",
		TokenScope: "items:write",
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."},
			{Name: "entry_type", Kind: usecase.KindString, Required: true, Description: "The kind of request, a key the catalogue offers."},
			{Name: "title", Kind: usecase.KindString, Required: true, Description: "What the request is about."},
			{Name: "notes", Kind: usecase.KindString, Description: "The request's description."},
			{Name: "custom_fields", Kind: usecase.KindObject, Description: "The form's answers by field key."},
		},
		Audit:    usecase.AuditDeclaration{Action: RequestRaisedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "the entry use case records the creation in the entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			fields, _ := in["custom_fields"].(map[string]any)
			return h.Execute(ctx, actor, RaiseRequestCommand{
				CollectionID: collectionID, EntryType: in.String("entry_type"),
				Title: in.String("title"), Notes: in.String("notes"), CustomFields: fields,
			})
		}),
	}, requestTarget)
	return d
}

// ListMyRequests answers the requests the actor raised in a collection, newest first
// (UC-SVC-05 check 3).
type ListMyRequests struct{ Portal Portal }

// Execute answers one page.
func (h ListMyRequests) Execute(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, cursor string, size int,
) (workrepo.ItemQueryResult, error) {
	if _, err := h.Portal.collectionOf(ctx, actor, collectionID, RequestsReadAction); err != nil {
		return workrepo.ItemQueryResult{}, err
	}
	filter, err := view.ParseFilter(map[string]any{
		"field": view.FieldCreatedBy, "op": "EQ", "value": actor.AccountID.String(),
	}, "/filter")
	if err != nil {
		return workrepo.ItemQueryResult{}, err
	}
	sort, err := view.ParseSort([]any{map[string]any{"field": view.FieldCreatedAt, "dir": "DESC"}}, "/sort")
	if err != nil {
		return workrepo.ItemQueryResult{}, err
	}
	if size < 1 {
		size = defaultRequestPage
	}
	size = min(size, maxRequestPage)
	search := workrepo.ItemSearch{
		Anchor: workrepo.Anchor{Kind: workrepo.AnchorCollection, CollectionID: collectionID},
		Spec:   view.Spec{Filter: filter, Sort: sort, Size: size, Cursor: cursor},
	}
	var result workrepo.ItemQueryResult
	err = h.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		result, err = h.Portal.Items.Query(ctx, search)
		return err
	})
	return result, err
}

// Descriptor is the catalogue entry.
func (h ListMyRequests) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ListMyRequestsName, Permission: service.PermissionRequest,
		Summary:     "The requests the caller raised in a collection, newest first, as the requester reads them.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection serving as the service."},
			{Name: "cursor", Kind: usecase.KindString, Description: "Continues a previous page."},
			{Name: "size", Kind: usecase.KindInt, Description: "At most 100; 50 when left out."},
		},
		Audit: usecase.AuditDeclaration{Action: RequestsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			collectionID, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			result, err := h.Execute(ctx, actor, collectionID, in.String("cursor"), in.Int("size"))
			if err != nil {
				return nil, err
			}
			data := make([]usecase.Output, 0, len(result.Items))
			for _, item := range result.Items {
				data = append(data, RequestOutput(item))
			}
			return usecase.Output{"data": data, "page_info": usecase.Output{
				"has_more": result.Info.HasMore, "next_cursor": result.Info.NextCursor,
			}}, nil
		}),
	}, requestTarget)
}

// MyRequest is one request with its public discussion and its SLA goals.
type MyRequest struct {
	Item     workdomain.WorkItem
	Comments []workdomain.Comment
	Goals    []domain.EntrySla
	// Rating is the requester's own score, nil while they have given none (UC-SVC-07).
	Rating *domain.Rating
}

// ReadMyRequest answers one of the actor's requests: where it stands, the public replies and
// never an internal note, and its SLA goals (UC-SVC-05 check 4, UC-SVC-04 check 2).
type ReadMyRequest struct{ Portal Portal }

// Execute answers the request.
func (h ReadMyRequest) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID) (MyRequest, error) {
	item, err := h.Portal.ownRequest(ctx, actor, itemID)
	if err != nil {
		return MyRequest{}, err
	}
	answer := MyRequest{Item: item, Comments: []workdomain.Comment{}}
	err = h.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		cursor := ""
		for {
			page, err := h.Portal.Comments.List(ctx, itemID, workrepo.Page{Cursor: cursor, Size: maxRequestPage})
			if err != nil {
				return err
			}
			answer.Comments = append(answer.Comments, PublicComments(page.Comments)...)
			if !page.Info.HasMore || len(answer.Comments) >= MaxPortalComments {
				break
			}
			cursor = page.Info.NextCursor
		}
		if len(answer.Comments) > MaxPortalComments {
			answer.Comments = answer.Comments[len(answer.Comments)-MaxPortalComments:]
		}
		if answer.Goals, err = h.Portal.Desk.Records.OfItem(ctx, itemID); err != nil {
			return err
		}
		if h.Portal.Desk.Ratings == nil {
			return nil
		}
		rating, err := h.Portal.Desk.Ratings.OfItem(ctx, itemID)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			return nil
		case err != nil:
			return err
		}
		answer.Rating = &rating
		return nil
	})
	return answer, err
}

// PublicComments keeps what a requester reads: what somebody wrote and did not mark internal.
func PublicComments(comments []workdomain.Comment) []workdomain.Comment {
	public := make([]workdomain.Comment, 0, len(comments))
	for _, comment := range comments {
		if comment.Internal || comment.Kind == workdomain.CommentBySystem {
			continue
		}
		public = append(public, comment)
	}
	return public
}

// Descriptor is the catalogue entry.
func (h ReadMyRequest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadMyRequestName, Permission: service.PermissionRequest,
		Summary: "One request the caller raised: where it stands, the public replies - never an " +
			"internal note -, how its SLA goals stand and the caller's own satisfaction score.",
		SideEffects: "None. Reads only.", TokenScope: deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The request."}},
		Audit: usecase.AuditDeclaration{Action: RequestsReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			answer, err := h.Execute(ctx, actor, itemID)
			if err != nil {
				return nil, err
			}
			out := RequestOutput(answer.Item)
			comments := make([]usecase.Output, 0, len(answer.Comments))
			for _, comment := range answer.Comments {
				body := any(nil)
				if comment.DeletedAt == nil {
					body = comment.Body
				}
				reply := usecase.Output{
					"id": comment.ID.String(), "author_id": comment.AuthorID.String(), "body": body,
					"created_at": comment.CreatedAt.UTC(), "parent_comment_id": nil,
					"mine": comment.AuthorID == actor.AccountID,
				}
				if !comment.ParentCommentID.IsZero() {
					reply["parent_comment_id"] = comment.ParentCommentID.String()
				}
				comments = append(comments, reply)
			}
			goals := make([]usecase.Output, 0, len(answer.Goals))
			for _, goal := range answer.Goals {
				goals = append(goals, RecordOutput(goal))
			}
			out["comments"], out["sla"] = comments, goals
			out["satisfaction"] = RatingOutput(answer.Rating)
			return out, nil
		}),
	}, requestTarget)
}

// CommentOnMyRequest answers on one of the actor's requests (UC-SVC-05 check 5): a public
// comment, written through AddComment on the PORTAL door.
type CommentOnMyRequest struct{ Portal Portal }

// Execute writes the comment.
func (h CommentOnMyRequest) Execute(
	ctx context.Context, actor appshared.ActorContext, itemID, parentID shared.ID, body string,
) (usecase.Output, error) {
	if _, err := h.Portal.ownRequest(ctx, actor, itemID); err != nil {
		return nil, err
	}
	in := usecase.Input{"item_id": itemID.String(), "body": body}
	if !parentID.IsZero() {
		in["parent_comment_id"] = parentID.String()
	}
	out, err := h.Portal.Entries.Invoke(ctx, addCommentUseCase, portal(actor), in)
	if err != nil {
		return nil, err
	}
	err = h.Portal.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return h.Portal.Desk.record(ctx, actor, RequestCommentedAction, requestTarget, itemID, h.Portal.Desk.Clock.Now())
	})
	return out, err
}

// Descriptor is the catalogue entry. AddComment audits and announces the comment.
func (h CommentOnMyRequest) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CommentOnMyRequestName, Permission: service.PermissionRequest,
		Summary:     "Answers on a request the caller raised, as a public comment the team reads.",
		SideEffects: "Writes the comment through AddComment on the PORTAL door: its event, history and audit entry.",
		TokenScope:  "comments:write",
		Input: []usecase.Field{
			{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The request."},
			{Name: "body", Kind: usecase.KindString, Required: true, Description: "The text, up to 20000 characters."},
			{Name: "parent_comment_id", Kind: usecase.KindID, Description: "The public comment this one replies to."},
		},
		Audit:    usecase.AuditDeclaration{Action: RequestCommentedAction},
		Activity: usecase.ActivityDeclaration{Exempt: "AddComment records the comment in the entry's history."},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			parentID, err := in.ID("parent_comment_id")
			if err != nil {
				return nil, err
			}
			return h.Execute(ctx, actor, itemID, parentID, in.String("body"))
		}),
	}, requestTarget)
}
