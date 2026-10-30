// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package forge is the code on an entry's use cases (UC-INT-13): a hub's forge hooks - defined,
// changed, switched off and given a new secret under STRUCTURE on the hub - and an entry's commits,
// branches and pull requests, read by whoever reads the entry.
//
// A forge posts; nothing here calls it. A delivery is trusted by the hook's secret - GitHub's and
// Forgejo's HMAC signature of the body, GitLab's token - and is applied as the person who last
// defined the hook, only while they may still write in the hub, and only to the hub's own entries.
package forge

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/forge"
	changelog "github.com/Jersyfi/hubtask/core/application/repository/sync"
	workrepo "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/forge"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
)

// The use case names.
const (
	ListCodeHooksName        = "ListCodeHooks"
	CreateCodeHookName       = "CreateCodeHook"
	UpdateCodeHookName       = "UpdateCodeHook"
	RotateCodeHookSecretName = "RotateCodeHookSecret"
	ListEntryCodeLinksName   = "ListEntryCodeLinks"
)

// The stable audit codes (audit.md §2). A secret never reaches the trail, nor a commit's message
// or a pull request's title (rule 10).
const (
	HooksReadAction     audit.Action = "forge.hooks_read"
	HookCreatedAction   audit.Action = "forge.hook_created"
	HookChangedAction   audit.Action = "forge.hook_changed"
	HookRotatedAction   audit.Action = "forge.hook_secret_rotated"
	HookDeletedAction   audit.Action = "forge.hook_deleted"
	LinksReadAction     audit.Action = "forge.links_read"
	DeliveryTakenAction audit.Action = "forge.delivery_received"
	hookTarget                       = "code_hook"
	itemTarget                       = "work_item"
	hooksRead                        = "containers:read"
	hooksWrite                       = "automation:manage"
	linksRead                        = "items:read"
	linksWrite                       = "items:write"
	exemptReason                     = "a forge hook is the hub's configuration and a code link the forge's record; the audit trail and the link itself are their history."
	maxLinksListed                   = 200
	saltBytes                        = 24
)

// LinkEntity is the change log's entity of a code link, filed under its entry's collection so that
// a device holding the entry holds its code (offline-sync.md §13).
const LinkEntity = "code_link"

// DeliveryMemory is how long a delivery is remembered by its digest, so that one repeated - a
// forge's redelivery, a replay - is refused.
const DeliveryMemory = 72 * time.Hour

// Authorizer records a refusal and answers it.
type Authorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
}

// Keys resolves an entry's key: the collection's handle and the entry's number.
type Keys interface {
	ItemByKey(ctx context.Context, key mail.Key) (shared.ID, error)
}

// Entries reads the summaries of live entries: their hub and collection.
type Entries interface {
	Entries(ctx context.Context, ids []shared.ID) ([]workrepo.EntrySummary, error)
}

// Secrets derives a hook's secret from the installation's key and the hook's salt; it is never
// stored (security.md §5).
type Secrets interface {
	Derive(tenantID, hookID shared.ID, salt string) string
}

// Forge is what every code use case shares.
type Forge struct {
	Store      repository.Store
	Containers workrepo.Containers
	Keys       Keys
	Entries    Entries
	Secrets    Secrets
	Authorizer Authorizer
	Audit      audit.Sink
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
	Entropy    clock.Entropy
	// Changes and HLC tell offline devices of a link written or removed. A nil Changes records
	// nothing, a test's convenience.
	Changes changelog.ChangeLog
	HLC     clock.HLCSource
}

func invalid(path, code string) error {
	return shared.ErrValidation.WithDetail(code).WithFields(shared.FieldError{Path: path, Code: code})
}

func hubPath(hubID shared.ID) []identity.Scope {
	return []identity.Scope{identity.TenantScope(), identity.HubScope(hubID)}
}

