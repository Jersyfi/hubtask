<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *My notes* (M3): today's note in the reader's personal space - a private hub only they read,
  // made with the first note (UC-DOC-07) - the way into that space, and the pages other people
  // shared with them read-only, which is where a guest outside every hub starts (UC-DOC-08).

  import { untrack } from 'svelte';
  import { Button, EmptyState, ErrorState, ListRow, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { PersonalSpace, SharedPageList } from '@hubtask/sync-engine';

  import { pages, personalSpacePath, sharedPagesPath } from '../lib/data/pages.svelte.ts';
  import { resource } from '../lib/data/resource.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  $effect(() => page.entitle(t('app.notes.title')));

  const space = untrack(() => resource<PersonalSpace>(personalSpacePath));
  const shared = untrack(() => resource<SharedPageList>(sharedPagesPath));
  const hubId = $derived(space.state.status === 'ready' ? space.state.data.hub_id : null);
  const sharedRows = $derived(shared.state.status === 'ready' ? shared.state.data.data : []);
  const sharedFailure = $derived(shared.state.status === 'failed' ? renderProblem(shared.state.error, messages) : undefined);

  let isOpening = $state(false);
  let refusal = $state<string | undefined>(undefined);

  async function openToday() {
    isOpening = true;
    refusal = undefined;
    try {
      const note = await pages.today();
      onnavigate?.(`/pages/${note.id}`);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isOpening = false;
    }
  }
</script>

<Stack gap="300">
  <PageHeader title={t('app.notes.title')} subtitle={t('app.notes.intro')} isTitleInBar={viewport.isCompact} />

  <section aria-labelledby="notes-own">
    <Stack gap="100">
      <h2 class="heading" id="notes-own">{t('app.notes.own')}</h2>
      {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
      <div class="actions">
        <Button tone="primary" icon="sun" isBusy={isOpening} busyLabel={t('app.notes.opening')} onclick={() => void openToday()}>
          {t('app.notes.today')}
        </Button>
        {#if hubId}
          <Button tone="secondary" icon="file-text" onclick={() => onnavigate?.(`/hubs/${hubId}/pages`)}>{t('app.notes.all')}</Button>
        {/if}
      </div>
      <p class="quiet">{t('app.notes.own_hint')}</p>
    </Stack>
  </section>

  <section aria-labelledby="notes-shared">
    <Stack gap="100">
      <h2 class="heading" id="notes-shared">{t('app.notes.shared')}</h2>
      {#if sharedFailure}
        <ErrorState title={sharedFailure.message} reference={sharedFailure.reference} referenceLabel={t('app.reference')}
          retryLabel={t('app.retry')} onRetry={() => void shared.refresh()} />
      {:else if shared.state.status !== 'ready'}
        <div aria-busy="true"><Skeleton lines={2} /></div>
      {:else if sharedRows.length === 0}
        <EmptyState kind="unused" icon="file-text" title={t('app.notes.shared_none')} description={t('app.notes.shared_none_body')} />
      {:else}
        {#each sharedRows as row (row.id)}
          <ListRow href={`/pages/${row.id}`} onactivate={() => onnavigate?.(`/pages/${row.id}`)}>
            {#if row.icon}<span aria-hidden="true">{row.icon}</span>{/if}
            {row.title}
          </ListRow>
        {/each}
      {/if}
    </Stack>
  </section>
</Stack>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .actions { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
