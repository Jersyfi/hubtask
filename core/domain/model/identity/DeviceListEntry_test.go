// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"testing"
)

func TestADeviceListEntryHoldsAHostAndAName(t *testing.T) {
	for _, tc := range []struct {
		name, host, display string
		ok                  bool
		want                DeviceListEntry
	}{
		{"a subdomain", "Acme.Tasks.Example.net", " Acme Ltd ", true, DeviceListEntry{"acme.tasks.example.net", "Acme Ltd"}},
		{"an own domain", "tasks.acme.com", "Acme", true, DeviceListEntry{"tasks.acme.com", "Acme"}},
		{"no host", "", "Acme", false, DeviceListEntry{}},
		{"a scheme", "https://acme.example.net", "Acme", false, DeviceListEntry{}},
		{"a port", "acme.example.net:8443", "Acme", false, DeviceListEntry{}},
		{"a path", "acme.example.net/x", "Acme", false, DeviceListEntry{}},
		{"the root's dot", "acme.example.net.", "Acme", false, DeviceListEntry{}},
		{"no name", "acme.example.net", "  ", false, DeviceListEntry{}},
		{"a name too long", "acme.example.net", strings.Repeat("a", DeviceListEntryNameMax+1), false, DeviceListEntry{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := NewDeviceListEntry(tc.host, tc.display)
			if ok != tc.ok || got != tc.want {
				t.Fatalf("NewDeviceListEntry(%q, %q) = %+v, %v; want %+v, %v", tc.host, tc.display, got, ok, tc.want, tc.ok)
			}
		})
	}
}
