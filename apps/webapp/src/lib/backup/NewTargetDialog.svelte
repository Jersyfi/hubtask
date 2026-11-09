<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Add a target* as a Dialog M (page-system §9.3, §9.4): the name, the kind and what the kind
  // needs; the rare fields under *Advanced*. The workspace's targets and the installation's alike:
  // the caller says where the target is created.
  import { Button, Dialog } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup, type TargetDraft } from '../data/backup.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import TargetForm from './TargetForm.svelte';

  interface Props {
    isOpen: boolean;
    /** Creates the target where it belongs: the workspace's by default. */
    create?: (draft: TargetDraft) => Promise<unknown>;
    /** What is announced once it is added. */
    addedLabel?: string;
  }

  let { isOpen = $bindable(false), create = (draft) => backup.createTarget(draft), addedLabel }: Props = $props();

  let form = $state<HTMLFormElement | undefined>(undefined);
  let isTouched = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  /** A new form each time the dialog opens, so a closed draft never comes back. */
  let round = $state(0);

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => isTouched, close);

  async function add(draft: TargetDraft): Promise<boolean> {
    failure = undefined;
    isWorking = true;
    try {
      await create(draft);
      announcer.say(addedLabel ?? t('app.backup.target_added_announced'));
      close();
      return true;
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
      return false;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.backup.add_target')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={() => { failure = undefined; round += 1; }}
  primary={{ label: t('app.backup.add_target'), isBusy: isWorking, busyLabel: t('app.backup.adding'), onclick: () => form?.requestSubmit() }}
>
  {#if failure}<FailureNote problem={failure} />{/if}
  {#key round}
    <TargetForm onadd={add} bind:form bind:isTouched />
  {/key}
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />
