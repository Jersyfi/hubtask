<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One schedule as a Dialog M (page-system §9.3): what its row has no room for - the zone its rule
  // is read in, its generations and the floor under them. Switching it on and off is the row's,
  // removing it the row's ⋯.
  import { Badge, Button, Dialog } from '@hubtask/design-system/components';

  import type { Schedule } from '../data/backup.svelte.ts';
  import { generations } from '../data/backup.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { nextRun, ruleSentence } from './words.svelte.ts';

  interface Props {
    schedule: Schedule | undefined;
    targetName: string;
    onClose: () => void;
  }

  const { schedule, targetName, onClose }: Props = $props();
</script>

<Dialog
  size="m"
  isOpen={schedule !== undefined}
  title={schedule ? ruleSentence(schedule) : ''}
  dismissLabel={t('app.dismiss')}
  {onClose}
>
  {#if schedule}
    <dl class="facts">
      <dt>{t('app.backup.enabled')}</dt>
      <dd>
        <Badge tone={schedule.enabled === false ? 'neutral' : 'success'}>
          {schedule.enabled === false ? t('app.backup.off') : t('app.backup.on')}
        </Badge>
      </dd>
      <dt>{t('app.backup.schedule_target')}</dt>
      <dd>{targetName}</dd>
      <dt>{t('app.backup.schedule_zone_field')}</dt>
      <dd>{schedule.timezone ?? 'UTC'}</dd>
      <dt>{t('app.backup.column_next')}</dt>
      <dd>{nextRun(schedule)}</dd>
      <dt>{t('app.backup.column_kept')}</dt>
      <dd>
        <ul class="plan">
          {#each generations(schedule.retention) as line (line.kind)}
            <li>{t(`app.backup.keep_${line.kind}`, { count: String(line.keep) })}</li>
          {/each}
          <!-- A floor under all of them rather than a sixth generation: a retention rule may never
               result in no backup being left (`backup-restore.md` §6). -->
          <li>{t('app.backup.min_keep', { count: String(schedule.retention?.min_keep ?? 3) })}</li>
        </ul>
      </dd>
    </dl>
    <p class="quiet">{t('app.backup.switch_off_rather_than_delete')}</p>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .facts { margin: 0; display: grid; grid-template-columns: max-content 1fr; gap: var(--sp-100) var(--sp-200); }
  dt { color: var(--text-secondary); }
  dd { margin: 0; }
  .plan { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .quiet { margin: var(--sp-200) 0 0; color: var(--text-secondary); font-size: var(--fs-075); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .facts { grid-template-columns: 1fr; gap: var(--sp-025); }
    dd { margin-block-end: var(--sp-100); }
  }
</style>
