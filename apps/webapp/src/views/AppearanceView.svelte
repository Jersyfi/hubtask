<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the product looks and moves at this reader, on this device.
  //
  // **Two of the three belong to the device rather than to the account** (ADR-0043): the theme and
  // reduced motion apply at once and are kept in this browser, because how bright a screen should
  // be is a question about that screen. The account is not asked, because there is nothing above
  // the device to resolve to. A radio group rather than a select: three words, all visible, one
  // press.
  //
  // **The third is the account's** (F6-12): somebody who switched the moments off switched them
  // off everywhere, so it is written to the account and put back when a write is refused - the
  // control never shows a choice the server did not take.

  import { untrack } from 'svelte';

  import { EmptyState, Radio, Stack, Switch } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { preferences } from '../lib/data/preferences.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { device } from '../lib/device.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);
  const accountId = $derived(account?.id);

  const THEMES = ['system', 'light', 'dark'] as const;
  const MOTIONS = ['system', 'reduced'] as const;

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  /** The radio groups' bound values; the device store is told when they move, and says so. */
  let themeChoice = $state<string>(device.theme);
  let motionChoice = $state<string>(device.motion);

  $effect(() => {
    const choice = THEMES.find((each) => each === themeChoice) ?? 'system';
    if (choice === untrack(() => device.theme)) return;
    device.setTheme(choice);
    announcer.say(t('app.profile.theme_changed_announced', { choice: t(`app.profile.theme_${choice}`) }));
  });

  $effect(() => {
    const choice = MOTIONS.find((each) => each === motionChoice) ?? 'system';
    if (choice === untrack(() => device.motion)) return;
    device.setMotion(choice);
    announcer.say(t('app.profile.motion_changed_announced', { choice: t(`app.profile.motion_${choice}`) }));
  });

  let isSavingCelebrations = $state(false);
  let celebrationsOn = $state(true);
  $effect(() => {
    celebrationsOn = account?.celebrations !== false;
  });

  async function setCelebrations(isOn: boolean) {
    if (!accountId) return;
    isSavingCelebrations = true;
    failure = undefined;
    try {
      // `true` is written as the default's own value rather than cleared: what the person chose
      // is a choice, and "on" is not the same fact as "never decided".
      await preferences.setAccount(accountId, { celebrations: isOn });
      announcer.say(t(isOn ? 'app.profile.celebrations_on_announced' : 'app.profile.celebrations_off_announced'));
    } catch (error) {
      failure = renderProblem(error as never, messages);
      celebrationsOn = account?.celebrations !== false;
    } finally {
      isSavingCelebrations = false;
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="appearance" />

    <div class="panel" data-tour="profile">
      <Stack gap="200">
        <p class="quiet">{t('app.profile.device_hint')}</p>
        <Radio
          label={t('app.profile.theme')}
          bind:value={themeChoice}
          options={THEMES.map((each) => ({ value: each, label: t(`app.profile.theme_${each}`) }))}
        />
        <Radio
          label={t('app.profile.motion')}
          hint={t('app.profile.motion_hint')}
          bind:value={motionChoice}
          options={MOTIONS.map((each) => ({ value: each, label: t(`app.profile.motion_${each}`) }))}
        />
        <!-- The one switch of design-system.md §7, beside the theme's and motion's - and unlike
             them the account's (ADR-0043, F6-12): a person who switched the moments off has
             switched them off everywhere. Absent means on. -->
        <Switch
          label={t('app.profile.celebrations')}
          hint={t('app.profile.celebrations_hint')}
          bind:checked={celebrationsOn}
          disabledReason={isSavingCelebrations ? t('app.workspace.saving') : undefined}
          onchange={() => void setCelebrations(celebrationsOn)}
        />
        {#if failure}<p class="failure" role="alert">{failure.message}</p>{/if}
      </Stack>
    </div>
  </Stack>
{/if}

<style>
  /* The choices stand on a surface and keep a measure of their own: a radio group as wide as the
     region is a row of words with a mile between the mark and the label (ADR-0065 decision 2). */
  .panel {
    max-inline-size: 60ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
