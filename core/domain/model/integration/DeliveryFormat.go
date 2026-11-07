// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"net/url"
	"regexp"
	"strings"
)

// DeliveryFormat is what a subscription's target receives (UC-INT-12): the CloudEvent, or a
// sentence for a chat room. A chat format is the same subscription - its types, condition, scope,
// retries and switching off - with another body, so a team's room is told under the rules a
// workspace's server is.
type DeliveryFormat string

const (
	// FormatCloudEvents is the signed CloudEvent of UC-INT-01, and every subscription before
	// chat rooms existed.
	FormatCloudEvents DeliveryFormat = "CLOUDEVENTS"
	// FormatMatrix sends a notice into a Matrix room through the client-server API, as the
	// account whose access token the subscription holds sealed.
	FormatMatrix DeliveryFormat = "MATRIX"
	// FormatSlack, FormatMattermost and FormatTeams post to the room's incoming webhook; its
	// address is the target.
	FormatSlack      DeliveryFormat = "SLACK"
	FormatMattermost DeliveryFormat = "MATTERMOST"
	FormatTeams      DeliveryFormat = "TEAMS"
)

// maxChatRoom bounds a Matrix room identifier as the column and the contract do.
const maxChatRoom = 255

// matrixRoomID is a room identifier, `!opaque:server`. An alias (`#name:server`) would need a
// lookup on every delivery, and a room renamed would quietly start sending elsewhere.
var matrixRoomID = regexp.MustCompile(`^![^:\s]+:[^\s]+$`)

// chatLocale is a language tag's shape, `de` or `pt-BR`. Whether the installation carries the
// language is the renderer's question; an unknown one falls back to the source language.
var chatLocale = regexp.MustCompile(`^[a-z]{2,3}(-[A-Z]{2})?$`)

// ParseDeliveryFormat reads a format; empty is the CloudEvent.
func ParseDeliveryFormat(raw string) (DeliveryFormat, error) {
	switch format := DeliveryFormat(strings.TrimSpace(raw)); format {
	case "":
		return FormatCloudEvents, nil
	case FormatCloudEvents, FormatMatrix, FormatSlack, FormatMattermost, FormatTeams:
		return format, nil
	}
	return "", fieldError("/format", "webhooks.format_unknown")
}

// IsChat reports a format that carries a sentence rather than the CloudEvent. A chat delivery is
// not signed: no chat platform verifies a signature, and the room's own credential is what
// admits the call.
func (f DeliveryFormat) IsChat() bool { return f != "" && f != FormatCloudEvents }

// ChatTarget is where a chat format delivers and in which language, checked together because
// what is required depends on the format.
type ChatTarget struct {
	Format DeliveryFormat
	// Room is a Matrix room identifier, and empty for every other format.
	Room string
	// Locale is the language of the sentence; empty is the installation's source language.
	Locale string
}

// ValidChatTarget refuses a room where the format takes none, a Matrix format without one, and a
// language for the CloudEvent, which has no sentence to write in it.
func ValidChatTarget(in ChatTarget) (ChatTarget, error) {
	format, err := ParseDeliveryFormat(string(in.Format))
	if err != nil {
		return ChatTarget{}, err
	}
	room := strings.TrimSpace(in.Room)
	locale := strings.TrimSpace(in.Locale)
	switch {
	case format == FormatMatrix && room == "":
		return ChatTarget{}, fieldError("/room", "webhooks.room_required")
	case format == FormatMatrix && (len(room) > maxChatRoom || !matrixRoomID.MatchString(room)):
		return ChatTarget{}, fieldError("/room", "webhooks.room_malformed")
	case format != FormatMatrix && room != "":
		return ChatTarget{}, fieldError("/room", "webhooks.room_not_for_format")
	case !format.IsChat() && locale != "":
		return ChatTarget{}, fieldError("/locale", "webhooks.locale_not_for_format")
	case locale != "" && !chatLocale.MatchString(locale):
		return ChatTarget{}, fieldError("/locale", "webhooks.locale_malformed")
	}
	return ChatTarget{Format: format, Room: room, Locale: locale}, nil
}

// TakesIncomingAddress reports a format whose target is a room's incoming webhook. Its address is
// the room's credential - whoever holds it may post there - so it is sealed like a secret, and
// what is stored, listed and audited as the target is its origin alone.
func (f DeliveryFormat) TakesIncomingAddress() bool {
	return f == FormatSlack || f == FormatMattermost || f == FormatTeams
}

// IncomingAddress checks a room's incoming webhook address and answers its origin, the part that
// may be shown.
func IncomingAddress(raw string) (address, origin string, err error) {
	address, err = TargetURL(raw)
	if err != nil {
		return "", "", err
	}
	parsed, _ := url.Parse(address)
	if parsed.Path == "" || parsed.Path == "/" {
		// An origin alone is no room: the address the platform hands out names one in its path.
		return "", "", fieldError("/target_url", "webhooks.incoming_address_incomplete")
	}
	return address, parsed.Scheme + "://" + parsed.Host, nil
}

// SameOrigin reports whether an opened incoming address still belongs to the stored target, so a
// sealed address can never send anywhere the listing does not show.
func SameOrigin(address, origin string) bool {
	parsed, err := url.Parse(address)
	return err == nil && parsed.Scheme+"://"+parsed.Host == origin
}
