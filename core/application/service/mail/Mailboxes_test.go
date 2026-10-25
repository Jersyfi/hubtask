// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"context"
	"errors"
	"testing"
	"time"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/application/usecase"
	domain "github.com/Jersyfi/hubtask/core/domain/model/mail"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/crypto"
	mailboxport "github.com/Jersyfi/hubtask/core/port/mailbox"
	"github.com/Jersyfi/hubtask/core/port/queue"
	"github.com/Jersyfi/hubtask/core/shared/secret"
)

const boxID = shared.ID("01936f2a-7c1e-7000-8000-0000000000b1")

type mailboxStore struct {
	boxes  map[shared.ID]domain.Mailbox
	sealed map[shared.ID]crypto.Sealed
}

func (s *mailboxStore) List(context.Context) ([]domain.Mailbox, error) {
	var out []domain.Mailbox
	for _, box := range s.boxes {
		out = append(out, box)
	}
	return out, nil
}
func (s *mailboxStore) Find(_ context.Context, id shared.ID) (domain.Mailbox, crypto.Sealed, error) {
	box, ok := s.boxes[id]
	if !ok {
		return domain.Mailbox{}, crypto.Sealed{}, shared.ErrNotFound
	}
	return box, s.sealed[id], nil
}
func (s *mailboxStore) Insert(_ context.Context, box domain.Mailbox, sealed crypto.Sealed, _ shared.ID) error {
	s.boxes[box.ID], s.sealed[box.ID] = box, sealed
	return nil
}
func (s *mailboxStore) RecordCheck(_ context.Context, box domain.Mailbox) error {
	s.boxes[box.ID] = box
	return nil
}
func (s *mailboxStore) Delete(_ context.Context, id shared.ID) error {
	if _, ok := s.boxes[id]; !ok {
		return shared.ErrNotFound
	}
	delete(s.boxes, id)
	return nil
}
func (s *mailboxStore) Count(context.Context) (int, error) { return len(s.boxes), nil }

// plainSeal "seals" by keeping the value beside its purpose, so a test sees both were used.
type plainSeal struct{}

func (plainSeal) Seal(_ context.Context, value secret.Secret, purpose crypto.Purpose) (crypto.Sealed, error) {
	return crypto.Sealed{KeyID: string(purpose), Ciphertext: []byte(value.Reveal())}, nil
}
func (plainSeal) Open(_ context.Context, sealed crypto.Sealed, purpose crypto.Purpose) (secret.Secret, error) {
	if sealed.KeyID != string(purpose) {
		return secret.Secret{}, errors.New("purpose mismatch")
	}
	return secret.New(string(sealed.Ciphertext)), nil
}

type mailServer struct {
	password string
	messages []mailboxport.Message
	done     []string
	opened   int
	broken   bool
}

func (m *mailServer) Open(_ context.Context, connection mailboxport.Connection) (mailboxport.Session, error) {
	m.opened++
	if m.broken {
		return nil, shared.ErrUnavailable.WithDetail("mail.mailbox_unreachable")
	}
	if connection.Secret.Reveal() != m.password {
		return nil, shared.ErrValidation.WithDetail("mail.mailbox_refused")
	}
	return m, nil
}
func (m *mailServer) Fetch(_ context.Context, limit int) ([]mailboxport.Message, error) {
	return m.messages[:min(limit, len(m.messages))], nil
}
func (m *mailServer) Done(_ context.Context, ref string) error {
	m.done = append(m.done, ref)
	return nil
}
func (m *mailServer) Close() error { return nil }

type queued struct{ requests []queue.Request }

func (q *queued) Enqueue(_ context.Context, request queue.Request) (shared.ID, error) {
	q.requests = append(q.requests, request)
	return "j", nil
}

type fixedID struct{}

func (fixedID) NewID() shared.ID { return boxID }

type receiver struct {
	got  []jumbleservice.ReceivedMail
	fail map[string]error
}

func (r *receiver) Execute(_ context.Context, message jumbleservice.ReceivedMail) (jumbleservice.MailDeliveryResult, error) {
	if err := r.fail[message.Subject]; err != nil {
		return jumbleservice.MailDeliveryResult{}, err
	}
	r.got = append(r.got, message)
	return jumbleservice.MailDeliveryResult{Outcome: domain.OutcomeEntry}, nil
}

