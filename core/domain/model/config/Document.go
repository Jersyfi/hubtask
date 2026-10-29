// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Package config is the configuration document (F-6, D-15): a workspace's settings and building
// blocks named by stable keys rather than identifiers, the difference between two documents as
// the changes a plan shows, the canonical form a plan's hash is taken over, and the policy that
// keeps an agent's plan waiting for a person (D-16).
//
// A document holds configuration only - never an entry, a comment or a secret - so that one
// document applies to a second workspace unchanged.
package config

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Jersyfi/hubtask/core/domain/model/shared"
)

// Block is one building block or setting: its attributes by name. Values are the normalised
// shapes a document is read in - string, float64, bool, nil, []any, map[string]any.
type Block map[string]any

// Section is one family of blocks by stable key: `types`, `workflows`, `modules`.
type Section map[string]Block

// Document is a configuration by section. A nil block in a section, in a document a person or an
// agent writes, removes the block it names.
type Document map[string]Section

// Op is what a change does to a block.
type Op string

const (
	OpAdd    Op = "ADD"
	OpChange Op = "CHANGE"
	OpRemove Op = "REMOVE"
)

// Mode is how a desired document is read against the one in force. MERGE names only what it
// changes: an absent block or attribute is kept, a nil block removes. REPLACE states each section
// it names whole: a block absent from a named section is removed. A rollback is REPLACE; a
// template and a hand-written change are MERGE.
type Mode string

const (
	ModeMerge   Mode = "MERGE"
	ModeReplace Mode = "REPLACE"
)

// BlockChange is one block's difference: its attributes before and after, and the names that differ.
type BlockChange struct {
	Section string
	Key     string
	Op      Op
	Before  Block
	After   Block
	Fields  []string
}

// Path is the change's stable key path, `types.incident`.
func (c BlockChange) Path() string { return c.Section + "." + c.Key }

// ValidKey reports whether a block key is one the document can name: lower case, digits and
// underscores, starting with a letter, at most 63.
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

// Resolve is the document a desired one asks for, read in its mode against the one in force:
// what a plan compares the current document with.
func Resolve(current, desired Document, mode Mode) Document {
	out := Document{}
	for name, section := range current {
		out[name] = cloneSection(section)
	}
	for name, section := range desired {
		target := out[name]
		if target == nil || mode == ModeReplace {
			target = Section{}
		}
		for key, block := range section {
			if block == nil {
				delete(target, key)
				continue
			}
			merged := Block{}
			if existing, held := current[name][key]; held && mode == ModeMerge {
				for attribute, value := range existing {
					merged[attribute] = value
				}
			}
			for attribute, value := range block {
				if value == nil {
					delete(merged, attribute)
					continue
				}
				merged[attribute] = value
			}
			target[key] = merged
		}
		out[name] = target
	}
	return out
}

// Diff is every change from one document to another, by section and key in order.
func Diff(from, to Document) []BlockChange {
	names := sortedKeys(from, to)
	var changes []BlockChange
	for _, name := range names {
		before, after := from[name], to[name]
		for _, key := range sortedKeys(before, after) {
			old, had := before[key]
			now, has := after[key]
			switch {
			case had && !has:
				changes = append(changes, BlockChange{Section: name, Key: key, Op: OpRemove, Before: old})
			case !had && has:
				changes = append(changes, BlockChange{Section: name, Key: key, Op: OpAdd, After: now, Fields: attributeNames(now)})
			default:
				if fields := differing(old, now); len(fields) > 0 {
					changes = append(changes, BlockChange{Section: name, Key: key, Op: OpChange, Before: old, After: now, Fields: fields})
				}
			}
		}
	}
	return changes
}

func differing(a, b Block) []string {
	var fields []string
	for _, name := range sortedKeys(a, b) {
		if Canonical(a[name]) != Canonical(b[name]) {
			fields = append(fields, name)
		}
	}
	return fields
}

