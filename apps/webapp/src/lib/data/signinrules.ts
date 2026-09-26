// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The rules a password has to meet, and whether the one being typed meets them.
 *
 * **The server answers rules, never sentences.** `GET /auth/sign-in-rules` hands back which
 * switches this workspace has on and with what numbers; this module turns each of them into a
 * message code with parameters, and the renderer turns *that* into a sentence in the reader's
 * language (ADR-0011). The same codes are what `field_errors[]` carries when the server refuses a
 * password, so one fact has one sentence whether the client saw the refusal coming or the server
 * sent it.
 *
 * **Two prüfer, one truth.** What this predicts, the domain decides. That is the second
 * implementation `capability.ts` already warns about, and the answer is the same: a fixture under
 * `api/` carries rules, passwords and the violations they produce, and both sides read it. Where
 * the two disagree in front of a person, the server wins and the line flips - a prediction is a
 * courtesy, the refusal is the verdict.
 *
 * **What is checked here and what is not.** Length, the four classes, repetition and the context
 * words are arithmetic over the characters: the client has them all and can answer at every
 * keystroke. The lists, the breach corpus, the history and "not the one you have now" are the
 * server's, and are asked for once the local rules hold - `signin.svelte.ts` is where that waiting
 * lives, because it is the part that talks.
 *
 * Normalisation is NFKC before anything is counted, so the same word typed on two keyboards is one
 * password - the same normalisation the domain applies before it hashes.
 */

/** One switch of the password half of the policy, as the contract answers it. */
export interface PasswordRules {
  readonly min_length: number;
  readonly min_lowercase: number;
  readonly min_uppercase: number;
  readonly min_digits: number;
  readonly min_symbols: number;
  readonly min_classes: number;
  /** Most of the same character in a row, or `null` where the switch is off. */
  readonly max_repeat: number | null;
  readonly common_passwords: boolean;
  readonly context_words: boolean;
  readonly breach_check: boolean;
  /** How many previous passwords are remembered. Answered only to a reader who has one. */
  readonly history_count: number;
  /** Whether "not the password you have now" applies - only where there is one. */
  readonly not_current: boolean;
}

/** One provider this workspace signs in with, as the sign-in screen needs it. */
export interface ProviderSummary {
  readonly id: string;
  readonly display_name: string;
  /** The preset it was configured from: what decides which mark is drawn (`ProviderMark`). */
  readonly kind: string;
  /** Whether the installation offers it to every workspace, or this workspace configured it. */
  readonly scope: 'installation' | 'workspace';
}

/** The links a workspace's operator is obliged to show, resolved for this host. */
export interface LegalLinks {
  readonly imprint_url?: string;
  readonly privacy_url?: string;
  readonly terms_url?: string;
  readonly accessibility_url?: string;
}

/** What `GET /auth/sign-in-rules` answers. Deliberately the least a sign-in screen needs. */
export interface SignInRules {
  readonly workspace_host: string;
  readonly methods: readonly string[];
  readonly providers: readonly ProviderSummary[];
  readonly password: PasswordRules;
  readonly legal: LegalLinks;
}

/** Who is setting the password, for the rules that are about them rather than about the string. */
export interface PasswordContext {
  readonly email?: string;
  readonly displayName?: string;
  readonly workspaceName?: string;
  readonly workspaceHost?: string;
}

/**
 * Where a rule stands.
 *
 * `server` and `checking` are two states of one rule rather than one state with a flag, because a
 * reader reads them differently: the first says "this will be checked", the second says "it is
 * being checked now". `unchecked` is the honest third: the server could not be reached, nothing is
 * known, and sending is still allowed.
 */
export type RuleState = 'met' | 'unmet' | 'failed' | 'server' | 'checking' | 'unchecked';

/** One line of the list under the field. */
export interface RuleLine {
  /** Stable across renders: the key a keyed `{#each}` uses and the server's violation names. */
  readonly id: string;
  readonly code: string;
  readonly params: Record<string, string>;
  readonly state: RuleState;
  /** Whether this one is the server's to answer. */
  readonly isServerSide: boolean;
}

const CODE = 'auth.password_rule';

/** NFKC, because the same word typed on two keyboards has to be one password. */
export function normalise(password: string): string {
  return password.normalize('NFKC');
}

/**
 * The letters a list lookup compares.
 *
 * Case is folded and the obvious substitutions are undone, because `P@ssw0rd` is `password` to
 * everybody except a naive comparison - which is what a blocklist that missed it would be.
 */
export function flatten(value: string): string {
  const substitutions: Record<string, string> = { '0': 'o', '1': 'l', '3': 'e', '4': 'a', '5': 's', '7': 't', '@': 'a', $: 's', '!': 'i' };
  return [...normalise(value).toLowerCase()].map((character) => substitutions[character] ?? character).join('');
}

const isLower = (character: string) => /\p{Ll}/u.test(character);
const isUpper = (character: string) => /\p{Lu}/u.test(character);
const isDigit = (character: string) => /\p{Nd}/u.test(character);

/** Counts by Unicode category, so a Greek letter is a letter and an emoji is "other". */
export function classesOf(password: string): { lower: number; upper: number; digits: number; symbols: number; classes: number } {
  let lower = 0;
  let upper = 0;
  let digits = 0;
  let symbols = 0;
  for (const character of normalise(password)) {
    if (isLower(character)) lower += 1;
    else if (isUpper(character)) upper += 1;
    else if (isDigit(character)) digits += 1;
    else symbols += 1;
  }
  const classes = [lower, upper, digits, symbols].filter((count) => count > 0).length;
  return { lower, upper, digits, symbols, classes };
}

