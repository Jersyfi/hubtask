// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The language a name the client stores from a default text is written in - "Untitled page", a
 * shipped template's title.
 *
 * Not the interface's: a stored name is read by everybody in the workspace, so it follows the
 * account's own language, else the workspace's default, never the browser at hand
 * (`i18n-l10n.md` §2, the server's `ActorContext.NamingLocale`). The interface's locale answers
 * only while neither is known yet. Pure, so the order is tested in plain Node.
 */

import { SOURCE_LOCALE, type Catalogue } from './catalogue.ts';
import { resolveLocale, type SupportedLocale } from './locale.ts';
import { createMessages, type Messages } from './messages.ts';

export interface NamingSources {
  /** `account.locale`, where the account set one. */
  readonly account?: string | null;
  /** The workspace's `default_locale`, once it is read. */
  readonly workspace?: string | null;
}

/** The supported locale a stored name is rendered in: the account's, else the workspace's. */
export function namingLocale(sources: NamingSources, supported: readonly SupportedLocale[], interfaceLocale: string): string {
  const chosen = sources.account?.trim() || sources.workspace?.trim();
  return chosen ? resolveLocale({ account: chosen }, supported) : interfaceLocale;
}

/**
 * The messages of a naming locale, read through the one catalogue loader, the source under the
 * translation as the interface's chain has it. A catalogue that does not arrive leaves the source.
 */
export async function namingMessages(
  locale: string,
  load: (locale: string) => Promise<Catalogue | undefined>,
): Promise<Messages> {
  const quietly = (tag: string) => load(tag).catch(() => undefined);
  const [own, source] = await Promise.all([locale === SOURCE_LOCALE ? undefined : quietly(locale), quietly(SOURCE_LOCALE)]);
  const catalogues = [own, source].filter((catalogue): catalogue is Catalogue => catalogue !== undefined);
  return createMessages({ locale, catalogues });
}
