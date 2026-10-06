// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package audit

import (
	"maps"
	"sort"
	"strings"
)

// renamed is every action that changed its name, new name to old.
//
// An action is renamed, never rewritten: new entries carry the new name, and the entries already
// stored keep the one they were written with, because the hash covers the stored shape (audit.md §3) -
// rewriting a row would be indistinguishable from tampering with it. What keeps the two one action is
// the read: a search by either name, or by a family either belongs to, finds both (Aliases). Not
// exported, so that nothing outside this file can add to it or take from it at run time.
var renamed = map[Action]Action{
	// SC-29: the second factor's actions are one family, `auth.mfa_*`.
	Action("auth.mfa_recovery_regenerated"): Action("mfa.recovery_regenerated"),
}

// Renamed answers every rename, current name to former, as a copy.
func Renamed() map[Action]Action { return maps.Clone(renamed) }

// Aliases answers the other names a prefix search must also match: the old name of every renamed
// action whose new name the prefix reaches, and the new name of every one whose old name it reaches.
// Empty for the empty prefix, which already matches everything.
func Aliases(prefix string) []string {
	if prefix == "" {
		return nil
	}
	var also []string
	for current, former := range renamed {
		if strings.HasPrefix(string(current), prefix) {
			also = append(also, string(former))
		}
		if strings.HasPrefix(string(former), prefix) {
			also = append(also, string(current))
		}
	}
	sort.Strings(also)
	return also
}
