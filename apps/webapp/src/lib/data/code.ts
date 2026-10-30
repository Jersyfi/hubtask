// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The code on an entry (UC-INT-13): the paths of a hub's forge hooks and an entry's code links,
// the address a forge posts to, and how a link reads - pure, so that it is tested without a server.

import type { CodeHook, CodeLink } from '@hubtask/sync-engine';

export const codeHooksPath = (hubId: string) => `/hubs/${hubId}/code-hooks`;
export const codeHookPath = (hookId: string) => `/code-hooks/${hookId}`;
export const codeHookSecretPath = (hookId: string) => `/code-hooks/${hookId}/secret`;
export const codeLinksPath = (itemId: string) => `/items/${itemId}/code-links`;

/** The forges a hook may be for, in the order the form offers them. */
export const FORGES: readonly CodeHook['kind'][] = ['GITHUB', 'GITLAB', 'FORGEJO'];

/** The address a forge's webhook posts to. */
export function codeIntakeAddress(origin: string, tenantId: string, hookId: string): string {
  return `${origin.replace(/\/+$/u, '')}/api/v1/code-intake/${tenantId}/${hookId}`;
}

/** How a link is named on its row: a commit by its short hash, a pull request by its number. */
export function codeRef(link: Pick<CodeLink, 'kind' | 'ref'>): string {
  if (link.kind === 'COMMIT') return link.ref.slice(0, 8);
  if (link.kind === 'PULL_REQUEST') return `#${link.ref}`;
  return link.ref;
}

/** A link's state as a badge's tone: merged succeeds, closed is over, open and pushed are neutral. */
export function codeTone(state: CodeLink['state']): 'success' | 'neutral' | 'info' {
  if (state === 'MERGED') return 'success';
  if (state === 'OPEN') return 'info';
  return 'neutral';
}

/** The address a link opens, only an http or https one - a forge's payload is not trusted further. */
export function codeHref(url: string): string | undefined {
  try {
    const parsed = new URL(url);
    return parsed.protocol === 'https:' || parsed.protocol === 'http:' ? parsed.href : undefined;
  } catch {
    return undefined;
  }
}

/** The pull requests first, then branches, then commits - each the latest first, as the server orders. */
export function codeGroups(links: readonly CodeLink[]): { kind: CodeLink['kind']; links: CodeLink[] }[] {
  const order: CodeLink['kind'][] = ['PULL_REQUEST', 'BRANCH', 'COMMIT'];
  return order
    .map((kind) => ({ kind, links: links.filter((each) => each.kind === kind) }))
    .filter((group) => group.links.length > 0);
}
