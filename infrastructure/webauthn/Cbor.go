// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package webauthn

import (
	"encoding/binary"
	"math"
	"unicode/utf8"
)

// The CBOR subset a WebAuthn ceremony needs (RFC 8949), and nothing else (ADR-0085 §1).
//
// An attestation object, a COSE key and the authenticator's extensions are maps of integers, byte
// strings and text strings, with the occasional array and boolean. So this reads major types 0 to 5
// and the simple values false, true and null; everything else - tags, floats, `undefined`, the
// indefinite lengths - is refused rather than skipped, because a decoder that skips what it does not
// know is one whose input can mean two things. Bounds: a depth, a size, and no container that claims
// more items than bytes remain.

const (
	// maxCBORDepth bounds nesting. The deepest legitimate structure, an extension's value inside the
	// extensions map inside the authenticator data, is three levels.
	maxCBORDepth = 16
	// maxCBORSize bounds one encoded item, which bounds the work any input can cause.
	maxCBORSize = 64 << 10
)

type cborKind uint8

const (
	cborUnsigned cborKind = iota + 1
	cborNegative
	cborBytes
	cborText
	cborArray
	cborMap
	cborBool
	cborNull
)

// cborValue is one decoded item. Integers keep their argument: an unsigned one is `n`, a negative
// one is `-1 - n`, so the whole of both ranges is represented without overflow.
type cborValue struct {
	kind    cborKind
	n       uint64
	bytes   []byte
	array   []cborValue
	pairs   []cborPair
	boolean bool
}

type cborPair struct {
	key   cborValue
	value cborValue
}

// decodeCBOR reads one item from the front of data and answers what follows it. Whether trailing
// bytes are allowed is the caller's to decide: inside the authenticator data a COSE key is followed
// by the extensions, at the top of an attestation object nothing may follow.
func decodeCBOR(data []byte) (cborValue, []byte, error) {
	if len(data) > maxCBORSize {
		return cborValue{}, nil, refuse("cbor.too_large")
	}
	d := cborDecoder{data: data}
	value, err := d.item(0)
	if err != nil {
		return cborValue{}, nil, err
	}
	return value, d.data[d.at:], nil
}

// decodeCBORWhole reads exactly one item and refuses a trailing byte.
func decodeCBORWhole(data []byte) (cborValue, error) {
	value, rest, err := decodeCBOR(data)
	if err != nil {
		return cborValue{}, err
	}
	if len(rest) != 0 {
		return cborValue{}, refuse("cbor.trailing_bytes")
	}
	return value, nil
}

type cborDecoder struct {
	data []byte
	at   int
}

func (d *cborDecoder) remaining() int { return len(d.data) - d.at }

func (d *cborDecoder) item(depth int) (cborValue, error) {
	if depth > maxCBORDepth {
		return cborValue{}, refuse("cbor.too_deep")
	}
	if d.remaining() < 1 {
		return cborValue{}, refuse("cbor.truncated")
	}
	initial := d.data[d.at]
	d.at++
	major, info := initial>>5, initial&0x1f

	if major == 7 {
		return d.simple(info)
	}
	if major == 6 {
		return cborValue{}, refuse("cbor.tag")
	}
	argument, err := d.argument(info)
	if err != nil {
		return cborValue{}, err
	}

	switch major {
	case 0:
		return cborValue{kind: cborUnsigned, n: argument}, nil
	case 1:
		return cborValue{kind: cborNegative, n: argument}, nil
	case 2, 3:
		length, err := d.length(argument, 1)
		if err != nil {
			return cborValue{}, err
		}
		end := d.at + length
		content := d.data[d.at:end]
		d.at = end
		if major == 2 {
			return cborValue{kind: cborBytes, bytes: content}, nil
		}
		if !utf8.Valid(content) {
			return cborValue{}, refuse("cbor.text_not_utf8")
		}
		return cborValue{kind: cborText, bytes: content}, nil
	case 4:
		count, err := d.length(argument, 1)
		if err != nil {
			return cborValue{}, err
		}
		items := make([]cborValue, 0, count)
		for range count {
			item, err := d.item(depth + 1)
			if err != nil {
				return cborValue{}, err
			}
			items = append(items, item)
		}
		return cborValue{kind: cborArray, array: items}, nil
	default: // 5
		count, err := d.length(argument, 2)
		if err != nil {
			return cborValue{}, err
		}
		pairs := make([]cborPair, 0, count)
		for range count {
			key, err := d.item(depth + 1)
			if err != nil {
				return cborValue{}, err
			}
			if key.kind != cborUnsigned && key.kind != cborNegative && key.kind != cborText {
				return cborValue{}, refuse("cbor.map_key_kind")
			}
			for _, earlier := range pairs {
				if sameKey(earlier.key, key) {
					return cborValue{}, refuse("cbor.duplicate_key")
				}
			}
			value, err := d.item(depth + 1)
			if err != nil {
				return cborValue{}, err
			}
			pairs = append(pairs, cborPair{key: key, value: value})
		}
		return cborValue{kind: cborMap, pairs: pairs}, nil
	}
}

