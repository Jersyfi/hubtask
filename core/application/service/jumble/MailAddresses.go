// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"

	repository "github.com/Jersyfi/hubtask/core/application/repository/jumble"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

const (
	ShowMailAddressName      = "ShowMailAddress"
	RotateMailAddressName    = "RotateMailAddress"
	SetMailSenderPolicyName  = "SetMailSenderPolicy"
	containersWriteScope     = "containers:write"
	mailAddressTarget        = "mail_address"
	mailAddressRotatedAction = audit.Action("mail.address_rotated")
	mailPolicyChangedAction  = audit.Action("mail.sender_policy_changed")
	mailAddressShownAction   = audit.Action("mail.address_shown")
)

// PlaceFinder reads one place in the transaction's tenant - MailPlaces in production.
type PlaceFinder interface {
	Find(ctx context.Context, target domain.MailTarget) (MailPlace, error)
}

// MailPlaces reads the place an address leads to: its name, the path its permissions are resolved
// along, and whether it takes anything (ADR-0092 decision 4).
type MailPlaces struct {
	Items      workrepo.Items
	Containers workrepo.Containers
}

// MailPlace is one place, as the mail side needs it.
type MailPlace struct {
	Target domain.MailTarget
	// Name is the collection's name or the entry's title; empty for the jumble.
	Name string
	// Path is where a membership is resolved along; the tenant for the jumble.
	Path []identity.Scope
	// Closed is a place that takes nothing: archived or in the trash, itself or its collection.
	Closed bool
	// CollectionID is the collection an entry lives in, or the collection itself.
	CollectionID shared.ID
}

// Find reads one place in the transaction's tenant. A place that does not exist is the same
// not-found every unknown address gets.
func (p MailPlaces) Find(ctx context.Context, target domain.MailTarget) (MailPlace, error) {
	switch target.Kind {
	case domain.TargetJumble:
		return MailPlace{Target: target, Path: []identity.Scope{identity.TenantScope()}}, nil
	case domain.TargetCollection:
		collection, err := p.Containers.Find(ctx, target.ID)
		if err != nil || collection.ParentID.IsZero() {
			return MailPlace{}, placeGone(err)
		}
		return MailPlace{
			Target: target, Name: collection.Name, Path: service.ContainerScopes(collection),
			Closed:       collection.IsTrashed() || collection.IsEffectivelyArchived(),
			CollectionID: collection.ID,
		}, nil
	case domain.TargetItem:
		item, err := p.Items.Find(ctx, target.ID)
		if err != nil {
			return MailPlace{}, placeGone(err)
		}
		collection, err := p.Containers.Find(ctx, item.CollectionID)
		if err != nil {
			return MailPlace{}, placeGone(err)
		}
		return MailPlace{
			Target: target, Name: item.Title, Path: service.ContainerScopes(collection),
			Closed: item.IsTrashed() || item.IsArchived() || collection.IsTrashed() ||
				collection.IsEffectivelyArchived(),
			CollectionID: collection.ID,
		}, nil
	default:
		return MailPlace{}, mailGone()
	}
}

// placeGone answers a missing place as an unknown address, and passes anything else on.
func placeGone(err error) error {
	if err == nil || errors.Is(err, shared.ErrNotFound) {
		return mailGone()
	}
	return err
}

// mailGone is the one refusal every address that leads nowhere gets (T-21).
func mailGone() error { return shared.ErrNotFound.WithDetail("mail.address_not_found") }

// MailAddressWriter is what the three address use cases share.
type MailAddressWriter struct {
	Addresses  repository.MailAddresses
	Places     PlaceFinder
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
	// Domain is the installation's receiving domain (HUBTASK_MAIL_INBOUND_DOMAIN). Empty means the
	// installation receives no mail at addresses, and every read says so (ADR-0092 decision 9).
	Domain string
}

// MailAddressView is an address as a person sees it.
type MailAddressView struct {
	// Available is false where the installation has no receiving domain.
	Available bool
	Address   domain.MailAddress
	Text      string
}

// ShowMailAddress answers a place's address, minting it on the first read.
//
// Minted on read rather than by an act of its own, because an address is what a place *is*
// (ADR-0092 decision 1): the person who looks for it should find it, not a button that makes one.
type ShowMailAddress struct{ Writer MailAddressWriter }

