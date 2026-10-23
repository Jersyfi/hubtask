<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What this workspace has put aside (ADR-0063 decision 1).
  //
  // **It exists because archiving a container is otherwise a one-way door.** An archived *entry*
  // stays in its list and says so — `items.svelte.ts` asks for `include_archived` deliberately, and
  // that is right (I-W4). An archived hub or collection does neither: the tree never asks for them,
  // the route leaves the parameter at `false`, and the container drops out of the navigation with
  // nothing anywhere to show it again. Archiving is offered as the reversible alternative to the
  // trash, so a reversal with no route in the interface was the half that was missing.
  //
  // **It reads what exists and adds no endpoint.** The two levels `ListContainers` answers, with
  // `include_archived` on both: the hubs, and each hub's collections. One request per hub, which is
  // what the API's shape costs and what the tree already pays for the hubs somebody opens.
  //
  // **Entries are listed by the search, and this screen leads there.** An archived entry stays in
  // its list and says so; every archived entry at once is the workspace-wide read `POST /search`
  // answers with the archive filter (`is:archived`), so the search's list - with its selection,
  // its menu and its narrowing - is the one place they are listed, rather than a second list here.

  import { untrack } from 'svelte';

  import { Badge, Button, EmptyState, ErrorState, Icon, ListRow, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';
  import type { Container } from '@hubtask/sync-engine';

  import { announcer } from '../lib/announce.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  $effect(() => untrack(() => containers.openArchive()));
  $effect(() => page.entitle(t('app.archive.title')));

  const rows = $derived(containers.archived);
  const failure = $derived(
    containers.archiveState.status === 'failed' ? renderProblem(containers.archiveState.error, messages) : undefined,
  );

  /** The one that is on its way back, so the row says which rather than the whole list going busy. */
  let restoring = $state<string | undefined>(undefined);
  let writeFailure = $state<string | undefined>(undefined);

  async function bringBack(container: Container) {
    restoring = container.id;
    writeFailure = undefined;
    try {
      await containers.setArchived(container.id, false, crypto.randomUUID());
      announcer.say(t('app.archive.unarchived'));
    } catch (error) {
      writeFailure = renderProblem(error as never, messages).message;
    } finally {
      restoring = undefined;
    }
  }

  const pathOf = (container: Container) => (container.type === 'HUB' ? `/hubs/${container.id}` : `/collections/${container.id}`);

  /** The search, narrowed to what is archived: the narrowing travels in the address, no words. */
  const ARCHIVED_ENTRIES = `/search?${new URLSearchParams({ f: 'is:archived' }).toString()}`;
</script>

<Stack gap="300">
  <PageHeader title={t('app.archive.title')} subtitle={t('app.archive.intro')} isTitleInBar={viewport.isCompact} />

  {#if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => containers.refresh()}
    />
  {:else if !containers.isArchiveReady}
    <div aria-busy="true"><Skeleton lines={3} /></div>
  {:else if rows.length === 0}
    <!-- `unused` rather than `filtered`: nothing is filtering this list, and an empty archive is
         the ordinary state of a workspace rather than a search that found nothing. -->
    <EmptyState kind="unused" icon="archive" title={t('app.archive.none')} description={t('app.archive.none_body')} />
  {:else}
    <Stack gap="100">
      {#if writeFailure}<p class="failure" role="alert">{writeFailure}</p>{/if}
      {#each rows as row (row.container.id)}
        <ListRow href={pathOf(row.container)} onactivate={() => onnavigate?.(pathOf(row.container))}>
          {#snippet leading()}
            <Badge icon={row.container.type === 'HUB' ? 'hub' : 'collection'}>
              {row.parent ? t('app.archive.in_hub', { hub: row.parent.name }) : t('app.archive.a_hub')}
            </Badge>
          {/snippet}
          {row.container.name}
          {#snippet trailing()}
            <Button
              size="sm"
              tone="secondary"
              icon="rotate-ccw"
              isBusy={restoring === row.container.id}
              busyLabel={t('app.workspace.saving')}
              onclick={() => void bringBack(row.container)}
            >
              {t('app.archive.unarchive', { name: row.container.name })}
            </Button>
          {/snippet}
        </ListRow>
      {/each}
    </Stack>
  {/if}

  <!-- Entries: they stay in their lists, and the search lists all of them with the archive filter. -->
  <Stack gap="100">
    <h2 class="section">{t('app.archive.entries_title')}</h2>
    <p class="quiet">{t('app.archive.entries')}</p>
    <ListRow href={ARCHIVED_ENTRIES} onactivate={() => onnavigate?.(ARCHIVED_ENTRIES)}>
      {#snippet leading()}<Icon name="search" size="sm" />{/snippet}
      {t('app.archive.entries_search')}
    </ListRow>
  </Stack>
</Stack>

<style>
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; max-width: 64ch; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
