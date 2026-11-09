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
  // not here: a screen called *On this device* holds only what this device keeps. A settings page of
  // one section (page-system §9.5), each choice a row; a choice applies at once, so no save bar.

  import { untrack } from 'svelte';

  import { EmptyState, Radio, SettingRow, SettingsLayout, SettingsSection } from '@hubtask/design-system/components';

  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';

  import { actor } from '../lib/data/account.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { device } from '../lib/device.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const account = $derived(actor.account);
  const title = $derived(profileTitle('appearance'));
  $effect(() => page.entitle(title));

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
  <SettingsLayout
    head={profileHead('appearance')}
    nav={{ label: t('app.settings_page.sections', { name: title }), sections: [{ id: 'appearance', label: title, href: '/profile/appearance' }], current: 'appearance' }}
    view="detail"
    back={{ label: t('app.nav.profile'), onback: () => onnavigate?.('/profile') }}
  >
    <SettingsSection title={t('app.profile.device_title')} description={t('app.profile.device_hint')}>
      <!-- Wrapped: a `Radio` group takes no attribute of its own, and the palette lands on the row. -->
      <div data-setting="theme">
        <SettingRow label={t('app.profile.theme')} fieldWidth="lg">
          {#snippet control()}
            <Radio label={t('app.profile.theme')} bind:value={themeChoice} options={THEMES.map((each) => ({ value: each, label: t(`app.profile.theme_${each}`) }))} />
          {/snippet}
        </SettingRow>
      </div>
      <div data-setting="motion">
        <SettingRow label={t('app.profile.motion')} help={t('app.profile.motion_hint')} fieldWidth="lg">
          {#snippet control()}
            <Radio label={t('app.profile.motion')} bind:value={motionChoice} options={MOTIONS.map((each) => ({ value: each, label: t(`app.profile.motion_${each}`) }))} />
          {/snippet}
        </SettingRow>
      </div>
      <!-- How an entry opens from a list (design-system.md §11.8): beside it, or whole. The same
           switch is in the panel's head, where the person notices they want it. -->
      <div data-setting="open">
        <SettingRow label={t('app.profile.open')} help={t('app.profile.open_hint')} fieldWidth="lg">
          {#snippet control()}
            <Radio label={t('app.profile.open')} bind:value={openChoice} options={OPENINGS.map((each) => ({ value: each, label: t(`app.profile.open_${each}`) }))} />
          {/snippet}
        </SettingRow>
      </div>
    </SettingsSection>
  </SettingsLayout>
{/if}
