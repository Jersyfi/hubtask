// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"strconv"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	stepupport "github.com/Jersyfi/hubtask/core/port/stepup"
	"github.com/Jersyfi/hubtask/core/port/text"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// One use case sets every password (ADR-0068 §5).
//
// Four doors reach it - an invitation token, a reset token, the pending credential of a sign-in the
// change step interrupted, and a bearer with a step-up - and behind all four there is one policy
// check, one audit action, one place the history is written, and one moment recorded. Four use cases
// that each set a password would be four places for the rule to be applied slightly differently,
// and the fourth would be the one that forgot the blocklist.
//
// **The step-up is the proof of the old password**, which is why the bearer door has no "current
// password" field beside it: asking for both is asking twice for one thing (WCAG 2.2 SC 3.3.7).

const (
	ChangePasswordName = "ChangePassword"
	CheckPasswordName  = "CheckPassword"

	accountTargetForPassword = "account"
)

// PasswordChangedAction is a password replaced, whichever door it came through. Warning rather than
// notice: a password change is what an account takeover looks like from the trail, and a reader
// scanning for one should see it (audit.md §2).
const PasswordChangedAction audit.Action = "account.password_changed"

// PasswordBlocklist is the operator's own list of refused passwords, read offline (security.md §5).
//
// A port rather than a file path in the service, because what an installation points at is its
// business: a text file today, and whatever a later milestone wires. Nil is "no list configured",
// which is not an error - `sign_in.blocklist_file` is how an installation says it wants one.
type PasswordBlocklist interface {
	// Contains reports whether the folded candidate carries an entry. The folding is the domain's
	// and has already happened: an implementation compares, it does not interpret.
	Contains(ctx context.Context, folded string) (bool, error)
}

// BreachCorpus is the set of passwords a known breach exposed. Nil is "none configured", and the
// rule then resolves off however the switch is set - a line under a field that nothing can answer
// would be a line nobody can ever meet.
type BreachCorpus interface {
	// Contains reports whether the candidate appears in the corpus. The candidate travels as a
	// secret, because an implementation that reaches a network must be the one deciding what it
	// sends - and today's does not reach one.
	Contains(ctx context.Context, candidate secret.Secret) (bool, error)
}

// PasswordWriter is what every door shares. One dependency set, SessionWriter's shape, because the
// rules about one password belong in one place.
type PasswordWriter struct {
	// Session carries the hasher, the audit sink, the clock, the sessions and the attempt ledger.
	// Embedded as a value rather than duplicated: the doors that open a session need all of it, and
	// the door that does not still needs the hasher and the trail.
	Session SessionWriter
	// Resolver is the three levels of the rule.
	Resolver SignInPolicyResolver

	Accounts  repository.PasswordAccounts
	Histories repository.PasswordHistories
	Pending   repository.PendingCredentials
	// StepUp is the bearer door's proof (H-03).
	StepUp stepupport.Verifier

	// Blocklist and Breach are the two optional corpora. Both nil on a plain installation.
	Blocklist PasswordBlocklist
	Breach    BreachCorpus

	// Text is NFKC. Applied before the rule counts anything and before the hash is computed, so
	// that what was counted is what is stored (ADR-0068 §7). Named as every other constructor that
	// stores text names it, which is what the gate M-07 left behind checks.
	Text       text.Normalizer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	IDs        clock.IDGenerator
}

// PasswordCandidate is one password on its way in, with who is setting it and under what rule.
type PasswordCandidate struct {
	TenantID shared.ID
	Account  repository.PasswordAccount
	Password secret.Secret
	// IsReset says the moment came from a mailbox rather than from a session. It is the one thing
	// that changes the rule: the minimum age is ignored, because somebody who has lost their
	// password cannot be told to wait a day for it (ADR-0068 §6).
	IsReset bool
	// Rules is the rule in force, resolved once by the caller so that a door which already has it
	// does not read it twice.
	Rules ResolvedPolicy
}

