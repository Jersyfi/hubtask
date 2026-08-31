// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const RedeemInvitationName = "RedeemInvitation"

// InvitationRedeemedAction is the moment an invited account becomes a person (audit.md §2). The
// target type is InviteAccount's accountTarget: redemption is the invitation's other half.
const InvitationRedeemedAction audit.Action = "auth.invitation_redeemed"

// MintRedemptionToken mints the credential an invitation mail carries (H-01, data-catalog.md
// §7.5): shown once in the mail, stored only as a hash under its own purpose label, dead on
// redemption or after its fortnight.
//
// It is minted at delivery time rather than at the invite, deliberately: the queue's payload
// carries identifiers only (rule 10), and a token minted beside the account row would have to
// travel through it. A delivery retried mints again and the newest token wins - an invitation
// mail can only be acted on if it arrived, and the one that arrived is the newest.
type MintRedemptionToken struct {
	Accounts   RedemptionTokenStore
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	Entropy    clock.Entropy
}

// RedemptionTokenStore is the one-method slice the minter needs of the sign-in accounts.
type RedemptionTokenStore interface {
	SetRedemptionToken(
		ctx context.Context, accountID shared.ID, presented domain.Token,
		expiresAt, now time.Time,
	) (bool, error)
}

// MintRedemptionToken draws, stores the hash, and answers the plaintext for the mail - or the
// empty secret when the account is no longer waiting, which tells the caller to compose the
// plain link instead.
func (m MintRedemptionToken) MintRedemptionToken(
	ctx context.Context, tenantID, accountID shared.ID,
) (secret.Secret, error) {
	material, err := m.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return secret.Secret{}, shared.ErrInternal.
			WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewRedemptionToken(tenantID, material)
	if err != nil {
		return secret.Secret{}, err
	}

	var waiting bool
	err = m.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			now := m.Clock.Now()
			changed, err := m.Accounts.SetRedemptionToken(
				ctx, accountID, presented, now.Add(domain.RedemptionLifetime).UTC(), now)
			waiting = changed
			return err
		})
	if err != nil {
		return secret.Secret{}, err
	}
	if !waiting {
		return secret.Secret{}, nil
	}
	return secret.New(presented.Secret()), nil
}

// RedeemInvitationCommand carries what the public redemption route received.
type RedeemInvitationCommand struct {
	Token    secret.Secret
	Password secret.Secret
	// UserAgent and RemoteAddr are recorded on the session the redemption opens.
	UserAgent  string
	RemoteAddr string
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// RedeemInvitation closes the invitation loop (H-01): the token from the mail, a password under
// the policy, and the account moves from INVITED to ACTIVE - signed in, because making somebody
// who just proved control of the mailbox type the password again teaches nothing.
type RedeemInvitation struct{ Writer SessionWriter }

// Execute redeems. Unknown, expired and already-redeemed are one indistinguishable refusal:
// which addresses hold unredeemed invitations is not for a probe to enumerate.
func (h RedeemInvitation) Execute(
	ctx context.Context, cmd RedeemInvitationCommand,
) (SessionPair, error) {
	w := h.Writer

	token, err := domain.ParseRedemptionToken(cmd.Token.Reveal())
	if err != nil {
		w.failure(ctx, FailureRedemption)
		return SessionPair{}, redemptionRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}
	if err := domain.CheckPassword(cmd.Password.Reveal()); err != nil {
		// The policy binds where a password is set - and it is checked before the token is
		// looked up, so a policy refusal says nothing about whether the token was real.
		return SessionPair{}, err
	}

	// The hash is computed outside the transaction, Argon2id being deliberately slow.
	passwordHash, err := w.Passwords.Hash(cmd.Password)
	if err != nil {
		return SessionPair{}, err
	}

	scope := persistence.Scope{TenantID: token.TenantID()}
	var account domain.Account
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		found, err := w.Accounts.FindByRedemptionToken(ctx, token)
		if err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				w.failure(ctx, FailureRedemption)
				return redemptionRefused()
			}
			return err
		}

		now := w.Clock.Now()
		if !found.ExpiresAt.After(now) || found.Account.Status != domain.AccountInvited {
			w.failure(ctx, FailureRedemption)
			return redemptionRefused()
		}

		redeemed, err := w.Accounts.Redeem(ctx, found.Account.ID, passwordHash, now)
		if err != nil {
			return err
		}
		if !redeemed {
			// Somebody was here first - the second redemption the acceptance demands refused,
			// with the same answer an unknown token gets.
			w.failure(ctx, FailureRedemption)
			return redemptionRefused()
		}

		account = found.Account
		account.Status = domain.AccountActive

		return w.Audit.Append(ctx, audit.Entry{
			TenantID:   token.TenantID(),
			OccurredAt: now,
			Action:     InvitationRedeemedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityNotice,
			ActorKind:  appshared.ActorUser,
			ActorID:    account.ID,
			ActorLabel: account.DisplayName,
			TargetType: accountTarget,
			TargetID:   account.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			Changes: audit.Changes(audit.Change{
				Field: "status", Classification: audit.Open,
				From: string(domain.AccountInvited), To: string(domain.AccountActive),
			}),
		})
	})
	if err != nil {
		return SessionPair{}, err
	}

	return w.openSession(ctx, scope, token.TenantID(), account,
		cmd.UserAgent, cmd.RemoteAddr, SignedInAction, nil)
}

// redemptionRefused is the one probe-facing refusal of the redemption route.
func redemptionRefused() error {
	return shared.ErrUnauthenticated.WithDetail("auth.redemption_failed")
}

// Descriptor is the catalogue entry.
func (h RedeemInvitation) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: RedeemInvitationName,
		Summary: "Redeems an invitation: the token from the invitation mail, a password under " +
			"the policy, and the account moves from INVITED to ACTIVE - signed in, with the " +
			"same pair sign-in answers. The token dies on redemption, so a second redemption " +
			"is refused; an expired or unknown token is the same indistinguishable refusal.",
		SideEffects: "Sets the first password, activates the account, kills the token, opens a " +
			"session, and writes audit entries.",
		Input: []usecase.Field{
			{
				Name: "token", Kind: usecase.KindString, Required: true,
				Description: "The redemption token from the invitation, shown once and usable once.",
			},
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The first password, at least twelve characters (security.md §5).",
			},
			{
				Name: "user_agent", Kind: usecase.KindString,
				Description: "The client as it introduced itself; recorded on the session.",
			},
			{
				Name: "remote_addr", Kind: usecase.KindString,
				Description: "The peer's address. Only its network class is ever recorded.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent. It may confirm the " +
					"token's tenant, never overrule it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: InvitationRedeemedAction, TargetType: accountTarget,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "An account is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h RedeemInvitation) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, RedeemInvitationCommand{
		Token:        secret.New(in.String("token")),
		Password:     secret.New(in.String("password")),
		UserAgent:    in.String("user_agent"),
		RemoteAddr:   in.String("remote_addr"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}
