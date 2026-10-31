// SPDX-License-Identifier: Apache-2.0
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
 * `core/domain/model/view/Field.go` accepts the equalities, `CONTAINS`, `IS_NULL` and the ranges for
 * every key, and the use case holds the ranges, the sort and the grouping to the kind
 * (`CustomShape.go`): they read the typed value, so a number ranges as a number and a date as a
 * day. This narrows that set per kind and never widens it, so nothing offered here can be refused
 * by name; `queryFieldsFor` carries the table.
 */

import { calculate } from '@hubtask/expression';
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
  return kind === 'SELECT' || kind === 'MULTI_SELECT' || kind === 'CASCADING_SELECT' || kind === 'STATUS';
}

/**
 * The options a two-level choice offers, as the contract writes them: a parent, and a parent and a
 * child joined by `/`, each child under a parent the list holds. Answers the first option that
 * breaks the rule, so the dialog can name it before the server refuses it.
 */
export function cascadingOptionFault(options: readonly string[]): string | undefined {
  for (const option of options) {
    const parts = option.split('/');
    if (parts.length > 2 || parts.some((part) => part.trim() === '')) return option;
    if (parts.length === 2 && !options.includes(parts[0] as string)) return option;
  }
  return undefined;
}

/** The kind whose definition names a currency. Every other kind carries none. */
export function takesCurrency(kind: string): boolean {
  return kind === 'MONEY';
}

/** The kind the server computes with a formula: nobody fills it in, and it is never required. */
export function isCalculated(kind: string): boolean {
  return kind === 'CALCULATED';
}

/** The kinds a formula reads (`IsFormulaInput`): numbers, money, durations and booleans. */
const FORMULA_INPUTS = ['NUMBER', 'MONEY', 'DURATION', 'BOOL'];

/**
 * A calculated field's value as the entry's values now stand, computed in the browser
 * (`@hubtask/expression`) for an entry whose writes wait to be sent: the value the server last
 * computed predates them. The inputs are the scope's number, money, duration and boolean fields,
 * as the server's are. `null` where the formula cannot answer, and for a definition that is not
 * calculated.
 */
export function calculatedOf(
  definition: Pick<CustomFieldDefinition, 'kind' | 'formula'>,
  inScope: readonly Pick<CustomFieldDefinition, 'key' | 'kind'>[],
  values: Readonly<Record<string, unknown>>,
): number | null {
  if (!isCalculated(definition.kind as string) || !definition.formula) return null;
  const inputs: Record<string, unknown> = {};
  for (const input of inScope) {
    if (FORMULA_INPUTS.includes(input.kind as string)) inputs[input.key] = values[input.key];
  }
  return calculate(definition.formula, inputs);
}

/** A currency's shape, as the contract's pattern writes it: three capital letters (ISO 4217). */
export function isValidCurrency(code: string): boolean {
  return /^[A-Z]{3}$/.test(code);
}

/** The kinds a value of which is several entries of a list rather than one. */
function isListKind(kind: string): boolean {
  return kind === 'MULTI_SELECT' || kind === 'USER_SET' || kind === 'LABEL';
}

/** The kinds that travel as a JSON number. */
function isNumberKind(kind: string): boolean {
  return kind === 'NUMBER' || kind === 'MONEY' || kind === 'DURATION' || kind === 'CALCULATED';
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

  if (kind === 'LABEL') {
    // Labels are written in lower case and kept once each, as the server stores them.
    const words = (Array.isArray(raw) ? raw : String(raw).split(/[\s,]+/))
      .map((word) => String(word).trim().toLowerCase())
      .filter((word) => word !== '');
    const unique = [...new Set(words)];
    return unique.length === 0 ? null : unique;
  }
  if (isListKind(kind)) {
    const list = Array.isArray(raw) ? raw : [];
    return list.length === 0 ? null : list;
  }
  if (Array.isArray(raw)) return raw.length === 0 ? null : raw;

  if (kind === 'BOOL') return typeof raw === 'boolean' ? raw : String(raw) === 'true';

  if (isNumberKind(kind)) {
    if (typeof raw === 'number') return Number.isFinite(raw) ? raw : null;
    const text = String(raw).trim();
    if (text === '') return null;
    const parsed = Number(text);
    // A number this client cannot read is sent as it was typed rather than as NaN: the server's
    // `validation_failed` names the field, and a silent null would clear a value nobody asked to
    // clear.
    return Number.isFinite(parsed) ? parsed : text;
  }

  // Formatted text keeps its own line breaks and indentation: Markdown reads them.
  if (kind === 'RICH_TEXT') {
    const text = String(raw);
    return text.trim() === '' ? null : text;
  }

  const text = String(raw).trim();
  return text === '' ? null : text;
}