func attributeNames(b Block) []string {
	out := make([]string, 0, len(b))
	for name := range b {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sortedKeys[V any](maps ...map[string]V) []string {
	seen := map[string]struct{}{}
	for _, m := range maps {
		for key := range m {
			seen[key] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func cloneSection(s Section) Section {
	out := make(Section, len(s))
	for key, block := range s {
		copied := make(Block, len(block))
		for attribute, value := range block {
			copied[attribute] = value
		}
		out[key] = copied
	}
	return out
}

// Canonical is a value's one spelling: maps by sorted key, numbers without a trailing fraction,
// so two documents that say the same thing hash the same whatever door wrote them.
func Canonical(v any) string {
	var b strings.Builder
	writeCanonical(&b, v)
	return b.String()
}

func writeCanonical(b *strings.Builder, v any) {
	switch value := v.(type) {
	case nil:
		b.WriteString("null")
	case bool:
		b.WriteString(strconv.FormatBool(value))
	case string:
		b.WriteString(strconv.Quote(value))
	case float64:
		if value == math.Trunc(value) && math.Abs(value) < 1e15 {
			b.WriteString(strconv.FormatInt(int64(value), 10))
		} else {
			b.WriteString(strconv.FormatFloat(value, 'g', -1, 64))
		}
	case int:
		b.WriteString(strconv.Itoa(value))
	case int64:
		b.WriteString(strconv.FormatInt(value, 10))
	case []any:
		b.WriteByte('[')
		for i, item := range value {
			if i > 0 {
				b.WriteByte(',')
			}
			writeCanonical(b, item)
		}
		b.WriteByte(']')
	case []string:
		items := make([]any, 0, len(value))
		for _, item := range value {
			items = append(items, item)
		}
		writeCanonical(b, items)
	case Block:
		writeCanonical(b, map[string]any(value))
	case Section:
		m := make(map[string]any, len(value))
		for key, block := range value {
			m[key] = block
		}
		writeCanonical(b, m)
	case Document:
		m := make(map[string]any, len(value))
		for key, section := range value {
			m[key] = section
		}
		writeCanonical(b, m)
	case map[string]any:
		keys := make([]string, 0, len(value))
		for key := range value {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		b.WriteByte('{')
		for i, key := range keys {
			if i > 0 {
				b.WriteByte(',')
			}
			b.WriteString(strconv.Quote(key))
			b.WriteByte(':')
			writeCanonical(b, value[key])
		}
		b.WriteByte('}')
	default:
		// A shape the normalised document never holds; spelled so that it still hashes stably.
		b.WriteString(strconv.Quote("?"))
	}
}

// Hash is a document's fingerprint: what an export answers as its ETag and what an apply holds
// the configuration in force to, so that a change through any other door since the plan refuses.
func Hash(d Document) string { return digest(Canonical(d)) }

// PlanHash names one plan: the fingerprint of the document it was planned against and of the
// document it leads to. Applying by it applies exactly what the person saw.
func PlanHash(base string, target Document) string {
	return digest(base + "\n" + Canonical(target))
}

func digest(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// Flags are what a plan must have acknowledged before it applies (config-document.md §3).
type Flags struct {
	HidesData          bool
	ChangesPermissions bool
}

// Any reports whether a flag is raised.
func (f Flags) Any() bool { return f.HidesData || f.ChangesPermissions }

// Refusal names what a plan refuses: the message code and the key path.
func Refusal(code, path string, params map[string]string) shared.FieldError {
	return shared.FieldError{Path: "/" + strings.ReplaceAll(path, ".", "/"), Code: code, Params: params}
}

// ErrRefused is a validation error carrying every refusal at once, so an agent corrects them all.
func ErrRefused(refusals []shared.FieldError) error {
	return shared.ErrValidation.WithDetail("config.refused").WithFields(refusals...)
}

// ErrVersionConflict refuses an apply against a configuration changed since its plan.
func ErrVersionConflict() error {
	return shared.ErrConflict.WithDetail("config.version_conflict")
}

// ErrPlanMismatch refuses an apply whose document does not lead to the plan named.
func ErrPlanMismatch() error {
	return shared.ErrValidation.WithDetail("config.plan_mismatch").
		WithFields(shared.FieldError{Path: "/plan_hash", Code: "config.plan_mismatch"})
}

// ErrNotAcknowledged refuses a flagged plan applied without its flags acknowledged.
func ErrNotAcknowledged() error {
	return shared.ErrValidation.WithDetail("config.acknowledge_required").
		WithFields(shared.FieldError{Path: "/acknowledge", Code: "config.acknowledge_required"})
}

// Sections answers a document's section names in order.
func (d Document) Sections() []string {
	names := make([]string, 0, len(d))
	for name := range d {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
