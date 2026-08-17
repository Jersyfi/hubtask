// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package service holds the domain services: the rules that belong to no single entity.
//
// They are pure functions over values. Nothing here reads a clock, a database, or a request -
// which is what makes the rules testable at their boundaries rather than through three layers of
// fakes.
package service

import (
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Rank keys, or "fractional indexing" (offline-sync.md §4.2).
//
// A position is a lexicographic key between its neighbours rather than an integer. Inserting
// between two rows writes one row instead of renumbering every successor - which is what lets two
// offline devices insert into the same list without either one's order being discarded on sync
// (test SY-4).
//
// The scheme is the established one described by David Greenspan ("Implementing Fractional
// Indexing", 2021) and implemented by the `fractional-indexing` library: a key is an integer part
// followed by an optional fraction, and the integer part's first character encodes how many digits
// follow it. That encoding is what keeps appending cheap. Without it, every append would have to
// sit lexicographically inside the previous key's space, and a thousand appends would produce a
// thousand-character key; with it, `a0` is followed by `a1`, `az` by `b00`, and the length grows
// with the logarithm of the number of rows.
//
// Letters `a`..`z` head the non-negative integers and `A`..`Z` the negative ones, so a key can
// always be produced *before* the first row as well - which is what "move to the top" needs.

// orderKeyDigits are the digits of a key, in ASCII order, so that comparing two keys as strings
// compares them as numbers. Base 62 keeps a key short, and a database sorts it with a plain index.
const orderKeyDigits = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// orderKeyZero is the first key of an empty list: integer zero, no fraction.
const orderKeyZero = "a0"

// smallestOrderInteger is the lower end of the integer space. A key may not be exactly this
// value, because there would then be no key left below it and "move to the top" would have no
// answer.
var smallestOrderInteger = "A" + strings.Repeat("0", 26)

// OrderKeyBetween returns a key that sorts strictly after previous and strictly before next.
//
// An empty previous means "before everything", an empty next means "after everything". Both empty
// gives the first key of an empty list. A key is never empty, so the empty string is unambiguous
// as "no bound".
func OrderKeyBetween(previous, next string) (string, error) {
	if err := validateOrderKey(previous); err != nil {
		return "", err
	}
	if err := validateOrderKey(next); err != nil {
		return "", err
	}
	if previous != "" && next != "" && previous >= next {
		// The caller passed neighbours that are not neighbours, or passed them the wrong way
		// round. Nothing a client sent could have caused it (security.md §9).
		return "", shared.ErrInternal.
			WithDetail("ordering.bounds_invalid").
			WithParams(map[string]string{"previous": previous, "next": next})
	}

	switch {
	case previous == "" && next == "":
		return orderKeyZero, nil
	case previous == "":
		return keyBefore(next)
	case next == "":
		return keyAfter(previous)
	}
	return keyBetween(previous, next)
}

// OrderKeyAfter returns a key that sorts after previous. It is what appending to a list needs: an
// empty previous is an empty list.
func OrderKeyAfter(previous string) (string, error) { return OrderKeyBetween(previous, "") }

// keyBefore is the "move to the top" case: one integer step below next, or half of next's
// fraction when there is no integer step left.
func keyBefore(next string) (string, error) {
	integer, fraction := splitOrderKey(next)

	if integer == smallestOrderInteger {
		middle, err := midpoint("", fraction)
		return integer + middle, err
	}
	// next carries a fraction, so its own integer part already sorts below it.
	if integer < next {
		return integer, nil
	}

	decremented, err := decrementInteger(integer)
	if err != nil {
		return "", err
	}
	if decremented == "" {
		return "", shared.ErrInternal.
			WithDetail("ordering.space_exhausted").
			WithParams(map[string]string{"next": next})
	}
	return decremented, nil
}

// keyAfter is the append case, and the one that has to stay short: one integer step up, and only
// when the integer space is exhausted does the key grow a fraction.
func keyAfter(previous string) (string, error) {
	integer, fraction := splitOrderKey(previous)

	incremented, err := incrementInteger(integer)
	if err != nil {
		return "", err
	}
	if incremented != "" {
		return incremented, nil
	}

	middle, err := midpoint(fraction, "")
	return integer + middle, err
}

func keyBetween(previous, next string) (string, error) {
	previousInteger, previousFraction := splitOrderKey(previous)
	nextInteger, nextFraction := splitOrderKey(next)

	if previousInteger == nextInteger {
		middle, err := midpoint(previousFraction, nextFraction)
		return previousInteger + middle, err
	}

	incremented, err := incrementInteger(previousInteger)
	if err != nil {
		return "", err
	}
	// One integer step up is the shortest key there is, as long as it still sorts below next.
	if incremented != "" && incremented < next {
		return incremented, nil
	}

	middle, err := midpoint(previousFraction, "")
	return previousInteger + middle, err
}

// midpoint returns the shortest fraction strictly between a and b, where an empty b means "1".
// Both are fractions without a leading dot and without a trailing zero digit.
func midpoint(a, b string) (string, error) {
	zero := orderKeyDigits[0]

	if b != "" && a >= b {
		return "", shared.ErrInternal.
			WithDetail("ordering.bounds_invalid").
			WithParams(map[string]string{"previous": a, "next": b})
	}
	if strings.HasSuffix(a, string(zero)) || (b != "" && strings.HasSuffix(b, string(zero))) {
		// A trailing zero is a fraction with a shorter equal - it would break the comparison the
		// whole scheme rests on.
		return "", shared.ErrInternal.
			WithDetail("ordering.key_malformed").
			WithParams(map[string]string{"key": a + " " + b})
	}

	if b != "" {
		// A shared prefix is carried over untouched; only the first differing digit decides.
		common := 0
		for common < len(b) {
			digit := zero
			if common < len(a) {
				digit = a[common]
			}
			if digit != b[common] {
				break
			}
			common++
		}
		if common > 0 {
			rest, err := midpoint(cut(a, common), b[common:])
			return b[:common] + rest, err
		}
	}

	lower := 0
	if a != "" {
		lower = digitIndex(a[0])
	}
	upper := len(orderKeyDigits)
	if b != "" {
		upper = digitIndex(b[0])
	}

	if upper-lower > 1 {
		// Rounded rather than truncated, so that the digit sits in the middle of the gap and the
		// next insertion on either side has the same room.
		return string(orderKeyDigits[(lower+upper+1)/2]), nil
	}
	if len(b) > 1 {
		return b[:1], nil
	}
	// The digits are adjacent and b has nothing more to give: keep a's digit and look one
	// position further, where b no longer constrains anything.
	rest, err := midpoint(cut(a, 1), "")
	return string(orderKeyDigits[lower]) + rest, err
}

// cut is s[n:] for a string that may be shorter than n.
func cut(s string, n int) string {
	if n >= len(s) {
		return ""
	}
	return s[n:]
}

// integerLength reads the length the head character declares. It is the trick that makes appending
// cheap: `b00` sorts after `az` because `b` follows `a`, not because it is longer.
func integerLength(head byte) (int, error) {
	switch {
	case head >= 'a' && head <= 'z':
		return int(head-'a') + 2, nil
	case head >= 'A' && head <= 'Z':
		return int('Z'-head) + 2, nil
	}
	return 0, shared.ErrInternal.
		WithDetail("ordering.key_malformed").
		WithParams(map[string]string{"key": string(head)})
}

// splitOrderKey separates the integer part from the fraction.
//
// A key whose head declares more digits than it has is returned whole, so that validateInteger
// gets to report it rather than this function reaching past the end of the string.
func splitOrderKey(key string) (integer, fraction string) {
	length, err := integerLength(key[0])
	if err != nil || length > len(key) {
		return key, ""
	}
	return key[:length], key[length:]
}

// incrementInteger returns the next integer, or the empty string when there is none - the top of
// the space, where the caller falls back to a fraction.
func incrementInteger(integer string) (string, error) {
	if err := validateInteger(integer); err != nil {
		return "", err
	}

	head, digits := integer[0], []byte(integer[1:])
	carry := true
	for i := len(digits) - 1; carry && i >= 0; i-- {
		if next := digitIndex(digits[i]) + 1; next == len(orderKeyDigits) {
			digits[i] = orderKeyDigits[0]
		} else {
			digits[i] = orderKeyDigits[next]
			carry = false
		}
	}

	if !carry {
		return string(head) + string(digits), nil
	}
	switch head {
	case 'Z':
		// The last negative integer is followed by zero.
		return orderKeyZero, nil
	case 'z':
		return "", nil
	}
	if head+1 > 'a' {
		digits = append(digits, orderKeyDigits[0])
	} else {
		digits = digits[1:]
	}
	return string(head+1) + string(digits), nil
}

// decrementInteger returns the previous integer, or the empty string at the bottom of the space.
func decrementInteger(integer string) (string, error) {
	if err := validateInteger(integer); err != nil {
		return "", err
	}

	last := orderKeyDigits[len(orderKeyDigits)-1]
	head, digits := integer[0], []byte(integer[1:])
	borrow := true
	for i := len(digits) - 1; borrow && i >= 0; i-- {
		if previous := digitIndex(digits[i]) - 1; previous < 0 {
			digits[i] = last
		} else {
			digits[i] = orderKeyDigits[previous]
			borrow = false
		}
	}

	if !borrow {
		return string(head) + string(digits), nil
	}
	switch head {
	case 'a':
		return string('Z') + string(last), nil
	case 'A':
		return "", nil
	}
	if head-1 < 'Z' {
		digits = append(digits, last)
	} else {
		digits = digits[1:]
	}
	return string(head-1) + string(digits), nil
}

func validateInteger(integer string) error {
	if integer == "" {
		return shared.ErrInternal.WithDetail("ordering.key_malformed").
			WithParams(map[string]string{"key": integer})
	}
	length, err := integerLength(integer[0])
	if err != nil {
		return err
	}
	if len(integer) != length {
		return shared.ErrInternal.
			WithDetail("ordering.key_malformed").
			WithParams(map[string]string{"key": integer})
	}
	return nil
}

// validateOrderKey refuses a key this package could not have produced. The keys come from storage,
// so an unusable one means a row was written by something that does not share this scheme - and
// continuing would place the new key in an unpredictable position. The empty string is the absent
// bound and passes.
func validateOrderKey(key string) error {
	if key == "" {
		return nil
	}
	malformed := shared.ErrInternal.
		WithDetail("ordering.key_malformed").
		WithParams(map[string]string{"key": key})

	for i := range len(key) {
		if digitIndex(key[i]) < 0 {
			return malformed
		}
	}
	if key == smallestOrderInteger {
		// Permitted as a prefix, refused on its own: a key there would leave nothing below it.
		return malformed
	}

	integer, fraction := splitOrderKey(key)
	if err := validateInteger(integer); err != nil {
		return err
	}
	if strings.HasSuffix(fraction, string(orderKeyDigits[0])) {
		return malformed
	}
	return nil
}

func digitIndex(digit byte) int { return strings.IndexByte(orderKeyDigits, digit) }
