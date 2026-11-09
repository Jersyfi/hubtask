<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Write a rule* as a Dialog M (page-system §5.1): the class of data, what happens to it, after
  // how long, and why longer than the ceiling.
  //
  // **A rule is written switched off, then previewed.** `:preview` is a `POST` on a stored rule, so
  // the rule has to exist before anybody can see what it would catch - and one that existed *and
  // was enabled* before it had been looked at is what the preview exists to prevent. Arming it is
  // a second, deliberate press on the rule.
  //
  // **The mode a broad rule lands in is said where it lands** (`data-retention.md` §7): a new rule
  // whose first run would affect more than five per cent is stored as `NOTIFY_ONLY` whatever was
  // asked. The caller compares what came back with what was asked and says so.
  //
  // **The kinds come from the manifest**: a list typed here would be the one place that still
  // needed a code change for a new kind. One whose `actions` is empty is named and removed by
  // nothing, and the dialog says so rather than offering a rule that would be refused.
  import { Button, Dialog, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';
  import type { RetentionDataKind as DataKind } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { policies, type Policy, type RetentionAction } from '../data/policies.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { actionWord, kindWord } from './words.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** The rule as written, and the action asked for where the server stored another. */
    onWritten: (policy: Policy, asked: string) => void;
  }

  let { isOpen = $bindable(false), onWritten }: Props = $props();

  let kind = $state('');
  let days = $state('30');
  let action = $state<string>('NOTIFY_ONLY');
  let justification = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  /** The catalogue, from the manifest and from nowhere else. */
  const kinds = $derived<readonly DataKind[]>(manifest.value?.retention_data_kinds ?? []);
  const chosen = $derived(kinds.find((each) => each.data_kind === kind));
  /** What this build can do to the chosen kind. Empty means the catalogue names it and nothing removes it. */
  const kindActions = $derived<readonly string[]>(chosen?.actions ?? []);

  function reset(): void {
    kind = '';
    days = '30';
    action = 'NOTIFY_ONLY';
    justification = '';
    failure = undefined;
  }

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => kind !== '' || days !== '30' || justification.trim() !== '', close);

  async function write(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const retained = Number.parseInt(days, 10);
    if (!kind || !Number.isFinite(retained)) return;
    failure = undefined;
    isWorking = true;
    try {
      const asked = action;
      const written = await policies.create({
        scope: { kind: 'TENANT' },
        data_kind: kind,
        retain_days: retained,
        action: asked as RetentionAction,
        enabled: false,
        ...(justification.trim() ? { justification: justification.trim() } : {}),
      });
      announcer.say(t('app.retention.written_announced'));
      close();
      onWritten(written, asked);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.retention.write_title')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t('app.retention.write'), isBusy: isWorking, busyLabel: t('app.retention.writing'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={write}>
    <Stack gap="150">
      {#if failure}
        <FailureNote problem={failure}>
          {#if failure.fields.size > 0}
            <Stack gap="050">{#each [...failure.fields] as [path, message] (path)}<span>{message}</span>{/each}</Stack>
          {/if}
        </FailureNote>
      {/if}
      <p class="quiet">{t('app.retention.write_note')}</p>
      <Select
        label={t('app.retention.kind')}
        bind:value={kind}
        placeholder={t('app.retention.choose_kind')}
        options={kinds.map((each) => ({ value: each.data_kind ?? '', label: kindWord(each.data_kind) }))}
      />
      {#if kind && kindActions.length === 0}
        <!-- Named by the catalogue and removed by nothing: not a kind that does not exist, and a
             rule for it would be refused. -->
        <p class="warning">{t('app.retention.kind_not_swept')}</p>
      {:else if kind}
        <Select
          label={t('app.retention.action')}
          bind:value={action}
          options={kindActions.map((each) => ({ value: each, label: actionWord(each) }))}
        />
      {/if}
      <Input
        label={t('app.retention.days')}
        hint={chosen?.max_days ? t('app.retention.days_hint_ceiling', { days: String(chosen.max_days) }) : t('app.retention.days_hint')}
        type="number"
        bind:value={days}
      />
      <Textarea
        label={t('app.retention.justification')}
        hint={t('app.retention.justification_hint')}
        bind:value={justification}
        rows={2}
      />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
  .warning { margin: 0; color: var(--text-warning); font-size: var(--fs-075); }
</style>
