<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Starting an erasure is the one start that cannot be undone, so it is confirmed first as a
  // Dialog M, naming the person, what the chosen mode removes and what a legal hold will keep (P-04,
  // UC-PRV-03 checks 8 and 11). Nothing is sent before the confirmation.
  import { Button, Dialog, Stack } from '@hubtask/design-system/components';
  import type { ErasureKept, ErasureLine } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { keptPartPhrase, privacy, type Request } from '../data/privacy.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ErasureLines from '../workspace/ErasureLines.svelte';
  import { subjectOf } from './words.svelte.ts';

  interface Props {
    request: Request | undefined;
    onClose: () => void;
  }

  const { request, onClose }: Props = $props();

  let previewed = $state<'checking' | 'unknown' | readonly ErasureKept[]>('checking');
  let lines = $state<readonly ErasureLine[]>([]);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => {
    const asked = request;
    previewed = 'checking';
    lines = [];
    failure = undefined;
    if (!asked) return;
    void privacy.preview(asked.id, asked.erasure_mode ?? 'ANONYMIZE').then((answer) => {
      if (request?.id !== asked.id) return;
      previewed = answer ? answer.kept : 'unknown';
      lines = answer?.lines ?? [];
    });
  });

  const scopeWord = (part: ErasureKept) => t(`app.privacy.hold_scope_${part.hold_scope.kind.toLowerCase()}`);

  async function start(): Promise<void> {
    if (!request) return;
    failure = undefined;
    isWorking = true;
    try {
      await privacy.change(request.id, { status: 'IN_PROGRESS' });
      announcer.say(t('app.privacy.started_announced'));
      onClose();
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.privacy.confirm_erase_title', { name: request ? subjectOf(request) : '' })}
  isOpen={request !== undefined}
  dismissLabel={t('app.privacy.confirm_erase_cancel')}
  {onClose}
  primary={{ label: t('app.privacy.confirm_erase'), tone: 'danger', isBusy: isWorking, busyLabel: t('app.privacy.starting'), onclick: () => void start() }}
>
  <Stack gap="100">
    <!-- A refused start is said here: the page's own banner sits behind the dialog. -->
    {#if failure}<FailureNote problem={failure} />{/if}
    <p>{t(`app.privacy.confirm_erase_${(request?.erasure_mode ?? 'ANONYMIZE').toLowerCase()}`, { name: request ? subjectOf(request) : '' })}</p>
    {#if previewed === 'checking'}
      <p class="quiet small">{t('app.privacy.preview_checking')}</p>
    {:else if previewed === 'unknown'}
      <p class="quiet small">{t('app.privacy.preview_unknown')}</p>
    {:else if previewed.length === 0}
      <p class="quiet small">{t('app.privacy.preview_none')}</p>
    {:else}
      <p>{t('app.privacy.preview_intro')}</p>
      {#each previewed as part (part.hold_id)}
        {@const line = keptPartPhrase(part, scopeWord(part))}
        <p class="small">{t(line.code, line.params)}</p>
      {/each}
    {/if}
    <ErasureLines {lines} />
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.privacy.confirm_erase_cancel')}</Button>
  {/snippet}
</Dialog>

<style>
  p { margin: 0; }
  .quiet { color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
</style>
