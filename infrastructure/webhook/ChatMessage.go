// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webhook

import (
	"encoding/json"
	"html"
	"net/http"
	"net/url"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/event"
	domain "github.com/Jersyfi/hubtask/core/domain/model/integration"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// ChatTexts renders a chat room's sentence from the message catalogue (i18n-l10n.md §3): the
// declared exception to rule 8 that mail has, for the same reason - the room is read by people.
// An unknown code renders as itself, which is how a type without a sentence of its own is told.
type ChatTexts interface {
	Render(locale, code string, params map[string]string) string
}

// chatOther is the sentence of an event type that has none of its own.
const chatOther = "chat.event_other"

// ChatMessage is what a room is told: a sentence, the entry's title and where to look - never a
// note, a comment or anything else a person wrote (UC-INT-12 check 4, data-protection.md §9).
type ChatMessage struct {
	Headline string
	Title    string
	Link     string
	// Open is the words of a card's link button, for a platform that draws one.
	Open string
}

// chatMessageOf renders the event for a room. The code is the event's entity and verb under
// `chat.` (`de.hubtask.work.item.completed.v1` → `chat.item_completed`).
func chatMessageOf(texts ChatTexts, locale, base string, envelope event.Envelope) ChatMessage {
	code := chatCode(envelope.Type)
	headline := texts.Render(locale, code, nil)
	if headline == code || headline == "" {
		headline = texts.Render(locale, chatOther, nil)
	}
	message := ChatMessage{Headline: headline, Open: texts.Render(locale, "chat.open", nil)}

	data := envelope.Payload
	itemID := ""
	if strings.Contains(string(envelope.Type), ".work.item.") {
		// An entry's own events carry it whole or by `item_id`; the title is only in the first.
		if title, ok := data["title"].(string); ok {
			message.Title = title
		}
		itemID, _ = data["id"].(string)
	}
	if itemID == "" {
		itemID, _ = data["item_id"].(string)
	}
	base = strings.TrimSuffix(base, "/")
	if base != "" {
		message.Link = base
		if parsed, err := shared.ParseID(itemID); err == nil && !parsed.IsZero() {
			message.Link = base + "/items/" + url.PathEscape(parsed.String())
		}
	}
	return message
}

// chatCode is the catalogue key of an event type's sentence.
func chatCode(eventType event.Type) string {
	name := strings.TrimPrefix(string(eventType), "de.hubtask.")
	if cut := strings.LastIndex(name, ".v"); cut > 0 {
		name = name[:cut]
	}
	parts := strings.Split(name, ".")
	if len(parts) < 2 {
		return chatOther
	}
	// The bounded context is left out: an entry is an entry wherever the event was raised.
	return "chat." + strings.Join(parts[1:], "_")
}

// chatRequest is the call a format makes. The sealed value opened is the room's credential: the
// Matrix account's token, or the incoming webhook's whole address.
func chatRequest(
	subscription domain.WebhookSubscription, credential string, envelope event.Envelope,
	message ChatMessage,
) (method, target string, header map[string][]string, body []byte, err error) {
	header = map[string][]string{"Content-Type": {"application/json; charset=utf-8"}}
	var document any
	switch subscription.Format {
	case domain.FormatMatrix:
		method = http.MethodPut
		// The event's identifier is the transaction: a retry of one delivery is one notice, which
		// is what the client-server API deduplicates on.
		target = strings.TrimSuffix(subscription.TargetURL, "/") + "/_matrix/client/v3/rooms/" +
			url.PathEscape(subscription.Room) + "/send/m.room.message/" +
			url.PathEscape("hubtask-"+envelope.ID.String())
		header["Authorization"] = []string{"Bearer " + credential}
		document = matrixNotice(message)
	case domain.FormatSlack, domain.FormatMattermost, domain.FormatTeams:
		if !domain.SameOrigin(credential, subscription.TargetURL) {
			// The sealed address no longer belongs to the target shown: send nowhere.
			return "", "", nil, nil, shared.ErrForbidden.WithDetail("webhooks.target_mismatch")
		}
		method, target = http.MethodPost, credential
		switch subscription.Format {
		case domain.FormatSlack:
			document = map[string]any{"text": slackText(message)}
		case domain.FormatMattermost:
			document = map[string]any{"text": markdownText(message)}
		default:
			document = teamsCard(message)
		}
	default:
		return "", "", nil, nil, shared.ErrInternal.WithDetail("webhooks.format_unknown")
	}
	body, err = json.Marshal(document)
	if err != nil {
		return "", "", nil, nil, shared.ErrInternal.WithDetail("webhooks.body_unrenderable").WithCause(err)
	}
	return method, target, header, body, nil
}

// plainText is the message as lines, for a client that draws no markup.
func plainText(message ChatMessage) string {
	lines := []string{message.Headline}
	if message.Title != "" {
		lines = append(lines, message.Title)
	}
	if message.Link != "" {
		lines = append(lines, message.Link)
	}
	return strings.Join(lines, "\n")
}

func matrixNotice(message ChatMessage) map[string]any {
	formatted := "<strong>" + html.EscapeString(message.Headline) + "</strong>"
	label := message.Title
	if label == "" {
		label = message.Link
	}
	if message.Link != "" {
		formatted += "<br><a href=\"" + html.EscapeString(message.Link) + "\">" + html.EscapeString(label) + "</a>"
	} else if message.Title != "" {
		formatted += "<br>" + html.EscapeString(message.Title)
	}
	return map[string]any{
		// A notice rather than a text: clients do not answer it and bots do not react to it.
		"msgtype": "m.notice", "body": plainText(message),
		"format": "org.matrix.custom.html", "formatted_body": formatted,
	}
}

// slackEscape is the three characters the platform's text reads as markup.
var slackEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func slackText(message ChatMessage) string {
	text := "*" + slackEscape.Replace(message.Headline) + "*"
	switch {
	case message.Link != "" && message.Title != "":
		text += "\n<" + message.Link + "|" + slackEscape.Replace(message.Title) + ">"
	case message.Link != "":
		text += "\n" + message.Link
	case message.Title != "":
		text += "\n" + slackEscape.Replace(message.Title)
	}
	return text
}

// markdownEscape keeps a title from becoming markup in a room that reads Markdown.
var markdownEscape = strings.NewReplacer(
	`\`, `\\`, "*", `\*`, "_", `\_`, "[", `\[`, "]", `\]`, "(", `\(`, ")", `\)`,
	"`", "\\`", "#", `\#`, "<", `\<`, ">", `\>`, "|", `\|`, "~", `\~`,
)

func markdownText(message ChatMessage) string {
	text := "**" + markdownEscape.Replace(message.Headline) + "**"
	switch {
	case message.Link != "" && message.Title != "":
		text += "\n[" + markdownEscape.Replace(message.Title) + "](" + message.Link + ")"
	case message.Link != "":
		text += "\n" + message.Link
	case message.Title != "":
		text += "\n" + markdownEscape.Replace(message.Title)
	}
	return text
}

// teamsCard is the message as an Adaptive Card, which is what a room's incoming workflow takes.
// Its text blocks are plain (no markup is read from a title), and the link is an action.
func teamsCard(message ChatMessage) map[string]any {
	blocks := []any{map[string]any{
		"type": "TextBlock", "text": message.Headline, "weight": "Bolder", "wrap": true,
	}}
	if message.Title != "" {
		blocks = append(blocks, map[string]any{"type": "TextBlock", "text": message.Title, "wrap": true})
	}
	card := map[string]any{
		"type": "AdaptiveCard", "version": "1.4",
		"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
		"body":    blocks,
	}
	if message.Link != "" {
		card["actions"] = []any{map[string]any{"type": "Action.OpenUrl", "title": message.Open, "url": message.Link}}
	}
	return map[string]any{
		"type": "message",
		"attachments": []any{map[string]any{
			"contentType": "application/vnd.microsoft.card.adaptive", "content": card,
		}},
	}
}
