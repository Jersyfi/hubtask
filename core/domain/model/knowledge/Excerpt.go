// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"regexp"
	"strings"
)

var (
	quotePrefix = regexp.MustCompile(`^(?:>\s?)+`)
	// The marks of one line's prose, leftmost first, so an escaped mark is never read as a mark.
	excerptMark = regexp.MustCompile("\\\\([\\\\`*_~\\[\\]#|])|(!?)\\[([^\\]]*)\\]\\(([^)\\s]*)\\)|\\*\\*|~~|__|\\*([^*\\s][^*]*?)\\*|(^|[\\s(])_([^_\\s][^_]*?)_")
)

// PlainExcerpt is a search hit's passage as words (UC-DOC-01/3): the body's Markdown removed, its
// lines joined, the « » of the words found kept. The passage is a fragment of the body, so a mark
// whose partner was cut off is removed alone. A reference or an image shows nothing: the
// passage resolves no title the reader might not be allowed to see.
func PlainExcerpt(fragment string) string {
	var words []string
	for _, line := range strings.Split(strings.ReplaceAll(fragment, "\r\n", "\n"), "\n") {
		if text := plainLine(strings.TrimSpace(line)); text != "" {
			words = append(words, text)
		}
	}
	return strings.Join(strings.Fields(strings.Join(words, " ")), " ")
}

func plainLine(line string) string {
	switch {
	case line == "", line == "$$", tableDelimiter.MatchString(line), fenceLine.MatchString(line),
		closeLine.MatchString(line), ruleLine.MatchString(line), leafLine.MatchString(line):
		return ""
	case containerLine.MatchString(line):
		return plainInline(containerLine.FindStringSubmatch(line)[3])
	case strings.HasPrefix(line, "|") || strings.HasSuffix(line, "|"):
		var cells []string
		for _, cell := range tableCells(line) {
			if text := plainInline(cell); text != "" {
				cells = append(cells, text)
			}
		}
		return strings.Join(cells, " · ")
	}
	if m := headingLine.FindStringSubmatch(line); m != nil {
		line = m[2]
	}
	line = quotePrefix.ReplaceAllString(line, "")
	if m := bulletLine.FindStringSubmatch(line); m != nil {
		line = m[2]
	} else if m := orderedLine.FindStringSubmatch(line); m != nil {
		line = m[3]
	}
	if m := taskText.FindStringSubmatch(line); m != nil {
		line = m[2]
	}
	return plainInline(line)
}

// plainInline removes the inline marks; the text between backticks is code and stays as written.
func plainInline(text string) string {
	text = statusChip.ReplaceAllString(dayChip.ReplaceAllString(text, "$1"), "$1")
	parts := strings.Split(text, "`")
	for i := 0; i < len(parts); i += 2 {
		parts[i] = plainProse(parts[i])
	}
	return strings.TrimSpace(strings.Join(parts, ""))
}

func plainProse(text string) string {
	var out strings.Builder
	at := 0
	for _, m := range excerptMark.FindAllStringSubmatchIndex(text, -1) {
		out.WriteString(text[at:m[0]])
		group := func(n int) string {
			if m[2*n] < 0 {
				return ""
			}
			return text[m[2*n]:m[2*n+1]]
		}
		switch {
		case m[2] >= 0:
			out.WriteString(group(1))
		case m[6] >= 0:
			if group(2) == "" && !strings.HasPrefix(group(4), "hubtask:") {
				out.WriteString(group(3))
			}
		case m[10] >= 0:
			out.WriteString(group(5))
		case m[14] >= 0:
			out.WriteString(group(6) + group(7))
		}
		at = m[1]
	}
	out.WriteString(text[at:])
	return out.String()
}