// RotateMailAddress replaces a place's address; the old one stops working at once.
type RotateMailAddress struct{ Writer MailAddressWriter }

// SetMailSenderPolicy sets who may write into a place by mail.
type SetMailSenderPolicy struct{ Writer MailAddressWriter }

// Execute answers the address.
func (h ShowMailAddress) Execute(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
) (MailAddressView, error) {
	w := h.Writer
	place, err := w.place(ctx, actor, target)
	if err != nil {
		return MailAddressView{}, err
	}
	if err := w.authorize(ctx, actor, place, false); err != nil {
		return MailAddressView{}, err
	}
	if w.Domain == "" {
		return MailAddressView{}, nil
	}

	var address domain.MailAddress
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Addresses.Live(ctx, target)
		if err == nil {
			address = found
			return nil
		}
		if !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		address, err = w.mint(actor, target)
		if err != nil {
			return err
		}
		return w.Addresses.Replace(ctx, address, address.CreatedAt)
	})
	if errors.Is(err, shared.ErrConflict) {
		// Two first reads raced; the other one's address is the place's now.
		err = w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			address, err = w.Addresses.Live(ctx, target)
			return err
		})
	}
	if err != nil {
		return MailAddressView{}, err
	}
	// Every read is recorded, the first one minting: an address is a key to the place, and who
	// looked at it is what a review after a leak asks.
	w.record(ctx, actor, mailAddressShownAction, address)
	return w.view(address, place), nil
}

// Execute replaces the address.
func (h RotateMailAddress) Execute(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
) (MailAddressView, error) {
	w := h.Writer
	place, err := w.place(ctx, actor, target)
	if err != nil {
		return MailAddressView{}, err
	}
	if err := w.authorize(ctx, actor, place, true); err != nil {
		return MailAddressView{}, err
	}
	if w.Domain == "" {
		return MailAddressView{}, shared.ErrConflict.WithDetail("mail.inbound_domain_missing")
	}

	address, err := w.mint(actor, target)
	if err != nil {
		return MailAddressView{}, err
	}
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		// Who may send survives the new code: replacing a leaked address is not a decision about
		// who may use the next one.
		if previous, err := w.Addresses.Live(ctx, target); err == nil {
			address.Policy, address.OpenedBy = previous.Policy, previous.OpenedBy
		} else if !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		return w.Addresses.Replace(ctx, address, address.CreatedAt)
	})
	if err != nil {
		return MailAddressView{}, err
	}
	w.record(ctx, actor, mailAddressRotatedAction, address)
	return w.view(address, place), nil
}

// Execute sets the policy.
func (h SetMailSenderPolicy) Execute(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
	policy domain.SenderPolicy,
) (MailAddressView, error) {
	w := h.Writer
	place, err := w.place(ctx, actor, target)
	if err != nil {
		return MailAddressView{}, err
	}
	if err := w.authorize(ctx, actor, place, true); err != nil {
		return MailAddressView{}, err
	}
	if w.Domain == "" {
		return MailAddressView{}, shared.ErrConflict.WithDetail("mail.inbound_domain_missing")
	}

	var address domain.MailAddress
	err = w.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Addresses.Live(ctx, target)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			if found, err = w.mint(actor, target); err != nil {
				return err
			}
			if found, err = found.WithPolicy(policy, actor.AccountID); err != nil {
				return err
			}
			address = found
			return w.Addresses.Replace(ctx, address, address.CreatedAt)
		case err != nil:
			return err
		}
		if address, err = found.WithPolicy(policy, actor.AccountID); err != nil {
			return err
		}
		return w.Addresses.SetPolicy(ctx, address)
	})
	if err != nil {
		return MailAddressView{}, err
	}
	w.record(ctx, actor, mailPolicyChangedAction, address)
	return w.view(address, place), nil
}

// place reads the place in the actor's tenant.
func (w MailAddressWriter) place(
	ctx context.Context, actor appshared.ActorContext, target domain.MailTarget,
) (MailPlace, error) {
	var place MailPlace
	err := w.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := w.Places.Find(ctx, target)
		place = found
		return err
	})
	return place, err
}

