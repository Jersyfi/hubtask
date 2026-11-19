// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The reads of the one search over every kind and of the question in words (§6.11, Q-6), through
 * the engine. Both are `POST`s: what somebody looks for and what they ask are their content, and
 * a body never reaches an access log (UC-WRK-18/4). Neither writes, so neither makes a read stale.
 */

import type { Answer } from '../search/everything.ts';
import { engine } from './engine.ts';

/** `POST /search:all` with a body built by `requestOf`. */
export function searchAll(body: { q: string; kinds: readonly string[]; hub_id?: string; size?: number }): Promise<Answer> {
  return engine.mutate<Answer>('POST', '/search:all', body, { invalidates: [] });
}

/** `POST /query:ask`: the question written as a query, which nothing runs (Q-6). */
export async function askQuery(question: string): Promise<string> {
  const answer = await engine.mutate<{ query: string }>('POST', '/query:ask', { question: question.trim().slice(0, 500) }, { invalidates: [] });
  return answer.query;
}

/** `POST /items:aggregate`'s count of a line while it is typed (Q-4); undefined where it cannot count. */
export async function countLine(body: unknown): Promise<number | undefined> {
  const answer = await engine.mutate<{ data: readonly { count: number }[] }>('POST', '/items:aggregate', body, { invalidates: [] });
  return answer.data?.[0]?.count ?? 0;
}
