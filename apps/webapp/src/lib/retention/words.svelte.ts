// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The retention screen's words: a class of data and an action, never the constant behind them. */

import { messages, t } from '../i18n/i18n.svelte.ts';

/** What a rule does, in words; an action this client has no words for yet, as its constant. */
export const actionWord = (action: string): string =>
  messages.has(`app.retention.action_${action.toLowerCase()}`) ? t(`app.retention.action_${action.toLowerCase()}`) : action;

/**
 * A class of data in words. A class the catalogue gained before this client learnt its words is
 * shown by its constant rather than not at all; the locale test holds every class the server names
 * to its words.
 */
export const kindWord = (kind: string | null | undefined): string =>
  kind && messages.has(`app.retention.kind.${kind}`) ? t(`app.retention.kind.${kind}`) : (kind ?? '');
