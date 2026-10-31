// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package notification

import (
	"context"
	"errors"
	"net/url"
	"slices"
	"strings"
	"time"

	identityrepo "github.com/Jersyfi/hubtask/core/application/repository/identity"
	integrationrepo "github.com/Jersyfi/hubtask/core/application/repository/integration"
	repository "github.com/Jersyfi/hubtask/core/application/repository/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/automation"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/knowledge"
	domain "github.com/Jersyfi/hubtask/core/domain/model/notification"
	"github.com/Jersyfi/hubtask/core/domain/model/planning"
	"github.com/Jersyfi/hubtask/core/domain/model/servicedesk"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/whiteboard"
	"github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/core/port/mail"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

// The message codes an email is built from. Under `email.*`, as i18n-l10n.md §3 prescribes, and
// paired: a subject and a body per category, and a second body for the message that may not carry
// the title.
//
// Two body keys rather than one with a conditional placeholder, because a placeholder with no
// parameter is left standing by the catalogue - a body written for a title it does not get would
// print `{title}` at somebody. The withheld variant is a different sentence, and a translator
// needs to see it as one.
const (
	subjectAssignment  = "email.assignment.subject"
	subjectMembership  = "email.membership.subject"
	subjectComment     = "email.comment.subject"
	subjectInvitation  = "email.invitation.subject"
	subjectReminder    = "email.reminder.subject"
	subjectIntegration = "email.integration.subject"
	subjectRetention   = "email.retention.subject"
	// A rule that was switched off: INTEGRATION's category, its own sentence, because
	// "we stopped calling your server" is the wrong news about a rule.
	subjectRuleDisabled = "email.rule_disabled.subject"
	bodyRuleDisabled    = "email.rule_disabled.body"
	bodyAssignment      = "email.assignment.body"
	bodyMembership      = "email.membership.body"
	bodyComment         = "email.comment.body"
	bodyInvitation      = "email.invitation.body"
	bodyReminder        = "email.reminder.body"
	bodyIntegration     = "email.integration.body"
	bodyRetention       = "email.retention.body"
	// A private hub nobody was left in went to the trash: MEMBERSHIP's category, its own sentence,
	// and never a title - there is none it may carry (ADR-0073 §5).
	subjectPrivateHubTrashed = "email.private_hub_trashed.subject"
	bodyPrivateHubTrashed    = "email.private_hub_trashed.body"
	// The workspace's owner opened a private hub the recipient is a member of (ADR-0073 §4): who,
	// from when until when, and why. The hub's name only where the recipient lets titles travel.
	subjectEmergencyAccess = "email.emergency_access.subject"
	bodyEmergencyAccess    = "email.emergency_access.body"
	// A role somebody was given: MEMBERSHIP's category, its own sentence, naming the role and the
	// place. A workspace-wide role has no place to name and no title to withhold.
	subjectRoleGranted          = "email.role_granted.subject"
	bodyRoleGranted             = "email.role_granted.body"
	subjectRoleGrantedWorkspace = "email.role_granted.workspace.subject"
	bodyRoleGrantedWorkspace    = "email.role_granted.workspace.body"
	// A page whose verification ran out, told to its owner (UC-DOC-12 check 4): REMINDER's
	// category, its own sentence - nobody asked for this reminder, the page's verification did.
	subjectPageReview = "email.page_review.subject"
	bodyPageReview    = "email.page_review.body"
	// An agent that did what its policy lets it do with its owner told (UC-AGT-02, NOTIFY).
	subjectAgentNotice = "email.agent_notice.subject"
	bodyAgentNotice    = "email.agent_notice.body"
	// A person a whiteboard's comment named (UC-CNV-09): COMMENT's category, its own sentence - the
	// subject is a board, not an entry.
	subjectWhiteboardMention = "email.whiteboard_mention.subject"
	bodyWhiteboardMention    = "email.whiteboard_mention.body"
	// An approval waiting for the recipient's yes, a reminder of it, and a level overdue told to
	// the person who asked (UC-SVC-09 check 4): the subject is an approval, its title the entry's.
	subjectApprovalAsked    = "email.approval_asked.subject"
	bodyApprovalAsked       = "email.approval_asked.body"
	subjectApprovalReminder = "email.approval_reminder.subject"
	bodyApprovalReminder    = "email.approval_reminder.body"
	subjectApprovalOverdue  = "email.approval_overdue.subject"
	bodyApprovalOverdue     = "email.approval_overdue.body"
	// A plan beyond its time tolerance, told to whoever decided it (configuration.md §10): the subject
	// is a baseline, its title the plan's place.
	subjectPlanException = "email.plan_exception.subject"
	bodyPlanException    = "email.plan_exception.body"
	// rolePrefix and placePrefix name a role and a kind of place in the catalogue.
	rolePrefix  = "email.role."
	placePrefix = "email.role_granted.place."
	// withheldSuffix names the variant of a message that has no title to put in it - because the
	// recipient asked for none, or because the entry is gone.
	withheldSuffix = ".withheld"
)

// DeliverNotification sends one notification.
//
// It owns its transactions, because it cannot live inside one: rendering is cheap and sending is
// not, and a transaction held open across an SMTP conversation is what
// observability-reliability.md §8 forbids. Three parts, in the order the media reconciliation uses
// for the same reason:
//
//  1. Read what the message is about - the record, the recipient, the entry - in one transaction.
//  2. Render and send, outside any transaction.
//  3. Write the outcome back, in a transaction of its own.
//
// Safe to run twice, which is what lets it declare itself detached (queue.Detached), and the
// second run is not free of consequence: a process that died between the send and the write-back
// leaves a message sent and a record that still says pending, and the retry sends it again. That
// is the honest trade of at-least-once for email, and it is the right side of it - a duplicate
// notification is a nuisance, and a lost one is the thing somebody needed to know.
type DeliverNotification struct {
	Notifications repository.Notifications
	Preferences   repository.Preferences
	Accounts      identityrepo.Accounts
	Items         Entries
	// Rules and Subscriptions name the subjects that are not an entry: the rule the
	// message says was switched off, the subscription it says is no longer called. Optional -
	// nil leaves the title out, which is the withheld sentence rather than a wrong one.
	Rules         RuleReader
	Subscriptions SubscriptionReader
	// Accesses names the emergency access a member is told of. Optional like the two above: nil
	// leaves the hub, the opener's words and the times out, which the withheld sentence survives.
	Accesses AccessReader
	// Grants and Places name the role somebody was given: the grant, and the hub or collection it
	// is on (an entry is read through Items). Grants nil leaves the role out of the message - the
	// membership's withheld sentence; Places nil leaves the place's name out.
	Grants GrantReader
	Places PlaceReader
	// Pages names the page whose review fell due. Optional: nil leaves its title out.
	Pages PageReader
	// Agents names the agent its owner is told of. Optional: nil leaves its name out.
	Agents AgentReader
	// Boards names the whiteboard a comment named the recipient on. Optional: nil leaves its title
	// out.
	Boards BoardReader
	// Approvals names the approval a message is about, and through it the entry. Optional: nil
	// leaves the entry's title out.
	Approvals ApprovalReader
	// Baselines names the baseline a plan slipped from, and through it the plan; its place is read
	// through Places. Optional: nil leaves the plan's name and its link out.
	Baselines BaselineReader
	// Replies answers the address of the entry a message is about, so an answer by mail becomes a
	// comment there (ADR-0092 decision 8). Optional: nil, or an installation without a receiving
	// domain, sends no Reply-To.
	Replies    ReplyAddresses
	Mail       mail.Sender
	Renderer   i18n.Renderer
	UnitOfWork persistence.UnitOfWork
	Clock      clock.Clock
	// Workspaces answers the workspace's own default language, the second link of the chain a
	// recipient's locale is resolved down (i18n-l10n.md §2) - an invited person has not chosen
	// one yet, by definition, and the invitation is the first thing they read. Optional: nil
	// skips to the installation's default.
	Workspaces WorkspaceReader
	// FallbackLocale is the installation's default, the last link of the same chain.
	FallbackLocale string
	// Addresses answers the workspace's address, so an email carries a link somebody can click. Nil
	// composes none: a relative path in an email is a dead link.
	Addresses WorkspaceAddresses
	// Redemptions mints the credential an invitation mail carries: minted here, at
	// delivery, so the queue's payload stays identifiers-only and the plaintext exists exactly
	// once, in the message on its way out. Nil composes the plain link, which is what an
	// installation without the sign-in flow sent before the flow existed.
	Redemptions RedemptionMinter
	// Signals is the observability slice. Optional, like everywhere else in this package.
	Signals Signals
}

// RuleReader answers a rule by its identifier - the automation repository's Find.
type RuleReader interface {
	Find(ctx context.Context, id shared.ID) (automation.Rule, error)
}

// SubscriptionReader answers a subscription by its identifier - the integration repository's Find.
type SubscriptionReader interface {
	Find(ctx context.Context, id shared.ID) (integrationrepo.StoredSubscription, error)
}

// AccessReader answers an emergency access with its hub's name - the work repository's Find.
type AccessReader interface {
	Find(ctx context.Context, id shared.ID) (work.EmergencyAccess, string, error)
}

// GrantReader answers a membership by its identifier - the identity repository's Find.
type GrantReader interface {
	Find(ctx context.Context, id shared.ID) (identity.Grant, error)
}

// PlaceReader answers a hub or a collection by its identifier - the container repository's Find.
type PlaceReader interface {
	Find(ctx context.Context, id shared.ID) (work.Container, error)
}

// PageReader answers a page by its identifier - the knowledge repository's Find.
type PageReader interface {
	Find(ctx context.Context, id shared.ID) (knowledge.Page, error)
}

// AgentReader answers an agent's name by its identifier - the agents' store.
type AgentReader interface {
	AgentName(ctx context.Context, id shared.ID) (string, error)
}

// BoardReader answers a whiteboard by its identifier - the whiteboard repository's Find.
type BoardReader interface {
	Find(ctx context.Context, id shared.ID) (whiteboard.Board, error)
}

// ApprovalReader answers an approval by its identifier - the service desk repository's Find.
type ApprovalReader interface {
	Find(ctx context.Context, id shared.ID) (servicedesk.Approval, error)
}

// BaselineReader answers a baseline by its identifier - the planning repository's Find.
type BaselineReader interface {
	Find(ctx context.Context, id shared.ID) (planning.Baseline, error)
}

// RedemptionMinter is the identity service's seam. The empty secret means the account is no
// longer waiting, and the mail carries the plain link.
type RedemptionMinter interface {
	MintRedemptionToken(ctx context.Context, tenantID, accountID shared.ID) (secret.Secret, error)
}

// subject is what a message needs before it can be rendered: the record, who is receiving it, and
// what it is about.
type subject struct {
	record    domain.Notification
	recipient identityAccount
	// decision is what the recipient's preference says *now*, rebuilt at delivery rather than
	// carried from the consumer. A person who switched a category off while the message waited in
	// the queue has said something more recent than the record, and the later answer is the right
	// one - which also means a queue draining after an outage does not deliver what somebody has
	// since asked not to receive.
	decision  domain.Decision
	title     string
	actorName string
	// access is the emergency access the record names, zero for every other message.
	access work.EmergencyAccess
	// grant is the role the record names, zero for every other message.
	grant identity.Grant
	// approval is the approval the record names, zero for every other message.
	approval servicedesk.Approval
	// plan is the hub or collection a baseline's plan is, zero for every other message.
	plan work.Container
	// base is the workspace's address every link of the message names, read with the rest.
	base string
}

// identityAccount is the slice of an account this package renders from: the locale that decides
// the language, and the address that decides whether there is anywhere to send.
type identityAccount struct {
	locale  string
	address string
	// zone is where the recipient reads a time: the emergency access says from when until when.
	zone string
}

// Execute sends the notification the job names, and reports whether it worked.
//
// `lastAttempt` is the queue's judgement rather than this service's: the attempt budget lives on
// the job, and a record that counted its own would be a second budget disagreeing with the first.
func (d DeliverNotification) Execute(
	ctx context.Context, tenantID, notificationID shared.ID, lastAttempt bool,
) error {
	scope := persistence.Scope{TenantID: tenantID}

	loaded, err := d.load(ctx, scope, notificationID)
	switch {
	case errors.Is(err, shared.ErrNotFound):
		// The record went while its delivery waited - the retention sweep, or the entry it was
		// about being purged. Nothing to send and nothing to record: a job whose subject no longer
		// exists is done, not failed.
		return nil
	case err != nil:
		return err
	}
	if !loaded.record.Pending() {
		// Already decided. A duplicate job, or a retry of one that got as far as the write-back.
		return nil
	}

	if !loaded.decision.Send {
		// Switched off, or nowhere to send, since the record was written. Suppressed with the
		// reason rather than sent, and the job is done rather than failed.
		return d.write(ctx, scope, loaded.record.Suppress(loaded.decision.Reason))
	}

	started := d.Clock.Now()
	sendErr := d.send(ctx, loaded)
	if sendErr != nil {
		return d.recordFailure(ctx, scope, loaded, lastAttempt, sendErr)
	}

	sentAt := d.Clock.Now()
	if err := d.write(ctx, scope, loaded.record.Sent(sentAt)); err != nil {
		return err
	}
	if d.Signals != nil {
		d.Signals.NotificationSent(ctx,
			loaded.record.Category.String(), loaded.record.Channel.String(),
			sentAt.Sub(started).Seconds())
	}
	return nil
}

// load reads everything the message is built from, in one transaction.
//
// Read at delivery rather than carried in the job's payload, which is the reason the payload holds
// one identifier: an entry renamed while the notification waited is announced under its new name,
// and nothing personal sits in a queue table nothing cleans (rule 10).
func (d DeliverNotification) load(
	ctx context.Context, scope persistence.Scope, notificationID shared.ID,
) (subject, error) {
	var loaded subject

	err := d.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		record, err := d.Notifications.Find(ctx, notificationID)
		if err != nil {
			return err
		}
		loaded.record = record

		recipient, err := d.Accounts.Find(ctx, record.RecipientID)
		if err != nil {
			return err
		}
		locale, err := d.recipientLocale(ctx, recipient.Locale)
		if err != nil {
			return err
		}
		loaded.recipient = identityAccount{locale: locale, address: recipient.Email, zone: recipient.TimeZone}
		if loaded.base, err = addressOf(ctx, d.Addresses); err != nil {
			return err
		}

		preference, err := d.Preferences.Find(ctx, record.RecipientID, record.Category, record.Channel)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			// Saying nothing is the default, and the default is on. Written down once, in the
			// domain, and read here rather than copied.
			preference = domain.DefaultPreference(
				record.TenantID, record.RecipientID, record.Category, record.Channel)
		case err != nil:
			return err
		}
		loaded.decision = domain.Decide(record, domain.Recipient{
			AccountID: record.RecipientID, HasAddress: recipient.Email != "",
		}, preference)

		if !record.ActorID.IsZero() {
			actor, err := d.Accounts.Find(ctx, record.ActorID)
			switch {
			case errors.Is(err, shared.ErrNotFound):
				// The person who caused it has left. The message still makes sense without a
				// name, and the catalogue's `{actor}` placeholder is left standing rather than
				// blanked - which is exactly what the withheld variants are for.
			case err != nil:
				return err
			default:
				loaded.actorName = actor.DisplayName
			}
		}

		// What the message is about, by which subject the record names. Gone is not
		// an error for any of them: the message becomes the one without a title rather than no
		// message - somebody was told about something, and the link will tell them it is gone.
		switch {
		case !record.RuleID.IsZero() && d.Rules != nil:
			rule, err := d.Rules.Find(ctx, record.RuleID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.title = rule.Name
		case !record.SubscriptionID.IsZero() && d.Subscriptions != nil:
			stored, err := d.Subscriptions.Find(ctx, record.SubscriptionID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			// The host and nothing more: a target address may carry a token in its path, and a
			// mail is the wrong place to repeat one.
			if target, err := url.Parse(stored.Subscription.TargetURL); err == nil {
				loaded.title = target.Host
			}
		case !record.PrivateHubAccessID.IsZero() && d.Accesses != nil:
			access, hubName, err := d.Accesses.Find(ctx, record.PrivateHubAccessID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.access, loaded.title = access, hubName
		case !record.MembershipID.IsZero() && d.Grants != nil:
			// A grant that is gone was revoked, and its record went with it - this read lost
			// the race by a moment. Not found, so the job ends without telling anybody of a role
			// they no longer hold.
			grant, err := d.Grants.Find(ctx, record.MembershipID)
			if err != nil {
				return err
			}
			loaded.grant = grant
			title, err := d.placeName(ctx, grant.Scope)
			if err != nil {
				return err
			}
			loaded.title = title
		case !record.PageID.IsZero() && d.Pages != nil:
			page, err := d.Pages.Find(ctx, record.PageID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.title = page.Title
		case !record.WhiteboardID.IsZero() && d.Boards != nil:
			board, err := d.Boards.Find(ctx, record.WhiteboardID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			// A trashed board's title stays out: the link will say it is gone.
			if board.DeletedAt == nil {
				loaded.title = board.Title
			}
		case !record.ApprovalID.IsZero() && d.Approvals != nil:
			approval, err := d.Approvals.Find(ctx, record.ApprovalID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.approval = approval
			if !approval.ItemID.IsZero() {
				item, err := d.Items.Find(ctx, approval.ItemID)
				if err != nil && !errors.Is(err, shared.ErrNotFound) {
					return err
				}
				if item.DeletedAt == nil {
					loaded.title = item.Title
				}
			}
		case !record.BaselineID.IsZero() && d.Baselines != nil:
			baseline, err := d.Baselines.Find(ctx, record.BaselineID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			if !baseline.PlanID.IsZero() && d.Places != nil {
				place, err := d.Places.Find(ctx, baseline.PlanID)
				if err != nil && !errors.Is(err, shared.ErrNotFound) {
					return err
				}
				if place.DeletedAt == nil {
					loaded.plan, loaded.title = place, place.Name
				}
			}
		case !record.AgentID.IsZero() && d.Agents != nil:
			name, err := d.Agents.AgentName(ctx, record.AgentID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.title = name
		case !record.ItemID.IsZero():
			item, err := d.Items.Find(ctx, record.ItemID)
			if err != nil && !errors.Is(err, shared.ErrNotFound) {
				return err
			}
			loaded.title = item.Title
		}
		return nil
	})
	if err != nil {
		return subject{}, err
	}
	return loaded, nil
}

// placeName is the name of the hub, collection or entry a role is on; empty for the workspace and
// for a place that is gone, which the withheld sentence survives.
func (d DeliverNotification) placeName(ctx context.Context, scope identity.Scope) (string, error) {
	var name string
	var err error
	switch scope.Type {
	case identity.ScopeHub, identity.ScopeCollection:
		if d.Places == nil {
			return "", nil
		}
		var place work.Container
		place, err = d.Places.Find(ctx, scope.ID)
		name = place.Name
	case identity.ScopeItem:
		var item work.WorkItem
		item, err = d.Items.Find(ctx, scope.ID)
		name = item.Title
	}
	if err != nil && !errors.Is(err, shared.ErrNotFound) {
		return "", err
	}
	return name, nil
}

// send renders the message and hands it to the channel.
func (d DeliverNotification) send(ctx context.Context, loaded subject) error {
	message := d.compose(loaded)

	// The invitation's link is the redemption token: minted now, shown in this one
	// message, stored only as a hash. In the fragment rather than the query, so a proxy or a
	// server log between the mail client and the interface never sees it.
	if loaded.record.Category == domain.CategoryInvitation && d.Redemptions != nil {
		token, err := d.Redemptions.MintRedemptionToken(
			ctx, loaded.record.TenantID, loaded.record.RecipientID)
		if err != nil {
			return err
		}
		if !token.IsEmpty() {
			message.Link = identity.InvitationLink(loaded.base, token.Reveal())
		}
	}

	locale := loaded.recipient.locale
	params := message.Params()
	// The role and the place are words of the recipient's language, rendered before the sentence
	// that carries them.
	if message.RoleCode != "" {
		params["role"] = d.Renderer.Render(locale, message.RoleCode, nil)
	}
	if message.PlaceCode != "" {
		params["place"] = d.Renderer.Render(locale, message.PlaceCode, nil)
	}
	return d.Mail.Send(ctx, mail.Message{
		ReplyTo: d.replyTo(ctx, loaded.record),
		To:      loaded.recipient.address,
		// The recipient's locale, never the actor's (i18n-l10n.md §1). The port takes it as an
		// argument for exactly this reason: there is no call that can forget it.
		Subject: d.Renderer.Render(locale, message.SubjectCode, params),
		Body:    d.Renderer.Render(locale, message.BodyCode, params),
	})
}

// compose builds the message from the record and what may travel in it.
//
// The title reaches the message through domain.Message.WithTitle and the decision that permits it,
// which is the one door there is. Whether it is permitted is the preference's answer; whether
// there is one at all is the entry's - an entry that was purged while the message waited leaves
// the same message as a preference that withholds the title, which is the honest outcome in both
// cases: somebody is told that something concerns them, and where to look.
func (d DeliverNotification) compose(loaded subject) domain.Message {
	if loaded.record.PrivateHubTrashed() {
		return domain.Message{
			SubjectCode: subjectPrivateHubTrashed,
			BodyCode:    bodyPrivateHubTrashed,
			Link:        link(loaded.base, loaded.record),
		}
	}
	subjectCode, bodyCode := codesFor(loaded.record.Category)
	if !loaded.record.RuleID.IsZero() {
		subjectCode, bodyCode = subjectRuleDisabled, bodyRuleDisabled
	}
	if !loaded.record.PageID.IsZero() {
		subjectCode, bodyCode = subjectPageReview, bodyPageReview
	}
	if !loaded.record.AgentID.IsZero() {
		subjectCode, bodyCode = subjectAgentNotice, bodyAgentNotice
	}
	if !loaded.record.WhiteboardID.IsZero() {
		subjectCode, bodyCode = subjectWhiteboardMention, bodyWhiteboardMention
	}
	isOverdue := false
	if !loaded.record.ApprovalID.IsZero() {
		subjectCode, bodyCode = approvalCodes(loaded)
		isOverdue = subjectCode == subjectApprovalOverdue
	}

	message := domain.Message{
		SubjectCode: subjectCode,
		BodyCode:    bodyCode,
		ActorName:   loaded.actorName,
		Link:        link(loaded.base, loaded.record),
	}
	message = message.WithTitle(loaded.title, loaded.decision)
	if !loaded.record.BaselineID.IsZero() {
		message.SubjectCode, message.BodyCode = subjectPlanException, bodyPlanException
		message.Link = planLink(loaded.base, loaded.plan)
	}
	if isOverdue && loaded.base != "" {
		// The person who asked decides nothing; the entry is where they look.
		message.Link = strings.TrimSuffix(loaded.base, "/") + "/items/" + url.PathEscape(loaded.approval.ItemID.String())
	}
	if !loaded.grant.ID.IsZero() {
		message.SubjectCode, message.BodyCode = subjectRoleGranted, bodyRoleGranted
		message.RoleCode = rolePrefix + strings.ToLower(string(loaded.grant.Role))
		message.Link = placeLink(loaded.base, loaded.grant.Scope)
		if loaded.grant.Scope.Type == identity.ScopeTenant {
			// The whole workspace: no place to name, so no title to withhold either.
			message.SubjectCode, message.BodyCode = subjectRoleGrantedWorkspace, bodyRoleGrantedWorkspace
			return message
		}
		message.PlaceCode = placePrefix + strings.ToLower(string(loaded.grant.Scope.Type))
	}
	if loaded.record.Category == domain.CategoryEmergencyAccess && !loaded.access.ID.IsZero() {
		message.Reason = loaded.access.Reason
		message.When = inZone(loaded.access.OpenedAt, loaded.recipient.zone)
		message.Until = inZone(loaded.access.EndsAt, loaded.recipient.zone)
		// The hub itself, where a member reads every access to it.
		if loaded.base != "" {
			message.Link = loaded.base + "/hubs/" + url.PathEscape(loaded.access.HubID.String())
		}
	}
	if message.Title == "" {
		// A body written for a title it does not get would print `{title}` at somebody: the
		// catalogue leaves a placeholder with no parameter standing. The withheld variant is a
		// different sentence, and a translator needs to see it as one.
		message.SubjectCode += withheldSuffix
		message.BodyCode += withheldSuffix
	}
	return message
}

// planLink is the plan's screen, where the exception is drawn; the workspace when the plan is
// gone or unread.
func planLink(base string, plan work.Container) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" || plan.ID.IsZero() {
		return base
	}
	kind := "/collections/"
	if plan.Type == work.ContainerHub {
		kind = "/hubs/"
	}
	return base + kind + url.PathEscape(plan.ID.String()) + "/plan"
}

// approvalCodes is an approval message's pair: asked where a level began counting the recipient, a
// reminder while it still counts them, overdue for anybody else - the person who asked. An approval
// that is gone reads as a reminder, which the link then shows decided.
func approvalCodes(loaded subject) (subjectCode, bodyCode string) {
	switch {
	case loaded.record.Category == domain.CategoryAssignment:
		return subjectApprovalAsked, bodyApprovalAsked
	case loaded.approval.ID.IsZero() || slices.Contains(currentCounted(loaded.approval), loaded.record.RecipientID):
		return subjectApprovalReminder, bodyApprovalReminder
	}
	return subjectApprovalOverdue, bodyApprovalOverdue
}

func currentCounted(approval servicedesk.Approval) []shared.ID {
	if approval.Current < 0 || approval.Current >= len(approval.Levels) {
		return nil
	}
	return approval.Levels[approval.Current].Counted
}

// codesFor is the category's pair of catalogue keys. A switch rather than a map, so that a
// category added without its messages does not compile.
func codesFor(category domain.Category) (subjectCode, bodyCode string) {
	switch category {
	case domain.CategoryAssignment:
		return subjectAssignment, bodyAssignment
	case domain.CategoryMembership:
		return subjectMembership, bodyMembership
	case domain.CategoryComment:
		return subjectComment, bodyComment
	case domain.CategoryInvitation:
		return subjectInvitation, bodyInvitation
	case domain.CategoryReminder:
		// The pairs with no {actor} in them: nobody caused a reminder, a disabled integration or a
		// retention warning - the clock and this system did - and a message naming an actor that
		// is not there would print the placeholder at somebody.
		return subjectReminder, bodyReminder
	case domain.CategoryIntegration:
		// Every category has its own case: a record that fell through to the assignment pair
		// would turn "we stopped calling your server" into "somebody gave you something".
		return subjectIntegration, bodyIntegration
	case domain.CategoryRetention:
		return subjectRetention, bodyRetention
	case domain.CategoryEmergencyAccess:
		return subjectEmergencyAccess, bodyEmergencyAccess
	case domain.CategoryRequestReply:
		// The comment's sentence: somebody wrote on the request; only the link differs.
		return subjectComment, bodyComment
	}
	// Not reachable through New, which refuses an unknown category. Answered rather than panicked:
	// a message with the wrong words is better than a worker that dies on a row.
	return subjectAssignment, bodyAssignment
}

// link is where to look, absolute so that a mail client has something to click.
func link(base string, record domain.Notification) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		return ""
	}
	// The identifier is a parsed UUID by the time it is here, so there is nothing to escape - and
	// it is escaped anyway, because a link built by concatenation is the shape of an injection
	// even when this particular value cannot be one.
	switch {
	case record.Category == domain.CategoryRequestReply && !record.ItemID.IsZero():
		// A requester reads their request in the portal; the entry's own screen is the team's.
		return base + "/portal/requests/" + url.PathEscape(record.ItemID.String())
	case !record.RuleID.IsZero():
		return base + "/administration/rules/" + url.PathEscape(record.RuleID.String())
	case !record.SubscriptionID.IsZero():
		return base + "/administration/webhooks"
	case !record.ItemID.IsZero():
		return base + "/items/" + url.PathEscape(record.ItemID.String())
	case !record.PageID.IsZero():
		return base + "/pages/" + url.PathEscape(record.PageID.String())
	case !record.AgentID.IsZero():
		return base + "/administration/agents"
	case !record.WhiteboardID.IsZero():
		return base + "/whiteboards/" + url.PathEscape(record.WhiteboardID.String())
	case !record.ApprovalID.IsZero():
		// The list where the reader decides; it opens the app and signs nobody in (P-02).
		return base + "/approvals"
	case !record.MembershipID.IsZero():
		// The place the role is on; compose has the grant and names it.
		return base
	case !record.PrivateHubAccessID.IsZero():
		// The installation, where the access is gone: compose names the hub when it is there.
		return base
	case record.PrivateHubTrashed():
		return base + "/administration/private-hubs"
	}
	return base
}

// placeLink is where a role's place opens in the web app; the installation for the workspace.
func placeLink(base string, scope identity.Scope) string {
	base = strings.TrimSuffix(base, "/")
	if base == "" {
		return ""
	}
	switch scope.Type {
	case identity.ScopeHub:
		return base + "/hubs/" + url.PathEscape(scope.ID.String())
	case identity.ScopeCollection:
		return base + "/collections/" + url.PathEscape(scope.ID.String())
	case identity.ScopeItem:
		return base + "/items/" + url.PathEscape(scope.ID.String())
	}
	return base
}

// recordFailure writes what went wrong and decides whether the job comes back.
func (d DeliverNotification) recordFailure(
	ctx context.Context, scope persistence.Scope, loaded subject, lastAttempt bool, cause error,
) error {
	if err := d.write(ctx, scope, loaded.record.Failed(lastAttempt)); err != nil {
		return err
	}
	if d.Signals != nil {
		d.Signals.NotificationFailed(ctx,
			loaded.record.Category.String(), loaded.record.Channel.String(),
			shared.AsError(cause).DetailCode)
	}
	// Returned either way, so the queue retries or dead-letters. The record's state is this
	// package's answer to "what happened"; the job's fate is the queue's.
	return cause
}

// write records the outcome in a transaction of its own.
func (d DeliverNotification) write(
	ctx context.Context, scope persistence.Scope, record domain.Notification,
) error {
	err := d.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return d.Notifications.Save(ctx, record)
	})
	if errors.Is(err, shared.ErrNotFound) {
		// The retention sweep reached the record while the message was in flight. The message went
		// out, and there is nothing left to say so - which is a fact about a ninety-day-old
		// notification and not a failure of this job.
		return nil
	}
	return err
}

