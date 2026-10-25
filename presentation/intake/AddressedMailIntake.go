// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package intake

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"strings"

	jumbleservice "github.com/Jersyfi/hubtask/core/application/service/jumble"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// AddressedMailIntake is the operator's inbound service's door (ADR-0094 decision 1): the service
// posts each raw message, authenticated by the installation's inbound key, and the parsed message
// goes into the intake of the workspace its address names.
//
// The parser is MailIntake's, unchanged; a connected mailbox hands its messages to the same use case
// through Parsed.
type AddressedMailIntake struct {
	Deliveries AddressedMailDeliverer
	Limits     MailLimits
	// Key is HUBTASK_MAIL_INBOUND_KEY. Empty leaves the door shut: an installation without a key
	// takes nothing at it.
	Key string
}

// AddressedMailDeliverer is the use case this hands a parsed message to.
type AddressedMailDeliverer interface {
	Execute(
		ctx context.Context, mail jumbleservice.ReceivedMail,
	) (jumbleservice.MailDeliveryResult, error)
}

// Authorized reports whether the presented Authorization header carries the key: as a bearer token,
// or as the password of basic credentials - the shape the inbound services that only take a URL can
// send. Compared in constant time.
func (i AddressedMailIntake) Authorized(header string) bool {
	if i.Key == "" {
		return false
	}
	scheme, value, found := strings.Cut(strings.TrimSpace(header), " ")
	if !found {
		return false
	}
	presented := ""
	switch strings.ToLower(scheme) {
	case "bearer":
		presented = strings.TrimSpace(value)
	case "basic":
		decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(value))
		if err != nil {
			return false
		}
		_, password, ok := strings.Cut(string(decoded), ":")
		if !ok {
			return false
		}
		presented = password
	default:
		return false
	}
	return subtle.ConstantTimeCompare([]byte(presented), []byte(i.Key)) == 1
}

// Deliver parses the message and routes it. The recipient the inbound service named goes first;
// the message's own headers after it.
func (i AddressedMailIntake) Deliver(
	ctx context.Context, recipient string, raw []byte,
) (jumbleservice.MailDeliveryResult, error) {
	var recipients []string
	if recipient = strings.TrimSpace(recipient); recipient != "" {
		for _, one := range strings.Split(recipient, ",") {
			recipients = append(recipients, strings.TrimSpace(one))
		}
	}
	mail, err := Parsed(raw, recipients, i.Limits)
	if err != nil {
		return jumbleservice.MailDeliveryResult{}, err
	}
	return i.Deliveries.Execute(ctx, mail)
}

// Parsed is a raw message as the intake takes it: the recipients the door knows first, then the
// message's own; an unparseable message is kept as bytes rather than refused.
func Parsed(raw []byte, recipients []string, limits MailLimits) (jumbleservice.ReceivedMail, error) {
	mail := jumbleservice.ReceivedMail{Raw: raw, Recipients: recipients}
	parsed, err := ParseMail(raw, limits)
	switch {
	case err != nil && shared.AsError(err).DetailCode == CodeMailUnparseable:
		mail.Unparseable = true
	case err != nil:
		return jumbleservice.ReceivedMail{}, err
	default:
		mail.Recipients = append(mail.Recipients, parsed.Recipients...)
		mail.Sender, mail.SenderName = parsed.Sender, parsed.SenderName
		mail.Subject, mail.Body = parsed.Subject, parsed.Text
		mail.Attachments = filesOf(parsed.Attachments)
		mail.Automatic = parsed.Signals.Automatic
		mail.Spam = parsed.Signals.Spam
		mail.AuthFailed = parsed.Signals.AuthFailed
		mail.MessageID = parsed.MessageID
	}
	return mail, nil
}