// Judge answers every rule the candidate breaks, local and server-side, as one refusal.
//
// Local first and the server's behind them, in the order a screen draws them: the list then tells a
// reader where they are. A caller that has already had the local rules predicted still gets them
// checked - a prediction is a courtesy and this is the verdict.
func (w PasswordWriter) Judge(ctx context.Context, candidate PasswordCandidate) error {
	policy := candidate.Rules.Effective.Policy.Password
	plain := candidate.Password.Reveal()

	if err := domain.CheckPasswordAgainst(policy, w.Text, plain, w.contextOf(candidate)); err != nil {
		// Length, the classes, the repetition and the context words: everything a client could
		// have predicted, refused with the same codes it predicted with.
		return err
	}

	violations, err := w.serverViolations(ctx, candidate)
	if err != nil {
		return err
	}
	return domain.PasswordRefused(violations)
}

// serverViolations answers the rules only this side can decide: the two lists, the corpus, the
// history and the one in force.
func (w PasswordWriter) serverViolations(
	ctx context.Context, candidate PasswordCandidate,
) ([]domain.PasswordViolation, error) {
	policy := candidate.Rules.Effective.Policy.Password
	plain := candidate.Password.Reveal()
	violations := make([]domain.PasswordViolation, 0, 4)

	if policy.CommonPasswords {
		common := domain.IsCommonPassword(w.Text, plain)
		if !common && w.Blocklist != nil {
			found, err := w.Blocklist.Contains(ctx, domain.FlattenPassword(w.Text, plain))
			if err != nil {
				return nil, err
			}
			common = found
		}
		if common {
			violations = append(violations, domain.PasswordViolation{Rule: domain.RuleCommon})
		}
	}

	if policy.BreachCheck && w.Breach != nil {
		found, err := w.Breach.Contains(ctx, candidate.Password)
		if err != nil {
			return nil, err
		}
		if found {
			violations = append(violations, domain.PasswordViolation{Rule: domain.RuleBreach})
		}
	}

	held, err := w.historyViolations(ctx, candidate)
	if err != nil {
		return nil, err
	}
	return append(violations, held...), nil
}

// historyViolations compares against the current hash and the remembered ones.
//
// Argon2 comparisons, which is what bounds the depth at ten and what makes the check route's own
// rate-limit bucket necessary. The comparison is the verifier's, so a hash written under older
// parameters still answers - which is the same property that lets sign-in re-apply them.
func (w PasswordWriter) historyViolations(
	ctx context.Context, candidate PasswordCandidate,
) ([]domain.PasswordViolation, error) {
	violations := make([]domain.PasswordViolation, 0, 2)
	normalised := secret.New(domain.NormalisePassword(w.Text, candidate.Password.Reveal()))

	if !candidate.Account.PasswordHash.IsEmpty() {
		same, err := w.Session.Passwords.Verify(candidate.Account.PasswordHash.Reveal(), normalised)
		if err != nil {
			return nil, err
		}
		if same {
			// Its own rule rather than the history's: "you already have this one" and "you used it
			// a while ago" are different sentences, and a reader acts on them differently.
			violations = append(violations, domain.PasswordViolation{Rule: domain.RuleNotCurrent})
		}
	}

	depth := candidate.Rules.Effective.Policy.Password.HistoryCount
	if depth <= 0 || w.Histories == nil {
		return violations, nil
	}

	var hashes []secret.Secret
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: candidate.TenantID},
		func(ctx context.Context) error {
			read, err := w.Histories.Recent(ctx, candidate.Account.Account.ID, depth)
			hashes = read
			return err
		})
	if err != nil {
		return nil, err
	}
	for _, hash := range hashes {
		same, err := w.Session.Passwords.Verify(hash.Reveal(), normalised)
		if err != nil {
			return nil, err
		}
		if same {
			violations = append(violations, domain.HistoryViolation(depth))
			break
		}
	}
	return violations, nil
}

// contextOf builds the words a password may not carry, from who is setting it and where.
func (w PasswordWriter) contextOf(candidate PasswordCandidate) domain.PasswordContext {
	return domain.PasswordContext{
		Email:         candidate.Account.Account.Email,
		DisplayName:   candidate.Account.Account.DisplayName,
		WorkspaceName: candidate.Rules.Workspace.DisplayName,
		WorkspaceHost: candidate.Rules.Workspace.Slug,
	}
}

