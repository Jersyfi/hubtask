// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"testing"

	repository "github.com/Jersyfi/hubtask/core/application/repository/identity"
)

// A page carries its total where the walk counted it and leaves the field out where it did not
// (api-guidelines.md §5).
func TestAPageCarriesItsTotalOnlyWhereCounted(t *testing.T) {
	counted := 1240
	for _, tc := range []struct {
		name string
		info repository.PageInfo
		want any
	}{
		{"counted", repository.PageInfo{HasMore: true, NextCursor: "c", Total: &counted}, 1240},
		{"not counted", repository.PageInfo{HasMore: false}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, _ := pageOutput(nil, tc.info)["page"].(map[string]any)
			got, held := page["total"]
			if tc.want == nil {
				if held {
					t.Fatalf("total = %v, want the field absent", got)
				}
				return
			}
			if got != tc.want {
				t.Fatalf("total = %v, want %v", got, tc.want)
			}
		})
	}
}
