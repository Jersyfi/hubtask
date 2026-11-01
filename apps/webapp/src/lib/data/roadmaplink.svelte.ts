// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A portfolio roadmap's public link (UC-GOL-03 check 15): made and taken away by who manages the
 * workspace's members, and the workspace's switch that allows it.
 */

import type { PortfolioPublicLinkMinted, PublicLinksPolicy } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

export const publicRoadmapsPolicyPath = '/portfolios:public-links';
export const roadmapLinkPath = (portfolioId: string) => `/portfolios/${portfolioId}/public-link`;

export const roadmapLinks = {
  /** Makes the roadmap's public link, replacing the previous one, and answers the token - once. */
  make(portfolioId: string): Promise<PortfolioPublicLinkMinted> {
    return engine.mutate<PortfolioPublicLinkMinted>('POST', roadmapLinkPath(portfolioId), undefined, {
      invalidates: [roadmapLinkPath(portfolioId)],
    });
  },

  remove(portfolioId: string): Promise<unknown> {
    return engine.mutate<unknown>('DELETE', roadmapLinkPath(portfolioId), undefined, { invalidates: [roadmapLinkPath(portfolioId)] });
  },

  /** Allows or forbids public roadmaps in the workspace, inside what the plan leaves. */
  setAllowed(allowed: boolean): Promise<PublicLinksPolicy> {
    return engine.mutate<PublicLinksPolicy>('PUT', publicRoadmapsPolicyPath, { allowed }, { invalidates: [publicRoadmapsPolicyPath] });
  },
};
