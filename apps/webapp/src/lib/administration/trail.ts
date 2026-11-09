// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The trail of an administration page's head (page-system §7): *Administration › {screen}*, and
 * further steps where the page is one object of a list.
 */

import { t } from '../i18n/i18n.svelte.ts';

interface Step {
  readonly id: string;
  readonly label: string;
  readonly href?: string;
}

export function adminTrail(...steps: readonly Step[]) {
  return {
    trail: [{ id: 'administration', label: t('app.admin.title'), href: '/administration' }, ...steps],
    label: t('app.admin.trail'),
    expandLabel: t('app.admin.expand_trail'),
  };
}
