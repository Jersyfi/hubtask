// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/text"
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
//
// The first of the four doors a password is set through (ADR-0068 §5). The rule it is judged
// against is the workspace's own, resolved from the tenant the token names - which is known before
// the token is looked up, so the policy half of the refusal still discloses nothing about whether
// the token was real.
type RedeemInvitation struct {
	Writer SessionWriter
	// Passwords is the one place the rule lives. Nil on an installation wired before ADR-0068,
	// where the product's own default is what a password is judged against - which is exactly what
	// this route enforced before there was a rule to resolve.
	Passwords *PasswordWriter
}

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
	// A workspace that switched the password off sets none, however the invitation arrived (SC-24):
	// the person signs in through the workspace's provider. Asked before the token is looked up -
	// and once: where the password is open only as the fallback, this answer is what the trail
	// records, so that a person invited into a workspace with no way in that works (a provisioned
	// workspace born under an installation default without the password, say) can accept the
	// invitation, and its administrators can see that they did (E2, #1138).
	var fallback FallbackCause
	if h.Passwords != nil {
		open, viaFallback, err := h.Passwords.PasswordOpen(ctx, token.TenantID())
		if err := refuseShut(open, viaFallback, err); err != nil {
			return SessionPair{}, err
		}
		fallback = viaFallback
	}
	// The policy binds where a password is set - and the part of it that needs no account is
	// checked before the token is looked up, so this half of the refusal says nothing about
	// whether the token was real. The workspace's own rule is used, resolved from the tenant the
	// token names rather than from a row anybody had to find.
	rules, err := h.rulesFor(ctx, token.TenantID())
	if err != nil {
		return SessionPair{}, err
	}
	if err := domain.CheckPasswordAgainst(
		rules.Effective.Policy.Password, h.form(), cmd.Password.Reveal(),
		domain.PasswordContext{
			WorkspaceName: rules.Workspace.DisplayName, WorkspaceHost: rules.Workspace.Slug,
		},
	); err != nil {
		return SessionPair{}, err
	}

	// The hash is computed outside the transaction, Argon2id being deliberately slow. Of the
	// normalised password, so that what the rule counted is what is stored (ADR-0068 §7).
	passwordHash, err := w.Passwords.Hash(
		secret.New(domain.NormalisePassword(h.form(), cmd.Password.Reveal())))
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
		// The workspace's standing (H-06): an invitation into a suspended workspace waits the
		// suspension out rather than opening a first session into it.
		if err := found.TenantStatus.Verify(); err != nil {
			return err
		}
		// And the half of the rule that needs the account: the context words drawn from the
		// address and the name, and the offline lists. Refusing here is safe - the caller is
		// already holding a token that was real.
		if err := h.judge(ctx, token.TenantID(), found.Account, cmd.Password, rules); err != nil {
			return err
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

		if err := w.Audit.Append(ctx, audit.Entry{
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
		}); err != nil {
			return err
		}
		if !fallback.Opens() {
			return nil
		}
		// In the redemption's own transaction: the password is set and the entry lands, or neither.
		return w.Audit.Append(ctx, fallbackEntry(ctx, token.TenantID(), account, now, fallback))
	})
	if err != nil {
		return SessionPair{}, err
	}

	return w.openSessionWith(ctx, scope, token.TenantID(), account,
		cmd.UserAgent, cmd.RemoteAddr, SignedInAction, nil,
		rules.Effective.Policy.Sessions, domain.SignedInWithInvitation)
}

// rulesFor resolves the workspace's rule, or the product's default on an installation wired
// before there was one to resolve.
func (h RedeemInvitation) rulesFor(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error) {
	if h.Passwords == nil {
		return ResolvedPolicy{
			Effective: domain.Effective(domain.PolicyLayer{}, domain.PolicyLayer{}, domain.PolicyLayer{}),
		}, nil
	}
	return h.Passwords.ResolveFor(ctx, tenantID)
}

// judge runs the half of the rule that needs the account.
func (h RedeemInvitation) judge(
	ctx context.Context, tenantID shared.ID, account domain.Account,
	password secret.Secret, rules ResolvedPolicy,
) error {
	if h.Passwords == nil {
		return nil
	}
	return h.Passwords.Judge(ctx, PasswordCandidate{
		TenantID: tenantID,
		Account:  repository.PasswordAccount{Account: account},
		Password: password,
		Rules:    rules,
	})
}

func (h RedeemInvitation) form() text.Normalizer {
	if h.Passwords == nil {
		return nil
	}
	return h.Passwords.Text
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
