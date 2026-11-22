// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// ReadInvitationDeliveriesName is the people list's read of how the last mail to each person went.
const ReadInvitationDeliveriesName = "ReadInvitationDeliveries"

// MailReplacedByLink is the reason an unsent invitation mail was set aside: its invitation was
// handed over by link instead (RenewInvitation, delivery LINK).
const MailReplacedByLink = "notifications.replaced_by_link"

// InvitationDelivery is the last mail to one account in the people list's words.
type InvitationDelivery struct {
	AccountID shared.ID
	// State is QUEUED, SENDING, SENT, FAILED or CANCELLED.
	State  string
	At     time.Time
	Code   string
	Params map[string]string
	// Category is what a recorded mail told the person; empty for an invitation not yet recorded.
	Category string
}

// ReadInvitationDeliveries answers, for the people list's rows, how the last mail to each account
// went (user-experience.md §8.3) - for a person still invited the invitation, otherwise the newer
// of it and any other mail: a person who asks "did my invitation go out?" or "why did she not hear
// of it?" is answered on the row, not by the operator's log. Asked of MANAGE_MEMBERS at the
// workspace, the right that sends an invitation.
type ReadInvitationDeliveries struct {
	Jobs repository.InvitationJobs
	// Accounts says who is still invited; MANAGE_MEMBERS covers reading them.
	Accounts   repository.Accounts
	Authorizer Authorizer
	UnitOfWork persistence.UnitOfWork
}

// Execute reads the deliveries on record; an account without one is left out.
func (h ReadInvitationDeliveries) Execute(
	ctx context.Context, actor appshared.ActorContext, accountIDs []shared.ID,
) ([]InvitationDelivery, error) {
	if len(accountIDs) == 0 {
		return nil, shared.ErrValidation.WithDetail("accounts.account_required").
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.account_required"})
	}
	if len(accountIDs) > MaxResolvedAccounts {
		return nil, shared.ErrValidation.
			WithDetail("accounts.too_many").WithParams(map[string]string{"maximum": "200"}).
			WithFields(shared.FieldError{Path: "/ids", Code: "accounts.too_many"})
	}
	if err := h.Authorizer.Authorize(ctx, actor, access.Request{
		Permission: service.PermissionManageMembers,
		Path:       []domain.Scope{domain.TenantScope()},
		Action:     AccountReadAction,
		TokenScope: membersRead,
		TargetType: accountTarget,
	}); err != nil {
		return nil, err
	}
	var jobs []repository.InvitationJob
	var mails, invitations []repository.LastMail
	var invited map[shared.ID]bool
	err := h.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if jobs, err = h.Jobs.Latest(ctx, accountIDs); err != nil {
			return err
		}
		if mails, err = h.Jobs.LatestMail(ctx, accountIDs); err != nil {
			return err
		}
		if invited, err = h.stillInvited(ctx, mails); err != nil || len(invited) == 0 {
			return err
		}
		ids := make([]shared.ID, 0, len(invited))
		for id := range invited {
			ids = append(ids, id)
		}
		invitations, err = h.Jobs.LatestInvitationMail(ctx, ids)
		return err
	})
	if err != nil {
		return nil, err
	}
	out := make([]InvitationDelivery, 0, len(jobs))
	for _, job := range jobs {
		delivery := InvitationDelivery{AccountID: job.AccountID, At: job.QueuedAt, Code: job.Code, Params: job.Params}
		switch job.State {
		case "PENDING":
			delivery.State = "QUEUED"
		case "RUNNING":
			delivery.State = "SENDING"
		case "SUCCEEDED":
			// The job recorded the notification and queued its delivery; what the mail did is the
			// record's. No record: nothing went out to say anything about.
			mail, said := mailDelivery(job)
			if !said {
				continue
			}
			delivery = mail
		case "CANCELLED":
			delivery.State, delivery.At = "CANCELLED", job.FinishedAt
		default:
			// FAILED and DEAD_LETTER: given up on, whichever way the queue ended it.
			delivery.State, delivery.At = "FAILED", job.FinishedAt
		}
		if delivery.At.IsZero() {
			delivery.At = job.QueuedAt
		}
		out = append(out, delivery)
	}
	return newerMail(out, jobs, mails, invited, invitations), nil
}

