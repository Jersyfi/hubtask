// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product home's frame for the tabs drawn by a view of their own (the navigation concept §4):
 * the head and the `ViewBar`'s views, the same on every tab, so *Reports*, *Classes* or *Settings*
 * keep the way to their siblings. A tab a reader may not open is left out (P-05).
 */

import type { PageHead, ViewLink } from '@hubtask/design-system/components';

import { holdsHere, reachesAdministrationArea } from '../data/capability.svelte.ts';
import { t } from '../i18n/i18n.svelte.ts';
import { agentsTrail, AGENTS_HOME } from '../agents/trail.ts';
import { homeTabs } from './home.ts';

const NAME: Readonly<Record<string, string>> = {
  SERVICE_DESK: 'app.nav.desk',
  ASSETS: 'app.assets.title',
  TIME: 'app.time.title',
  AGENTS: 'app.agents.title',
};

/** Whether the reader may open a home's tab: the same rule the home itself draws by. */
function mayOpen(module: string, id: string): boolean {
  if (module === 'SERVICE_DESK') return id !== 'reports' || holdsHere('STRUCTURE') === true;
  if (module === 'ASSETS') return id === 'inventory' || reachesAdministrationArea() === true;
  if (module === 'TIME') {
    return id !== 'settings'
      || ['STRUCTURE', 'APPROVE_TIME', 'MANAGE_COSTS', 'MANAGE_MEMBERS'].some((each) => holdsHere(each as never) === true);
  }
  if (module === 'AGENTS') return id !== 'settings' || holdsHere('MANAGE_MEMBERS') === true;
  return true;
}

export function productViews(module: string): ViewLink[] {
  return homeTabs(module).filter((each) => mayOpen(module, each.id)).map((each) => ({ id: each.id, label: t(each.code), href: each.path }));
}

export function productName(module: string): string {
  return t(NAME[module] ?? 'app.nav.products');
}

/** The home's head without its create act: a setup tab offers none (§5.4). */
export function productHead(module: string): PageHead {
  const title = productName(module);
  if (module === 'AGENTS') return { title, subtitle: t('app.agents.intro'), breadcrumb: agentsTrail(AGENTS_HOME) };
  return {
    title,
    subtitle: t(`app.modules.hint_${module.toLowerCase()}`),
    breadcrumb: {
      trail: [
        { id: 'products', label: t('app.nav.products') },
        { id: 'product', label: title },
      ],
      label: t('app.product_home.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
  };
}
