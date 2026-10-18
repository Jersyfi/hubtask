// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

// SecondFactor is what an account holds as its second factor (identity.md §23.2): the authenticator
// app, armed, and its passkeys and security keys. Every reading of "does this account hold one" and
// "may this one go" asks it, so the profile, turning the app off, the codes and removing a key never
// disagree.
type SecondFactor struct {
	// App is an armed authenticator app; an enrolment begun and never confirmed is none.
	App bool
	// Keys is how many WebAuthn credentials the account holds.
	Keys int
}

// Held is whether the account holds a second factor at all.
func (f SecondFactor) Held() bool { return f.App || f.Keys > 0 }

// AppRemovable is whether the app may be turned off, where required says whether the workspace's
// rule demands a factor of this person: only while something else remains to answer the rule.
func (f SecondFactor) AppRemovable(required bool) bool {
	return f.App && (!required || f.Keys > 0)
}

// KeyRemovable is whether one of the keys may be removed, under the same reading: the last one goes
// only where the app is armed or nothing demands a factor.
func (f SecondFactor) KeyRemovable(required bool) bool {
	return f.Keys > 0 && (!required || f.App || f.Keys > 1)
}
