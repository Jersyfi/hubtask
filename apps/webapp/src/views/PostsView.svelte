<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's posts (scene `posts`, UC-DOC-17, UC-DOC-18): unread, by me and scheduled as a switch,
  // labels as a filter, the featured post on top, the rest by month, newest first. Which posts
  // the reader sees is the server's to judge; scheduled ones reach writers only (B-3).

  import { Avatar, Badge, EmptyState, Icon, Select, Spinner, StateLozenge, Tabs, VisuallyHidden, WorkPage } from '@hubtask/design-system/components';
  import type { Post, PostList } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { featuredPost, hubPostsPath, postMoment, posts, postsByMonth, type PostFilter } from '../lib/data/posts.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page as frame } from '../lib/frame/page.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { formatDateTime, formatDay, formatMonth, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { coverBackground } from '../lib/pages/covers.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    hubId: string;
    onnavigate: (path: string) => void;
  }

  const { hubId, onnavigate }: Props = $props();

  $effect(() => frame.locate(hubId));
  const hub = $derived(containers.find(hubId));

  // The whole list answers the counts; a filter reads its own narrowed list.
  const all = untrack(() => resource<PostList>(hubPostsPath(hubId)));
  let filter = $state<PostFilter>('ALL');
  const narrowed = $derived(filter === 'ALL' ? undefined : resource<PostList>(hubPostsPath(hubId, filter)));
  const listed = $derived(all.state.status === 'ready' ? all.state.data : undefined);
  const shown = $derived(
    filter === 'ALL' ? listed?.data : narrowed?.state.status === 'ready' ? narrowed.state.data.data : undefined,
  );
  const mayWrite = $derived(listed?.may_write ?? false);

  let label = $state('');
  const labels = $derived([...new Set((listed?.data ?? []).flatMap((post) => post.labels))].sort((a, b) => a.localeCompare(b, messages.locale)));
  const rows = $derived((shown ?? []).filter((post) => label === '' || post.labels.includes(label)));
  const unread = $derived((listed?.data ?? []).filter((post) => post.state === 'PUBLISHED' && !post.seen).length);
  const scheduled = $derived((listed?.data ?? []).filter((post) => post.state === 'SCHEDULED').length);
  const featured = $derived(filter === 'ALL' ? featuredPost(rows.filter((post) => post.state === 'PUBLISHED')) : undefined);
  const months = $derived(postsByMonth(rows.filter((post) => post !== featured)));

  $effect(() => accounts.resolve((listed?.data ?? []).map((post) => post.created_by)));
  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  const tabs = $derived([
    { id: 'ALL', label: t('app.posts.filter_all') },
    { id: 'UNREAD', label: t('app.posts.filter_unread'), count: unread },
    { id: 'MINE', label: t('app.posts.filter_mine') },
    ...(mayWrite ? [{ id: 'SCHEDULED', label: t('app.posts.filter_scheduled'), count: scheduled }] : []),
  ]);

  let isCreating = $state(false);
  async function create() {
    if (isCreating) return;
    isCreating = true;
    try {
      const made = await posts.create(hubId, { title: t('app.posts.untitled'), publish_at: null, audience: 'HUB', group_ids: [], discussion: true, notify: true });
      onnavigate(`/posts/${made.page_id}/write`);
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isCreating = false;
    }
  }

  const figure = $derived(
    [t('app.posts.subtitle'), unread > 0 ? t('app.posts.unread_count', { count: String(unread) }) : '', mayWrite && scheduled > 0 ? t('app.posts.scheduled_count', { count: String(scheduled) }) : '']
      .filter((part) => part !== '')
      .join(' · '),
  );
  const background = (post: Post) => (post.cover ? coverBackground(post.cover) : undefined);
</script>

