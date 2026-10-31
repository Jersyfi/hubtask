// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package servicedesk

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/servicedesk"
	settingrepo "github.com/Jersyfi/hubtask/core/application/repository/setting"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/module"
	domain "github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/setting"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	workdomain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// The use case names and trail codes of a collection's public form (UC-SVC-13).
const (
	CreatePublicFormName      = "CreatePublicForm"
	ReadPublicFormName        = "ReadPublicForm"
	DeletePublicFormName      = "DeletePublicForm"
	ReadPublicFormsPolicyName = "ReadPublicFormsPolicy"
	SetPublicFormsPolicyName  = "SetPublicFormsPolicy"

	PublicFormCreatedAction      audit.Action = "public_form.created"
	PublicFormReadAction         audit.Action = "public_form.read"
	PublicFormDeletedAction      audit.Action = "public_form.deleted"
	PublicFormsPolicyReadAction  audit.Action = "public_forms.policy_read"
	PublicFormsPolicySetAction   audit.Action = "public_forms.policy_set"
	publicFormTarget                          = "public_form"
	publicFormsPolicyTarget                   = "public_forms"
	publicFormsPrefix                         = "servicedesk."
	publicSubmissionNotesDivider              = "\n\n"
)

// ModuleStates answers the modules on at a place (setting.Modules): the unauthenticated route
// passes no catalogue gate, so it asks itself.
type ModuleStates interface {
	On(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) (module.Set, error)
}

// Permitter answers a question of rights without recording a refusal.
type Permitter interface {
	Permits(ctx context.Context, actor appshared.ActorContext, request access.Request) (bool, error)
}

// RequesterContacts is the record of requesters without an account (REQUESTER, accepted.md B).
type RequesterContacts interface {
	RecordContact(ctx context.Context, address, name string, itemID shared.ID, at time.Time) error
}

// ChallengeSigner is the keyed hash a proof-of-work challenge is signed with; its key is the
// adapter's (security.md §8).
type ChallengeSigner interface {
	Hash(presented string) []byte
}

// PublicForms is what a collection's public form needs beside the portal. A nil Links offers
// none at all.
type PublicForms struct {
	Portal     Portal
	Links      repository.PublicForms
	Settings   settingrepo.Settings
	Entropy    clock.Entropy
	Accounts   identityrepo.Accounts
	Modules    ModuleStates
	Permits    Permitter
	Requesters RequesterContacts
	Signer     ChallengeSigner
	// Spent remembers the challenges answered in this process, so one solved challenge files one
	// request; nil remembers none and leaves the per-address limit as the only bound.
	Spent *SpentChallenges
}

// SpentChallenges is the set of challenges this process took, kept until they expire. Per process
// as the rate limiter is: a replica that never saw a challenge takes it once more, which the
// per-address limit bounds.
type SpentChallenges struct {
	mutex sync.Mutex
	until map[string]time.Time
}

// NewSpentChallenges starts an empty set.
func NewSpentChallenges() *SpentChallenges {
	return &SpentChallenges{until: map[string]time.Time{}}
}

// take reserves a challenge; false when it was taken already.
func (s *SpentChallenges) take(key string, until, now time.Time) bool {
	if s == nil {
		return true
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	for each, expires := range s.until {
		if !now.Before(expires) {
			delete(s.until, each)
		}
	}
	if _, taken := s.until[key]; taken {
		return false
	}
	s.until[key] = until
	return true
}

// release gives a reserved challenge back: a request refused for what it said may be corrected
// and sent again with the same work.
func (s *SpentChallenges) release(key string) {
	if s == nil {
		return
	}
	s.mutex.Lock()
	defer s.mutex.Unlock()
	delete(s.until, key)
}

// allowed resolves the workspace's switch inside the caller's transaction; without a settings
// store it is off.
func (f PublicForms) allowed(ctx context.Context, tenant shared.ID) (setting.Resolution, error) {
	definition := domain.PublicFormsDefinition()
	if f.Settings == nil {
		return setting.Resolve(definition), nil
	}
	instance, plan, err := f.Settings.Above(ctx, publicFormsPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	workspace := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: tenant}
	stored, err := f.Settings.At(ctx, []settingrepo.Place{workspace}, publicFormsPrefix)
	if err != nil {
		return setting.Resolution{}, err
	}
	layers := append([]setting.Layer{instance, plan}, settingrepo.LayersOf([]settingrepo.Place{workspace}, stored)...)
	return setting.Resolve(definition, layers...), nil
}

func (f PublicForms) administer(ctx context.Context, actor appshared.ActorContext, action audit.Action, scope string) error {
	return f.Portal.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers, Path: []identity.Scope{identity.TenantScope()},
		Action: action, TokenScope: scope, TargetType: publicFormsPolicyTarget, TargetID: actor.TenantID,
	})
}

