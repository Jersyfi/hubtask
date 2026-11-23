// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The words `MultiSelect` asks of every caller, from the one set of shared codes
 * (`app.choose_several.*`), so that "Nothing chosen", the filter and *All* / *None* read the same
 * on every screen that chooses several of many. Spread into the component where it is drawn, so a
 * change of language reaches it.
 */

import { messages, t } from '../i18n/i18n.svelte.ts';

export function chooseSeveral() {
  return {
    requiredLabel: t('app.choose_several.required'),
    emptyLabel: t('app.choose_several.empty'),
    filterLabel: t('app.choose_several.filter'),
    clearLabel: t('app.choose_several.clear'),
    noMatchLabel: t('app.choose_several.no_match'),
    allLabel: t('app.choose_several.all'),
    noneLabel: t('app.choose_several.none'),
    doneLabel: t('app.choose_several.done'),
    countLabel: (chosen: number, total: number) => t('app.choose_several.count', { chosen, total }),
    locale: messages.locale,
  };
}
