// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"log/slog"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// Forgetting a password, and coming back from it (ADR-0068 §6).
//
// **One answer, whatever is true.** `password:forgot` answers `202` for an address that holds an
// account and for one that does not, byte for byte: which addresses have accounts is exactly what a
// probe is after (T-02). Behind that one answer sits a job on the queue the invitation already
// uses, so that an unreachable mail server never fails the request - and never becomes the
// difference a probe was looking for.
//
// **Control of a mailbox is one proof, and it does not replace the other.** A reset of an account
// with a second factor answers the `202` that asks for it rather than a session. The mailbox proves
// the address; the factor proves the person, and the account already demanded both.

const (
	ForgetPasswordName = "ForgetPassword"
	ResetPasswordName  = "ResetPassword"
)

// PasswordResetRequestedAction is somebody asking for a link (ADR-0068 §6).
//
// Recorded only where an address held an account, which is the one place there is a workspace trail
// to record it in - so the trail never becomes a list of the addresses a guesser tried. For the
// account holder it is the entry that matters most in this whole flow: a run of these is what an
// account takeover looks like from the outside, which is why it is a notice rather than info.
const PasswordResetRequestedAction audit.Action = "auth.password_reset_requested"

// ForgetPasswordCommand is what the public route received.
type ForgetPasswordCommand struct {
	Email string
	// TenantSlug and TenantHeader resolve the workspace, sign-in's way: an address is per
	// workspace, so there is nothing to look up until one is known.
	TenantSlug   string
	TenantHeader string
}

// ForgetPassword asks for a link.
type ForgetPassword struct {
	Writer PasswordWriter
	// Notifier is the queue. Nil sends nothing, which is what an installation with no mail server
	// configured does - and it still answers the same `202`.
	Notifier Notifier
	// Multi is decision 3's mode switch, SessionWriter's.
	Multi bool
	// Tenants resolves the host.
	Tenants repository.TenantDirectory
}

// Execute queues a link where there is somewhere to send one, and answers nothing either way.
//
// Every failure behind the answer is swallowed deliberately: an address nobody holds, an account
// that signs in through a provider, a queue that refused the job. What a caller learns is that the
// request arrived, which is all there is to learn.
func (h ForgetPassword) Execute(ctx context.Context, cmd ForgetPasswordCommand) error {
	tenantID, err := h.resolveTenant(ctx, cmd.TenantSlug, cmd.TenantHeader)
	if err != nil || tenantID.IsZero() {
		// No workspace answers here. The same silence, for the same reason.
		return nil //nolint:nilerr // the answer is the same whatever went wrong behind it (T-02)
	}

	address := domain.LookupAddress(cmd.Email, h.Writer.Session.Domains)
	if address == "" {
		return nil
	}

	var found repository.PasswordAccount
	err = h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			read, err := h.Writer.Accounts.FindByEmailForPassword(ctx, address)
			if err != nil {
				if errors.Is(err, shared.ErrNotFound) {
					return nil
				}
				return err
			}
			found = read
			return nil
		})
	if err != nil || found.Account.ID.IsZero() {
		return nil //nolint:nilerr // the answer is the same whatever went wrong behind it (T-02)
	}
	if err := found.Account.Verify(); err != nil {
		// A disabled account gets no link. Silently: whether an address is disabled is as much a
		// probe's question as whether it exists.
		return nil //nolint:nilerr // same answer, same reason
	}

	// The trail entry and the job in one transaction: a request recorded without a mail queued
	// would be a trail saying somebody asked and nothing happening, and the other way round would
	// be a mail with no record of who asked for it.
	if err := h.queue(ctx, tenantID, found.Account); err != nil {
		// Logged and swallowed. The caller's answer must not depend on this - it is the same `202`
		// either way - but an operator whose queue is refusing jobs has to be able to find out,
		// and silence at both ends is how a mail server nobody noticed stays unnoticed.
		slog.WarnContext(ctx, "queueing a password reset failed",
			slog.String("error", err.Error()))
	}
	return nil
}

