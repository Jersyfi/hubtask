// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"
	"unicode/utf8"
)

// DeviceListEntryNameMax bounds the display name an entry carries, as the workspace's own name is
// bounded: an entry travels in an address, and a name longer than any workspace's is no name.
const DeviceListEntryNameMax = 200

// DeviceListEntry is one workspace as a device's list of workspaces holds it (ADR-0095 §7,
// UC-ID-25): its main address and its display name - nothing of the person who signed in, no
// token. The installation signs it, so the list takes only a workspace the installation named.
type DeviceListEntry struct {
	Host string
	Name string
}

// NewDeviceListEntry checks what an entry may hold. Ok is false for an entry that holds no host,
// a host with a scheme, a path or a port, or no name.
func NewDeviceListEntry(host, name string) (DeviceListEntry, bool) {
	host = strings.ToLower(strings.TrimSpace(host))
	name = strings.TrimSpace(name)
	if host == "" || strings.ContainsAny(host, ":/?#@ ") || strings.HasSuffix(host, ".") {
		return DeviceListEntry{}, false
	}
	if name == "" || utf8.RuneCountInString(name) > DeviceListEntryNameMax || !utf8.ValidString(name) {
		return DeviceListEntry{}, false
	}
	return DeviceListEntry{Host: host, Name: name}, true
}