/**
 * The comparisons this client offers for a kind, and why each set is what it is.
 *
 * The server accepts the same set for every key and holds the ranges to the kind once it has read
 * the definitions (`core/domain/model/view/CustomShape.go`). Everything below is therefore a
 * **subset**, never an addition:
 *
 * | Kind | Offered | Left out, and why |
 * |---|---|---|
 * | `TEXT`, `URL` | the equalities, `IS_NULL`, `CONTAINS`, `STARTS_WITH` | the ranges — an order of words is the server's to give, not a filter anybody asks for |
 * | `SELECT`, `STATUS` | `EQ NEQ IN NOT_IN IS_NULL` | `CONTAINS` — a substring of a closed set is a chooser used wrongly |
 * | `CASCADING_SELECT` | `EQ NEQ IN NOT_IN IS_NULL STARTS_WITH` | `CONTAINS`; `STARTS_WITH` finds a parent with every child under it |
 * | `MULTI_SELECT`, `USER_SET`, `LABEL` | `CONTAINS CONTAINS_ANY CONTAINS_ALL IS_NULL` | the equalities — they compare whole arrays, which is not what "has this one" means |
 * | `NUMBER`, `MONEY`, `DURATION`, `DATE`, `DATE_TIME` | the equalities, `IS_NULL` and `LT LTE GT GTE BETWEEN` | `CONTAINS` — a substring of a number |
 * | `IP_ADDRESS` | those and `STARTS_WITH` | `CONTAINS`; an address ranges as an address, and a prefix finds a subnet written out |
 * | `USER`, `REFERENCE`, `GROUP`, `FILE` | `EQ NEQ IN NOT_IN IS_NULL` | the ranges — a person, an entry, a group or a file has no order |
 * | `BOOL` | `EQ NEQ IS_NULL` | `IN`/`NOT_IN` over two values is `EQ`/`NEQ`; `CONTAINS` is meaningless |
 * | `RICH_TEXT` | `CONTAINS IS_NULL` | everything that compares a whole value — formatted text is never compared |
 * | anything else | `EQ NEQ IS_NULL` | a kind this client has never met gets what is true of any value |
 *
 * The last row is the rule `domain-model.md` §2 asks for: a newer server's kind is filterable
 * rather than invisible, on the comparisons that mean the same thing whatever it turns out to be.
 */
export function operatorsFor(kind: string): readonly string[] {
  switch (kind) {
    case 'TEXT':
    case 'URL':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'CONTAINS', 'STARTS_WITH'];
    case 'NUMBER':
    case 'MONEY':
    case 'DURATION':
    case 'CALCULATED':
    case 'DATE':
    case 'DATE_TIME':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'LT', 'LTE', 'GT', 'GTE', 'BETWEEN'];
    case 'IP_ADDRESS':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'LT', 'LTE', 'GT', 'GTE', 'BETWEEN', 'STARTS_WITH'];
    case 'CASCADING_SELECT':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL', 'STARTS_WITH'];
    case 'SELECT':
    case 'STATUS':
    case 'USER':
    case 'REFERENCE':
    case 'GROUP':
    case 'FILE':
      return ['EQ', 'NEQ', 'IN', 'NOT_IN', 'IS_NULL'];
    case 'MULTI_SELECT':
    case 'USER_SET':
    case 'LABEL':
      return ['CONTAINS', 'CONTAINS_ANY', 'CONTAINS_ALL', 'IS_NULL'];
    case 'RICH_TEXT':
      return ['CONTAINS', 'IS_NULL'];
    case 'BOOL':
      return ['EQ', 'NEQ', 'IS_NULL'];
    default:
      return ['EQ', 'NEQ', 'IS_NULL'];
  }
}

