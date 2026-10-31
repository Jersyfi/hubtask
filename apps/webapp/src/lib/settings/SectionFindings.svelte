<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The configuration check's findings on the page of the blocks they are about (configuration/ui.md
  // A5, UC-CFG-01 check 10): the same `/config/check` the overview reads, narrowed to the sections
  // this page configures, so the page and the overview never disagree. Drawn only for a reader the
  // check answers (P-05), and only when there is something to say.

  import { untrack } from 'svelte';
  import { Badge } from '@hubtask/design-system/components';
  import type { ConfigCheck } from '@hubtask/sync-engine';

  import { findingsIn } from '../administration/overview.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { CONFIG_CHECK } from '../data/touches.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The document's sections the page configures: `types`, `workflows`, … */
    sections: readonly string[];
    /** A hub whose own document the page configures; the page's own gate stands for this one. */
    hubId?: string;
  }

  const { sections, hubId }: Props = $props();

  const mayRead = $derived(hubId !== undefined || holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true);
  let check = $state<ConfigCheck>();
  $effect(() => {
    if (!mayRead) return;
    const path = hubId ? `${CONFIG_CHECK}?hub_id=${encodeURIComponent(hubId)}` : CONFIG_CHECK;
    return untrack(() => engine.subscribe<ConfigCheck>({ path }, (next) => {
      check = next.status === 'ready' ? next.data : undefined;
    }));
  });
  const findings = $derived(findingsIn(check?.findings ?? [], sections));
</script>

{#if mayRead && findings.length > 0}
  <section class="findings" aria-labelledby="section-findings" data-testid="section-findings">
    <h2 id="section-findings">{t('app.admin.check_title')}</h2>
    <ul>
      {#each findings as finding, index (index)}
        <li>
          <span>{t(finding.code, finding.params ?? {})}<span class="quiet"> · {finding.path}</span></span>
          <Badge tone={finding.severity === 'ERROR' ? 'danger' : 'warning'}>
            {t(finding.severity === 'ERROR' ? 'app.admin.check_error' : 'app.admin.check_warning')}
          </Badge>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .findings {
    display: grid;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
  }
  h2 { margin: 0; font-size: var(--fs-200); }
  ul { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }
  li {
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto;
    align-items: start;
    gap: var(--sp-100);
  }
  .quiet { color: var(--text-secondary); }
</style>
