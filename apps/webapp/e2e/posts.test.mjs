// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's posts in the web app (work-surface concept B-1 … B-6): the web app's half of every check
// of UC-DOC-17 and UC-DOC-18, the server answered by routes that keep what was written. Who sees which
// post, the moment a scheduled one goes out and who is told are the server's to judge, and proven
// there:
//
// - UC-DOC-17/2 (a scheduled post published at its moment, its writers' alone until then),
//   UC-DOC-17/4 (a person outside the audience neither lists nor opens it), UC-DOC-17/5 (an agent
//   or a service account follows nothing; followers within the audience are told, not the writer)
//   and UC-DOC-17/6 (every door the same) by core/application/service/knowledge/Posts_test.go,
//   FoundPosts_test.go, core/application/service/notification/RecordPost_test.go and
//   test/integration/post_repository_test.go;
// - UC-DOC-18/2 (who confirmed only for writers, the reminder to those who have not),
//   UC-DOC-18/3 (one level deep, a reply taken back by its author or a writer),
//   UC-DOC-18/4 (only a person confirms, reacts and is counted as seen) by Posts_test.go;
// - UC-DOC-18/5 (the unread posts across hubs) by Posts_test.go and post_repository_test.go;
// - UC-DOC-18/6 (seen and confirmed erased with the person, never in the trail) by
//   post_repository_test.go and Posts_test.go. An agent or a service account has no web app, so
//   UC-DOC-18/4 has no walk here.
//
// A post's pictures and files open the one viewer as a page's do (UC-MED-05/2); that only those the
// post is for read them is proven by Posts_test.go.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { ACCOUNT, HUB, MANIFEST, OTHER, signedIn, stub } from './fixture.mjs';
import { serve } from './serve.mjs';

const DIST = join(dirname(fileURLToPath(import.meta.url)), '..', 'dist');
const served = await serve(DIST);
test.after(() => served.close());

const id = (n) => `01a0e2e0-0000-7000-8000-0000000017${String(n).padStart(2, '0')}`;
const DAY = 86_400_000;
const ago = (days) => new Date(Date.now() - days * DAY).toISOString();
const ahead = (days) => new Date(Date.now() + days * DAY).toISOString();
const GROUP = { id: id(90), name: 'Night shift', version: 1 };
const REFUSED = id(99);
const empty = { data: [], page: { next_cursor: null, has_more: false } };

const post = (n, title, extra = {}) => ({
  page_id: id(n), hub_id: HUB.id, title, icon: null, template: null, state: 'PUBLISHED', publish_at: null, published_at: ago(n),
  featured_until: null, featured: false, audience: 'HUB', group_ids: [], everyone_in_workspace: false, discussion: true, notify: true,
  confirm_by: null, created_by: OTHER.id, created_at: ago(n + 1), seen_count: 12, seen: true, confirmed: false, may_write: false,
  cover: null, excerpt: `The opening words of ${title}.`, reading_minutes: 2, reply_count: 0, labels: [], confirmations: null,
  ...extra,
});

/** The hub's posts: one featured though older, one unread, one of mine, one scheduled for writers. */
function hubPosts() {
  return [
    post(1, 'Monday release', { seen: false, template: 'RELEASE_NOTE', labels: ['Release'] }),
    post(3, 'Mine from last week', { created_by: ACCOUNT.id }),
    post(40, 'Security rule', { featured_until: ahead(5), featured: true, labels: ['Security'] }),
    post(5, 'Next sprint', { state: 'SCHEDULED', publish_at: ahead(2), published_at: null }),
  ];
}

/**
 * A tab whose server keeps the hub's posts. `writer` decides whether the reader writes in the hub,
 * `wide` whether every member of the workspace reads it.
 */