func entryPath(s workrepo.EntrySummary) []identity.Scope {
	path := []identity.Scope{identity.TenantScope()}
	if !s.HubID.IsZero() {
		path = append(path, identity.HubScope(s.HubID))
	}
	return append(path, identity.CollectionScope(s.CollectionID), identity.ItemScope(s.ID))
}

func (f Forge) authorizeHub(ctx context.Context, actor appshared.ActorContext, hubID shared.ID, write bool, action audit.Action, id shared.ID) error {
	permission, scope := service.PermissionRead, hooksRead
	if write {
		permission, scope = service.PermissionStructure, hooksWrite
	}
	return f.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: permission, Path: hubPath(hubID), Action: action, TokenScope: scope,
		TargetType: hookTarget, TargetID: id,
	})
}

func (f Forge) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, target string, id shared.ID, changes ...audit.Change) error {
	return f.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: f.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityInfo,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: target, TargetID: id, Changes: audit.Changes(changes...),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// liveHub refuses anything that is not a living hub.
func (f Forge) liveHub(ctx context.Context, actor appshared.ActorContext, hubID shared.ID) error {
	return f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		c, err := f.Containers.Find(ctx, hubID)
		if err != nil {
			return err
		}
		if c.Type != work.ContainerHub || c.DeletedAt != nil {
			return shared.ErrNotFound.WithDetail("forge.hub_not_found")
		}
		return nil
	})
}

func (f Forge) findHook(ctx context.Context, actor appshared.ActorContext, id shared.ID) (domain.Hook, error) {
	var h domain.Hook
	err := f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		h, err = f.Store.FindHook(ctx, id)
		return err
	})
	return h, err
}