func publicFormsPolicyOutput(r setting.Resolution) usecase.Output {
	out := usecase.Output{"allowed": domain.PublicFormsOf(r), "source": string(r.Source)}
	if r.Locked() {
		out["locked_by"] = string(r.LockedBy)
	}
	return out
}

// ReadPublicFormsPolicy answers whether the workspace lets a collection take requests through a
// public form, the level that decided it and the level that locked it.
type ReadPublicFormsPolicy struct{ Forms PublicForms }

// Descriptor is the catalogue entry.
func (h ReadPublicFormsPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPublicFormsPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Answers whether the workspace lets a collection take requests through a public " +
			"form without signing in, the level that decided it and the level that locked it.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Audit: usecase.AuditDeclaration{Action: PublicFormsPolicyReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
			if err := h.Forms.administer(ctx, actor, PublicFormsPolicyReadAction, deskRead); err != nil {
				return nil, err
			}
			var r setting.Resolution
			err := h.Forms.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				r, err = h.Forms.allowed(ctx, actor.TenantID)
				return err
			})
			if err != nil {
				return nil, err
			}
			return publicFormsPolicyOutput(r), nil
		}),
	}, publicFormsPolicyTarget)
}

// SetPublicFormsPolicy switches public forms on or off for the workspace, inside what the
// installation and the plan leave. Switched off, every public form stops answering; the links stay
// and answer again when it is switched back on.
type SetPublicFormsPolicy struct{ Forms PublicForms }

// Execute switches.
func (h SetPublicFormsPolicy) Execute(ctx context.Context, actor appshared.ActorContext, allowed bool) (setting.Resolution, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return setting.Resolution{}, shared.ErrForbidden.WithDetail("public_forms.agent_decides_policy_not")
	}
	if err := h.Forms.administer(ctx, actor, PublicFormsPolicySetAction, deskWrite); err != nil {
		return setting.Resolution{}, err
	}
	if h.Forms.Settings == nil {
		return setting.Resolution{}, shared.ErrUnavailable.WithDetail("public_forms.unavailable")
	}
	desk := h.Forms.Portal.Desk
	definition := domain.PublicFormsDefinition()
	now := desk.Clock.Now()
	var r setting.Resolution
	err := desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		instance, plan, err := h.Forms.Settings.Above(ctx, publicFormsPrefix)
		if err != nil {
			return err
		}
		entry := setting.Entry{Value: allowed, Hold: setting.HoldSet, By: actor.AccountID}
		if err := setting.Check(definition, setting.LevelWorkspace, setting.Resolve(definition, instance, plan), entry); err != nil {
			return err
		}
		place := settingrepo.Place{Level: setting.LevelWorkspace, ScopeID: actor.TenantID}
		if err := h.Forms.Settings.Put(ctx, desk.IDs.NewID(), place, domain.PublicFormsKey, entry, now); err != nil {
			return err
		}
		if r, err = h.Forms.allowed(ctx, actor.TenantID); err != nil {
			return err
		}
		return desk.record(ctx, actor, PublicFormsPolicySetAction, publicFormsPolicyTarget, actor.TenantID, now,
			audit.Change{Field: string(domain.PublicFormsKey), Classification: audit.Open, To: strconv.FormatBool(allowed)})
	})
	return r, err
}

// Descriptor is the catalogue entry.
func (h SetPublicFormsPolicy) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: SetPublicFormsPolicyName, Permission: service.PermissionManageMembers,
		Summary: "Switches whether a collection of the workspace may take requests through a public " +
			"form without signing in. Off by default; the installation or the plan may lock it. " +
			"Switched off, every public form stops answering until it is switched on again.",
		SideEffects: "Stores the setting and writes an audit entry.",
		TokenScope:  deskWrite,
		Input: []usecase.Field{
			{Name: "allowed", Kind: usecase.KindBool, Required: true, Description: "Whether public forms are allowed."},
		},
		Audit: usecase.AuditDeclaration{Action: PublicFormsPolicySetAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			r, err := h.Execute(ctx, actor, in.Bool("allowed"))
			if err != nil {
				return nil, err
			}
			return publicFormsPolicyOutput(r), nil
		}),
	}, publicFormsPolicyTarget)
}

