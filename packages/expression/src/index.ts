// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The browser's evaluator of a type's form and of a calculated field, for a device the server
// cannot reach (foundation.md F-4, offline-sync.md §4.4). Online the server answers both - one
// evaluator decides what an editor shows and what a write is refused for (P-05) - and this one only
// stands in for it: it reads the same CEL text over the same names the server's environments
// declare (`FormEnvironment`, `FormulaEnvironment`), and its answer is replaced by the server's as
// soon as the write arrives there. It decides nothing the server does not decide again.

import { Environment } from '@marcbachmann/cel-js';
import type { components } from '@hubtask/api-client';

type EntryFormState = components['schemas']['EntryFormState'];
type Schema = components['schemas'];

/**
 * A type's form as the contract spells it, its defaults left optional: a form read from the
 * replica carries what the server wrote, and a default the server fills is absent there.
 */
export interface EntryForm {
  readonly fields?: readonly (Partial<Schema['EntryFormField']> & { readonly key: string })[];
  readonly validations?: readonly (Partial<NonNullable<Schema['EntryForm']['validations']>[number]> & { readonly key: string; readonly holds: string })[];
  readonly channels?: readonly (Partial<Schema['EntryFormChannel']> & { readonly channel: string })[];
}

/** The doors a form tells apart; an offline write reaches the server through `SYNC`. */
export type FormChannel = 'APP' | 'MCP' | 'RULE' | 'SYNC' | 'IMPORT' | 'MAIL';

/** The entry a form is asked about, as the draft stands. */
export interface FormEntry {
  readonly title?: string;
  readonly notes?: string;
  readonly level?: string;
  readonly entryType?: string;
  readonly completed?: boolean;
  /** An instant as the API spells one; read as the server reads it, in UTC to the second. */
  readonly dueAt?: string | null;
  readonly startAt?: string | null;
  readonly customFields?: Readonly<Record<string, unknown>>;
}

/** Who writes, as far as the device knows: the built-in role, every role and permission held. */
export interface FormActor {
  readonly role?: string;
  readonly roles?: readonly string[];
  readonly permissions?: readonly string[];
  readonly kind?: string;
}

/** The bounds of one expression, the server's (`MaxFormExpressionLength`) and the parser's. */
const MAX_EXPRESSION_LENGTH = 4096;
const LIMITS = { maxAstNodes: 2000, maxDepth: 64, maxListElements: 500, maxMapEntries: 500, maxCallArguments: 16 };

/** The largest magnitude a calculated value holds (`MaxCustomFieldMoneyMagnitude`). */
const MAX_CALCULATED_MAGNITUDE = 1e11;

const CUSTOM = 'custom_fields.';

function formEnvironment(): Environment {
  return new Environment({ limits: LIMITS })
    .registerVariable('entry', 'map')
    .registerVariable('actor', 'map')
    .registerVariable('channel', 'string')
    .registerVariable('step', 'string');
}

function formulaEnvironment(): Environment {
  return new Environment({ limits: LIMITS }).registerVariable('entry', 'map');
}

/** An instant as Go's `time.RFC3339` writes it in UTC: no fraction of a second. */
function instantOf(value: string | null | undefined): string | null {
  if (!value) return null;
  const at = new Date(value);
  if (Number.isNaN(at.getTime())) return null;
  return at.toISOString().replace(/\.\d{3}Z$/, 'Z');
}

/** The keys of the custom fields a form names, the only ones its conditions read. */
function customKeysOf(form: EntryForm): string[] {
  return (form.fields ?? []).flatMap((field) => (field.key.startsWith(CUSTOM) ? [field.key.slice(CUSTOM.length)] : []));
}

/**
 * How a type's form stands for a draft: which fields are required, read-only or hidden, which
 * validations fail, whether the door is closed - `Form.State` in the domain, answered here.
 * `undefined` where a condition cannot be read or answered: the editor then draws the form as it
 * stands and the server names the field once the write arrives.
 */