func boxes(auth *authorizer) (Boxes, *mailboxStore, *mailServer, *queued) {
	w, _, _ := writer(auth)
	store := &mailboxStore{boxes: map[shared.ID]domain.Mailbox{}, sealed: map[shared.ID]crypto.Sealed{}}
	server := &mailServer{password: "app-pw"}
	jobs := &queued{}
	return Boxes{Writer: w, Mailboxes: store, Reader: server, Encryptor: plainSeal{}, Jobs: jobs, IDs: fixedID{}}, store, server, jobs
}

func TestAMailboxIsStoredOnlyOnceItSignsIn(t *testing.T) {
	b, store, server, jobs := boxes(&authorizer{})
	connect := ConnectMailbox{Boxes: b}.Descriptor().Handler
	if _, err := connect.Invoke(context.Background(), person, usecase.Input{"preset": "gmail", "address": "familie@gmail.com", "secret": "wrong"}); shared.AsError(err).DetailCode != "mail.mailbox_refused" {
		t.Fatalf("a wrong password answered %v", err)
	}
	if len(store.boxes) != 0 {
		t.Fatal("a mailbox that could not sign in was stored")
	}
	if _, err := connect.Invoke(context.Background(), person, usecase.Input{"preset": "gmail", "address": "familie@gmail.com"}); shared.AsError(err).DetailCode != "mail.mailbox_secret_required" {
		t.Errorf("no secret answered %v", err)
	}
	out, err := connect.Invoke(context.Background(), person, usecase.Input{"preset": "gmail", "address": "familie@gmail.com", "secret": "app-pw"})
	if err != nil {
		t.Fatal(err)
	}
	if out["host"] != "imap.gmail.com" || out["plus_addresses"] != true || out["failure"] != nil {
		t.Errorf("answered %+v", out)
	}
	sealed := store.sealed[boxID]
	if sealed.KeyID != "mailbox.secret:"+boxID.String() || string(sealed.Ciphertext) != "app-pw" {
		t.Errorf("sealed %+v", sealed)
	}
	if len(jobs.requests) != 1 || jobs.requests[0].Kind != queue.KindMailboxPoll || jobs.requests[0].DedupeKey != boxID.String() {
		t.Errorf("queued %+v", jobs.requests)
	}
	if server.opened != 2 {
		t.Errorf("signed in %d times", server.opened)
	}
	listed, err := ListMailboxes{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{})
	if err != nil || len(listed["mailboxes"].([]any)) != 1 || len(listed["presets"].([]any)) != len(domain.Presets) {
		t.Errorf("listed %+v, %v", listed, err)
	}
}

func TestAWorkspaceConnectsAFewMailboxesAtMost(t *testing.T) {
	b, store, _, _ := boxes(&authorizer{})
	for index := range domain.MaxMailboxes {
		store.boxes[shared.ID(string(rune('a'+index)))] = domain.Mailbox{}
	}
	_, err := ConnectMailbox{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"preset": "gmx", "address": "x@gmx.de", "secret": "app-pw"})
	if !errors.Is(err, shared.ErrConflict) {
		t.Errorf("a sixth mailbox answered %v", err)
	}
}