// MinimumAge refuses a change that comes too soon after the last one.
//
// Its own method rather than a rule in the list: it is not a property of the password, so a line
// under the field saying "not within six hours" would be a line about the clock. A reset ignores it
// entirely - somebody who has lost their password cannot be told to wait (ADR-0068 §6).
func (w PasswordWriter) MinimumAge(candidate PasswordCandidate) error {
	hours := candidate.Rules.Effective.Policy.Password.MinAgeHours
	if candidate.IsReset || hours <= 0 || candidate.Account.PasswordSetAt.IsZero() {
		return nil
	}
	earliest := candidate.Account.PasswordSetAt.Add(time.Duration(hours) * time.Hour)
	if !w.Clock.Now().Before(earliest) {
		return nil
	}
	return shared.ErrValidation.
		WithDetail("auth.password_too_young").
		WithParams(map[string]string{"hours": strconv.Itoa(hours)}).
		WithFields(shared.FieldError{
			Path: "/password", Code: "auth.password_too_young",
			Params: map[string]string{"hours": strconv.Itoa(hours)},
		})
}

// Write is the one place a password is stored (ADR-0068 §5).
//
// In one transaction: the hash and its moment, the previous hash into the history, the history
// trimmed to the policy, and the audit entry. The hashing happens outside it, Argon2id being
// deliberately slow and a held connection being what turns a burst of sign-ins into an outage.
//
// `endOtherSessions` is the caller's, because the four doors differ on exactly that: a change from a
// profile ends every other session and keeps this one, a reset ends all of them including the
// caller's, and a first password has none to end.
func (w PasswordWriter) Write(
	ctx context.Context, candidate PasswordCandidate, keepSessionID shared.ID, endOtherSessions bool,
) error {
	normalised := domain.NormalisePassword(w.Text, candidate.Password.Reveal())
	hash, err := w.Session.Passwords.Hash(secret.New(normalised))
	if err != nil {
		return err
	}

	account := candidate.Account.Account
	scope := persistence.Scope{TenantID: candidate.TenantID, ActorID: account.ID}
	return w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		now := w.Clock.Now()

		depth := candidate.Rules.Effective.Policy.Password.HistoryCount
		if depth > 0 && w.Histories != nil && !candidate.Account.PasswordHash.IsEmpty() {
			at := candidate.Account.PasswordSetAt
			if at.IsZero() {
				// The row predates the column. Recorded at the moment it stopped being current,
				// which is the only moment this transaction can honestly claim to know.
				at = now
			}
			if err := w.Histories.Append(ctx, w.IDs.NewID(), account.ID,
				candidate.Account.PasswordHash.Reveal(), at); err != nil {
				return err
			}
			if err := w.Histories.Trim(ctx, account.ID, depth); err != nil {
				return err
			}
		}

		written, err := w.Accounts.SetPassword(ctx, account.ID, hash, now)
		if err != nil {
			return err
		}
		if !written {
			return shared.ErrNotFound.WithDetail("accounts.not_found")
		}

		if endOtherSessions {
			if err := w.endOthers(ctx, account.ID, keepSessionID, now); err != nil {
				return err
			}
		}

		return w.Session.Audit.Append(ctx, audit.Entry{
			TenantID:   candidate.TenantID,
			OccurredAt: now,
			Action:     PasswordChangedAction,
			Outcome:    audit.OutcomeSuccess,
			Severity:   audit.SeverityWarning,
			ActorKind:  appshared.ActorUser,
			ActorID:    account.ID,
			ActorLabel: account.DisplayName,
			TargetType: accountTargetForPassword,
			TargetID:   account.ID,
			Context:    audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
			// The moment and nothing else. Not the password, not its length, not its shape: a
			// trail that narrowed the search space would be a trail worth stealing (rule 10).
			Changes: audit.Changes(audit.Change{
				Field: "password_set_at", Classification: audit.Open,
				To: now.UTC().Format(time.RFC3339),
			}),
		})
	})
}

// endOthers ends every session of the account but the one the caller is holding.
//
// Every one of them, and then the kept one re-opened by nothing: `RevokeAll` is the only statement
// that can promise none was missed, so the caller's is revoked with the rest and this method says so
// rather than pretending otherwise - except that `keepSessionID`, when given, is revoked last and
// re-recorded by the caller. Personal access tokens are untouched: they are their own credentials
// with their own expiry and their own list, and a person who minted one did not mint it in a browser.
func (w PasswordWriter) endOthers(
	ctx context.Context, accountID, keepSessionID shared.ID, now time.Time,
) error {
	live, err := w.Session.Sessions.ForAccount(ctx, accountID, now)
	if err != nil {
		return err
	}
	for _, session := range live {
		if !keepSessionID.IsZero() && session.ID == keepSessionID {
			continue
		}
		if _, err := w.Session.Sessions.Revoke(ctx, session.ID, accountID, now); err != nil {
			return err
		}
	}
	return nil
}

