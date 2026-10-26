// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mail is the administration of a workspace's mail intake (ADR-0094): its settings, its
// log, a collection's handle, and the address a notification's reply goes to. Receiving a message
// is the jumble's (ReceiveMail), where the jumble entry it may become is made.
package mail

import (
	"context"
	"strconv"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/mail"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ShowMailSettingsName    = "ShowMailSettings"
	SaveMailSettingsName    = "SaveMailSettings"
	ListMailReceiptsName    = "ListMailReceipts"
	SetCollectionHandleName = "SetCollectionHandle"

	automationManage = "automation:manage"
	containersWrite  = "containers:write"
	settingsTarget   = "mail_settings"
	containerTarget  = "container"

	settingsReadAction  = audit.Action("mail.settings_read")
	settingsSavedAction = audit.Action("mail.settings_saved")
	receiptsReadAction  = audit.Action("mail.receipts_read")
	handleSetAction     = audit.Action("mail.handle_set")

	receiptsShown = 100
)

// Authorizer is the permission question.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Writer is what the intake's administration shares.
type Writer struct {
	Settings   repository.Settings
	Receipts   repository.Receipts
	Places     repository.Places
	Containers workrepo.Containers
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Domain is the operator's receiving domain (HUBTASK_MAIL_INBOUND_DOMAIN); empty where the
	// installation receives through mailboxes only.
	Domain string
}

// ShowMailSettings answers the workspace's intake settings and the addresses they make.
type ShowMailSettings struct{ Writer Writer }

// SaveMailSettings replaces them.
type SaveMailSettings struct{ Writer Writer }

// ListMailReceipts answers the newest messages the intake received and what each became.
type ListMailReceipts struct{ Writer Writer }

// SetCollectionHandle gives a collection another handle.
type SetCollectionHandle struct{ Writer Writer }

func (w Writer) authorizeIntake(ctx context.Context, actor appshared.ActorContext, action audit.Action) error {
	return w.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: automationManage, TargetType: settingsTarget,
	})
}

func (h ShowMailSettings) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorizeIntake(ctx, actor, settingsReadAction); err != nil {
		return nil, err
	}
	var settings domain.Settings
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Settings.Find(ctx)
		settings = found
		return err
	})
	if err != nil {
		return nil, err
	}
	return w.output(settings), nil
}

func (h SaveMailSettings) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorizeIntake(ctx, actor, settingsSavedAction); err != nil {
		return nil, err
	}
	settings, err := settingsOf(in)
	if err != nil {
		return nil, err
	}
	if settings, err = settings.Normalised(); err != nil {
		return nil, err
	}
	// Whoever saves is whom a sender who is not a member writes through: the person who decided
	// that such senders may write at all.
	settings.OpenedBy = actor.AccountID
	now := w.Clock.Now()
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, id := range placesOf(settings) {
			collection, err := w.Containers.Find(ctx, id)
			if err != nil || collection.ParentID.IsZero() {
				return shared.ErrValidation.WithDetail("mail.place_not_found").
					WithFields(shared.FieldError{Path: "/default_collection_id", Code: "mail.place_not_found"})
			}
		}
		return w.Settings.Save(ctx, settings, actor.AccountID, now)
	})
	if err != nil {
		return nil, err
	}
	if w.Audit != nil {
		_ = w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: now, Action: settingsSavedAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: settingsTarget, TargetID: actor.TenantID,
			Changes: audit.Changes(
				audit.Change{Field: "enabled", Classification: audit.Open, To: boolWord(settings.Enabled)},
				audit.Change{Field: "name", Classification: audit.Open, To: settings.Name},
				audit.Change{Field: "sender_level", Classification: audit.Open, To: string(settings.Senders.Level)},
			),
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}
	return w.output(settings), nil
}

func (h ListMailReceipts) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	w := h.Writer
	if err := w.authorizeIntake(ctx, actor, receiptsReadAction); err != nil {
		return nil, err
	}
	var receipts []domain.Receipt
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Receipts.Recent(ctx, receiptsShown)
		receipts = found
		return err
	})
	if err != nil {
		return nil, err
	}
	rows := make([]any, 0, len(receipts))
	for _, receipt := range receipts {
		row := map[string]any{
			"id": receipt.ID.String(), "received_at": receipt.ReceivedAt, "sender": receipt.Sender,
			"subject": receipt.Subject, "outcome": string(receipt.Outcome), "door": receipt.Door,
		}
		if receipt.Reason != domain.ReasonNone {
			row["reason"] = string(receipt.Reason)
		}
		if !receipt.ItemID.IsZero() {
			row["item_id"] = receipt.ItemID.String()
		}
		if !receipt.EntryID.IsZero() {
			row["entry_id"] = receipt.EntryID.String()
		}
		rows = append(rows, row)
	}
	return usecase.Output{"receipts": rows}, nil
}

