// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package knowledge is the knowledge module's use cases (M3): writing, finding, moving, versioning,
// commenting and exporting a hub's pages.
//
// A page is judged at its hub: reading needs READ there, writing and commenting WRITE_ITEMS, so a
// page inherits the hub's memberships and a private hub's pages are its owner's alone (ADR-0073).
// A restricted page narrows that to the people it names, never widens it; a share opens one page
// to one person for reading beside the hub (Access.go).
// The token scopes are the entries' own, as the live document's are (realtime): a page is work as
// an entry is, and a token's vocabulary is the catalogue's.
package knowledge

import (
	"context"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/knowledge"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes, the entries' own.
const (
	pagesRead  = "items:read"
	pagesWrite = "items:write"
)

// What the trail names.
const (
	pageTarget    = "page"
	commentTarget = "page_comment"
)

// The stable audit codes (audit.md §2). A page's words are content and go to the trail only as
// fingerprints (rule 10).
const (
	PageReadAction          audit.Action = "page.read"
	PageCreatedAction       audit.Action = "page.created"
	PageChangedAction       audit.Action = "page.changed"
	PageMovedAction         audit.Action = "page.moved"
	PageTrashedAction       audit.Action = "page.trashed"
	PageRestoredAction      audit.Action = "page.restored"
	PageVerifiedAction      audit.Action = "page.verified"
	PageVersionNamedAction  audit.Action = "page.version_named"
	PageVersionBackAction   audit.Action = "page.version_restored"
	PageExportedAction      audit.Action = "page.exported"
	PageCommentAddedAction  audit.Action = "page_comment.added"
	PageCommentChangeAction audit.Action = "page_comment.changed"
	PageCommentDeleteAction audit.Action = "page_comment.deleted"
)

// pagesExempt is why no page use case writes an entry's history: a page is not an entry, and its
// own history is its versions (UC-DOC-06) and the trail.
const pagesExempt = "a page is not an entry; its history is its versions and the audit trail."

// Authorizer records a refusal and answers it; Permits answers without a record, for a question
// that is nobody trying anything (whether to offer writing; which hits a reader may see).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// Reader filters many places at once without a record (a search, backlinks).
type Reader interface {
	Permitted(ctx context.Context, actor appshared.ActorContext, request access.Request, paths [][]identity.Scope) ([]bool, error)
}

// Space is what every page use case shares.
type Space struct {
	Pages      repository.Pages
	Containers workrepo.Containers
	// Entries resolves the entries a backlink names, so that a reader is shown only those they see.
	Entries    workrepo.References
	Authorizer Authorizer
	Reader     Reader
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
	// Reviews arms the clock of a page's review when it is verified; nil arms none.
	Reviews ReviewTimers
	// Changes and HLC tell offline devices of a page written, moved, trashed or restored, so that
	// it can be read without the server. A nil Changes records nothing, a test's convenience.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
	// PersonalHubs makes a person's own private hub on first use (UC-DOC-07), named in their
	// language by Words; nil offers no personal space.
	PersonalHubs PersonalHubs
	Words        i18n.Renderer
}

// announce records each page as it now stands, body included, filed under its hub.
func (s Space) announce(ctx context.Context, actor appshared.ActorContext, pages ...domain.Page) error {
	if s.Changes == nil {
		return nil
	}
	now := s.Clock.Now()
	ids := make([]shared.ID, 0, len(pages))
	for _, page := range pages {
		ids = append(ids, page.ID)
	}
	readers, err := s.Pages.Readers(ctx, ids)
	if err != nil {
		return err
	}
	for _, page := range pages {
		change := changelog.Change{
			TenantID: actor.TenantID, Entity: domain.DocumentKind, EntityID: page.ID, Op: changelog.Upsert,
			ContainerID: page.HubID, ActorID: actor.AccountID, HLC: s.HLC.Next(),
			Payload: PageOutput(page, true, now),
		}
		// A change is told to every reader of the hub; a page behind a restriction is not theirs
		// all, so the devices drop it and read it from the server.
		if readers[page.ID].Restricted {
			change.Op, change.Payload = changelog.Delete, nil
		}
		if err := s.Changes.Record(ctx, change); err != nil {
			return err
		}
	}
	return nil
}

// announceSubtree records a page and the pages under it, after a trash or a restore moved them
// together.
func (s Space) announceSubtree(ctx context.Context, actor appshared.ActorContext, id shared.ID, trashed bool) error {
	if s.Changes == nil {
		return nil
	}
	pages, err := s.Pages.Subtree(ctx, id, trashed, domain.MaxPagesPerHub)
	if err != nil {
		return err
	}
	return s.announce(ctx, actor, pages...)
}

func hubPath(hubID shared.ID) []identity.Scope {
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
}

func (s Space) request(hubID shared.ID, permission service.Permission, action audit.Action, scope, target string, id shared.ID) access.Request {
	return access.Request{
		Permission: permission, Path: hubPath(hubID), Action: action, TokenScope: scope,
		TargetType: target, TargetID: id,
	}
}

// authorizeRead asks READ at the page's hub.
func (s Space) authorizeRead(ctx context.Context, actor appshared.ActorContext, page domain.Page, action audit.Action) error {
	return s.Authorizer.Authorize(ctx, actor, s.request(page.HubID, service.PermissionRead, action, pagesRead, pageTarget, page.ID))
}

// authorizeWrite asks WRITE_ITEMS at a hub.
func (s Space) authorizeWrite(ctx context.Context, actor appshared.ActorContext, hubID, id shared.ID, action audit.Action, target string) error {
	return s.Authorizer.Authorize(ctx, actor, s.request(hubID, service.PermissionWriteItems, action, pagesWrite, target, id))
}

// mayWrite answers whether the reader may write in a hub, without a record.
func (s Space) mayWrite(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (bool, error) {
	if actor.RequireScope(pagesWrite) != nil {
		return false, nil
	}
	return s.Authorizer.Permits(ctx, actor, s.request(hubID, service.PermissionWriteItems, PageChangedAction, pagesWrite, pageTarget, hubID))
}

// hub reads the hub a page sits in, refusing anything that is not a hub or is in the trash.
func (s Space) hub(ctx context.Context, hubID shared.ID) (work.Container, error) {
	container, err := s.Containers.Find(ctx, hubID)
	if err != nil {
		return work.Container{}, err
	}
	if container.Type != work.ContainerHub || container.DeletedAt != nil {
		return work.Container{}, shared.ErrNotFound.WithDetail("pages.space_not_found")
	}
	return container, nil
}

// find reads a page that is not trashed, inside the caller's transaction.
func (s Space) find(ctx context.Context, id shared.ID, lock bool) (domain.Page, error) {
	var page domain.Page
	var err error
	if lock {
		page, err = s.Pages.FindForUpdate(ctx, id)
	} else {
		page, err = s.Pages.Find(ctx, id)
	}
	if err != nil {
		return domain.Page{}, err
	}
	if page.IsTrashed() {
		return domain.Page{}, shared.ErrNotFound.WithDetail("pages.not_found")
	}
	return page, nil
}

// readable reads a page and asks READ at its hub.
func (s Space) readable(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action) (domain.Page, error) {
	if err := actor.RequireScope(pagesRead); err != nil {
		return domain.Page{}, err
	}
	var page domain.Page
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		page, err = s.find(ctx, id, false)
		return err
	})
	if err != nil {
		return domain.Page{}, err
	}
	if err := s.admit(ctx, actor, page.ID); err != nil {
		return domain.Page{}, err
	}
	return page, s.authorizeRead(ctx, actor, page, action)
}