func (f Forge) salt() (string, error) {
	b, err := f.Entropy.Bytes(saltBytes)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func optionalTime(t *time.Time) any {
	if t == nil {
		return nil
	}
	return t.UTC().Format(time.RFC3339)
}

// HookOutput is a hook as every read answers it: never its secret, nor its salt.
func HookOutput(h domain.Hook) usecase.Output {
	return usecase.Output{
		"id": h.ID.String(), "hub_id": h.HubID.String(), "kind": string(h.Kind), "name": h.Name,
		"enabled": h.Enabled, "run_as": h.RunAs.String(),
		"created_at": h.CreatedAt.UTC().Format(time.RFC3339), "updated_at": h.UpdatedAt.UTC().Format(time.RFC3339),
		"last_delivery_at": optionalTime(h.LastDeliveryAt),
	}
}

// LinkOutput is a code link as the entry's readers see it, and as the change log carries it.
func LinkOutput(l domain.Link, collectionID shared.ID) usecase.Output {
	return usecase.Output{
		"id": l.ID.String(), "item_id": l.ItemID.String(), "hub_id": l.HubID.String(),
		"collection_id": collectionID.String(), "hook_id": l.HookID.String(), "kind": string(l.Kind),
		"repository": l.Repository, "ref": l.Ref, "title": l.Title, "url": l.URL, "author": l.Author,
		"state": string(l.State), "happened_at": l.HappenedAt.UTC().Format(time.RFC3339),
		"updated_at": l.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func hubInput() usecase.Field {
	return usecase.Field{Name: "hub_id", Kind: usecase.KindID, Required: true, Description: "The hub."}
}

func hookInput() usecase.Field {
	return usecase.Field{Name: "hook_id", Kind: usecase.KindID, Required: true, Description: "The hook."}
}

func kindNames() []string {
	out := make([]string, 0, len(domain.Kinds()))
	for _, k := range domain.Kinds() {
		out = append(out, string(k))
	}
	return out
}

// ListCodeHooks answers a hub's forge hooks.
type ListCodeHooks struct{ Forge Forge }

// Descriptor is the catalogue entry.
func (h ListCodeHooks) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListCodeHooksName, Permission: service.PermissionRead,
		Summary: "Lists the hooks code forges (GitHub, GitLab, Forgejo or Gitea) post a hub's pushes and pull " +
			"requests to, each with its forge, name, whether it is on, who it writes as and its last delivery - " +
			"never its secret.",
		SideEffects: "None. Reads only.", TokenScope: hooksRead, ReadOnly: true,
		Input:    []usecase.Field{hubInput()},
		Audit:    usecase.AuditDeclaration{Action: HooksReadAction, TargetType: hookTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			f := h.Forge
			if err := f.authorizeHub(ctx, actor, hub, false, HooksReadAction, hub); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				list, err := f.Store.ListHooks(ctx, hub)
				for _, hook := range list {
					rows = append(rows, HookOutput(hook))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// CreateCodeHook adds a forge's hook to a hub and answers its secret, once.
type CreateCodeHook struct{ Forge Forge }

// Descriptor is the catalogue entry.
func (h CreateCodeHook) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CreateCodeHookName, Permission: service.PermissionStructure,
		Summary: "Adds a hook a code forge posts a hub's pushes and pull requests to: GitHub, GitLab or Forgejo " +
			"(Gitea too). Commits, branches and pull requests naming an entry's key are then kept on the entry " +
			"with their state. The secret is answered once and never again.",
		SideEffects: "Creates a hook that writes code links as the caller while they may write in the hub.",
		TokenScope:  hooksWrite,
		Input: []usecase.Field{
			hubInput(),
			{Name: "kind", Kind: usecase.KindString, Required: true, Enum: kindNames(), Description: "The forge."},
			{Name: "name", Kind: usecase.KindString, Required: true, Description: "What the hook is called, at most 80 characters."},
		},
		Audit:    usecase.AuditDeclaration{Action: HookCreatedAction, TargetType: hookTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			hub, err := in.ID("hub_id")
			if err != nil {
				return nil, err
			}
			f := h.Forge
			if err := f.authorizeHub(ctx, actor, hub, true, HookCreatedAction, hub); err != nil {
				return nil, err
			}
			kind := domain.Kind(in.String("kind"))
			name, err := domain.CheckHook(kind, in.String("name"))
			if err != nil {
				return nil, err
			}
			if err := f.liveHub(ctx, actor, hub); err != nil {
				return nil, err
			}
			salt, err := f.salt()
			if err != nil {
				return nil, err
			}
			now := f.Clock.Now()
			hook := domain.Hook{
				ID: f.IDs.NewID(), TenantID: actor.TenantID, HubID: hub, Kind: kind, Name: name, Enabled: true,
				Salt: salt, RunAs: actor.AccountID, CreatedAt: now, UpdatedAt: now,
			}
			err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				n, err := f.Store.CountHooks(ctx, hub)
				if err != nil {
					return err
				}
				if n >= domain.MaxHooksPerHub {
					return invalid("/hub_id", "forge.hooks_too_many")
				}
				if err := f.Store.PutHook(ctx, hook); err != nil {
					return err
				}
				return f.record(ctx, actor, HookCreatedAction, hookTarget, hook.ID,
					audit.Change{Field: "kind", Classification: audit.Open, To: string(kind)},
					audit.Change{Field: "hub_id", Classification: audit.Open, To: hub.String()})
			})
			if err != nil {
				return nil, err
			}
			out := HookOutput(hook)
			out["secret"] = f.Secrets.Derive(actor.TenantID, hook.ID, hook.Salt)
			return out, nil
		}),
	}
}

// UpdateCodeHook renames a hook or switches it on or off; whoever changes it is who it writes as.
type UpdateCodeHook struct{ Forge Forge }

