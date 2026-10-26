// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The mail intake (ADR-0094): a workspace's settings, its log, and a collection's handle - the
 * short name a subject (`#einkauf`), an address and the ticket keys (`EINKAUF-42`) use.
 *
 * **One intake, any door.** The address on the operator's receiving domain is answered with the
 * settings where the installation has one; a connected mailbox is the other door.
 */

import { engine } from './engine.ts';

const SETTINGS = '/mail/settings';

export type SenderLevel = 'MEMBERS' | 'MEMBERS_AND_ALLOWED' | 'ANYONE';

export type MailReason =
  | 'NO_ROUTE'
  | 'UNKNOWN_PLACE'
  | 'SENDER_NOT_ALLOWED'
  | 'NOT_PERMITTED'
  | 'REFUSED_BY_PLACE'
  | 'SPAM'
  | 'UNPARSEABLE'
  | 'SENDER_BLOCKED'
  | 'AUTOMATIC'
  | 'TOO_MANY';

export interface SenderRoute {
  readonly match: string;
  readonly collection_id: string;
}

export interface MailSettings {
  readonly enabled: boolean;
  readonly name?: string | null;
  readonly default_collection_id?: string | null;
  readonly senders: { readonly level: SenderLevel; readonly allowed: readonly string[]; readonly blocked: readonly string[] };
  readonly sender_routes: readonly SenderRoute[];
  /** The operator's receiving domain and the workspace's address there; absent without one. */
  readonly domain?: string | null;
  readonly address?: string | null;
}

export interface MailReceipt {
  readonly id: string;
  readonly received_at: string;
  readonly sender: string;
  readonly subject: string;
  readonly outcome: 'ENTRY' | 'COMMENT' | 'JUMBLE' | 'DISCARDED';
  readonly reason?: MailReason | null;
  readonly item_id?: string | null;
  readonly entry_id?: string | null;
  readonly door: string;
}

export const DEFAULT_MAIL_SETTINGS: MailSettings = {
  enabled: false,
  senders: { level: 'MEMBERS', allowed: [], blocked: [] },
  sender_routes: [],
};

export async function readMailSettings(): Promise<MailSettings | undefined> {
  const read = await engine.refresh<MailSettings>({ path: SETTINGS });
  if (read.status === 'failed') throw read.error;
  return read.status === 'ready' ? { ...DEFAULT_MAIL_SETTINGS, ...read.data } : undefined;
}

export async function saveMailSettings(settings: MailSettings): Promise<MailSettings> {
  const { domain: _domain, address: _address, ...body } = settings;
  return engine.mutate<MailSettings>('PUT', SETTINGS, body, { idempotencyKey: crypto.randomUUID() });
}

export async function readMailReceipts(): Promise<readonly MailReceipt[]> {
  const read = await engine.refresh<{ receipts: MailReceipt[] }>({ path: '/mail/receipts' });
  if (read.status === 'failed') throw read.error;
  return read.status === 'ready' ? read.data.receipts : [];
}

/** The list a person types, one address or domain per line or comma, as the settings hold it. */
export function linesOf(text: string): string[] {
  return text.split(/[\n,;]+/).map((line) => line.trim()).filter((line) => line !== '');
}

/** A workspace name a domain takes, suggested from the workspace's own name: "Familie Meier" → "familie-meier". */
export function suggestedName(workspace: string): string {
  return workspace
    .toLowerCase()
    .replace(/ä/g, 'ae').replace(/ö/g, 'oe').replace(/ü/g, 'ue').replace(/ß/g, 'ss')
    .normalize('NFD').replace(/[̀-ͯ]/g, '')
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 40)
    .replace(/-+$/g, '');
}

/**
 * How mail reaches a collection on this set-up: the subject word that works through every door,
 * and the address where the receiving domain makes one (`familie-meier+einkauf@in.hubtask.app`).
 */
export function routesTo(handle: string, settings: MailSettings | undefined): { subject: string; address?: string } {
  const subject = `#${handle}`;
  if (!settings?.domain || !settings.name) return { subject };
  return { subject, address: `${settings.name}+${handle}@${settings.domain}` };
}

/**
 * Copies a collection's subject word, `#einkauf`, and answers the sentence to announce - or
 * undefined where the clipboard refused. The word works through every door the intake has.
 */
