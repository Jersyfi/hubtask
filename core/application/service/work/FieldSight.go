// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

package work

import (
	"context"
	"maps"
	"slices"
	"strings"

	repository "github.com/Jersyfi/hubtask/core/application/repository/work"
	"github.com/Jersyfi/hubtask/core/application/service/access"
	appshared "github.com/Jersyfi/hubtask/core/application/shared"
	"github.com/Jersyfi/hubtask/core/domain/model/identity"
	"github.com/Jersyfi/hubtask/core/domain/model/shared"
	domain "github.com/Jersyfi/hubtask/core/domain/model/work"
	"github.com/Jersyfi/hubtask/core/domain/service"
	"github.com/Jersyfi/hubtask/core/port/expression"
	"github.com/Jersyfi/hubtask/core/port/persistence"
)

// FieldReadRestrictedCode refuses a query that filters, sorts or groups by a field hidden from
// the reader: an order or a range over a value is the value told another way.
const FieldReadRestrictedCode = "fields.read_restricted"

// FieldSight takes from every answer the fields hidden from its reader (UC-ID-27 check 3): a
// definition restricted for reading whose FIELD_READ:<key> and FIELD_WRITE:<key> the reader holds
// along neither of the entry's paths. Asked by every read - the entry, a list, a query and so an
// export, a search, the change stream - and by every write's answer, from this one place.
//
// A calculated field whose formula reads a hidden field is hidden with it: its value is the hidden
// one told another way. A build without the compiler hides every calculated field beside a hidden
// one rather than guess which formula reads it.
//
// A build without it narrows nothing, which is every workspace without a hidden field anyway.
type FieldSight struct {
	Hidings    repository.FieldHidings
	Containers repository.Containers
	Roles      FormRoles
	Compiler   expression.Compiler
}

// Wired reports whether the sight has what it needs.
func (s FieldSight) Wired() bool { return s.Hidings != nil && s.Containers != nil && s.Roles != nil }

// FieldLens is what one reader may not see, worked out collection by collection as an answer
// meets them: one read of the hidden definitions, one held-along question per collection.
type FieldLens struct {
	sight   FieldSight
	actor   appshared.ActorContext
	hidings  []repository.FieldHiding
	formulas []repository.FieldFormula
	hidden   map[shared.ID][]string
}

// Lens starts the narrowing of one answer for its reader, in a read-only transaction of its own.
func (s FieldSight) Lens(
	ctx context.Context, uow persistence.UnitOfWork, actor appshared.ActorContext,
) (*FieldLens, error) {
	lens := &FieldLens{sight: s, actor: actor, hidden: map[shared.ID][]string{}}
	if !s.Wired() || uow == nil {
		return lens, nil
	}
	err := uow.WithinReadOnly(ctx, actor.PersistenceScope(), func(ctx context.Context) error {
		var err error
		if lens.hidings, err = s.Hidings.ReadRestricted(ctx); err != nil || len(lens.hidings) == 0 {
			return err
		}
		lens.formulas, err = s.Hidings.ReadFormulas(ctx)
		return err
	})
	if err != nil {
		return nil, err
	}
	return lens, nil
}

// Empty reports whether the workspace hides no field at all: most do not, and pay one statement.
func (l *FieldLens) Empty() bool { return l == nil || len(l.hidings) == 0 }

// HiddenIn answers the keys hidden from the reader in a collection.
func (l *FieldLens) HiddenIn(ctx context.Context, collection domain.Container) ([]string, error) {
	if l.Empty() {
		return nil, nil
	}
	if keys, decided := l.hidden[collection.ID]; decided {
		return keys, nil
	}
	candidates := []string{}
	for _, hiding := range l.hidings {
		if (hiding.CollectionID.IsZero() || hiding.CollectionID == collection.ID) &&
			!slices.Contains(candidates, hiding.Key) {
			candidates = append(candidates, hiding.Key)
		}
	}
	keys := []string{}
	if len(candidates) > 0 {
		held, err := l.sight.Roles.HeldAlong(ctx, l.actor, containerPath(collection))
		if err != nil {
			return nil, err
		}
		for _, key := range candidates {
			if !service.SeesField(carriedBy(held), key) {
				keys = append(keys, key)
			}
		}
	}
	keys = append(keys, l.calculatedReading(l.formulasIn(collection.ID), keys)...)
	l.hidden[collection.ID] = keys
	return keys, nil
}

// hidesAnyIn reports whether any hidden definition is in force in the collection.
func (l *FieldLens) hidesAnyIn(collectionID shared.ID) bool {
	return slices.ContainsFunc(l.hidings, func(h repository.FieldHiding) bool {
		return h.CollectionID.IsZero() || h.CollectionID == collectionID
	})
}

// HiddenFrom answers the keys hidden from the reader somewhere along a path that a query ranges
// over: a key hidden in any collection under it counts, because a sort or a range over the whole
// answer orders the entries of that collection too.
func (l *FieldLens) HiddenFrom(ctx context.Context, path []identity.Scope, keys []string) ([]string, error) {
	if l.Empty() || len(keys) == 0 {
		return nil, nil
	}
	held, err := l.sight.Roles.HeldAlong(ctx, l.actor, path)
	if err != nil {
		return nil, err
	}
	// Every hidden key, named or not: a calculated field the query names may read one it does not.
	unseen := []string{}
	for _, hiding := range l.hidings {
		if !slices.Contains(unseen, hiding.Key) && !service.SeesField(carriedBy(held), hiding.Key) {
			unseen = append(unseen, hiding.Key)
		}
	}
	hidden := []string{}
	for _, key := range keys {
		if slices.Contains(unseen, key) && !slices.Contains(hidden, key) {
			hidden = append(hidden, key)
		}
	}
	for _, key := range l.calculatedReading(l.formulas, unseen) {
		if slices.Contains(keys, key) && !slices.Contains(hidden, key) {
			hidden = append(hidden, key)
		}
	}
	return hidden, nil
}

