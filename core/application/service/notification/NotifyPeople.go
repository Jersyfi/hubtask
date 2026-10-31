// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"slices"
	"strconv"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
)

const (
	NotifyAccountName = "NotifyAccount"
	NotifyGroupName   = "NotifyGroup"
	NotifyRoleName    = "NotifyRole"

	// RuleNoticeSentAction is a notification step having told people about an entry: who caused
	// it and how many were told, never who - the records say that, to their recipients.
	RuleNoticeSentAction audit.Action = "notifications.rule_notice_sent"

	// RecipientUnknownCode refuses a step naming an account, a group or a role the workspace does
	// not have (UC-AUT-09 check 4) - the save and the run give the same answer.
	RecipientUnknownCode = "notifications.recipient_unknown"
	// TooManyRecipientsCode refuses a step that would tell more people than MaxNoticeRecipients:
	// refused whole, before a record is written, rather than half sent.
	TooManyRecipientsCode = "notifications.too_many_recipients"

	// MaxNoticeRecipients bounds one step. A role held on the workspace by a large tenant would
	// otherwise be one run writing thousands of records inside the engine's transaction.
	MaxNoticeRecipients = 500

	noticeScope      = "automation:manage"
	noticeItemTarget = "item"
)

// NoticeAuthorizer is the slice of the authorisation service a notification step asks: whether the
// caller may, whether each recipient may read the entry, and which roles a candidate holds where
// the entry is (ADR-0005).
type NoticeAuthorizer interface {
	Authorize(ctx context.Context, actor appshared.ActorContext, request access.Request) error
	CanSee(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope) (bool, error)
	RolesBy(ctx context.Context, actor appshared.ActorContext, accountID shared.ID, path []identity.Scope) ([]string, error)
}

// NoticeContainers reads the collection an entry is in, for its path.
type NoticeContainers interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// NoticeGroups reads a group and who is in it.
type NoticeGroups interface {
	Find(ctx context.Context, groupID shared.ID) (identity.Group, error)
	Members(ctx context.Context, groupID shared.ID) ([]shared.ID, error)
}

// RoleHolders answers the candidates holding a role anywhere on a path.
type RoleHolders interface {
	RoleHolders(ctx context.Context, path []identity.Scope, role string) ([]shared.ID, error)
}

// CustomRoleKeys answers whether a custom role exists, by its key.
type CustomRoleKeys interface {
	Find(ctx context.Context, key string) (identityrepo.CustomRoleRow, error)
}

// RestrictedAccounts answers which of the accounts may not be processed automatically
// (data-protection.md §4).
type RestrictedAccounts interface {
	Restricted(ctx context.Context, accountIDs []shared.ID) (map[shared.ID]bool, error)
}

// NoticeDesk is what the three notification steps share (UC-AUT-09).
//
// Through the notification path every other message takes: the record, the preference decided by
// decideFor, the send a job. A recipient who may not read the entry is left out rather than
// refused, so a group with one member outside the hub still tells the others; a person whose
// processing is restricted is left out of a rule's message as automatic assignment leaves them out.
type NoticeDesk struct {
	Notifications repository.Notifications
	Preferences   repository.Preferences
	Accounts      identityrepo.Accounts
	Items         Entries
	Containers    NoticeContainers
	Groups        NoticeGroups
	Holders       RoleHolders
	CustomRoles   CustomRoleKeys
	Restricted    RestrictedAccounts
	Authorizer    NoticeAuthorizer
	Audit         audit.Sink
	Jobs          Queue
	UnitOfWork    persistence.UnitOfWork
	Clock         clock.Clock
	IDs           clock.IDGenerator
}

// NotifyAccount tells one account.
type NotifyAccount struct{ Desk NoticeDesk }

// NotifyGroup tells each member of a group.
type NotifyGroup struct{ Desk NoticeDesk }

// NotifyRole tells the holders of a role along the entry's path.
type NotifyRole struct{ Desk NoticeDesk }

// recipients names whom a step tells: exactly one of the three.
type recipients struct {
	account shared.ID
	group   shared.ID
	role    string
}

