// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { t } from '../i18n/i18n.svelte.ts';

/** One step of a trail, as `PageHeader`'s breadcrumb takes it. */
interface Step {
  readonly id: string;
  readonly label: string;
  readonly href?: string;
}

/**
 * The trail of an installation screen: *Installation › the screens above › this one*, its first
 * step leading to the installation's overview (design-system.md §11.5, every section screen
 * carries one). The overview's own trail is the one step, unlinked.
 */
export function installationTrail(...steps: readonly Step[]) {
  const head: Step = steps.length === 0
    ? { id: 'instance', label: t('app.instance.nav') }
    : { id: 'instance', label: t('app.instance.nav'), href: '/instance' };
  return {
    trail: [head, ...steps],
    label: t('app.instance.trail'),
    expandLabel: t('app.admin.expand_trail'),
  };
}
