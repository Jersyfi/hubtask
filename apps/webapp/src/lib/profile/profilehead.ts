// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The head every page of Your settings draws (page-system §9.5): the title is the column's row's
 * own heading, the trail *Your settings › this screen* - read from the same row, so the column and
 * the page cannot disagree about what a screen is called.
 */

import type { PageHead } from '@hubtask/design-system/components';

import { t } from '../i18n/i18n.svelte.ts';
import { SETTINGS } from '../navigation.ts';

/** The heading of a row of Your settings' column, by its id. */
export function profileTitle(row: string): string {
  const found = SETTINGS.flatMap((group) => group.rows).find((each) => each.id === row);
  return t(found?.titleCode ?? found?.code ?? 'app.nav.profile');
}

/** The trail to a row's screen, and one step further where a page lies under it (*New*). */
export function profileTrail(row: string, under?: string): NonNullable<PageHead['breadcrumb']> {
  const title = profileTitle(row);
  return {
    trail: [
      { id: 'settings', label: t('app.nav.profile'), href: '/profile' },
      under === undefined
        ? { id: row, label: title }
        : { id: row, label: title, href: SETTINGS.flatMap((group) => group.rows).find((each) => each.id === row)?.path },
      ...(under === undefined ? [] : [{ id: `${row}-under`, label: under }]),
    ],
    label: t('app.you.trail'),
    expandLabel: t('app.you.expand_trail'),
  };
}

export function profileHead(row: string, extra: Omit<PageHead, 'title' | 'breadcrumb'> = {}): PageHead {
  return { title: profileTitle(row), breadcrumb: profileTrail(row), ...extra };
}

