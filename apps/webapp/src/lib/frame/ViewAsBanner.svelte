<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The frame's banner while the session shows the app as another person (UC-ID-29): on every page,
  // naming them and the end, with the one act a view may make.

  import { Banner, Button } from '@hubtask/design-system/components';

  import { viewAs } from '../data/viewas.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  let isEnding = $state(false);
  let failure = $state<string | undefined>(undefined);

  async function end(): Promise<void> {
    isEnding = true;
    failure = undefined;
    try {
      await viewAs.end();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      isEnding = false;
    }
  }
</script>

{#if viewAs.running}
  <div class="view-as" data-view-as="frame">
    <Banner tone="warning">
      {t('app.view_as.banner', {
        name: viewAs.running.display_name ?? '',
        until: formatDateTime(viewAs.running.until ?? '', messages.locale),
      })}
      {#if failure}<span class="failure">{failure}</span>{/if}
      {#snippet action()}
        <Button size="sm" isBusy={isEnding} busyLabel={t('app.view_as.ending')} onclick={() => void end()}>
          {t('app.view_as.end')}
        </Button>
      {/snippet}
    </Banner>
  </div>
{/if}

<style>
  .view-as { margin-block-end: var(--sp-200); }
  .failure { margin-inline-start: var(--sp-100); }
</style>
