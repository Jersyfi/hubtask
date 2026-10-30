// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The requester portal's paths and writes (UC-SVC-05, UC-SVC-07, UC-DOC-13): a service's
 * catalogue, the reader's own requests and the articles the service publishes. Every write
 * invalidates the same set the screens read.
 */

import type { ArticleRating, PortalRequest, PortalRequestDetail, RequestSatisfaction } from '@hubtask/sync-engine';

import { engine } from './engine.ts';

const query = (collectionId: string) => `collection_id=${encodeURIComponent(collectionId)}`;

export const cataloguePath = (collectionId: string) => `/request-catalogue?${query(collectionId)}`;
export const myRequestsPath = (collectionId: string) => `/requests?${query(collectionId)}`;
export const myRequestPath = (itemId: string) => `/requests/${itemId}`;
export const articlePath = (collectionId: string, pageId: string) => `/articles/${pageId}?${query(collectionId)}`;
export function suggestionsPath(collectionId: string, text: string): string {
  return text === '' ? `/articles?${query(collectionId)}` : `/articles?${query(collectionId)}&text=${encodeURIComponent(text)}`;
}

export interface RequestDraft {
  readonly entry_type: string;
  readonly title: string;
  readonly notes?: string;
  readonly custom_fields?: Record<string, unknown>;
}

class Portal {
  raise(collectionId: string, draft: RequestDraft): Promise<PortalRequest> {
    return engine.mutate<PortalRequest>('POST', '/requests', { collection_id: collectionId, ...draft }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [myRequestsPath(collectionId)],
    });
  }

  answer(request: PortalRequestDetail, body: string, parentCommentId?: string): Promise<unknown> {
    return engine.mutate('POST', `${myRequestPath(request.id)}/comments`, { body, parent_comment_id: parentCommentId ?? null }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [myRequestPath(request.id)],
    });
  }

  rate(request: PortalRequestDetail, score: number, comment: string): Promise<RequestSatisfaction> {
    const body = comment.trim() === '' ? { score } : { score, comment: comment.trim() };
    return engine.mutate<RequestSatisfaction>('PUT', `${myRequestPath(request.id)}/satisfaction`, body, {
      invalidates: [myRequestPath(request.id)],
    });
  }

  rateArticle(collectionId: string, pageId: string, helpful: boolean): Promise<ArticleRating> {
    return engine.mutate<ArticleRating>('POST', `/articles/${pageId}:rate`, { collection_id: collectionId, helpful }, {
      invalidates: [],
    });
  }
}

export const portal = new Portal();
