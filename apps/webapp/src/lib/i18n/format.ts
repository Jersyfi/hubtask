// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * ICU MessageFormat, the subset this product's catalogue uses, rendered in the browser.
 *
 * The server sends a code and parameters and never a sentence (ADR-0011, `i18n-l10n.md` §1). This
 * is the half that turns the pair into words, and the reason it is written here rather than
 * installed is that the hard part is already in the platform: `Intl.PluralRules` knows the CLDR
 * categories for every locale a browser supports, which is the one piece of an ICU implementation
 * that cannot be got right by hand. What is left is a parser of about two hundred lines.
 *
 * **What it implements:** simple arguments, `plural` (with `offset:` and `=n` exact matches),
 * `selectordinal`, `select`, `#` inside a plural, nested messages inside every branch, and ICU's
 * apostrophe quoting.
 *
 * **What it refuses:** everything else - `{n, number}`, `{d, date, short}`, `{t, time}` and any
 * other argument type - by throwing `MessageSyntaxError` naming what it found. That is the
 * condition F1-07 put on writing this rather than installing it: "a plural rule that silently
 * renders the `other` branch is a defect nobody sees in English and everybody sees in Polish". A
 * subset that guessed would be exactly that defect. `catalogue.test.ts` parses every message in
 * `locales/en.json`, so a construct this cannot render turns the build red on the day it is
 * written into the catalogue, rather than on the day somebody reads Polish.
 */

/** A value a message can carry. Dates are deliberately absent: date formatting is `i18n-l10n.md` §4. */
export type MessageValue = string | number;

export type MessageParams = Readonly<Record<string, MessageValue>>;

/** Thrown by `parse` for anything this renderer would otherwise have to guess about. */
export class MessageSyntaxError extends Error {
  readonly pattern: string;

  constructor(message: string, pattern: string) {
    super(message);
    this.name = 'MessageSyntaxError';
    this.pattern = pattern;
  }
}

type Node =
  | { readonly kind: 'text'; readonly value: string }
  | { readonly kind: 'argument'; readonly name: string }
  /** `#` - the plural's own number, less its offset. */
  | { readonly kind: 'number'; readonly name: string; readonly offset: number }
  | {
      readonly kind: 'plural';
      readonly name: string;
      readonly type: 'cardinal' | 'ordinal';
      readonly offset: number;
      readonly exact: ReadonlyMap<number, readonly Node[]>;
      readonly categories: ReadonlyMap<string, readonly Node[]>;
    }
  | { readonly kind: 'select'; readonly name: string; readonly options: ReadonlyMap<string, readonly Node[]> };

/** The CLDR categories a plural branch may name. An unknown keyword is a typo, and typos are loud. */
const CATEGORIES = new Set(['zero', 'one', 'two', 'few', 'many', 'other']);