// ResolveFor reads the rule in force for one workspace.
func (w PasswordWriter) ResolveFor(ctx context.Context, tenantID shared.ID) (ResolvedPolicy, error) {
	return w.Resolver.Resolve(ctx, tenantID)
}

// AccountFor reads the account a door names, with its hash and the moment.
func (w PasswordWriter) AccountFor(
	ctx context.Context, tenantID, accountID shared.ID,
) (repository.PasswordAccount, error) {
	var held repository.PasswordAccount
	err := w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: tenantID, ActorID: accountID},
		func(ctx context.Context) error {
			read, err := w.Accounts.FindForPassword(ctx, accountID)
			held = read
			return err
		})
	if err != nil {
		return repository.PasswordAccount{}, err
	}
	return held, nil
}

// ChangePasswordCommand is the bearer door.
type ChangePasswordCommand struct {
	Password secret.Secret
	// StepUpToken is the proof of the old password (H-03). Demanded, because a bearer alone is a
	// stolen tab.
	StepUpToken string
}

// ChangePassword is `POST /auth/password`: the password of the signed-in account, behind a step-up.
type ChangePassword struct{ Writer PasswordWriter }

// Execute changes it, and ends every other session of the account.
func (h ChangePassword) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd ChangePasswordCommand,
) error {
	w := h.Writer
	if !actor.IsAuthenticated() || actor.AccountID.IsZero() {
		return shared.ErrUnauthenticated.WithDetail("access.credential_required")
	}
	// Demand names the methods on the refusal, which is the whole of what the prompt is built
	// from: without them a person with an authenticator was asked for their password.
	if err := stepupport.Demand(
		ctx, w.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
		return err
	}

	candidate, err := h.candidateFor(ctx, actor, cmd.Password)
	if err != nil {
		return err
	}
	if err := w.MinimumAge(candidate); err != nil {
		return err
	}
	if err := w.Judge(ctx, candidate); err != nil {
		return err
	}
	// This session stays and every other one goes: a person changing their password is telling us
	// the old one may be known, and the other sessions are what it opened.
	return w.Write(ctx, candidate, actor.TokenID, true)
}

func (h ChangePassword) candidateFor(
	ctx context.Context, actor appshared.ActorContext, password secret.Secret,
) (PasswordCandidate, error) {
	w := h.Writer
	account, err := w.AccountFor(ctx, actor.TenantID, actor.AccountID)
	if err != nil {
		return PasswordCandidate{}, err
	}
	if account.PasswordHash.IsEmpty() {
		// An account that signs in through a provider has no password to change, and giving it one
		// here would be a second way in that nobody asked for.
		return PasswordCandidate{}, shared.ErrConflict.WithDetail("auth.password_sign_in_unavailable")
	}
	rules, err := w.ResolveFor(ctx, actor.TenantID)
	if err != nil {
		return PasswordCandidate{}, err
	}
	return PasswordCandidate{
		TenantID: actor.TenantID, Account: account, Password: password, Rules: rules,
	}, nil
}

// Descriptor is the catalogue entry.
func (h ChangePassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: ChangePasswordName,
		Summary: "Changes the password of the signed-in account, behind a step-up. The step-up " +
			"*is* the proof of the old password, which is why there is no current-password " +
			"field beside it: asking for both is asking twice for one thing. Every other " +
			"session of the account is ended and this one is not; personal access tokens keep " +
			"working, because they are their own credentials with their own expiry.",
		SideEffects: "Stores the new hash and its moment, records the previous hash in the " +
			"history, ends every other session, and writes an audit entry.",
		Input: []usecase.Field{
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The new password, judged against this workspace's rule.",
			},
			{
				// Declared but not required: the registry's own refusal for a missing input is a
				// `422` about a field, and what a caller without a proof needs is the `403` that
				// carries `auth.step_up_required` - the code the client renders as "prove it is
				// you first". The use case refuses it, one line in, with that code.
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "The proof from `/auth/step-up`. It is consumed by this call, and " +
					"the call is refused without it.",
			},
		},
		Audit: usecase.AuditDeclaration{
			Action: PasswordChangedAction, TargetType: accountTargetForPassword,
			Severity: audit.SeverityWarning, Required: true,
		},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A password is not an entry, and the item history is keyed on an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ChangePassword) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	if err := h.Execute(ctx, actor, ChangePasswordCommand{
		Password:    secret.New(in.String("password")),
		StepUpToken: in.String("step_up_token"),
	}); err != nil {
		return nil, err
	}
	return usecase.Output{}, nil
}