// queue writes the trail entry and the job together.
func (h ForgetPassword) queue(
	ctx context.Context, tenantID shared.ID, account domain.Account,
) error {
	return h.Writer.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			if err := h.record(ctx, tenantID, account); err != nil {
				return err
			}
			if h.Notifier == nil {
				return nil
			}
			_, enqueued := h.Notifier.Enqueue(ctx, queue.Request{
				Kind:     queue.KindPasswordResetEmail,
				TenantID: tenantID,
				// One pending link per account: a person who presses the button twice gets one
				// mail, and the job already waiting is the one that sends it.
				DedupeKey: account.ID.String(),
				// Identifiers only (rule 10). The address is on the row the job reads, and a
				// payload carrying one would put it in the queue table and in every log line
				// about the job.
				Payload: map[string]any{"account_id": account.ID.String()},
			})
			return enqueued
		})
}

// record writes the trail entry, inside the caller's transaction. Nothing is written for an address
// nobody holds, and the reason is the same one the answer gives: there is no workspace to write it in.
func (h ForgetPassword) record(
	ctx context.Context, tenantID shared.ID, account domain.Account,
) error {
	sink := h.Writer.Session.Audit
	if sink == nil {
		return nil
	}
	return sink.Append(ctx, audit.Entry{
		TenantID:   tenantID,
		OccurredAt: h.Writer.Clock.Now(),
		Action:     PasswordResetRequestedAction,
		Outcome:    audit.OutcomeSuccess,
		Severity:   audit.SeverityNotice,
		// The account itself, not whoever pressed the button: nobody has signed in, and
		// the person this concerns is the one the address names.
		ActorKind:  appshared.ActorUser,
		ActorID:    account.ID,
		ActorLabel: account.DisplayName,
		TargetType: accountTargetForPassword,
		TargetID:   account.ID,
		Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

func (h ForgetPassword) resolveTenant(ctx context.Context, slug, header string) (shared.ID, error) {
	lookup := slug
	if !h.Multi {
		lookup = ""
	} else if slug == "" {
		if header == "" {
			return "", nil
		}
		return shared.ParseID(header)
	}

	var tenantID shared.ID
	err := h.Writer.UnitOfWork.WithinReadOnly(ctx, persistence.InstallationScope(),
		func(ctx context.Context) error {
			id, err := h.Tenants.Resolve(ctx, lookup)
			tenantID = id
			return err
		})
	return tenantID, err
}

// Descriptor is the catalogue entry.
func (h ForgetPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ForgetPasswordName,
		Summary: "Asks for a password reset link. It answers the same thing for an address that " +
			"holds an account and for one that does not - which addresses have accounts is " +
			"exactly what a probe is after. Behind that one answer sits a job on the queue the " +
			"invitation already uses, so an unreachable mail server never fails the request.",
		SideEffects: "Queues a mail where there is somewhere to send one. Nothing is written to " +
			"the account, and nothing about the outcome reaches the caller.",
		Input: []usecase.Field{
			{
				Name: "email", Kind: usecase.KindString, Required: true,
				Description: "The address to send the link to, if it holds an account.",
			},
			{
				Name: "tenant_slug", Kind: usecase.KindString,
				Description: "The subdomain the request arrived under, in multi mode.",
			},
			{
				Name: "tenant_header", Kind: usecase.KindString,
				Description: "The X-Hubtask-Tenant header, when sent.",
			},
		},
		// Recorded only where an address held an account: an entry per *request* would be a list
		// of the addresses somebody guessed at, written by the guesser, and there is no workspace
		// trail to write one in for an address nobody holds.
		Audit: usecase.AuditDeclaration{
			Action: PasswordResetRequestedAction, TargetType: accountTargetForPassword,
			Severity: audit.SeverityNotice, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A reset request is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ForgetPassword) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, ForgetPasswordCommand{
		Email:        in.String("email"),
		TenantSlug:   in.String("tenant_slug"),
		TenantHeader: in.String("tenant_header"),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// MintResetToken mints the credential a reset mail carries.
//
// Minted at delivery rather than at the request, MintRedemptionToken's reasoning verbatim: the
// queue's payload carries identifiers only, and a token minted beside the request would have to
// travel through it. A delivery retried mints again and the newest token wins.
type MintResetToken struct{ Writer PasswordWriter }

// ResetLink is what the mail needs: the plaintext token, and whether this account signs in with a
// password at all.
type ResetLink struct {
	Token secret.Secret
	// HasPassword is false for an account that signs in through a provider. The mail then says so
	// rather than carrying a link to a password screen that would set a second way in.
	HasPassword bool
	// First is the exception (ADR-0077 §3 and §4): an account without a password that no provider
	// lets in here any more - its workspace's last way in was an offer that ended, or the provider
	// it is connected to is gone or switched off while the password is on. There is no provider to
	// point to, so the mail carries a link to set a first password.
	First bool
	// Connect is the other exception (ADR-0078 §1): in a workspace that switched the password off,
	// an active account that no provider switched on there lets in - one that holds a password but
	// was never connected, one whose only identity is at an offer that ended, one that holds no
	// credential at all. The mail carries a link to connect the workspace's provider: the link and a
	// fresh sign-in there are the account's proof at the provider's first arrival.
	Connect bool
	Address string
	Locale  string
}

// MintResetToken draws, stores the hash, and answers the plaintext for the mail.
func (m MintResetToken) MintResetToken(
	ctx context.Context, tenantID, accountID shared.ID,
) (ResetLink, error) {
	w := m.Writer

	held, err := w.AccountFor(ctx, tenantID, accountID)
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			// The account went away between the request and the job. Nothing to send, and not a
			// failure - a link for somebody who no longer exists is finished business.
			return ResetLink{}, nil
		}
		return ResetLink{}, err
	}
	link := ResetLink{
		Address:     held.Account.Email,
		Locale:      held.Account.Locale,
		HasPassword: !held.PasswordHash.IsEmpty(),
	}
	// A workspace that switched the password off gets the mail that points to its provider, for
	// every account in it (SC-24): a link to set a password there would offer a closed door.
	open, fallback, err := w.PasswordOpen(ctx, tenantID)
	if err != nil {
		return ResetLink{}, err
	}
	if !open {
		link.HasPassword = false
		// The provider mail points to a provider that lets the account in; where none does, the
		// mailbox is the way back - to connect one, since the password is not a way in here.
		connect, err := w.connectOpens(ctx, tenantID, held.Account)
		if err != nil {
			return ResetLink{}, err
		}
		link.Connect = connect
	}
	// Nobody is left without a way in (ADR-0077 §3, §4): where no provider lets this account in any
	// more, the mail would point to nothing, and the mailbox is the proof a reset accepts from every
	// other account.
	if open && !link.HasPassword {
		first, err := w.firstPasswordOpens(ctx, tenantID, accountID, fallback.Opens())
		if err != nil {
			return ResetLink{}, err
		}
		link.First = first
	}
	if !link.HasPassword && !link.First && !link.Connect {
		return link, nil
	}
	purpose := domain.PendingReset
	if link.Connect {
		// The reset link's window and discipline: the mailbox is the same proof, read the same way.
		purpose = domain.PendingConnect
	}

	material, err := w.Session.Entropy.Bytes(domain.TokenSecretBytes)
	if err != nil {
		return ResetLink{}, shared.ErrInternal.WithDetail("auth.session_unmintable").WithCause(err)
	}
	presented, err := domain.NewPendingToken(tenantID, material)
	if err != nil {
		return ResetLink{}, err
	}

	err = w.UnitOfWork.Within(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			now := w.Clock.Now()
			// A second request replaces the first link rather than adding one (UC-ID-04 check 2):
			// whichever mail the person opens, only the newest works - a link to set a password and
			// one to connect a provider alike, since the workspace's switch decides which is mailed.
			for _, earlier := range []domain.PendingPurpose{domain.PendingReset, domain.PendingConnect} {
				if _, err := w.Pending.Supersede(ctx, accountID, earlier, now); err != nil {
					return err
				}
			}
			return w.Pending.Insert(ctx, domain.PendingCredential{
				ID:        w.IDs.NewID(),
				TenantID:  tenantID,
				AccountID: accountID,
				Purpose:   purpose,
				CreatedAt: now.UTC(),
				ExpiresAt: now.Add(domain.ResetLifetime).UTC(),
			}, presented)
		})
	if err != nil {
		return ResetLink{}, err
	}

	link.Token = secret.New(presented.Secret())
	return link, nil
}

// firstPasswordOpens answers whether an account without a password may set one through the reset:
// under the fallback, or where none of the providers it is connected to is a way in here now. A
// writer that cannot ask - built without the provider stores, as tests of other doors are - answers
// only the fallback, the shape before ADR-0077 §4.
func (w PasswordWriter) firstPasswordOpens(
	ctx context.Context, tenantID, accountID shared.ID, fallback bool,
) (bool, error) {
	if fallback {
		return true, nil
	}
	p := w.Session.StepUpProviders
	if p.Providers == nil || p.External == nil {
		return false, nil
	}
	var reaches bool
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			_, found, err := p.connectedHere(ctx, accountID, w.Clock.Now())
			reaches = found
			return err
		})
	return !reaches, err
}

