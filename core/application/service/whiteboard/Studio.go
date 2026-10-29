// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package whiteboard is the whiteboard module's use cases (M4): making, drawing, trashing,
// exporting a hub's boards, and the boards an entry is drawn on.
//
// A board is judged at its hub, as a page is: reading needs READ there, writing WRITE_ITEMS, so a
// board inherits the hub's memberships and a private hub's boards are its owner's alone
// (ADR-0073). The token scopes are the entries' own, as the live document's are (realtime).
package whiteboard

import (
	"context"
	"strconv"

	repository "github.com/Jersyfi/hubtask/core/application/repository/whiteboard"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	"github.com/Jersyfi/hubtask/core/domain/model/reference"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The token scopes, the entries' own.
const (
	boardsRead  = "items:read"
	boardsWrite = "items:write"
)

const boardTarget = "whiteboard"

// The stable audit codes (audit.md §2). A board's words are content and go to the trail only as
// fingerprints (rule 10).
const (
	BoardReadAction     audit.Action = "whiteboard.read"
	BoardCreatedAction  audit.Action = "whiteboard.created"
	BoardChangedAction  audit.Action = "whiteboard.changed"
	BoardTrashedAction  audit.Action = "whiteboard.trashed"
	BoardRestoredAction audit.Action = "whiteboard.restored"
	BoardExportedAction audit.Action = "whiteboard.exported"
)

// boardsExempt is why no board use case writes an entry's history.
const boardsExempt = "a whiteboard is not an entry; its history is the audit trail and its live document."

// Authorizer records a refusal and answers it; Permits answers without a record, for a question
// that is nobody trying anything (whether to offer drawing).
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// Studio is what every board use case shares.
type Studio struct {
	Boards     repository.Boards
	Containers workrepo.Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Text       text.Normalizer
}

func hubPath(hubID shared.ID) []identity.Scope {
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
}

func (s Studio) request(hubID shared.ID, permission service.Permission, action audit.Action, scope string, id shared.ID) access.Request {
	return access.Request{
		Permission: permission, Path: hubPath(hubID), Action: action, TokenScope: scope,
		TargetType: boardTarget, TargetID: id,
	}
}

func (s Studio) authorizeRead(ctx context.Context, actor appshared.ActorContext, hubID, id shared.ID, action audit.Action) error {
	return s.Authorizer.Authorize(ctx, actor, s.request(hubID, service.PermissionRead, action, boardsRead, id))
}

func (s Studio) authorizeWrite(ctx context.Context, actor appshared.ActorContext, hubID, id shared.ID, action audit.Action) error {
	return s.Authorizer.Authorize(ctx, actor, s.request(hubID, service.PermissionWriteItems, action, boardsWrite, id))
}

// mayWrite answers whether the reader may draw in a hub, without a record.
func (s Studio) mayWrite(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (bool, error) {
	if actor.RequireScope(boardsWrite) != nil {
		return false, nil
	}
	return s.Authorizer.Permits(ctx, actor, s.request(hubID, service.PermissionWriteItems, BoardChangedAction, boardsWrite, hubID))
}

// hub reads the hub a board sits in, refusing anything that is not a hub or is in the trash.
func (s Studio) hub(ctx context.Context, hubID shared.ID) error {
	container, err := s.Containers.Find(ctx, hubID)
	if err != nil {
		return err
	}
	if container.Type != work.ContainerHub || container.DeletedAt != nil {
		return shared.ErrNotFound.WithDetail("whiteboards.hub_not_found")
	}
	return nil
}

// find reads a board inside the caller's transaction; trashed answers whether a trashed one is
// wanted (a restore) or not found (everything else).
func (s Studio) find(ctx context.Context, id shared.ID, lock, trashed bool) (domain.Board, error) {
	var board domain.Board
	var err error
	if lock {
		board, err = s.Boards.FindForUpdate(ctx, id)
	} else {
		board, err = s.Boards.Find(ctx, id)
	}
	if err != nil {
		return domain.Board{}, err
	}
	if board.IsTrashed() != trashed {
		return domain.Board{}, shared.ErrNotFound.WithDetail("whiteboards.not_found")
	}
	return board, nil
}

// placeOf reads the board's hub before a write: the permission is asked there, outside the
// write's transaction, the way a reader asks before it reads.
func (s Studio) placeOf(ctx context.Context, actor appshared.ActorContext, id shared.ID, trashed bool) (domain.Board, error) {
	var board domain.Board
	err := s.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		board, err = s.find(ctx, id, false, trashed)
		return err
	})
	return board, err
}

