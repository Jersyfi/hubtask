<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A page's labels under its title (UC-DOC-01 check 9): chips for a reader, and for a writer a
  // chip that comes off and a field that adds words. Without labels a reader sees nothing (P-05).

  import { untrack } from 'svelte';
  import { Badge, IconButton, Inline } from '@hubtask/design-system/components';
  import type { PageLabels, PageRead } from '@hubtask/sync-engine';

  import { labelsPath, pages, withLabels } from '../data/pages.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    page: PageRead;
    /** A label chosen: the space's search narrowed to it. */
    onchoose?: (label: string) => void;
  }

  const { page, onchoose }: Props = $props();

  const read = untrack(() => resource<PageLabels>(labelsPath(page.id)));
  const labels = $derived(read.state.status === 'ready' ? read.state.data.labels : []);

  let typed = $state('');
  let refusal = $state<string | undefined>(undefined);

  async function write(next: readonly string[]) {
    refusal = undefined;
    try {
      await pages.setLabels(page, next);
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  async function add() {
    const next = withLabels(labels, typed);
    if (next.length === labels.length) return;
    typed = '';
    await write(next);
  }
</script>

{#if labels.length > 0 || page.writable}
  <div class="labels" data-page-labels>
    <Inline gap="050" align="center" isWrapping>
      {#each labels as label (label)}
        <span class="label">
          {#if onchoose}
            <button type="button" class="choose" onclick={() => onchoose(label)}><Badge size="sm">{label}</Badge></button>
          {:else}
            <Badge size="sm">{label}</Badge>
          {/if}
          {#if page.writable}
            <IconButton size="sm" icon="x" label={t('app.pages.label_remove', { label })} onclick={() => void write(labels.filter((each) => each !== label))} />
          {/if}
        </span>
      {/each}
      {#if page.writable}
        <input
          class="add"
          aria-label={t('app.pages.label_add')}
          placeholder={t('app.pages.label_add')}
          bind:value={typed}
          onkeydown={(event) => { if (event.key === 'Enter') { event.preventDefault(); void add(); } }}
          onblur={() => void add()}
        />
      {/if}
    </Inline>
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
  </div>
{/if}

<style>
  .label { display: inline-flex; align-items: center; }
  .choose { padding: 0; border: 0; background: none; cursor: pointer; }
  .add {
    min-inline-size: 12ch;
    border: var(--bw-hairline) solid transparent;
    border-radius: var(--r-sm);
    background: none;
    color: var(--text-primary);
    font: inherit;
    padding: var(--sp-025) var(--sp-050);
  }
  .add:hover { border-color: var(--border-subtle); }
  .add:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); }
  .refusal { margin: 0; color: var(--text-danger); }
</style>
