// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ChatBot sits in one room for one hub and speaks back: a link to one of the hub's entries posted
// in the room is answered with the entry's title and status, and a command becomes an entry in the
// hub's collection, as the person who set it up reads and writes the hub. On Matrix it is an
// account reading the room through the client-server API's sync (UC-INT-18); on Slack, Teams and
// Mattermost an app the platform posts to at the bot's intake (UC-INT-19).
type ChatBot struct {
	ID           shared.ID
	TenantID     shared.ID
	HubID        shared.ID
	CollectionID shared.ID
	// Platform is MATRIX, SLACK, TEAMS or MATTERMOST.
	Platform DeliveryFormat
	// Homeserver is the https origin of the Matrix homeserver or the Mattermost server; Slack's
	// and the Bot Framework's own on those platforms.
	Homeserver string
	Room       string
	// BotUser is the bot's own identifier as its platform named it - a Matrix user, a Slack bot
	// user, a Teams bot's application - so its own messages are never read back.
	BotUser string
	Locale  string
	Unfurl  bool
	Capture bool
	Enabled bool
	// RunAs is who set it up: the bot reads and writes as them, while they may.
	RunAs shared.ID
	// Since is the sync position the next run reads on from; empty until the first run.
	Since     string
	Failure   string
	CreatedAt time.Time
	UpdatedAt time.Time
	Version   int
}

// Bounds of a bot and of what one run does.
const (
	MaxChatBotsPerHub = 10
	MaxCapturedTitle  = 500
	MaxUnfurlsPerPost = 3
	MaxMessagesPerRun = 50
	// ChatBotInterval is how often a bot reads its room.
	ChatBotInterval = time.Minute
)

// CaptureCommand is what a message starts with to become an entry.
const CaptureCommand = "!task"

// The fixed addresses of the platforms whose API has one.
const (
	SlackOrigin = "https://slack.com"
	TeamsOrigin = "https://api.botframework.com"
)

var (
	slackChannel      = regexp.MustCompile(`^[CGD][A-Z0-9]{8,20}$`)
	mattermostChannel = regexp.MustCompile(`^[a-z0-9]{26}$`)
	teamsConversation = regexp.MustCompile(`^(19|a|8):[^\s;]{1,250}$`)
)

// ParseChatPlatform reads a bot's platform; empty is MATRIX, CloudEvents is no chat.
func ParseChatPlatform(raw string) (DeliveryFormat, error) {
	if strings.TrimSpace(raw) == "" {
		return FormatMatrix, nil
	}
	f, err := ParseDeliveryFormat(raw)
	if err != nil || !f.IsChat() {
		return "", fieldError("/platform", "chatbot.platform_unknown")
	}
	return f, nil
}

// PostsToIntake reports a platform that posts to the bot's intake rather than being read.
func (f DeliveryFormat) PostsToIntake() bool {
	return f == FormatSlack || f == FormatTeams || f == FormatMattermost
}

// CheckChatBot bounds a Matrix bot as an administrator sets it up.
func CheckChatBot(homeserver, room, locale string, unfurl, capture bool) (string, string, string, error) {
	return CheckChatBotOn(FormatMatrix, homeserver, room, locale, unfurl, capture)
}

// CheckChatBotOn bounds a bot as an administrator sets it up: an https origin where the platform
// has no fixed one, the room's identifier in the platform's shape (never an alias or a name, so a
// renamed channel stays the bot's), a language in the catalogue's shape, and at least one thing to
// do. It answers the origin, the room and the language as stored.
func CheckChatBotOn(platform DeliveryFormat, homeserver, room, locale string, unfurl, capture bool) (string, string, string, error) {
	switch platform {
	case FormatSlack:
		homeserver = SlackOrigin
	case FormatTeams:
		homeserver = TeamsOrigin
	case FormatMatrix, FormatMattermost:
		u, err := url.Parse(strings.TrimSpace(homeserver))
		if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.User != nil {
			return "", "", "", fieldError("/homeserver", "chatbot.homeserver_invalid")
		}
		homeserver = "https://" + strings.ToLower(u.Host)
	default:
		return "", "", "", fieldError("/platform", "chatbot.platform_unknown")
	}
	room = strings.TrimSpace(room)
	shape := map[DeliveryFormat]*regexp.Regexp{FormatSlack: slackChannel, FormatMattermost: mattermostChannel, FormatTeams: teamsConversation}[platform]
	if shape != nil && !shape.MatchString(room) {
		return "", "", "", fieldError("/room", "webhooks.room_malformed")
	}
	locale = strings.TrimSpace(locale)
	if platform == FormatMatrix {
		target, err := ValidChatTarget(ChatTarget{Format: FormatMatrix, Room: room, Locale: locale})
		if err != nil {
			return "", "", "", err
		}
		room = target.Room
	} else if locale != "" && !chatLocale.MatchString(locale) {
		return "", "", "", fieldError("/locale", "webhooks.locale_malformed")
	}
	if !unfurl && !capture {
		return "", "", "", fieldError("/unfurl", "chatbot.nothing_to_do")
	}
	return homeserver, room, locale, nil
}

// SameRoom reports whether a delivery's room is the bot's: a Teams thread's reply names its
// conversation with `;messageid=…` after it.
func (b ChatBot) SameRoom(room string) bool {
	if b.Platform == FormatTeams {
		room, _, _ = strings.Cut(room, ";")
	}
	return room != "" && room == b.Room
}

// Captured answers the title a message asks an entry for: the text after `!task`, its first line,
// at most MaxCapturedTitle characters; ok is false for any other message.
func Captured(body string) (string, bool) {
	body = strings.TrimSpace(body)
	if len(body) < len(CaptureCommand) || !strings.EqualFold(body[:len(CaptureCommand)], CaptureCommand) {
		return "", false
	}
	rest := body[len(CaptureCommand):]
	if rest != "" && rest[0] != ' ' && rest[0] != '\t' {
		return "", false
	}
	title := strings.TrimSpace(strings.SplitN(strings.TrimSpace(rest), "\n", 2)[0])
	if title == "" {
		return "", false
	}
	if utf8.RuneCountInString(title) > MaxCapturedTitle {
		title = string([]rune(title)[:MaxCapturedTitle])
	}
	return title, true
}

var entryLink = regexp.MustCompile(`^/items/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})`)

// LinkedEntries answers the entries a message links on this workspace's own address, each once,
// at most MaxUnfurlsPerPost: a link elsewhere is not this workspace's to answer.
func LinkedEntries(body, base string) []shared.ID {
	base = strings.TrimSuffix(strings.TrimSpace(base), "/")
	out := []shared.ID{}
	if base == "" {
		return out
	}
	seen := map[string]bool{}
	for rest := body; len(out) < MaxUnfurlsPerPost; {
		i := strings.Index(rest, base+"/")
		if i < 0 {
			break
		}
		rest = rest[i+len(base):]
		m := entryLink.FindStringSubmatch(rest)
		if m == nil {
			continue
		}
		if id := strings.ToLower(m[1]); !seen[id] {
			seen[id] = true
			out = append(out, shared.ID(id))
		}
	}
	return out
}

// Read records a run: where the next one reads on from, and a refusal's code or none.
func (b *ChatBot) Read(since, failure string, now time.Time) {
	if since != "" {
		b.Since = since
	}
	b.Failure, b.UpdatedAt = failure, now
}
