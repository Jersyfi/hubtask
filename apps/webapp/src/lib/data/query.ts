// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The query language, read out of the manifest and turned into a document.
 *
 * `/meta/capabilities` answers `query_fields`, and the schema says what that is for in its own
 * words: *"a client builds its filter editor from this rather than from a hard-coded list, because
 * the set grows with the installation's features — a field whose use case this version does not
 * have is not in it, and filtering on it is refused rather than silently matching nothing."* So
 * the editor offers what the installation reports, with the operators each field declares, and a
 * field this client has never heard of is one it simply does not show.
 *
 * **The rule this file exists to keep is the other half of that sentence.** A condition naming a
 * field the manifest does not report, or an operator the field does not declare, is **not sent**.
 * It is not silently corrected either — it is left out of the document, because a filter the
 * server refuses by name (`query.field_unknown`) is a request the reader gets nothing back from.
 * The refusal still renders as a sentence when it comes from somewhere this cannot see, which is
 * what `problem.ts` is for; this is the half that stops the client from causing it.
 *
 * Pure functions over a `Capabilities` document, for the reason `capability.ts` is: the
 * interesting part is the answers, and a module that reached for the manifest itself could only be
 * tested by mounting the application.
 */

import type { Capabilities, FilterNode, QueryField } from '@hubtask/sync-engine';

/** Every field the installation reports, in the order it reports them. */
export function queryFields(manifest: Capabilities | undefined): readonly QueryField[] {
  return manifest?.query_fields ?? [];
}

export function fieldNamed(
  manifest: Capabilities | undefined,
  name: string,
): QueryField | undefined {
  return queryFields(manifest).find((field) => field.field === name);
}

/**
 * The fields a condition can be built on: the ones that declare at least one operator.
 *
 * "Empty for a field that may only be ordered or grouped by" is the contract's own note, and it is
 * why this is a filter rather than the whole list — offering `order_key` in a filter editor that
 * can express nothing about it would be offering a row that cannot be completed.
 */
export function filterableFields(manifest: Capabilities | undefined): readonly QueryField[] {
  return queryFields(manifest).filter((field) => (field.operators?.length ?? 0) > 0);
}

export function sortableFields(manifest: Capabilities | undefined): readonly QueryField[] {
  return queryFields(manifest).filter((field) => field.sortable);
}

export function groupableFields(manifest: Capabilities | undefined): readonly QueryField[] {
  return queryFields(manifest).filter((field) => field.groupable);
}

/** The languages this installation can index text in, and what a `content_language` picker offers. */
export function textLanguages(manifest: Capabilities | undefined): readonly string[] {
  return manifest?.text_languages ?? [];
}

/** One comparison a reader has built. The value is text, because an input is text. */
export interface Condition {
  readonly field: string;
  readonly op: string;
  readonly value: string;
}

/** The operators that take no value at all. `IS_NULL` asks about absence, and absence has none. */
const VALUELESS = new Set(['IS_NULL']);

/** The operators whose value is a list. The contract names them, and this is that list. */
const LISTED = new Set(['IN', 'NOT_IN', 'CONTAINS_ANY', 'CONTAINS_ALL', 'BETWEEN']);

/** Whether an operator takes a value, for an editor deciding whether to draw the third control. */
export function takesValue(op: string): boolean {
  return !VALUELESS.has(op);
}

export function takesList(op: string): boolean {
  return LISTED.has(op);
}

/**
 * A condition's value, in the shape the field's kind asks for.
 *
 * `boolean` and `integer` are the two the contract types as something other than a string, and a
 * string sent for either is a `422` rather than a match. Everything else travels as text — an
 * identifier, a timestamp and an enum value are all strings on the wire, and a placeholder like
 * `@today` is a string this client never interprets.
 */
function valueFor(field: QueryField, op: string, raw: string): unknown {
  const one = (text: string): unknown => {
    const trimmed = text.trim();
    if (field.kind === 'boolean') return trimmed.toLowerCase() === 'true';
    if (field.kind === 'integer') {
      const parsed = Number(trimmed);
      return Number.isFinite(parsed) ? parsed : trimmed;
    }
    return trimmed;
  };

  if (!takesList(op)) return one(raw);
  return raw
    .split(',')
    .map((part) => part.trim())
    .filter((part) => part !== '')
    .map(one);
}

/**
 * Whether this condition is one the installation accepts, and therefore one worth sending.
 *
 * Three questions, and all three are the manifest's: is the field reported, does it declare this
 * operator, and — for `IS_NULL` — is it a field that can be absent at all. The contract puts the
 * third one on `nullable` in so many words: "whether the field can be absent, and `IS_NULL`
 * therefore means something".
 */
export function isSendable(manifest: Capabilities | undefined, condition: Condition): boolean {
  const field = fieldNamed(manifest, condition.field);
  if (!field) return false;
  if (!field.operators?.includes(condition.op)) return false;
  if (condition.op === 'IS_NULL' && !field.nullable) return false;
  // A value that has not been typed yet is not a condition. Sending `EQ ""` would be asking for
  // the entries whose title is the empty string, which is not what an unfinished row means.
  if (takesValue(condition.op) && condition.value.trim() === '') return false;
  return true;
}

/**
 * The filter document for a set of conditions, or `undefined` when none of them is sendable.
 *
 * `AND` between them, and one condition is sent as the leaf rather than wrapped: a combination of
 * one is a node the grammar allows and a reader would never have asked for. `OR` and `NOT` are in
 * the grammar (ADR-0026) and are not offered here — a two-level editor is a different control from
 * a list of conditions, and building half of one would be worse than not offering it.
 */
export function filterOf(
  manifest: Capabilities | undefined,
  conditions: readonly Condition[],
): FilterNode | undefined {
  const nodes = conditions
    .filter((condition) => isSendable(manifest, condition))
    .map((condition) => {
      const field = fieldNamed(manifest, condition.field)!;
      const leaf: FilterNode = { op: condition.op as FilterNode['op'], field: condition.field };
      return takesValue(condition.op)
        ? { ...leaf, value: valueFor(field, condition.op, condition.value) }
        : leaf;
    });

  if (nodes.length === 0) return undefined;
  return nodes.length === 1 ? nodes[0] : { op: 'AND', nodes };
}

/** One ordering. `undefined` where the field is not one the installation says may be sorted on. */
export function sortOf(
  manifest: Capabilities | undefined,
  field: string,
  dir: 'ASC' | 'DESC',
): readonly { field: string; dir: 'ASC' | 'DESC' }[] | undefined {
  const declared = fieldNamed(manifest, field);
  return declared?.sortable ? [{ field, dir }] : undefined;
}

/** The grouping, or nothing. Same rule: the manifest decides, never a list written here. */
export function groupOf(
  manifest: Capabilities | undefined,
  field: string,
): { field: string; limit_per_group: number } | undefined {
  const declared = fieldNamed(manifest, field);
  return declared?.groupable ? { field, limit_per_group: 50 } : undefined;
}
