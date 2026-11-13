// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The search page's one line (§6.11, Q-1 to Q-3): `@hubtask/query`'s language and catalogue for
 * the design system's QueryLine, the keys named in the reader's language. A key without a name of
 * its own keeps its English spelling, which is also what a saved query holds.
 */

import { createCatalogue, createLanguage, type QueryError, type QueryKey } from '@hubtask/query';

type Translate = (code: string, params?: Record<string, string | number>) => string;

/** The message code of a key's name: `app.qkey.desk_sla` for `desk.sla`. */
export const keyCode = (key: string) => `app.qkey.${key.replaceAll('.', '_')}`;

/** The wording the query package takes: names where there are messages, the key otherwise. */
export function wordingOf(t: Translate, has: (code: string) => boolean) {
  return {
    key: (key: string) => (has(keyCode(key)) ? t(keyCode(key)) : key),
    problem: (error: QueryError) => (has(error.code) ? t(error.code, error.params) : error.code),
  };
}

/** The language and the catalogue QueryLine is fed, from the manifest's `query_keys`. */
export function lineOf(keys: readonly QueryKey[] | undefined, t: Translate, has: (code: string) => boolean) {
  const wording = wordingOf(t, has);
  return { language: createLanguage(wording), catalogue: createCatalogue(keys ?? [], wording), known: new Set((keys ?? []).map((key) => key.key)) };
}

/**
 * The line as the entry search's own reader takes it (`data/searchquery.ts`): `key: value` written
 * `key:value`, the spelling it was built on. The line on screen keeps the query's own spelling.
 */
export function compactLine(line: string): string {
  return line.replace(/(^|\s)(-?[A-Za-z][\w.]*):\s+(?=\S)/gu, '$1$2:');
}
