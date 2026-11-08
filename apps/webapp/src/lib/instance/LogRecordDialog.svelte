<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One row of an installation log opened as its record (page-system §7.6): what it says as label
  // and value rows, the keys only under *Details for support* (PS-15). A Dialog M: a handful of
  // facts and one fold.
  import type { Snippet } from 'svelte';

  import { Button, Dialog, Disclosure, Stack } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  export interface LogFact {
    readonly label: string;
    readonly value: string;
  }

  interface Props {
    title: string;
    isOpen: boolean;
    facts: readonly LogFact[];
    /** The keys and identifiers, folded. */
    support?: readonly LogFact[];
    /** An act of the record, beside *Close*. */
    extra?: Snippet;
    onClose: () => void;
  }

  const { title, isOpen, facts, support = [], extra, onClose }: Props = $props();
</script>

<Dialog size="m" {isOpen} {title} dismissLabel={t('app.page.close')} {onClose}>
  <Stack gap="200">
    <dl class="facts">
      {#each facts as fact (fact.label)}
        <div class="fact"><dt>{fact.label}</dt><dd>{fact.value}</dd></div>
      {/each}
    </dl>
    {#if support.length > 0}
      <Disclosure label={t('app.audit.support')} tone="quiet">
        <dl class="support">
          {#each support as fact (fact.label)}<dt>{fact.label}</dt><dd class="mono">{fact.value}</dd>{/each}
        </dl>
      </Disclosure>
    {/if}
  </Stack>
  {#snippet actions()}
    {@render extra?.()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .facts { margin: 0; display: grid; gap: var(--sp-100); }
  .fact { display: grid; grid-template-columns: minmax(0, 14ch) minmax(0, 1fr); gap: var(--sp-150); align-items: baseline; }
  .fact dt { color: var(--text-secondary); }
  .fact dd { margin: 0; overflow-wrap: anywhere; }
  .support { margin: 0; display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); font-size: var(--fs-075); }
  .support dt { color: var(--text-secondary); }
  .support dd { margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
