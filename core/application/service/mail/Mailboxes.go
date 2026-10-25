// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"errors"
	"time"

	repository "github.com/Jersyfi/hubtask/core/application/repository/mail"
	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/audit"
	"github.com/Jersyfi/hubtask/core/port/clock"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/persistence"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/correlation"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const (
	ListMailboxesName   = "ListMailboxes"
	ConnectMailboxName  = "ConnectMailbox"
	CheckMailboxName    = "CheckMailbox"
	RemoveMailboxName   = "RemoveMailbox"
	mailboxTarget       = "mailbox"
	mailboxConnected    = audit.Action("mail.mailbox_connected")
	mailboxChecked      = audit.Action("mail.mailbox_checked")
	mailboxRemoved      = audit.Action("mail.mailbox_removed")
	mailboxesReadAction = audit.Action("mail.mailboxes_read")

	// fetchedPerRead bounds one read: a mailbox with a backlog is worked through a minute at a time
	// rather than in one job that holds a worker for as long as the backlog is.
	fetchedPerRead = 25
	// openTimeout bounds a connection test the person waits for.
	openTimeout = 30 * time.Second
)

// Jobs is the queue's slice the mailboxes seed their reads into.
type Jobs interface {
	Enqueue(ctx context.Context, request queue.Request) (shared.ID, error)
}

// Sealer is the slice of the installation's encryptor a mailbox's secret needs.
type Sealer interface {
	Seal(ctx context.Context, plaintext secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error)
	Open(ctx context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error)
}

var _ Sealer = crypto.Encryptor(nil)

// Boxes is what the mailbox use cases share.
type Boxes struct {
	Writer    Writer
	Mailboxes repository.Mailboxes
	Reader    mailboxport.Reader
	Encryptor Sealer
	Jobs      Jobs
	IDs       clock.IDGenerator
}

// ListMailboxes answers the workspace's connected mailboxes and how their last read went.
type ListMailboxes struct{ Boxes Boxes }

// ConnectMailbox signs in to a mailbox once, and stores it when that worked.
type ConnectMailbox struct{ Boxes Boxes }

// CheckMailbox signs in to a connected mailbox again and reads it now.
type CheckMailbox struct{ Boxes Boxes }

// RemoveMailbox forgets a mailbox and its secret.
type RemoveMailbox struct{ Boxes Boxes }

// secretPurpose binds a mailbox's sealed secret to its row.
func secretPurpose(id shared.ID) crypto.Purpose {
	return crypto.Purpose("mailbox.secret:" + id.String())
}

func (h ListMailboxes) invoke(ctx context.Context, actor appshared.ActorContext, _ usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxesReadAction); err != nil {
		return nil, err
	}
	var boxes []domain.Mailbox
	err := b.Writer.UnitOfWork.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, err := b.Mailboxes.List(ctx)
		boxes = found
		return err
	})
	if err != nil {
		return nil, err
	}
	rows := make([]any, 0, len(boxes))
	for _, box := range boxes {
		rows = append(rows, mailboxOutput(box, b.Writer.Clock.Now()))
	}
	presets := make([]any, 0, len(domain.Presets))
	for _, preset := range domain.Presets {
		presets = append(presets, map[string]any{
			"key": preset.Key, "protocol": string(preset.Protocol), "plus_addresses": preset.PlusAddresses, "hint": preset.Hint,
		})
	}
	return usecase.Output{"mailboxes": rows, "presets": presets}, nil
}

func (h ConnectMailbox) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxConnected); err != nil {
		return nil, err
	}
	now := b.Writer.Clock.Now()
	box, err := domain.NewMailbox(domain.NewMailboxInput{
		ID: b.IDs.NewID(), TenantID: actor.TenantID, Preset: in.String("preset"),
		Protocol: domain.Protocol(in.String("protocol")), Address: in.String("address"),
		Host: in.String("host"), Port: in.Int("port"), SessionURL: in.String("session_url"),
		Username: in.String("username"), Folder: in.String("folder"), DoneFolder: in.String("done_folder"),
		Now: now,
	})
	if err != nil {
		return nil, err
	}
	password := secret.New(in.String("secret"))
	if password.IsEmpty() {
		return nil, shared.ErrValidation.WithDetail("mail.mailbox_secret_required").
			WithFields(shared.FieldError{Path: "/secret", Code: "mail.mailbox_secret_required"})
	}
	// Signed in once before anything is stored: a mailbox that cannot be read is said now, in the
	// form, and not an hour later in a log.
	if err := b.probe(ctx, box, password); err != nil {
		return nil, err
	}
	box = box.Checked(now, "")
	sealed, err := b.Encryptor.Seal(ctx, password, secretPurpose(box.ID))
	if err != nil {
		return nil, err
	}
	err = b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		count, err := b.Mailboxes.Count(ctx)
		if err != nil {
			return err
		}
		if count >= domain.MaxMailboxes {
			return shared.ErrConflict.WithDetail("mail.mailboxes_full")
		}
		if err := b.Mailboxes.Insert(ctx, box, sealed, actor.AccountID); err != nil {
			return err
		}
		return b.Mailboxes.RecordCheck(ctx, box)
	})
	if err != nil {
		return nil, err
	}
	if err := b.seed(ctx, box, now); err != nil {
		return nil, err
	}
	b.record(ctx, actor, mailboxConnected, box)
	return mailboxOutput(box, now), nil
}

