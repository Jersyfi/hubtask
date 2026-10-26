// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package mail is the mail intake's own rules (ADR-0094): where a message is meant to go - read off
// its subject and the address it was sent to - and who may write at all.
package mail

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Key is a ticket key: a collection's handle and an entry's number in it, "EINKAUF-42".
type Key struct {
	Handle string
	Number int64
}

// String is the key as people write it.
func (k Key) String() string {
	return strings.ToUpper(k.Handle) + "-" + strconv.FormatInt(k.Number, 10)
}

// Door is how a message reached the intake, which decides which address forms carry a route.
type Door struct {
	// Mailbox is a connected mailbox's own address, "familie@gmail.com": a plus tag on it routes,
	// "familie+einkauf@gmail.com".
	Mailbox string
	// Domain is the operator's receiving domain and Workspace the workspace's name on it:
	// "einkauf.familie-meier@in.hubtask.app" and "familie-meier+einkauf@in.hubtask.app" both route.
	Domain    string
	Workspace string
}

// Route is what a message says about where it is meant to go: the keys and handles it names, most
// explicit first. They are candidates - "COVID-19" in a subject looks like a key and names nothing -
// and the intake takes the first that resolves.
type Route struct {
	Keys    []Key
	Handles []string
}

var (
	bracketedKey = regexp.MustCompile(`\[([A-Za-z][A-Za-z0-9]{1,19})-([0-9]{1,12})\]`)
	// The two below match without their surroundings - Go's expressions have no look-around - and
	// standing alone is checked on the indexes (alone).
	bareKey    = regexp.MustCompile(`([A-Za-z][A-Za-z0-9]{1,19})-([0-9]{1,12})`)
	hashHandle = regexp.MustCompile(`#([\pL][\pL\pN]{1,19})`)
	tagKey     = regexp.MustCompile(`^([a-z][a-z0-9]{1,19})-([0-9]{1,12})$`)
	tagHandle  = regexp.MustCompile(`^[a-z][a-z0-9]{1,19}$`)
)

// RouteOf reads a message's route. The address comes first because nobody types it by accident;
// then the subject's bracketed key, its #handle, and last a bare key in its text.
func RouteOf(subject string, recipients []string, door Door) Route {
	var route Route
	for _, recipient := range recipients {
		tag, ok := door.TagOf(recipient)
		if !ok {
			continue
		}
		if key, ok := keyOf(tag); ok {
			route.addKey(key)
		} else if tagHandle.MatchString(tag) {
			route.addHandle(tag)
		}
	}
	for _, match := range bracketedKey.FindAllStringSubmatch(subject, -1) {
		if key, ok := keyFrom(match[1], match[2]); ok {
			route.addKey(key)
		}
	}
	for _, at := range alone(subject, hashHandle, "") {
		for _, handle := range HandleSpellings(subject[at[2]:at[3]]) {
			route.addHandle(handle)
		}
	}
	for _, at := range alone(subject, bareKey, "#@+.-[") {
		if key, ok := keyFrom(subject[at[2]:at[3]], subject[at[4]:at[5]]); ok {
			route.addKey(key)
		}
	}
	return route
}

// TagOf is what an address says beyond the door itself: the plus tag on the mailbox or on the
// workspace's name, or the label in front of the workspace's name on the operator's domain. False
// for an address that is not this door's at all; true with an empty tag for the door's own address.
func (d Door) TagOf(recipient string) (string, bool) {
	local, domain, ok := split(recipient)
	if !ok {
		return "", false
	}
	if box, boxDomain, ok := split(d.Mailbox); ok && domain == boxDomain {
		base, tag, _ := strings.Cut(local, "+")
		if base == box {
			return tag, true
		}
	}
	if d.Domain == "" || d.Workspace == "" || domain != strings.ToLower(d.Domain) {
		return "", false
	}
	workspace := strings.ToLower(d.Workspace)
	if local == workspace {
		return "", true
	}
	if base, tag, ok := strings.Cut(local, "+"); ok && base == workspace {
		return tag, true
	}
	if label, ok := strings.CutSuffix(local, "."+workspace); ok && label != "" {
		return label, true
	}
	return "", false
}

// WorkspaceOf is the workspace an address on the operator's receiving domain names - the part after
// the label's dot or before the plus - so the intake knows whose message it is before it knows
// anything else.
func WorkspaceOf(recipient, receivingDomain string) (string, bool) {
	local, domain, ok := split(recipient)
	if !ok || receivingDomain == "" || domain != strings.ToLower(receivingDomain) {
		return "", false
	}
	if base, _, found := strings.Cut(local, "+"); found {
		local = base
	}
	if index := strings.LastIndex(local, "."); index >= 0 {
		local = local[index+1:]
	}
	if !workspaceName.MatchString(local) {
		return "", false
	}
	return local, true
}