// MintedPublicForm is a public form with its token, answered once: only the hash is kept.
type MintedPublicForm struct {
	Form  domain.PublicForm
	Token integration.FormLinkToken
}

// collection reads a collection and asks MANAGE_MEMBERS there: publishing a door to the world is
// sharing the collection, as a page link is sharing its page.
func (f PublicForms) collection(
	ctx context.Context, actor appshared.ActorContext, collectionID shared.ID, action audit.Action, scope string,
) (workdomain.Container, error) {
	if collectionID.IsZero() {
		return workdomain.Container{}, malformed("/collection_id", "requests.collection_required")
	}
	var collection workdomain.Container
	err := f.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := f.Portal.Containers.Find(ctx, collectionID)
		collection = found
		return err
	})
	if errors.Is(err, shared.ErrNotFound) || (err == nil && (collection.Type != workdomain.ContainerCollection || collection.DeletedAt != nil)) {
		return workdomain.Container{}, appshared.ContainerNotFound(collectionID)
	}
	if err != nil {
		return workdomain.Container{}, err
	}
	err = f.Portal.Desk.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers, Path: service.ContainerScopes(collection),
		Action: action, TokenScope: scope, TargetType: publicFormTarget, TargetID: collectionID,
	})
	return collection, err
}

// CreatePublicForm makes a collection's public form, replacing the one it had: who manages the
// collection's members makes it, where the workspace allows public forms.
type CreatePublicForm struct{ Forms PublicForms }

// Execute makes the form's link.
func (h CreatePublicForm) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) (MintedPublicForm, error) {
	if actor.Kind == appshared.ActorAIAgent || !actor.OnBehalfOf.IsZero() {
		return MintedPublicForm{}, shared.ErrForbidden.WithDetail("public_forms.agent_decides_policy_not")
	}
	if h.Forms.Links == nil || h.Forms.Entropy == nil {
		return MintedPublicForm{}, shared.ErrUnavailable.WithDetail("public_forms.unavailable")
	}
	collection, err := h.Forms.collection(ctx, actor, collectionID, PublicFormCreatedAction, deskWrite)
	if err != nil {
		return MintedPublicForm{}, err
	}
	secret, err := h.Forms.Entropy.Bytes(integration.FormLinkTokenSecretBytes)
	if err != nil {
		return MintedPublicForm{}, shared.ErrInternal.WithDetail("public_forms.link_unmintable").WithCause(err)
	}
	token, err := integration.NewFormLinkToken(actor.TenantID, secret)
	if err != nil {
		return MintedPublicForm{}, err
	}
	desk := h.Forms.Portal.Desk
	now := desk.Clock.Now()
	var form domain.PublicForm
	err = desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		r, err := h.Forms.allowed(ctx, actor.TenantID)
		if err != nil {
			return err
		}
		if !domain.PublicFormsOf(r) {
			return shared.ErrForbidden.WithDetail("public_forms.off")
		}
		if form, err = domain.NewPublicForm(actor.TenantID, collection.ID, actor.AccountID, now); err != nil {
			return err
		}
		if err := h.Forms.Links.Put(ctx, form, token); err != nil {
			return err
		}
		return desk.record(ctx, actor, PublicFormCreatedAction, publicFormTarget, collection.ID, now)
	})
	if err != nil {
		return MintedPublicForm{}, err
	}
	return MintedPublicForm{Form: form, Token: token}, nil
}

// PublicFormOutput is a form's link without its token (api/openapi.yaml, schema PublicForm).
func PublicFormOutput(form domain.PublicForm) usecase.Output {
	return usecase.Output{
		"collection_id": form.CollectionID.String(), "created_by": form.CreatedBy.String(), "created_at": form.CreatedAt.UTC(),
	}
}

