// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A hub's pages: the paths the *Pages* screen reads, and the writes, so every write invalidates the
 * same set (UC-DOC-01 … UC-DOC-12). The reads are `resource()`s at the call sites.
 *
 * **A body write is not an edit of a form.** While writers edit together the live document is
 * the page; the editor saves its Markdown beside it without `If-Match`, the write landing on what
 * stands (`updatePage`), so two writers' saves never refuse each other. A title, an icon and the
 * marks are written with the page's version, as any record.
 */

import type {
  Page, PageChange, PageComment, PageCommentCreate, PageCreate, PageExport, PageHitList, PageMove, PagePublicLinkMinted, PageShare, PublicLinksPolicy, PageTrashed, PageVersionNamed,
} from '@hubtask/sync-engine';

import { engine } from './engine.ts';
import { livePath } from './pages.ts';

export * from './pages.ts';

export const pagesPath = (hubId: string) => `/hubs/${hubId}/pages`;
export const templatesPath = (hubId: string) => `/hubs/${hubId}/pages?templates=true`;
export const trashedPagesPath = (hubId: string) => `/hubs/${hubId}/trashed-pages`;
export const pagePath = (pageId: string) => `/pages/${pageId}`;
export const versionsPath = (pageId: string) => `/pages/${pageId}/versions`;
export const versionPath = (pageId: string, seq: number) => `/pages/${pageId}/versions/${seq}`;
export const pageCommentsPath = (pageId: string) => `/pages/${pageId}/comments`;
export const backlinksPath = (pageId: string) => `/pages/${pageId}/backlinks`;
export const accessPath = (pageId: string) => `/pages/${pageId}/access`;
export const sharedPagesPath = '/pages:shared';
export const personalSpacePath = '/pages:personal';
export const publicLinksPolicyPath = '/pages:public-links';

/** What a write to one page makes stale: the page, its space, and its history. */
const touched = (hubId: string, pageId: string) => [pagePath(pageId), `/hubs/${hubId}/pages`, `/hubs/${hubId}/trashed-pages`, versionsPath(pageId)];

class Pages {
  create(hubId: string, body: PageCreate): Promise<Page> {
    return engine.mutate<Page>('POST', pagesPath(hubId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [`/hubs/${hubId}/pages`],
    });
  }

  /** A title, an icon, a mark or the language, with the page's version. */
  change(page: Page, change: PageChange): Promise<Page> {
    return engine.mutate<Page>('PATCH', pagePath(page.id), change, {
      ifMatch: `"${page.version}"`,
      invalidates: touched(page.hub_id, page.id),
    });
  }

  /**
   * The editor's Markdown, saved beside the live document. It invalidates the history and the
   * space only: the page itself is not read again, or the screen would draw a body over the one
   * being typed.
   */
  saveBody(page: Page, body: string): Promise<Page> {
    return engine.mutate<Page>('PATCH', pagePath(page.id), { body }, {
      invalidates: [versionsPath(page.id), backlinksPath(page.id)],
    });
  }

  move(page: Page, to: PageMove): Promise<Page> {
    return engine.mutate<Page>('POST', `${pagePath(page.id)}:move`, to, {
      invalidates: touched(page.hub_id, page.id),
    });
  }

  trash(page: Page): Promise<PageTrashed> {
    return engine.mutate<PageTrashed>('DELETE', pagePath(page.id), undefined, {
      invalidates: touched(page.hub_id, page.id),
    });
  }

  restore(hubId: string, pageId: string): Promise<Page> {
    return engine.mutate<Page>('POST', `${pagePath(pageId)}:restore`, {}, {
      invalidates: touched(hubId, pageId),
    });
  }

  /** Verifies the page until a moment ahead, or takes the verification away (UC-DOC-12). */
  verify(page: Page, until: string | undefined, ownerId?: string): Promise<Page> {
    return engine.mutate<Page>('POST', `${pagePath(page.id)}:verify`, {
      ...(until ? { verified_until: until } : {}),
      ...(ownerId ? { owner_id: ownerId } : {}),
    }, { invalidates: touched(page.hub_id, page.id) });
  }

  nameVersion(page: Page, seq: number, name: string): Promise<PageVersionNamed> {
    return engine.mutate<PageVersionNamed>('PATCH', versionPath(page.id, seq), { name }, {
      invalidates: [versionsPath(page.id)],
    });
  }