// Title is the subject an entry is named by: the routing tokens and the reply and forward prefixes
// taken out, because "#einkauf" and "WG:" are how the mail was sent, not what it is about.
func Title(subject string) string {
	title := bracketedKey.ReplaceAllString(subject, " ")
	for _, at := range reversed(alone(title, hashHandle, "")) {
		title = title[:at[0]] + " " + title[at[1]:]
	}
	title = strings.Join(strings.Fields(title), " ")
	for {
		trimmed := strings.TrimSpace(replyPrefix.ReplaceAllString(title, ""))
		if trimmed == title {
			break
		}
		title = trimmed
	}
	return title
}

var replyPrefix = regexp.MustCompile(`(?i)^(re|aw|fw|fwd|wg|antw|tr|sv|vs)\s*(\[\d+\])?\s*:\s*`)

// HandleSpellings are the handles a typed word may mean: lower case with the accents dropped, the
// way the store derives one from a name ("Einkäufe" → "einkaufe"), and with the German umlauts
// spelled out, the way a person may have typed it ("einkaeufe").
func HandleSpellings(word string) []string {
	lower := strings.ToLower(word)
	plain := strings.NewReplacer("ä", "a", "ö", "o", "ü", "u", "ß", "ss", "é", "e", "è", "e", "à", "a").Replace(lower)
	spelled := strings.NewReplacer("ä", "ae", "ö", "oe", "ü", "ue", "ß", "ss", "é", "e", "è", "e", "à", "a").Replace(lower)
	var out []string
	for _, candidate := range []string{plain, spelled} {
		if tagHandle.MatchString(candidate) && !contains(out, candidate) {
			out = append(out, candidate)
		}
	}
	return out
}

// alone is the matches that stand as words of their own: no letter or digit, nor one of the extra
// characters, right before or after - "#einkauf" but not "a#einkauf", "WEB-42" but not "x@WEB-42".
func alone(text string, pattern *regexp.Regexp, extra string) [][]int {
	var out [][]int
	for _, at := range pattern.FindAllStringSubmatchIndex(text, -1) {
		before, _ := utf8.DecodeLastRuneInString(text[:at[0]])
		after, _ := utf8.DecodeRuneInString(text[at[1]:])
		if at[0] > 0 && (isWordRune(before) || strings.ContainsRune(extra, before)) {
			continue
		}
		if at[1] < len(text) && (isWordRune(after) || after == '-' || strings.ContainsRune(extra, after)) {
			continue
		}
		out = append(out, at)
	}
	return out
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' }

func reversed(matches [][]int) [][]int {
	for i, j := 0, len(matches)-1; i < j; i, j = i+1, j-1 {
		matches[i], matches[j] = matches[j], matches[i]
	}
	return matches
}

func keyOf(tag string) (Key, bool) {
	match := tagKey.FindStringSubmatch(strings.ToLower(tag))
	if match == nil {
		return Key{}, false
	}
	return keyFrom(match[1], match[2])
}

func keyFrom(handle, number string) (Key, bool) {
	n, err := strconv.ParseInt(number, 10, 64)
	if err != nil || n < 1 {
		return Key{}, false
	}
	return Key{Handle: strings.ToLower(handle), Number: n}, true
}

func (r *Route) addKey(key Key) {
	for _, known := range r.Keys {
		if known == key {
			return
		}
	}
	r.Keys = append(r.Keys, key)
}

func (r *Route) addHandle(handle string) {
	if !contains(r.Handles, handle) {
		r.Handles = append(r.Handles, handle)
	}
}

// split is an address's local part and domain, lower case, without a display name around it.
func split(address string) (string, string, bool) {
	address = strings.TrimSpace(address)
	if open := strings.LastIndex(address, "<"); open >= 0 {
		address = strings.TrimSuffix(address[open+1:], ">")
	}
	local, domain, ok := strings.Cut(strings.ToLower(address), "@")
	if !ok || local == "" || domain == "" || strings.Contains(domain, "@") {
		return "", "", false
	}
	return local, domain, true
}

func contains(values []string, value string) bool {
	for _, known := range values {
		if known == value {
			return true
		}
	}
	return false
}

// ValidHandle reports whether a handle is one a collection may carry: a letter, then letters and
// digits, two to twenty in all - no hyphen, so "WEB-42" can only ever be a key.
func ValidHandle(handle string) bool { return tagHandle.MatchString(handle) }
