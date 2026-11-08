<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who reads a page beyond its hub (UC-DOC-07, UC-DOC-08): its restriction to fewer of the hub's
  // readers, one above it, and the people it is shared with read-only. A writer restricts and
  // lifts; somebody who manages the hub's members shares (`may_share`). What the reader may not do
  // is not drawn (P-05). A public link, where the workspace allows one, is shown once when made:
  // only its hash is kept.

  import { untrack } from 'svelte';
  import { Badge, Button, CheckboxGroup, Dialog, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { PageAccess, PageLanguageVariants, PageList, PageRead } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { accessPath, languagesPath, pages, pagesPath, publicLinkAddress, publicPreviewOf } from '../data/pages.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    page: PageRead;
    hubId: string;
  }

  const { page, hubId }: Props = $props();

  const access = untrack(() => resource<PageAccess>(accessPath(page.id)));
  const info = $derived(access.state.status === 'ready' ? access.state.data : undefined);
  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  $effect(() => {
    if (info) accounts.resolve([...info.restricted_to, ...(info.readers ?? []), ...info.shares.map((share) => share.account_id)]);
  });

  let refusal = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done: string) {
    refusal = undefined;
    try {
      await run();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  // --- Restricting -------------------------------------------------------------------------------

  let isChoosing = $state(false);
  let chosen = $state<readonly string[]>([]);
  const self = $derived(actor.account?.id ?? '');
  const hubReaders = $derived(people.candidates({ hubId }));
  const readerOptions = $derived(
    [...new Set([self, ...hubReaders])].filter(Boolean).map((id) => ({
      value: id,
      label: id === self ? t('app.pages.access_you', { name: nameOf(id) }) : nameOf(id),
      ...(id === self ? { disabledReason: t('app.pages.access_you_stay') } : {}),
    })),
  );

  function startRestrict() {
    chosen = info && info.restricted ? [...new Set([self, ...info.restricted_to])] : [self];
    isChoosing = true;
  }

  async function restrict() {
    await act(() => pages.restrict(page, [...new Set([self, ...chosen])]), t('app.pages.access_restricted'));
    isChoosing = false;
  }

  // --- Sharing -----------------------------------------------------------------------------------

  let shareWith = $state('');
  const shareOptions = $derived([
    { value: '', label: t('app.pages.access_share_choose') },
    ...people
      .known()
      .filter((id) => id !== self && !(info?.shares ?? []).some((share) => share.account_id === id))
      .map((id) => ({ value: id, label: nameOf(id) })),
  ]);

  async function share() {
    if (!shareWith) return;
    const whom = shareWith;
    await act(() => pages.share(page.id, whom), t('app.pages.access_shared', { name: nameOf(whom) }));
    shareWith = '';
  }

  // --- A public link -----------------------------------------------------------------------------

  let madeAddress = $state<string | undefined>(undefined);
  const linkable = $derived(info?.may_share === true && info.public_links_allowed && !info.restricted && !info.restricted_above);

  // The link is made from a preview of what it shows (user-experience.md §7.1, UC-DOC-08 check 5):
  // counted from the body as the server resolves it, so nothing appears publicly by surprise.
  let isPreviewing = $state(false);
  const preview = $derived(publicPreviewOf(page.body ?? ''));

  async function makeLink() {
    isPreviewing = false;
    refusal = undefined;
    try {
      const made = await pages.link(page.id);
      madeAddress = publicLinkAddress(globalThis.location.origin, made.token);
      void navigator.clipboard?.writeText(madeAddress).catch(() => undefined);
      toast.show({ text: t('app.pages.access_link_made'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  // --- Published to a service's portal (UC-DOC-13) -----------------------------------------------

  $effect(() => untrack(() => containers.openLevel(hubId)));
  let publishTo = $state('');
  const serviceName = (id: string) => containers.find(id)?.name ?? t('app.pages.access_article_service');
  const publishOptions = $derived([
    { value: '', label: t('app.pages.access_article_choose') },
    ...containers
      .collectionsOf(hubId)
      .filter((collection) => !(info?.articles ?? []).some((article) => article.collection_id === collection.id))
      .map((collection) => ({ value: collection.id, label: collection.name })),
  ]);
  const publishable = $derived(info?.may_share === true && !info.restricted && !info.restricted_above);

  async function publish() {
    if (!publishTo) return;
    const where = publishTo;
    await act(() => pages.publish(page.id, where), t('app.pages.access_article_published', { name: serviceName(where) }));
    publishTo = '';
  }

  // --- Written in other languages for a portal (UC-DOC-13 check 4) -------------------------------

  const languages = untrack(() => resource<PageLanguageVariants>(languagesPath(page.id)));
  const spoken = $derived(languages.state.status === 'ready' ? languages.state.data : undefined);
  const space = untrack(() => resource<PageList>(pagesPath(hubId)));
  const spacePages = $derived(space.state.status === 'ready' ? space.state.data.data : []);
  const titleOf = (id: string) => spacePages.find((candidate) => candidate.id === id)?.title || t('app.pages.untitled');
  let sourceId = $state('');
  let language = $state(untrack(() => page.content_language ?? ''));
  const sourceOptions = $derived([
    { value: '', label: t('app.pages.access_language_choose') },
    ...spacePages.filter((candidate) => candidate.id !== page.id).map((candidate) => ({ value: candidate.id, label: candidate.title || t('app.pages.untitled') })),
  ]);

  async function linkLanguage() {
    if (!sourceId || language.trim() === '') return;
    const source = sourceId;
    const tag = language.trim();
    await act(() => pages.linkLanguage(page.id, source, tag), t('app.pages.access_language_linked', { language: tag, title: titleOf(source) }));
    sourceId = '';
  }

  async function removeLink() {
    madeAddress = undefined;
    await act(() => pages.unlink(page.id), t('app.pages.access_link_removed'));
  }
</script>

<section aria-labelledby="page-access-heading">
  <Stack gap="100">
    <h3 class="heading" id="page-access-heading">{t('app.pages.access')}</h3>
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    {#if access.state.status === 'failed'}
      <p class="refusal">{renderProblem(access.state.error, messages).message}</p>
    {:else if info}
      {#if info.restricted}
        <p class="meta">
          <Badge size="sm" tone="warning">{t('app.pages.access_restricted_badge')}</Badge>
          {t('app.pages.access_restricted_to', { names: info.restricted_to.map(nameOf).join(', ') })}
        </p>
      {:else if info.restricted_above}
        <p class="meta">{t('app.pages.access_restricted_above', { names: (info.readers ?? []).map(nameOf).join(', ') })}</p>
      {:else}
        <p class="quiet">{t('app.pages.access_hub')}</p>
      {/if}

      {#if page.writable}
        {#if isChoosing}
          <CheckboxGroup label={t('app.pages.access_who')} options={readerOptions} selected={chosen} onChange={(next) => (chosen = next)} />
          <Inline gap="100">
            <Button size="sm" tone="primary" onclick={restrict}>{t('app.pages.access_restrict_save')}</Button>
            <Button size="sm" tone="subtle" onclick={() => (isChoosing = false)}>{t('app.pages.cancel')}</Button>
          </Inline>
        {:else}
          <Inline gap="100" isWrapping>
            <Button size="sm" tone="secondary" icon="lock" onclick={startRestrict}>
              {info.restricted ? t('app.pages.access_restrict_change') : t('app.pages.access_restrict')}
            </Button>
            {#if info.restricted}
              <Button size="sm" tone="subtle" onclick={() => act(() => pages.restrict(page, undefined), t('app.pages.access_lifted'))}>
                {t('app.pages.access_lift')}
              </Button>
            {/if}
          </Inline>
        {/if}
      {/if}

      <h4 class="sub">{t('app.pages.access_shares')}</h4>
      <ul class="shares">
        {#each info.shares as held (held.account_id)}
          <li>
            <span>{nameOf(held.account_id)}</span>
            {#if info.may_share}
              <Button size="sm" tone="subtle" onclick={() => act(() => pages.unshare(page.id, held.account_id), t('app.pages.access_unshared'))}>
                {t('app.pages.access_unshare')}
              </Button>
            {/if}
          </li>
        {:else}
          <li class="quiet">{t('app.pages.access_no_shares')}</li>
        {/each}
      </ul>
      {#if info.may_share}
        <Inline gap="100" align="end" isWrapping>
          <Select label={t('app.pages.access_share_with')} size="sm" options={shareOptions} bind:value={shareWith} />
          <Button size="sm" tone="secondary" onclick={share}>{t('app.pages.access_share')}</Button>
        </Inline>
        <p class="quiet">{t('app.pages.access_share_hint')}</p>
      {/if}

      {#if info.articles.length > 0 || publishable}
        <h4 class="sub">{t('app.pages.access_articles')}</h4>
        <ul class="shares">
          {#each info.articles as article (article.collection_id)}
            <li>
              <span>
                {serviceName(article.collection_id)}
                <span class="meta">{t('app.pages.access_article_counts', {
                  views: String(article.views), helpful: String(article.helpful), unhelpful: String(article.unhelpful),
                })}</span>
              </span>
              {#if info.may_share}
                <Button size="sm" tone="subtle" onclick={() => act(() => pages.unpublish(page.id, article.collection_id), t('app.pages.access_article_unpublished'))}>
                  {t('app.pages.access_article_unpublish')}
                </Button>
              {/if}
            </li>
          {:else}
            <li class="quiet">{t('app.pages.access_no_articles')}</li>
          {/each}
        </ul>
        {#if publishable && publishOptions.length > 1}
          <Inline gap="100" align="end" isWrapping>
            <Select label={t('app.pages.access_article_to')} size="sm" options={publishOptions} bind:value={publishTo} />
            <Button size="sm" tone="secondary" onclick={publish}>{t('app.pages.access_article_publish')}</Button>
          </Inline>
          <p class="quiet">{t('app.pages.access_article_hint')}</p>
        {/if}
      {/if}

      {#if spoken && (spoken.source_page_id || spoken.variants.length > 0 || info.may_share)}
        <h4 class="sub">{t('app.pages.access_languages')}</h4>
        {#if spoken.source_page_id}
          {@const source = spoken.source_page_id}
          <ul class="shares">
            <li>
              <span>{t('app.pages.access_language_of', { language: spoken.language ?? '', title: titleOf(source) })}</span>
              {#if info.may_share}
                <Button size="sm" tone="subtle" onclick={() => act(() => pages.unlinkLanguage(page.id, source), t('app.pages.access_language_unlinked'))}>
                  {t('app.pages.access_language_unlink')}
                </Button>
              {/if}
            </li>
          </ul>
        {:else}
          <ul class="shares">
            {#each spoken.variants as variant (variant.page_id)}
              <li><span>{variant.language} <span class="meta">{titleOf(variant.page_id)}</span></span></li>
            {:else}
              <li class="quiet">{t('app.pages.access_no_languages')}</li>
            {/each}
          </ul>
          {#if info.may_share && spoken.variants.length === 0 && sourceOptions.length > 1}
            <Inline gap="100" align="end" isWrapping>
              <Select label={t('app.pages.access_language_source')} size="sm" options={sourceOptions} bind:value={sourceId} />
              <Input label={t('app.pages.access_language_tag')} size="sm" bind:value={language} />
              <Button size="sm" tone="secondary" onclick={linkLanguage}>{t('app.pages.access_language_link')}</Button>
            </Inline>
            <p class="quiet">{t('app.pages.access_language_hint')}</p>
          {/if}
        {/if}
      {/if}

      {#if info.public_link || linkable}
        <h4 class="sub">{t('app.pages.access_link')}</h4>
        {#if info.public_link}
          <p class="meta">
            {info.public_links_allowed
              ? t('app.pages.access_link_open', { name: nameOf(info.public_link.created_by) })
              : t('app.pages.access_link_paused')}
          </p>
        {:else}
          <p class="quiet">{t('app.pages.access_link_none')}</p>
        {/if}
        {#if madeAddress}
          <Input label={t('app.pages.access_link_address')} value={madeAddress} readonly />
          <p class="quiet">{t('app.pages.access_link_once')}</p>
        {/if}
        {#if info.may_share}
          <Inline gap="100" isWrapping>
            {#if linkable}
              <Button size="sm" tone="secondary" icon="link" data-public-link-make onclick={() => (isPreviewing = true)}>
                {info.public_link ? t('app.pages.access_link_renew') : t('app.pages.access_link_make')}
              </Button>
            {/if}
            {#if info.public_link}
              <Button size="sm" tone="subtle" onclick={removeLink}>{t('app.pages.access_link_remove')}</Button>
            {/if}
          </Inline>
          <p class="quiet">{t('app.pages.access_link_hint')}</p>
        {/if}
      {/if}
    {/if}
  </Stack>
</section>

{#if isPreviewing}
  <Dialog size="l" primary={{ label: t('app.pages.public_make'), onclick: makeLink }} bind:isOpen={isPreviewing} title={t('app.pages.public_preview_title', { title: page.title })} dismissLabel={t('app.pages.cancel')}>
    <div class="preview" data-public-preview>
      <p class="quiet">{t('app.pages.public_preview_lead')}</p>
      <h4 class="sub">{t('app.pages.public_shows')}</h4>
      <ul class="facts">
        <li>{t('app.pages.public_shows_words')}</li>
        {#if preview.media > 0}<li>{t('app.pages.public_shows_media', { count: preview.media })}</li>{/if}
      </ul>
      <h4 class="sub">{t('app.pages.public_hides')}</h4>
      <ul class="facts">
        {#if preview.pages > 0}<li>{t('app.pages.public_hides_pages', { count: preview.pages })}</li>{/if}
        {#if preview.others > 0}<li>{t('app.pages.public_hides_others', { count: preview.others })}</li>{/if}
        <li>{t('app.pages.public_hides_rest')}</li>
      </ul>
    </div>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => (isPreviewing = false)}>{t('app.pages.cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .sub { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .preview { display: grid; gap: var(--sp-100); }
  .facts { margin: 0; padding-inline-start: var(--sp-200); display: grid; gap: var(--sp-050); }
  .shares { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .shares li { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-100); }
</style>