// Descriptor is the catalogue entry.
func (h UpdateCodeHook) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: UpdateCodeHookName, Permission: service.PermissionStructure,
		Summary: "Renames a forge hook or switches it on or off. A hook switched off answers its forge as unknown " +
			"and links nothing; the links it wrote stay, and a hook is never removed. Whoever changes it is who it writes as from then on.",
		SideEffects: "Changes the hook.", TokenScope: hooksWrite,
		Input: []usecase.Field{
			hookInput(),
			{Name: "name", Kind: usecase.KindString, Description: "A new name."},
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether deliveries are taken."},
		},
		Audit:    usecase.AuditDeclaration{Action: HookChangedAction, TargetType: hookTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hook_id")
			if err != nil {
				return nil, err
			}
			f := h.Forge
			hook, err := f.findHook(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := f.authorizeHub(ctx, actor, hook.HubID, true, HookChangedAction, id); err != nil {
				return nil, err
			}
			if name := in.OptionalString("name"); name != nil {
				if hook.Name, err = domain.CheckHook(hook.Kind, *name); err != nil {
					return nil, err
				}
			}
			if in.Present("enabled") {
				hook.Enabled = in.Bool("enabled")
			}
			hook.RunAs, hook.UpdatedAt = actor.AccountID, f.Clock.Now()
			err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := f.Store.PutHook(ctx, hook); err != nil {
					return err
				}
				return f.record(ctx, actor, HookChangedAction, hookTarget, hook.ID,
					audit.Change{Field: "enabled", Classification: audit.Open, To: hook.Enabled})
			})
			if err != nil {
				return nil, err
			}
			return HookOutput(hook), nil
		}),
	}
}

// RotateCodeHookSecret draws a hook's new secret and answers it, once; the old one stops at once.
type RotateCodeHookSecret struct{ Forge Forge }

// Descriptor is the catalogue entry.
func (h RotateCodeHookSecret) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RotateCodeHookSecretName, Permission: service.PermissionStructure,
		Summary: "Draws a forge hook's new secret and answers it once. The old secret stops at once, so the " +
			"forge's webhook is given the new one straight after.",
		SideEffects: "Replaces the hook's secret.", TokenScope: hooksWrite,
		Input:    []usecase.Field{hookInput()},
		Audit:    usecase.AuditDeclaration{Action: HookRotatedAction, TargetType: hookTarget, Severity: audit.SeverityNotice},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			id, err := in.ID("hook_id")
			if err != nil {
				return nil, err
			}
			f := h.Forge
			hook, err := f.findHook(ctx, actor, id)
			if err != nil {
				return nil, err
			}
			if err := f.authorizeHub(ctx, actor, hook.HubID, true, HookRotatedAction, id); err != nil {
				return nil, err
			}
			if hook.Salt, err = f.salt(); err != nil {
				return nil, err
			}
			hook.RunAs, hook.UpdatedAt = actor.AccountID, f.Clock.Now()
			err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				if err := f.Store.PutHook(ctx, hook); err != nil {
					return err
				}
				return f.record(ctx, actor, HookRotatedAction, hookTarget, hook.ID)
			})
			if err != nil {
				return nil, err
			}
			out := HookOutput(hook)
			out["secret"] = f.Secrets.Derive(actor.TenantID, hook.ID, hook.Salt)
			return out, nil
		}),
	}
}

// ListEntryCodeLinks answers an entry's commits, branches and pull requests.
type ListEntryCodeLinks struct{ Forge Forge }

// Descriptor is the catalogue entry.
func (h ListEntryCodeLinks) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ListEntryCodeLinksName, Permission: service.PermissionRead,
		Summary: "Lists the commits, branches and pull requests that named an entry's key in a forge hooked to its " +
			"hub, the latest first, each with its repository, title, address, author and state - pushed, open, " +
			"merged or closed.",
		SideEffects: "None. Reads only.", TokenScope: linksRead, ReadOnly: true,
		Input:    []usecase.Field{{Name: "item_id", Kind: usecase.KindID, Required: true, Description: "The entry."}},
		Audit:    usecase.AuditDeclaration{Action: LinksReadAction, TargetType: itemTarget, Severity: audit.SeverityInfo},
		Activity: usecase.ActivityDeclaration{Exempt: exemptReason},
		Handler: usecase.HandlerFunc(func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			item, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			f := h.Forge
			var summaries []workrepo.EntrySummary
			if err := f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				var err error
				summaries, err = f.Entries.Entries(ctx, []shared.ID{item})
				return err
			}); err != nil {
				return nil, err
			}
			if len(summaries) != 1 {
				return nil, shared.ErrNotFound.WithDetail("work_item.not_found")
			}
			if err := f.Authorizer.Authorize(ctx, actor, access.Request{
				Permission: service.PermissionRead, Path: entryPath(summaries[0]), Action: LinksReadAction,
				TokenScope: linksRead, TargetType: itemTarget, TargetID: item,
			}); err != nil {
				return nil, err
			}
			rows := []usecase.Output{}
			err = f.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
				links, err := f.Store.ListLinks(ctx, item, maxLinksListed)
				for _, l := range links {
					rows = append(rows, LinkOutput(l, summaries[0].CollectionID))
				}
				return err
			})
			return usecase.Output{"data": rows}, err
		}),
	}
}