export function evaluateForm(
  form: EntryForm, entryTypeKey: string, channel: FormChannel, entry: FormEntry, actor: FormActor, step = '',
): EntryFormState | undefined {
  const custom: Record<string, unknown> = {};
  for (const key of customKeysOf(form)) custom[key] = entry.customFields?.[key] ?? null;
  const context = {
    entry: {
      title: entry.title ?? '', notes: entry.notes ?? '', level: entry.level ?? '',
      entry_type: entryTypeKey, completed: entry.completed ?? false,
      due_date: instantOf(entry.dueAt), start_at: instantOf(entry.startAt), custom_fields: custom,
    },
    actor: {
      role: actor.role ?? '', roles: [...(actor.roles ?? [])], permissions: [...(actor.permissions ?? [])], kind: actor.kind ?? '',
    },
    channel,
    step,
  };
  const environment = formEnvironment();
  const ask = (expression: string | undefined): boolean => {
    if (!expression || expression.trim() === '') return false;
    if (expression.length > MAX_EXPRESSION_LENGTH) throw new RangeError('expression.too_long');
    const value = environment.evaluate(expression, context);
    if (typeof value !== 'boolean') throw new TypeError('expression.not_boolean');
    return value;
  };

  const rule = (form.channels ?? []).find((each) => each.channel === channel);
  const hiddenByDoor = new Set(rule?.hidden ?? []);
  try {
    const fields: EntryFormState['fields'] = {};
    for (const field of form.fields ?? []) {
      const hidden = ask(field.hidden_when) || hiddenByDoor.has(field.key);
      const readOnly = ask(field.read_only_when);
      const required = (field.required ?? false) || ask(field.required_when);
      fields[field.key] = { required: required && !hidden, read_only: readOnly, hidden };
    }
    const failed: string[] = [];
    for (const validation of form.validations ?? []) {
      if (validation.field && fields[validation.field]?.hidden) continue;
      if (!ask(validation.holds)) failed.push(validation.key);
    }
    return { entry_type: entryTypeKey, channel, fields, failed, closed: rule?.closed ?? false };
  } catch {
    return undefined;
  }
}

/**
 * A formula's whole-number literals as decimals, as the server's CEL adapter rewrites them
 * (`wholeNumbersAsDoubles`): a number field always reaches a formula as a double and CEL keeps
 * int and double apart, so `hours * 2` would fail. Strings, identifiers, hexadecimal, unsigned
 * and decimal literals are left as written.
 */
export function wholeNumbersAsDecimals(formula: string): string {
  let out = '';
  let i = 0;
  const identifier = (c: string | undefined) => c !== undefined && /[A-Za-z0-9_]/.test(c);
  while (i < formula.length) {
    const c = formula[i] as string;
    if (c === '"' || c === "'") {
      const raw = /[rR]/.test(formula[i - 1] ?? '') && !identifier(formula[i - 2]);
      const triple = formula.startsWith(c.repeat(3), i);
      const close = triple ? c.repeat(3) : c;
      let j = i + close.length;
      while (j < formula.length && !formula.startsWith(close, j)) j += !raw && formula[j] === '\\' ? 2 : 1;
      j = Math.min(formula.length, j + close.length);
      out += formula.slice(i, j);
      i = j;
      continue;
    }
    if (identifier(c) && !/[0-9]/.test(c)) {
      let j = i;
      while (identifier(formula[j])) j += 1;
      out += formula.slice(i, j);
      i = j;
      continue;
    }
    if (/[0-9]/.test(c) && formula[i - 1] !== '.') {
      if (c === '0' && /[xX]/.test(formula[i + 1] ?? '')) {
        let j = i + 2;
        while (/[0-9A-Fa-f]/.test(formula[j] ?? '')) j += 1;
        if (/[uU]/.test(formula[j] ?? '')) j += 1;
        out += formula.slice(i, j);
        i = j;
        continue;
      }
      let j = i;
      while (/[0-9]/.test(formula[j] ?? '')) j += 1;
      let whole = true;
      if (formula[j] === '.' && /[0-9]/.test(formula[j + 1] ?? '')) {
        whole = false;
        j += 1;
        while (/[0-9]/.test(formula[j] ?? '')) j += 1;
      }
      if (/[eE]/.test(formula[j] ?? '')) {
        whole = false;
        j += 1;
        if (/[+-]/.test(formula[j] ?? '')) j += 1;
        while (/[0-9]/.test(formula[j] ?? '')) j += 1;
      }
      if (whole && /[uU]/.test(formula[j] ?? '')) {
        whole = false;
        j += 1;
      }
      out += whole ? `${formula.slice(i, j)}.0` : formula.slice(i, j);
      i = j;
      continue;
    }
    out += c;
    i += 1;
  }
  return out;
}

/**
 * A calculated field's value over an entry's values, as the server computes it (`calculate`):
 * the inputs given - the scope's number, money, duration and boolean fields - every number a
 * double, the result rounded to four places. `null` where the formula cannot answer: a field it
 * reads is empty, a division by zero, a value past the bound.
 */
export function calculate(formula: string, inputs: Readonly<Record<string, unknown>>): number | null {
  const values: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(inputs)) {
    if (value === null || value === undefined) continue;
    values[key] = typeof value === 'bigint' ? Number(value) : value;
  }
  if (formula.length > MAX_EXPRESSION_LENGTH) return null;
  let result: unknown;
  try {
    result = formulaEnvironment().evaluate(wholeNumbersAsDecimals(formula), { entry: { custom_fields: values } });
  } catch {
    return null;
  }
  const number = typeof result === 'bigint' ? Number(result) : result;
  if (typeof number !== 'number' || !Number.isFinite(number) || Math.abs(number) >= MAX_CALCULATED_MAGNITUDE) return null;
  return Math.round(number * 1e4) / 1e4;
}