// readable reads a board and asks READ at its hub.
func (s Studio) readable(ctx context.Context, actor appshared.ActorContext, id shared.ID, action audit.Action) (domain.Board, error) {
	if err := actor.RequireScope(boardsRead); err != nil {
		return domain.Board{}, err
	}
	board, err := s.placeOf(ctx, actor, id, false)
	if err != nil {
		return domain.Board{}, err
	}
	return board, s.authorizeRead(ctx, actor, board.HubID, board.ID, action)
}

// record writes the evidence: what changed by name, a board's words as fingerprints (rule 10).
func (s Studio) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, board domain.Board, fields []string) error {
	changes := make([]audit.Change, 0, len(fields))
	for _, field := range fields {
		switch field {
		case domain.FieldTitle:
			changes = append(changes, audit.Change{Field: field, Classification: audit.Sensitive, To: board.Title})
		case domain.FieldScene:
			changes = append(changes, audit.Change{Field: field, Classification: audit.Open, To: len(board.Scene.Elements)})
		default:
			changes = append(changes, audit.Change{Field: field, Classification: audit.Open})
		}
	}
	return s.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: s.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: boardTarget, TargetID: board.ID,
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		Changes: audit.Changes(changes...),
	})
}

// BoardOutput is the shape every door returns (api/openapi.yaml, schema Whiteboard). A list's rows
// leave the scene out and count its objects.
func BoardOutput(board domain.Board, elements int, withScene bool) usecase.Output {
	out := usecase.Output{
		"id": board.ID.String(), "hub_id": board.HubID.String(), "title": board.Title,
		"element_count": elements, "trashed_at": nil,
		"created_by": board.CreatedBy.String(), "created_at": board.CreatedAt,
		"updated_by": board.UpdatedBy.String(), "updated_at": board.UpdatedAt, "version": board.Version,
	}
	if board.DeletedAt != nil {
		out["trashed_at"] = *board.DeletedAt
	}
	if withScene {
		out["elements"] = ElementsOutput(board.Scene)
	}
	return out
}

// ElementsOutput is a scene as the contract carries it (schema WhiteboardElement).
func ElementsOutput(scene domain.Scene) []usecase.Output {
	out := make([]usecase.Output, len(scene.Elements))
	for i, e := range scene.Elements {
		element := usecase.Output{
			"id": e.ID, "kind": string(e.Kind), "x": e.X, "y": e.Y, "width": e.Width, "height": e.Height,
			"text": e.Text, "color_token": nilIfEmpty(string(e.Colour)), "locked": e.Locked,
		}
		if e.Group != "" {
			element["group"] = e.Group
		}
		switch e.Kind {
		case domain.KindShape:
			element["shape"] = string(e.Shape)
		case domain.KindConnector:
			element["from"], element["to"] = nilIfEmpty(e.From), nilIfEmpty(e.To)
			element["route"], element["start"], element["end"] = string(e.Route), string(e.Start), string(e.End)
		case domain.KindCard:
			element["target"] = usecase.Output{"kind": string(e.Target.Kind), "id": e.Target.ID.String()}
		case domain.KindStroke:
			element["pen"], element["points"] = string(e.Pen), FlatPoints(e.Points)
		case domain.KindLink:
			element["url"] = e.URL
		}
		out[i] = element
	}
	return out
}

// FlatPoints are a stroke's points as the contract carries them: x and y after each other.
func FlatPoints(points []domain.Point) []float64 {
	out := make([]float64, 0, 2*len(points))
	for _, point := range points {
		out = append(out, point.X, point.Y)
	}
	return out
}

