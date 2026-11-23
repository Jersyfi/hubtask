<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The views setting's note on columns no board shows (UC-WRK-05 check 13): they still decide
  // whether an entry is done, so the settings say how many there are and offer *Remove columns* -
  // the collection's own `board:switch` to NONE, previewed and confirmed in the level dialog.
  import { untrack } from 'svelte';
  import { Button, Callout, Stack } from '@hubtask/design-system/components';
  import type { Container, StateMode } from '@hubtask/sync-engine';

  import { buckets } from '../data/buckets.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import BoardLevelDialog from '../entries/BoardLevelDialog.svelte';
  import { t } from '../i18n/i18n.svelte.ts';
  import { unusedColumns } from './unusedColumns.ts';

  interface Props {
    container: Container;
    /** The views the collection offers as saved. */
    offered: readonly string[];
    mayShape: boolean;
  }

  const { container, offered, mayShape }: Props = $props();

  const isCollection = $derived(container.type === 'COLLECTION');
  const withoutBoard = $derived(isCollection && mayShape && !offered.includes('KANBAN'));
  let level = $state<StateMode | undefined>(undefined);
  // Read only where no board is offered: everywhere else the note is never drawn.
  $effect(() => {
    if (!withoutBoard) return;
    const id = container.id;
    return untrack(() => {
      const stops = [buckets.open(id), containers.levelOf(id, (next) => (level = next))];
      return () => {
        for (const stop of stops) stop();
      };
    });
  });
  const count = $derived(unusedColumns({ isCollection, mayShape, level, columns: buckets.of(container.id).length, offered }));
  let isAsking = $state(false);
</script>

{#if count !== undefined}
  <div class="hint" data-unused-columns={count}>
    <Callout>
      <Stack gap="100">
        <span>{t('app.structure.columns_without_board', { count })}</span>
        <span><Button size="sm" onclick={() => (isAsking = true)}>{t('app.board.level.remove_confirm')}</Button></span>
      </Stack>
    </Callout>
  </div>
{/if}
{#if isAsking}
  <BoardLevelDialog collectionId={container.id} asked="remove" onclose={() => (isAsking = false)} />
{/if}

<style>
  /* In line with the setting rows around it, whose padding it takes. */
  .hint { padding: 0 var(--sp-250) var(--sp-200); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .hint { padding-inline: var(--sp-200); }
  }
</style>