// WorkspaceReader is the slice of the workspace repository a delivery needs: the row, for its
// default language, and nothing that writes.
type WorkspaceReader interface {
	Find(ctx context.Context) (identity.Workspace, error)
}

// WorkspaceAddresses answers the address of the workspace a transaction is bound to: its main row's,
// read when the message is written, so a mail after a change of address links to the new one
// (identity service's WorkspaceAddress, ADR-0095 §2). Every link in this package is built on it.
type WorkspaceAddresses interface {
	Of(ctx context.Context) (string, error)
}

// addressOf is the workspace's address inside a transaction bound to it; empty without Addresses,
// and for a workspace row gone while its accounts are not, which composes no link - the message
// still goes, as it does without the workspace's name.
func addressOf(ctx context.Context, addresses WorkspaceAddresses) (string, error) {
	if addresses == nil {
		return "", nil
	}
	base, err := addresses.Of(ctx)
	if errors.Is(err, shared.ErrNotFound) {
		return "", nil
	}
	return strings.TrimSuffix(base, "/"), err
}

// recipientLocale is §2's chain for a recipient: the account's own language, otherwise the
// workspace's default, otherwise the installation's - read inside the delivery's own transaction,
// which is bound to the workspace the record belongs to. An empty answer at the end is the
// renderer's fallback to the source language.
func (d DeliverNotification) recipientLocale(ctx context.Context, own string) (string, error) {
	if own != "" {
		return own, nil
	}
	if d.Workspaces != nil {
		workspace, err := d.Workspaces.Find(ctx)
		switch {
		case errors.Is(err, shared.ErrNotFound):
			// A workspace row that is gone while its accounts are not is not this delivery's to
			// explain; the message still goes, in the installation's language.
		case err != nil:
			return "", err
		case workspace.DefaultLocale != "":
			return workspace.DefaultLocale, nil
		}
	}
	return d.FallbackLocale, nil
}

