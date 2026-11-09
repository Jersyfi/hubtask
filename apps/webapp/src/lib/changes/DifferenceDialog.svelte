<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One changed block as a Dialog L: the block in words, both sides of the file's text beside each
  // other for the expert, the path under *Details for support* (PS-15).
  import { Button, Dialog, Disclosure, Stack } from '@hubtask/design-system/components';

  import { differenceWords, type BlockDifference } from '../data/configdiff.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    difference: BlockDifference | undefined;
    from: string;
    to: string;
    onClose: () => void;
  }

  const { difference, from, to, onClose }: Props = $props();

  const blockText = (block: Record<string, unknown> | undefined) => (block ? JSON.stringify(block, null, 2) : '');
  const title = $derived.by(() => {
    if (!difference) return '';
    const said = differenceWords(difference);
    return t(said.code, { section: t(said.section), key: said.module ? t(said.module) : said.key });
  });
</script>

<Dialog size="l" isOpen={difference !== undefined} {title} dismissLabel={t('app.page.close')} {onClose}>
  {#if difference}
    <Stack gap="150">
      <div class="sides" data-difference={difference.path}>
        <pre class="side" aria-label={t('app.changes.compare_version', { version: from })}>{blockText(difference.before)}</pre>
        <pre class="side" aria-label={t('app.changes.compare_version', { version: to })}>{blockText(difference.after)}</pre>
      </div>
      <Disclosure tone="quiet" label={t('app.audit.support')}>
        <p class="mono">{difference.path}</p>
      </Disclosure>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .sides { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, var(--layout-compare-side)), 1fr)); gap: var(--sp-100); }
  .side {
    margin: 0;
    padding: var(--sp-100);
    min-block-size: var(--sp-300);
    overflow-x: auto;
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }
  .mono { margin: 0; font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
</style>
