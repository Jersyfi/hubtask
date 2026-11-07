// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package realtime

import (
	"encoding/json"
	"errors"
	"slices"
	"unicode/utf8"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// CheckCovers refuses a snapshot that lacks content or a deletion of the log up to its place:
// stored, it would hide that from every later opener, who reads the snapshot and only what follows.
func CheckCovers(state []byte, log Coverage) error {
	held, err := ReadCoverage(state)
	if err != nil {
		return shared.ErrValidation.WithDetail("realtime.snapshot_unreadable")
	}
	if !held.Covers(log) {
		return shared.ErrValidation.WithDetail("realtime.snapshot_incomplete")
	}
	return nil
}

// Coverage is what a CRDT update names, read from its headers only: per editor (a client id) the
// clock ranges of the content it carries and the ranges it marks deleted. The content itself -
// the words, their place in the document - is never interpreted, only skipped over.
//
// It answers whether a snapshot holds everything the log before it held (offline-sync.md §13): a
// snapshot that lacks an update, or a deletion, would hide it from every later opener. The format
// is the editors' update encoding, version one (the structs, then the delete set); a reader of
// our own, since a CRDT library on the server is ruled out (offline-sync.md SY-F).
type Coverage struct {
	structs map[uint64][]span
	deleted map[uint64][]span
}

// span is the clocks [from, to) of one editor.
type span struct{ from, to uint64 }

// The bounds a reader keeps against a hostile update: a nesting the editors' own reader would
// overflow on, and a clock past the integers they count in.
const (
	maxAnyDepth = 1000
	maxClock    = 1<<53 - 1
)

var errUnreadable = errors.New("realtime: update unreadable")

// ReadCoverage reads an update's coverage. An update the editors could not read is refused with
// realtime.update_unreadable: stored, it would break every opener of the document.
func ReadCoverage(data []byte) (Coverage, error) {
	r := updateReader{data: data}
	c := Coverage{structs: map[uint64][]span{}, deleted: map[uint64][]span{}}
	if err := r.structs(c); err != nil {
		return Coverage{}, shared.ErrValidation.WithDetail("realtime.update_unreadable")
	}
	if err := r.deleteSet(c); err != nil {
		return Coverage{}, shared.ErrValidation.WithDetail("realtime.update_unreadable")
	}
	c.normalize()
	return c, nil
}

// Add takes another coverage into this one: the log read update by update.
func (c *Coverage) Add(other Coverage) {
	if c.structs == nil {
		c.structs, c.deleted = map[uint64][]span{}, map[uint64][]span{}
	}
	for client, spans := range other.structs {
		c.structs[client] = merge(append(c.structs[client], spans...))
	}
	for client, spans := range other.deleted {
		c.deleted[client] = merge(append(c.deleted[client], spans...))
	}
}

// Covers reports whether this coverage holds every range of the other: a snapshot may hold more
// than the log up to its place (its writer's words not yet sent), never less.
func (c Coverage) Covers(other Coverage) bool {
	return within(c.structs, other.structs) && within(c.deleted, other.deleted)
}

func within(outer, inner map[uint64][]span) bool {
	for client, spans := range inner {
		held := outer[client]
		for _, s := range spans {
			i, _ := slices.BinarySearchFunc(held, s.from, func(h span, from uint64) int {
				switch {
				case h.to <= from:
					return -1
				case h.from > from:
					return 1
				}
				return 0
			})
			if i == len(held) || held[i].from > s.from || held[i].to < s.to {
				return false
			}
		}
	}
	return true
}

func (c Coverage) normalize() {
	for client, spans := range c.structs {
		c.structs[client] = merge(spans)
	}
	for client, spans := range c.deleted {
		c.deleted[client] = merge(spans)
	}
}

// merge sorts the spans and joins those that touch or overlap.
func merge(spans []span) []span {
	slices.SortFunc(spans, func(a, b span) int {
		switch {
		case a.from < b.from:
			return -1
		case a.from > b.from:
			return 1
		}
		return 0
	})
	out := spans[:0]
	for _, s := range spans {
		if s.to <= s.from {
			continue
		}
		if n := len(out); n > 0 && s.from <= out[n-1].to {
			out[n-1].to = max(out[n-1].to, s.to)
			continue
		}
		out = append(out, s)
	}
	return out
}

func add(m map[uint64][]span, client, from, length uint64) error {
	if length > maxClock || from > maxClock-length {
		return errUnreadable
	}
	m[client] = append(m[client], span{from, from + length})
	return nil
}

type updateReader struct {
	data []byte
	pos  int
}

func (r *updateReader) next() (byte, error) {
	if r.pos >= len(r.data) {
		return 0, errUnreadable
	}
	b := r.data[r.pos]
	r.pos++
	return b, nil
}

// varUint is an unsigned variable-length integer, seven bits a byte.
func (r *updateReader) varUint() (uint64, error) {
	var n uint64
	for shift := uint(0); ; shift += 7 {
		if shift > 49 {
			return 0, errUnreadable
		}
		b, err := r.next()
		if err != nil {
			return 0, err
		}
		n |= uint64(b&0x7f) << shift
		if n > maxClock {
			return 0, errUnreadable
		}
		if b < 0x80 {
			return n, nil
		}
	}
}

// varInt is a signed variable-length integer: a sign and six bits first, then seven a byte.
func (r *updateReader) varInt() error {
	b, err := r.next()
	if err != nil {
		return err
	}
	if b&0x80 == 0 {
		return nil
	}
	n, err := r.varUint()
	if err != nil {
		return err
	}
	if n > maxClock>>6 {
		return errUnreadable
	}
	return nil
}

func (r *updateReader) skip(n uint64) error {
	if n > uint64(len(r.data)-r.pos) { //nolint:gosec // pos never passes len
		return errUnreadable
	}
	r.pos += int(n) //nolint:gosec // bounded by the length just compared
	return nil
}

func (r *updateReader) bytes() ([]byte, error) {
	n, err := r.varUint()
	if err != nil {
		return nil, err
	}
	start := r.pos
	if err := r.skip(n); err != nil {
		return nil, err
	}
	return r.data[start:r.pos], nil
}

// varString answers its length as the editors count it: in UTF-16 code units. The editors never
// write invalid UTF-8, so a string that is not is an update they did not write.
func (r *updateReader) varString() (uint64, error) {
	raw, err := r.bytes()
	if err != nil {
		return 0, err
	}
	if !utf8.Valid(raw) {
		return 0, errUnreadable
	}
	var units uint64
	for _, c := range string(raw) {
		units++
		if c >= 0x10000 {
			units++
		}
	}
	return units, nil
}

func (r *updateReader) json() error {
	raw, err := r.bytes()
	if err != nil {
		return err
	}
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return errUnreadable
	}
	return nil
}