// Execute tells the account and answers how many were told.
func (h NotifyAccount) Execute(ctx context.Context, actor appshared.ActorContext, itemID, accountID shared.ID) (int, error) {
	return h.Desk.notify(ctx, actor, itemID, recipients{account: accountID})
}

// Execute tells the group's members and answers how many were told.
func (h NotifyGroup) Execute(ctx context.Context, actor appshared.ActorContext, itemID, groupID shared.ID) (int, error) {
	return h.Desk.notify(ctx, actor, itemID, recipients{group: groupID})
}

// Execute tells the role's holders and answers how many were told.
func (h NotifyRole) Execute(ctx context.Context, actor appshared.ActorContext, itemID shared.ID, role string) (int, error) {
	return h.Desk.notify(ctx, actor, itemID, recipients{role: role})
}

// notify is the whole of the three: the caller's permission on the entry, the candidates, those
// who may read it, then the records and their sends in one transaction.
func (d NoticeDesk) notify(
	ctx context.Context, actor appshared.ActorContext, itemID shared.ID, to recipients,
) (int, error) {
	if itemID.IsZero() {
		return 0, shared.ErrValidation.WithDetail("items.item_id_required").
			WithFields(shared.FieldError{Path: "/item_id", Code: "items.item_id_required"})
	}

	var item work.WorkItem
	var collection work.Container
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if item, err = d.Items.Find(ctx, itemID); err != nil {
			return err
		}
		if item.DeletedAt != nil {
			// A trashed entry is nobody's news: the link would open the trash.
			return appshared.ItemNotFound(itemID)
		}
		collection, err = d.Containers.Find(ctx, item.CollectionID)
		return err
	})
	if err != nil {
		return 0, err
	}

	if err := d.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionAutomation,
		Path:       service.ContainerScopes(collection),
		Action:     RuleNoticeSentAction,
		TokenScope: noticeScope,
		TargetType: noticeItemTarget,
		TargetID:   itemID,
		On:         access.ItemSubject{Does: service.ItemRead, ID: item.ID, Assignee: item.AssigneeID},
	}); err != nil {
		return 0, err
	}

	path := append(service.ContainerScopes(collection), identity.ItemScope(item.ID))
	candidates, err := d.candidates(ctx, actor, path, to)
	if err != nil {
		return 0, err
	}
	told, err := d.readers(ctx, actor, path, candidates)
	if err != nil {
		return 0, err
	}
	if len(told) > MaxNoticeRecipients {
		return 0, shared.ErrValidation.WithDetail(TooManyRecipientsCode).
			WithParams(map[string]string{"limit": strconv.Itoa(MaxNoticeRecipients)})
	}

	err = d.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		for _, recipient := range told {
			if err := d.record(ctx, actor, item, recipient); err != nil {
				return err
			}
		}
		return d.recordSent(ctx, actor, item, len(told))
	})
	if err != nil {
		return 0, err
	}
	return len(told), nil
}

