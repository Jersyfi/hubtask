// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/port/text"
)

// The shared corpus is read here rather than beside the rule it checks, because the domain may not
// import a serialiser (project-structure.md §3) - and reading it from the layer that *applies* the
// rule is the more honest place anyway: what the client predicts, this layer decides.

// The shared corpus of ADR-0068 §7, read here and by `pnpm --filter @hubtask/webapp test`. Two
// implementations of one rule cannot be prevented - the client has to predict what this decides -
// so they are held to one table instead.
type ruleFixture struct {
	Cases []struct {
		Name  string `json:"name"`
		Rules struct {
			MinLength       int  `json:"min_length"`
			MinLowercase    int  `json:"min_lowercase"`
			MinUppercase    int  `json:"min_uppercase"`
			MinDigits       int  `json:"min_digits"`
			MinSymbols      int  `json:"min_symbols"`
			MinClasses      int  `json:"min_classes"`
			MaxRepeat       *int `json:"max_repeat"`
			CommonPasswords bool `json:"common_passwords"`
			ContextWords    bool `json:"context_words"`
			BreachCheck     bool `json:"breach_check"`
			HistoryCount    int  `json:"history_count"`
			NotCurrent      bool `json:"not_current"`
		} `json:"rules"`
		Context struct {
			Email         string `json:"email"`
			DisplayName   string `json:"display_name"`
			WorkspaceName string `json:"workspace_name"`
			WorkspaceHost string `json:"workspace_host"`
		} `json:"context"`
		Password        string   `json:"password"`
		LocalViolations []string `json:"local_violations"`
		ServerLines     []string `json:"server_lines"`
	} `json:"cases"`
}

func TestThePasswordRulesAgreeWithTheSharedFixture(t *testing.T) {
	raw, err := os.ReadFile("../../../../api/fixtures/password-rules.json")
	if err != nil {
		t.Fatalf("the fixture could not be read: %v", err)
	}
	var fixture ruleFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("the fixture does not parse: %v", err)
	}
	if len(fixture.Cases) == 0 {
		t.Fatal("the fixture holds no cases")
	}

	for _, entry := range fixture.Cases {
		t.Run(entry.Name, func(t *testing.T) {
			policy := domain.PasswordPolicy{
				MinLength:       entry.Rules.MinLength,
				MinLowercase:    entry.Rules.MinLowercase,
				MinUppercase:    entry.Rules.MinUppercase,
				MinDigits:       entry.Rules.MinDigits,
				MinSymbols:      entry.Rules.MinSymbols,
				MinClasses:      entry.Rules.MinClasses,
				CommonPasswords: entry.Rules.CommonPasswords,
				ContextWords:    entry.Rules.ContextWords,
				BreachCheck:     entry.Rules.BreachCheck,
				HistoryCount:    entry.Rules.HistoryCount,
			}
			if entry.Rules.MaxRepeat != nil {
				policy.MaxRepeat = *entry.Rules.MaxRepeat
			}

			violations := domain.LocalPasswordViolations(policy, text.Composing{}, entry.Password,
				domain.PasswordContext{
					Email:         entry.Context.Email,
					DisplayName:   entry.Context.DisplayName,
					WorkspaceName: entry.Context.WorkspaceName,
					WorkspaceHost: entry.Context.WorkspaceHost,
				})

			named := make([]string, 0, len(violations))
			for _, violation := range violations {
				named = append(named, string(violation.Rule))
			}
			want := entry.LocalViolations
			if want == nil {
				want = []string{}
			}
			if !reflect.DeepEqual(named, want) {
				t.Errorf("violations %v, want %v", named, want)
			}
		})
	}
}