// any is one value of the editors' binary encoding of JSON-like data.
func (r *updateReader) any(depth int) error {
	if depth > maxAnyDepth {
		return errUnreadable
	}
	tag, err := r.next()
	if err != nil {
		return err
	}
	switch tag {
	case 127, 126, 121, 120: // undefined, null, false, true
		return nil
	case 125:
		return r.varInt()
	case 124:
		return r.skip(4)
	case 123, 122: // float64, bigint64
		return r.skip(8)
	case 119:
		_, err := r.varString()
		return err
	case 118:
		n, err := r.varUint()
		if err != nil {
			return err
		}
		for range n {
			if _, err := r.varString(); err != nil {
				return err
			}
			if err := r.any(depth + 1); err != nil {
				return err
			}
		}
		return nil
	case 117:
		n, err := r.varUint()
		if err != nil {
			return err
		}
		for range n {
			if err := r.any(depth + 1); err != nil {
				return err
			}
		}
		return nil
	case 116:
		_, err := r.bytes()
		return err
	}
	return errUnreadable
}

func (r *updateReader) structs(c Coverage) error {
	clients, err := r.varUint()
	if err != nil {
		return err
	}
	for range clients {
		count, err := r.varUint()
		if err != nil {
			return err
		}
		client, err := r.varUint()
		if err != nil {
			return err
		}
		clock, err := r.varUint()
		if err != nil {
			return err
		}
		for range count {
			info, err := r.next()
			if err != nil {
				return err
			}
			length, deleted, err := r.item(info)
			if err != nil {
				return err
			}
			if info&0x1f == 10 {
				// A skip: a gap the update does not fill, so nothing it carries.
				if clock > maxClock-length {
					return errUnreadable
				}
				clock += length
				continue
			}
			if err := add(c.structs, client, clock, length); err != nil {
				return err
			}
			if deleted {
				if err := add(c.deleted, client, clock, length); err != nil {
					return err
				}
			}
			clock += length
		}
	}
	return nil
}