func (h SetCollectionHandle) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	w := h.Writer
	id, err := in.ID("collection_id")
	if err != nil || id.IsZero() {
		return nil, shared.ErrValidation.WithDetail("mail.place_required").
			WithFields(shared.FieldError{Path: "/collection_id", Code: "mail.place_required"})
	}
	handle := strings.ToLower(strings.TrimSpace(in.String("handle")))
	if !domain.ValidHandle(handle) {
		return nil, shared.ErrValidation.WithDetail("mail.handle_invalid").
			WithFields(shared.FieldError{Path: "/handle", Code: "mail.handle_invalid"})
	}
	var before string
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		collection, err := w.Containers.Find(ctx, id)
		if err != nil {
			return err
		}
		if collection.ParentID.IsZero() {
			return shared.ErrNotFound.WithDetail("containers.not_found")
		}
		if err := w.Authorizer.Authorize(ctx, actor, access.Request{
			Permission: service.PermissionStructure, Path: service.ContainerScopes(collection),
			Action: handleSetAction, TokenScope: containersWrite, TargetType: containerTarget, TargetID: id,
		}); err != nil {
			return err
		}
		before = collection.Handle
		if before == handle {
			return nil
		}
		return w.Places.SetHandle(ctx, id, handle)
	})
	if err != nil {
		return nil, err
	}
	if w.Audit != nil && before != handle {
		_ = w.Audit.Append(ctx, audit.Entry{
			TenantID: actor.TenantID, OccurredAt: w.Clock.Now(), Action: handleSetAction,
			Outcome: audit.OutcomeSuccess, Severity: audit.SeverityNotice,
			ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
			TargetType: containerTarget, TargetID: id,
			Changes: audit.Changes(audit.Change{Field: "handle", Classification: audit.Open, From: before, To: handle}),
			Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
		})
	}
	return usecase.Output{"collection_id": id.String(), "handle": handle}, nil
}

// output is the settings with what they make: the workspace's address on the operator's domain.
func (w Writer) output(settings domain.Settings) usecase.Output {
	routes := make([]any, 0, len(settings.SenderRoutes))
	for _, route := range settings.SenderRoutes {
		routes = append(routes, map[string]any{"match": route.Match, "collection_id": route.CollectionID.String()})
	}
	out := usecase.Output{
		"enabled": settings.Enabled,
		"senders": map[string]any{
			"level":   string(settings.Senders.Level),
			"allowed": stringsOf(settings.Senders.Allowed),
			"blocked": stringsOf(settings.Senders.Blocked),
		},
		"sender_routes": routes,
	}
	if settings.Name != "" {
		out["name"] = settings.Name
	}
	if !settings.DefaultCollectionID.IsZero() {
		out["default_collection_id"] = settings.DefaultCollectionID.String()
	}
	if w.Domain != "" {
		out["domain"] = w.Domain
		if settings.Name != "" {
			out["address"] = settings.Name + "@" + w.Domain
		}
	}
	return out
}

func settingsOf(in usecase.Input) (domain.Settings, error) {
	settings := domain.Settings{Enabled: in.Bool("enabled"), Name: in.String("name")}
	var err error
	if settings.DefaultCollectionID, err = in.ID("default_collection_id"); err != nil {
		return domain.Settings{}, err
	}
	senders, _ := in["senders"].(map[string]any)
	level, _ := senders["level"].(string)
	settings.Senders = domain.Senders{
		Level: domain.Level(level), Allowed: listOf(senders["allowed"]), Blocked: listOf(senders["blocked"]),
	}
	rows, _ := in["sender_routes"].([]any)
	for index, raw := range rows {
		row, _ := raw.(map[string]any)
		route := domain.SenderRoute{}
		route.Match, _ = row["match"].(string)
		if text, _ := row["collection_id"].(string); text != "" {
			if route.CollectionID, err = shared.ParseID(text); err != nil {
				return domain.Settings{}, shared.ErrValidation.WithDetail("mail.settings_invalid").
					WithFields(shared.FieldError{Path: "/sender_routes/" + strconv.Itoa(index) + "/collection_id", Code: "mail.route_without_place"})
			}
		}
		settings.SenderRoutes = append(settings.SenderRoutes, route)
	}
	return settings, nil
}

// placesOf is every collection the settings send mail to, each once.
func placesOf(settings domain.Settings) []shared.ID {
	var ids []shared.ID
	seen := map[shared.ID]bool{}
	add := func(id shared.ID) {
		if !id.IsZero() && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(settings.DefaultCollectionID)
	for _, route := range settings.SenderRoutes {
		add(route.CollectionID)
	}
	return ids
}

func listOf(value any) []string {
	rows, _ := value.([]any)
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		if text, ok := row.(string); ok {
			out = append(out, text)
		}
	}
	return out
}

func stringsOf(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func boolWord(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
