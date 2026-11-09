<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Remove this target?* as a Dialog S. The one sentence somebody deleting a target needs: the
  // archives stay where they are - Hubtask never deletes a file it did not write, nor the ones it
  // did on the way out. A target a schedule names is refused, and the refusal is said here.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup, type Target } from '../data/backup.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    target: Target | undefined;
    onClose: () => void;
    onRemoved?: () => void;
  }

  const { target, onClose, onRemoved }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function remove(): Promise<void> {
    if (!target) return;
    failure = undefined;
    isWorking = true;
    try {
      await backup.deleteTarget(target.id);
      announcer.say(t('app.backup.target_removed_announced'));
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
  isOpen={target !== undefined}
  title={t('app.backup.remove_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.backup.remove_confirm'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.backup.removing'), onclick: () => void remove() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.backup.remove_note')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
