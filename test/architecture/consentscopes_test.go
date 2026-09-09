// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package architecture

import (
	"strings"
	"testing"

	usecases "github.com/Jersyfi/hubtask/core/application/catalogue"
)

// Every scope a person can be asked to allow has a sentence.
//
// The consent screen shows a scope this build has no wording for as its identifier — which is the
// honest fallback, and a bad thing to rely on: `items:write` is not a decision anybody can make.
// A scope added to a descriptor without a sentence beside it turns this red, which is the only
// moment somebody is still thinking about what the new scope means.
//
// The mapping is the screen's: `:` and `.` become `_`, because a message code's segments are
// separated by dots.
func TestEveryScopeCanBeSaidOnTheConsentScreen(t *testing.T) {
	t.Parallel()

	messages := loadCatalogue(t)
	for _, scope := range usecases.Scopes() {
		key := "app.consent.scope." + strings.NewReplacer(":", "_", ".", "_").Replace(scope)
		if _, ok := messages[key]; !ok {
			t.Errorf("the scope %q has no sentence: %s is missing from locales/en.json", scope, key)
		}
	}
}
