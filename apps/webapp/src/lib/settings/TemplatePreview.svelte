<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A template card's *Preview* (user-experience.md §5.2): what taking it creates, in words, per
  // part, folded so a gallery stays calm - and unfolded in the form that takes it, where it is the
  // plan said before the press (§3.2). The names are the template's own, as its blocks carry them.
  import { Disclosure } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { createsOf } from '../workspace/hubGallery.ts';

  interface Props {
    template: Parameters<typeof createsOf>[0];
    /** Drawn open, without its fold: the form taking the template. */
    isUnfolded?: boolean;
  }
  const { template, isUnfolded = false }: Props = $props();

  const SHOWN = 6;
  const parts = $derived(createsOf(template));
  const said = (names: readonly string[]) =>
    names.length > SHOWN ? t('app.templates.creates_more', { names: names.slice(0, SHOWN).join(', '), count: String(names.length - SHOWN) }) : names.join(', ');
</script>

{#snippet creates()}
  <dl class="creates" data-template-preview>
    {#each parts as each (each.part)}
      <div class="part">
        <dt>{t(`app.templates.creates.${each.part}`)}</dt>
        <dd>{said(each.names)}</dd>
      </div>
    {/each}
  </dl>
{/snippet}

{#if parts.length > 0 && isUnfolded}
  {@render creates()}
{:else if parts.length > 0}
  <Disclosure label={t('app.templates.preview')} tone="quiet" description={t('app.templates.preview_hint')}>
    {@render creates()}
  </Disclosure>
{/if}

<style>
  .creates { display: grid; gap: var(--sp-050); margin: 0; font-size: var(--fs-075); }
  .part { display: grid; gap: var(--sp-025); min-inline-size: 0; }
  dt { color: var(--text-secondary); font-weight: var(--fw-semibold); }
  dd { margin: 0; overflow-wrap: anywhere; }
</style>