// placeOf reads the page's hub before a write: the permission is asked there, outside the write's
// transaction, the way a reader asks before it reads.
func (s Space) placeOf(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Page, error) {
	var page domain.Page
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if page, err = s.find(ctx, id, false); err != nil {
			return err
		}
		return s.admitted(ctx, actor, page.ID)
	})
	return page, err
}

// snapshot keeps the page's state in its history after a write of its title or body.
func (s Space) snapshot(ctx context.Context, page domain.Page, author shared.ID, now time.Time) error {
	last, found, err := s.Pages.LastVersion(ctx, page.ID)
	if err != nil {
		return err
	}
	var previous *domain.Version
	if found {
		previous = &last
	}
	version, fold := domain.Snapshot(previous, page, author, now)
	if fold {
		return s.Pages.FoldVersion(ctx, version)
	}
	return s.Pages.InsertVersion(ctx, version)
}

// record writes the evidence: what changed by name, a page's words as fingerprints (rule 10).
func (s Space) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID,
	changes []audit.Change, now time.Time,
) error {
	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: now, Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// changed names the fields a write changed: words as Sensitive, the rest open.
func changed(fields []string, page domain.Page) []audit.Change {
	out := make([]audit.Change, 0, len(fields))
	for _, field := range fields {
		switch field {
		case domain.FieldTitle:
			out = append(out, audit.Change{Field: field, Classification: audit.Sensitive, To: page.Title})
		case domain.FieldBody:
			out = append(out, audit.Change{Field: field, Classification: audit.Sensitive, To: page.Body})
		case domain.FieldArchived:
			out = append(out, audit.Change{Field: field, Classification: audit.Open, To: page.IsArchived()})
		case domain.FieldTemplate:
			out = append(out, audit.Change{Field: field, Classification: audit.Open, To: page.Template})
		default:
			out = append(out, audit.Change{Field: field, Classification: audit.Open})
		}
	}
	return out
}

// PageOutput is the shape every door returns (api/openapi.yaml, schema Page). The body is left out
// of a tree's rows.
func PageOutput(page domain.Page, withBody bool, now time.Time) usecase.Output {
	out := usecase.Output{
		"id": page.ID.String(), "hub_id": page.HubID.String(), "parent_id": idOrNil(page.ParentID),
		"title": page.Title, "icon": nilIfEmpty(page.Icon), "position": page.OrderKey,
		"template": page.Template, "content_language": nilIfEmpty(page.ContentLanguage),
		"owner_id": idOrNil(page.OwnerID), "verified_until": timeOrNil(page.VerifiedUntil),
		"verified_at": timeOrNil(page.VerifiedAt), "verified_by": idOrNil(page.VerifiedBy),
		"due_for_review": page.DueForReview(now), "archived": page.IsArchived(),
		"restricted": page.Restricted, "daily_for": dayOrNil(page.DailyFor),
		"trashed_at": timeOrNil(page.DeletedAt),
		"created_by": page.CreatedBy.String(), "created_at": page.CreatedAt,
		"updated_by": page.UpdatedBy.String(), "updated_at": page.UpdatedAt, "version": page.Version,
	}
	if withBody {
		out["body"] = page.Body
	}
	return out
}

func idOrNil(id shared.ID) any {
	if id.IsZero() {
		return nil
	}
	return id.String()
}

func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

func dayOrNil(day *time.Time) any {
	if day == nil {
		return nil
	}
	return day.Format(time.DateOnly)
}

func timeOrNil(at *time.Time) any {
	if at == nil {
		return nil
	}
	return *at
}

// optionalID reads an identifier field that may be absent.
func optionalID(in usecase.Input, field string) (shared.ID, error) {
	if in.String(field) == "" {
		return "", nil
	}
	return in.ID(field)
}

// rawText reads a prose field as sent: a body's leading spaces are its code block.
func rawText(in usecase.Input, field string) *string {
	raw, present := in[field]
	if !present || raw == nil {
		return nil
	}
	value, ok := raw.(string)
	if !ok {
		return nil
	}
	return &value
}

func optionalBool(in usecase.Input, field string) *bool {
	if !in.Present(field) {
		return nil
	}
	value := in.Bool(field)
	return &value
}

// descriptor fills what every page use case declares the same way.
func descriptor(d usecase.Descriptor) usecase.Descriptor {
	d.Module = module.Knowledge
	if d.Activity.Exempt == "" && d.Activity.Verb == "" {
		d.Activity = usecase.ActivityDeclaration{Exempt: pagesExempt}
	}
	return d
}

type repositoryAncestor = repository.Ancestor