// CheckPasswordCommand is what the check route received: the candidate, and the same proof the
// setting demands.
type CheckPasswordCommand struct {
	Password secret.Secret
	// One of the four proofs. Without any of them the route is an oracle that tells anybody
	// whether a word is on a blocklist or in somebody's history, which is why it refuses.
	StepUpToken     string
	PendingToken    secret.Secret
	InvitationToken secret.Secret
	ResetToken      secret.Secret
}

// CheckPassword is `POST /auth/password:check`: what only the server knows about a candidate.
type CheckPassword struct{ Writer PasswordWriter }

// Execute answers the rules this candidate breaks, as identifiers and never as sentences.
//
// It answers `200` with a list rather than a refusal, because it is a question and not an attempt:
// a client asks it while somebody is still typing, and a `422` would be an error state on a form
// nobody has submitted.
func (h CheckPassword) Execute(
	ctx context.Context, actor appshared.ActorContext, cmd CheckPasswordCommand,
) ([]domain.PasswordViolation, error) {
	w := h.Writer

	candidate, err := h.candidateFor(ctx, actor, cmd)
	if err != nil {
		return nil, err
	}
	return w.serverViolations(ctx, candidate)
}

// candidateFor resolves the proof into the account whose rule and history apply.
func (h CheckPassword) candidateFor(
	ctx context.Context, actor appshared.ActorContext, cmd CheckPasswordCommand,
) (PasswordCandidate, error) {
	w := h.Writer

	switch {
	case cmd.StepUpToken != "" && actor.IsAuthenticated() && !actor.AccountID.IsZero():
		if err := stepupport.Demand(
			ctx, w.StepUp, actor.TenantID, actor.AccountID, cmd.StepUpToken); err != nil {
			return PasswordCandidate{}, err
		}
		return h.forAccount(ctx, actor.TenantID, actor.AccountID, cmd.Password, false)
	case actor.IsAuthenticated() && !actor.AccountID.IsZero():
		// A bearer alone is enough to ask about one's *own* password: the answer is about the
		// account the caller already holds, and demanding the step-up here would make the field
		// prompt for a password before it could say anything about one. The step-up is demanded
		// where the password is *set*.
		return h.forAccount(ctx, actor.TenantID, actor.AccountID, cmd.Password, false)
	case !cmd.PendingToken.IsEmpty():
		return h.forPending(ctx, cmd.PendingToken, cmd.Password)
	case !cmd.ResetToken.IsEmpty():
		return h.forPending(ctx, cmd.ResetToken, cmd.Password)
	case !cmd.InvitationToken.IsEmpty():
		return h.forInvitation(ctx, cmd.InvitationToken, cmd.Password)
	}
	return PasswordCandidate{}, shared.ErrUnauthenticated.WithDetail("auth.password_check_proof_required")
}

func (h CheckPassword) forAccount(
	ctx context.Context, tenantID, accountID shared.ID, password secret.Secret, isReset bool,
) (PasswordCandidate, error) {
	w := h.Writer
	account, err := w.AccountFor(ctx, tenantID, accountID)
	if err != nil {
		return PasswordCandidate{}, err
	}
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return PasswordCandidate{}, err
	}
	return PasswordCandidate{
		TenantID: tenantID, Account: account, Password: password, Rules: rules, IsReset: isReset,
	}, nil
}

// forPending resolves a pending credential without consuming it: this is a question, and a question
// that spent the credential would end the sign-in it was asked during.
func (h CheckPassword) forPending(
	ctx context.Context, presented secret.Secret, password secret.Secret,
) (PasswordCandidate, error) {
	w := h.Writer
	token, err := domain.ParsePendingToken(presented.Reveal())
	if err != nil {
		return PasswordCandidate{}, challengeRefused()
	}

	var lookup repository.PendingLookup
	err = w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: token.TenantID()},
		func(ctx context.Context) error {
			read, err := w.Pending.FindByToken(ctx, token)
			lookup = read
			return err
		})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return PasswordCandidate{}, challengeRefused()
		}
		return PasswordCandidate{}, err
	}
	if err := lookup.Credential.Verify(w.Clock.Now()); err != nil {
		return PasswordCandidate{}, challengeRefused()
	}
	return h.forAccount(ctx, token.TenantID(), lookup.Account.ID, password,
		lookup.Credential.Purpose == domain.PendingReset)
}

