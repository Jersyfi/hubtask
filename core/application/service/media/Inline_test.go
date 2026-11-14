// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package media

import (
	"testing"

	domain "github.com/Jersyfi/hubtask/core/domain/model/media"
)

// The projection tells the file viewer which objects it may hand the browser by URL (F-5): only a
// sealed object of an allowlisted type, never a claim still waiting for its judgement.
func TestTheProjectionSaysWhichObjectsTheBrowserMayDecode(t *testing.T) {
	cases := []struct {
		name   string
		status domain.Status
		ctype  string
		want   bool
	}{
		{"a sealed video", domain.StatusReady, "video/mp4", true},
		{"a sealed picture", domain.StatusReady, "image/png", true},
		{"a sealed PDF is drawn by the viewer, not the browser", domain.StatusReady, "application/pdf", false},
		{"a sealed SVG", domain.StatusReady, "image/svg+xml", false},
		{"a claimed video not yet judged", domain.StatusPending, "video/mp4", false},
		{"a video held for the scanner", domain.StatusPendingScan, "video/mp4", false},
	}
	for _, each := range cases {
		t.Run(each.name, func(t *testing.T) {
			out := mediaOutput(domain.Object{Status: each.status, ContentType: each.ctype})
			if got, _ := out["inline"].(bool); got != each.want {
				t.Errorf("inline = %v, want %v", got, each.want)
			}
		})
	}
}
