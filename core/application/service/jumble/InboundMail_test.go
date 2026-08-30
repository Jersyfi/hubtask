// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"context"
	"errors"
	"strings"
	"testing"

	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/event"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	"github.com/Jersyfi/hubtask/core/port/clock"
)

// The mail intake's application half (G-11): a parsed message becomes an EMAIL entry, its files go
// through the media pipeline, and a message nobody could parse still lands.

// ingestedMedia is the media pipeline's slice, in memory.
type ingestedMedia struct {
	files  []mediaservice.IngestedFile
	tenant shared.ID
	answer []shared.ID
	err    error
}

func (m *ingestedMedia) Execute(
	_ context.Context, tenantID shared.ID, files []mediaservice.IngestedFile,
) ([]shared.ID, error) {
	m.tenant = tenantID
	m.files = append(m.files, files...)
	if m.err != nil {
		return nil, m.err
	}
	if m.answer != nil {
		return m.answer, nil
	}
	stored := make([]shared.ID, 0, len(files))
	for range files {
		stored = append(stored, attachmentID)
	}
	return stored, nil
}

const attachmentID = shared.ID("0192f000-0000-7000-8000-0000000000a1")

// mailDoor is the intake with a minted address, and the token that opens it.
func mailDoor(t *testing.T) (IntakeMail, *ingestedMedia, *entryStore, *published, integration.InboundToken) {
	t.Helper()

	store, entries, events, media := &intakeStore{}, newEntryStore(), &published{}, &ingestedMedia{}
	token, err := integration.NewInboundToken(tenant, []byte(strings.Repeat("k",
		integration.InboundTokenSecretBytes)))
	if err != nil {
		t.Fatalf("minting: %v", err)
	}
	if err := store.SetToken(context.Background(), token, now); err != nil {
		t.Fatal(err)
	}

	return IntakeMail{
		Intake: store, Entries: entries, Media: media, Events: events,
		UnitOfWork: unitOfWork{}, Clock: clock.Fixed(now), IDs: ids{next: entryID},
	}, media, entries, events, token
}

