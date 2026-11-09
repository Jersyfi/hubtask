<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Withdraw this rule?* as a Dialog S: final, so it asks (ux-inventory §E10), and says what
  // stays - what the rule already removed stays removed.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { policies, type Policy } from '../data/policies.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    policy: Policy | undefined;
    onClose: () => void;
  }

  const { policy, onClose }: Props = $props();

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  async function withdraw(): Promise<void> {
    if (!policy) return;
    failure = undefined;
    isWorking = true;
    try {
      await policies.withdraw(policy.id);
      announcer.say(t('app.retention.withdrawn_announced'));
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
  isOpen={policy !== undefined}
  title={t('app.retention.withdraw_title')}
  dismissLabel={t('app.workspace.cancel')}
  onClose={() => { failure = undefined; onClose(); }}
  primary={{ label: t('app.retention.withdraw'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.retention.withdrawing'), onclick: () => void withdraw() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  <p class="body">{t('app.retention.withdraw_note')}</p>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  .body { margin: 0; }
</style>