// Descriptor is the catalogue entry.
func (h CreatePublicForm) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: CreatePublicFormName, Permission: service.PermissionManageMembers,
		Summary: "Makes a collection's public form and answers its token, once: whoever holds the " +
			"link raises a request of a type whose form opens the PUBLIC channel, without signing " +
			"in. A new link replaces the collection's previous one. Only where the workspace allows " +
			"public forms. A request is filed as the person who made the link: once they may no " +
			"longer create entries there, the form answers nothing.",
		SideEffects: "Writes the link's hash and an audit entry; the previous link stops answering.",
		TokenScope:  deskWrite,
		Input:       []usecase.Field{{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."}},
		Audit:       usecase.AuditDeclaration{Action: PublicFormCreatedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			minted, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			out := PublicFormOutput(minted.Form)
			out["token"] = minted.Token.Secret()
			return out, nil
		}),
	}, publicFormTarget)
}

// ReadPublicForm answers whether a collection has a public form, who made it and when; never its
// token.
type ReadPublicForm struct{ Forms PublicForms }

// Execute answers the link.
func (h ReadPublicForm) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) (domain.PublicForm, error) {
	if h.Forms.Links == nil {
		return domain.PublicForm{}, linkGone()
	}
	if _, err := h.Forms.collection(ctx, actor, collectionID, PublicFormReadAction, deskRead); err != nil {
		return domain.PublicForm{}, err
	}
	var (
		form  domain.PublicForm
		found bool
	)
	err := h.Forms.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		form, found, err = h.Forms.Links.Find(ctx, collectionID)
		return err
	})
	if err == nil && !found {
		err = linkGone()
	}
	return form, err
}

// Descriptor is the catalogue entry.
func (h ReadPublicForm) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: ReadPublicFormName, Permission: service.PermissionManageMembers,
		Summary:     "Answers a collection's public form - who made it and when, never its token - or that it has none.",
		SideEffects: "None. Reads only.",
		TokenScope:  deskRead, ReadOnly: true,
		Input: []usecase.Field{{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."}},
		Audit: usecase.AuditDeclaration{Action: PublicFormReadAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			form, err := h.Execute(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			return PublicFormOutput(form), nil
		}),
	}, publicFormTarget)
}

// DeletePublicForm takes a collection's public form away.
type DeletePublicForm struct{ Forms PublicForms }

// Execute takes it away.
func (h DeletePublicForm) Execute(ctx context.Context, actor appshared.ActorContext, collectionID shared.ID) error {
	if h.Forms.Links == nil {
		return linkGone()
	}
	if _, err := h.Forms.collection(ctx, actor, collectionID, PublicFormDeletedAction, deskWrite); err != nil {
		return err
	}
	desk := h.Forms.Portal.Desk
	now := desk.Clock.Now()
	return desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		removed, err := h.Forms.Links.Delete(ctx, collectionID)
		if err != nil {
			return err
		}
		if !removed {
			return linkGone()
		}
		return desk.record(ctx, actor, PublicFormDeletedAction, publicFormTarget, collectionID, now)
	})
}

// Descriptor is the catalogue entry.
func (h DeletePublicForm) Descriptor() usecase.Descriptor {
	return descriptor(usecase.Descriptor{
		Name: DeletePublicFormName, Permission: service.PermissionManageMembers,
		Summary:     "Takes a collection's public form away; its link answers nothing from then on.",
		SideEffects: "Removes the link and writes an audit entry.",
		TokenScope:  deskWrite,
		Destructive: true,
		Input:       []usecase.Field{{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."}},
		Audit:       usecase.AuditDeclaration{Action: PublicFormDeletedAction},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("collection_id")
			if err != nil {
				return nil, err
			}
			return nil, h.Execute(ctx, actor, id)
		}),
	}, publicFormTarget)
}

// linkGone is the one answer every refusal on the public route gives about the link (T-21): an
// unknown token, a form taken away, switched off, its collection gone, its maker gone or no longer
// allowed to create there.
func linkGone() error { return shared.ErrNotFound.WithDetail("public_forms.link_not_found") }

// openForm is a public form as it stands now: its collection, the person it files as, and what
// it offers.
type openForm struct {
	collection workdomain.Container
	maker      appshared.ActorContext
	catalogue  []domain.CatalogueEntry
}

