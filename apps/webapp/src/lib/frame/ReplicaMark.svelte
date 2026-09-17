<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The one line a screen says when what it shows came from this device's copy rather than from
  // the server (F6-04): as of when the copy was last synchronised, through the `Intl` formats
  // F5-09 built. One component in four places - the list, the board, the tree, the entry - so
  // that "shown from the copy" is one sentence and not four.
  //
  // It draws nothing for a state that came from the server, so a caller renders it unconditionally
  // beside the data and never has to ask where the data came from.

  import type { ResourceState } from '@hubtask/sync-engine';

  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  const { state }: { state: ResourceState<unknown> | undefined } = $props();

  const asOf = $derived(
    state?.status === 'ready' && state.source === 'replica'
      ? formatDateTime(new Date(state.at).toISOString(), messages.locale)
      : undefined,
  );
</script>

{#if asOf !== undefined}
  <p class="replica" role="status">{t('app.offline.as_of', { moment: asOf })}</p>
{/if}

<style>
  .replica {
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }
</style>
