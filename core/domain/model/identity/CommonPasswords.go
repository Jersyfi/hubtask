// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package identity

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/port/text"
)

// The passwords this product refuses out of the box, and the reason the list is short.
//
// **It is written here rather than imported.** A real corpus - rockyou, SecLists, the Pwned
// Passwords set - is somebody else's work under somebody else's terms, and adding one is a
// supply-chain and licence decision rather than a commit (security.md §11). ADR-0068's own escape
// is taken instead: what ships is a short in-house list of the families every leak's top hundred is
// made of, and an installation that needs a real corpus points `sign_in.blocklist_file` at one. The
// switch then consults both, and neither says which refused a password - that would tell a guesser
// which corpus to avoid.
//
// **Comparison is on the folded form.** Every entry is stored as `FlattenPassword` produces it -
// lower case, the obvious substitutions undone - so `P@ssw0rd1` meets `password1` without the list
// having to carry every spelling of every word.
//
// **A substring, not an equality.** `Sommer2024!` is `sommer2024!`, which contains `sommer2024`;
// a list compared by equality would miss every password somebody decorated. The entries are
// therefore long enough that a substring match is not an accident - nothing here is shorter than
// six characters.

// commonPasswords is the list, folded. Kept as a map because the lookup runs on every set and the
// set is the only place it runs.
var commonPasswords = buildCommonPasswords()

// commonPasswordSeeds are the entries as they are written: readable, in families, so that a reader
// can see what the list is for rather than only what is in it.
var commonPasswordSeeds = []string{
	// Keyboard walks, the families rather than the variants: the fold and the substring match
	// cover the decorations people add to them.
	"qwerty", "qwertz", "azerty", "qwertyuiop", "asdfgh", "asdfghjkl", "zxcvbn", "zxcvbnm",
	"qazwsx", "qwaszx", "1qaz2wsx", "1q2w3e4r", "1q2w3e4r5t", "q1w2e3r4",
	// Runs of digits and letters.
	"123456", "1234567", "12345678", "123456789", "1234567890", "987654321",
	"111111", "000000", "121212", "123123", "112233", "abcdef", "abcdefg", "abcd1234",
	// The word itself, in the languages this product ships a sign-in screen in.
	"password", "passwort", "passw0rd", "password1", "password123", "motdepasse", "contrasena",
	"kennwort", "geheim", "secret", "wachtwoord",
	// "let me in" and its relatives.
	"letmein", "iloveyou", "trustno1", "changeme", "welcome", "welcome1", "login", "signin",
	"access", "starwars", "sunshine", "princess", "dragon", "monkey", "football", "baseball",
	"superman", "batman", "pokemon", "shadow", "master", "freedom", "whatever", "cheese",
	// Administrative names people use as passwords.
	"admin", "admin123", "administrator", "root123", "default", "guest123", "testtest",
	"test123", "temporary", "temp1234", "qwerty123", "letmein1",
	// The product's own name and the words around it. `hubtask` is also a context word, so this
	// is the part of it a context rule would not catch - the decorated forms in a list somebody
	// pasted from another installation.
	"hubtask", "hubtask1", "hubtask123", "hubtaskadmin",
	// Seasons and years, the pattern a quarterly expiry produces - which is exactly why ADR-0068
	// ships `max_age_days` off and says so.
	"sommer", "winter", "fruehling", "herbst", "summer", "spring", "autumn",
	"january", "december", "januar", "dezember",
	"sommer2024", "sommer2025", "sommer2026", "winter2024", "winter2025", "winter2026",
	"summer2024", "summer2025", "summer2026", "spring2025", "spring2026",
	"passwort2025", "passwort2026", "password2025", "password2026",
	// The strings a developer leaves behind.
	"changeit", "secret123", "insecure", "localhost", "example", "sample123",
	"development", "staging", "production", "notsecure", "placeholder",
	// The phrase every strength meter's documentation uses, which is therefore the phrase people
	// type into one.
	"correcthorsebatterystaple",
}

func buildCommonPasswords() map[string]struct{} {
	held := make(map[string]struct{}, len(commonPasswordSeeds))
	for _, seed := range commonPasswordSeeds {
		// Folded with the same function a candidate is, and never shorter than six: a shorter
		// entry would match by accident inside a long passphrase.
		folded := FlattenPassword(nil, seed)
		if len(folded) < 6 {
			continue
		}
		held[folded] = struct{}{}
	}
	return held
}

// IsCommonPassword reports whether the candidate carries one of the entries.
//
// The candidate is folded by the caller's normaliser, because NFKC is the port's and this function
// is the domain's - and a caller with no normaliser (the compatibility shim) gets the same answer
// for every ASCII password, which is all of them in this list.
func IsCommonPassword(form text.Normalizer, password string) bool {
	folded := FlattenPassword(form, password)
	if len(folded) < 6 {
		// Too short to carry an entry, and too short to pass the length rule either.
		return false
	}
	// The separators are stripped for a second look rather than refused: NIST asks for spaces to be
	// allowed, and the four-word passphrase every strength meter's documentation prints is typed
	// with spaces, with hyphens and with neither. One entry, three spellings caught.
	joined := separators.Replace(folded)
	for entry := range commonPasswords {
		if strings.Contains(folded, entry) || strings.Contains(joined, entry) {
			return true
		}
	}
	return false
}

// separators is the second look's fold. Only the four a person puts between words - anything more
// would start matching a short entry inside an unrelated passphrase.
var separators = strings.NewReplacer(" ", "", "-", "", "_", "", ".", "")

// CommonPasswordCount is how many entries ship. Read by the test that keeps the list from being
// quietly emptied, and by nothing else.
func CommonPasswordCount() int { return len(commonPasswords) }
