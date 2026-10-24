<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the product looks and moves at this reader, on this device.
  //
  // **Both belong to the device rather than to the account** (ADR-0043): the theme and reduced
  // motion apply at once and are kept in this browser, because how bright a screen should be is a
  // question about that screen. The account is not asked, because there is nothing above the device
  // to resolve to. A radio group rather than a select: three words, all visible, one press.
  //
  // "Mark the moments" is the account's, so it stands on *You* (`lib/profile/CelebrationsSwitch`),
  // not here: a screen called *On this device* holds only what this device keeps.

  import { untrack } from 'svelte';

  import { EmptyState, Radio, Stack } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { device } from '../lib/device.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  const account = $derived(actor.account);

  const THEMES = ['system', 'light', 'dark'] as const;
  const MOTIONS = ['system', 'reduced'] as const;
  const OPENINGS = ['panel', 'whole'] as const;

  /** The radio groups' bound values; the device store is told when they move, and says so. */
  let themeChoice = $state<string>(device.theme);
  let motionChoice = $state<string>(device.motion);
  let openChoice = $state<string>(device.openMode);

  $effect(() => {
    const choice = OPENINGS.find((each) => each === openChoice) ?? 'panel';
    if (choice === untrack(() => device.openMode)) return;
    device.setOpenMode(choice);
    announcer.say(t('app.profile.open_changed_announced', { choice: t(`app.profile.open_${choice}`) }));
  });

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
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="appearance" />

    <div class="panel">
      <Stack gap="200">
        <p class="quiet">{t('app.profile.device_hint')}</p>
        <!-- Wrapped: a `Radio` group takes no attribute of its own, and the palette lands on the row. -->
        <div data-setting="theme">
          <Radio
            label={t('app.profile.theme')}
            bind:value={themeChoice}
            options={THEMES.map((each) => ({ value: each, label: t(`app.profile.theme_${each}`) }))}
          />
        </div>
        <div data-setting="motion">
          <Radio
            label={t('app.profile.motion')}
            hint={t('app.profile.motion_hint')}
            bind:value={motionChoice}
            options={MOTIONS.map((each) => ({ value: each, label: t(`app.profile.motion_${each}`) }))}
          />
        </div>
        <!-- How an entry opens from a list (design-system.md §11.8): beside it, or whole. The same
             switch is in the panel's head, where the person notices they want it. -->
        <div data-setting="open">
          <Radio
            label={t('app.profile.open')}
            hint={t('app.profile.open_hint')}
            bind:value={openChoice}
            options={OPENINGS.map((each) => ({ value: each, label: t(`app.profile.open_${each}`) }))}
          />
        </div>
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
    box-shadow: var(--shadow-raised);
  }

  .quiet { margin: 0; color: var(--text-secondary); }
</style>
