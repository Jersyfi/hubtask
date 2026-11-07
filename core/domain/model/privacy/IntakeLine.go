// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package privacy

import "strings"

// IntakePrefix opens the line the mail intake writes above what an account files on a requester's
// behalf: "From: name <address>", or "From: address" without a name.
const IntakePrefix = "From: "

// StripIntakeLine removes the intake's first line naming address from text, with the blank lines
// after it (third answers 2). It answers false for a text whose first line names somebody else or
// is not the intake's: a person's own words starting with "From:" stay.
func StripIntakeLine(text, address string) (string, bool) {
	address = strings.ToLower(strings.TrimSpace(address))
	if address == "" {
		return text, false
	}
	first, rest, _ := strings.Cut(text, "\n")
	line := strings.ToLower(strings.TrimSpace(first))
	if !strings.HasPrefix(line, strings.ToLower(IntakePrefix)) {
		return text, false
	}
	named := strings.TrimSpace(line[len(IntakePrefix):])
	if named != address && !strings.HasSuffix(named, "<"+address+">") {
		return text, false
	}
	return strings.TrimLeft(rest, "\r\n \t"), true
}

// WithoutIntakeLine removes the intake's first line from text whatever address it names, with the
// blank lines after it: what is copied out of an entry - a knowledge draft - must not carry a
// requester's name and address to where no erasure of the address looks. A first line starting
// with the prefix but naming no address is a person's own words and stays.
func WithoutIntakeLine(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	line := strings.TrimSpace(first)
	if !strings.HasPrefix(strings.ToLower(line), strings.ToLower(IntakePrefix)) {
		return text
	}
	named := strings.TrimSpace(line[len(IntakePrefix):])
	address := named
	if open := strings.LastIndex(named, "<"); open >= 0 && strings.HasSuffix(named, ">") {
		address = named[open+1 : len(named)-1]
	}
	if strings.ContainsAny(address, " \t") || !strings.Contains(address, "@") {
		return text
	}
	out, _ := StripIntakeLine(text, address)
	return out
}