// authorize asks the place's own question: reading it to see its address, shaping it to change it.
//
// Shaping is the jumble's automation permission (the intake's), a collection's structure
// permission (its settings') and an entry's write permission.
func (w MailAddressWriter) authorize(
	ctx context.Context, actor appshared.ActorContext, place MailPlace, shaping bool,
) error {
	request := access.Request{
		Permission: service.PermissionRead, Path: place.Path, Action: mailAddressShownAction,
		TokenScope: itemsReadScope, TargetType: mailAddressTarget, TargetID: place.Target.ID,
	}
	if shaping {
		request.Action = mailAddressRotatedAction
		switch place.Target.Kind {
		case domain.TargetJumble:
			request.Permission, request.TokenScope = service.PermissionAutomation, automationManageScope
		case domain.TargetCollection:
			request.Permission, request.TokenScope = service.PermissionStructure, containersWriteScope
		default:
			request.Permission, request.TokenScope = service.PermissionWriteItems, itemsWriteScope
		}
	}
	return w.Authorizer.Authorize(ctx, actor, request)
}

// mint draws a new address for the place.
func (w MailAddressWriter) mint(
	actor appshared.ActorContext, target domain.MailTarget,
) (domain.MailAddress, error) {
	secret, err := w.Entropy.Bytes(domain.MailCodeBytes)
	if err != nil {
		return domain.MailAddress{}, shared.ErrUnavailable.WithDetail("mail.code_undrawable")
	}
	return domain.NewMailAddress(
		w.IDs.NewID(), actor.TenantID, target, secret, actor.AccountID, w.Clock.Now())
}

// view is the address as a person reads it.
func (w MailAddressWriter) view(address domain.MailAddress, place MailPlace) MailAddressView {
	label := place.Name
	if place.Target.Kind == domain.TargetJumble {
		label = "jumble"
	}
	return MailAddressView{
		Available: true, Address: address, Text: address.Address(label, w.Domain),
	}
}

// record writes the trail entry: which place's address changed, never the code.
func (w MailAddressWriter) record(
	ctx context.Context, actor appshared.ActorContext, action audit.Action,
	address domain.MailAddress,
) {
	if w.Audit == nil {
		return
	}
	_ = w.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: w.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: severityOf(action),
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: mailAddressTarget, TargetID: address.ID,
		Changes: audit.Changes(
			audit.Change{Field: "target_kind", Classification: audit.Open, To: string(address.Target.Kind)},
			audit.Change{Field: "target_id", Classification: audit.Open, To: address.Target.ID.String()},
			audit.Change{Field: "sender_policy", Classification: audit.Open, To: string(address.Policy)},
		),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// targetOf reads the place a request names.
func targetOf(in usecase.Input) (domain.MailTarget, error) {
	kind := domain.TargetKind(in.String("target_kind"))
	var id shared.ID
	if raw := in.String("target_id"); raw != "" {
		parsed, err := shared.ParseID(raw)
		if err != nil {
			return domain.MailTarget{}, shared.ErrValidation.WithDetail("mail.target_required").
				WithFields(shared.FieldError{Path: "/target_id", Code: "mail.target_required"})
		}
		id = parsed
	}
	return domain.NewMailTarget(kind, id)
}

// mailAddressOutput is the answer every address use case gives.
func mailAddressOutput(view MailAddressView) usecase.Output {
	if !view.Available {
		return usecase.Output{"available": false}
	}
	out := usecase.Output{
		"available":     true,
		"address":       view.Text,
		"target_kind":   string(view.Address.Target.Kind),
		"sender_policy": string(view.Address.Policy),
		"created_at":    view.Address.CreatedAt,
	}
	if !view.Address.Target.ID.IsZero() {
		out["target_id"] = view.Address.Target.ID.String()
	}
	if !view.Address.OpenedBy.IsZero() {
		out["opened_by"] = view.Address.OpenedBy.String()
	}
	return out
}

var mailTargetFields = []usecase.Field{
	{
		Name: "target_kind", Kind: usecase.KindString, Required: true,
		Description: "JUMBLE, COLLECTION or ITEM.",
	},
	{
		Name: "target_id", Kind: usecase.KindID,
		Description: "The collection or the entry; absent for the jumble.",
	},
}

// Descriptor is the catalogue entry.
func (h ShowMailAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ShowMailAddressName,
		Summary: "Answers the address a place takes mail at on the installation's receiving domain, " +
			"minting it on the first read; says unavailable where no receiving domain is set.",
		SideEffects: "Mints the place's address the first time it is read.",
		TokenScope:  itemsReadScope,
		Audit: usecase.AuditDeclaration{
			Action: mailAddressShownAction, TargetType: mailAddressTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An address is about the place's ingress rather than about its content.",
		},
		Input:   mailTargetFields,
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ShowMailAddress) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	target, err := targetOf(in)
	if err != nil {
		return nil, err
	}
	view, err := h.Execute(ctx, actor, target)
	if err != nil {
		return nil, err
	}
	return mailAddressOutput(view), nil
}

