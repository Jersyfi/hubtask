<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An article a service publishes, read in its portal (UC-DOC-13 checks 4 and 5): the page as
  // it stands now, and whether it helped - one answer per reader, which the publisher counts and
  // never names. The Markdown is shown as written, never as markup a page could inject.

  import { untrack } from 'svelte';
  import { Button, ErrorState, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { Article } from '@hubtask/sync-engine';

  import { articlePath, portal } from '../lib/data/portal.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { collectionId, pageId, onnavigate }: { collectionId: string; pageId: string; onnavigate?: (path: string) => void } = $props();

  const article = untrack(() => resource<Article>(articlePath(collectionId, pageId)));
  const read = $derived(article.state.status === 'ready' ? article.state.data : undefined);
  const failure = $derived(article.state.status === 'failed' ? renderProblem(article.state.error, messages) : undefined);
  $effect(() => page.entitle(read?.title ?? t('app.portal.article')));

  let answered = $state<boolean | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);

  async function rate(helpful: boolean) {
    refusal = undefined;
    try {
      await portal.rateArticle(collectionId, pageId, helpful);
      answered = helpful;
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }
</script>

<Stack gap="300">
  <PageHeader title={read ? `${read.icon ? `${read.icon} ` : ''}${read.title}` : t('app.portal.article')} isTitleInBar={viewport.isCompact} />
  {#if failure}
    <ErrorState title={failure.message} reference={failure.reference} referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')} onRetry={() => void article.refresh()} />
  {:else if !read}
    <div aria-busy="true"><Skeleton lines={6} /></div>
  {:else}
    <div><Button tone="subtle" size="sm" icon="arrow-left" onclick={() => onnavigate?.(`/portal/${collectionId}`)}>{t('app.portal.back')}</Button></div>
    <article class="body" lang={read.content_language ?? undefined}>{read.body}</article>
    <section aria-labelledby="article-helped">
      <Stack gap="100">
        <h2 class="heading" id="article-helped">{t('app.portal.helped')}</h2>
        {#if answered !== undefined}
          <p class="quiet" role="status">{answered ? t('app.portal.helped_yes') : t('app.portal.helped_no')}</p>
        {/if}
        <div class="answers">
          <Button tone="secondary" icon="circle-check" onclick={() => void rate(true)}>{t('app.portal.yes')}</Button>
          <Button tone="secondary" icon="circle-x" onclick={() => void rate(false)}>{t('app.portal.no')}</Button>
          {#if answered === false}
            <Button tone="primary" icon="send" onclick={() => onnavigate?.(`/portal/${collectionId}`)}>{t('app.portal.ask_anyway')}</Button>
          {/if}
        </div>
        {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
      </Stack>
    </section>
  {/if}
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .body { margin: 0; max-inline-size: 72ch; white-space: pre-wrap; overflow-wrap: anywhere; }
  .answers { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