// candidates are the accounts the step names before anybody's read right is asked. A name the
// workspace does not have is refused; an account of another tenant reads as one that does not
// exist, because row level security finds neither (T-04).
func (d NoticeDesk) candidates(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope, to recipients,
) ([]shared.ID, error) {
	var found []shared.ID
	err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		switch {
		case !to.account.IsZero():
			if _, err := d.Accounts.Find(ctx, to.account); err != nil {
				return unknownRecipient(err, "/account_id")
			}
			found = []shared.ID{to.account}
		case !to.group.IsZero():
			if _, err := d.Groups.Find(ctx, to.group); err != nil {
				return unknownRecipient(err, "/group_id")
			}
			members, err := d.Groups.Members(ctx, to.group)
			if err != nil {
				return err
			}
			found = members
		case to.role != "":
			known, err := RoleKnown(ctx, d.CustomRoles, to.role)
			if err != nil {
				return err
			}
			if !known {
				return unknownRecipient(shared.ErrNotFound, "/role")
			}
			holders, err := d.Holders.RoleHolders(ctx, path, to.role)
			if err != nil {
				return err
			}
			found = holders
		default:
			return unknownRecipient(shared.ErrNotFound, "")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if to.role == "" {
		return found, nil
	}

	// A role held on the workspace does not count inside a private hub (ADR-0073 §1): the query
	// finds where it was granted, the authoriser whether it counts where the entry is.
	var counting []shared.ID
	for _, candidate := range found {
		held, err := d.Authorizer.RolesBy(ctx, actor, candidate, path)
		if err != nil {
			return nil, err
		}
		if slices.Contains(held, to.role) {
			counting = append(counting, candidate)
		}
	}
	return counting, nil
}

// readers keeps the candidates who may read the entry (UC-AUT-09 check 3) and, under a rule, are
// not restricted from automatic processing - without duplicates, in a stable order.
func (d NoticeDesk) readers(
	ctx context.Context, actor appshared.ActorContext, path []identity.Scope, candidates []shared.ID,
) ([]shared.ID, error) {
	slices.Sort(candidates)
	candidates = slices.Compact(candidates)

	var restricted map[shared.ID]bool
	if d.Restricted != nil && !actor.RunsForRule.IsZero() && len(candidates) > 0 {
		err := d.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
			var err error
			restricted, err = d.Restricted.Restricted(ctx, candidates)
			return err
		})
		if err != nil {
			return nil, err
		}
	}

	told := make([]shared.ID, 0, len(candidates))
	for _, candidate := range candidates {
		if restricted[candidate] {
			continue
		}
		sees, err := d.Authorizer.CanSee(ctx, actor, candidate, path)
		if err != nil {
			return nil, err
		}
		if sees {
			told = append(told, candidate)
		}
	}
	return told, nil
}

// record writes one recipient's message, suppressed where their preference says so, and queues
// its send.
func (d NoticeDesk) record(
	ctx context.Context, actor appshared.ActorContext, item work.WorkItem, recipient shared.ID,
) error {
	if _, err := d.Accounts.Find(ctx, recipient); errors.Is(err, shared.ErrNotFound) {
		// The account went between the read and the write; the foreign key would refuse a record.
		return nil
	} else if err != nil {
		return err
	}
	written, err := domain.New(domain.NewInput{
		ID:          d.IDs.NewID(),
		TenantID:    actor.TenantID,
		RecipientID: recipient,
		Category:    domain.CategorySLA,
		Channel:     domain.ChannelEmail,
		// The entry's identifier, never its title (rule 10): the delivery reads the title when
		// it renders, with the recipient's own preference about whether it may travel.
		ItemID: item.ID,
		At:     d.Clock.Now(),
	})
	if err != nil {
		return err
	}
	decision, err := decideFor(ctx, d.Accounts, d.Preferences, written, recipient, domain.CategorySLA)
	if err != nil {
		return err
	}
	if !decision.Send {
		written = written.Suppress(decision.Reason)
	}
	first, err := d.Notifications.Insert(ctx, written)
	if err != nil {
		return err
	}
	if !first || !decision.Send {
		return nil
	}
	_, err = d.Jobs.Enqueue(ctx, queue.Request{
		Kind:      queue.KindNotificationDeliver,
		TenantID:  written.TenantID,
		DedupeKey: written.ID.String(),
		RunAt:     written.CreatedAt,
		Payload:   map[string]any{"notification_id": written.ID.String()},
	})
	return err
}

// recordSent writes the step's audit entry: the entry and the count, never a recipient's name.
func (d NoticeDesk) recordSent(
	ctx context.Context, actor appshared.ActorContext, item work.WorkItem, told int,
) error {
	if d.Audit == nil {
		return nil
	}
	return d.Audit.Append(ctx, audit.Entry{
		TenantID:   actor.TenantID,
		OccurredAt: d.Clock.Now(),
		Action:     RuleNoticeSentAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityInfo,
		ActorKind:  actor.Kind,
		ActorID:    actor.AccountID,
		ActorLabel: actor.AccountName,
		TargetType: noticeItemTarget,
		TargetID:   item.ID,
		Changes: audit.Changes(
			audit.Change{Field: "recipients", Classification: audit.Open, To: strconv.Itoa(told)},
		),
	})
}

