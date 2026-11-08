<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Remove this schedule?* as a Dialog S. Removing is final, so it asks; switching a schedule off is
  // the reversible way and stays the row's switch. The archives it wrote stay where they are.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup, type Schedule } from '../data/backup.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    schedule: Schedule | undefined;
    onClose: () => void;
    onRemoved?: () => void;
  }

  const { schedule, onClose, onRemoved }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function remove(): Promise<void> {
    if (!schedule) return;
    failure = undefined;
    isWorking = true;
    try {
      await backup.deleteSchedule(schedule.id);
      announcer.say(t('app.backup.schedule_removed_announced'));
      onClose();
      onRemoved?.();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={schedule !== undefined}
  title={t('app.backup.remove_schedule_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.backup.remove_confirm'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.backup.removing'), onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.backup.remove_schedule_note')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