// connectOpens answers whether an account in a workspace without the password is mailed a link to
// connect a provider (ADR-0078 §1): an active person whom no provider switched on here lets in, while
// one is switched on to connect. An account connected to a provider that works here keeps the mail
// that points to it. A writer that cannot ask - built without the provider stores, as tests of other
// doors are - answers no, the shape before SC-33.
func (w PasswordWriter) connectOpens(
	ctx context.Context, tenantID shared.ID, account domain.Account,
) (bool, error) {
	if account.Status != domain.AccountActive || account.Kind == domain.AccountServiceAccount {
		return false, nil
	}
	p := w.Session.StepUpProviders
	if p.Providers == nil || p.External == nil {
		return false, nil
	}
	var opens bool
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID},
		func(ctx context.Context) error {
			stands, err := p.connectStands(ctx, account.ID, w.Clock.Now())
			opens = stands
			return err
		})
	return opens, err
}

// ResetPasswordCommand carries the spent link and the new password.
type ResetPasswordCommand struct {
	Token    secret.Secret
	Password secret.Secret
	// UserAgent and RemoteAddr are recorded on the session the reset opens.
	UserAgent  string
	RemoteAddr string
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// ResetPassword spends the link and sets the password.
type ResetPassword struct{ Writer PasswordWriter }

// Execute resets. The token dies whatever happens next; every session of the account ends; and
// where a second factor is armed the answer is the challenge rather than the pair.
func (h ResetPassword) Execute(
	ctx context.Context, cmd ResetPasswordCommand,
) (SignInResult, error) {
	w := h.Writer

	token, err := domain.ParsePendingToken(cmd.Token.Reveal())
	if err != nil {
		return SignInResult{}, resetRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SignInResult{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}

	tenantID := token.TenantID()
	scope := persistence.Scope{TenantID: tenantID}
	// A link mailed before the workspace switched the password off sets nothing after (SC-24).
	open, fallback, err := w.PasswordOpen(ctx, tenantID)
	if err := refuseShut(open, fallback, err); err != nil {
		return SignInResult{}, err
	}

	var lookup repository.PendingLookup
	err = w.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		read, err := w.Pending.FindByToken(ctx, token)
		lookup = read
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return SignInResult{}, resetRefused()
		}
		return SignInResult{}, err
	}

	now := w.Clock.Now()
	// Unknown, spent and expired are one refusal: which of the three applies is not for the holder
	// of a link they found somewhere to learn.
	if lookup.Credential.Purpose != domain.PendingReset || lookup.Credential.Verify(now) != nil {
		return SignInResult{}, resetRefused()
	}
	if err := lookup.Account.Verify(); err != nil {
		return SignInResult{}, err
	}
	if err := lookup.TenantStatus.Verify(); err != nil {
		return SignInResult{}, err
	}

	held, err := w.AccountFor(ctx, tenantID, lookup.Account.ID)
	if err != nil {
		return SignInResult{}, err
	}
	// A first password is set only while no provider lets the account in - the condition the link
	// was mailed under (ADR-0077 §3, §4). Once one does again, the link is a spent one.
	if held.PasswordHash.IsEmpty() {
		first, err := w.firstPasswordOpens(ctx, tenantID, held.Account.ID, fallback.Opens())
		if err != nil {
			return SignInResult{}, err
		}
		if !first {
			return SignInResult{}, resetRefused()
		}
	}
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return SignInResult{}, err
	}
	candidate := PasswordCandidate{
		TenantID: tenantID, Account: held, Password: cmd.Password, Rules: rules,
		// A reset ignores the minimum age, and says so to the rule rather than skipping the check:
		// somebody who has lost their password cannot be told to wait a day for it.
		IsReset: true,
	}
	if err := w.Judge(ctx, candidate); err != nil {
		return SignInResult{}, err
	}

	// The token dies before the password moves. A password set under a link that a race had
	// already spent would be a password set twice from one mail.
	var spent bool
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		consumed, err := w.Pending.Consume(ctx, lookup.Credential.ID, now)
		spent = consumed
		return err
	})
	if err != nil {
		return SignInResult{}, err
	}
	if !spent {
		return SignInResult{}, resetRefused()
	}

	// Every session, this time: the person asking for a reset is saying the old password may be
	// known, and there is no session of theirs to keep - they have none.
	if err := w.Write(ctx, candidate, "", true); err != nil {
		return SignInResult{}, err
	}

	// A session the fallback opens is recorded as one, whichever step follows - the sign-in's rule.
	if fallback.Opens() {
		if err := w.Session.recordFallback(ctx, scope, lookup.Account, fallback); err != nil {
			return SignInResult{}, err
		}
	}
	// The address from the account's own row: the pending credential's read carries none.
	return h.answer(ctx, scope, tenantID, lookup.Account, held.Account.Email, cmd)
}