// item reads one struct after its info byte and answers how many clocks it takes and whether it
// is content already deleted (a garbage-collected struct, or deleted content).
func (r *updateReader) item(info byte) (uint64, bool, error) {
	switch info & 0x1f {
	case 0: // garbage collected
		n, err := r.varUint()
		return n, true, err
	case 10: // skip
		n, err := r.varUint()
		return n, false, err
	}
	if info&0x80 != 0 { // origin
		if err := r.ids(1); err != nil {
			return 0, false, err
		}
	}
	if info&0x40 != 0 { // right origin
		if err := r.ids(1); err != nil {
			return 0, false, err
		}
	}
	if info&0xc0 == 0 {
		named, err := r.varUint()
		if err != nil {
			return 0, false, err
		}
		if named == 1 {
			if _, err := r.varString(); err != nil {
				return 0, false, err
			}
		} else if err := r.ids(1); err != nil {
			return 0, false, err
		}
		if info&0x20 != 0 { // a key in a map
			if _, err := r.varString(); err != nil {
				return 0, false, err
			}
		}
	}
	return r.content(info & 0x1f)
}

func (r *updateReader) ids(n int) error {
	for range 2 * n {
		if _, err := r.varUint(); err != nil {
			return err
		}
	}
	return nil
}

func (r *updateReader) content(kind byte) (uint64, bool, error) {
	switch kind {
	case 1: // deleted
		n, err := r.varUint()
		return n, true, err
	case 2: // JSON, each value a string
		n, err := r.varUint()
		if err != nil {
			return 0, false, err
		}
		for range n {
			raw, err := r.bytes()
			if err != nil {
				return 0, false, err
			}
			if string(raw) != "undefined" && (!utf8.Valid(raw) || !json.Valid(raw)) {
				return 0, false, errUnreadable
			}
		}
		return n, false, nil
	case 3: // binary
		_, err := r.bytes()
		return 1, false, err
	case 4: // string
		n, err := r.varString()
		return n, false, err
	case 5: // embed
		return 1, false, r.json()
	case 6: // format: a key and a JSON value
		if _, err := r.varString(); err != nil {
			return 0, false, err
		}
		return 1, false, r.json()
	case 7: // a type
		ref, err := r.varUint()
		if err != nil {
			return 0, false, err
		}
		switch ref {
		case 3, 5: // an XML element or hook, with its name
			_, err = r.varString()
		case 0, 1, 2, 4, 6:
		default:
			err = errUnreadable
		}
		return 1, false, err
	case 8: // values in the binary encoding
		n, err := r.varUint()
		if err != nil {
			return 0, false, err
		}
		for range n {
			if err := r.any(0); err != nil {
				return 0, false, err
			}
		}
		return n, false, nil
	case 9: // a sub-document: its id and options
		if _, err := r.varString(); err != nil {
			return 0, false, err
		}
		return 1, false, r.any(0)
	}
	return 0, false, errUnreadable
}

func (r *updateReader) deleteSet(c Coverage) error {
	clients, err := r.varUint()
	if err != nil {
		return err
	}
	for range clients {
		client, err := r.varUint()
		if err != nil {
			return err
		}
		count, err := r.varUint()
		if err != nil {
			return err
		}
		for range count {
			clock, err := r.varUint()
			if err != nil {
				return err
			}
			length, err := r.varUint()
			if err != nil {
				return err
			}
			if err := add(c.deleted, client, clock, length); err != nil {
				return err
			}
		}
	}
	return nil
}
