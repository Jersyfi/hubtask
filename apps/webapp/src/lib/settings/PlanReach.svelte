<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a configuration plan reaches beyond the configuration (config-document.md §2), drawn the
  // same wherever a plan is shown before it applies: a rollback, a template, a sandbox's promotion,
  // an agent's proposal.

  import { reachLines, type ConfigReach } from '../data/configreach.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  let { reach }: { reach: readonly ConfigReach[] | null | undefined } = $props();

  const lines = $derived(reachLines(reach));
</script>

{#if lines.length > 0}
  <ul class="reach" data-reach>
    {#each lines as line, index (index)}
      <li>{t(line.code, line.params)}</li>
    {/each}
  </ul>
{/if}

<style>
  .reach { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); color: var(--text-secondary); }
</style>
