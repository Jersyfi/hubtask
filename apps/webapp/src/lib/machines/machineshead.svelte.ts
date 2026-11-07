// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/** The head both of *Machines and apps*' list pages draw: the title, the trail, the figure. */

import type { Snippet } from 'svelte';
import type { PageAction } from '@hubtask/design-system/components';

import { t } from '../i18n/i18n.svelte.ts';

export function machinesHead(primary: PageAction | undefined, figure: Snippet) {
  return {
    title: t('app.machines.title'),
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'machines', label: t('app.machines.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    primary,
    figure,
  };
}