func nilIfEmpty(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// sceneOf reads the `elements` field of an input: absent is nil, anything else is judged whole
// (domain.CheckScene).
func (s Studio) sceneOf(in usecase.Input) (*domain.Scene, error) {
	raw, present := in["elements"]
	if !present || raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, malformed("/elements")
	}
	scene := domain.Scene{Elements: make([]domain.Element, 0, len(list))}
	if len(list) > domain.MaxElements {
		return nil, shared.ErrValidation.WithDetail("whiteboards.too_many_elements").
			WithParams(map[string]string{"maximum": strconv.Itoa(domain.MaxElements)}).
			WithFields(shared.FieldError{Path: "/elements", Code: "whiteboards.too_many_elements"})
	}
	for i, item := range list {
		element, err := elementOf(item, "/elements/"+strconv.Itoa(i))
		if err != nil {
			return nil, err
		}
		scene.Elements = append(scene.Elements, element)
	}
	checked, err := domain.CheckScene(scene, s.Text)
	if err != nil {
		return nil, err
	}
	return &checked, nil
}

func elementOf(raw any, path string) (domain.Element, error) {
	fields, ok := raw.(map[string]any)
	if !ok {
		return domain.Element{}, malformed(path)
	}
	text := func(name string) string { value, _ := fields[name].(string); return value }
	number := func(name string) (float64, error) {
		switch value := fields[name].(type) {
		case nil:
			return 0, nil
		case float64:
			return value, nil
		case int:
			return float64(value), nil
		case int64:
			return float64(value), nil
		}
		return 0, malformed(path + "/" + name)
	}
	element := domain.Element{
		ID: text("id"), Kind: domain.Kind(text("kind")), Text: text("text"),
		Colour: shared.LabelToken(text("color_token")), Shape: domain.Shape(text("shape")),
		From: text("from"), To: text("to"), Route: domain.Route(text("route")),
		Start: domain.Head(text("start")), End: domain.Head(text("end")),
		Pen: domain.Pen(text("pen")), URL: text("url"), Group: text("group"),
	}
	element.Locked, _ = fields["locked"].(bool)
	if raw, present := fields["points"]; present && raw != nil {
		list, ok := raw.([]any)
		if !ok || len(list)%2 != 0 || len(list) > 2*domain.MaxStrokePoints {
			return domain.Element{}, malformed(path + "/points")
		}
		element.Points = make([]domain.Point, 0, len(list)/2)
		for i := 0; i < len(list); i += 2 {
			x, okX := asNumber(list[i])
			y, okY := asNumber(list[i+1])
			if !okX || !okY {
				return domain.Element{}, malformed(path + "/points")
			}
			element.Points = append(element.Points, domain.Point{X: x, Y: y})
		}
	}
	var err error
	for _, member := range []struct {
		name string
		into *float64
	}{{"x", &element.X}, {"y", &element.Y}, {"width", &element.Width}, {"height", &element.Height}} {
		if *member.into, err = number(member.name); err != nil {
			return domain.Element{}, err
		}
	}
	if target, ok := fields["target"].(map[string]any); ok {
		kind, _ := target["kind"].(string)
		raw, _ := target["id"].(string)
		id, err := shared.ParseID(raw)
		if err != nil {
			return domain.Element{}, shared.ErrValidation.WithDetail("whiteboards.card_target_malformed").
				WithFields(shared.FieldError{Path: path + "/target", Code: "whiteboards.card_target_malformed"})
		}
		element.Target = reference.Reference{Kind: reference.Kind(kind), ID: id}
	}
	return element, nil
}

func asNumber(raw any) (float64, bool) {
	switch value := raw.(type) {
	case float64:
		return value, true
	case int:
		return float64(value), true
	case int64:
		return float64(value), true
	}
	return 0, false
}

func malformed(path string) error {
	return shared.ErrValidation.WithDetail("whiteboards.elements_malformed").
		WithFields(shared.FieldError{Path: path, Code: "whiteboards.elements_malformed"})
}

// elementsField declares the scene on every door that writes one.
var elementsField = usecase.Field{
	Name: "elements", Kind: usecase.KindList,
	Description: "The board's objects in drawing order, the last on top: sticky notes, shapes, text, " +
		"connectors, frames, cards of entries, freehand strokes and web links (schema WhiteboardElement). Sent whole; at most 5000.",
}

// descriptor fills what every board use case declares the same way.
func descriptor(d usecase.Descriptor) usecase.Descriptor {
	d.Module = module.Whiteboard
	if d.Activity.Exempt == "" && d.Activity.Verb == "" {
		d.Activity = usecase.ActivityDeclaration{Exempt: boardsExempt}
	}
	return d
}