// Descriptor is the catalogue entry.
func (h RotateMailAddress) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotateMailAddressName,
		Summary: "Replaces a place's mail address; the old one answers not-found from the same " +
			"instant. Who may send stays as it was.",
		SideEffects: "Revokes the place's address, stores a new one and writes an audit entry.",
		TokenScope:  itemsWriteScope,
		Audit: usecase.AuditDeclaration{
			Action: mailAddressRotatedAction, TargetType: mailAddressTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An address is about the place's ingress rather than about its content.",
		},
		Input:   mailTargetFields,
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RotateMailAddress) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	target, err := targetOf(in)
	if err != nil {
		return nil, err
	}
	view, err := h.Execute(ctx, actor, target)
	if err != nil {
		return nil, err
	}
	return mailAddressOutput(view), nil
}

// Descriptor is the catalogue entry.
func (h SetMailSenderPolicy) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetMailSenderPolicyName,
		Summary: "Sets who may write into a place by mail: MEMBERS (the default; everybody else " +
			"waits in the jumble) or ANYONE (an unknown sender's mail is written as the caller).",
		SideEffects: "Changes the address's sender policy and writes an audit entry.",
		TokenScope:  itemsWriteScope,
		Audit: usecase.AuditDeclaration{
			Action: mailPolicyChangedAction, TargetType: mailAddressTarget,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An address is about the place's ingress rather than about its content.",
		},
		Input: append(append([]usecase.Field{}, mailTargetFields...), usecase.Field{
			Name: "sender_policy", Kind: usecase.KindString, Required: true,
			Description: "MEMBERS or ANYONE.",
		}),
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h SetMailSenderPolicy) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	target, err := targetOf(in)
	if err != nil {
		return nil, err
	}
	view, err := h.Execute(ctx, actor, target, domain.SenderPolicy(in.String("sender_policy")))
	if err != nil {
		return nil, err
	}
	return mailAddressOutput(view), nil
}

// MailReplies answers an entry's address for a notification's Reply-To, minting it where the entry
// has none (ADR-0092 decision 8). The system mints it: nobody asked, and the address is what the
// entry is.
type MailReplies struct{ Writer MailAddressWriter }

// ReplyAddress answers the address, or empty where the installation receives no mail or the entry
// takes none.
func (r MailReplies) ReplyAddress(ctx context.Context, tenantID, itemID shared.ID) (string, error) {
	w := r.Writer
	if w.Domain == "" || tenantID.IsZero() || itemID.IsZero() {
		return "", nil
	}
	target := domain.MailTarget{Kind: domain.TargetItem, ID: itemID}
	system := appshared.ActorContext{Kind: appshared.ActorSystem, TenantID: tenantID}
	var text string
	err := w.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		place, err := w.Places.Find(ctx, target)
		if err != nil || place.Closed {
			return err
		}
		address, err := w.Addresses.Live(ctx, target)
		if errors.Is(err, shared.ErrNotFound) {
			if address, err = w.mint(system, target); err != nil {
				return err
			}
			err = w.Addresses.Replace(ctx, address, address.CreatedAt)
		}
		if err != nil {
			return err
		}
		text = w.view(address, place).Text
		return nil
	})
	if err != nil {
		return "", err
	}
	return text, nil
}

// severityOf is a read's notice and a change's warning.
func severityOf(action audit.Action) audit.Severity {
	if action == mailAddressShownAction {
		return audit.SeverityNotice
	}
	return audit.SeverityWarning
}
