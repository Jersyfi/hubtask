<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Reading a post (scene `post`) and writing one (scene `postwrite`), UC-DOC-17 and UC-DOC-18: the
  // page's own reader and editor (B-1), with the post's head - kind, person, date, reading time,
  // *seen by* -, the read confirmation's band, the discussion under the words, and for writers
  // *confirmed 21 of 45* with *Remind*, *Publish* now or at a moment, and the templates as chips.

  import { Badge, Banner, Button, Icon, Menu, ProgressBar, Spinner, StateLozenge, VisuallyHidden } from '@hubtask/design-system/components';
  import type { Page, PageList, Post, PostList } from '@hubtask/sync-engine';
  import { untrack } from 'svelte';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { engine } from '../lib/data/engine.ts';
  import { pagePath, pages, templatesPath, writtenBody } from '../lib/data/pages.svelte.ts';
  import { POST_TEMPLATES, hubPostsPath, postMoment, postPath, posts, readingMinutes, settingsOf } from '../lib/data/posts.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime, formatDay, formatRelative } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { isBlankBody } from '../lib/pages/starts.ts';
  import PostDiscussion from '../lib/posts/PostDiscussion.svelte';
  import PostPublish from '../lib/posts/PostPublish.svelte';
  import { renderProblem } from '../lib/problem.ts';
  import PagesView from './PagesView.svelte';

  interface Props {
    pageId: string;
    /** The writing scene was asked for: a writer's draft or scheduled post opens there anyway. */
    isWriteAsked?: boolean;
    onnavigate: (path: string) => void;
  }

  const { pageId, isWriteAsked = false, onnavigate }: Props = $props();

  const read = untrack(() => resource<Post>(postPath(pageId)));
  const post = $derived(read.state.status === 'ready' ? read.state.data : undefined);
  const page = untrack(() => resource<Page>(pagePath(pageId)));
  const words = $derived(page.state.status === 'ready' ? writtenBody(page.state.data) : '');
  const hub = $derived(post ? containers.find(post.hub_id) : undefined);
  const isWriting = $derived(!!post?.may_write && (isWriteAsked || post.state !== 'PUBLISHED'));

  // Opening a published post counts the reader among those who have seen it, once.
  let isCounted = false;
  $effect(() => {
    if (!post || isCounted || post.state !== 'PUBLISHED' || post.seen) return;
    isCounted = true;
    untrack(() => void posts.seen(post).catch(() => undefined));
  });

  $effect(() => accounts.resolve(post ? [post.created_by] : []));
  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  // Whether every member reads the hub decides the audience's words (B-6): the hub's list knows.
  const list = $derived(post?.may_write ? resource<PostList>(hubPostsPath(post.hub_id)) : undefined);
  const isWorkspaceWide = $derived(list?.state.status === 'ready' ? list.state.data.workspace_wide : false);
  const audienceWords = $derived(
    !post ? '' : post.audience === 'GROUPS' ? t('app.posts.to_groups') : post.everyone_in_workspace || isWorkspaceWide ? t('app.posts.to_workspace') : t('app.posts.to_hub', { hub: hub?.name ?? '' }),
  );

  /** The audience as a name, for the writers' panel: *Everyone in the workspace*. */
  const audienceName = $derived(
    !post ? '' : post.audience === 'GROUPS' ? t('app.posts.audience_groups') : post.everyone_in_workspace || isWorkspaceWide ? t('app.posts.audience_workspace') : t('app.posts.audience_hub', { hub: hub?.name ?? '' }),
  );

  let isPublishing = $state(false);
  let isBusy = $state(false);
  async function act<T>(run: () => Promise<T>, done?: string): Promise<T | undefined> {
    isBusy = true;
    try {
      const answer = await run();
      if (done) toast.show({ text: done, tone: 'success' });
      await read.refresh();
      return answer;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    } finally {
      isBusy = false;
    }
  }

  // --- Templates as chips (scene postwrite 2): the post's own first, then the hub's page templates.
  let replaceWith = $state<{ body: string } | undefined>(undefined);
  const templateRead = $derived(isWriting && post ? resource<PageList>(templatesPath(post.hub_id)) : undefined);
  const pageTemplates = $derived(templateRead?.state.status === 'ready' ? templateRead.state.data.data : []);
  const startOf = (key: string) => t(`app.posts.start_${key}`);
  /** A start replaces words only while there are none of the writer's own: blank, or a start untouched. */
  const isUntouched = $derived(isBlankBody(words) || POST_TEMPLATES.some((each) => startOf(each.id).trim() === words.trim()));
  async function start(body: string, template: Post['template']) {
    if (!post || page.state.status !== 'ready' || !isUntouched) return;
    const current = page.state.data;
    await act(async () => {
      await pages.saveBody(current, body);
      replaceWith = { body };
      if (template !== post.template) await posts.settle(post, { ...settingsOf(post), template });
    });
  }
  async function startFromPage(id: string) {
    const answer = await engine.refresh<Page>({ path: pagePath(id) });
    if (answer.status === 'ready') await start(writtenBody(answer.data), null);
  }

  // --- Writers' panel (scene post 4): confirmed 21 of 45, Remind --------------------------------
  const confirmations = $derived(post?.confirmations ?? undefined);
  const share = $derived(confirmations && confirmations.audience > 0 ? Math.round((confirmations.confirmed / confirmations.audience) * 100) : 0);
