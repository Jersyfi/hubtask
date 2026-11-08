<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Remove the AI provider?* as a Dialog S, from the head's ⋯ (page-system §7.3): the
  // configuration and its sealed key go, and saying what that costs comes before the press.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import FailureNote from '../frame/FailureNote.svelte';
  import { t } from '../i18n/i18n.svelte.ts';
  import type { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    isWorking: boolean;
    failure?: ReturnType<typeof renderProblem>;
    /** Removes it; the dialog closes when the caller says it went. */
    onremove: () => void;
  }

  let { isOpen = $bindable(false), isWorking, failure, onremove }: Props = $props();
</script>

<Dialog
  size="s"
  bind:isOpen
  title={t('app.ai.remove_confirm')}
  dismissLabel={t('app.ai.keep')}
  primary={{ label: t('app.ai.remove_now'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.ai.removing'), onclick: onremove }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.ai.remove_cost')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (isOpen = false)}>{t('app.ai.keep')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
