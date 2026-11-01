// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package mail

import (
	"reflect"
	"testing"
)

// Alert A-29's numbers per installation app: Graph and IMAP share the Microsoft app, and an app
// whose mailboxes all went reads zero rather than keeping its last count.
func TestInstallationAppsFoldPresetsOntoTheirApp(t *testing.T) {
	tests := []struct {
		name string
		in   []PresetCount
		want []AppHealth
	}{
		{"nothing connected", nil, []AppHealth{{App: "google"}, {App: "microsoft"}}},
		{
			"graph and imap share the microsoft app",
			[]PresetCount{{Provider: "microsoft", Connected: 3, Failing: 1}, {Provider: "microsoft_graph", Connected: 2, Failing: 2}},
			[]AppHealth{{App: "google"}, {App: "microsoft", Connected: 5, Failing: 3}},
		},
		{
			"a provider without a preset keeps its own name",
			[]PresetCount{{Provider: "google", Connected: 1}, {Provider: "custom", Connected: 1, Failing: 1}},
			[]AppHealth{{App: "custom", Connected: 1, Failing: 1}, {App: "google", Connected: 1}, {App: "microsoft"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := InstallationApps(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("InstallationApps = %v, want %v", got, tt.want)
			}
		})
	}
}
