// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

import (
	"strings"
	"time"
)

// LoadZone loads an IANA zone a person, a workspace or an entry names, false where it is no such
// name. `time.LoadLocation` answers the server's own zone for "Local" and UTC for "": the first
// differs between the nodes of one installation and no browser knows it, so a stored "Local"
// moves every day it anchors and makes the web app's date formatting throw.
func LoadZone(name string) (*time.Location, bool) {
	name = strings.TrimSpace(name)
	if name == "" || strings.EqualFold(name, "Local") {
		return nil, false
	}
	location, err := time.LoadLocation(name)
	return location, err == nil
}
