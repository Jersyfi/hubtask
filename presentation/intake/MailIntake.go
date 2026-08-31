// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"context"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	mediaservice "github.com/Jersyfi/hubtask/core/application/service/media"
	"github.com/Jersyfi/hubtask/core/domain/model/integration"
	domain "github.com/Jersyfi/hubtask/core/domain/model/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// MailIntake turns one posted message into a jumble entry (G-11).
//
// The seam between the two halves of the mail intake, and it is deliberately thin: the parser
// above it knows MIME and knows nothing about this system, the use case below it knows the inbox
// and knows nothing about MIME, and what happens here is that one becomes the other. That is what
// makes the transport replaceable - an IMAP poll, when AM-1 is decided, produces bytes and calls
// this same function (automation.md §5).
//
// It authenticates nothing itself. The token travels through to the use case, which resolves the
// tenant from it before a byte is stored, and every reason not to serve answers the same
// not-found - a malformed token included, because "that is not the right shape" tells somebody
// guessing that the shape is what to fix (T-21).
type MailIntake struct {
	Deliveries MailDeliverer
	// Limits are the parser's bounds. Zero fields take the documented defaults, so an installation
	// that configures nothing is bounded rather than unbounded.
	Limits MailLimits
}

// MailDeliverer is the use case this hands a parsed message to.
type MailDeliverer interface {
	Execute(ctx context.Context, delivery jumbleservice.MailDelivery) (domain.Entry, error)
}

// Deliver parses the payload and stores what it found.
//
// A payload the parser cannot read is not a refusal: the delivery goes on with the raw bytes, and
// the entry carries them. A payload that breaks one of the parser's *bounds* is a refusal, with
// the code that says which one - the difference between "look at the entry" and "raise the bound"
// is the difference between a message this installation could not read and one it declines to
// take.
func (i MailIntake) Deliver(
	ctx context.Context, presented string, raw []byte,
) (domain.Entry, error) {
	token, err := integration.ParseInboundToken(presented)
	if err != nil {
		return domain.Entry{}, shared.ErrNotFound.WithDetail("jumble.inbound_not_found")
	}

	delivery := jumbleservice.MailDelivery{Token: token, Raw: raw}
	parsed, err := ParseMail(raw, i.Limits)
	switch {
	// By the code rather than by errors.Is: every refusal in this parser is a validation error,
	// so matching on the kind would read "over its bound" as "could not be parsed" and store a
	// message this installation had just declined to take.
	case err != nil && shared.AsError(err).DetailCode == CodeMailUnparseable:
		delivery.Unparseable = true
	case err != nil:
		return domain.Entry{}, err
	default:
		delivery.Sender = parsed.Sender
		delivery.Subject = parsed.Subject
		delivery.Body = parsed.Text
		delivery.Attachments = filesOf(parsed.Attachments)
	}

	return i.Deliveries.Execute(ctx, delivery)
}

// filesOf maps what the parser found onto what the media pipeline takes. Two structs rather than
// one shared type, because the direction of the dependency is the point: the application layer
// does not learn that MIME exists.
func filesOf(attachments []MailAttachment) []mediaservice.IngestedFile {
	files := make([]mediaservice.IngestedFile, 0, len(attachments))
	for _, attachment := range attachments {
		files = append(files, mediaservice.IngestedFile{
			FileName: attachment.FileName,
			// The type the part claimed, held against the bytes by the media guard and never
			// trusted on its own - a mail is the input that lies about this most often (T-11).
			ClaimedType: attachment.ContentType,
			Content:     attachment.Content,
		})
	}
	return files
}
