// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package knowledge

import (
	"slices"
	"strings"
	"testing"
)

func TestAPagesLabelsHaveOneWrittenForm(t *testing.T) {
	tooMany := make([]string, MaxPageLabels+1)
	for i := range tooMany {
		tooMany[i] = "l" + strings.Repeat("x", i+1)
	}
	tests := []struct {
		name string
		raw  []string
		want []string
		code string
	}{
		{name: "lower case, each once, alphabetical", raw: []string{" Onboarding", "hr", "onboarding", "v1.2"}, want: []string{"hr", "onboarding", "v1.2"}},
		{name: "letters of any script", raw: []string{"Übergabe", "how-to", "q_3"}, want: []string{"how-to", "q_3", "übergabe"}},
		{name: "none", raw: nil, want: []string{}},
		{name: "empty", raw: []string{"  "}, code: "pages.label_empty"},
		{name: "two words", raw: []string{"two words"}, code: "pages.label_malformed"},
		{name: "a slash", raw: []string{"a/b"}, code: "pages.label_malformed"},
		{name: "too long", raw: []string{strings.Repeat("a", MaxLabelLength+1)}, code: "pages.label_too_long"},
		{name: "too many", raw: tooMany, code: "pages.too_many_labels"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Labels(tt.raw)
			if tt.code != "" {
				if detail(err) != tt.code {
					t.Fatalf("Labels(%q) = %v, want %s", tt.raw, err, tt.code)
				}
				return
			}
			if err != nil || !slices.Equal(got, tt.want) {
				t.Errorf("Labels(%q) = %q, %v; want %q", tt.raw, got, err, tt.want)
			}
		})
	}
}