// Delivery is one post to a hook, as the intake adapter received it.
type Delivery struct {
	TenantID shared.ID
	HookID   shared.ID
	// Event is the forge's event header: `push`, `pull_request`, `Push Hook`, `Merge Request Hook`.
	Event string
	// Signature is GitHub's `X-Hub-Signature-256` or Forgejo's and Gitea's signature header.
	Signature string
	// Token is GitLab's `X-Gitlab-Token`.
	Token string
	// DeliveryID is the forge's delivery identifier, by which a redelivery is recognised.
	DeliveryID string
	Body       []byte
}

// Parser translates a forge's delivery into signals; the format is the adapter's, the kind the
// hook's.
type Parser func(kind domain.Kind, event string, body []byte) ([]domain.Signal, error)

// Receipt is what a delivery did.
type Receipt struct {
	Received int
	Linked   int
	// Unknown counts keys that name no live entry of the hook's hub.
	Unknown int
}

// hookNotFound is every refusal before the delivery is trusted: an unknown hook, one switched off,
// a wrong secret, a delivery seen already, a writer who may no longer write. One answer, so that
// nothing tells somebody guessing which check failed (T-21).
func hookNotFound() error { return shared.ErrNotFound.WithDetail("forge.hook_not_found") }

// verify answers whether the delivery proves the hook's secret.
func verify(kind domain.Kind, d Delivery, key string) bool {
	switch kind {
	case domain.GitLab:
		return d.Token != "" && subtle.ConstantTimeCompare([]byte(d.Token), []byte(key)) == 1
	case domain.GitHub, domain.Forgejo:
		sig := strings.ToLower(strings.TrimSpace(d.Signature))
		if kind == domain.GitHub && !strings.HasPrefix(sig, "sha256=") {
			return false
		}
		presented, err := hex.DecodeString(strings.TrimPrefix(sig, "sha256="))
		if err != nil || len(presented) == 0 {
			return false
		}
		mac := hmac.New(sha256.New, []byte(key))
		mac.Write(d.Body)
		return hmac.Equal(presented, mac.Sum(nil))
	}
	return false
}