func (h CheckMailbox) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxChecked); err != nil {
		return nil, err
	}
	id, err := in.ID("mailbox_id")
	if err != nil || id.IsZero() {
		return nil, shared.ErrValidation.WithDetail("mail.mailbox_required").
			WithFields(shared.FieldError{Path: "/mailbox_id", Code: "mail.mailbox_required"})
	}
	box, password, err := b.load(ctx, actor.PersistenceScope(), id)
	if err != nil {
		return nil, err
	}
	now := b.Writer.Clock.Now()
	box = box.Checked(now, codeOf(b.probe(ctx, box, password)))
	if err := b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		return b.Mailboxes.RecordCheck(ctx, box)
	}); err != nil {
		return nil, err
	}
	// Read now rather than in a minute: whoever pressed *Check* wants to see their test mail.
	if err := b.seed(ctx, box, now); err != nil {
		return nil, err
	}
	b.record(ctx, actor, mailboxChecked, box)
	return mailboxOutput(box, now), nil
}

func (h RemoveMailbox) invoke(ctx context.Context, actor appshared.ActorContext, in usecase.Input) (usecase.Output, error) {
	b := h.Boxes
	if err := b.Writer.authorizeIntake(ctx, actor, mailboxRemoved); err != nil {
		return nil, err
	}
	id, err := in.ID("mailbox_id")
	if err != nil || id.IsZero() {
		return nil, shared.ErrValidation.WithDetail("mail.mailbox_required").
			WithFields(shared.FieldError{Path: "/mailbox_id", Code: "mail.mailbox_required"})
	}
	var box domain.Mailbox
	err = b.Writer.UnitOfWork.Within(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		found, _, err := b.Mailboxes.Find(ctx, id)
		if err != nil {
			return err
		}
		box = found
		return b.Mailboxes.Delete(ctx, id)
	})
	if err != nil {
		return nil, err
	}
	b.record(ctx, actor, mailboxRemoved, box)
	return usecase.Output{"id": id.String(), "removed": true}, nil
}

// probe signs in and opens the folder, and lets go again.
func (b Boxes) probe(ctx context.Context, box domain.Mailbox, password secret.Secret) error {
	ctx, cancel := context.WithTimeout(ctx, openTimeout)
	defer cancel()
	session, err := b.Reader.Open(ctx, mailboxport.Connection{Mailbox: box, Secret: password})
	if err != nil {
		return err
	}
	return session.Close()
}

// load reads a mailbox and opens its secret.
func (b Boxes) load(ctx context.Context, scope persistence.Scope, id shared.ID) (domain.Mailbox, secret.Secret, error) {
	var (
		box    domain.Mailbox
		sealed crypto.Sealed
	)
	err := b.Writer.UnitOfWork.WithinReadOnly(ctx, scope, func(ctx context.Context) error {
		var err error
		box, sealed, err = b.Mailboxes.Find(ctx, id)
		return err
	})
	if err != nil {
		return domain.Mailbox{}, secret.Secret{}, err
	}
	password, err := b.Encryptor.Open(ctx, sealed, secretPurpose(box.ID))
	if err != nil {
		return domain.Mailbox{}, secret.Secret{}, err
	}
	return box, password, nil
}

// seed makes sure the mailbox's read is queued, now. The dedupe key keeps one per mailbox.
func (b Boxes) seed(ctx context.Context, box domain.Mailbox, now time.Time) error {
	if b.Jobs == nil {
		return nil
	}
	_, err := b.Jobs.Enqueue(ctx, queue.Request{
		Kind: queue.KindMailboxPoll, TenantID: box.TenantID, DedupeKey: box.ID.String(), RunAt: now,
		Payload: map[string]any{"mailbox_id": box.ID.String()},
	})
	return err
}

func (b Boxes) record(ctx context.Context, actor appshared.ActorContext, action audit.Action, box domain.Mailbox) {
	if b.Writer.Audit == nil {
		return
	}
	_ = b.Writer.Audit.Append(ctx, audit.Entry{
		TenantID: actor.TenantID, OccurredAt: b.Writer.Clock.Now(), Action: action,
		Outcome: audit.OutcomeSuccess, Severity: audit.SeverityWarning,
		ActorKind: actor.Kind, ActorID: actor.AccountID, ActorLabel: actor.AccountName,
		TargetType: mailboxTarget, TargetID: box.ID,
		Changes: audit.Changes(
			audit.Change{Field: "protocol", Classification: audit.Open, To: string(box.Protocol)},
			audit.Change{Field: "host", Classification: audit.Open, To: box.Host + box.SessionURL},
			audit.Change{Field: "failure", Classification: audit.Open, To: box.Failure},
		),
		Context: audit.Context{RequestID: correlation.RequestIDFrom(ctx)},
	})
}