{#snippet meta(post: Post)}
  <span class="person">
    <Avatar name={nameOf(post.created_by)} size="sm" />
    <span>{nameOf(post.created_by)}</span>
  </span>
  <span class="quiet">{t('app.posts.reading_minutes', { count: String(post.reading_minutes) })}</span>
{/snippet}

{#snippet card(post: Post)}
  {@const isAhead = post.state !== 'PUBLISHED'}
  <a class="card" class:ahead={isAhead} href={isAhead ? `/posts/${post.page_id}/write` : `/posts/${post.page_id}`} data-post-card={post.page_id} data-post-state={post.state}>
    <span class="cover" class:blank={!background(post)} style:background={background(post)}></span>
    <span class="text">
      <span class="line">
        {#if post.state === 'PUBLISHED' && !post.seen}<span class="dot" data-post-unread></span><VisuallyHidden>{t('app.posts.unread')}</VisuallyHidden>{/if}
        {#if isAhead}
          <StateLozenge tone={post.state === 'SCHEDULED' ? 'warning' : 'neutral'} icon={post.state === 'SCHEDULED' ? 'clock' : 'pencil'}
            label={post.state === 'SCHEDULED' && post.publish_at ? t('app.posts.scheduled_for', { when: formatDateTime(post.publish_at, messages.locale) }) : t('app.posts.draft')} />
          <span class="quiet">{t('app.posts.writers_only')}</span>
        {:else}
          {#each post.labels as each (each)}<Badge size="sm">{each}</Badge>{/each}
          <span class="quiet">{formatDay(postMoment(post), messages.locale)}</span>
        {/if}
      </span>
      <span class="title">{post.title}</span>
      {#if post.excerpt}<span class="excerpt">{post.excerpt}</span>{/if}
      <span class="foot">
        {@render meta(post)}
        <span class="grow"></span>
        {#if post.discussion}<span class="quiet" data-post-replies><Icon name="message-square" size="sm" /> {post.reply_count}<VisuallyHidden>{t('app.posts.replies')}</VisuallyHidden></span>{/if}
      </span>
    </span>
  </a>
{/snippet}

<WorkPage
  head={{
    title: t('app.posts.title'),
    breadcrumb: hub ? { trail: [{ id: hub.id, label: hub.name, href: `/hubs/${hub.id}` }], label: t('app.pages.breadcrumb'), expandLabel: t('app.pages.breadcrumb_more') } : undefined,
    primary: mayWrite ? { label: t('app.posts.new'), icon: 'plus', onclick: () => void create(), tour: 'post-new' } : undefined,
  }}
>
  {#snippet toolbar()}
    <div class="bar" data-posts-bar>
      <Tabs label={t('app.posts.filter')} {tabs} selected={filter} onselect={(id) => (filter = id as PostFilter)} />
      <span class="grow"></span>
      {#if labels.length > 0}
        <span class="labels" data-posts-labels>
          <Select size="sm" label={t('app.posts.label')} isLabelHidden bind:value={label}
            options={[{ value: '', label: t('app.posts.label_all') }, ...labels.map((each) => ({ value: each, label: each }))]} />
        </span>
      {/if}
    </div>
  {/snippet}

  <p class="figure" data-posts-figure>{figure}</p>
  {#if all.state.status === 'failed'}
    <p class="refusal">{renderProblem(all.state.error, messages).message}</p>
  {:else if !shown}
    <Spinner label={t('app.posts.loading')} />
  {:else if rows.length === 0}
    <EmptyState kind={filter === 'ALL' && label === '' ? 'unused' : 'filtered'} icon="bell"
      title={t(filter === 'ALL' && label === '' ? 'app.posts.empty' : 'app.posts.empty_filtered')}
      description={mayWrite && filter === 'ALL' ? t('app.posts.empty_hint') : undefined} />
  {:else}
    {#if featured}
      <a class="featured" href={`/posts/${featured.page_id}`} data-post-featured={featured.page_id}>
        <span class="cover" class:blank={!background(featured)} style:background={background(featured)}></span>
        <span class="text">
          <span class="line">
            {#if featured.featured_until}<StateLozenge tone="brand" icon="star" label={t('app.posts.featured_until', { date: formatDay(featured.featured_until, messages.locale) })} />{/if}
            {#if !featured.seen}<span class="dot" data-post-unread></span><VisuallyHidden>{t('app.posts.unread')}</VisuallyHidden>{/if}
          </span>
          <span class="title big">{featured.title}</span>
          {#if featured.excerpt}<span class="excerpt">{featured.excerpt}</span>{/if}
          <span class="foot">
            <span class="person"><Avatar name={nameOf(featured.created_by)} size="sm" /><span>{nameOf(featured.created_by)} · {formatRelative(postMoment(featured), messages.locale)}</span></span>
            <span class="grow"></span>
            {#if featured.confirm_by}
              <StateLozenge tone={featured.confirmed ? 'success' : 'info'} icon="check" label={t(featured.confirmed ? 'app.posts.confirmed_mine' : 'app.posts.confirm_asked')} />
            {/if}
          </span>
        </span>
      </a>
    {/if}
    {#each months as group (group.month)}
      <section class="month" aria-labelledby={`posts-${group.month}`}>
        <h2 class="caps" id={`posts-${group.month}`}>{formatMonth(group.month, messages.locale)}</h2>
        <div class="cards">
          {#each group.posts as post (post.page_id)}{@render card(post)}{/each}
        </div>
      </section>
    {/each}
  {/if}
</WorkPage>

<style>
  .bar { display: flex; align-items: center; gap: var(--sp-200); inline-size: 100%; flex-wrap: wrap; }
  .grow { flex: 1; }
  .labels { min-inline-size: var(--sp-1600); }
  .figure { margin: 0 0 var(--sp-200); color: var(--text-secondary); font-size: var(--fs-090); }
  .refusal { color: var(--text-danger); }
  .caps {
    margin: var(--sp-300) 0 var(--sp-150);
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .cards { display: flex; flex-direction: column; gap: var(--sp-200); }
  .card, .featured {
    display: grid; grid-template-columns: var(--sp-1600) minmax(0, 1fr); gap: var(--sp-200);
    padding: var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg);
    background: var(--bg-raised); color: inherit; text-decoration: none;
  }
  .card:hover, .featured:hover { border-color: var(--border-strong); }
  .card:focus-visible, .featured:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .card.ahead { border-style: dashed; }
  .card.ahead .cover { opacity: 0.5; }
  .featured { grid-template-columns: minmax(0, 1.2fr) minmax(0, 1fr); padding: 0; overflow: hidden; margin-block-end: var(--sp-300); }
  .featured .cover { block-size: auto; min-block-size: var(--sp-1600); border-radius: 0; }
  .featured .text { padding: var(--sp-300); }
  .cover { display: block; block-size: var(--sp-1000); border-radius: var(--r-md); background: var(--bg-surface-sunken); }
  .text { display: flex; flex-direction: column; gap: var(--sp-050); min-inline-size: 0; }
  .line, .foot, .person { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; }
  .foot { margin-block-start: auto; font-size: var(--fs-075); }
  .title { font-weight: var(--fw-semibold); font-size: var(--fs-200); }
  .title.big { font: var(--fw-semibold) var(--fs-450) / 1.3 var(--font-display); }
  .excerpt { color: var(--text-secondary); font-size: var(--fs-090); }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); display: inline-flex; align-items: center; gap: var(--sp-050); }
  .dot { inline-size: var(--sp-100); block-size: var(--sp-100); border-radius: var(--r-full); background: var(--accent-primary); flex: none; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .featured { grid-template-columns: minmax(0, 1fr); }
    .featured .cover { min-block-size: var(--sp-1000); }
    .card { grid-template-columns: minmax(0, 1fr); }
    .card .cover { display: none; }
  }
</style>
