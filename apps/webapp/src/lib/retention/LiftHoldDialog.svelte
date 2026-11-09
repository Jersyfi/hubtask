<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Lift the hold* as a Dialog S of its one field: why it is lifted, which is kept beside it and
  // is what an auditor reads.
  import { tick } from 'svelte';

  import { Button, Dialog, Input } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { policies, type Hold } from '../data/policies.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    hold: Hold | undefined;
    /** What the hold covers, in words. */
    covers: string;
    onClose: () => void;
  }

  const { hold, covers, onClose }: Props = $props();

  let reason = $state('');
  let missing = $state<Missing>({});
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  async function lift(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!hold) return;
    missing = check([{ kind: 'field', key: 'reason', code: 'app.forms.required', isFilled: reason.trim() !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    failure = undefined;
    isWorking = true;
    try {
      await policies.release(hold.id, reason.trim());
      announcer.say(t('app.retention.released_announced'));
      onClose();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="s"
  isOpen={hold !== undefined}
  title={t('app.retention.lift_title', { covers })}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { reason = ''; missing = {}; failure = undefined; onClose(); }}
  primary={{ label: t('app.retention.release_confirm'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.retention.releasing'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={lift}>
    {#if failure}<FailureNote problem={failure} />{/if}
    <Input label={t('app.retention.release_reason')} bind:value={reason} isRequired error={missing.reason && !reason.trim() ? t(missing.reason) : undefined} />
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  form { margin: 0; display: flex; flex-direction: column; gap: var(--sp-150); }
</style>