/**
 * Whether entries can be put in order by a kind's one value: the server's own answer
 * (`CustomShape.sorts`). A set, a person, an entry and formatted text have no single order.
 */
export function sortsBy(kind: string): boolean {
  return [
    'TEXT', 'URL', 'SELECT', 'NUMBER', 'MONEY', 'DURATION', 'CALCULATED', 'DATE', 'BOOL',
    'DATE_TIME', 'IP_ADDRESS', 'CASCADING_SELECT', 'STATUS',
  ].includes(kind);
}

/**
 * Whether entries can be put in one column per value of a kind (`CustomShape.groups`). A set has
 * several values and formatted text none worth a column.
 */
export function groupsBy(kind: string): boolean {
  return sortsBy(kind) || kind === 'USER' || kind === 'REFERENCE' || kind === 'GROUP' || kind === 'FILE';
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
  if (isNumberKind(kind)) return 'number';
  if (kind === 'BOOL') return 'boolean';
  if (takesOptions(kind)) return 'enum';
  if (['USER', 'USER_SET', 'REFERENCE', 'GROUP', 'FILE'].includes(kind)) return 'id';
  return 'string';
}

/** Where a custom field's name begins. The grammar's own prefix, written once. */
export const FILTER_PREFIX = 'custom_fields.';

/**
 * The definitions as fields a filter editor can offer.
 *
 * They are **not** in `query_fields` and that is deliberate on the server's side ("which
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
    // By the kind, as the server decides it once it has read the definitions.
    sortable: sortsBy(definition.kind),
    groupable: groupsBy(definition.kind),
  }));
}

/** What `valueText` needs to say a value the way the reader reads it, all of it resolved. */
export interface ValueReading {
  /** A number the locale's way. */
  readonly number: (value: number) => string;
  /** An amount with its currency, as the catalogue joins the two. */
  readonly amount: (value: number, currency: string) => string;
  readonly duration: (minutes: number) => string;
  /** A person's name, or `undefined` for one this replica does not know. */
  readonly nameOf: (accountId: string) => string | undefined;
  /** An entry's title, or `undefined` for one this replica does not hold. */
  readonly titleOf: (itemId: string) => string | undefined;
  /** A group's name, a file's name and a moment in the reader's clock, where the caller has them. */
  readonly groupOf?: (groupId: string) => string | undefined;
  readonly fileOf?: (mediaId: string) => string | undefined;
  readonly moment?: (iso: string) => string;
  /** What an account or an entry this client cannot name reads as. */
  readonly unknown: string;
}

/**
 * A stored value as one line of text, for a row that shows rather than edits it; `undefined` for
 * no value. People and entries by name, never by identifier, and formatted text by its first line.
 */
export function valueText(
  definition: { readonly kind: string; readonly currency?: string | null },
  held: unknown,
  reading: ValueReading,
): string | undefined {
  if (held === undefined || held === null || held === '') return undefined;
  if (Array.isArray(held) && held.length === 0) return undefined;
  const kind = definition.kind as string;
  switch (kind) {
    case 'NUMBER':
    case 'CALCULATED':
      return typeof held === 'number' ? reading.number(held) : String(held);
    case 'MONEY':
      return typeof held === 'number' ? reading.amount(held, definition.currency ?? '') : String(held);
    case 'DURATION':
      return typeof held === 'number' ? reading.duration(held) : String(held);
    case 'USER':
      return reading.nameOf(String(held)) ?? reading.unknown;
    case 'USER_SET':
      return (Array.isArray(held) ? held : [held]).map((id) => reading.nameOf(String(id)) ?? reading.unknown).join(', ');
    case 'REFERENCE':
      return reading.titleOf(String(held)) ?? reading.unknown;
    case 'GROUP':
      return reading.groupOf?.(String(held)) ?? reading.unknown;
    case 'FILE':
      return reading.fileOf?.(String(held)) ?? reading.unknown;
    case 'DATE_TIME':
      return reading.moment ? reading.moment(String(held)) : String(held);
    case 'CASCADING_SELECT':
      return String(held).split('/').join(' › ');
    case 'RICH_TEXT':
      return String(held).trim().split('\n')[0];
    default:
      return Array.isArray(held) ? held.map(String).join(', ') : String(held);
  }
}
