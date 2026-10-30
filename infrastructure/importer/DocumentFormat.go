// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package importer

import (
	"encoding/json"
	"strconv"
	"strings"
)

// docNode is one node of the issue tracker's rich-text document: a tree of typed nodes, text
// leaves with marks. Only what Markdown can say is carried; a media node is the attachment the
// entry carries anyway.
type docNode struct {
	Type    string         `json:"type"`
	Text    string         `json:"text"`
	Attrs   map[string]any `json:"attrs"`
	Marks   []docMark      `json:"marks"`
	Content []docNode      `json:"content"`
}

type docMark struct {
	Type  string         `json:"type"`
	Attrs map[string]any `json:"attrs"`
}

// richText answers a field the source wrote either as a document or as plain text - the API's
// older version answers its own markup as a string, kept as written - as Markdown.
func richText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var plain string
	if json.Unmarshal(raw, &plain) == nil {
		return strings.TrimSpace(plain)
	}
	var doc docNode
	if json.Unmarshal(raw, &doc) != nil {
		return ""
	}
	var out strings.Builder
	writeBlocks(&out, doc.Content, "")
	return strings.TrimSpace(out.String())
}

func writeBlocks(out *strings.Builder, nodes []docNode, indent string) {
	for _, node := range nodes {
		switch node.Type {
		case "paragraph":
			out.WriteString(indent + inline(node.Content) + "\n\n")
		case "heading":
			level := 1
			if n, ok := node.Attrs["level"].(float64); ok && n >= 1 && n <= 6 {
				level = int(n)
			}
			out.WriteString(strings.Repeat("#", level) + " " + inline(node.Content) + "\n\n")
		case "bulletList", "orderedList", "taskList":
			for i, item := range node.Content {
				marker := "- "
				if node.Type == "orderedList" {
					marker = strconv.Itoa(i+1) + ". "
				}
				if item.Type == "taskItem" {
					marker = "- [ ] "
					if state, _ := item.Attrs["state"].(string); state == "DONE" {
						marker = "- [x] "
					}
					out.WriteString(indent + marker + inline(item.Content) + "\n")
					continue
				}
				var inner strings.Builder
				writeBlocks(&inner, item.Content, "")
				lines := strings.Split(strings.TrimSpace(inner.String()), "\n")
				for j, line := range lines {
					if j == 0 {
						out.WriteString(indent + marker + line + "\n")
					} else if strings.TrimSpace(line) != "" {
						out.WriteString(indent + "  " + line + "\n")
					}
				}
			}
			out.WriteString("\n")
		case "codeBlock":
			language, _ := node.Attrs["language"].(string)
			out.WriteString("```" + language + "\n" + plainText(node.Content) + "\n```\n\n")
		case "blockquote", "panel":
			var inner strings.Builder
			writeBlocks(&inner, node.Content, "")
			for _, line := range strings.Split(strings.TrimSpace(inner.String()), "\n") {
				out.WriteString(indent + "> " + line + "\n")
			}
			out.WriteString("\n")
		case "rule":
			out.WriteString("---\n\n")
		case "table":
			writeTable(out, node)
		case "mediaSingle", "mediaGroup", "media":
			// The file itself is the entry's attachment.
		default:
			if len(node.Content) > 0 {
				writeBlocks(out, node.Content, indent)
			} else if node.Text != "" {
				out.WriteString(indent + node.Text + "\n\n")
			}
		}
	}
}

func writeTable(out *strings.Builder, table docNode) {
	for i, row := range table.Content {
		cells := make([]string, 0, len(row.Content))
		for _, cell := range row.Content {
			var inner strings.Builder
			writeBlocks(&inner, cell.Content, "")
			cells = append(cells, strings.ReplaceAll(strings.TrimSpace(inner.String()), "\n", " "))
		}
		out.WriteString("| " + strings.Join(cells, " | ") + " |\n")
		if i == 0 {
			out.WriteString("|" + strings.Repeat(" --- |", len(cells)) + "\n")
		}
	}
	out.WriteString("\n")
}

func inline(nodes []docNode) string {
	var out strings.Builder
	for _, node := range nodes {
		switch node.Type {
		case "text":
			out.WriteString(marked(node.Text, node.Marks))
		case "hardBreak":
			out.WriteString("  \n")
		case "mention", "emoji", "status", "date":
			if label, ok := node.Attrs["text"].(string); ok {
				out.WriteString(label)
			} else if short, ok := node.Attrs["shortName"].(string); ok {
				out.WriteString(short)
			}
		case "inlineCard", "blockCard":
			if url, ok := node.Attrs["url"].(string); ok {
				out.WriteString("<" + url + ">")
			}
		default:
			out.WriteString(inline(node.Content))
		}
	}
	return out.String()
}

func marked(text string, marks []docMark) string {
	// Markdown's emphasis does not open on a space: the spaces go outside the markers.
	lead := text[:len(text)-len(strings.TrimLeft(text, " "))]
	trail := text[len(strings.TrimRight(text, " ")):]
	text = strings.TrimSpace(text)
	if text == "" {
		return lead + trail
	}
	for _, mark := range marks {
		switch mark.Type {
		case "strong":
			text = "**" + text + "**"
		case "em":
			text = "*" + text + "*"
		case "strike":
			text = "~~" + text + "~~"
		case "code":
			text = "`" + text + "`"
		case "link":
			if href, ok := mark.Attrs["href"].(string); ok {
				text = "[" + text + "](" + href + ")"
			}
		}
	}
	return lead + text + trail
}

func plainText(nodes []docNode) string {
	var out strings.Builder
	for _, node := range nodes {
		out.WriteString(node.Text)
		out.WriteString(plainText(node.Content))
	}
	return out.String()
}
