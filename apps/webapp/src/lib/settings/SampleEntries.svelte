<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's sample entries in its settings (configuration.md §6, UC-CFG-01 check 12): the entries a
  // template brought to try its way of working on, and the one act that moves them all into the
  // trash. Shown only while the hub has some; entries nobody marked are never touched.

  import { untrack } from 'svelte';
  import { Banner, Button } from '@hubtask/design-system/components';
  import type { SampleEntryList } from '@hubtask/sync-engine';

  import { config, sampleEntriesPath } from '../data/config.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hubId: string;
  }

  let { hubId }: Props = $props();

  const samples = untrack(() => resource<SampleEntryList>(sampleEntriesPath(hubId)));
  const count = $derived(samples.state.status === 'ready' ? samples.state.data.samples.length : 0);
  let isRemoving = $state(false);

  async function remove() {
    isRemoving = true;
    try {
      const { removed } = await config.removeSamples(hubId);
      toast.show({ text: t('app.templates.samples_removed', { count: removed }), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isRemoving = false;
    }
  }
</script>

{#if count > 0}
  <div data-sample-entries>
    <Banner tone="info" title={t('app.templates.samples_here', { count })}>
      <p>{t('app.templates.samples_here_about')}</p>
      <Button tone="secondary" isBusy={isRemoving} data-remove-samples onclick={remove}>{t('app.templates.samples_remove')}</Button>
    </Banner>
  </div>
{/if}