// length turns a string's length or a container's count into an int, refusing one the remaining
// bytes cannot hold: every item takes at least a byte, a pair two, so a larger count is a lie told to
// make the decoder allocate.
func (d *cborDecoder) length(argument uint64, bytesEach int) (int, error) {
	if argument > maxCBORSize {
		return 0, refuse("cbor.truncated")
	}
	n := int(argument)
	if n*bytesEach > d.remaining() {
		return 0, refuse("cbor.truncated")
	}
	return n, nil
}

// argument reads the additional information's value. 28 to 30 are reserved and 31 is an
// indefinite length; both are refused.
func (d *cborDecoder) argument(info byte) (uint64, error) {
	var width int
	switch {
	case info < 24:
		return uint64(info), nil
	case info == 24:
		width = 1
	case info == 25:
		width = 2
	case info == 26:
		width = 4
	case info == 27:
		width = 8
	case info == 31:
		return 0, refuse("cbor.indefinite_length")
	default:
		return 0, refuse("cbor.reserved")
	}
	if d.remaining() < width {
		return 0, refuse("cbor.truncated")
	}
	raw := d.data[d.at : d.at+width]
	d.at += width
	switch width {
	case 1:
		return uint64(raw[0]), nil
	case 2:
		return uint64(binary.BigEndian.Uint16(raw)), nil
	case 4:
		return uint64(binary.BigEndian.Uint32(raw)), nil
	default:
		return binary.BigEndian.Uint64(raw), nil
	}
}

// simple reads major type 7: false, true and null are what extensions use; a float or any other
// simple value has no place in a ceremony.
func (d *cborDecoder) simple(info byte) (cborValue, error) {
	switch info {
	case 20:
		return cborValue{kind: cborBool, boolean: false}, nil
	case 21:
		return cborValue{kind: cborBool, boolean: true}, nil
	case 22:
		return cborValue{kind: cborNull}, nil
	case 31:
		return cborValue{}, refuse("cbor.indefinite_length")
	default:
		return cborValue{}, refuse("cbor.simple_value")
	}
}

func sameKey(a, b cborValue) bool {
	if a.kind != b.kind {
		return false
	}
	if a.kind == cborText {
		return string(a.bytes) == string(b.bytes)
	}
	return a.n == b.n
}

// int answers an integer item as an int64, where it is one and fits.
func (v cborValue) int() (int64, bool) {
	switch v.kind {
	case cborUnsigned:
		if v.n > math.MaxInt64 {
			return 0, false
		}
		return int64(v.n), true
	case cborNegative:
		if v.n > math.MaxInt64 {
			return 0, false
		}
		return -1 - int64(v.n), true
	default:
		return 0, false
	}
}

// lookup answers the value under an integer key of a map.
func (v cborValue) lookup(key int64) (cborValue, bool) {
	for _, pair := range v.pairs {
		if k, ok := pair.key.int(); ok && k == key {
			return pair.value, true
		}
	}
	return cborValue{}, false
}

// field answers the value under a text key of a map.
func (v cborValue) field(key string) (cborValue, bool) {
	for _, pair := range v.pairs {
		if pair.key.kind == cborText && string(pair.key.bytes) == key {
			return pair.value, true
		}
	}
	return cborValue{}, false
}
