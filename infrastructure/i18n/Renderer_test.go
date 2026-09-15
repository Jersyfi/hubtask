// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package i18n_test

import (
	"strings"
	"testing"

	port "github.com/Jersyfi/hubtask/core/port/i18n"
	"github.com/Jersyfi/hubtask/infrastructure/i18n"
)

func renderer(t *testing.T) port.Renderer {
	t.Helper()
	built, err := i18n.NewRenderer()
	if err != nil {
		t.Fatalf("building the renderer: %v", err)
	}
	return built
}

func TestTheRendererSubstitutesInTheAskedForLocale(t *testing.T) {
	rendered := renderer(t).Render("en", "notifications.category_unknown",
		map[string]string{"value": "GOSSIP"})

	if !strings.Contains(rendered, "GOSSIP") {
		t.Errorf("the parameter did not reach the sentence: %q", rendered)
	}
	if strings.Contains(rendered, "{value}") {
		t.Errorf("the placeholder is still standing: %q", rendered)
	}
}

// The fallback chain of i18n-l10n.md §2, walked down to the source language: a locale with no
// catalogue, and a locale whose catalogue lacks the key, both land on English - and none fails.
func TestEveryLocaleResolvesToSomething(t *testing.T) {
	const code = "notifications.no_address" // outside the families de.json translates
	english := renderer(t).Render("en", code, nil)

	for _, locale := range []string{"de-AT", "de", "pt-BR", "zh-Hans", "", "  ", "nonsense"} {
		if got := renderer(t).Render(locale, code, nil); got != english {
			t.Errorf("%q rendered %q, want the source language %q", locale, got, english)
		}
	}
}

// The second catalogue, as the server uses it: a German workspace is seeded with German
// structure and its people are written to in German (M-01), with de-AT falling to de.
func TestGermanRendersWhatTheServerRenders(t *testing.T) {
	for _, tc := range []struct{ locale, code, want string }{
		{"de", "seed.bucket.todo", "Zu erledigen"},
		{"de-AT", "seed.bucket.doing", "In Arbeit"},
		{"de-CH", "seed.bucket.done", "Erledigt"},
		{"de", "email.invitation.subject", "Du wurdest zu Hubtask eingeladen"},
		{"de", "errors.forbidden", "Dazu fehlt dir die Berechtigung."},
	} {
		if got := renderer(t).Render(tc.locale, tc.code, nil); got != tc.want {
			t.Errorf("%s %s rendered %q, want %q", tc.locale, tc.code, got, tc.want)
		}
	}

	rendered := renderer(t).Render("de", "email.reminder.subject", map[string]string{"title": "Angebot prüfen"})
	if rendered != "Erinnerung: „Angebot prüfen“" {
		t.Errorf("the parameter did not reach the German sentence: %q", rendered)
	}
}

// A missing translation must never become an undelivered email: the port answers rather than
// failing, and an unknown code renders as itself (i18n-l10n.md §3).
func TestAnUnknownCodeRendersAsItself(t *testing.T) {
	const code = "email.nothing.like.this"

	if got := renderer(t).Render("en", code, nil); got != code {
		t.Errorf("rendered %q, want the code itself", got)
	}
}

func TestTheRendererIsTheAdapterForThePort(t *testing.T) {
	var _ port.Renderer = i18n.Renderer{}
}