// answer opens the session, or hands back the second step where the account demands one.
func (h ResetPassword) answer(
	ctx context.Context, scope persistence.Scope, tenantID shared.ID,
	account domain.Account, address string, cmd ResetPasswordCommand,
) (SignInResult, error) {
	session := h.Writer.Session

	challenge, verdict, err := session.challengeFor(ctx, scope, account, SignInCommand{
		UserAgent: cmd.UserAgent, RemoteAddr: cmd.RemoteAddr,
		// The password was just set and judged in full, so the change step must not fire again on
		// the very password the rule accepted. The empty candidate is what says so: nothing to
		// judge, and the verdict's other answers - the factor and the bounds - are what is wanted.
		Password: secret.Secret{},
	}, nil)
	if err != nil {
		return SignInResult{}, err
	}
	if challenge != nil {
		// The person arrived from the link in the mail and typed no address, so the step says
		// whose account it is: the card's identity line, and its "Not you?" (UC-ID-04 check 5).
		// Holding the link proved the mailbox, so the address tells them nothing they lack.
		challenge.Email = address
		return SignInResult{Challenge: challenge}, nil
	}

	pair, err := session.openSessionWith(ctx, scope, tenantID, account,
		cmd.UserAgent, cmd.RemoteAddr, SignedInAction, nil,
		verdict.Sessions, domain.SignedInWithReset)
	if err != nil {
		return SignInResult{}, err
	}
	return SignInResult{Pair: &pair}, nil
}