// formulasIn are the formula definitions in force in a collection: the workspace-wide ones, and
// the collection's own in their place where it defines the key again.
func (l *FieldLens) formulasIn(collectionID shared.ID) []repository.FieldFormula {
	byKey := map[string]int{}
	var effective []repository.FieldFormula
	for _, formula := range l.formulas {
		if !formula.CollectionID.IsZero() && formula.CollectionID != collectionID {
			continue
		}
		if at, seen := byKey[formula.Key]; seen {
			if !formula.CollectionID.IsZero() {
				effective[at] = formula
			}
			continue
		}
		byKey[formula.Key] = len(effective)
		effective = append(effective, formula)
	}
	return effective
}

// calculatedReading answers the CALCULATED keys among the definitions whose formula reads one of
// the hidden keys: held to the inputs left visible, such a formula no longer compiles.
func (l *FieldLens) calculatedReading(definitions []repository.FieldFormula, hidden []string) []string {
	if len(hidden) == 0 {
		return nil
	}
	var visible []string
	for _, definition := range definitions {
		if definition.Kind.IsFormulaInput() && !slices.Contains(hidden, definition.Key) && !slices.Contains(visible, definition.Key) {
			visible = append(visible, definition.Key)
		}
	}
	environment := FormulaEnvironment(visible)
	var reading []string
	for _, definition := range definitions {
		if definition.Kind != domain.CustomFieldCalculated || slices.Contains(reading, definition.Key) {
			continue
		}
		if l.sight.Compiler != nil {
			if _, err := l.sight.Compiler.Compile(definition.Formula, environment, expression.Number); err == nil {
				continue
			}
		}
		reading = append(reading, definition.Key)
	}
	return reading
}

// refuseHidden is the refusal of a query over a hidden field, naming the first.
func refuseHidden(hidden []string) error {
	if len(hidden) == 0 {
		return nil
	}
	return shared.ErrForbidden.WithDetail(FieldReadRestrictedCode).
		WithParams(map[string]string{"key": hidden[0]}).
		WithFields(shared.FieldError{
			Path: "/custom_fields/" + hidden[0], Code: FieldReadRestrictedCode,
			Params: map[string]string{"key": hidden[0]},
		})
}

// Items narrows a page of entries, reading the collections it has not met yet.
func (l *FieldLens) Items(
	ctx context.Context, uow persistence.UnitOfWork, items []domain.WorkItem,
) ([]domain.WorkItem, error) {
	if l.Empty() || len(items) == 0 {
		return items, nil
	}
	collections := map[shared.ID]domain.Container{}
	err := uow.WithinReadOnly(ctx, l.actor.PersistenceScope(), func(ctx context.Context) error {
		for _, item := range items {
			if _, read := collections[item.CollectionID]; read {
				continue
			}
			if _, decided := l.hidden[item.CollectionID]; decided {
				continue
			}
			if !l.hidesAnyIn(item.CollectionID) {
				l.hidden[item.CollectionID] = []string{}
				continue
			}
			collection, err := l.sight.Containers.Find(ctx, item.CollectionID)
			if err != nil {
				return err
			}
			collections[item.CollectionID] = collection
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	out := make([]domain.WorkItem, len(items))
	for i, item := range items {
		keys, decided := l.hidden[item.CollectionID]
		if !decided {
			if keys, err = l.HiddenIn(ctx, collections[item.CollectionID]); err != nil {
				return nil, err
			}
		}
		out[i] = WithoutFields(item, keys)
	}
	return out, nil
}

// Item narrows one entry.
func (l *FieldLens) Item(ctx context.Context, uow persistence.UnitOfWork, item domain.WorkItem) (domain.WorkItem, error) {
	narrowed, err := l.Items(ctx, uow, []domain.WorkItem{item})
	if err != nil {
		return domain.WorkItem{}, err
	}
	return narrowed[0], nil
}

// WithoutFields is the entry with the keys taken out of its values, the stored map untouched.
func WithoutFields(item domain.WorkItem, keys []string) domain.WorkItem {
	if len(keys) == 0 || len(item.CustomFields) == 0 {
		return item
	}
	values := maps.Clone(item.CustomFields)
	for _, key := range keys {
		delete(values, key)
	}
	item.CustomFields = values
	return item
}

// PayloadWithout is a change log payload with the keys taken out: under `custom_fields` as a
// snapshot carries them, and as `custom_fields.<key>` as a change of one key does.
func PayloadWithout(payload map[string]any, keys []string) map[string]any {
	if len(keys) == 0 || payload == nil {
		return payload
	}
	out := maps.Clone(payload)
	for field := range payload {
		if key, one := strings.CutPrefix(field, domain.FieldCustomFields+"."); one && slices.Contains(keys, key) {
			delete(out, field)
		}
	}
	if values, carried := payload[domain.FieldCustomFields].(map[string]any); carried {
		values = maps.Clone(values)
		for _, key := range keys {
			delete(values, key)
		}
		out[domain.FieldCustomFields] = values
	}
	return out
}

// carriedBy is whether what is held carries a permission, as the decision reads it: the
// permissions over the place, or a role that names a field's.
func carriedBy(held access.Held) func(service.Permission) bool {
	return func(permission service.Permission) bool {
		return slices.Contains(held.Permissions, permission) ||
			slices.ContainsFunc(held.Sources, func(s access.Source) bool { return s.Carries(permission) })
	}
}
