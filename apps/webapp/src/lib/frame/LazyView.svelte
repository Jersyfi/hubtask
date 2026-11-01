<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A screen loaded when its route is first reached (F-10, route-level code splitting): the
  // administration, the instance level and the settings are their own chunks, so the first paint
  // of the work carries none of them. A chunk the module system has loaded is cached by it, so
  // a second visit draws at once.
  //
  // A chunk that cannot be fetched is told apart (chunkfailure.ts): after an upgrade the page
  // reloads once onto the new release, or says Hubtask was updated; offline it says so; otherwise
  // it offers to load again. A reload is the retry: a browser may keep a failed module import.
  import type { Component } from 'svelte';
  import { t } from '../i18n/i18n.svelte.ts';
  import { settle } from './chunkfailure.ts';
  import ViewFailed from './ViewFailed.svelte';

  type Props = {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- each screen has its own props
    load: () => Promise<{ default: Component<any> }>;
    props?: Record<string, unknown>;
  };
  let { load, props = {} }: Props = $props();

  const pending = $derived(load());
</script>

{#await pending then module}
  {@const View = module.default}
  <!-- Its own boundary: a screen drawn once its chunk arrived is drawn after the frame's boundary
       has settled, and a throw there would otherwise escape it. -->
  <svelte:boundary>
    <View {...props} />
    {#snippet failed(error, reset)}
      <ViewFailed {error} {reset} />
    {/snippet}
  </svelte:boundary>
{:catch}
  {#await settle() then outcome}
    {#if outcome !== 'reload'}
      <h1>{t(outcome === 'updated' ? 'app.view_updated' : outcome === 'offline' ? 'app.view_unavailable' : 'app.view_failed')}</h1>
      <p>
        <button type="button" onclick={() => location.reload()}>{t('app.view_reload')}</button>
        <a href="/">{t('app.back_to_start')}</a>
      </p>
    {/if}
  {/await}
{/await}
