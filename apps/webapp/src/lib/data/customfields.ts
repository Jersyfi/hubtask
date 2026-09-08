// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the client decides about the fields an installation added, pure so each decision is tested.
 *
 * **The key and the kind are fixed once defined.** The contract says why in its own words — "a key
 * that moved would orphan every value stored under it", "a kind that changed would reinterpret
 * them" — and `CustomFieldDefinitionUpdate` does not carry either. So both are rendered read-only
 * with that reason rather than left out: a control that is absent tells the reader nothing about
 * why it is absent.
 *
 * **A value is written one key per call.** `offline-sync.md` §4.2 makes the merge rule per key, so
 * two devices setting two different keys converge to both — and a form that PUT the whole
 * `custom_fields` document would erase every key it had not loaded.
 *
 * **The comparisons a key takes are the server's, narrowed only where one cannot mean anything.**
 * `core/domain/model/view/Field.go` offers the same six for every kind —
 * `EQ, NEQ, IN, NOT_IN, IS_NULL, CONTAINS` — and says why the ordering comparisons are not among
 * them: "`LT` over a jsonb value would compare a NUMBER as text on one entry and as a number on
 * the next". This narrows that set per kind and never widens it, so nothing offered here can be
 * refused by name; `queryFieldsFor` carries the table.
 */

import type { CustomFieldDefinition } from '@hubtask/sync-engine';

import type { FilterField } from './query.ts';

/** The key's shape, as the contract's pattern writes it. */
const KEY = /^[a-z][a-z0-9_]{0,49}$/;

export function isValidKey(key: string): boolean {
  return KEY.test(key);
}

/** A definition that belongs to no collection applies to the whole workspace. */
export function isWorkspaceWide(definition: CustomFieldDefinition): boolean {
  return definition.collection_id === null || definition.collection_id === undefined;
}

/** Whether a type carries the field. Never empty on the wire — "a definition no type carries". */
export function appliesTo(definition: CustomFieldDefinition, type: string): boolean {
  return ((definition.applies_to ?? []) as readonly string[]).includes(type);
}

/** The definitions an entry of this type carries, in the order the server listed them. */
export function definitionsFor(
  definitions: readonly CustomFieldDefinition[],
  type: string,
): readonly CustomFieldDefinition[] {
  return definitions.filter((definition) => appliesTo(definition, type));
}

/** The kinds whose options are a closed set. Every other kind carries none. */
export function takesOptions(kind: string): boolean {
  return kind === 'SELECT' || kind === 'MULTI_SELECT';
}

/**
 * A value in the shape the kind asks for, or `null` to clear the key.
 *
 * The renderer hands back what its control produced — a string from an input, a boolean from a
 * checkbox, an array from a multi-select — and the contract pins what each kind travels as. The
 * one conversion that matters is `NUMBER`: a number arriving as a string is `validation_failed`
 * with the field path, never a stored value.
 *
 * An empty string is `null` and not `""`. "Not set" and "set to nothing" are different states, and
 * only the first one is what clearing a field means.
 */
export function valueFor(kind: string, raw: string | number | boolean | readonly string[] | null): unknown {
  if (raw === null || raw === undefined) return null;

  if (kind === 'MULTI_SELECT') {
    const list = Array.isArray(raw) ? raw : [];
    return list.length === 0 ? null : list;
  }
  if (Array.isArray(raw)) return raw.length === 0 ? null : raw;

  if (kind === 'BOOL') return typeof raw === 'boolean' ? raw : String(raw) === 'true';

  if (kind === 'NUMBER') {
    if (typeof raw === 'number') return Number.isFinite(raw) ? raw : null;
    const text = String(raw).trim();
    if (text === '') return null;
    const parsed = Number(text);
    // A number this client cannot read is sent as it was typed rather than as NaN: the server's
    // `validation_failed` names the field, and a silent null would clear a value nobody asked to
    // clear.
    return Number.isFinite(parsed) ? parsed : text;
  }

  const text = String(raw).trim();
  return text === '' ? null : text;
}

