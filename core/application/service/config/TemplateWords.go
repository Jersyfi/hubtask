// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package config

import (
	"fmt"
	"hash/fnv"

	domain "github.com/Jersyfi/hubtask/core/domain/model/config"
	"github.com/Jersyfi/hubtask/core/port/i18n"
)

// templateWordFields are the attributes of a shipped template whose text a person reads: the names
// of types, fields, options, statuses, steps, roles, views and rules, a sample's or a page's title
// and a page's or a reply's body.
var templateWordFields = map[string]bool{"name": true, "title": true, "body": true}

// TemplateWordCode is the message code of one word of a shipped template: its English text's
// hash, so the same word in two templates is one translation and a reworded text is a new code
// rather than a stale translation (user-experience.md §4).
func TemplateWordCode(text string) string {
	sum := fnv.New32a()
	_, _ = sum.Write([]byte(text))
	return fmt.Sprintf("templates.text.%08x", sum.Sum32())
}

// localizedTemplate is a Hubtask template with its words in the locale, else as shipped: resolved
// when it is planned and stored as plain text from then on, so the workspace edits it as its own.
// A template of any other origin is the workspace's own words and stays as it is.
func localizedTemplate(t domain.Template, words i18n.Renderer, locale string) domain.Template {
	if words == nil || t.Origin != domain.OriginHubtask {
		return t
	}
	t.Document = localizedDocument(t.Document, words, locale)
	t.Hub = localizedDocument(t.Hub, words, locale)
	t.Collection = localizedDocument(t.Collection, words, locale)
	return t
}

func localizedDocument(document domain.Document, words i18n.Renderer, locale string) domain.Document {
	if document == nil {
		return nil
	}
	out := make(domain.Document, len(document))
	for name, section := range document {
		copied := make(domain.Section, len(section))
		for key, block := range section {
			if block == nil {
				copied[key] = nil
				continue
			}
			copied[key] = domain.Block(localizedValue(map[string]any(block), words, locale).(map[string]any))
		}
		out[name] = copied
	}
	return out
}

func localizedValue(value any, words i18n.Renderer, locale string) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, inner := range typed {
			if text, ok := inner.(string); ok && templateWordFields[key] && text != "" {
				out[key] = templateWord(text, words, locale)
				continue
			}
			out[key] = localizedValue(inner, words, locale)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, inner := range typed {
			out[i] = localizedValue(inner, words, locale)
		}
		return out
	default:
		return value
	}
}

// templateWord renders one word, the shipped text where no catalogue holds its code.
func templateWord(text string, words i18n.Renderer, locale string) string {
	code := TemplateWordCode(text)
	if said := words.Render(locale, code, nil); said != "" && said != code {
		return said
	}
	return text
}