export async function copyHandle(
  handle: string,
  t: (code: string, params?: Record<string, string>) => string,
): Promise<string | undefined> {
  const subject = `#${handle}`;
  try {
    await navigator.clipboard.writeText(subject);
    return t('app.mail.copied_menu', { subject });
  } catch {
    return t('app.mail.copy_failed', { address: subject });
  }
}

export interface Mailbox {
  readonly id: string;
  readonly protocol: 'IMAP' | 'JMAP';
  readonly address: string;
  readonly host?: string | null;
  readonly port?: number | null;
  readonly session_url?: string | null;
  readonly username: string;
  readonly folder: string;
  readonly done_folder: string;
  readonly preset?: string | null;
  readonly plus_addresses?: boolean | null;
  readonly enabled: boolean;
  readonly checked_at?: string | null;
  readonly failure?: string | null;
  readonly failing: boolean;
  readonly created_at: string;
  readonly auth: 'PASSWORD' | 'OAUTH';
  readonly oauth_provider?: string | null;
}

/** A provider a mailbox connects to through its own sign-in, and whether the installation brings an app. */
export interface MailboxOAuthProvider {
  readonly key: string;
  readonly installation: boolean;
  readonly hint: string;
}

/** What starts a connection through a provider's sign-in: a preset, or any provider by its endpoints. */
export interface MailboxAuthorizationRequest {
  provider: string;
  address?: string;
  client_id?: string;
  client_secret?: string;
  authorize_url?: string;
  token_url?: string;
  scopes?: string[];
  host?: string;
  port?: number;
}

export interface MailboxPreset {
  readonly key: string;
  readonly protocol: 'IMAP' | 'JMAP';
  readonly plus_addresses: boolean;
  readonly hint: string;
}

export interface MailboxConnection {
  preset?: string;
  protocol?: 'IMAP' | 'JMAP';
  address: string;
  host?: string;
  port?: number;
  session_url?: string;
  username?: string;
  secret: string;
}

export interface MailboxesRead {
  readonly mailboxes: readonly Mailbox[];
  readonly presets: readonly MailboxPreset[];
  readonly oauth_providers?: readonly MailboxOAuthProvider[];
  readonly oauth_redirect_url?: string;
}

export async function readMailboxes(): Promise<MailboxesRead> {
  const read = await engine.refresh<MailboxesRead>({ path: '/mail/mailboxes' });
  if (read.status === 'failed') throw read.error;
  return read.status === 'ready' ? read.data : { mailboxes: [], presets: [] };
}

/** Starts connecting through the provider's sign-in; answers the provider's page to go to. */
export async function authorizeMailbox(request: MailboxAuthorizationRequest): Promise<string> {
  const answer = await engine.mutate<{ authorize_url: string }>('POST', '/mail/mailboxes:authorize', request, {
    idempotencyKey: crypto.randomUUID(),
  });
  return answer.authorize_url;
}

/** Finishes it with what the provider sent back. */
export async function completeMailboxAuthorization(answer: { state: string; code?: string; error?: string }): Promise<Mailbox> {
  return engine.mutate<Mailbox>('POST', '/mail/mailboxes:complete', answer, { idempotencyKey: crypto.randomUUID() });
}

export async function connectMailbox(connection: MailboxConnection): Promise<Mailbox> {
  return engine.mutate<Mailbox>('POST', '/mail/mailboxes', connection, { idempotencyKey: crypto.randomUUID() });
}

export async function checkMailbox(id: string): Promise<Mailbox> {
  return engine.mutate<Mailbox>('POST', `/mail/mailboxes/${encodeURIComponent(id)}:check`, {}, { idempotencyKey: crypto.randomUUID() });
}

export async function removeMailbox(id: string): Promise<void> {
  await engine.mutate<void>('DELETE', `/mail/mailboxes/${encodeURIComponent(id)}`, undefined);
}

/**
 * How mail reaches a collection through a connected mailbox: `familie+einkauf@gmail.com` where the
 * provider delivers plus addresses, else the mailbox's own address with `#einkauf` in the subject.
 */
export function mailboxRouteTo(handle: string, mailbox: Mailbox): string {
  const [local, domain] = mailbox.address.split('@');
  return mailbox.plus_addresses && local && domain ? `${local}+${handle}@${domain}` : mailbox.address;
}
