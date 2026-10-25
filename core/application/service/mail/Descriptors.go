// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"errors"
	"strconv"

	"github.com/Jersyfi/hubtask/core/application/usecase"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// Descriptor is the catalogue entry.
func (h ShowMailSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ShowMailSettingsName,
		Summary:     "The workspace's mail intake: whether it is on, its name on the receiving domain, the default collection, who may write and the rules that file a sender's mail.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: settingsReadAction, TargetType: settingsTarget, Severity: audit.SeverityInfo},
		Handler:     usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h SaveMailSettings) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        SaveMailSettingsName,
		Summary:     "Replaces the workspace's mail intake settings in one write; whoever saves is the account a sender who is not a member writes through.",
		SideEffects: "Stores the settings and writes an audit entry.",
		TokenScope:  automationManage,
		Input: []usecase.Field{
			{Name: "enabled", Kind: usecase.KindBool, Description: "Whether the intake takes mail."},
			{Name: "name", Kind: usecase.KindString, Description: "The workspace's name on the receiving domain, familie-meier; unique on the installation."},
			{Name: "default_collection_id", Kind: usecase.KindID, Description: "Where a message naming no place goes; absent for the jumble."},
			{Name: "senders", Kind: usecase.KindObject, Description: "level (MEMBERS, MEMBERS_AND_ALLOWED, ANYONE), allowed and blocked: addresses and @domains."},
			{Name: "sender_routes", Kind: usecase.KindList, Description: "Objects of match (an address or @domain) and collection_id."},
		},
		Audit: usecase.AuditDeclaration{
			Action: settingsSavedAction, TargetType: settingsTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "Settings of the workspace, not of an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ListMailReceipts) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ListMailReceiptsName,
		Summary:     "The newest messages the mail intake received - when, from whom, what each became and why - for the last 30 days.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: receiptsReadAction, TargetType: settingsTarget, Severity: audit.SeverityInfo},
		Handler:     usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h SetCollectionHandle) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        SetCollectionHandleName,
		Summary:     "Gives a collection another handle - the short name a mail subject, an address and its entries' ticket keys use for it.",
		SideEffects: "Changes the handle and writes an audit entry; mail to the old handle no longer reaches the collection.",
		TokenScope:  containersWrite,
		Input: []usecase.Field{
			{Name: "collection_id", Kind: usecase.KindID, Required: true, Description: "The collection."},
			{Name: "handle", Kind: usecase.KindString, Required: true, Description: "A letter, then letters and digits; two to twenty."},
		},
		Audit: usecase.AuditDeclaration{
			Action: handleSetAction, TargetType: containerTarget, Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "A collection's setting, not an entry's history."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Replies answers the address a notification's reply goes to: the workspace's address on the
// operator's domain with the entry's ticket key, "familie-meier+web-42@in.hubtask.app". Empty
// where the intake is off or has no name there - the reply then goes nowhere special.
type Replies struct{ Writer Writer }

// ReplyAddress answers the address.
func (r Replies) ReplyAddress(ctx context.Context, tenantID, itemID shared.ID) (string, error) {
	w := r.Writer
	if w.Domain == "" || tenantID.IsZero() || itemID.IsZero() {
		return "", nil
	}
	var address string
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID}, func(ctx context.Context) error {
		settings, err := w.Settings.Find(ctx)
		if err != nil || !settings.Enabled || settings.Name == "" {
			return err
		}
		key, err := w.Places.KeyOf(ctx, itemID)
		if err != nil {
			return err
		}
		address = settings.Name + "+" + key.Handle + "-" + strconv.FormatInt(key.Number, 10) + "@" + w.Domain
		return nil
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return "", nil
		}
		return "", err
	}
	return address, nil
}

// Descriptor is the catalogue entry.
func (h ListMailboxes) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ListMailboxesName,
		Summary:     "The workspace's connected mailboxes - where each is, how its last read went - and the provider presets a new one can start from. Never a secret.",
		SideEffects: "None. Reads only.",
		TokenScope:  automationManage,
		ReadOnly:    true,
		Audit:       usecase.AuditDeclaration{Action: mailboxesReadAction, TargetType: mailboxTarget, Severity: audit.SeverityInfo},
		Handler:     usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h ConnectMailbox) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        ConnectMailboxName,
		Summary:     "Connects a mailbox to the workspace's mail intake: signs in once, stores it with its secret sealed, and reads it every minute from then on. Only mail that arrives after connecting is taken.",
		SideEffects: "Signs in to the mail server, stores the mailbox, queues its read and writes an audit entry.",
		TokenScope:  automationManage,
		Input: []usecase.Field{
			{Name: "preset", Kind: usecase.KindString, Description: "gmail, gmx, webde, ionos or fastmail; empty for another provider."},
			{Name: "protocol", Kind: usecase.KindString, Description: "IMAP or JMAP, where no preset says which."},
			{Name: "address", Kind: usecase.KindString, Required: true, Description: "The mailbox's own address."},
			{Name: "host", Kind: usecase.KindString, Description: "The IMAP server, where no preset names it."},
			{Name: "port", Kind: usecase.KindInt, Description: "The IMAP port with TLS; 993 by default."},
			{Name: "session_url", Kind: usecase.KindString, Description: "The JMAP session resource, where no preset names it."},
			{Name: "username", Kind: usecase.KindString, Description: "The sign-in name; the address by default."},
			{Name: "secret", Kind: usecase.KindString, Required: true, Description: "The password, app password or token. Stored sealed, never answered."},
			{Name: "folder", Kind: usecase.KindString, Description: "The folder read; INBOX by default."},
			{Name: "done_folder", Kind: usecase.KindString, Description: "Where a taken message is moved; Hubtask by default."},
		},
		Audit: usecase.AuditDeclaration{
			Action: mailboxConnected, TargetType: mailboxTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "Settings of the workspace, not of an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h CheckMailbox) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        CheckMailboxName,
		Summary:     "Signs in to a connected mailbox again, records how that went, and reads it now.",
		SideEffects: "Signs in to the mail server, records the result, queues a read and writes an audit entry.",
		TokenScope:  automationManage,
		Input: []usecase.Field{
			{Name: "mailbox_id", Kind: usecase.KindID, Required: true, Description: "The mailbox."},
		},
		Audit: usecase.AuditDeclaration{
			Action: mailboxChecked, TargetType: mailboxTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "Settings of the workspace, not of an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}

// Descriptor is the catalogue entry.
func (h RemoveMailbox) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:        RemoveMailboxName,
		Summary:     "Disconnects a mailbox: its secret is forgotten and it is read no more. The mail in it stays where it is.",
		SideEffects: "Removes the mailbox and writes an audit entry.",
		TokenScope:  automationManage,
		Input: []usecase.Field{
			{Name: "mailbox_id", Kind: usecase.KindID, Required: true, Description: "The mailbox."},
		},
		Audit: usecase.AuditDeclaration{
			Action: mailboxRemoved, TargetType: mailboxTarget, Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{Exempt: "Settings of the workspace, not of an entry."},
		Handler:  usecase.HandlerFunc(h.invoke),
	}
}