// forInvitation resolves a redemption token. The account has no password yet, so the history and
// "not the one you have now" answer nothing - which is the honest answer and not an omission.
func (h CheckPassword) forInvitation(
	ctx context.Context, presented secret.Secret, password secret.Secret,
) (PasswordCandidate, error) {
	w := h.Writer
	token, err := domain.ParseRedemptionToken(presented.Reveal())
	if err != nil {
		return PasswordCandidate{}, redemptionRefused()
	}

	var found repository.RedemptionAccount
	err = w.UnitOfWork.WithinReadOnly(ctx, persistence.Scope{TenantID: token.TenantID()},
		func(ctx context.Context) error {
			read, err := w.Session.Accounts.FindByRedemptionToken(ctx, token)
			found = read
			return err
		})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return PasswordCandidate{}, redemptionRefused()
		}
		return PasswordCandidate{}, err
	}
	if !found.ExpiresAt.After(w.Clock.Now()) {
		return PasswordCandidate{}, redemptionRefused()
	}
	return h.forAccount(ctx, token.TenantID(), found.Account.ID, password, false)
}

// Descriptor is the catalogue entry.
func (h CheckPassword) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: CheckPasswordName,
		Summary: "Answers what only the server knows about a candidate password: the offline " +
			"lists, the breach corpus, the last few passwords of this account, and whether it " +
			"is the one in force. Identifiers, never sentences - the client already holds the " +
			"message code for each rule. It demands the same proof the setting demands: " +
			"without one the route would be an oracle telling anybody whether a word is on a " +
			"blocklist or in somebody's history. Its rate limit is its own, beside the auth " +
			"bucket, because it costs Argon2 comparisons.",
		SideEffects: "None. It consumes no credential - a question asked while somebody is still " +
			"typing must not end the sign-in it is asked during.",
		ReadOnly: true,
		Input: []usecase.Field{
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The candidate.",
			},
			{
				Name: "step_up_token", Kind: usecase.KindString,
				Description: "A step-up proof, for a signed-in caller.",
			},
			{
				Name: "pending_token", Kind: usecase.KindString,
				Description: "The pending credential of a sign-in the change step interrupted.",
			},
			{
				Name: "invitation_token", Kind: usecase.KindString,
				Description: "The token from an invitation mail.",
			},
			{
				Name: "reset_token", Kind: usecase.KindString,
				Description: "The token from a reset mail.",
			},
		},
		// No action and nothing required: the answer is about a password nobody has set, and an
		// entry per keystroke's worth of checking would be a trail of typing.
		Audit: usecase.AuditDeclaration{},
		Activity: usecase.ActivityDeclaration{
			Exempt: "A candidate password is not an entry.",
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h CheckPassword) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	violations, err := h.Execute(ctx, actor, CheckPasswordCommand{
		Password:        secret.New(in.String("password")),
		StepUpToken:     in.String("step_up_token"),
		PendingToken:    secret.New(in.String("pending_token")),
		InvitationToken: secret.New(in.String("invitation_token")),
		ResetToken:      secret.New(in.String("reset_token")),
	})
	if err != nil {
		return nil, err
	}
	return violationsOutput(violations), nil
}

// violationsOutput is the projection every channel gets: rule identifiers and their parameters.
func violationsOutput(violations []domain.PasswordViolation) usecase.Output {
	rows := make([]any, 0, len(violations))
	for _, violation := range violations {
		row := usecase.Output{"rule": string(violation.Rule)}
		if len(violation.Params) > 0 {
			params := usecase.Output{}
			for name, value := range violation.Params {
				params[name] = value
			}
			row["params"] = params
		}
		rows = append(rows, row)
	}
	return usecase.Output{"violations": rows}
}