</script>

{#snippet kicker()}
  {#if post}
    <span class="kicker" data-post-kicker>
      {#if post.template}<span>{t(`app.posts.template_${post.template}`)}</span> · {/if}
      {t('app.posts.kind_line', { hub: hub?.name ?? '' })} · {audienceWords}
    </span>
  {/if}
{/snippet}

{#snippet head()}
  {#if post}
    <div class="meta" data-post-meta>
      <span class="who"><strong>{nameOf(post.created_by)}</strong>
        · {post.published_at ? formatRelative(post.published_at, messages.locale) : formatDateTime(postMoment(post), messages.locale)}
        · {t('app.posts.reading_minutes', { count: String(post.reading_minutes || readingMinutes(words)) })}</span>
      <span class="grow"></span>
      {#if post.state === 'PUBLISHED'}
        <span class="seen" data-post-seen><Icon name="eye" size="sm" /> {t('app.posts.seen_by', { count: String(post.seen_count) })}</span>
      {:else}
        <StateLozenge tone={post.state === 'SCHEDULED' ? 'warning' : 'neutral'} icon={post.state === 'SCHEDULED' ? 'clock' : 'pencil'}
          label={post.state === 'SCHEDULED' && post.publish_at ? t('app.posts.scheduled_for', { when: formatDateTime(post.publish_at, messages.locale) }) : t('app.posts.draft')} />
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet band()}
  {#if post && isPublishing}
    <div class="band">
      <PostPublish {post} hubName={hub?.name ?? ''} {isWorkspaceWide} onclose={() => (isPublishing = false)}
        ondone={(next) => { isPublishing = false; void read.refresh(); if (next.state === 'PUBLISHED') onnavigate(`/posts/${next.page_id}`); }} />
    </div>
  {/if}
  {#if post && post.state === 'PUBLISHED' && post.confirm_by}
    <div class="band" data-post-confirm-band>
      {#if post.confirmed}
        <Banner tone="success" icon="check"><span>{t('app.posts.confirmed_band')}</span></Banner>
      {:else}
        <Banner tone="info" icon="check">
          <span>{t('app.posts.confirm_band', { name: nameOf(post.created_by), date: formatDay(post.confirm_by, messages.locale) })}</span>
          {#snippet action()}
            <Button size="sm" tone="primary" {isBusy} data-post-confirm-go onclick={() => void act(() => posts.confirm(post), t('app.posts.confirmed_toast'))}>
              {viewport.isCompact ? t('app.posts.confirm_short') : t('app.posts.confirm_go')}
            </Button>
          {/snippet}
        </Banner>
      {/if}
    </div>
  {/if}
  {#if post && isWriting && post.state !== 'PUBLISHED' && isUntouched}
    <div class="chips" data-post-templates>
      <span class="quiet">{t('app.posts.with_template')}</span>
      {#each POST_TEMPLATES as each (each.id)}
        <button type="button" class="chip" aria-pressed={post.template === each.id} data-post-template={each.id}
          onclick={() => void start(startOf(each.id), each.id)}><span aria-hidden="true">{each.mark}</span> {t(`app.posts.template_${each.id}`)}</button>
      {/each}
      {#if pageTemplates.length > 0}
        <Menu label={t('app.posts.page_templates')} items={pageTemplates.map((each) => ({ id: each.id, label: each.title }))} onselect={(id) => void startFromPage(id)}>
          {#snippet trigger(props)}<button type="button" class="chip" data-post-page-templates {...props}>+ {t('app.posts.page_templates')}</button>{/snippet}
        </Menu>
      {/if}
    </div>
  {/if}
{/snippet}

{#snippet aside()}
  {#if post && post.may_write && post.state === 'PUBLISHED'}
    <section class="writers" aria-labelledby="post-writers" data-post-writers>
      <h3 class="caps" id="post-writers">{t('app.posts.for_writers')}</h3>
      {#if confirmations}
        <span data-post-confirmed>{t('app.posts.confirmed_of', { confirmed: String(confirmations.confirmed), audience: String(confirmations.audience) })}
          {#if confirmations.may_remind}
            · <button type="button" class="link" data-post-remind onclick={() => void act(() => posts.remind(post), t('app.posts.reminded'))}>{t('app.posts.remind')}</button>
          {/if}
        </span>
        <ProgressBar label={t('app.posts.confirmed_of', { confirmed: String(confirmations.confirmed), audience: String(confirmations.audience) })} value={share} />
        {#if confirmations.overdue}<Badge size="sm" tone="warning">{t('app.posts.overdue')}</Badge>{/if}
      {/if}
      <span class="quiet">{[t('app.posts.audience_line', { audience: audienceName }), post.featured_until ? t('app.posts.featured_until', { date: formatDay(post.featured_until, messages.locale) }) : ''].filter(Boolean).join(' · ')}</span>
    </section>
  {/if}
{/snippet}

{#snippet foot()}
  {#if post && post.state === 'PUBLISHED' && post.discussion}
    {#key post.page_id}<PostDiscussion {post} />{/key}
  {/if}
{/snippet}

{#snippet acts()}
  {#if post && post.may_write}
    {#if isWriting && post.state !== 'PUBLISHED'}
      <Button size="sm" tone="subtle" icon="eye" data-post-preview onclick={() => onnavigate(`/posts/${post.page_id}`)}>{t('app.posts.preview')}</Button>
    {/if}
    <Button size="sm" tone={post.state === 'PUBLISHED' ? 'secondary' : 'primary'} icon={post.state === 'PUBLISHED' ? 'settings' : 'chevron-down'}
      aria-expanded={isPublishing} data-post-publish-open onclick={() => (isPublishing = !isPublishing)}>
      {post.state === 'PUBLISHED' ? t('app.posts.settings') : t('app.posts.publish')}
    </Button>
  {/if}
{/snippet}

{#if read.state.status === 'failed'}
  <VisuallyHidden as="h1">{t('app.posts.post')}</VisuallyHidden>
  <p class="refusal">{renderProblem(read.state.error, messages).message}</p>
{:else if !post}
  <VisuallyHidden as="h1">{t('app.posts.post')}</VisuallyHidden>
  <Spinner label={t('app.posts.loading')} />
{:else}
  <VisuallyHidden>{t(isWriting ? 'app.posts.writing' : 'app.posts.reading')}</VisuallyHidden>
  <PagesView pageId={post.page_id} {onnavigate}
    post={{ trail: { label: t('app.posts.title'), href: `/hubs/${post.hub_id}/posts` }, kicker, head, band, foot, aside, acts, replaceWith }} />
{/if}

<style>
  .refusal { color: var(--text-danger); }
  .kicker { display: block; color: var(--text-subtle); font-size: var(--fs-075); }
  .meta { display: flex; align-items: center; gap: var(--sp-150); flex-wrap: wrap; font-size: var(--fs-075); color: var(--text-secondary); margin-block: var(--sp-100); }
  .meta strong { color: var(--text-primary); }
  .grow { flex: 1; }
  .seen { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-subtle); }
  .band { margin-block: var(--sp-200); }
  .chips { display: flex; align-items: center; flex-wrap: wrap; gap: var(--sp-100); margin-block: var(--sp-200); }
  .chip {
    border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-full); background: var(--bg-surface);
    padding: var(--sp-050) var(--sp-150); font: inherit; font-size: var(--fs-090); color: var(--text-primary); cursor: pointer;
  }
  .chip[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }
  .chip:focus-visible, .link:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .quiet { color: var(--text-subtle); font-size: var(--fs-075); }
  .writers { display: flex; flex-direction: column; gap: var(--sp-100); padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); font-size: var(--fs-090); }
  .caps {
    margin: 0;
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .link { border: none; background: none; padding: 0; font: inherit; color: var(--text-brand); cursor: pointer; }
</style>