func TestACheckRecordsHowTheReadWentAndARemovalForgets(t *testing.T) {
	b, store, server, jobs := boxes(&authorizer{})
	if _, err := (ConnectMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"preset": "ionos", "address": "info@familie.de", "secret": "app-pw"}); err != nil {
		t.Fatal(err)
	}
	server.broken = true
	out, err := CheckMailbox{Boxes: b}.Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{"mailbox_id": boxID.String()})
	if err != nil || out["failure"] != "mail.mailbox_unreachable" || store.boxes[boxID].FailingSince == nil {
		t.Errorf("checked %+v, %v", out, err)
	}
	if len(jobs.requests) != 2 {
		t.Errorf("a check queued %d reads", len(jobs.requests)-1)
	}
	if _, err := (CheckMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{}); !errors.Is(err, shared.ErrValidation) {
		t.Errorf("no mailbox answered %v", err)
	}
	if _, err := (RemoveMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{"mailbox_id": boxID.String()}); err != nil {
		t.Fatal(err)
	}
	if len(store.boxes) != 0 {
		t.Error("the mailbox stayed")
	}
	if _, err := (RemoveMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person, usecase.Input{"mailbox_id": boxID.String()}); !errors.Is(err, shared.ErrNotFound) {
		t.Errorf("removing it twice answered %v", err)
	}
	refused := &authorizer{refuse: shared.ErrForbidden}
	other, _, _, _ := boxes(refused)
	for _, descriptor := range []usecase.Descriptor{
		ListMailboxes{Boxes: other}.Descriptor(), ConnectMailbox{Boxes: other}.Descriptor(),
		CheckMailbox{Boxes: other}.Descriptor(), RemoveMailbox{Boxes: other}.Descriptor(),
	} {
		if _, err := descriptor.Handler.Invoke(context.Background(), person, usecase.Input{"mailbox_id": boxID.String()}); !errors.Is(err, shared.ErrForbidden) {
			t.Errorf("%s without the right answered %v", descriptor.Name, err)
		}
	}
}

func TestAReadFilesWhatArrivedAndComesBackInAMinute(t *testing.T) {
	b, store, server, _ := boxes(&authorizer{})
	if _, err := (ConnectMailbox{Boxes: b}).Descriptor().Handler.Invoke(context.Background(), person,
		usecase.Input{"preset": "gmail", "address": "familie@gmail.com", "secret": "app-pw"}); err != nil {
		t.Fatal(err)
	}
	server.messages = []mailboxport.Message{{Ref: "1", Raw: []byte("a")}, {Ref: "2", Raw: []byte("b")}, {Ref: "3", Raw: []byte("c")}}
	got := &receiver{fail: map[string]error{"b": shared.ErrUnavailable}}
	poll := PollMailbox{Boxes: b, Receive: got, Parse: func(raw []byte, _ string) (jumbleservice.ReceivedMail, error) {
		if string(raw) == "c" {
			return jumbleservice.ReceivedMail{}, shared.ErrValidation
		}
		return jumbleservice.ReceivedMail{Subject: string(raw)}, nil
	}}
	job := queue.Job{TenantID: tenant, Payload: map[string]any{"mailbox_id": boxID.String()}}
	result, err := poll.Run(context.Background(), job)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Repeat || result.RepeatAfter != time.Minute || result.Counts["taken"] != 1 {
		t.Errorf("result %+v", result)
	}
	if len(got.got) != 1 || got.got[0].TenantID != tenant || got.got[0].Mailbox != "familie@gmail.com" {
		t.Errorf("received %+v", got.got)
	}
	if len(server.done) != 1 || server.done[0] != "1" {
		t.Errorf("moved %v: only what was filed may leave the inbox", server.done)
	}

	// The intake switched off: the mail waits, the mailbox says why.
	got.fail["a"] = shared.ErrNotFound
	if _, err := poll.Run(context.Background(), job); err != nil {
		t.Fatal(err)
	}
	if store.boxes[boxID].Failure != "mail.intake_off" {
		t.Errorf("failure %q", store.boxes[boxID].Failure)
	}
	// A refused sign-in is recorded and read again later.
	server.password = "changed"
	if result, _ := poll.Run(context.Background(), job); !result.Repeat || store.boxes[boxID].Failure != "mail.mailbox_refused" {
		t.Errorf("a refused read %+v, %q", result, store.boxes[boxID].Failure)
	}
	// Disabled or removed: the read ends.
	disabled := store.boxes[boxID]
	disabled.Enabled = false
	store.boxes[boxID] = disabled
	if result, _ := poll.Run(context.Background(), job); result.Repeat {
		t.Error("a disabled mailbox is read again")
	}
	delete(store.boxes, boxID)
	if result, _ := poll.Run(context.Background(), job); result.Repeat || result.Outcome != "removed" {
		t.Errorf("a removed mailbox %+v", result)
	}
	if result, _ := poll.Run(context.Background(), queue.Job{TenantID: tenant, Payload: map[string]any{}}); result.Repeat {
		t.Error("a malformed job repeats")
	}
}