// SignInVerdict is what the sign-in path needs to know about the rule, in one read.
//
// One value rather than four questions, because the sign-in path asks all of them at the same
// moment about the same account, and four reads of the same two rows would be four chances for them
// to disagree.
type SignInVerdict struct {
	// MustChangePassword is the whole of ADR-0068 §3's enforcement: a password that is right and no
	// longer meets the rule. Three reasons, one answer - too short for a tightened policy, older
	// than `max_age_days`, older than `rotation_from` - because what the person has to do is the
	// same in all three, and the screen says which only if it can say it usefully.
	MustChangePassword bool
	// Rules is what the new password will be judged against, answered with the challenge so that
	// the list under the field is there before the first keystroke.
	Rules PasswordRulesView
	// FactorRequired is who this workspace demands a second factor of.
	FactorRequired domain.MfaRequirement
	// Sessions is the two bounds a session opened now will answer to.
	Sessions domain.SessionPolicy
	// RotationFrom is the moment every older session is refused from.
	RotationFrom time.Time
}

// JudgeSignIn answers the verdict for an account whose password was just accepted.
//
// The plaintext is in hand here and nowhere else, which is the whole reason the enforcement is a
// step of the sign-in rather than a job: no job can check a password, and a job that walked accounts
// or tenants is what this project's own rules forbid.
func (w PasswordWriter) JudgeSignIn(
	ctx context.Context, tenantID shared.ID, account domain.Account, password secret.Secret,
) (SignInVerdict, error) {
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return SignInVerdict{}, err
	}
	policy := rules.Effective.Policy

	verdict := SignInVerdict{
		Rules:          passwordRulesView(rules, true),
		FactorRequired: policy.MfaRequiredFor,
		Sessions:       policy.Sessions,
		RotationFrom:   policy.RotationFrom,
	}

	if password.IsEmpty() {
		// No candidate to judge. Callers ask that way on purpose - the second step of a sign-in,
		// where the password was settled at the first, a reset, where it was just judged in full,
		// and a provider arrival, which never held one - and all want the verdict's *other*
		// answers: who a factor is demanded of, and what bounds the session. Treating an absent
		// password as one that fails the rule would put the change step in front of the very
		// password the rule had accepted.
		return verdict, nil
	}

	// The account's row only where there is a password to judge: a provider-only account has no
	// moment to compare, and the callers above want nothing from it.
	held, err := w.AccountFor(ctx, tenantID, account.ID)
	if err != nil {
		return SignInVerdict{}, err
	}

	// The rule as it stands, against the password as it is. Only the local half: the lists and the
	// history cost a round of Argon2 each and a sign-in is not the place to spend them - what they
	// would catch is a password that was already accepted under an earlier rule, and the change
	// step will judge the replacement in full.
	refused := domain.CheckPasswordAgainst(
		policy.Password, w.Text, password.Reveal(),
		domain.PasswordContext{
			Email:         account.Email,
			DisplayName:   account.DisplayName,
			WorkspaceName: rules.Workspace.DisplayName,
			WorkspaceHost: rules.Workspace.Slug,
		}) != nil

	setAt := held.PasswordSetAt
	now := w.Clock.Now()
	expired := policy.Password.MaxAgeDays > 0 && !setAt.IsZero() &&
		now.Sub(setAt) > time.Duration(policy.Password.MaxAgeDays)*24*time.Hour
	// A password with no recorded moment is not rotated out: an unknown date is the product's gap
	// rather than the person's, and locking somebody out over one would be our mistake charged to
	// them. `max_age_days` reads it the same way.
	rotated := !policy.RotationFrom.IsZero() && !setAt.IsZero() && setAt.Before(policy.RotationFrom)

	verdict.MustChangePassword = refused || expired || rotated
	return verdict, nil
}

const SetPasswordAndSignInName = "SetPasswordAndSignIn"

// SetPasswordAndSignInCommand completes the change step of a sign-in.
type SetPasswordAndSignInCommand struct {
	PendingToken secret.Secret
	Password     secret.Secret
	// TenantHeader may confirm the token's tenant, never overrule it.
	TenantHeader string
}

// SetPasswordAndSignIn is `POST /auth/sessions:set-password`: the fourth door (ADR-0068 §3, §5).
//
// The password was right and no longer meets the rule, so the sign-in continues by setting a new
// one - and confirming it *is* the sign-in, exactly as the enrolment step already works. The pending
// credential can do one thing and this is it.
type SetPasswordAndSignIn struct{ Writer PasswordWriter }