  restoreVersion(page: Page, seq: number): Promise<Page> {
    return engine.mutate<Page>('POST', `${versionPath(page.id, seq)}:restore`, {}, {
      invalidates: touched(page.hub_id, page.id),
    });
  }

  addComment(pageId: string, body: PageCommentCreate): Promise<PageComment> {
    return engine.mutate<PageComment>('POST', pageCommentsPath(pageId), body, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [pageCommentsPath(pageId)],
    });
  }

  changeComment(comment: PageComment, change: { body?: string; resolved?: boolean }): Promise<PageComment> {
    return engine.mutate<PageComment>('PATCH', `/page-comments/${comment.id}`, change, {
      ifMatch: `"${comment.version}"`,
      invalidates: [pageCommentsPath(comment.page_id)],
    });
  }

  deleteComment(comment: PageComment): Promise<unknown> {
    return engine.mutate<unknown>('DELETE', `/page-comments/${comment.id}`, undefined, {
      invalidates: [pageCommentsPath(comment.page_id)],
    });
  }

  /** The page, or it and every page under it, as files of one format. A read: it invalidates nothing. */
  export(pageId: string, subtree: boolean, format: PageExport['format']): Promise<PageExport> {
    return engine.mutate<PageExport>('POST', `${pagePath(pageId)}:export`, { subtree, format }, { invalidates: [] });
  }

  /** The pages that match, over every space the reader may read or one hub's. A read. */
  async search(text: string, hubId?: string, limit = 20): Promise<PageHitList['data']> {
    const state = await engine.refresh<PageHitList>({ path: '/pages:search', body: { text, limit, ...(hubId ? { hub_id: hubId } : {}) } });
    if (state.status === 'failed') throw state.error;
    return state.status === 'ready' ? state.data.data : [];
  }

  /**
   * Restricts who of the hub's readers may read the page to the people named - the reader among
   * them - or, without anybody, lifts the page's own restriction (UC-DOC-07).
   */
  restrict(page: Page, accountIds: readonly string[] | undefined): Promise<Page> {
    return engine.mutate<Page>('POST', `${pagePath(page.id)}:restrict`, accountIds ? { account_ids: [...accountIds] } : { lift: true }, {
      invalidates: [...touched(page.hub_id, page.id), accessPath(page.id)],
    });
  }

  /** Shares the page read-only with one person beside its hub (UC-DOC-08). */
  share(pageId: string, accountId: string): Promise<PageShare> {
    return engine.mutate<PageShare>('POST', `${pagePath(pageId)}/shares`, { account_id: accountId }, {
      idempotencyKey: crypto.randomUUID(),
      invalidates: [accessPath(pageId)],
    });
  }

  unshare(pageId: string, accountId: string): Promise<unknown> {
    return engine.mutate<unknown>('DELETE', `${pagePath(pageId)}/shares/${accountId}`, undefined, {
      invalidates: [accessPath(pageId), sharedPagesPath],
    });
  }

  /**
   * Makes the page's public link, replacing the one it had, and answers its token - once
   * (UC-DOC-08). Only where the workspace allows public links.
   */
  link(pageId: string): Promise<PagePublicLinkMinted> {
    return engine.mutate<PagePublicLinkMinted>('POST', `${pagePath(pageId)}/public-link`, undefined, {
      invalidates: [accessPath(pageId)],
    });
  }

  unlink(pageId: string): Promise<unknown> {
    return engine.mutate<unknown>('DELETE', `${pagePath(pageId)}/public-link`, undefined, { invalidates: [accessPath(pageId)] });
  }

  /** Allows or forbids public links in the workspace, inside what the plan leaves. */
  setPublicLinks(allowed: boolean): Promise<PublicLinksPolicy> {
    return engine.mutate<PublicLinksPolicy>('PUT', publicLinksPolicyPath, { allowed }, { invalidates: [publicLinksPolicyPath] });
  }

  /** Today's note in the reader's personal space, made with the space on first use (UC-DOC-07). */
  today(): Promise<Page> {
    return engine.mutate<Page>('POST', '/pages:today', {}, { invalidates: [personalSpacePath] });
  }

  /** The page's live document socket; the editor asks for a new one on every reconnect. */
  liveSocket(pageId: string): WebSocket {
    return engine.liveSocket<WebSocket>(livePath(pageId));
  }
}

export const pages = new Pages();
