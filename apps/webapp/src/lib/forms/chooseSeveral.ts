// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The words `MultiSelect` asks of every caller, from the one set of shared codes
 * (`app.choose_several.*`), so that "Nothing chosen", the filter and *All* / *None* read the same
 * on every screen that chooses several of many. Spread into the component where it is drawn, so a
 * change of language reaches it.
 */

import { multiSelectPresentationOf } from '@hubtask/design-system/components';

import { messages, t } from '../i18n/i18n.svelte.ts';

/**
 * Which presentation a list takes, so that no place flips between two looks as data grows or by
 * who opens it (design-system.md §4): a list that comes from data - people, groups, roles,
 * statuses, entry types, hubs, restricted fields, workflow steps - is always the field, also for
 * two; a fixed catalogue goes by its full size, not by how many of it this reader is offered.
 */
export const FROM_DATA = 'field' as const;
export const byCatalogue = (size: number) => multiSelectPresentationOf('auto', size);

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
