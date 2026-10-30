// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package shared

// ValidKey reports whether a stable key is one the configuration document can name: lower case,
// digits and underscores, starting with a letter, at most 63 - the same check the database holds
// a view's and a rule's key to (migration 0286).
func ValidKey(key string) bool {
	if key == "" || len(key) > 63 || key[0] < 'a' || key[0] > 'z' {
		return false
	}
	for _, r := range key {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}
