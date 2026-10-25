// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package jumble

import (
	"regexp"
	"strings"
)

// replyHeads are the lines a mail program writes above the message it quotes. Matched on a whole
// line, case-insensitively: "On Mon, 3 Feb 2026, Anna wrote:", "Am 03.02.2026 um 10:00 schrieb
// Anna:", Outlook's "-----Original Message-----" and the forwarded block's own line.
var replyHeads = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^\s*on .{3,200} wrote:\s*$`),
	regexp.MustCompile(`(?i)^\s*am .{3,200} schrieb .{0,200}:\s*$`),
	regexp.MustCompile(`(?i)^\s*le .{3,200} a écrit\s*:\s*$`),
	regexp.MustCompile(`(?i)^\s*-{2,}\s*(original message|ursprüngliche nachricht|forwarded message|weitergeleitete nachricht)\s*-{2,}\s*$`),
	regexp.MustCompile(`(?i)^\s*_{10,}\s*$`),
}

// CutQuotedReply keeps what the sender wrote and drops what they answered to (ADR-0092
// decision 6): everything from the first reply head on, and a trailing block of `>` lines.
//
// Where cutting would leave nothing - a mail that is only a forward - the text stays whole: a
// forwarded mail is the content, and an empty description would lose it.
func CutQuotedReply(text string) string {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	end := len(lines)
	for index, line := range lines {
		if isReplyHead(line) {
			end = index
			break
		}
	}
	for end > 0 {
		line := strings.TrimSpace(lines[end-1])
		if line == "" || strings.HasPrefix(line, ">") {
			end--
			continue
		}
		break
	}
	kept := strings.TrimSpace(strings.Join(lines[:end], "\n"))
	if kept == "" {
		return strings.TrimSpace(text)
	}
	return kept
}

func isReplyHead(line string) bool {
	for _, head := range replyHeads {
		if head.MatchString(line) {
			return true
		}
	}
	return false
}