/**
 * The comparisons this client offers for a kind, and why each set is what it is.
 *
 * The server's set is the same six for every kind, because the kind is not known when a filter is
 * validated — it is the definition's, which is data. Everything below is therefore a **subset**,
 * never an addition:
 *
 * | Kind | Offered | Left out, and why |
 * |---|---|---|
 * | `TEXT`, `URL` | all six | — |
 * | `SELECT` | `EQ NEQ IN NOT_IN IS_NULL` | `CONTAINS` — a substring of a closed set is a chooser used wrongly |
 * | `MULTI_SELECT` | `CONTAINS IS_NULL` | the four equalities — they compare whole arrays, which is not what "has this option" means |
 * | `NUMBER` | `EQ NEQ IN NOT_IN IS_NULL` | `CONTAINS` — a substring of a number |
 * | `BOOL` | `EQ NEQ IS_NULL` | `IN`/`NOT_IN` over two values is `EQ`/`NEQ`; `CONTAINS` is meaningless |
 * | `DATE` | `EQ NEQ IN NOT_IN IS_NULL` | `CONTAINS`; the ordering comparisons are the **server's** omission, not this one |
 * | anything else | `EQ NEQ IS_NULL` | a kind this client has never met gets what is true of any value |
 *
 * The last row is the rule `domain-model.md` §2 asks for: a newer server's kind is filterable
 * rather than invisible, on the comparisons that mean the same thing whatever it turns out to be.
 */
export function operatorsFor(kind: string): readonly string[] {
  switch (kind) {
    case 'TEXT':
    case 'URL':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'CONTAINS'];
    case 'SELECT':
    case 'NUMBER':
    case 'DATE':
    case 'USER':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL'];
    case 'MULTI_SELECT':
      return ['CONTAINS', 'IS_NULL'];
    case 'BOOL':
      return ['EQ', 'NEQ', 'IS_NULL'];
    default:
      return ['EQ', 'NEQ', 'IS_NULL'];
  }
}

/**
 * The value shape the filter document sends for a kind.
 *
 * `number` and `boolean` are the two that are not text on the wire. The names are the query
 * grammar's own (`core/domain/model/view/Field.go`), and `number` exists there for exactly this
 * case: "the shape a *value* takes when it is a JSON number and no column pins its type: a custom
 * field's".
 */
function filterKindOf(kind: string): string {
  if (kind === 'NUMBER') return 'number';
  if (kind === 'BOOL') return 'boolean';
  if (kind === 'SELECT' || kind === 'MULTI_SELECT') return 'enum';
  if (kind === 'USER') return 'id';
  return 'string';
}

/** Where a custom field's name begins. The grammar's own prefix, written once. */
export const FILTER_PREFIX = 'custom_fields.';

/**
 * The definitions as fields a filter editor can offer.
 *
 * They are **not** in `query_fields` and that is deliberate on the server's side (C-07: "which
 * keys exist is `/custom-fields`' answer"), so they are composed here from the definitions in
 * force for the collection on screen rather than read from the manifest.
 *
 * `nullable` is true for every one of them, which is what the grammar says: a key an entry never
 * had is absent, so `IS_NULL` always means something.
 */
export function queryFieldsFor(
  definitions: readonly CustomFieldDefinition[],
): readonly FilterField[] {
  return definitions.map((definition) => ({
    field: FILTER_PREFIX + definition.key,
    kind: filterKindOf(definition.kind),
    operators: [...operatorsFor(definition.kind)],
    values: takesOptions(definition.kind) ? [...(definition.options ?? [])] : undefined,
    nullable: true,
    // Neither, and the server agrees: `custom_fields.<key>` is filterable and is in no sort or
    // group list. Offering an order over a jsonb value is the same mistake `LT` would be.
    sortable: false,
    groupable: false,
  }));
}
