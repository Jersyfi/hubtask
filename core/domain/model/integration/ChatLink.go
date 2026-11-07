// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package integration

import (
	"crypto/sha256"
	"encoding/base32"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ChatLink is a person's chat account linked to a hub's chat bot (UC-SVC-09 check 6): the bot
// tells them of an approval waiting for their yes at Address and takes what ChatUser says as
// theirs.
type ChatLink struct {
	ID        shared.ID
	BotID     shared.ID
	AccountID shared.ID
	// AccountName is the person's label, for the trail of what they decide from chat.
	AccountName string
	ChatUser    string
	// Address is where a card to the person goes: a Slack user, a Teams personal conversation and
	// its service, a Mattermost `@name`, a Matrix room.
	Address string
}

// ChatLinkLifetime is how long a link's code may be said.
const ChatLinkLifetime = 15 * time.Minute

// ChatLinkCodeBytes is how much entropy a code carries: 40 bits, eight characters.
const ChatLinkCodeBytes = 5

// The commands a linked person says to a bot.
const (
	LinkCommand    = "!link"
	ApproveCommand = "!approve"
	DeclineCommand = "!decline"
)

// MaxChatReason bounds a decline's reason said in chat.
const MaxChatReason = 2000

var codeAlphabet = base32.StdEncoding.WithPadding(base32.NoPadding)

// ChatLinkCode is a code's text from its random bytes.
func ChatLinkCode(random []byte) string { return codeAlphabet.EncodeToString(random) }

// ChatLinkCodeHash is what is kept of a code; a code is read case- and space-insensitively.
func ChatLinkCodeHash(code string) []byte {
	sum := sha256.Sum256([]byte(strings.ToUpper(strings.Join(strings.Fields(code), ""))))
	return sum[:]
}

// ApprovalRef is the short reference a chat card names an approval by: the identifier's last
// eight hex digits, its random part - the first ones are its moment and repeat between approvals
// opened close together.
func ApprovalRef(id shared.ID) string {
	s := strings.ReplaceAll(id.String(), "-", "")
	if len(s) < 8 {
		return strings.ToUpper(s)
	}
	return strings.ToUpper(s[len(s)-8:])
}

// ChatCommand is a link, approve or decline command read from what a person said.
type ChatCommand struct {
	Verb string
	// Arg is a link's code or an approval's reference.
	Arg    string
	Reason string
}

var (
	linkCode    = regexp.MustCompile(`^[A-Z2-7]{8}$`)
	approvalRef = regexp.MustCompile(`^[0-9A-F]{8}$`)
)

// ParseChatCommand reads `!link <code>`, `!approve <ref>` or `!decline <ref> <reason>` from the
// first line of a message, the command's case ignored; anything else is no command. A slash
// command's text (bare) may leave the `!` out, so `/hubtask approve 7F3A2C10` reads the same - the
// code or reference must then have its shape, so a title "approve the budget" stays a title.
func ParseChatCommand(text string, bare bool) (ChatCommand, bool) {
	line := strings.TrimSpace(strings.SplitN(strings.TrimSpace(text), "\n", 2)[0])
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return ChatCommand{}, false
	}
	verb := strings.ToLower(fields[0])
	if bare && !strings.HasPrefix(verb, "!") {
		verb = "!" + verb
	}
	arg := strings.ToUpper(fields[1])
	switch {
	case verb == LinkCommand && linkCode.MatchString(arg):
	case (verb == ApproveCommand || verb == DeclineCommand) && approvalRef.MatchString(arg):
	default:
		return ChatCommand{}, false
	}
	cmd := ChatCommand{Verb: verb, Arg: arg}
	if verb == DeclineCommand {
		rest := strings.TrimSpace(line[strings.Index(line, fields[1])+len(fields[1]):])
		if utf8.RuneCountInString(rest) > MaxChatReason {
			rest = string([]rune(rest)[:MaxChatReason])
		}
		cmd.Reason = rest
	}
	return cmd, true
}