// open evaluates a link now. Every way it can be dead answers linkGone.
func (f PublicForms) open(ctx context.Context, token integration.FormLinkToken) (openForm, error) {
	if token.IsZero() || f.Links == nil || f.Accounts == nil {
		return openForm{}, linkGone()
	}
	var out openForm
	scope := persistence.Scope{TenantID: token.TenantID()}
	err := f.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		link, err := f.Links.FindByToken(ctx, token)
		if err != nil {
			return err
		}
		r, err := f.allowed(ctx, token.TenantID())
		if err != nil {
			return err
		}
		if !domain.PublicFormsOf(r) {
			return linkGone()
		}
		collection, err := f.Portal.Containers.Find(ctx, link.CollectionID)
		if err != nil {
			return err
		}
		if collection.Type != workdomain.ContainerCollection || collection.DeletedAt != nil ||
			collection.ArchivedAt != nil || collection.ParentArchivedAt != nil {
			return linkGone()
		}
		account, err := f.Accounts.Find(ctx, link.CreatedBy)
		if err != nil {
			return err
		}
		if err := account.Verify(); err != nil {
			return linkGone()
		}
		out.collection = collection
		out.maker = appshared.ActorContext{
			Kind: appshared.ActorUser, TenantID: token.TenantID(), AccountID: account.ID,
			AccountName: account.DisplayName, Locale: account.Locale, TimeZone: account.TimeZone,
			// The one act a submission performs, and nothing else.
			Scopes:  []string{itemsWriteScope},
			Channel: string(workdomain.ChannelPublic),
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return openForm{}, linkGone()
		}
		return openForm{}, err
	}
	if f.Modules != nil {
		on, err := f.Modules.On(ctx, out.maker, out.collection.ParentID)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return openForm{}, err
		}
		if !on.Has(module.ServiceDesk) {
			return openForm{}, linkGone()
		}
	}
	if f.Permits != nil {
		creates, err := f.Permits.Permits(ctx, out.maker, access.Request{
			Permission: service.PermissionWriteItems, Path: service.ContainerScopes(out.collection),
			TokenScope: itemsWriteScope, TargetType: publicFormTarget, TargetID: out.collection.ID,
			On: access.ItemSubject{Does: service.ItemCreate},
		})
		if err != nil {
			return openForm{}, err
		}
		if !creates {
			return openForm{}, linkGone()
		}
	}
	catalogue, err := f.catalogue(ctx, out.maker, out.collection)
	if err != nil {
		return openForm{}, err
	}
	if len(catalogue) == 0 {
		return openForm{}, linkGone()
	}
	out.catalogue = catalogue
	return out, nil
}

func (f PublicForms) catalogue(ctx context.Context, actor appshared.ActorContext, collection workdomain.Container) ([]domain.CatalogueEntry, error) {
	var catalogue []domain.CatalogueEntry
	err := f.Portal.Desk.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		rows, err := f.Portal.EntryTypes.List(ctx)
		if err != nil {
			return err
		}
		offered, err := f.Portal.Gate.OfferedTaskTypes(ctx, collection)
		if err != nil {
			return err
		}
		var definitions []workdomain.CustomFieldDefinition
		if f.Portal.Fields != nil {
			if definitions, err = f.Portal.Fields.ListInScope(ctx, collection.ID); err != nil {
				return err
			}
		}
		catalogue = domain.CatalogueThrough(workdomain.ChannelPublic, workdomain.NewEntryTypes(rows), offered, definitions)
		return nil
	})
	return catalogue, err
}

// PublicFormView is what a public form shows: the kinds of request it takes, and the challenge a
// submission answers.
type PublicFormView struct {
	Catalogue []domain.CatalogueEntry
	Challenge domain.Challenge
}

// OpenPublicForm answers what a public form shows. Not a use case and not in the catalogue, for
// the page link's reason: it answers a credential nobody in the system holds. The tenant comes
// from the token alone, and every question is asked here (ADR-0005).
type OpenPublicForm struct{ Forms PublicForms }

// Execute answers the form, or that there is none.
func (h OpenPublicForm) Execute(ctx context.Context, token integration.FormLinkToken) (PublicFormView, error) {
	opened, err := h.Forms.open(ctx, token)
	if err != nil {
		return PublicFormView{}, err
	}
	challenge, err := h.Forms.challenge(token)
	if err != nil {
		return PublicFormView{}, err
	}
	return PublicFormView{Catalogue: opened.catalogue, Challenge: challenge}, nil
}

