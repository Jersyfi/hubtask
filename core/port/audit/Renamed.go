// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"sort"
	"strings"
)

// Renamed is every action that changed its name, new name to old.
//
// An action is renamed, never rewritten: new entries carry the new name, and the entries already
// stored keep the one they were written with, because the hash covers the stored shape (audit.md §3) -
// rewriting a row would be indistinguishable from tampering with it. What keeps the two one action is
// the read: a search by either name, or by a family either belongs to, finds both (Aliases).
var Renamed = map[Action]Action{
	// SC-29: the second factor's actions are one family, `auth.mfa_*`.
	Action("auth.mfa_recovery_regenerated"): Action("mfa.recovery_regenerated"),
}

// Aliases answers the other names a prefix search must also match: the old name of every renamed
// action whose new name the prefix reaches, and the new name of every one whose old name it reaches.
// Empty for the empty prefix, which already matches everything.
func Aliases(prefix string) []string {
	if prefix == "" {
		return nil
	}
	var also []string
	for renamed, former := range Renamed {
		if strings.HasPrefix(string(renamed), prefix) {
			also = append(also, string(former))
		}
		if strings.HasPrefix(string(former), prefix) {
			also = append(also, string(renamed))
		}
	}
	sort.Strings(also)
	return also
}
