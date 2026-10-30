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

// ChatBot is a Matrix account that sits in one room for one hub and speaks back (UC-INT-18): a
// link to one of the hub's entries posted in the room is answered with the entry's key, title and
// status, and a message `!task <title>` becomes an entry in the hub's collection. It reads the
// room through the client-server API's sync with the account's own token, as the person who set
// it up reads the hub.
type ChatBot struct {
	ID           shared.ID
	TenantID     shared.ID
	HubID        shared.ID
	CollectionID shared.ID
	// Homeserver is the https origin of the account's homeserver.
	Homeserver string
	Room       string
	// BotUser is the account's own Matrix identifier, as the homeserver named it: its own messages
	// are never read back.
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

// CheckChatBot bounds a bot as an administrator sets it up: an https homeserver origin, a room
// identifier (never an alias), a language in the catalogue's shape, and at least one thing to do.
func CheckChatBot(homeserver, room, locale string, unfurl, capture bool) (string, string, string, error) {
	u, err := url.Parse(strings.TrimSpace(homeserver))
	if err != nil || !strings.EqualFold(u.Scheme, "https") || u.Host == "" || u.User != nil {
		return "", "", "", fieldError("/homeserver", "chatbot.homeserver_invalid")
	}
	homeserver = "https://" + strings.ToLower(u.Host)
	target, err := ValidChatTarget(ChatTarget{Format: FormatMatrix, Room: room, Locale: locale})
	if err != nil {
		return "", "", "", err
	}
	if !unfurl && !capture {
		return "", "", "", fieldError("/unfurl", "chatbot.nothing_to_do")
	}
	return homeserver, target.Room, target.Locale, nil
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