// resetRefused is the reset's one probe-facing refusal.
func resetRefused() error {
	return shared.ErrUnauthenticated.WithDetail("auth.reset_failed")
}

// Descriptor is the catalogue entry.
func (h ResetPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ResetPasswordName,
		Summary: "Spends a reset link and sets the password. The link works once and for half an " +
			"hour; unknown, spent and expired are one indistinguishable refusal. Every session " +
			"of the account ends. Where a second factor is armed the answer is the challenge " +
			"rather than the pair: control of a mailbox is one proof, and it does not replace " +
			"the one the account already demanded.",
		SideEffects: "Spends the link, stores the new hash and its moment, records the previous " +
			"hash in the history, ends every session, and writes an audit entry.",
		Input: []usecase.Field{
			{
				Name: "token", Kind: usecase.KindString, Required: true,
				Description: "The token from the reset mail. It dies on use.",
			},
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The new password, judged against this workspace's rule.",
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
			Action: PasswordChangedAction, TargetType: accountTargetForPassword,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A password is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ResetPassword) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	result, err := h.Execute(ctx, ResetPasswordCommand{
		Token:        secret.New(in.String("token")),
		Password:     secret.New(in.String("password")),
		UserAgent:    in.String("user_agent"),
		RemoteAddr:   in.String("remote_addr"),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	if result.Challenge != nil {
		return challengeOutput(*result.Challenge), nil
	}
	return pairOutput(*result.Pair), nil
}