async function opened(browser, { width = 1280, writer = false, wide = false, rows = hubPosts(), replies = [], words = {}, media = () => undefined } = {}) {
  const written = [];
  const kept = new Map(rows.map((each) => [each.page_id, structuredClone(each)]));
  const bodies = new Map(Object.entries(words));
  let following = false;
  let thread = structuredClone(replies);
  const { page, failures, context } = await signedIn(browser, width, 900, { timezoneId: 'UTC' });
  await context.unroute('**/api/v1/**');
  const shown = (one) => ({
    ...one, may_write: writer, everyone_in_workspace: writer && wide && one.audience === 'HUB',
    confirmations: writer && one.confirm_by ? { confirmed: 21, audience: 45, confirm_by: one.confirm_by, may_remind: true, overdue: false } : null,
  });
  const visible = () => [...kept.values()].filter((each) => writer || each.state === 'PUBLISHED');
  const answer = (one) => {
    if (one.publish_at === null && one.state !== 'PUBLISHED') return { ...one, state: 'DRAFT' };
    if (one.publish_at && one.publish_at > new Date().toISOString()) return { ...one, state: 'SCHEDULED', published_at: null };
    return one;
  };
  await context.route('**/api/v1/**', async (route) => {
    const request = route.request();
    const url = new URL(request.url());
    const path = url.pathname.replace('/api/v1', '');
    const method = request.method();
    if (method !== 'GET') written.push({ method, path, body: request.postData() ? request.postDataJSON() : undefined });
    if (path === '/meta/capabilities') return route.fulfill({ json: { ...MANIFEST, modules_on: ['KNOWLEDGE'] } });
    if (/^\/containers\/[^/]+\/modules$/.test(path)) return route.fulfill({ json: { modules_on: ['KNOWLEDGE'] } });
    if (path === '/groups') return route.fulfill({ json: { ...empty, data: [GROUP] } });

    if (path === `/hubs/${HUB.id}/posts` && method === 'GET') {
      const filter = url.searchParams.get('filter');
      const data = visible().filter((each) => filter === 'UNREAD' ? each.state === 'PUBLISHED' && !each.seen
        : filter === 'MINE' ? each.created_by === ACCOUNT.id
          : filter === 'SCHEDULED' ? each.state === 'SCHEDULED' : true);
      return route.fulfill({ json: { data: data.map(shown), may_write: writer, workspace_wide: writer && wide, following } });
    }
    if (path === `/hubs/${HUB.id}/posts` && method === 'POST') {
      const made = post(50, request.postDataJSON().title, { state: 'DRAFT', published_at: null, created_by: ACCOUNT.id, seen_count: 0, seen: false, reading_minutes: 1, excerpt: '' });
      kept.set(made.page_id, made);
      return route.fulfill({ status: 201, json: shown(made) });
    }
    if (path === `/hubs/${HUB.id}/posts/following` && method === 'PUT') {
      following = request.postDataJSON().following;
      return route.fulfill({ json: { following } });
    }
    if (path === '/posts' && method === 'GET') return route.fulfill({ json: { data: visible().filter((each) => each.state === 'PUBLISHED' && !each.seen).map(shown) } });

    const at = /^\/posts\/([^/:]+)(?::(\w+)|\/(.+))?$/.exec(path);
    if (at && at[1] === REFUSED) {
      return route.fulfill({ status: 404, contentType: 'application/problem+json', body: JSON.stringify({ type: 'about:blank', title: 'not found', status: 404, code: 'not_found', detail_code: 'posts.not_found' }) });
    }
    const one = at ? kept.get(at[1]) : undefined;
    if (one) {
      const [, , verb, below] = at;
      if (!verb && !below && method === 'GET') return route.fulfill({ json: shown(one) });
      if (!verb && !below && method === 'PUT') {
        const next = answer({ ...one, ...request.postDataJSON() });
        next.featured = !!next.featured_until && next.featured_until > new Date().toISOString();
        kept.set(one.page_id, next);
        return route.fulfill({ json: shown(next) });
      }
      if (verb === 'publish') kept.set(one.page_id, { ...one, state: 'PUBLISHED', publish_at: null, published_at: new Date().toISOString() });
      if (verb === 'seen') kept.set(one.page_id, { ...one, seen: true, seen_count: one.seen_count + 1 });
      if (verb === 'confirm') kept.set(one.page_id, { ...one, seen: true, confirmed: true });
      if (verb === 'remind') return route.fulfill({ json: { reminded: 24 } });
      if (verb) return route.fulfill({ json: shown(kept.get(one.page_id)) });
      if (below === 'confirmations') {
        if (!writer) return route.fulfill({ status: 403, contentType: 'application/problem+json', body: JSON.stringify({ status: 403, code: 'forbidden', detail_code: 'posts.writers_only' }) });
        return route.fulfill({ json: { confirmed: 21, audience: 45, confirm_by: one.confirm_by, may_remind: true, overdue: false, confirmed_ids: [OTHER.id] } });
      }
      if (below === 'replies' && method === 'GET') return route.fulfill({ json: { data: thread } });
      if (below === 'replies' && method === 'POST') {
        const reply = { id: id(60 + thread.length), page_id: one.page_id, author_id: ACCOUNT.id, created_at: new Date().toISOString(), deleted: false, reactions: [], parent_id: null, ...request.postDataJSON() };
        thread = [...thread, reply];
        return route.fulfill({ status: 201, json: reply });
      }
      const reacted = /^replies\/([^/]+)\/reactions\/(\w+)$/.exec(below ?? '');
      if (reacted) {
        const isOn = method === 'PUT';
        thread = thread.map((reply) => reply.id !== reacted[1] ? reply : {
          ...reply,
          reactions: [...reply.reactions.filter((each) => each.reaction !== reacted[2]),
            ...(isOn ? [{ reaction: reacted[2], count: 1, mine: true }] : [])],
        });
        return route.fulfill({ json: thread.find((reply) => reply.id === reacted[1]) });
      }
      const gone = /^replies\/([^/]+)$/.exec(below ?? '');
      if (gone && method === 'DELETE') {
        thread = thread.map((reply) => reply.id === gone[1] ? { ...reply, deleted: true, body: '' } : reply);
        return route.fulfill({ status: 204 });
      }
    }

    // The page that holds a post's words: the same reader and editor as a page.
    const words = /^\/pages\/([^/]+)$/.exec(path);
    const held = words ? kept.get(words[1]) : undefined;
    if (held) {
      if (method === 'PATCH') bodies.set(held.page_id, request.postDataJSON().body);
      return route.fulfill({ json: {
        id: held.page_id, hub_id: HUB.id, parent_id: null, title: held.title, icon: null, position: 'a0', template: false, content_language: null, owner_id: null,
        verified_until: null, verified_at: null, verified_by: null, due_for_review: false, archived: false, restricted: false, daily_for: null, trashed_at: null,
        created_by: held.created_by, created_at: held.created_at, updated_by: held.created_by, updated_at: held.created_at, version: 1,
        body: bodies.get(held.page_id) ?? (held.state === 'DRAFT' ? '' : `${held.excerpt}\n\nMore words follow.`),
        ancestors: [], writable: writer, shared: false, drafts: false, unpublished: false, draft: null,
      } });
    }
    if (path === `/hubs/${HUB.id}/pages`) return route.fulfill({ json: empty });
    if (path === `/hubs/${HUB.id}/page-labels` || path === `/hubs/${HUB.id}/trashed-pages`) return route.fulfill({ json: empty });
    if (path === `/hubs/${HUB.id}/page-drafts`) return route.fulfill({ json: { hub_id: HUB.id, drafts: false } });
    if (/^\/pages\/[^/]+\/(comments|versions|backlinks)$/.test(path)) return route.fulfill({ json: empty });
    if (/^\/pages\/[^/]+\/cover-images$/.test(path)) return route.fulfill({ json: { offered: false, pictures: [] } });
    if (/^\/pages\/([^/]+)\/language-variants$/.test(path)) return route.fulfill({ json: { page_id: path.split('/')[2], source_page_id: null, language: null, variants: [] } });
    if (/^\/pages\/([^/]+)\/labels$/.test(path)) return route.fulfill({ json: { page_id: path.split('/')[2], labels: [] } });
    if (/^\/pages\/([^/]+)\/access$/.test(path)) {
      return route.fulfill({ json: { page_id: path.split('/')[2], restricted: false, restricted_to: [], restricted_above: false, readers: [], shares: [], may_share: false, public_link: null, public_links_allowed: false, articles: [] } });
    }
    return media(route, path) ?? stub(route);
  });
  return { page, failures, written, close: () => context.close() };
}