func (f PublicForms) challenge(token integration.FormLinkToken) (domain.Challenge, error) {
	if f.Signer == nil || f.Entropy == nil {
		return domain.Challenge{}, shared.ErrUnavailable.WithDetail("public_forms.unavailable")
	}
	nonce, err := f.Entropy.Bytes(domain.ChallengeNonceBytes)
	if err != nil {
		return domain.Challenge{}, shared.ErrInternal.WithDetail("public_forms.challenge_unmintable").WithCause(err)
	}
	return domain.IssueChallenge(token.Secret(), nonce, f.Portal.Desk.Clock.Now(), f.Signer.Hash)
}

// PublicSubmission is what a visitor sends.
type PublicSubmission struct {
	EntryType      string
	Title          string
	Notes          string
	CustomFields   map[string]any
	RequesterName  string
	RequesterEmail string
	Challenge      string
	Solution       string
}

// SubmitPublicForm files a visitor's request (UC-SVC-13): the work is checked before anything is
// read, the request is created through CreateWorkItem on the PUBLIC door as the link's maker on
// the visitor's behalf - held to the type's form there -, and the visitor's address is recorded
// as a requester's. Not in the catalogue, OpenPublicForm's reason.
//
// It answers the same whoever the address belongs to: no account is looked up by it (P-01), and
// nothing is mailed to it on submission, so a stranger's address cannot be made to receive mail.
type SubmitPublicForm struct{ Forms PublicForms }

// Execute files the request.
func (h SubmitPublicForm) Execute(ctx context.Context, token integration.FormLinkToken, in PublicSubmission) error {
	if h.Forms.Signer == nil {
		return shared.ErrUnavailable.WithDetail("public_forms.unavailable")
	}
	now := h.Forms.Portal.Desk.Clock.Now()
	nonce, err := domain.VerifyWork(token.Secret(), in.Challenge, in.Solution, now, h.Forms.Signer.Hash)
	if err != nil {
		return err
	}
	if !h.Forms.Spent.take(nonce, now.Add(domain.PublicFormChallengeLife), now) {
		return shared.ErrValidation.WithDetail("public_forms.work_spent").
			WithFields(shared.FieldError{Path: "/challenge", Code: "public_forms.work_spent"})
	}
	if err := h.file(ctx, token, in, now); err != nil {
		if !errors.Is(err, shared.ErrNotFound) {
			h.Forms.Spent.release(nonce)
		}
		return err
	}
	return nil
}

func (h SubmitPublicForm) file(ctx context.Context, token integration.FormLinkToken, in PublicSubmission, now time.Time) error {
	requester, err := domain.NewRequester(in.RequesterName, in.RequesterEmail)
	if err != nil {
		return err
	}
	opened, err := h.Forms.open(ctx, token)
	if err != nil {
		return err
	}
	if !domain.InCatalogue(opened.catalogue, in.EntryType) {
		return malformed("/entry_type", "requests.type_not_offered")
	}
	notes := strings.TrimSpace(in.Notes)
	if !requester.IsZero() {
		notes = strings.TrimSpace(requester.FromLine() + publicSubmissionNotesDivider + notes)
	}
	input := usecase.Input{
		"type": string(workdomain.ItemTask), "collection_id": opened.collection.ID.String(),
		"entry_type": in.EntryType, "title": in.Title,
	}
	if notes != "" {
		input["notes"] = notes
	}
	if len(in.CustomFields) > 0 {
		input["custom_fields"] = in.CustomFields
	}
	actor := opened.maker
	actor.ForSender = !requester.IsZero()
	err = h.Forms.Portal.Desk.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		out, err := h.Forms.Portal.Entries.Invoke(ctx, createEntryUseCase, actor, input)
		if err != nil {
			return err
		}
		if requester.IsZero() || h.Forms.Requesters == nil {
			return nil
		}
		itemID, err := shared.ParseID(out.String("id"))
		if err != nil {
			return shared.ErrInternal.WithDetail("public_forms.entry_incomplete").WithCause(err)
		}
		// In the act's transaction: an entry filed for somebody is never without the record that
		// says whose it is and when it goes.
		return h.Forms.Requesters.RecordContact(ctx, requester.Address, requester.Name, itemID, now)
	})
	if errors.Is(err, shared.ErrForbidden) {
		// The maker may no longer create there: to the visitor the form is simply gone.
		return linkGone()
	}
	return err
}