const NAME = /^[^\s,{}#]+$/;

class Parser {
  #at = 0;
  readonly pattern: string;

  constructor(pattern: string) {
    this.pattern = pattern;
  }

  /** The top level, and every branch body: text until an unmatched `}` or the end. */
  parseMessage(insidePlural: { name: string; offset: number } | null, depth: number): Node[] {
    const nodes: Node[] = [];
    let text = '';
    const flush = () => {
      if (text) nodes.push({ kind: 'text', value: text });
      text = '';
    };

    while (this.#at < this.pattern.length) {
      const char = this.pattern[this.#at]!;

      if (char === '}') {
        if (depth === 0) this.fail('a `}` with no `{` before it');
        break;
      }

      if (char === "'") {
        text += this.readQuoted();
        continue;
      }

      if (char === '#' && insidePlural) {
        flush();
        nodes.push({ kind: 'number', name: insidePlural.name, offset: insidePlural.offset });
        this.#at += 1;
        continue;
      }

      if (char === '{') {
        flush();
        nodes.push(this.parseArgument(depth));
        continue;
      }

      text += char;
      this.#at += 1;
    }

    flush();
    return nodes;
  }

  /**
   * ICU's apostrophe rule, which is subtle enough to be worth stating: `''` is one apostrophe, an
   * apostrophe before `{`, `}` or `#` quotes everything up to the next one, and an apostrophe
   * anywhere else is just an apostrophe. Getting this wrong turns "the item's owner" into a
   * swallowed sentence, in English, where nobody would look for a formatting bug.
   */
  readQuoted(): string {
    this.#at += 1; // the opening apostrophe
    if (this.pattern[this.#at] === "'") {
      this.#at += 1;
      return "'";
    }
    const next = this.pattern[this.#at];
    if (next !== '{' && next !== '}' && next !== '#') return "'";

    let out = '';
    while (this.#at < this.pattern.length) {
      const char = this.pattern[this.#at]!;
      if (char === "'") {
        // `''` inside a quoted run is still one apostrophe; a single one ends the run.
        if (this.pattern[this.#at + 1] === "'") {
          out += "'";
          this.#at += 2;
          continue;
        }
        this.#at += 1;
        return out;
      }
      out += char;
      this.#at += 1;
    }
    // An unterminated run reads to the end, which is what ICU does.
    return out;
  }

  parseArgument(depth: number): Node {
    this.#at += 1; // `{`
    const name = this.readUntil([',', '}']).trim();
    if (!NAME.test(name)) this.fail(`\`${name}\` is not an argument name`);

    if (this.pattern[this.#at] === '}') {
      this.#at += 1;
      return { kind: 'argument', name };
    }

    this.#at += 1; // `,`
    const type = this.readUntil([',', '}']).trim();

    if (type === 'select') {
      this.expect(',');
      const options = this.parseBranches(null, depth);
      if (!options.has('other')) this.fail(`the select \`${name}\` has no \`other\` branch`);
      return { kind: 'select', name, options };
    }

    if (type === 'plural' || type === 'selectordinal') {
      this.expect(',');
      const offset = this.readOffset();
      const branches = this.parseBranches({ name, offset }, depth);
      const exact = new Map<number, readonly Node[]>();
      const categories = new Map<string, readonly Node[]>();
      for (const [key, body] of branches) {
        if (key.startsWith('=')) {
          const value = Number(key.slice(1));
          if (!Number.isFinite(value)) this.fail(`\`${key}\` is not an exact plural match`);
          exact.set(value, body);
        } else if (CATEGORIES.has(key)) {
          categories.set(key, body);
        } else {
          this.fail(`\`${key}\` is not a CLDR plural category`);
        }
      }
      if (!categories.has('other')) this.fail(`the plural \`${name}\` has no \`other\` branch`);
      return {
        kind: 'plural',
        name,
        type: type === 'plural' ? 'cardinal' : 'ordinal',
        offset,
        exact,
        categories,
      };
    }

    // The refusal F1-07 asked for. Naming the type is the point: "unsupported syntax" sends a
    // reader looking, "{n, number} is not implemented" sends them to the one line to change.
    this.fail(`\`{${name}, ${type}}\` is not implemented by this renderer`);
  }

  readOffset(): number {
    const save = this.#at;
    this.skipSpace();
    if (!this.pattern.startsWith('offset:', this.#at)) {
      this.#at = save;
      return 0;
    }
    this.#at += 'offset:'.length;
    const digits = this.readUntil([' ', '\t', '\n', '{']);
    const value = Number(digits.trim());
    if (!Number.isFinite(value)) this.fail(`\`${digits}\` is not an offset`);
    return value;
  }

  parseBranches(insidePlural: { name: string; offset: number } | null, depth: number): Map<string, readonly Node[]> {
    const branches = new Map<string, readonly Node[]>();
    while (this.#at < this.pattern.length) {
      this.skipSpace();
      if (this.pattern[this.#at] === '}') {
        this.#at += 1;
        return branches;
      }
      const key = this.readUntil(['{', ' ', '\t', '\n']).trim();
      this.skipSpace();
      if (this.pattern[this.#at] !== '{') this.fail(`the branch \`${key}\` has no body`);
      this.#at += 1;
      branches.set(key, this.parseMessage(insidePlural, depth + 1));
      if (this.pattern[this.#at] !== '}') this.fail(`the branch \`${key}\` is not closed`);
      this.#at += 1;
    }
    this.fail('the argument is not closed');
  }

  readUntil(stops: readonly string[]): string {
    const start = this.#at;
    while (this.#at < this.pattern.length && !stops.includes(this.pattern[this.#at]!)) this.#at += 1;
    if (this.#at >= this.pattern.length) this.fail('the argument is not closed');
    return this.pattern.slice(start, this.#at);
  }

  expect(char: string): void {
    this.skipSpace();
    if (this.pattern[this.#at] !== char) this.fail(`expected \`${char}\``);
    this.#at += 1;
  }

  skipSpace(): void {
    while (this.#at < this.pattern.length && /\s/.test(this.pattern[this.#at]!)) this.#at += 1;
  }

  fail(what: string): never {
    throw new MessageSyntaxError(`${what} (at ${this.#at} in \`${this.pattern}\`)`, this.pattern);
  }
}

/** Parses a pattern, or throws. Exported so that a test can parse the whole catalogue. */
export function parse(pattern: string): readonly Node[] {
  return new Parser(pattern).parseMessage(null, 0);
}

const cache = new Map<string, readonly Node[]>();

function parseCached(pattern: string): readonly Node[] {
  let nodes = cache.get(pattern);
  if (!nodes) {
    nodes = parse(pattern);
    cache.set(pattern, nodes);
  }
  return nodes;
}

/**
 * A parameter that is not there leaves its placeholder standing rather than blanking it - the same
 * choice `infrastructure/i18n` made on the Go side, and for the same reason: `{request_id}` on the
 * screen says a value went missing, an empty gap says nothing at all.
 */
function render(nodes: readonly Node[], params: MessageParams, locale: string): string {
  let out = '';
  for (const node of nodes) {
    switch (node.kind) {
      case 'text':
        out += node.value;
        break;
      case 'argument': {
        const value = params[node.name];
        out += value === undefined ? `{${node.name}}` : formatValue(value, locale);
        break;
      }
      case 'number': {
        const value = numberOf(params[node.name]);
        out += value === undefined ? `{${node.name}}` : formatValue(value - node.offset, locale);
        break;
      }
      case 'plural': {
        const value = numberOf(params[node.name]);
        if (value === undefined) {
          out += `{${node.name}}`;
          break;
        }
        const exact = node.exact.get(value);
        const category = new Intl.PluralRules(locale, { type: node.type }).select(value - node.offset);
        out += render(exact ?? node.categories.get(category) ?? node.categories.get('other')!, params, locale);
        break;
      }
      case 'select': {
        const value = params[node.name];
        const chosen = value === undefined ? undefined : node.options.get(String(value));
        out += render(chosen ?? node.options.get('other')!, params, locale);
        break;
      }
    }
  }
  return out;
}

function numberOf(value: MessageValue | undefined): number | undefined {
  if (typeof value === 'number') return Number.isFinite(value) ? value : undefined;
  if (typeof value === 'string' && value.trim() !== '') {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

/** A number is written the way the locale writes numbers: 1000 is `1,000` in en and `1.000` in de. */
function formatValue(value: MessageValue, locale: string): string {
  return typeof value === 'number' ? new Intl.NumberFormat(locale).format(value) : value;
}

/** Renders a pattern. Throws `MessageSyntaxError` on syntax this renderer does not implement. */
export function format(pattern: string, params: MessageParams = {}, locale = 'en'): string {
  return render(parseCached(pattern), params, locale);
}
