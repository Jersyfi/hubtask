<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a restore did, as lines - and what an import did, because an import lands through the
  // restore and reports in its shape (backup-restore.md §9). One renderer, so that the count a
  // restore calls "left as they are" is the count an import calls the same.
  //
  // A count of zero is skipped rather than written: a mode that cannot produce one would
  // otherwise report seven zeros, and an empty report says so in one line.

  import type { Report } from '../data/restore.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    report: Report | undefined;
  }

  const { report }: Props = $props();

  const lines = $derived(
    (
      [
        { kind: 'new', count: report?.new ?? 0 },
        { kind: 'overwritten', count: report?.overwritten ?? 0 },
        { kind: 'skipped', count: report?.skipped ?? 0 },
        { kind: 'duplicated', count: report?.duplicated ?? 0 },
        { kind: 'conflicts', count: report?.conflicts ?? 0 },
        { kind: 'deleted', count: report?.deleted ?? 0 },
        { kind: 'media', count: report?.media ?? 0 },
      ] as const
    ).filter((line) => line.count > 0),
  );
</script>

<ul class="report">
  {#each lines as line (line.kind)}
    <li class="quiet small">
      {t(`app.restore.report_${line.kind}`, { count: String(line.count) })}
    </li>
  {:else}
    <li class="quiet small">{t('app.restore.report_nothing')}</li>
  {/each}
</ul>

{#each Object.entries(report?.withheld ?? {}) as [reason, count] (reason)}
  <!-- What was deliberately not brought back, and why. -->
  <p class="quiet small">
    {messages.has(`app.restore.withheld_${reason}`)
      ? t(`app.restore.withheld_${reason}`, { count: String(count) })
      : t('app.restore.withheld_other', { reason, count: String(count) })}
  </p>
{/each}

<style>
  .report { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }
</style>
