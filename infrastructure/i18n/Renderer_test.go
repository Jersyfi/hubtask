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

// The fallback chain of i18n-l10n.md §2, walked down to the source language. One catalogue exists
// today, so every one of these lands on English - what is proved is that none of them fails.
func TestEveryLocaleResolvesToSomething(t *testing.T) {
	english := renderer(t).Render("en", "notifications.no_address", nil)

	for _, locale := range []string{"de-AT", "de", "pt-BR", "zh-Hans", "", "  ", "nonsense"} {
		if got := renderer(t).Render(locale, "notifications.no_address", nil); got != english {
			t.Errorf("%q rendered %q, want the source language %q", locale, got, english)
		}
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
