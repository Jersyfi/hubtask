<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The owner's way out without a target (UC-BAK-07 check 7) as a Dialog M from the head's ⋯: the
  // whole workspace as one file, once in 24 hours, behind a fresh proof - `stepUp.around` turns the
  // refusal into the prompt. The passphrase encrypts the file to it and is stored nowhere.
  import { Button, Dialog, Input, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup } from '../data/backup.svelte.ts';
  import { stepUp } from '../data/stepup.svelte.ts';
  import { assertZipComplete, zipComplete } from '../data/zipComplete.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { platform } from '../platform/index.ts';
  import { renderProblem } from '../problem.ts';

  let { isOpen = $bindable(false) }: { isOpen: boolean } = $props();

  let passphrase = $state('');
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => passphrase !== '', close);

  async function download(): Promise<void> {
    // The picker first, while the press still counts: a whole workspace goes to the disk as it
    // arrives where the browser offers that, and through memory only where it does not.
    const target = await platform.saveStream('hubtask-export.zip');
    if (target === null) return;
    failure = undefined;
    isWorking = true;
    try {
      if (target === undefined) {
        const file = await stepUp.around((token) => backup.downloadWorkspace(token, passphrase));
        await assertZipComplete(file.body);
        platform.saveFile(file.body, file.fileName ?? 'hubtask-export.zip');
      } else {
        // Kept only when whole: an archive the server broke off is discarded, not left looking saved.
        const sink = zipComplete(target);
        let isWritten = false;
        try {
          await stepUp.around((token) => backup.downloadWorkspaceTo(sink, token, passphrase));
          isWritten = true;
        } finally {
          // A refusal or a closed proof prompt never touched the file; it is discarded all the same.
          if (!isWritten) await sink.abort().catch(() => undefined);
        }
      }
      passphrase = '';
      announcer.say(t('app.backup.download_done'));
      close();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.backup.download_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={() => { passphrase = ''; failure = undefined; }}
  primary={{ label: t('app.backup.download'), isBusy: isWorking, busyLabel: t('app.backup.downloading'), onclick: () => void download() }}
>
  <Stack gap="150">
    {#if failure}<FailureNote problem={failure} />{/if}
    <p class="body">{t('app.backup.download_intro')}</p>
    <Input
      label={t('app.backup.download_passphrase')}
      hint={t('app.backup.download_passphrase_hint')}
      type="password"
      autocomplete="new-password"
      revealLabel={t('app.backup.download_passphrase_show')}
      hideLabel={t('app.backup.download_passphrase_hide')}
      bind:value={passphrase}
    />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .body { margin: 0; }
</style>
