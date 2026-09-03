// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The seam a component calls: a code in, a sentence out.
 *
 * Everything underneath it is pure — the catalogue is data, `format` is a function, the locale is
 * a string — so this is a factory rather than a global. What the application holds is one instance
 * (`i18n.svelte.ts`); what a test holds is its own, with its own catalogues, which is why none of
 * this needs a DOM or a running app to check.
 */

import { SOURCE, type Catalogue, patternFor } from './catalogue.ts';
import { MessageSyntaxError, format, type MessageParams } from './format.ts';

export interface Messages {
  readonly locale: string;
  /** The sentence for a code. Never a blank, never a key — `i18n-l10n.md` §3. */
  t(code: string, params?: MessageParams): string;
  /**
   * Whether any catalogue in the chain knows the code, without rendering it. What a caller with
   * two codes in hand needs — a problem document carries `code` and the more specific
   * `detail_code`, and the specific one is only better if it exists (ADR-0025, and
   * `Catalogue.Has` on the Go side).
   */
  has(code: string): boolean;
}

export interface MessagesOptions {
  readonly locale: string;
  /**
   * The catalogues to read, the reader's language first. The source catalogue is appended if it is
   * not already there: §3's fallback is not optional, and a caller that forgot it would produce
   * exactly the empty screen the rule forbids.
   */
  readonly catalogues?: readonly Catalogue[];
  /** Where an unrenderable message is reported. Injected so a test can read it. */
  readonly onProblem?: (message: string) => void;
}

/**
 * A code nobody has a sentence for, made readable: `items.due_date_in_past` becomes "Due date in
 * past". Not a translation and not pretending to be one — it is the answer to F1-07's requirement
 * that an unknown code render "something a person can read rather than a blank or a key".
 *
 * The alternative, printing the code, is what the Go renderer does, and the two are right in
 * different places: `hubctl` prints for an operator who can look the code up, and this renders for
 * somebody who cannot. The code still reaches the console, so the person who *can* look it up has
 * it.
 */
export function humanise(code: string): string {
  const last = code.split('.').pop() ?? code;
  const words = last.replace(/[_-]+/g, ' ').trim();
  return words === '' ? code : words.charAt(0).toUpperCase() + words.slice(1);
}

export function createMessages({ locale, catalogues, onProblem }: MessagesOptions): Messages {
  const chain = catalogues && catalogues.length > 0 ? [...catalogues] : [SOURCE];
  if (!chain.includes(SOURCE)) chain.push(SOURCE);
  const report = onProblem ?? ((message: string) => console.warn(`i18n: ${message}`));
  const reported = new Set<string>();
  const reportOnce = (key: string, message: string) => {
    if (reported.has(key)) return;
    reported.add(key);
    report(message);
  };

  return {
    locale,
    has: (code) => patternFor(code, chain) !== undefined,
    t(code, params = {}) {
      const pattern = patternFor(code, chain);
      if (pattern === undefined) {
        reportOnce(code, `no message for the code ${code}`);
        return humanise(code);
      }
      try {
        return format(pattern, params, locale);
      } catch (error) {
        // Loud where somebody can act on it, and never a blank screen for the reader: the pattern
        // is at least the sentence, with a brace in it. `catalogue.test.ts` parses every message
        // in the source catalogue, so this branch is reachable only from a translation that
        // arrived after the build — which is exactly the case that must not take the page down.
        const detail = error instanceof MessageSyntaxError ? error.message : String(error);
        reportOnce(code, `the message ${code} cannot be rendered: ${detail}`);
        return pattern;
      }
    },
  };
}