/** The longest run of one character. Counted in code points, so an emoji is one. */
export function longestRun(password: string): number {
  let longest = 0;
  let run = 0;
  let previous: string | undefined;
  for (const character of normalise(password)) {
    run = character === previous ? run + 1 : 1;
    previous = character;
    if (run > longest) longest = run;
  }
  return longest;
}

/** The words this password may not contain, from who is setting it and where. */
export function contextWords(context: PasswordContext): string[] {
  const words: string[] = ['hubtask'];
  const local = context.email?.split('@')[0];
  if (local && local.length >= 3) words.push(local);
  if (context.email) words.push(context.email);
  for (const part of (context.displayName ?? '').split(/\s+/)) {
    if (part.length >= 4) words.push(part);
  }
  for (const part of (context.workspaceName ?? '').split(/\s+/)) {
    if (part.length >= 4) words.push(part);
  }
  const label = context.workspaceHost?.split('.')[0];
  if (label && label.length >= 4) words.push(label);
  return words.map(flatten).filter((word) => word.length >= 3);
}

/** Whether the password carries one of them. */
export function carriesContextWord(password: string, context: PasswordContext): boolean {
  const flat = flatten(password);
  return contextWords(context).some((word) => flat.includes(word));
}

/**
 * Every rule that is on, in the order they are checked.
 *
 * The local ones first and the server's as a block at the end: the list then tells a reader where
 * they are - what is already decided, and what is still out. An empty password leaves every line
 * `unmet` rather than `failed`: nothing has been refused yet, and red before the first keystroke
 * is a screen that starts by telling somebody off.
 */
export function evaluate(
  rules: PasswordRules,
  password: string,
  context: PasswordContext,
  server: {
    /** Which server-side rules this password has been refused by, as rule ids. */
    readonly violations?: readonly string[];
    /** Whether an answer for *this* password is in hand. */
    readonly isAnswered?: boolean;
    readonly isChecking?: boolean;
    readonly isUnreachable?: boolean;
  } = {},
): RuleLine[] {
  const typed = normalise(password);
  const counts = classesOf(typed);
  const lines: RuleLine[] = [];

  const local = (id: string, code: string, params: Record<string, string>, holds: boolean) => {
    lines.push({ id, code, params, isServerSide: false, state: typed === '' ? 'unmet' : holds ? 'met' : 'unmet' });
  };

  local('min_length', `${CODE}.min_length`, { minimum: String(rules.min_length) }, [...typed].length >= rules.min_length);
  if (rules.min_lowercase > 0) local('min_lowercase', `${CODE}.min_lowercase`, { count: String(rules.min_lowercase) }, counts.lower >= rules.min_lowercase);
  if (rules.min_uppercase > 0) local('min_uppercase', `${CODE}.min_uppercase`, { count: String(rules.min_uppercase) }, counts.upper >= rules.min_uppercase);
  if (rules.min_digits > 0) local('min_digits', `${CODE}.min_digits`, { count: String(rules.min_digits) }, counts.digits >= rules.min_digits);
  if (rules.min_symbols > 0) local('min_symbols', `${CODE}.min_symbols`, { count: String(rules.min_symbols) }, counts.symbols >= rules.min_symbols);
  if (rules.min_classes > 0) local('min_classes', `${CODE}.min_classes`, { count: String(rules.min_classes) }, counts.classes >= rules.min_classes);
  if (rules.max_repeat !== null) local('max_repeat', `${CODE}.max_repeat`, { count: String(rules.max_repeat) }, longestRun(typed) <= rules.max_repeat);
  if (rules.context_words) local('context_words', `${CODE}.context_words`, {}, !carriesContextWord(typed, context));

  const localHolds = lines.every((line) => line.state === 'met');
  const stateOfServerRule = (id: string): RuleState => {
    if (server.violations?.includes(id)) return 'failed';
    if (server.isUnreachable) return 'unchecked';
    if (server.isChecking) return 'checking';
    if (server.isAnswered) return 'met';
    return localHolds && typed !== '' ? 'checking' : 'server';
  };
  const remote = (id: string, code: string, params: Record<string, string> = {}) => {
    lines.push({ id, code, params, isServerSide: true, state: stateOfServerRule(id) });
  };

  // One line for both lists: which of the two a password was found in is not a reader's business,
  // and saying would tell a guesser which corpus to avoid.
  if (rules.common_passwords) remote('common', `${CODE}.common`);
  if (rules.breach_check) remote('breach', `${CODE}.breach`);
  if (rules.history_count > 0) remote('history', `${CODE}.history`, { count: String(rules.history_count) });
  if (rules.not_current) remote('not_current', `${CODE}.not_current`);

  return lines;
}

/** Whether every local rule holds - what decides that the server is worth asking. */
export function localRulesHold(rules: PasswordRules, password: string, context: PasswordContext): boolean {
  if (normalise(password) === '') return false;
  return evaluate(rules, password, context).filter((line) => !line.isServerSide).every((line) => line.state === 'met');
}

/** How many lines are not met - what the live region says once, rather than line by line. */
export function unmetCount(lines: readonly RuleLine[]): number {
  return lines.filter((line) => line.state === 'failed' || line.state === 'unmet').length;
}

/**
 * The rules an installation serves before anything is read.
 *
 * Not a guess at the policy: the least a screen can draw while the real answer is in flight, so
 * the list does not appear a moment after the field. It is replaced the instant the read lands.
 */
export const RULES_BEFORE_READING: PasswordRules = {
  min_length: 12,
  min_lowercase: 0,
  min_uppercase: 0,
  min_digits: 0,
  min_symbols: 0,
  min_classes: 0,
  max_repeat: null,
  common_passwords: true,
  context_words: true,
  breach_check: false,
  history_count: 0,
  not_current: false,
};
