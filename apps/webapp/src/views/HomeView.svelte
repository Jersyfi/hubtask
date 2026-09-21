<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The first screen after signing in, and the way into the workspace: the hubs.
  //
  // It used to say "the first views arrive with hubs and collections", which was true when F1 wrote
  // it and told a reader the application was not there yet for every day after F2 shipped one.
  //
  // The hubs and nothing else, deliberately. "Recently opened" and "what is due" are the two things
  // a home screen usually holds, and both need data this milestone does not have - a due date is
  // F3's, and a per-device history of what was opened is state nothing here keeps yet. A screen
  // that invented either would be a screen that has to be taken away again.
  //
  // It holds no data of its own: `AppFrame` starts `containers`, and the sidebar reads the same
  // level. A home screen that fetched the hubs a second time would be a second reader of one list.
  //
  // On the shell (ADR-0061) it is a page like the others: a `PageHeader` whose one primary action
  // is what a workspace page is for - a hub - the same dialog the tree's button opens (issue 879),
  // and the title handed to the bar on a phone.

  import { EmptyState, ErrorState, ListRow, PageHeader, Skeleton, Stack } from '@hubtask/design-system/components';

  import { containers } from '../lib/data/containers.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import CreateContainerDialog from '../lib/workspace/CreateContainerDialog.svelte';

  interface Props {
    onnavigate?: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  let isCreatingHub = $state(false);

  $effect(() => page.entitle(t('app.workspace.title')));

  const failure = $derived(
    containers.hubsState.status === 'failed'
      ? renderProblem(containers.hubsState.error, messages)
      : undefined,
  );
</script>

<Stack gap="200">
  <PageHeader
    title={t('app.workspace.title')}
    isTitleInBar={viewport.isCompact}
    primary={{ label: t('app.workspace.create_hub'), icon: 'plus', onclick: () => (isCreatingHub = true), opener: 'add-hub' }}
  />

  {#if containers.hubsState.status === 'loading' || containers.hubsState.status === 'idle'}
    <div aria-busy="true"><Skeleton lines={4} /></div>
  {:else if failure}
    <ErrorState
      title={failure.message}
      reference={failure.reference}
      referenceLabel={t('app.reference')}
      retryLabel={t('app.retry')}
      onRetry={() => containers.refresh()}
    />
  {:else if containers.hasNoHubs}
    <!-- `unused` and not `filtered`, for the reason the sidebar gives: nothing is filtering this. -->
    <EmptyState kind="unused" title={t('app.workspace.no_hubs')} icon="hub" />
  {:else}
    <Stack gap="050">
      {#each containers.hubs as hub (hub.id)}
        <ListRow href={`/hubs/${hub.id}`}>{hub.name}</ListRow>
      {/each}
    </Stack>
  {/if}
</Stack>

<CreateContainerDialog bind:isOpen={isCreatingHub} type="HUB" oncreated={(id) => onnavigate?.(`/hubs/${id}`)} />
