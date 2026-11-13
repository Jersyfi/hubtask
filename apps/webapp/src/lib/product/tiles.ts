// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import type { ProductId } from '@hubtask/design-system/components';

/** Each product module's tile (work-surface scene empty): the mark its first empty state draws. */
export const PRODUCT_TILES: Readonly<Record<string, ProductId>> = {
  PLANNING: 'plans', KNOWLEDGE: 'pages', WHITEBOARD: 'whiteboards', SERVICE_DESK: 'desk', ASSETS: 'assets', TIME: 'time', GOALS: 'goals', AGENTS: 'agents',
};
