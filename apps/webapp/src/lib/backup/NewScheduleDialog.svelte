<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New schedule* as a Dialog M (page-system §9.3): the target, when it runs - chosen, not
  // written as RFC 5545 (user-experience.md §5.4) -, the zone it is read in, and the floor under
  // every generation. When the rule next falls is the server's to work out.
  import { tick } from 'svelte';

  import { Button, Dialog, Input, Select, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { backup, type Target } from '../data/backup.svelte.ts';
  import FrequencyPicker from '../forms/FrequencyPicker.svelte';
  import ZonePicker from '../forms/ZonePicker.svelte';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { deviceZone } from '../i18n/zone.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    isOpen: boolean;
    targets: readonly Target[];
  }

  let { isOpen = $bindable(false), targets }: Props = $props();

  const NIGHTLY = 'FREQ=DAILY;BYHOUR=3;BYMINUTE=0';

  // Empty rather than undefined: Svelte refuses a binding whose initial value is undefined.
  let target = $state('');
  let rule = $state(NIGHTLY);
  let zone = $state(deviceZone());
  let minKeep = $state('3');
  let missing = $state<Missing>({});
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  function reset(): void {
    target = '';
    rule = NIGHTLY;
    zone = deviceZone();
    minKeep = '3';
    missing = {};
    failure = undefined;
  }

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => target !== '' || rule !== NIGHTLY || minKeep !== '3', close);

  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = check([{ kind: 'field', key: 'target', code: 'app.backup.target_needed', isFilled: target !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    const floor = Number.parseInt(minKeep, 10);
    failure = undefined;
    isWorking = true;
    try {
      await backup.createSchedule({
        target_id: target,
        scope: { kind: 'TENANT' },
        rrule: rule.trim(),
        timezone: zone,
        ...(Number.isFinite(floor) && floor > 0 ? { retention: { min_keep: floor } } : {}),
      });
      announcer.say(t('app.backup.schedule_added_announced'));
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
  title={t('app.backup.add_schedule')}
  bind:isOpen
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t('app.backup.add_schedule'), isBusy: isWorking, busyLabel: t('app.backup.adding'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={add}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Select
        label={t('app.backup.schedule_target')}
        bind:value={target}
        placeholder={t('app.backup.choose_target')}
        options={targets.map((each) => ({ value: each.id, label: each.name }))}
        error={missing.target && !target ? t(missing.target) : undefined}
      />
      <FrequencyPicker label={t('app.backup.schedule_rule')} rawHint={t('app.backup.schedule_rule_hint')} bind:rule />
      <ZonePicker label={t('app.backup.schedule_zone_field')} bind:value={zone} />
      <Input
        label={t('app.backup.schedule_min_keep')}
        hint={t('app.backup.schedule_min_keep_hint')}
        type="number"
        bind:value={minKeep}
      />
      <p class="quiet">{t('app.backup.rule_is_the_servers')}</p>
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
</style>
