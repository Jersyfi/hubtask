// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The query line's language as a collection's view speaks it (§6.11, E-21): the keys and problems
 * in the reader's words, and a `word:` that names no field searched as words, said at its chip
 * with the field it may have meant as the fix (Q-2). One for the panel and the phone's strip, so
 * both read a line alike.
 */

import { createLanguage, parseQuery, type QueryKey } from '@hubtask/query';

import { messages, t } from '../i18n/i18n.svelte.ts';

const named = (code: string, fallback: string, params?: Record<string, string>) => (messages.has(code) ? t(code, params) : fallback);

export const wording = {
  key: (key: string) => named(`app.qkey.${key.replaceAll('.', '_')}`, key),
  kind: (key: QueryKey) => [key.value, key.product ? named(`app.qproduct.${key.product}`, key.product) : ''].filter(Boolean).join(' · '),
  problem: (error: { code: string; params: Readonly<Record<string, string>> }) => named(error.code, error.code, error.params as Record<string, string>),
};

const base = createLanguage(wording);

export const language = {
  format: base.format,
  parse(text: string) {
    const read = base.parse(text);
    return {
      rest: read.rest,
      tokens: read.tokens.map((token) => {
        if (!token.problem) return token;
        const spelled = base.format(token);
        const parsed = parseQuery(spelled);
        if (parsed.ok || parsed.error.code !== 'query.field_unknown') return token;
        const suggestion = parsed.error.params['suggestion'];
        const rest = spelled.slice(spelled.indexOf(':') + 1).trim();
        return {
          ...token,
          problem: {
            message: t('app.viewcfg.as_words'),
            ...(suggestion ? { fix: `${suggestion}: ${rest}`, fixLabel: t('app.viewcfg.meant', { key: suggestion }) } : {}),
          },
        };
      }),
    };
  },
};
