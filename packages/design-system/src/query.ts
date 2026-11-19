// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shape `QueryLine` draws (work-surface concept §6.11, Q-1…Q-4). The line knows no grammar:
// the language - one parser on the server, a client that mirrors it - is handed in as `parse` and
// `format`, the fields as a catalogue. What lives here is what every spelling shares: a token, a
// suggestion, and how suggestions are found among the catalogue's fields and values.

/** A field of the catalogue: `desk.sla`, `who`, `cf.priorisiert`. Products bring fields, not dialects. */
export interface QueryField {
  /** The key as it is typed and saved; keywords stay English (§6.11). */
  readonly key: string;
  /** The localised name a chip and a suggestion show: *SLA*, *Assignee*. */
  readonly label: string;
  /** Its kind, said in the suggestion: *choice · Service desk*. Resolved text. */
  readonly detail?: string;
  /** An example spelling the suggestion shows at its end: `prio: P1, P2`. */
  readonly example?: string;
}

/** A value of a field, from the data, with how many objects carry it now. */
export interface QueryValue {
  readonly value: string;
  readonly label: string;
  /** *2 open*: resolved text, so the count says what it counts. */
  readonly countLabel?: string;
}

/** What is wrong with a token, said at its chip, with the spelling that fixes it (Q-4). */
export interface QueryProblem {
  readonly message: string;
  /** The token's text that would fix it, offered as one press. */
  readonly fix?: string;
  readonly fixLabel?: string;
}

/**
 * One piece of the line. A `chip` is a field with its comparison and values; a `text` chip is what
 * has no chip form - a group with OR, free words in quotes - and stays its spelling (Q-2).
 */
export type QueryToken =
  | {
      readonly kind: 'chip';
      readonly id: string;
      readonly field: string;
      readonly fieldLabel: string;
      /** The comparison as it reads: `:` is left out, `<`, `..` and `not` are said. */
      readonly operatorLabel?: string;
      readonly values: readonly { readonly value: string; readonly label: string }[];
      readonly isNegated?: boolean;
      readonly problem?: QueryProblem;
    }
  | { readonly kind: 'text'; readonly id: string; readonly text: string; readonly problem?: QueryProblem };

/** What a parse of the typed text gives back: the finished tokens and what is still being typed. */
export interface QueryParse {
  readonly tokens: readonly QueryToken[];
  readonly rest: string;
}

/** The language, as the line needs it. Phase 2 hands in the real one; stories hand in a stub. */
export interface QueryLanguage {
  parse(text: string): QueryParse;
  format(token: QueryToken): string;
}

/** The fields and, per field, the values the data holds. */
export interface FieldCatalogue {
  readonly fields: readonly QueryField[];
  values?(field: string, prefix: string): readonly QueryValue[];
}

/** One row of the suggestion list. `insert` is what pressing it puts into the line. */
export interface QuerySuggestion {
  readonly id: string;
  readonly group: 'field' | 'value';
  readonly label: string;
  readonly detail?: string;
  readonly hint?: string;
  readonly insert: string;
}

const fold = (text: string) => text.normalize('NFKD').replace(/\p{M}/gu, '').toLowerCase();

/**
 * The suggestions for what is being typed (Q-3): after `field:` the field's values from the data,
 * otherwise the fields whose key or name contain the word - those that start with it first.
 * Nothing typed, nothing suggested: the list opens with the first character.
 */
export function suggest(catalogue: FieldCatalogue, typed: string, { isBrowsing = false }: { isBrowsing?: boolean } = {}): QuerySuggestion[] {
  const line = typed.trimStart();
  // Inside an open group - `(label: vip OR ` - the part after the bracket or the last OR is what
  // is completed, and what comes before it is kept in front of every insert.
  const isGroupOpen = line.startsWith('(') && !line.includes(')');
  const or = line.lastIndexOf(' OR ');
  const cut = isGroupOpen ? Math.max(line.lastIndexOf('(') + 1, or >= 0 ? or + 4 : 0) : 0;
  const head = line.slice(0, cut);
  const word = line.slice(cut).trimStart();
  const grouped = (items: QuerySuggestion[]) => (head === '' ? items : items.map((item) => ({ ...item, insert: `${head}${item.insert}` })));
  if (word === '') return isBrowsing || isGroupOpen ? grouped(catalogue.fields.map(fieldSuggestion(''))) : [];
  return grouped(suggestWord(catalogue, word));
}

const fieldSuggestion = (negation: string) => (field: QueryField): QuerySuggestion => ({
  id: `field-${field.key}`,
  group: 'field' as const,
  label: field.label,
  detail: field.detail,
  hint: field.example,
  insert: `${negation}${field.key}: `,
});

function suggestWord(catalogue: FieldCatalogue, word: string): QuerySuggestion[] {
  const colon = /^(-?)([\w.]+):\s*(.*)$/.exec(word);
  if (colon) {
    const negation = colon[1] ?? '';
    const key = colon[2] ?? '';
    const prefix = colon[3] ?? '';
    const field = catalogue.fields.find((candidate) => candidate.key === key);
    if (!field) return [];
    const last = prefix.split(',').at(-1)?.trim() ?? '';
    const head = prefix.slice(0, prefix.length - last.length);
    return (catalogue.values?.(key, last) ?? [])
      .filter((value) => fold(value.label).includes(fold(last)) || fold(value.value).includes(fold(last)))
      .map((value) => ({
        id: `value-${key}-${value.value}`,
        group: 'value' as const,
        label: value.label,
        hint: value.countLabel,
        insert: `${negation}${key}: ${head}${value.value} `,
      }));
  }
  const needle = fold(word.replace(/^-/, ''));
  const negation = word.startsWith('-') ? '-' : '';
  const scored = catalogue.fields
    .map((field) => {
      const key = fold(field.key);
      const label = fold(field.label);
      const rank = key.startsWith(needle) || label.startsWith(needle) ? 0 : key.includes(needle) || label.includes(needle) ? 1 : -1;
      return { field, rank };
    })
    .filter(({ rank }) => rank >= 0)
    .sort((a, b) => a.rank - b.rank);
  return scored.map(({ field }) => fieldSuggestion(negation)(field));
}

/** The line as text, the spelling that is saved and shared: every token, then what is being typed. */
export function lineText(language: QueryLanguage, tokens: readonly QueryToken[], rest = ''): string {
  return [...tokens.map((token) => language.format(token)), rest.trim()].filter(Boolean).join(' ');
}
