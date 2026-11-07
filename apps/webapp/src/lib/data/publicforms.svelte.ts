// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A collection's public form (UC-SVC-13): its link made and taken away by who manages the
 * collection's members, the workspace's switch, and the form opened and sent by anybody holding
 * the link - without signing in.
 */

import type { PublicFormMinted, PublicLinksPolicy } from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { cataloguePath } from './portal.svelte.ts';
import type { submissionOf } from './publicforms.ts';

export const publicFormsPolicyPath = '/public-forms:policy';
export const publicFormPath = (collectionId: string) => `/collections/${collectionId}/public-form`;
export const openFormPath = (token: string) => `/public/forms/${encodeURIComponent(token)}`;

export const publicForms = {
  /** Makes the collection's public form, replacing its link, and answers the token - once. */
  make(collectionId: string): Promise<PublicFormMinted> {
    return engine.mutate<PublicFormMinted>('POST', publicFormPath(collectionId), undefined, {
      invalidates: [publicFormPath(collectionId), cataloguePath(collectionId)],
    });
  },

  remove(collectionId: string): Promise<unknown> {
    return engine.mutate<unknown>('DELETE', publicFormPath(collectionId), undefined, { invalidates: [publicFormPath(collectionId), cataloguePath(collectionId)] });
  },

  /** Allows or forbids public forms in the workspace, inside what the plan leaves. */
  setAllowed(allowed: boolean): Promise<PublicLinksPolicy> {
    return engine.mutate<PublicLinksPolicy>('PUT', publicFormsPolicyPath, { allowed }, { invalidates: [publicFormsPolicyPath] });
  },

  /** Sends a request through a public form, with the answered challenge. */
  submit(token: string, body: ReturnType<typeof submissionOf> & { challenge: string; solution: string }): Promise<unknown> {
    return engine.mutate<unknown>('POST', openFormPath(token), body);
  },
};