// stillInvited answers which of the accounts whose last mail is of another kind are still invited:
// only there can that mail stand where the invitation belongs. An account not found is not.
func (h ReadInvitationDeliveries) stillInvited(ctx context.Context, mails []repository.LastMail) (map[shared.ID]bool, error) {
	invited := map[shared.ID]bool{}
	for _, mail := range mails {
		if mail.Category == "INVITATION" {
			continue
		}
		account, err := h.Accounts.Find(ctx, mail.AccountID)
		if errors.Is(err, shared.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if account.Status == domain.AccountInvited {
			invited[mail.AccountID] = true
		}
	}
	return invited, nil
}

// newerMail puts the last mail of another kind in place of an account's invitation where it is
// newer, and adds it for an account without one. An invitation the queue still knows is said by
// its job, which also knows the mail before it was recorded; the record alone stands for one the
// queue has cleared. For a person still invited no other mail takes the invitation's place
// (UC-ID-14/8): the job, else the newest invitation record; with neither on record, the last
// other mail.
func newerMail(
	out []InvitationDelivery, jobs []repository.InvitationJob, mails []repository.LastMail,
	invited map[shared.ID]bool, invitations []repository.LastMail,
) []InvitationDelivery {
	at := make(map[shared.ID]int, len(out))
	for i, delivery := range out {
		at[delivery.AccountID] = i
	}
	queued := make(map[shared.ID]bool, len(jobs))
	for _, job := range jobs {
		queued[job.AccountID] = true
	}
	invitation := make(map[shared.ID]repository.LastMail, len(invitations))
	for _, mail := range invitations {
		invitation[mail.AccountID] = mail
	}
	for _, mail := range mails {
		if mail.Category == "INVITATION" && queued[mail.AccountID] {
			continue
		}
		if invited[mail.AccountID] {
			if _, ok := at[mail.AccountID]; ok {
				continue
			}
			if record, ok := invitation[mail.AccountID]; ok {
				mail = record
			}
		}
		delivery := lastMailDelivery(mail)
		if i, ok := at[mail.AccountID]; ok {
			if delivery.At.After(out[i].At) {
				out[i] = delivery
			}
			continue
		}
		at[mail.AccountID] = len(out)
		out = append(out, delivery)
	}
	return out
}

// lastMailDelivery is a recorded mail in the list's words. A record still PENDING whose delivery
// job has ended was given up on by the queue; one with a failure on its job waits to be tried
// again.
func lastMailDelivery(mail repository.LastMail) InvitationDelivery {
	delivery := InvitationDelivery{AccountID: mail.AccountID, Category: mail.Category, At: mail.RecordedAt}
	switch mail.State {
	case "SENT":
		delivery.State = "SENT"
		if !mail.SentAt.IsZero() {
			delivery.At = mail.SentAt
		}
	case "SUPPRESSED", "FAILED":
		delivery.State, delivery.Code = "FAILED", mail.Reason
		if mail.Reason == MailReplacedByLink {
			delivery.State, delivery.Code = "CANCELLED", ""
		}
		if delivery.Code == "" && delivery.State == "FAILED" {
			delivery.Code, delivery.Params = mail.DeliveryError, mail.DeliveryParams
		}
		if !mail.DeliveryEnded.IsZero() {
			delivery.At = mail.DeliveryEnded
		}
	default:
		delivery.State, delivery.Code, delivery.Params = "QUEUED", mail.DeliveryError, mail.DeliveryParams
		if !mail.DeliveryEnded.IsZero() {
			delivery.State, delivery.At = "FAILED", mail.DeliveryEnded
		}
	}
	return delivery
}

// mailDelivery is a recorded invitation in the list's words: waiting for its delivery, sent, or
// failed with the record's reason - a suppressed one (no address, no mail server) is a mail that
// did not go out, and so a failure the inviter can act on.
func mailDelivery(job repository.InvitationJob) (InvitationDelivery, bool) {
	delivery := InvitationDelivery{AccountID: job.AccountID, At: job.FinishedAt, Code: job.MailReason}
	switch job.MailState {
	case "PENDING":
		delivery.State = "QUEUED"
	case "SENT":
		delivery.State = "SENT"
		if !job.MailSentAt.IsZero() {
			delivery.At = job.MailSentAt
		}
	case "SUPPRESSED":
		delivery.State = "FAILED"
		if job.MailReason == MailReplacedByLink {
			// Set aside for a link handed over instead: withdrawn, not failed.
			delivery.State, delivery.Code = "CANCELLED", ""
		}
	case "FAILED":
		delivery.State = "FAILED"
	default:
		return InvitationDelivery{}, false
	}
	if delivery.At.IsZero() {
		delivery.At = job.QueuedAt
	}
	return delivery, true
}

// Descriptor registers the use case in all three channels.
func (h ReadInvitationDeliveries) Descriptor() usecase.Descriptor {
	return usecase.Descriptor{
		Name:       ReadInvitationDeliveriesName,
		Permission: service.PermissionManageMembers,
		Summary: "Reads, for up to 200 accounts, how the last mail to each went - the invitation, " +
			"or a newer mail of another kind with its category: queued, being sent, sent, failed " +
			"with its message code, or cancelled, and when. While an account is invited its " +
			"invitation mail is answered, whatever other mail went out since; with no invitation " +
			"mail on record, the last other mail. An account with no mail on record is left out.",
		SideEffects: "None. Reads only.",
		TokenScope:  membersRead,
		ReadOnly:    true,
		Input: []usecase.Field{
			{Name: "ids", Kind: usecase.KindIDList, Required: true, Description: "Whose invitations, 1 to 200 identifiers."},
		},
		Audit: usecase.AuditDeclaration{
			Action: AccountReadAction, TargetType: accountTarget,
			Severity: audit.SeverityInfo, Required: false,
		},
		Handler: usecase.HandlerFunc(h.invoke),
	}
}

func (h ReadInvitationDeliveries) invoke(
	ctx context.Context, actor appshared.ActorContext, in usecase.Input,
) (usecase.Output, error) {
	ids, err := in.IDList("ids")
	if err != nil {
		return nil, err
	}
	deliveries, err := h.Execute(ctx, actor, ids)
	if err != nil {
		return nil, err
	}
	rows := make([]usecase.Output, 0, len(deliveries))
	for _, d := range deliveries {
		row := usecase.Output{"account_id": d.AccountID.String(), "state": d.State, "at": d.At.UTC(), "code": nil}
		if d.Code != "" {
			row["code"] = d.Code
		}
		if len(d.Params) > 0 {
			row["params"] = d.Params
		}
		if d.Category != "" {
			row["category"] = d.Category
		}
		rows = append(rows, row)
	}
	return usecase.Output{"data": rows}, nil
}
