// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package admin

import (
	"context"
	"net/url"
	"strings"

	adminrepo "github.com/Jersyfi/hubtask/core/application/repository/admin"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const instanceResetLinkIssuedAction audit.Action = "instance.reset_link_issued"

// ResetMinter mints the one-time credential a password reset spends - the reset mail's own
// minting, so the machine's link is that link: one hash stored, the earlier one superseded, the
// same lifetime. A zero Token is an account no password reset opens (one that signs in only
// through a provider that still lets it in).
type ResetMinter interface {
	MintMachineReset(ctx context.Context, tenantID, accountID shared.ID) (MintedReset, error)
}

// MintedReset is the plaintext the link carries, and whether it connects a provider rather than
// sets a password (a workspace that switched the password off, ADR-0078 §1).
type MintedReset struct {
	Token   secret.Secret
	Connect bool
}

// WorkspaceBase answers the address a workspace's people open, read inside its own transaction.
type WorkspaceBase interface {
	Of(ctx context.Context) (string, error)
}

// IssueResetLink is `hubtask account reset-link --workspace <short name> --email <address>`: the
// way back for a person who forgot their password where no mail goes out (fifth answers 5). It
// prints the link the reset mail would carry. The door is access to the machine, as for
// RestoreOperator - no route, no Descriptor - and the act is journalled as the machine's.
type IssueResetLink struct {
	// Workspaces resolves the short name; Writer's Operators the address, its Journal the record.
	Workspaces RecoverWorkspace
	Writer     InstanceWriter
	Minter     ResetMinter
	Addresses  WorkspaceBase
}

// Execute answers the link. The journal records the account and workspace, never the address or
// the token.
func (h IssueResetLink) Execute(ctx context.Context, slug, email string) (string, error) {
	if strings.TrimSpace(slug) == "" || strings.TrimSpace(email) == "" {
		return "", shared.ErrValidation.WithDetail("admin.operator_incomplete")
	}
	machine, err := h.Workspaces.machine(ctx, slug)
	if err != nil {
		return "", err
	}
	var account shared.ID
	err = h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		found, err := h.Writer.Operators.Resolve(ctx, machine.TenantSlug, email)
		account = found
		return err
	})
	if err != nil {
		return "", err
	}
	if account.IsZero() {
		return "", shared.ErrValidation.WithDetail("admin.operator_unknown_account")
	}

	var base string
	err = h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: machine.TenantID},
		func(ctx context.Context) error {
			read, err := h.Addresses.Of(ctx)
			base = strings.TrimSuffix(read, "/")
			return err
		})
	if err != nil {
		return "", err
	}
	if base == "" {
		// A link without an address cannot be opened; HUBTASK_BASE_URL is what names it.
		return "", shared.ErrValidation.WithDetail("admin.reset_link_no_address")
	}

	minted, err := h.Minter.MintMachineReset(ctx, machine.TenantID, account)
	if err != nil {
		return "", err
	}
	if minted.Token.IsEmpty() {
		return "", shared.ErrValidation.WithDetail("admin.reset_link_provider")
	}

	err = h.Writer.UnitOfWork.Within(ctx, persistence.SystemScope(), func(ctx context.Context) error {
		return h.Writer.Journal.Record(ctx, adminrepo.InstanceEvent{
			ID: h.Writer.IDs.NewID(), OccurredAt: h.Writer.Clock.Now(),
			Action: string(instanceResetLinkIssuedAction), ActorLabel: machineLabel,
			Details: map[string]any{
				"account_id": account.String(), "tenant_id": machine.TenantID.String(), "door": machineLabel,
			},
		})
	})
	if err != nil {
		return "", err
	}

	// In the fragment, as the mail puts it: a proxy or a server log never sees what follows #.
	key := "token"
	if minted.Connect {
		key = "connect"
	}
	return base + "/reset#" + key + "=" + url.PathEscape(minted.Token.Reveal()), nil
}
