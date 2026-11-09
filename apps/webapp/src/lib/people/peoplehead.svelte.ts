// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The head every one of *People*'s list pages draws: the title, the trail, the one-line figure. */

import type { PageAction } from '@hubtask/design-system/components';

import { groups } from '../data/groups.svelte.ts';
import { people } from '../data/people.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';

const TENANT = { scopeType: 'TENANT' } as const;

/** *9 people · 3 groups*, from what is loaded; nothing until the memberships arrived. */
export function peopleFigure(): string | undefined {
  const holders = people.holders({}, TENANT);
  if (holders.length === 0) return undefined;
  return t('app.people.figure', { people: String(holders.filter((holder) => holder.accountId).length), groups: String(groups.all.length) });
}

export function peopleHead(primary?: PageAction, secondary?: PageAction) {
  return {
    title: t('app.people.title'),
    subtitle: peopleFigure(),
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'people', label: t('app.people.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    primary,
    secondary: secondary ? ([secondary] as const) : ([] as const),
  };
}