// emergencyTimeLayout is a moment as the emergency access's mail says it: to the minute, with the
// zone named, because an hour given without its zone is a wrong hour somewhere.
const emergencyTimeLayout = "2006-01-02 15:04"

// inZone writes the moment in the recipient's time zone, naming it; UTC where they chose none or
// one this installation does not know.
func inZone(at time.Time, zone string) string {
	location := time.UTC
	if zone != "" {
		if loaded, err := time.LoadLocation(zone); err == nil {
			location = loaded
		}
	}
	return at.In(location).Format(emergencyTimeLayout) + " (" + location.String() + ")"
}

// ReplyAddresses answers an entry's mail address, minting it where it has none. Empty where the
// installation receives no mail at addresses.
type ReplyAddresses interface {
	ReplyAddress(ctx context.Context, tenantID, itemID shared.ID) (string, error)
}

// replyTo is the entry's address for a message about one entry, and nothing otherwise. A failure
// to read it costs the Reply-To and never the message.
func (d DeliverNotification) replyTo(ctx context.Context, record domain.Notification) string {
	if d.Replies == nil || record.ItemID.IsZero() {
		return ""
	}
	switch record.Category {
	case domain.CategoryAssignment, domain.CategoryComment, domain.CategoryReminder:
	default:
		return ""
	}
	address, err := d.Replies.ReplyAddress(ctx, record.TenantID, record.ItemID)
	if err != nil {
		return ""
	}
	return address
}