const sentTo = (written, method, path) => written.filter((write) => write.method === method && write.path === path);

/** Waits until the server was sent `count` writes of a kind: the request is the proof. */
async function sent(page, written, method, path, count = 1) {
  for (let tries = 0; tries < 50 && sentTo(written, method, path).length < count; tries++) await page.waitForTimeout(100);
  return sentTo(written, method, path);
}

// UC-DOC-17 check 1: a writer starts a post, writes it from one of its own templates, and schedules
// it for a day and a time; a second one goes out now. The popover sends what the writer chose.
test('a writer writes a post from a template and schedules it, or publishes one now (UC-DOC-17/1)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, { writer: true, rows: [] });
    await page.goto(`${served.origin}/hubs/${HUB.id}/posts`);
    await page.locator('[data-tour="post-new"]').click({ timeout: 15_000 });
    await page.waitForURL(`**/posts/${id(50)}/write`, { timeout: 10_000 });
    const [made] = sentTo(written, 'POST', `/hubs/${HUB.id}/posts`);
    assert.equal(made.body.title, 'New post');
    assert.equal(made.body.publish_at, null, 'a new post is a draft until its writer publishes it');

    // The post's own four templates, as chips over its empty words.
    const chips = page.locator('[data-post-templates]');
    await chips.waitFor({ timeout: 15_000 });
    assert.deepEqual(await chips.locator('[data-post-template]').evaluateAll((each) => each.map((chip) => chip.dataset.postTemplate)),
      ['ANNOUNCEMENT', 'RELEASE_NOTE', 'MONTHLY_REVIEW', 'WELCOME']);
    await chips.locator('[data-post-template="RELEASE_NOTE"]').click();
    const [words] = await sent(page, written, 'PATCH', `/pages/${id(50)}`);
    assert.match(words.body.body, /^## New\n/u, 'the release note starts the words');
    const [kind] = await sent(page, written, 'PUT', `/posts/${id(50)}`);
    assert.equal(kind.body.template, 'RELEASE_NOTE');

    // Scheduled for Monday at nine, featured: no publishing now.
    await page.locator('[data-post-publish-open]').click();
    const panel = page.locator('[data-post-publish]');
    await panel.waitFor({ timeout: 5_000 });
    await panel.locator('[data-post-when="schedule"]').click();
    const monday = new Date(Date.now() + 3 * DAY).toISOString().slice(0, 10);
    await panel.getByLabel('Day').fill(monday);
    await panel.getByLabel('Time').fill('09:00');
    await panel.getByLabel('Feature', { exact: true }).check();
    await panel.locator('[data-post-publish-go]').click();
    const settled = await sent(page, written, 'PUT', `/posts/${id(50)}`, 2);
    assert.equal(settled[1].body.publish_at, `${monday}T09:00:00.000Z`, 'the day and the time asked, on this device clock');
    assert.ok(settled[1].body.featured_until, 'featured until a day');
    assert.equal(settled[1].body.notify, true);
    assert.equal(sentTo(written, 'POST', `/posts/${id(50)}:publish`).length, 0, 'a scheduled post is not published now');
    await page.locator('[data-post-meta]').getByText(/^Scheduled:/u).waitFor({ timeout: 5_000 });

    // Now: the settings, then the publishing, and the post opens as its readers see it.
    await page.locator('[data-post-publish-open]').click();
    await panel.locator('[data-post-when="now"]').click();
    await panel.locator('[data-post-publish-go]').click();
    await sent(page, written, 'POST', `/posts/${id(50)}:publish`);
    assert.equal(sentTo(written, 'PUT', `/posts/${id(50)}`).at(-1).body.publish_at, null);
    await page.waitForURL(`**/posts/${id(50)}`, { timeout: 10_000 });
    await page.locator('[data-post-seen]').waitFor({ timeout: 10_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 check 4: *Who sees it* is the hub's readers or chosen groups; *Everyone in the workspace*
// only where every member reads the hub. Check 5: the notify box goes with the settings.
test('who sees it offers everyone in the workspace only where all read the hub, and narrows to groups (UC-DOC-17/4, /5)', async () => {
  const browser = await chromium.launch();
  try {
    for (const wide of [true, false]) {
      const draft = post(50, 'A draft', { state: 'DRAFT', published_at: null, created_by: ACCOUNT.id, seen: false });
      const { page, failures, written, close } = await opened(browser, { writer: true, wide, rows: [draft] });
      await page.goto(`${served.origin}/posts/${id(50)}/write`);
      await page.locator('[data-post-publish-open]').click({ timeout: 15_000 });
      const panel = page.locator('[data-post-publish]');
      const audience = panel.getByLabel('Who sees it');
      await audience.waitFor({ timeout: 5_000 });
      const offered = await audience.locator('option').allTextContents();
      assert.deepEqual(offered.map((each) => each.trim()), [wide ? 'Everyone in the workspace' : `Everyone who reads ${HUB.name}`, 'Selected groups']);
      if (!wide) {
        await audience.selectOption('GROUPS');
        await panel.getByRole('group', { name: 'Groups' }).getByLabel(GROUP.name).check();
        await panel.getByLabel('Notify').uncheck();
        await panel.locator('[data-post-publish-go]').click();
        const [settings] = await sent(page, written, 'PUT', `/posts/${id(50)}`);
        assert.equal(settings.body.audience, 'GROUPS');
        assert.deepEqual(settings.body.group_ids, [GROUP.id]);
        assert.equal(settings.body.notify, false);
      }
      assert.deepEqual(failures, []);
      await close();
    }
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 checks 2, 3 and 7: the hub's posts for a writer - the featured one on top though older,
// the scheduled one dashed and said to be the writers' alone, the figures in the head - and the
// list narrowed to unread, mine, scheduled and a label.
test("a writer's list: featured on top, scheduled ones for writers only, unread, mine, scheduled (UC-DOC-17/2, /3, /7)", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, { writer: true });
    await page.goto(`${served.origin}/hubs/${HUB.id}/posts`);
    const featured = page.locator('[data-post-featured]');
    await featured.waitFor({ timeout: 15_000 });
    assert.equal(await featured.getAttribute('data-post-featured'), id(40));
    await featured.getByText(/^Featured until/u).waitFor();
    const order = await page.locator('[data-post-featured], [data-post-card]').evaluateAll((each) => each.map((card) => card.dataset.postFeatured ?? card.dataset.postCard));
    assert.equal(order[0], id(40), 'the featured post stands on top though it is the oldest');
    assert.equal(await page.locator(`[data-post-card="${id(40)}"]`).count(), 0, 'and only once');

    const scheduled = page.locator(`[data-post-card="${id(5)}"]`);
    assert.equal(await scheduled.getAttribute('data-post-state'), 'SCHEDULED');
    assert.equal(await scheduled.evaluate((card) => getComputedStyle(card).borderTopStyle), 'dashed');
    await scheduled.getByText("visible only to you and the hub's writers").waitFor();
    await scheduled.getByText(/^Scheduled:/u).waitFor();
    await page.getByText('News from the hub, newest first · 1 unread · 1 scheduled').waitFor();
    assert.equal(await page.locator(`[data-post-card="${id(1)}"] [data-post-unread]`).count(), 1, 'the unread post carries its dot');
    assert.equal(await page.locator(`[data-post-card="${id(3)}"] [data-post-unread]`).count(), 0);

    const tabs = page.getByRole('tablist', { name: 'Which posts' });
    const cards = () => page.locator('[data-post-card], [data-post-featured]').evaluateAll((each) => each.map((card) => card.dataset.postCard ?? card.dataset.postFeatured));
    const narrowed = async (name, wants) => {
      await tabs.getByRole('tab', { name: new RegExp(`^${name}`, 'u') }).click();
      for (let tries = 0; tries < 50 && JSON.stringify(await cards()) !== JSON.stringify(wants); tries++) await page.waitForTimeout(100);
      assert.deepEqual(await cards(), wants, name);
    };
    await narrowed('Unread', [id(1)]);
    await narrowed('By me', [id(3)]);
    await narrowed('Scheduled', [id(5)]);
    await narrowed('All', [id(40), id(5), id(1), id(3)]);
    await page.locator('[data-posts-labels] select').selectOption('Security');
    for (let tries = 0; tries < 50 && (await cards()).length !== 1; tries++) await page.waitForTimeout(100);
    assert.deepEqual(await cards(), [id(40)]);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 checks 2 and 3 for a reader: no scheduled tab, no *New post*; once its date has passed a
// post is no longer featured and stands among the others by its date.
test('a reader has no scheduled tab and a post past its feature date stands by its date (UC-DOC-17/2, /3)', async () => {
  const browser = await chromium.launch();
  try {
    const rows = hubPosts().map((each) => each.page_id === id(40) ? { ...each, featured_until: ago(1), featured: false } : each);
    const { page, failures, close } = await opened(browser, { rows });
    await page.goto(`${served.origin}/hubs/${HUB.id}/posts`);
    await page.locator(`[data-post-card="${id(1)}"]`).waitFor({ timeout: 15_000 });
    const tabs = page.getByRole('tablist', { name: 'Which posts' });
    assert.deepEqual((await tabs.getByRole('tab').allTextContents()).map((each) => each.replace(/\d+/gu, '').trim()), ['All', 'Unread', 'By me']);
    assert.equal(await page.locator('[data-tour="post-new"]').count(), 0, 'a reader writes no post');
    assert.equal(await page.locator('[data-post-featured]').count(), 0);
    assert.equal(await page.locator(`[data-post-card="${id(5)}"]`).count(), 0, 'the scheduled post is not listed for a reader');
    const order = await page.locator('[data-post-card]').evaluateAll((each) => each.map((card) => card.dataset.postCard));
    assert.deepEqual(order, [id(1), id(3), id(40)], 'newest first, the formerly featured one last by its date');
    await page.getByText('News from the hub, newest first · 1 unread', { exact: true }).waitFor();
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 check 5: a person follows the hub's posts and stops again.
test("a person follows a hub's posts and stops (UC-DOC-17/5)", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser);
    await page.goto(`${served.origin}/hubs/${HUB.id}/posts`);
    const follow = page.locator('[data-tour="post-follow"]');
    await follow.getByText('Follow', { exact: true }).waitFor({ timeout: 15_000 });
    await follow.click();
    await follow.getByText('Following', { exact: true }).waitFor({ timeout: 5_000 });
    await follow.click();
    await follow.getByText('Follow', { exact: true }).waitFor({ timeout: 5_000 });
    assert.deepEqual(sentTo(written, 'PUT', `/hubs/${HUB.id}/posts/following`).map((write) => write.body), [{ following: true }, { following: false }]);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 check 4: a post the person is not in the audience of answers `posts.not_found`, and the
// screen says so as of a post that is not there.
test('a post outside the reader audience is refused as if it were not there (UC-DOC-17/4)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser);
    await page.goto(`${served.origin}/posts/${REFUSED}`);
    await page.getByText('This post does not exist or is not for you.').waitFor({ timeout: 15_000 });
    assert.equal(await page.locator('[data-post-meta]').count(), 0);
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-17 check 7: the hub column's *Posts* at its foot - published ones newest first, the unread
// one with its dot, *All* at the section's head as the contents have it on a phone.
test("the hub column ends with its posts, unread ones dotted, All at the head (UC-DOC-17/7)", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser);
    await page.goto(`${served.origin}/hubs/${HUB.id}/posts`);
    const column = page.locator('aside.sidenav');
    const all = column.getByRole('button', { name: 'All', exact: true });
    await all.waitFor({ timeout: 15_000 });
    const links = await column.locator('a[href^="/posts/"], a[href$="/posts"]').evaluateAll((each) => each.map((link) => link.getAttribute('href')));
    assert.deepEqual(links, [`/posts/${id(1)}`, `/posts/${id(3)}`, `/posts/${id(40)}`], 'newest first, no link row after them');
    await column.getByRole('link', { name: /^Monday release\b.*\bunread\b/u }).waitFor();
    assert.equal(await column.getByRole('link', { name: /^Mine from last week\b.*\bunread\b/u }).count(), 0, 'a post read has no dot');
    await column.getByRole('link', { name: /^Monday release\b.*\bunread\b/u }).click();
    await page.waitForURL(`**/posts/${id(1)}`, { timeout: 10_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-18 check 1: the post's kind, title, writer, date, reading time and *seen by N*; opening it
// counts the reader once.
test('a reader opens a post: kind, title, writer, date, reading time, seen by N, counted once (UC-DOC-18/1)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser);
    await page.goto(`${served.origin}/posts/${id(1)}`);
    await page.locator('[data-post-kicker]').getByText(`Release note · Post from ${HUB.name} · to everyone who reads ${HUB.name}`).waitFor({ timeout: 15_000 });
    await page.getByRole('main').getByText('Monday release').first().waitFor();
    const meta = page.locator('[data-post-meta]');
    const said = await meta.locator('.who').evaluate((who) => who.textContent.replace(/\s+/gu, ' ').trim());
    assert.equal(said, `${OTHER.display_name} · yesterday · 2 min read`, 'writer, date and reading time');
    await sent(page, written, 'POST', `/posts/${id(1)}:seen`);
    await meta.locator('[data-post-seen]').getByText('seen by 13').waitFor({ timeout: 5_000 });
    assert.equal(await meta.locator('[data-post-seen]').evaluate((seen) => seen.textContent.trim()), 'seen by 13', 'a count, no names');
    // Opened again: already seen, not counted twice.
    await page.goto(`${served.origin}/posts/${id(1)}`);
    await meta.locator('[data-post-seen]').getByText('seen by 13').waitFor({ timeout: 15_000 });
    await page.waitForTimeout(500);
    assert.equal(sentTo(written, 'POST', `/posts/${id(1)}:seen`).length, 1, 'counted once');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-MED-05 check 2: a post is a page, so a picture and a file in its body open the one viewer,
// every file of the post at its foot, named after the post.
test("a post's picture and file open the one viewer, named after the post (UC-MED-05/2)", async () => {
  const PNG_ID = id(81);
  const TEXT_ID = id(82);
  const at = ago(2);
  const record = (mediaId, file_name, content_type, size, inline) => ({
    id: mediaId, file_name, content_type, size, usage: 'ATTACHMENT', status: 'READY', ref_count: 1, created_by: OTHER.id, created_at: at,
    inline, office: false, editable: false, download: { url: `${served.origin}/api/v1/e2e-content/${mediaId}`, method: 'GET', expires_at: ahead(1) },
  });
  const records = { [PNG_ID]: record(PNG_ID, 'stage.png', 'image/png', 68, true), [TEXT_ID]: record(TEXT_ID, 'changes.txt', 'text/plain', 24, false) };
  const ONE_PIXEL = Buffer.from('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==', 'base64');
  const media = (route, path) => {
    const read = /^\/media\/([^/:]+)$/.exec(path);
    if (read && records[read[1]]) return route.fulfill({ json: records[read[1]] });
    if (path === `/e2e-content/${PNG_ID}`) return route.fulfill({ status: 200, contentType: 'image/png', body: ONE_PIXEL });
    if (path === `/e2e-content/${TEXT_ID}`) return route.fulfill({ status: 200, contentType: 'text/plain', body: 'Tiles arrive on Monday.' });
    return undefined;
  };
  const words = { [id(1)]: `The release in a picture:\n\n![](hubtask:media/${PNG_ID})\n\nThe changes:\n\n![](hubtask:media/${TEXT_ID})\n` };
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser, { words, media });
    await page.goto(`${served.origin}/posts/${id(1)}`);
    const card = page.getByRole('button', { name: 'Open changes.txt' });
    await card.waitFor({ timeout: 15_000 });
    await card.click();
    const viewer = page.getByRole('dialog', { name: 'File viewer' });
    await viewer.waitFor({ timeout: 5_000 });
    assert.equal((await viewer.getByRole('heading', { level: 2 }).textContent()).trim(), 'changes.txt');
    const position = async () => ((await viewer.locator('[data-viewer-position]').textContent()) ?? '').replace(/\s+/g, ' ').trim();
    assert.equal(await position(), '· 2 of 2 · on Monday release', 'the post\'s files at the foot, named after the post');
    await viewer.getByText('Tiles arrive on Monday.').waitFor({ timeout: 5_000 });
    await page.keyboard.press('ArrowLeft');
    await viewer.getByRole('heading', { level: 2, name: 'stage.png' }).waitFor({ timeout: 5_000 });
    await page.keyboard.press('Escape');
    await viewer.waitFor({ state: 'hidden', timeout: 5_000 });
    // The picture in the body opens the viewer too.
    await page.getByRole('button', { name: 'Open stage.png' }).click();
    await viewer.getByRole('heading', { level: 2, name: 'stage.png' }).waitFor({ timeout: 5_000 });
    assert.equal(await position(), '· 1 of 2 · on Monday release');
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-18 check 2 for a reader: the band with the deadline and *Confirm*, at a phone width the
// short word; a reader never sees who confirmed, nor asks.
test('a reader confirms a post from its band and never sees who confirmed (UC-DOC-18/2)', async () => {
  const browser = await chromium.launch();
  try {
    for (const width of [1280, 375]) {
      const rows = [post(1, 'Security rule', { seen: false, confirm_by: ahead(7) })];
      const { page, failures, written, close } = await opened(browser, { width, rows });
      const asked = [];
      page.on('request', (request) => { if (request.url().includes('/confirmations')) asked.push(request.url()); });
      await page.goto(`${served.origin}/posts/${id(1)}`);
      const band = page.locator('[data-post-confirm-band]');
      await band.getByText(`${OTHER.display_name} asks for a read confirmation by`, { exact: false }).waitFor({ timeout: 15_000 });
      const go = band.locator('[data-post-confirm-go]');
      assert.equal((await go.textContent()).trim(), width < 600 ? 'Read' : 'Read and understood');
      await go.click();
      await sent(page, written, 'POST', `/posts/${id(1)}:confirm`);
      await band.getByText('You confirmed reading this post.').waitFor({ timeout: 5_000 });
      assert.equal(await page.locator('[data-post-writers]').count(), 0, 'a reader has no writers panel');
      assert.deepEqual(asked, [], 'a reader never asks who confirmed');
      const wide = await page.evaluate(() => document.documentElement.scrollWidth - document.documentElement.clientWidth);
      assert.ok(wide <= 0, `the post scrolls ${wide}px sideways at ${width}`);
      assert.deepEqual(failures, []);
      await close();
    }
  } finally {
    await browser.close();
  }
});

// UC-DOC-18 check 2 for a writer: "Confirmed: 21 of 45" with who confirmed, and *Remind*.
test('a writer sees how many and who confirmed, and reminds the others (UC-DOC-18/2)', async () => {
  const browser = await chromium.launch();
  try {
    const rows = [post(1, 'Security rule', { confirm_by: ahead(7) })];
    const { page, failures, written, close } = await opened(browser, { writer: true, rows });
    await page.goto(`${served.origin}/posts/${id(1)}`);
    const panel = page.locator('[data-post-writers]');
    await panel.locator('[data-post-confirmed]').getByText('Confirmed: 21 of 45', { exact: false }).waitFor({ timeout: 15_000 });
    await panel.locator('[data-post-confirmed-by]').getByText(OTHER.display_name).waitFor({ timeout: 5_000 });
    await panel.locator('[data-post-remind]').click();
    await sent(page, written, 'POST', `/posts/${id(1)}:remind`);
    await page.getByText('Those who have not confirmed were reminded').first().waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

const REPLIES = [
  { id: id(70), page_id: id(1), parent_id: null, author_id: OTHER.id, body: 'How does it apply to the night shift?', created_at: ago(1), deleted: false, reactions: [] },
];

// UC-DOC-18 check 3: replying, answering one level deep, the four reactions once per kind, and a
// reply taken back with its place kept.
test('a reader replies, answers one level deep, reacts and takes a reply back (UC-DOC-18/3)', async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, { replies: REPLIES });
    await page.goto(`${served.origin}/posts/${id(1)}`);
    const discussion = page.locator('[data-post-discussion]');
    await discussion.getByText('How does it apply to the night shift?').waitFor({ timeout: 15_000 });

    await discussion.getByLabel('Say something about it', { exact: true }).fill('Thanks for the heads-up');
    await discussion.getByRole('button', { name: 'Send' }).first().click();
    const [first] = await sent(page, written, 'POST', `/posts/${id(1)}/replies`);
    assert.deepEqual(first.body, { body: 'Thanks for the heads-up', parent_id: null });
    // Straight from the send to the answer below, pressed as a hand does: the empty field folds
    // only once that press has clicked, so the replies stay under the pointer until it is released.
    await discussion.locator('[data-post-reply]').filter({ hasText: 'Thanks for the heads-up' }).waitFor({ timeout: 5_000 });
    const asked = discussion.locator(`[data-post-reply="${id(70)}"]`);
    const aimed = await asked.locator(`[data-post-answer="${id(70)}"]`).boundingBox();
    await page.mouse.move(aimed.x + aimed.width / 2, aimed.y + aimed.height / 2);
    await page.mouse.down();
    await page.mouse.up();
    await asked.getByLabel(`Reply to ${OTHER.display_name}`).fill('Same as the day shift.');
    await asked.getByRole('button', { name: 'Send' }).click();
    const answers = await sent(page, written, 'POST', `/posts/${id(1)}/replies`, 2);
    assert.deepEqual(answers[1].body, { body: 'Same as the day shift.', parent_id: id(70) });
    const answer = asked.locator('[data-post-reply]').filter({ hasText: 'Same as the day shift.' });
    await answer.waitFor({ timeout: 5_000 });
    assert.equal(await answer.locator('[data-post-answer]').count(), 0, 'an answer is not answered again: one level');

    await asked.locator(`[data-post-react="${id(70)}"]`).first().click();
    const offered = await page.getByRole('menuitem').allTextContents();
    assert.deepEqual(offered.map((each) => each.trim()), ['🙏 Thanks', '👍 Agree', '🎉 Celebrate', '💡 Idea']);
    await page.getByRole('menuitem', { name: /Thanks/u }).click();
    await sent(page, written, 'PUT', `/posts/${id(1)}/replies/${id(70)}/reactions/THANKS`);
    const thanks = discussion.locator(`[data-post-reply="${id(70)}"] > .words > .acts [data-post-reaction="THANKS"]`);
    await thanks.waitFor({ timeout: 5_000 });
    assert.equal(await thanks.getAttribute('aria-pressed'), 'true');
    await thanks.click();
    await sent(page, written, 'DELETE', `/posts/${id(1)}/replies/${id(70)}/reactions/THANKS`);

    assert.equal(await asked.locator(`[data-post-take-back="${id(70)}"]`).count(), 0, "a reader does not take back another's reply");
    const mine = discussion.locator('[data-post-reply]').filter({ hasText: 'Thanks for the heads-up' });
    const mineId = await mine.getAttribute('data-post-reply');
    await mine.locator(`[data-post-take-back="${mineId}"]`).click();
    await sent(page, written, 'DELETE', `/posts/${id(1)}/replies/${mineId}`);
    await discussion.locator(`[data-post-reply="${mineId}"]`).getByText('This reply was taken back.').waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-18 check 3: a writer of the hub takes back another person's reply.
test("a writer of the hub takes back another person's reply (UC-DOC-18/3)", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, written, close } = await opened(browser, { writer: true, replies: REPLIES });
    await page.goto(`${served.origin}/posts/${id(1)}`);
    const asked = page.locator(`[data-post-reply="${id(70)}"]`);
    await asked.locator(`[data-post-take-back="${id(70)}"]`).click({ timeout: 15_000 });
    await sent(page, written, 'DELETE', `/posts/${id(1)}/replies/${id(70)}`);
    await asked.getByText('This reply was taken back.').waitFor({ timeout: 5_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});

// UC-DOC-18 check 5: Today's *New for you* lists the published posts the person has not read, each
// opening on its post.
test("Today's New for you lists the posts not read yet (UC-DOC-18/5)", async () => {
  const browser = await chromium.launch();
  try {
    const { page, failures, close } = await opened(browser);
    await page.goto(`${served.origin}/`);
    const tile = page.locator('[data-tile="NEWS"]');
    await tile.getByText('New for you').waitFor({ timeout: 15_000 });
    assert.deepEqual(await tile.locator('[data-news]').evaluateAll((each) => each.map((row) => row.dataset.news)), [id(1)]);
    await tile.getByText(`Post by ${OTHER.display_name}`, { exact: false }).waitFor();
    await tile.getByRole('link', { name: 'Monday release' }).click();
    await page.waitForURL(`**/posts/${id(1)}`, { timeout: 10_000 });
    assert.deepEqual(failures, []);
    await close();
  } finally {
    await browser.close();
  }
});
