// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What a search is narrowed by: the chips, what the address carries, and the tree they compile to.
 *
 * Pure, and tested without mounting anything. A chip is a **question with a closed set of
 * answers** — which kind, which state, which label, whose, when, where — and every one of them
 * compiles to a `FilterNode` the contract already accepts (ADR-0026, ADR-0064). Nothing here
 * invents a field: the names are the ones `/meta/capabilities` reports, and a chip whose field the
 * installation does not report is not offered.
 *
 * **The term is not in here, and that is deliberate.** `POST /search` has no `GET` because what
 * somebody is looking for is their content, and a query string travels through access logs,
 * proxies and browser history (`api-guidelines.md` §2, `search.svelte.ts`). The *narrowing* is
 * structural — a label, a state, a collection — so it goes in the address and makes a search a
 * link; the words stay where they live. ADR-0063 decision 4 asked for the words to travel too,
 * and that is the one sentence of it this does not do, because it would undo a decision the
 * product had already taken and written down twice.
 */

/** One chip: a question, and the values chosen in it. */
export type Chosen = Readonly<Record<string, readonly string[]>>;

/** The chips this screen offers, in the order they are drawn. */
export const CHIPS = ['type', 'state', 'when', 'label', 'who', 'where'] as const;

export type ChipName = (typeof CHIPS)[number];

/** Which query field each chip needs, so a chip whose field is not reported is not offered. */
export const CHIP_FIELDS: Readonly<Record<ChipName, string>> = {
  type: 'type',
  state: 'is_completed',
  when: 'due_at',
  label: 'labels',
  who: 'assignee_id',
  where: 'collection_id',
};

/** The values of the chips whose answers are the product's rather than the workspace's. */
export const TYPES = ['TASK', 'WORK_PACKAGE', 'ACTIVITY'] as const;
export const STATES = ['open', 'done'] as const;
export const WHENS = ['overdue', 'today', 'week', 'undated'] as const;

/** A filter node, in the shape the contract names. */
export interface Node {
  readonly op: string;
  readonly field?: string;
  readonly value?: unknown;
  readonly nodes?: readonly Node[];
}

/** Whether anything at all is chosen. */
export function isNarrowed(chosen: Chosen): boolean {
  return Object.values(chosen).some((values) => values.length > 0);
}

/** How many answers a chip carries, which is what the chip shows beside its word. */
export function countOf(chosen: Chosen, chip: ChipName): number {
  return chosen[chip]?.length ?? 0;
}

/** Toggles one answer of one chip, keeping the rest. */
export function toggle(chosen: Chosen, chip: ChipName, value: string): Chosen {
  const values = chosen[chip] ?? [];
  const next = values.includes(value) ? values.filter((each) => each !== value) : [...values, value];
  const result: Record<string, readonly string[]> = { ...chosen };
  if (next.length === 0) delete result[chip];
  else result[chip] = next;
  return result;
}

/** Empties one chip. */
export function clearChip(chosen: Chosen, chip: ChipName): Chosen {
  const result: Record<string, readonly string[]> = { ...chosen };
  delete result[chip];
  return result;
}

/**
 * The chips as the address carries them: one parameter per chip, values separated by a comma.
 *
 * A comma rather than a repeated parameter because the router reads one value per name, and
 * because a narrowing somebody can read in the address bar is one they can also edit.
 */
export function toQuery(chosen: Chosen): Record<string, string> {
  const query: Record<string, string> = {};
  for (const chip of CHIPS) {
    const values = chosen[chip];
    if (values && values.length > 0) query[chip] = values.join(',');
  }
  return query;
}

/** And back, ignoring anything that is not a chip this screen knows. */
export function fromQuery(query: Readonly<Record<string, string>>): Chosen {
  const chosen: Record<string, readonly string[]> = {};
  for (const chip of CHIPS) {
    const raw = query[chip];
    if (!raw) continue;
    const values = raw.split(',').map((value) => value.trim()).filter(Boolean);
    if (values.length > 0) chosen[chip] = values;
  }
  return chosen;
}

/**
 * The tree the chips compile to: an AND of one node per chip, and within a chip an OR of its
 * answers — several values of one question widen it, several questions narrow it.
 *
 * `@today` and the rest are the server's placeholders, resolved in the actor's time zone and the
 * actor's week start (`api-guidelines.md` §3). Nothing here computes a date, because a client that
 * did would compute it in the browser's zone and be wrong for everybody travelling.
 */
export function toFilter(chosen: Chosen): Node | undefined {
  const nodes: Node[] = [];
  for (const chip of CHIPS) {
    const values = chosen[chip] ?? [];
    if (values.length === 0) continue;
    const node = nodeFor(chip, values);
    if (node) nodes.push(node);
  }
  if (nodes.length === 0) return undefined;
  if (nodes.length === 1) return nodes[0];
  return { op: 'AND', nodes };
}

function nodeFor(chip: ChipName, values: readonly string[]): Node | undefined {
  switch (chip) {
    case 'type':
      return { op: 'IN', field: 'type', value: [...values] };
    case 'where':
      return { op: 'IN', field: 'collection_id', value: [...values] };
    case 'who':
      return { op: 'IN', field: 'assignee_id', value: [...values] };
    case 'label':
      // Any of them, which is what choosing two labels means to a reader.
      return { op: 'CONTAINS_ANY', field: 'labels', value: [...values] };
    case 'state':
      // Both chosen is no narrowing at all, and a tree that says so is a tree that costs nothing.
      if (values.length === STATES.length) return undefined;
      return { op: 'EQ', field: 'is_completed', value: values[0] === 'done' };
    case 'when':
      return whenNode(values);
    default:
      return undefined;
  }
}

function whenNode(values: readonly string[]): Node | undefined {
  const nodes = values.map(oneWhen).filter((node): node is Node => node !== undefined);
  if (nodes.length === 0) return undefined;
  if (nodes.length === 1) return nodes[0];
  return { op: 'OR', nodes };
}

function oneWhen(value: string): Node | undefined {
  switch (value) {
    case 'overdue':
      return { op: 'LT', field: 'due_at', value: '@today' };
    case 'today':
      return { op: 'LTE', field: 'due_at', value: '@today' };
    case 'week':
      return { op: 'LTE', field: 'due_at', value: '@end_of_week' };
    case 'undated':
      return { op: 'IS_NULL', field: 'due_at' };
    default:
      return undefined;
  }
}