// codeOf is a read's failure as a message code, never the server's words; empty for none.
func codeOf(err error) string {
	if err == nil {
		return ""
	}
	var coded *shared.Error
	if errors.As(err, &coded) && coded.DetailCode != "" {
		return coded.DetailCode
	}
	return "mail.mailbox_unreachable"
}

func mailboxOutput(box domain.Mailbox, now time.Time) usecase.Output {
	out := usecase.Output{
		"id": box.ID.String(), "protocol": string(box.Protocol), "address": box.Address,
		"username": box.Username, "folder": box.Folder, "done_folder": box.DoneFolder,
		"enabled": box.Enabled, "failing": box.Failing(now), "created_at": box.CreatedAt,
	}
	if box.Host != "" {
		out["host"], out["port"] = box.Host, box.Port
	}
	if box.SessionURL != "" {
		out["session_url"] = box.SessionURL
	}
	if box.Preset != "" {
		out["preset"] = box.Preset
	}
	if box.CheckedAt != nil {
		out["checked_at"] = *box.CheckedAt
	}
	if box.Failure != "" {
		out["failure"] = box.Failure
	}
	if preset, ok := domain.PresetNamed(box.Preset); ok && preset.PlusAddresses {
		out["plus_addresses"] = true
	}
	return out
}

// PollMailbox is the job that reads a mailbox into the intake (queue.KindMailboxPoll): sign in,
// take what arrived, hand each message to ReceiveMail, move what was filed to the done folder, and
// come back in a minute. A message the intake could not take stays where it is for the next read.
type PollMailbox struct {
	Boxes   Boxes
	Receive Receiver
	// Parse turns the raw bytes into what the intake reads; the presentation's parser, injected so
	// the application knows nothing about MIME.
	Parse func(raw []byte, mailbox string) (jumbleservice.ReceivedMail, error)
}

// Receiver is the intake's use case.
type Receiver interface {
	Execute(ctx context.Context, message jumbleservice.ReceivedMail) (jumbleservice.MailDeliveryResult, error)
}

var _ queue.Detached = PollMailbox{}

// OwnsItsTransactions: a read reaches a mail server between two writes, which must not hold a
// transaction open; a read that runs twice takes nothing twice (the receipts).
func (PollMailbox) OwnsItsTransactions() {}

// Run reads the mailbox once.
func (p PollMailbox) Run(ctx context.Context, job queue.Job) (queue.Result, error) {
	b := p.Boxes
	id, err := shared.ParseID(stringOf(job.Payload["mailbox_id"]))
	if err != nil {
		return queue.Result{Outcome: "malformed"}, nil
	}
	scope := persistence.Scope{TenantID: job.TenantID}
	box, password, err := b.load(ctx, scope, id)
	if errors.Is(err, shared.ErrNotFound) {
		// Removed: the read ends here, and nothing re-seeds it.
		return queue.Result{Outcome: "removed"}, nil
	}
	if err != nil {
		return queue.Result{}, err
	}
	if !box.Enabled {
		return queue.Result{Outcome: "disabled"}, nil
	}
	taken, failure := p.read(ctx, box, password)
	now := b.Writer.Clock.Now()
	box = box.Checked(now, failure)
	if err := b.Writer.UnitOfWork.Within(ctx, scope, func(ctx context.Context) error {
		return b.Mailboxes.RecordCheck(ctx, box)
	}); err != nil {
		return queue.Result{}, err
	}
	return queue.Result{
		Repeat: true, RepeatAfter: domain.PollInterval, Outcome: "read",
		Counts: map[string]int{"taken": taken},
	}, nil
}

// read takes what arrived and answers how many were filed, and the failure that stopped it.
func (p PollMailbox) read(ctx context.Context, box domain.Mailbox, password secret.Secret) (int, string) {
	session, err := p.Boxes.Reader.Open(ctx, mailboxport.Connection{Mailbox: box, Secret: password})
	if err != nil {
		return 0, codeOf(err)
	}
	defer func() { _ = session.Close() }()
	messages, err := session.Fetch(ctx, fetchedPerRead)
	if err != nil {
		return 0, codeOf(err)
	}
	taken := 0
	for _, message := range messages {
		parsed, err := p.Parse(message.Raw, box.Address)
		if err != nil {
			// A message the parser refuses outright (too many parts) stays in the mailbox for a
			// person to look at; the next read skips nothing else.
			continue
		}
		parsed.TenantID, parsed.Mailbox = box.TenantID, box.Address
		if _, err := p.Receive.Execute(ctx, parsed); err != nil {
			if errors.Is(err, shared.ErrNotFound) {
				// The intake is switched off: the mail waits in the mailbox until it is on again.
				return taken, "mail.intake_off"
			}
			continue
		}
		if err := session.Done(ctx, message.Ref); err != nil {
			return taken, codeOf(err)
		}
		taken++
	}
	return taken, ""
}

func stringOf(value any) string {
	text, _ := value.(string)
	return text
}
