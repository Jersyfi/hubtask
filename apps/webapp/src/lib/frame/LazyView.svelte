<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A screen loaded when its route is first reached (F-10, route-level code splitting): the
  // administration, the instance level and the settings are their own chunks, so the first paint
  // of the work carries none of them. A chunk the module system has loaded is cached by it, so
  // a second visit draws at once.
  //
  // A chunk that cannot be fetched - offline, or an address kept across a deploy that removed it -
  // says so with a way back, rather than leaving the frame empty.
  import type { Component } from 'svelte';
  import { t } from '../i18n/i18n.svelte.ts';

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
  <View {...props} />
{:catch}
  <h1>{t('app.view_unavailable')}</h1>
  <p><a href="/">{t('app.back_to_start')}</a></p>
{/await}