// RoleKnown answers whether a role key names a built-in role or a custom role of the workspace:
// what the step and the rule's save both ask (UC-AUT-09 check 4).
func RoleKnown(ctx context.Context, custom CustomRoleKeys, key string) (bool, error) {
	if identity.Role(key).Valid() {
		return true, nil
	}
	if custom == nil {
		return false, nil
	}
	_, err := custom.Find(ctx, key)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		return false, nil
	case err != nil:
		return false, err
	}
	return true, nil
}

func unknownRecipient(err error, path string) error {
	if !errors.Is(err, shared.ErrNotFound) {
		return err
	}
	refusal := shared.ErrValidation.WithDetail(RecipientUnknownCode)
	if path != "" {
		refusal = refusal.WithFields(shared.FieldError{Path: path, Code: RecipientUnknownCode})
	}
	return refusal
}

func noticeOutput(told int) usecase.Output { return usecase.Output{"recipients": told} }

func noticeDescriptor(name, summary string, recipient usecase.Field, handler usecase.HandlerFunc) usecase.Descriptor {
	return usecase.Descriptor{
		Permission: service.PermissionAutomation,
		Name:       name,
		Summary: summary + " Through the notification path every other message takes, under the " +
			"category SLA: the recipient's preference applies, the mail is in their language and " +
			"carries the entry's title and a link only. A recipient who may not read the entry is " +
			"not told. As a rule action, the run supplies the entry.",
		SideEffects: "Records one notification per recipient, queues each send, and writes an " +
			"audit entry with the count.",
		TokenScope: noticeScope,
		Input: []usecase.Field{
			{
				Name: "item_id", Kind: usecase.KindID, Required: true,
				Description: "The entry the message is about. A rule leaves this out and the run " +
					"supplies the entry it is about.",
			},
			recipient,
		},
		Output: []usecase.Field{
			{Name: "recipients", Kind: usecase.KindInt, Description: "How many a message was recorded for."},
		},
		Audit: usecase.AuditDeclaration{
			Action: RuleNoticeSentAction, TargetType: noticeItemTarget,
			Severity: audit.SeverityInfo, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A message tells people about the entry and changes nothing of it.",
		},
		Handler: handler,
	}
}

// Descriptor is the catalogue entry, and the automation action NOTIFY_ACCOUNT with it.
func (h NotifyAccount) Descriptor() usecase.Descriptor {
	return noticeDescriptor(NotifyAccountName, "Tells one account that something happened to an entry.",
		usecase.Field{Name: "account_id", Kind: usecase.KindID, Required: true,
			Description: "The account to tell. One the workspace does not have is refused."},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			accountID, err := in.ID("account_id")
			if err != nil {
				return nil, err
			}
			told, err := h.Execute(ctx, actor, itemID, accountID)
			if err != nil {
				return nil, err
			}
			return noticeOutput(told), nil
		})
}

// Descriptor is the catalogue entry, and the automation action NOTIFY_GROUP with it.
func (h NotifyGroup) Descriptor() usecase.Descriptor {
	return noticeDescriptor(NotifyGroupName, "Tells each member of a group that something happened to an entry; each member is one recipient.",
		usecase.Field{Name: "group_id", Kind: usecase.KindID, Required: true,
			Description: "The group whose members are told. One the workspace does not have is refused."},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			groupID, err := in.ID("group_id")
			if err != nil {
				return nil, err
			}
			told, err := h.Execute(ctx, actor, itemID, groupID)
			if err != nil {
				return nil, err
			}
			return noticeOutput(told), nil
		})
}

// Descriptor is the catalogue entry, and the automation action NOTIFY_ROLE with it.
func (h NotifyRole) Descriptor() usecase.Descriptor {
	return noticeDescriptor(NotifyRoleName, "Tells every account holding a role on the entry's path - the workspace, its hub, its collection or the entry, directly or through a group - that something happened to it.",
		usecase.Field{Name: "role", Kind: usecase.KindString, Required: true,
			Description: "A built-in role or the key of a custom role. One the workspace does not know is refused."},
		func(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
			itemID, err := in.ID("item_id")
			if err != nil {
				return nil, err
			}
			role := in.String("role")
			told, err := h.Execute(ctx, actor, itemID, role)
			if err != nil {
				return nil, err
			}
			return noticeOutput(told), nil
		})
}
