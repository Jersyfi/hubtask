// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package setting

import "github.com/Jersyfi/hubtask/core/domain/model/shared"

// Catalogue is every declared setting, in the order a screen draws them: immutable once built,
// for the reason the use-case registry is - a catalogue that grows at run time is one no parity
// check can speak for.
type Catalogue struct {
	definitions map[Key]Definition
	order       []Key
}

// NewCatalogue builds the catalogue and refuses an invalid or duplicate declaration.
func NewCatalogue(definitions ...Definition) (Catalogue, error) {
	catalogue := Catalogue{definitions: make(map[Key]Definition, len(definitions))}
	for _, definition := range definitions {
		if err := definition.Validate(); err != nil {
			return Catalogue{}, err
		}
		if _, duplicate := catalogue.definitions[definition.Key]; duplicate {
			return Catalogue{}, shared.ErrInternal.WithDetail("setting.definition_invalid").
				WithParams(map[string]string{"setting": string(definition.Key), "reason": "duplicate"})
		}
		catalogue.definitions[definition.Key] = definition
		catalogue.order = append(catalogue.order, definition.Key)
	}
	return catalogue, nil
}

// Lookup answers one definition.
func (c Catalogue) Lookup(key Key) (Definition, bool) {
	definition, held := c.definitions[key]
	return definition, held
}

// Definitions is every definition, in the declared order.
func (c Catalogue) Definitions() []Definition {
	out := make([]Definition, 0, len(c.order))
	for _, key := range c.order {
		out = append(out, c.definitions[key])
	}
	return out
}

// At is every definition a level may decide, in the declared order: the controls of that level's
// settings screen.
func (c Catalogue) At(level Level) []Definition {
	var out []Definition
	for _, key := range c.order {
		if definition := c.definitions[key]; definition.DecidedAt(level) {
			out = append(out, definition)
		}
	}
	return out
}

// ErrUnknown refuses a key the catalogue does not declare.
func ErrUnknown(key Key) error {
	return shared.ErrNotFound.WithDetail("setting.unknown").
		WithParams(map[string]string{"setting": string(key)})
}

// ErrLocked refuses a write under a lock, naming where it came from.
func ErrLocked(key Key, by Level) error {
	return shared.ErrValidation.WithDetail("setting.locked").
		WithParams(map[string]string{"setting": string(key), "origin": string(by)}).
		WithFields(shared.FieldError{Path: "/" + string(key), Code: "setting.locked"})
}

// ErrNotAllowed refuses a value outside what the levels above leave.
func ErrNotAllowed(key Key) error {
	return shared.ErrValidation.WithDetail("setting.not_allowed").
		WithParams(map[string]string{"setting": string(key)}).
		WithFields(shared.FieldError{Path: "/" + string(key), Code: "setting.not_allowed"})
}

// Check is the writer's question: may this level store this entry, under what the levels above
// resolve to? A lock above refuses; a value outside a restriction or, for MayTighten, looser than
// the value in force refuses; a hold the definition does not declare refuses.
func Check(definition Definition, level Level, above Resolution, entry Entry) error {
	if !definition.DecidedAt(level) {
		return ErrUnknown(definition.Key)
	}
	if above.Locked() {
		return ErrLocked(definition.Key, above.LockedBy)
	}
	if (entry.Hold == HoldLock && !definition.Lockable) ||
		(entry.Hold == HoldRestrict && !definition.Restrictable) || !entry.Hold.Valid() {
		return ErrNotAllowed(definition.Key)
	}
	if entry.Value != nil && !admits(definition, level, above, entry.Value) {
		return ErrNotAllowed(definition.Key)
	}
	return nil
}