// The ordinary mail: it becomes one entry on the EMAIL channel, its files go through the media
// pipeline, and the arrival is announced - which is what fires a JUMBLE_ENTRY rule (G-10).
func TestAMailBecomesAnEntryWithItsAttachments(t *testing.T) {
	door, media, entries, events, token := mailDoor(t)

	entry, err := door.Execute(context.Background(), MailDelivery{
		Token: token, Sender: "orders@example.org", Subject: "Order #42",
		Body: "The customer asked for a call back.",
		Attachments: []mediaservice.IngestedFile{
			{FileName: "invoice.pdf", ClaimedType: "application/pdf", Content: []byte("%PDF")},
		},
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}

	if entry.Channel != domain.ChannelEmail {
		t.Errorf("the entry arrived on %s, want EMAIL", entry.Channel)
	}
	if entry.TenantID != tenant {
		t.Errorf("the entry belongs to %q", entry.TenantID)
	}
	if entry.Sender != "orders@example.org" || entry.RawSubject != "Order #42" {
		t.Errorf("the entry reads %+v", entry)
	}
	if len(entry.Attachments) != 1 || entry.Attachments[0] != attachmentID {
		t.Errorf("the entry carries %v", entry.Attachments)
	}
	if media.tenant != tenant || len(media.files) != 1 {
		t.Errorf("the pipeline was given %d files for tenant %q", len(media.files), media.tenant)
	}
	if len(entries.rows) != 1 {
		t.Errorf("%d entries stored", len(entries.rows))
	}
	if len(events.envelopes) != 1 || events.envelopes[0].Type != event.JumbleEntryReceived {
		t.Fatalf("announced %+v", events.envelopes)
	}
	// The system, as the actor. A From header authenticates nothing, so naming an account would
	// invent an author for something nobody in this workspace did.
	if events.envelopes[0].Actor.Kind != shared.ActorSystem {
		t.Errorf("the event names %+v", events.envelopes[0].Actor)
	}
}

// A message that defeated the parser still lands: the bytes become an attachment and the readable
// half of them becomes the body. A jumble exists to catch, and "unparseable" is a thing to catch.
func TestAnUnparseablePayloadStillLands(t *testing.T) {
	door, media, _, _, token := mailDoor(t)

	entry, err := door.Execute(context.Background(), MailDelivery{
		Token:       token,
		Raw:         []byte("\x00\x01not a mail at all\x02"),
		Unparseable: true,
	})
	if err != nil {
		t.Fatalf("an unparseable payload was lost: %v", err)
	}

	if len(media.files) != 1 || media.files[0].FileName != RawMailFileName {
		t.Fatalf("the payload was stored as %+v", media.files)
	}
	// No claim about what the bytes are: the sniff decides, which is the honest reading of bytes
	// nobody could parse.
	if media.files[0].ClaimedType != "" {
		t.Errorf("the payload claimed to be %q", media.files[0].ClaimedType)
	}
	if !strings.Contains(entry.RawBody, "not a mail at all") {
		t.Errorf("the body is %q, want the readable half of the payload", entry.RawBody)
	}
	if strings.ContainsAny(entry.RawBody, "\x00\x01\x02") {
		t.Errorf("the body carries control bytes: %q", entry.RawBody)
	}
}

// The token is checked before a byte is stored. An unknown address costs one lookup rather than a
// file in somebody's bucket - and every reason not to serve is the same not-found (T-21).
func TestAWrongTokenStoresNothingAtAll(t *testing.T) {
	door, media, entries, _, _ := mailDoor(t)

	stranger, err := integration.NewInboundToken(tenant, make([]byte, integration.InboundTokenSecretBytes))
	if err != nil {
		t.Fatal(err)
	}

	for name, delivery := range map[string]MailDelivery{
		"a token nobody minted": {Token: stranger, Body: "x"},
		"no token at all":       {Body: "x"},
	} {
		_, err := door.Execute(context.Background(), delivery)
		if !errors.Is(err, shared.ErrNotFound) {
			t.Errorf("%s answered %v, want not found", name, err)
		}
	}
	if len(media.files) != 0 {
		t.Errorf("a refused delivery stored %d files", len(media.files))
	}
	if len(entries.rows) != 0 {
		t.Errorf("a refused delivery stored %d entries", len(entries.rows))
	}
}

// A subject longer than the column takes is cut rather than refused. The bounds exist so that a
// crafted message costs a refusal instead of storage; losing a legitimate message to a header
// nobody reads twice would be the inbox failing at its one job.
func TestALongSubjectIsCutRatherThanLost(t *testing.T) {
	door, _, _, _, token := mailDoor(t)

	entry, err := door.Execute(context.Background(), MailDelivery{
		Token:   token,
		Subject: strings.Repeat("ä", domain.MaxSubjectLength+50),
		Body:    "body",
	})
	if err != nil {
		t.Fatalf("a long subject lost the mail: %v", err)
	}
	if got := len([]rune(entry.RawSubject)); got != domain.MaxSubjectLength {
		t.Errorf("the subject is %d runes, want the bound", got)
	}
}

// A sender that cannot be an address is dropped rather than cut: half an address is not a shorter
// address, it is a different one - and the whole point of the field is judging where something
// came from.
func TestAnImpossibleSenderIsDroppedRatherThanCut(t *testing.T) {
	door, _, _, _, token := mailDoor(t)

	entry, err := door.Execute(context.Background(), MailDelivery{
		Token:  token,
		Sender: strings.Repeat("a", domain.MaxSenderLength+1),
		Body:   "body",
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if entry.Sender != "" {
		t.Errorf("the sender is %q, want nothing at all", entry.Sender)
	}
}

// An installation wired without the media pipeline takes the mail without its files rather than
// refusing the mail: the message is what somebody sent, and the attachments came with it.
func TestAMailWithoutSomewhereToPutItsFilesStillLands(t *testing.T) {
	door, _, _, _, token := mailDoor(t)
	door.Media = nil

	entry, err := door.Execute(context.Background(), MailDelivery{
		Token: token, Subject: "Order #42", Body: "body",
		Attachments: []mediaservice.IngestedFile{{FileName: "a.pdf", Content: []byte("%PDF")}},
	})
	if err != nil {
		t.Fatalf("delivering: %v", err)
	}
	if len(entry.Attachments) != 0 {
		t.Errorf("the entry names %v, and nothing stored them", entry.Attachments)
	}
}