// Sign is the signature GitHub, Forgejo and Gitea send with a delivery, for the documentation and
// the tests.
func Sign(key string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

// digest is what a delivery is recognised by again: the forge's delivery identifier, or the body.
func digest(d Delivery) []byte {
	h := sha256.New()
	if d.DeliveryID != "" {
		h.Write([]byte("id:" + d.DeliveryID))
	} else {
		h.Write([]byte("body:"))
		h.Write(d.Body)
	}
	return h.Sum(nil)
}

// Deliver authenticates a delivery by the hook's secret and keeps each commit, branch and pull
// request on the hub's entries its keys name (UC-INT-13).
func (f Forge) Deliver(ctx context.Context, d Delivery, parse Parser) (Receipt, error) {
	scope := appshared.ActorContext{TenantID: d.TenantID}.PersistenceScope()
	var hook domain.Hook
	if err := f.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		hook, err = f.Store.FindHook(ctx, d.HookID)
		return err
	}); err != nil || !hook.Enabled {
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return Receipt{}, err
		}
		return Receipt{}, hookNotFound()
	}
	if !verify(hook.Kind, d, f.Secrets.Derive(d.TenantID, hook.ID, hook.Salt)) {
		return Receipt{}, hookNotFound()
	}
	actor := appshared.ActorContext{
		Kind: appshared.ActorAutomation, TenantID: d.TenantID, AccountID: hook.RunAs,
		Scopes: []string{linksRead, linksWrite}, Channel: string(work.ChannelImport),
	}
	if err := f.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionWriteItems, Path: hubPath(hook.HubID), Action: DeliveryTakenAction,
		TokenScope: linksWrite, TargetType: hookTarget, TargetID: hook.ID,
	}); err != nil {
		return Receipt{}, hookNotFound()
	}
	signals, err := parse(hook.Kind, d.Event, d.Body)
	if err != nil {
		return Receipt{}, err
	}
	if len(signals) > domain.MaxSignalsPerDelivery {
		signals = signals[:domain.MaxSignalsPerDelivery]
	}
	now := f.Clock.Now()
	var receipt Receipt
	err = f.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		fresh, err := f.Store.SeeDelivery(ctx, hook.ID, digest(d), now, now.Add(-DeliveryMemory))
		if err != nil {
			return err
		}
		if !fresh {
			return hookNotFound()
		}
		for _, raw := range signals {
			sig, err := domain.CheckSignal(raw)
			if err != nil {
				return err
			}
			receipt.Received++
			if err := f.apply(ctx, actor, hook, sig, now, &receipt); err != nil {
				return err
			}
		}
		hook.LastDeliveryAt = &now
		if err := f.Store.PutHook(ctx, hook); err != nil {
			return err
		}
		return f.record(ctx, actor, DeliveryTakenAction, hookTarget, hook.ID,
			audit.Change{Field: "received", Classification: audit.Open, To: receipt.Received},
			audit.Change{Field: "linked", Classification: audit.Open, To: receipt.Linked},
			audit.Change{Field: "unknown", Classification: audit.Open, To: receipt.Unknown})
	})
	return receipt, err
}

// apply keeps one signal on every entry of the hook's hub its keys name.
func (f Forge) apply(ctx context.Context, actor appshared.ActorContext, hook domain.Hook, sig domain.Signal, now time.Time, r *Receipt) error {
	for _, key := range domain.KeysIn(sig.Texts...) {
		item, err := f.Keys.ItemByKey(ctx, key)
		if errors.Is(err, shared.ErrNotFound) {
			r.Unknown++
			continue
		}
		if err != nil {
			return err
		}
		summaries, err := f.Entries.Entries(ctx, []shared.ID{item})
		if err != nil {
			return err
		}
		if len(summaries) != 1 || summaries[0].HubID != hook.HubID {
			r.Unknown++
			continue
		}
		existing, err := f.Store.FindLink(ctx, item, hook.ID, sig.Kind, sig.Repository, sig.Ref)
		if err != nil && !errors.Is(err, shared.ErrNotFound) {
			return err
		}
		link, changed := domain.Apply(existing, sig, now)
		if !changed {
			continue
		}
		if link.ID.IsZero() {
			link.ID, link.TenantID, link.ItemID, link.HubID, link.HookID = f.IDs.NewID(), actor.TenantID, item, hook.HubID, hook.ID
		}
		if err := f.Store.PutLink(ctx, link); err != nil {
			return err
		}
		r.Linked++
		if f.Changes != nil {
			if err := f.Changes.Record(ctx, changelog.Change{
				TenantID: actor.TenantID, Entity: LinkEntity, EntityID: link.ID, Op: changelog.Upsert,
				ContainerID: summaries[0].CollectionID, ActorID: actor.AccountID, HLC: f.HLC.Next(),
				Payload: LinkOutput(link, summaries[0].CollectionID),
			}); err != nil {
				return err
			}
		}
	}
	return nil
}
