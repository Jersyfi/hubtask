// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The agents' trail: at the product's home `/agents` under *Products*, at the reader's own
// `/profile/agents` under *Your settings* (page-system §9.5) - one shape for the list, the
// settings and an agent's record page.

import { t } from '../i18n/i18n.svelte.ts';

export const AGENTS_HOME = '/agents';

export function agentsTrail(base: string, here?: string) {
  const isHome = base === AGENTS_HOME;
  const agents = { id: 'agents', label: t('app.agents.title'), ...(here ? { href: base } : {}) };
  return {
    trail: [
      isHome ? { id: 'products', label: t('app.nav.products') } : { id: 'settings', label: t('app.nav.profile'), href: '/profile' },
      agents,
      ...(here ? [{ id: 'here', label: here }] : []),
    ],
    label: isHome ? t('app.product_home.trail') : t('app.you.trail'),
    expandLabel: isHome ? t('app.admin.expand_trail') : t('app.you.expand_trail'),
  };
}