// Execute sets the password and opens the session the sign-in was going to open.
func (h SetPasswordAndSignIn) Execute(
	ctx context.Context, cmd SetPasswordAndSignInCommand,
) (SessionPair, error) {
	w := h.Writer

	token, err := domain.ParsePendingToken(cmd.PendingToken.Reveal())
	if err != nil {
		return SessionPair{}, challengeRefused()
	}
	if cmd.TenantHeader != "" && cmd.TenantHeader != token.TenantID().String() {
		return SessionPair{}, shared.ErrForbidden.WithDetail("access.tenant_mismatch")
	}

	tenantID := token.TenantID()
	scope := persistence.Scope{TenantID: tenantID}

	var lookup repository.PendingLookup
	err = w.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		read, err := w.Pending.FindByToken(ctx, token)
		lookup = read
		return err
	})
	if err != nil {
		if errors.Is(err, shared.ErrNotFound) {
			return SessionPair{}, challengeRefused()
		}
		return SessionPair{}, err
	}

	now := w.Clock.Now()
	if lookup.Credential.Purpose != domain.PendingPassword || lookup.Credential.Verify(now) != nil {
		// A credential of another purpose completes nothing here, CompleteSignIn's discipline: an
		// enrolment token that could set a password would be a password set by a factor nobody proved.
		return SessionPair{}, challengeRefused()
	}
	if err := lookup.Account.Verify(); err != nil {
		return SessionPair{}, err
	}
	if err := lookup.TenantStatus.Verify(); err != nil {
		return SessionPair{}, err
	}

	held, err := w.AccountFor(ctx, tenantID, lookup.Account.ID)
	if err != nil {
		return SessionPair{}, err
	}
	rules, err := w.ResolveFor(ctx, tenantID)
	if err != nil {
		return SessionPair{}, err
	}
	candidate := PasswordCandidate{
		TenantID: tenantID, Account: held, Password: cmd.Password, Rules: rules,
		// The minimum age does not apply: the rule is what asked for this change, and holding
		// somebody to a waiting period they did not choose to start would be a lockout.
		IsReset: true,
	}
	if err := w.Judge(ctx, candidate); err != nil {
		return SessionPair{}, err
	}

	var spent bool
	err = w.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		consumed, err := w.Pending.Consume(ctx, lookup.Credential.ID, now)
		spent = consumed
		return err
	})
	if err != nil {
		return SessionPair{}, err
	}
	if !spent {
		// Somebody completed this sign-in between our read and our write.
		return SessionPair{}, challengeRefused()
	}

	// Every session of the account: the rule refused the password that opened them, and a session
	// that outlived the password it was opened with is the hole `rotation_from` exists to close.
	if err := w.Write(ctx, candidate, "", true); err != nil {
		return SessionPair{}, err
	}

	return w.Session.openSessionWithHint(ctx, scope, tenantID, lookup.Account,
		lookup.Credential.UserAgent, lookup.Credential.IPClass, SignedInAction,
		rules.Effective.Policy.Sessions, domain.SignedInWithPassword)
}

// Descriptor is the catalogue entry.
func (h SetPasswordAndSignIn) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name: SetPasswordAndSignInName,
		Summary: "Completes a sign-in the change step interrupted: the pending credential the " +
			"password answered, and a new password under the rule that refused the old one. " +
			"Confirming it *is* the sign-in, exactly as the enrolment step works - the credential " +
			"can do nothing else, and it dies on use. Every session of the account ends, because " +
			"the rule refused the password that opened them.",
		SideEffects: "Spends the pending credential, stores the new hash and its moment, records " +
			"the previous hash in the history, ends every session, opens a new one, and writes " +
			"audit entries.",
		Input: []usecase.Field{
			{
				Name: "pending_token", Kind: usecase.KindString, Required: true,
				Description: "The challenge's credential. It dies on use.",
			},
			{
				Name: "password", Kind: usecase.KindString, Required: true,
				Description: "The new password, judged against this workspace's rule.",
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

func (h SetPasswordAndSignIn) invoke(
	ctx context.Context, _ appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	pair, err := h.Execute(ctx, SetPasswordAndSignInCommand{
		PendingToken: secret.New(in.String("pending_token")),
		Password:     secret.New(in.String("password")),
		TenantHeader: in.String("tenant_header"),
	})
	if err != nil {
		return nil, err
	}
	return pairOutput(pair), nil
}
